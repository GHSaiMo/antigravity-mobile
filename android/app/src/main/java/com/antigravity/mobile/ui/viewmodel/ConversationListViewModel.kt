package com.antigravity.mobile.ui.viewmodel

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.data.model.LocalDraftSession
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.CacheManager
import com.antigravity.mobile.data.service.PreferencesManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.encodeToString
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

sealed interface ConversationListUiState {
    data object Loading : ConversationListUiState
    data class Success(val conversations: List<ConversationItem>) : ConversationListUiState
    data class Error(val message: String) : ConversationListUiState
}

class ConversationListViewModel(
    private val apiClient: ApiClient,
    val prefs: PreferencesManager,
    val cacheManager: CacheManager? = null,
    private val liveActivityManager: com.antigravity.mobile.data.service.LiveActivityNotificationManager? = null
) : ViewModel() {

    private val _uiState = MutableStateFlow<ConversationListUiState>(ConversationListUiState.Loading)
    val uiState: StateFlow<ConversationListUiState> = _uiState.asStateFlow()

    private val _searchQuery = MutableStateFlow("")
    val searchQuery: StateFlow<String> = _searchQuery.asStateFlow()

    private val _quotaData = MutableStateFlow<CockpitQuotaResponse?>(null)
    val quotaData: StateFlow<CockpitQuotaResponse?> = _quotaData.asStateFlow()

    private val _isRefreshingQuota = MutableStateFlow(false)
    val isRefreshingQuota: StateFlow<Boolean> = _isRefreshingQuota.asStateFlow()

    private val _projects = MutableStateFlow<List<ProjectItem>>(loadInitialProjects())
    val projects: StateFlow<List<ProjectItem>> = _projects.asStateFlow()

    private val _isLoadingProjects = MutableStateFlow(false)
    val isLoadingProjects: StateFlow<Boolean> = _isLoadingProjects.asStateFlow()

    private val _projectsError = MutableStateFlow<String?>(null)
    val projectsError: StateFlow<String?> = _projectsError.asStateFlow()

    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    private val deletedCascadeIds = mutableSetOf<String>()
    private var rawConversations = loadInitialConversations()
    private var pollJob: kotlinx.coroutines.Job? = null

    init {
        if (prefs.isPaired()) {
            loadConversations()
            loadQuotas()
            loadProjects()
        } else {
            _uiState.value = ConversationListUiState.Success(emptyList())
        }
    }

    fun reloadFromCache() {
        prefs.purgeExpiredTombstones()
        val drafts = prefs.loadLocalDraftConversations()
        val cached = prefs.cachedConversationsJson
        val serverItems = if (!cached.isNullOrBlank()) {
            try {
                val list = apiClient.json.decodeFromString<List<ConversationItem>>(cached)
                list.filter { !it.isDraft && !it.isSubagent && !deletedCascadeIds.contains(it.id) && !prefs.isDeletedConversation(it.id) }
            } catch (e: Exception) {
                emptyList()
            }
        } else {
            emptyList()
        }
        val serverNonDrafts = if (rawConversations.any { !it.isDraft }) {
            rawConversations.filter { !it.isDraft && !deletedCascadeIds.contains(it.id) && !prefs.isDeletedConversation(it.id) }
        } else {
            serverItems
        }
        val all = drafts + serverNonDrafts
        if (all.isNotEmpty()) {
            rawConversations = all
            applyFilter()
            cacheManager?.prewarmSessions(all.take(15).map { it.id })
            liveActivityManager?.syncWithConversations(all)
        }
    }

    fun startAutoRefresh() {
        if (!prefs.isPaired()) return
        if (pollJob?.isActive == true) return
        pollJob = viewModelScope.launch {
            while (true) {
                val hasRunning = rawConversations.any { it.status.isRunning || it.status.needsAction }
                val delayMs = if (hasRunning) 4000L else 10000L
                kotlinx.coroutines.delay(delayMs)
                if (!prefs.isPaired()) break
                val convResult = apiClient.fetchConversations()
                convResult.onSuccess { list ->
                    rawConversations = mergeWithLocalConversations(list)
                    persistConversationsToCache(rawConversations)
                    applyFilter()
                    cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                    liveActivityManager?.syncWithConversations(rawConversations)
                }
            }
        }
    }

    fun stopAutoRefresh() {
        pollJob?.cancel()
        pollJob = null
    }

    private fun loadInitialConversations(): List<ConversationItem> {
        prefs.purgeExpiredTombstones()
        val drafts = prefs.loadLocalDraftConversations()
        val cached = prefs.cachedConversationsJson
        val serverItems = if (!cached.isNullOrBlank()) {
            try {
                val list = apiClient.json.decodeFromString<List<ConversationItem>>(cached)
                list.filter { !it.isDraft && !it.isSubagent && !deletedCascadeIds.contains(it.id) && !prefs.isDeletedConversation(it.id) }
            } catch (e: Exception) {
                Log.w("ConvListVM", "Failed to decode cached conversations: ${e.message}")
                emptyList()
            }
        } else {
            emptyList()
        }
        val all = drafts + serverItems
        if (all.isNotEmpty()) {
            _uiState.value = ConversationListUiState.Success(all)
            cacheManager?.prewarmSessions(all.take(15).map { it.id })
            liveActivityManager?.syncWithConversations(all)
        }
        return all
    }

    private fun persistConversationsToCache(list: List<ConversationItem>) {
        try {
            val nonDrafts = list.filter { !it.isDraft && !prefs.isDeletedConversation(it.id) }
            prefs.cachedConversationsJson = apiClient.json.encodeToString(nonDrafts)
        } catch (e: Exception) {
            Log.w("ConvListVM", "Failed to cache conversations: ${e.message}")
        }
    }

    private fun loadInitialProjects(): List<ProjectItem> {
        val cached = prefs.cachedProjectsJson
        if (!cached.isNullOrBlank()) {
            try {
                return apiClient.json.decodeFromString<List<ProjectItem>>(cached)
            } catch (e: Exception) {
                Log.w("ConvListVM", "Failed to decode cached projects: ${e.message}")
            }
        }
        return emptyList()
    }

    private fun mergeWithLocalConversations(serverList: List<ConversationItem>): List<ConversationItem> {
        prefs.purgeExpiredTombstones()
        val drafts = prefs.loadLocalDraftConversations()
        val serverFiltered = serverList.filter {
            !deletedCascadeIds.contains(it.id) &&
            !it.isSubagent &&
            !prefs.isDeletedConversation(it.id)
        }

        val existingMap = rawConversations.associateBy { it.id }

        // Enrich server items with known local titles/status/stepCount if server is lagging
        val enrichedServerItems = serverFiltered.map { serverItem ->
            val local = existingMap[serverItem.id]
            val healedLocal = if (local != null) {
                cacheManager?.healConversationTitleIfNeeded(local) ?: local
            } else {
                cacheManager?.healConversationTitleIfNeeded(serverItem) ?: serverItem
            }
            val resolvedTitle = if ((serverItem.title.isBlank() || serverItem.title == "未命名会话") &&
                healedLocal.title.isNotBlank() && healedLocal.title != "未命名会话" && healedLocal.title != "会话详情") {
                healedLocal.title
            } else {
                serverItem.title
            }
            val isRecentlyTriggeredLocally = local?.status == ConversationStatus.RUNNING &&
                (System.currentTimeMillis() - (local.lastModifiedEpochMs)) in 0..6000L
            val resolvedStatus = when {
                isRecentlyTriggeredLocally && serverItem.status == ConversationStatus.IDLE -> ConversationStatus.RUNNING
                serverItem.status != ConversationStatus.UNKNOWN -> serverItem.status
                else -> local?.status ?: ConversationStatus.IDLE
            }
            val resolvedSteps = maxOf(serverItem.stepCount, local?.stepCount ?: 0)
            val resolvedWorkspace = if (serverItem.workspaceName == "Chat" && local?.workspaceName != "Chat" && !local?.workspaceName.isNullOrBlank()) {
                local.workspaceName
            } else {
                serverItem.workspaceName
            }
            serverItem.copy(
                title = resolvedTitle,
                status = resolvedStatus,
                stepCount = resolvedSteps,
                workspaceName = resolvedWorkspace
            )
        }

        // Server list is authoritative for server conversations.
        // Any conversation deleted on computer is immediately removed.
        return drafts + enrichedServerItems
    }

    suspend fun loadInitialData(): Boolean {
        if (!prefs.isPaired()) return false
        var success = false
        try {
            val convResult = apiClient.fetchConversations()
            convResult.onSuccess { list ->
                rawConversations = mergeWithLocalConversations(list)
                persistConversationsToCache(rawConversations)
                applyFilter()
                cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                liveActivityManager?.syncWithConversations(rawConversations)
                success = true
            }.onFailure { err ->
                if (rawConversations.isNotEmpty()) {
                    applyFilter()
                    success = true
                } else {
                    _uiState.value = ConversationListUiState.Success(emptyList())
                }
            }
            apiClient.fetchCockpitQuotas().onSuccess {
                _quotaData.value = it
            }
            apiClient.fetchProjects().onSuccess { list ->
                _projects.value = list
                _projectsError.value = null
                if (list.isNotEmpty()) {
                    try {
                        prefs.cachedProjectsJson = apiClient.json.encodeToString(list)
                    } catch (_: Exception) {}
                }
            }
        } catch (e: Exception) {
            Log.w("ConvListVM", "loadInitialData failed: ${e.message}")
        }
        return success
    }

    fun refresh(onComplete: (() -> Unit)? = null) {
        if (!prefs.isPaired()) {
            _uiState.value = ConversationListUiState.Success(emptyList())
            onComplete?.invoke()
            return
        }
        viewModelScope.launch {
            _isRefreshing.value = true
            try {
                val convResult = apiClient.fetchConversations()
                convResult.onSuccess { list ->
                    rawConversations = mergeWithLocalConversations(list)
                    persistConversationsToCache(rawConversations)
                    applyFilter()
                    cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                }.onFailure { err ->
                    if (rawConversations.isEmpty()) {
                        _uiState.value = ConversationListUiState.Error(err.message ?: "无法获取会话列表")
                    }
                }
                apiClient.fetchCockpitQuotas().onSuccess {
                    _quotaData.value = it
                }
                apiClient.fetchProjects().onSuccess { list ->
                    _projects.value = list
                    _projectsError.value = null
                    if (list.isNotEmpty()) {
                        try {
                            prefs.cachedProjectsJson = apiClient.json.encodeToString(list)
                        } catch (_: Exception) {}
                    }
                }
            } finally {
                _isRefreshing.value = false
                onComplete?.invoke()
            }
        }
    }

    fun loadConversations() {
        if (!prefs.isPaired()) {
            _uiState.value = ConversationListUiState.Success(emptyList())
            return
        }
        viewModelScope.launch {
            val result = apiClient.fetchConversations()
            result.onSuccess { list ->
                rawConversations = mergeWithLocalConversations(list)
                persistConversationsToCache(rawConversations)
                applyFilter()
                cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                liveActivityManager?.syncWithConversations(rawConversations)
            }.onFailure { err ->
                if (rawConversations.isEmpty()) {
                    _uiState.value = ConversationListUiState.Error(err.message ?: "无法获取会话列表")
                }
            }
        }
    }

    fun loadQuotas() {
        viewModelScope.launch {
            apiClient.fetchCockpitQuotas().onSuccess {
                _quotaData.value = it
            }
        }
    }

    fun refreshQuotas() {
        _isRefreshingQuota.value = true
        viewModelScope.launch {
            apiClient.refreshCockpitQuotas().onSuccess {
                _quotaData.value = it
            }
            _isRefreshingQuota.value = false
        }
    }

    fun switchCockpitAccount(accountId: String) {
        viewModelScope.launch {
            apiClient.switchCockpitAccount(accountId).onSuccess {
                refreshQuotas()
                loadConversations()
            }
        }
    }

    fun loadProjects() {
        if (!prefs.isPaired()) {
            if (_projects.value.isEmpty()) {
                _projectsError.value = "请先配对电脑终端"
            }
            return
        }

        _isLoadingProjects.value = true
        _projectsError.value = null
        viewModelScope.launch {
            val result = apiClient.fetchProjects()
            result.onSuccess { list ->
                _projects.value = list
                _projectsError.value = null
                if (list.isNotEmpty()) {
                    try {
                        prefs.cachedProjectsJson = apiClient.json.encodeToString(list)
                    } catch (_: Exception) {}
                }
            }.onFailure { err ->
                Log.e("ConvListVM", "Failed to load projects: ${err.message}", err)
                if (_projects.value.isEmpty()) {
                    _projectsError.value = "拉取工作区失败: ${err.message ?: "网络或网关异常"}"
                }
            }
            _isLoadingProjects.value = false
        }
    }

    fun upsertConversation(item: ConversationItem) {
        if (item.isSubagent || deletedCascadeIds.contains(item.id)) return

        val existingIndex = rawConversations.indexOfFirst { it.id == item.id }
        val updatedList = if (existingIndex >= 0) {
            val old = rawConversations[existingIndex]
            val merged = old.copy(
                title = if (item.title.isNotBlank() && item.title != "未命名会话" && item.title != "会话详情") item.title else old.title,
                status = if (item.status != ConversationStatus.UNKNOWN) item.status else old.status,
                stepCount = maxOf(old.stepCount, item.stepCount),
                workspaceName = if (item.workspaceName.isNotBlank() && item.workspaceName != "Chat") item.workspaceName else old.workspaceName,
                lastModifiedTime = old.lastModifiedTime,
                isUnread = item.isUnread
            )
            val list = rawConversations.toMutableList()
            list[existingIndex] = merged
            list
        } else {
            listOf(item) + rawConversations
        }
        rawConversations = updatedList
        persistConversationsToCache(updatedList)
        applyFilter()
    }

    fun createLocalDraftSession(project: ProjectItem): LocalDraftSession {
        return prefs.createLocalDraftSession(project)
    }

    fun notifySessionFocus(cascadeId: String) {
        if (cascadeId.startsWith("local_draft_")) return
        apiClient.notifySessionFocus(cascadeId)
    }

    fun markConversationAsRead(cascadeId: String) {
        if (cascadeId.startsWith("local_draft_")) return
        val target = rawConversations.find { it.id == cascadeId }
        val modTime = target?.lastModifiedTime?.let { ConversationItem.parseIsoDate(it) } ?: 0L
        val viewTime = maxOf(System.currentTimeMillis(), modTime + 1000L)
        prefs.setLastViewTime(cascadeId, viewTime)

        rawConversations = rawConversations.map { item ->
            if (item.id == cascadeId) item.copy(isUnread = false) else item
        }
        persistConversationsToCache(rawConversations)
        applyFilter()

        viewModelScope.launch {
            apiClient.markConversationAsRead(cascadeId, viewTime)
        }
    }

    fun deleteConversation(cascadeId: String) {
        liveActivityManager?.cancelActivity(cascadeId)
        deletedCascadeIds.add(cascadeId)
        prefs.recordDeletedConversation(cascadeId)
        cacheManager?.deleteSession(cascadeId)
        if (cascadeId.startsWith("local_draft_")) {
            prefs.deleteLocalDraftSession(cascadeId)
            rawConversations = rawConversations.filter { it.id != cascadeId }
            persistConversationsToCache(rawConversations)
            applyFilter()
            return
        }
        rawConversations = rawConversations.filter { it.id != cascadeId }
        persistConversationsToCache(rawConversations)
        applyFilter()

        viewModelScope.launch {
            apiClient.deleteConversation(cascadeId).onSuccess {
                loadConversations()
            }
        }
    }

    fun renameConversation(cascadeId: String, newTitle: String) {
        val trimmed = newTitle.trim()
        if (trimmed.isNotBlank()) {
            cacheManager?.updateSessionTitle(cascadeId, trimmed)
            rawConversations = rawConversations.map { item ->
                if (item.id == cascadeId) item.copy(title = trimmed) else item
            }
            persistConversationsToCache(rawConversations)
            applyFilter()
        }

        viewModelScope.launch {
            apiClient.renameConversation(cascadeId, newTitle).onSuccess {
                loadConversations()
            }
        }
    }

    fun onSearchQueryChanged(query: String) {
        _searchQuery.value = query
        applyFilter()
    }

    private fun applyFilter() {
        val query = _searchQuery.value.trim().lowercase()
        val filtered = if (query.isEmpty()) {
            rawConversations
        } else {
            rawConversations.filter {
                it.title.lowercase().contains(query) ||
                        it.workspaceName.lowercase().contains(query)
            }
        }
        _uiState.value = ConversationListUiState.Success(filtered)
    }

    fun getConversation(cascadeId: String): ConversationItem? {
        return rawConversations.find { it.id == cascadeId }
    }

    fun getWorkspaceName(cascadeId: String): String? {
        return rawConversations.find { it.id == cascadeId }?.workspaceName
            ?: prefs.getLocalDraftSession(cascadeId)?.let { if (it.project.isPureChat) "Chat" else it.project.name }
    }

    fun getDraftProject(cascadeId: String): ProjectItem? {
        return rawConversations.find { it.id == cascadeId }?.draftProject
            ?: prefs.getLocalDraftSession(cascadeId)?.project
    }

    fun unpair(onComplete: () -> Unit = {}) {
        val client = apiClient
        viewModelScope.launch {
            try {
                withTimeoutOrNull(3000) {
                    client.unpair()
                }
            } catch (e: Exception) {
                Log.w("ConversationListVM", "Gateway unpair failed: ${e.message}")
            }
        }
        stopAutoRefresh()
        cacheManager?.clearAllSessions()
        prefs.clear()
        _uiState.value = ConversationListUiState.Success(emptyList())
        rawConversations = emptyList()
        _projects.value = emptyList()
        _projectsError.value = null
        _quotaData.value = null
        onComplete()
    }

    override fun onCleared() {
        super.onCleared()
        stopAutoRefresh()
    }
}
