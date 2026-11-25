package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.ui.util.loc
import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.TokenHistoryResponse
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ConversationSearchResult
import com.antigravity.mobile.data.model.GatewayCompatStore
import com.antigravity.mobile.data.model.GatewayFeature
import com.antigravity.mobile.data.model.GlobalBackgroundTaskItem
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.data.model.LocalDraftSession
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.CacheManager
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.ConversationEventsClient
import com.antigravity.mobile.data.service.PreferencesManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.encodeToString
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/** While the gateway event socket is up, pushed hints drive refreshes and polling only runs this slowly. */
private const val EVENTS_SAFETY_POLL_MS = 60_000L

/** Cooldown window for background list reordering so concurrent running sessions don't swap rows rapidly. */
private const val REORDER_COOLDOWN_MS = 8_000L

/** Auto-refresh interval for Token History tab (1 hour). */
private const val TOKEN_HISTORY_REFRESH_INTERVAL_MS = 60L * 60L * 1000L

/** Auto-refresh interval for /gateway/models catalog (3 minutes, matching iOS ModelCatalogStore). */
private const val MODEL_CATALOG_REFRESH_INTERVAL_MS = 3L * 60L * 1000L

sealed interface ConversationListUiState {
    data object Loading : ConversationListUiState
    data class Success(val conversations: List<ConversationItem>) : ConversationListUiState
    data class Error(val message: String) : ConversationListUiState
}

sealed interface WorkspaceFilter {
    data object Chat : WorkspaceFilter
    data class Workspace(val name: String) : WorkspaceFilter
    data object Archived : WorkspaceFilter

    val displayName: String
        get() = when (this) {
            is Chat -> "Chat"
            is Workspace -> name
            is Archived -> "已归档".loc
        }

    val isPureChat: Boolean
        get() = this is Chat

    val isArchived: Boolean
        get() = this is Archived

    fun matches(item: ConversationItem): Boolean = when (this) {
        is Chat -> !item.isArchived && item.isPureChat
        is Workspace -> !item.isArchived && !item.isPureChat && item.workspaceName == name
        is Archived -> item.isArchived
    }
}

data class WorkspaceFilterOption(
    val filter: WorkspaceFilter,
    val count: Int,
    val latestModifiedMs: Long
)

class ConversationListViewModel(
    private val apiClient: ApiClient,
    val prefs: PreferencesManager,
    val cacheManager: CacheManager? = null,
    private val liveActivityManager: com.antigravity.mobile.data.service.LiveActivityNotificationManager? = null,
    private val connectionManager: ConnectionManager? = null
) : ViewModel() {

    private val _uiState = MutableStateFlow<ConversationListUiState>(ConversationListUiState.Loading)
    val uiState: StateFlow<ConversationListUiState> = _uiState.asStateFlow()

    private val _searchQuery = MutableStateFlow("")
    val searchQuery: StateFlow<String> = _searchQuery.asStateFlow()

    private val _selectedWorkspaceFilter = MutableStateFlow<WorkspaceFilter?>(null)
    val selectedWorkspaceFilter: StateFlow<WorkspaceFilter?> = _selectedWorkspaceFilter.asStateFlow()

    private val _workspaceFilterOptions = MutableStateFlow<List<WorkspaceFilterOption>>(emptyList())
    val workspaceFilterOptions: StateFlow<List<WorkspaceFilterOption>> = _workspaceFilterOptions.asStateFlow()

    private val _totalVisibleCount = MutableStateFlow(0)
    val totalVisibleCount: StateFlow<Int> = _totalVisibleCount.asStateFlow()

    private val _archivedCount = MutableStateFlow(0)
    val archivedCount: StateFlow<Int> = _archivedCount.asStateFlow()

    private val _workspaceTotalCount = MutableStateFlow(0)
    val workspaceTotalCount: StateFlow<Int> = _workspaceTotalCount.asStateFlow()

    private val _hiddenPendingApprovalCount = MutableStateFlow(0)
    val hiddenPendingApprovalCount: StateFlow<Int> = _hiddenPendingApprovalCount.asStateFlow()

    /** 会话「内容」命中（language_server SearchConversations），与按标题过滤并行存在。 */
    private val _contentResults = MutableStateFlow<List<ConversationSearchResult>>(emptyList())
    val contentResults: StateFlow<List<ConversationSearchResult>> = _contentResults.asStateFlow()

    private val _isSearchingContent = MutableStateFlow(false)
    val isSearchingContent: StateFlow<Boolean> = _isSearchingContent.asStateFlow()

    private var contentSearchJob: kotlinx.coroutines.Job? = null

    private val _quotaData = MutableStateFlow<CockpitQuotaResponse?>(null)
    val quotaData: StateFlow<CockpitQuotaResponse?> = _quotaData.asStateFlow()

    private val _isRefreshingQuota = MutableStateFlow(false)
    val isRefreshingQuota: StateFlow<Boolean> = _isRefreshingQuota.asStateFlow()

    private val _tokenHistory = MutableStateFlow<TokenHistoryResponse?>(null)
    val tokenHistory: StateFlow<TokenHistoryResponse?> = _tokenHistory.asStateFlow()

    private val _isLoadingTokenHistory = MutableStateFlow(false)
    val isLoadingTokenHistory: StateFlow<Boolean> = _isLoadingTokenHistory.asStateFlow()

    private var lastTokenHistoryFetchAtMs: Long = 0L
    private var lastTokenHistorySessionKey: String? = null
    private var lastModelCatalogFetchAtMs: Long = 0L
    private var lastModelCatalogBaseUrl: String? = null
    @Volatile private var isLoadingModelCatalog: Boolean = false

    private val _projects = MutableStateFlow<List<ProjectItem>>(loadInitialProjects())
    val projects: StateFlow<List<ProjectItem>> = _projects.asStateFlow()

    private val _isLoadingProjects = MutableStateFlow(false)
    val isLoadingProjects: StateFlow<Boolean> = _isLoadingProjects.asStateFlow()

    private val _projectsError = MutableStateFlow<String?>(null)
    val projectsError: StateFlow<String?> = _projectsError.asStateFlow()

    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    private val _globalBackgroundTasks = MutableStateFlow<List<GlobalBackgroundTaskItem>>(emptyList())
    val globalBackgroundTasks: StateFlow<List<GlobalBackgroundTaskItem>> = _globalBackgroundTasks.asStateFlow()

    fun updateGlobalBackgroundTasks(tasks: List<GlobalBackgroundTaskItem>) {
        _globalBackgroundTasks.value = tasks
    }

    private val deletedCascadeIds = mutableSetOf<String>()
    private var rawConversations = emptyList<ConversationItem>()
    private var pollJob: kotlinx.coroutines.Job? = null
    private var eventsJob: kotlinx.coroutines.Job? = null
    private var lastReorderAtMs: Long = 0L
    private var pendingReorderJob: kotlinx.coroutines.Job? = null

    /** Gateway push hints for list changes; polling below is the fallback and a slow safety net. */
    private val eventsClient = ConversationEventsClient(
        prefs = prefs,
        baseUrlProvider = { apiClient.currentBaseUrl },
        onRelayUnavailable = {
            if (prefs.isPaired()) {
                viewModelScope.launch {
                    connectionManager?.probeEndpoints(prefs)
                }
            }
        }
    )
    /** Heartbeat facts backing the connection dot in the home quota bar. */
    val linkSnapshot: kotlinx.coroutines.flow.StateFlow<com.antigravity.mobile.data.service.LinkSnapshot> = eventsClient.snapshot
    fun reconnectEvents() = eventsClient.reconnect()
    fun reconnectEventsIfStale() = eventsClient.reconnectIfStale()

    @Volatile private var lastNotifiedRouteUrl: String? = null
    private val routeChangeListener: (String) -> Unit = { newUrl ->
        if (prefs.isPaired() && !ConnectionManager.isSameEndpoint(lastNotifiedRouteUrl, newUrl)) {
            lastNotifiedRouteUrl = newUrl
            Log.d("ConvListVM", "Route changed to $newUrl; reconnecting events and refreshing list/quota")
            eventsClient.reconnect()
            loadConversations()
            loadQuotas()
            refreshModelCatalogIfNeeded(force = false)
        }
    }

    init {
        viewModelScope.launch {
            _quotaData.collect {
                com.antigravity.mobile.data.service.ModelPickerQuotaCache.account.value = it?.currentAccount
            }
        }
        rawConversations = loadInitialConversations()
        connectionManager?.addOnRouteChangedListener(routeChangeListener)
        if (prefs.isPaired()) {
            loadConversations()
            loadQuotas()
            loadProjects()
            refreshModelCatalogIfNeeded(force = false)
            prewarmMcpAndSkillsCache()
        } else {
            _uiState.value = ConversationListUiState.Success(emptyList())
        }
    }

    private suspend fun probeAndDetectRouteChange(failedBaseUrl: String?): Boolean {
        val cm = connectionManager ?: return false
        val elected = cm.probeEndpoints(prefs) ?: return false
        return !ConnectionManager.isSameEndpoint(elected, failedBaseUrl)
    }

    private fun sortConversations(items: List<ConversationItem>): List<ConversationItem> {
        val flat = items.sortedWith(
            compareByDescending<ConversationItem> { it.status == ConversationStatus.ACTION }
                .thenByDescending { it.isPinned }
                .thenByDescending { it.status == ConversationStatus.RUNNING }
                .thenByDescending { it.lastModifiedEpochMs }
        )
        return arrangeBranchTree(flat)
    }

    /** 将旁路快问分支稳定挂载到父会话正下方，若父会话不在当前筛选集合内仍作为独立行保留。 */
    private fun arrangeBranchTree(items: List<ConversationItem>): List<ConversationItem> {
        if (items.size <= 1) return items
        val allIds = items.mapTo(HashSet(items.size)) { it.id }
        val childrenByParent = LinkedHashMap<String, MutableList<ConversationItem>>()
        val roots = ArrayList<ConversationItem>(items.size)
        for (item in items) {
            val parentId = item.branchParentId?.trim().orEmpty()
            if (item.isSideQuestion && parentId.isNotEmpty() && parentId != item.id && allIds.contains(parentId)) {
                childrenByParent.getOrPut(parentId) { mutableListOf() }.add(item)
            } else {
                roots.add(item)
            }
        }
        if (childrenByParent.isEmpty()) return items
        val result = ArrayList<ConversationItem>(items.size)
        val emitted = HashSet<String>(items.size)
        fun appendWithChildren(node: ConversationItem) {
            if (!emitted.add(node.id)) return
            result.add(node)
            childrenByParent[node.id]?.forEach { appendWithChildren(it) }
        }
        roots.forEach { appendWithChildren(it) }
        if (result.size < items.size) {
            items.forEach { if (emitted.add(it.id)) result.add(it) }
        }
        return result
    }

    private fun markReorderedNow() {
        lastReorderAtMs = android.os.SystemClock.elapsedRealtime()
        pendingReorderJob?.cancel()
        pendingReorderJob = null
    }

    private fun applyReorderBuffer(
        sorted: List<ConversationItem>,
        forceReorder: Boolean
    ): List<ConversationItem> {
        val now = android.os.SystemClock.elapsedRealtime()
        val prevActionIds = rawConversations.filter { it.status == ConversationStatus.ACTION }.mapTo(HashSet()) { it.id }
        val hasNewPendingAction = sorted.any { it.status == ConversationStatus.ACTION && !prevActionIds.contains(it.id) }
        val prevRunningIds = rawConversations.filter { it.status == ConversationStatus.RUNNING }.mapTo(HashSet()) { it.id }
        val hasNewRunning = sorted.any { it.status == ConversationStatus.RUNNING && !prevRunningIds.contains(it.id) }
        if (forceReorder || hasNewPendingAction || hasNewRunning || rawConversations.isEmpty() || lastReorderAtMs == 0L || (now - lastReorderAtMs) >= REORDER_COOLDOWN_MS) {
            markReorderedNow()
            return sorted
        }

        val newById = sorted.associateBy { it.id }
        val prevIds = rawConversations.map { it.id }
        val prevIdSet = prevIds.toHashSet()
        val preserved = prevIds.mapNotNull { newById[it] }
        val added = sorted.filter { !prevIdSet.contains(it.id) }

        val buffered = (added.filter { it.status == ConversationStatus.ACTION } + preserved.filter { it.status == ConversationStatus.ACTION }) +
            (added.filter { it.status != ConversationStatus.ACTION && it.isPinned } + preserved.filter { it.status != ConversationStatus.ACTION && it.isPinned }) +
            (added.filter { it.status != ConversationStatus.ACTION && !it.isPinned && it.status == ConversationStatus.RUNNING } + preserved.filter { it.status != ConversationStatus.ACTION && !it.isPinned && it.status == ConversationStatus.RUNNING }) +
            (added.filter { it.status != ConversationStatus.ACTION && !it.isPinned && it.status != ConversationStatus.RUNNING } + preserved.filter { it.status != ConversationStatus.ACTION && !it.isPinned && it.status != ConversationStatus.RUNNING })

        val orderChanged = buffered.size == sorted.size &&
            buffered.indices.any { buffered[it].id != sorted[it].id }
        if (orderChanged) {
            if (pendingReorderJob?.isActive != true) {
                val remainingMs = (REORDER_COOLDOWN_MS - (now - lastReorderAtMs)).coerceAtLeast(50L)
                pendingReorderJob = viewModelScope.launch {
                    kotlinx.coroutines.delay(remainingMs)
                    flushPendingReorder()
                }
            }
        } else {
            pendingReorderJob?.cancel()
            pendingReorderJob = null
        }
        return buffered
    }

    private fun flushPendingReorder() {
        pendingReorderJob = null
        if (rawConversations.isEmpty()) return
        val sorted = sortConversations(rawConversations)
        lastReorderAtMs = android.os.SystemClock.elapsedRealtime()
        val orderChanged = sorted.size != rawConversations.size ||
            sorted.indices.any { sorted[it].id != rawConversations[it].id }
        if (orderChanged) {
            rawConversations = sorted
            persistConversationsToCache(rawConversations)
            applyFilter()
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
            rawConversations = sortConversations(all)
            markReorderedNow()
            applyFilter()
            cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
            liveActivityManager?.syncWithConversations(rawConversations)
        }
    }

    fun startAutoRefresh() {
        if (!prefs.isPaired()) return
        if (pollJob?.isActive == true) return
        eventsClient.start()
        eventsJob = viewModelScope.launch {
            eventsClient.changes.collect { refreshConversationsFromServer() }
        }
        pollJob = viewModelScope.launch {
            while (true) {
                if (eventsClient.connected.value) {
                    // Pushed hints drive refreshes; poll slowly as a safety net, and drop back to
                    // fast polling the moment the event socket is lost.
                    kotlinx.coroutines.withTimeoutOrNull(EVENTS_SAFETY_POLL_MS) {
                        eventsClient.connected.first { !it }
                    }
                } else {
                    val hasRunning = rawConversations.any { it.status.isRunning || it.status.needsAction }
                    kotlinx.coroutines.delay(if (hasRunning) 4000L else 10000L)
                }
                if (!prefs.isPaired()) break
                refreshConversationsFromServer()
            }
        }
    }

    private suspend fun refreshConversationsFromServer() {
        val attemptedBase = apiClient.currentBaseUrl
        val firstResult = apiClient.fetchConversations()
        val effectiveResult = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
            apiClient.fetchConversations()
        } else {
            firstResult
        }
        effectiveResult.onSuccess { list ->
            rawConversations = mergeWithLocalConversations(list, forceReorder = false)
            persistConversationsToCache(rawConversations)
            applyFilter()
            cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
            liveActivityManager?.syncWithConversations(rawConversations)
        }
        apiClient.fetchGlobalBackgroundTasks().onSuccess {
            _globalBackgroundTasks.value = it
        }
        // Self-heal home quota bar if initial startup fetch failed before route migration completed.
        if (_quotaData.value == null) {
            apiClient.fetchCockpitQuotas().onSuccess {
                _quotaData.value = it
            }
        }
    }

    fun stopAutoRefresh() {
        pollJob?.cancel()
        pollJob = null
        eventsJob?.cancel()
        eventsJob = null
        pendingReorderJob?.cancel()
        pendingReorderJob = null
        eventsClient.stop()
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
        val sorted = sortConversations(all)
        if (sorted.isNotEmpty()) {
            markReorderedNow()
            updateWorkspaceFilterStats(sorted)
            _uiState.value = ConversationListUiState.Success(sorted)
            cacheManager?.prewarmSessions(sorted.take(15).map { it.id })
            liveActivityManager?.syncWithConversations(sorted)
        }
        return sorted
    }

    private fun persistConversationsToCache(list: List<ConversationItem>) {
        viewModelScope.launch(kotlinx.coroutines.Dispatchers.IO) {
            try {
                val nonDrafts = list.filter { !it.isDraft && !prefs.isDeletedConversation(it.id) }
                prefs.cachedConversationsJson = apiClient.json.encodeToString(nonDrafts)
            } catch (e: Exception) {
                Log.w("ConvListVM", "Failed to cache conversations: ${e.message}")
            }
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

    private fun mergeWithLocalConversations(
        serverList: List<ConversationItem>,
        forceReorder: Boolean = true
    ): List<ConversationItem> {
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
            val resolvedTitle = if ((serverItem.title.isBlank() || serverItem.title == "未命名会话".loc) &&
                healedLocal.title.isNotBlank() && healedLocal.title != "未命名会话".loc && healedLocal.title != "会话详情".loc) {
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
        val sorted = sortConversations(drafts + enrichedServerItems)
        return applyReorderBuffer(sorted, forceReorder)
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
            loadModelCatalog(force = false)
            prewarmMcpAndSkillsCache()
        } catch (e: Exception) {
            Log.w("ConvListVM", "loadInitialData failed: ${e.message}")
        }
        return success
    }

    private fun prewarmMcpAndSkillsCache() {
        viewModelScope.launch(kotlinx.coroutines.Dispatchers.IO) {
            try {
                apiClient.fetchMcpServerStates().onSuccess { prefs.saveCachedMcpServers(it) }
                apiClient.fetchAllSkills().onSuccess { prefs.saveCachedSkills(it) }
            } catch (_: Exception) {}
        }
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
                val cm = connectionManager
                if (cm != null) {
                    val isLinkUnhealthy = !eventsClient.snapshot.value.connected
                    val switchedToLan = cm.probeLanFastIfNeeded(prefs, force = isLinkUnhealthy)
                    if (switchedToLan) {
                        lastNotifiedRouteUrl = apiClient.currentBaseUrl
                        eventsClient.reconnect()
                    } else if (isLinkUnhealthy) {
                        val before = apiClient.currentBaseUrl
                        val elected = cm.probeEndpoints(prefs)
                        if (elected != null && !ConnectionManager.isSameEndpoint(before, elected)) {
                            lastNotifiedRouteUrl = elected
                            eventsClient.reconnect()
                        } else {
                            eventsClient.reconnectIfStale()
                        }
                    } else {
                        eventsClient.reconnectIfStale()
                    }
                } else {
                    eventsClient.reconnectIfStale()
                }

                val attemptedBase = apiClient.currentBaseUrl
                val firstConvResult = apiClient.fetchConversations()
                val convResult = if (firstConvResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                    lastNotifiedRouteUrl = apiClient.currentBaseUrl
                    eventsClient.reconnect()
                    apiClient.fetchConversations()
                } else {
                    firstConvResult
                }
                convResult.onSuccess { list ->
                    rawConversations = mergeWithLocalConversations(list)
                    persistConversationsToCache(rawConversations)
                    applyFilter()
                    cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                }.onFailure { err ->
                    if (rawConversations.isEmpty()) {
                        _uiState.value = ConversationListUiState.Error(err.message ?: "无法获取会话列表".loc)
                    }
                }

                // Refresh secondary data concurrently in background so pull-to-refresh retracts promptly once the list updates.
                if (convResult.isSuccess) {
                    viewModelScope.launch {
                        val quotaAttemptBase = apiClient.currentBaseUrl
                        val firstQuota = apiClient.fetchCockpitQuotas()
                        val quotaResult = if (firstQuota.isFailure && probeAndDetectRouteChange(quotaAttemptBase)) {
                            apiClient.fetchCockpitQuotas()
                        } else {
                            firstQuota
                        }
                        quotaResult.onSuccess {
                            _quotaData.value = it
                        }
                    }
                    viewModelScope.launch {
                        apiClient.fetchGlobalBackgroundTasks().onSuccess {
                            _globalBackgroundTasks.value = it
                        }
                    }
                    viewModelScope.launch {
                        apiClient.fetchProjects().onSuccess { list ->
                            _projects.value = list
                            _projectsError.value = null
                            if (list.isNotEmpty()) {
                                try {
                                    prefs.cachedProjectsJson = apiClient.json.encodeToString(list)
                                } catch (_: Exception) {}
                            }
                        }
                    }
                    loadModelCatalog(force = true)
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
        // 升级自检结果随列表一起刷新（一次很小的 GET），失败时保持上一次结果
        viewModelScope.launch { apiClient.refreshGatewayCompat() }
        refreshModelCatalogIfNeeded(force = false)
        viewModelScope.launch {
            apiClient.fetchGlobalBackgroundTasks().onSuccess {
                _globalBackgroundTasks.value = it
            }
        }
        viewModelScope.launch {
            val attemptedBase = apiClient.currentBaseUrl
            val firstResult = apiClient.fetchConversations()
            val result = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                apiClient.fetchConversations()
            } else {
                firstResult
            }
            result.onSuccess { list ->
                rawConversations = mergeWithLocalConversations(list)
                persistConversationsToCache(rawConversations)
                applyFilter()
                cacheManager?.prewarmSessions(rawConversations.take(15).map { it.id })
                liveActivityManager?.syncWithConversations(rawConversations)
                if (_quotaData.value == null) {
                    loadQuotas()
                }
            }.onFailure { err ->
                if (rawConversations.isEmpty()) {
                    _uiState.value = ConversationListUiState.Error(err.message ?: "无法获取会话列表".loc)
                }
            }
        }
    }

    fun loadQuotas() {
        viewModelScope.launch {
            val attemptedBase = apiClient.currentBaseUrl
            val firstResult = apiClient.fetchCockpitQuotas()
            val result = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                apiClient.fetchCockpitQuotas()
            } else {
                firstResult
            }
            result.onSuccess {
                _quotaData.value = it
            }
        }
    }

    fun refreshQuotas() {
        _isRefreshingQuota.value = true
        viewModelScope.launch {
            val attemptedBase = apiClient.currentBaseUrl
            val firstResult = apiClient.refreshCockpitQuotas()
            val result = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                apiClient.refreshCockpitQuotas()
            } else {
                firstResult
            }
            result.onSuccess {
                _quotaData.value = it
            }
            _isRefreshingQuota.value = false
        }
    }

    fun loadTokenHistory(force: Boolean = false) {
        if (!prefs.isPaired()) return
        if (_isLoadingTokenHistory.value) return
        val sessionKey = prefs.deviceToken.orEmpty()
        val nowMs = System.currentTimeMillis()
        if (!force && _tokenHistory.value != null && lastTokenHistorySessionKey == sessionKey) {
            val elapsedMs = nowMs - lastTokenHistoryFetchAtMs
            if (elapsedMs in 0L until TOKEN_HISTORY_REFRESH_INTERVAL_MS) return
        }
        _isLoadingTokenHistory.value = true
        viewModelScope.launch {
            try {
                val attemptedBase = apiClient.currentBaseUrl
                val firstResult = apiClient.fetchTokenHistory(force = force)
                val result = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                    apiClient.fetchTokenHistory(force = force)
                } else {
                    firstResult
                }
                result.onSuccess {
                    _tokenHistory.value = it
                    lastTokenHistoryFetchAtMs = System.currentTimeMillis()
                    lastTokenHistorySessionKey = sessionKey
                }
            } finally {
                _isLoadingTokenHistory.value = false
            }
        }
    }

    fun switchCockpitAccount(accountId: String) {
        viewModelScope.launch {
            apiClient.switchCockpitAccount(accountId).onSuccess {
                refreshQuotas()
                loadConversations()
                loadModelCatalog(force = true)
            }
        }
    }

    fun loadProjects() {
        if (!prefs.isPaired()) {
            if (_projects.value.isEmpty()) {
                _projectsError.value = "请先配对电脑终端".loc
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
                    _projectsError.value = "拉取工作区失败: %s".loc(err.message ?: "网络或网关异常".loc)
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
                title = if (item.title.isNotBlank() && item.title != "未命名会话".loc && item.title != "会话详情".loc) item.title else old.title,
                status = if (item.status != ConversationStatus.UNKNOWN) item.status else old.status,
                stepCount = maxOf(old.stepCount, item.stepCount),
                workspaceName = if (item.workspaceName.isNotBlank() && item.workspaceName != "Chat") item.workspaceName else old.workspaceName,
                lastModifiedTime = old.lastModifiedTime,
                isUnread = item.isUnread,
                hasSwarm = item.hasSwarm || old.hasSwarm,
                swarmWorkingCount = if (item.hasSwarm) item.swarmWorkingCount else old.swarmWorkingCount,
                swarmTotalMembers = if (item.hasSwarm) item.swarmTotalMembers else old.swarmTotalMembers
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

    fun togglePinConversation(cascadeId: String) {
        val willPin = prefs.togglePin(cascadeId)
        rawConversations = rawConversations.map { item ->
            if (item.id == cascadeId) item.copy(isPinned = willPin) else item
        }
        rawConversations = sortConversations(rawConversations)
        markReorderedNow()
        persistConversationsToCache(rawConversations)
        applyFilter()

        viewModelScope.launch {
            apiClient.setConversationPinned(cascadeId, willPin)
        }
    }

    fun toggleArchiveConversation(cascadeId: String) {
        val current = rawConversations.find { it.id == cascadeId } ?: return
        if (current.isDraft) return
        val willArchive = !current.isArchived
        rawConversations = rawConversations.map { item ->
            if (item.id == cascadeId) item.copy(isArchived = willArchive, isUnread = if (willArchive) false else item.isUnread) else item
        }
        if (!willArchive && _selectedWorkspaceFilter.value == WorkspaceFilter.Archived && rawConversations.none { !it.isSubagent && it.isArchived }) {
            _selectedWorkspaceFilter.value = null
        }
        rawConversations = sortConversations(rawConversations)
        markReorderedNow()
        persistConversationsToCache(rawConversations)
        applyFilter()

        viewModelScope.launch {
            apiClient.setConversationArchived(cascadeId, willArchive).onFailure {
                rawConversations = rawConversations.map { item ->
                    if (item.id == cascadeId) item.copy(isArchived = !willArchive) else item
                }
                rawConversations = sortConversations(rawConversations)
                persistConversationsToCache(rawConversations)
                applyFilter()
            }
        }
    }

    fun renameConversation(cascadeId: String, newTitle: String) {
        val trimmed = newTitle.trim()
        if (trimmed.isBlank()) return

        val originalItem = rawConversations.find { it.id == cascadeId }
        val originalTitle = originalItem?.title ?: ""
        if (trimmed == originalTitle) return

        // 1. Optimistic UI and local cache update
        cacheManager?.updateSessionTitle(cascadeId, trimmed)
        rawConversations = rawConversations.map { item ->
            if (item.id == cascadeId) item.copy(title = trimmed) else item
        }
        persistConversationsToCache(rawConversations)
        applyFilter()
        liveActivityManager?.syncWithConversations(rawConversations)

        // 2. Upstream remote synchronization
        viewModelScope.launch {
            apiClient.renameConversation(cascadeId, trimmed).onSuccess {
                loadConversations()
            }.onFailure { err ->
                Log.e("ConvListVM", "Failed to rename conversation $cascadeId: ${err.message}")
                if (originalItem != null) {
                    cacheManager?.updateSessionTitle(cascadeId, originalTitle)
                    rawConversations = rawConversations.map { item ->
                        if (item.id == cascadeId) item.copy(title = originalTitle) else item
                    }
                    persistConversationsToCache(rawConversations)
                    applyFilter()
                    liveActivityManager?.syncWithConversations(rawConversations)
                }
            }
        }
    }

    fun updateProjectAlias(project: ProjectItem, newAlias: String) {
        val trimmed = newAlias.trim()
        val targetPath = project.path.ifBlank { project.uri }
        val updatedAlias = trimmed.ifEmpty { null }

        // 1. Optimistic update in-memory projects state flow
        val currentList = _projects.value
        val updatedList = currentList.map { item ->
            val itemPath = item.path.ifBlank { item.uri }
            if (itemPath == targetPath || (item.id.isNotBlank() && item.id == project.id)) {
                item.copy(alias = updatedAlias)
            } else {
                item
            }
        }
        _projects.value = updatedList

        // 2. Persist to local cache for instant cold starts
        try {
            prefs.cachedProjectsJson = apiClient.json.encodeToString(updatedList)
        } catch (e: Exception) {
            Log.w("ConvListVM", "Failed to cache updated projects: ${e.message}")
        }

        // 3. Asynchronously sync to gateway host
        viewModelScope.launch {
            apiClient.updateProjectAlias(targetPath, trimmed).onFailure { err ->
                Log.e("ConvListVM", "Failed to sync project alias to gateway: ${err.message}")
            }
        }
    }

    /** 按 3 分钟节流或主机/路由变更自动静默刷新模型目录，对齐 iOS ModelCatalogStore.refreshIfNeeded。 */
    fun refreshModelCatalogIfNeeded(force: Boolean = false) {
        if (!prefs.isPaired() || isLoadingModelCatalog) return
        val currentBase = apiClient.currentBaseUrl
        val nowMs = System.currentTimeMillis()
        val sameBase = ConnectionManager.isSameEndpoint(lastModelCatalogBaseUrl, currentBase)
        if (!force && sameBase && prefs.cachedModelCatalog?.live == true) {
            val elapsedMs = nowMs - lastModelCatalogFetchAtMs
            if (elapsedMs in 0L until MODEL_CATALOG_REFRESH_INTERVAL_MS) return
        }
        viewModelScope.launch {
            loadModelCatalog(force = force)
        }
    }

    /** 设置页「默认模型」、下拉刷新与切号刷新用：向网关取可选模型目录并缓存/校验默认值。 */
    suspend fun loadModelCatalog(force: Boolean = false): Result<com.antigravity.mobile.data.model.ModelsResponse> {
        isLoadingModelCatalog = true
        return try {
            val attemptedBase = apiClient.currentBaseUrl
            val firstResult = apiClient.getModels(refresh = force)
            val result = if (firstResult.isFailure && probeAndDetectRouteChange(attemptedBase)) {
                apiClient.getModels(refresh = force)
            } else {
                firstResult
            }
            result.onSuccess { catalog ->
                prefs.updateModelCatalog(catalog)
                lastModelCatalogFetchAtMs = System.currentTimeMillis()
                lastModelCatalogBaseUrl = apiClient.currentBaseUrl
            }
        } finally {
            isLoadingModelCatalog = false
        }
    }

    fun onSearchQueryChanged(query: String) {
        _searchQuery.value = query
        applyFilter()
        searchContent(query.trim())
    }

    fun toggleWorkspaceFilter(item: ConversationItem) {
        val target = if (item.isPureChat) WorkspaceFilter.Chat else WorkspaceFilter.Workspace(item.workspaceName)
        _selectedWorkspaceFilter.value = if (_selectedWorkspaceFilter.value == target) null else target
        applyFilter()
        if (_searchQuery.value.isNotBlank()) {
            searchContent(_searchQuery.value.trim())
        }
    }

    fun selectWorkspaceFilter(filter: WorkspaceFilter) {
        if (_selectedWorkspaceFilter.value == filter) return
        _selectedWorkspaceFilter.value = filter
        applyFilter()
        if (_searchQuery.value.isNotBlank()) {
            searchContent(_searchQuery.value.trim())
        }
    }

    fun clearWorkspaceFilter() {
        if (_selectedWorkspaceFilter.value == null) return
        _selectedWorkspaceFilter.value = null
        applyFilter()
        if (_searchQuery.value.isNotBlank()) {
            searchContent(_searchQuery.value.trim())
        }
    }

    fun resolveProjectForFilter(filter: WorkspaceFilter): ProjectItem? {
        return when (filter) {
            is WorkspaceFilter.Chat -> ProjectItem.PURE_CHAT
            is WorkspaceFilter.Archived -> null
            is WorkspaceFilter.Workspace -> {
                _projects.value.firstOrNull { !it.isPureChat && (it.name == filter.name || it.displayName == filter.name) }
                    ?: rawConversations
                        .asSequence()
                        .filter { filter.matches(it) }
                        .mapNotNull { it.draftProject }
                        .firstOrNull { !it.isPureChat }
            }
        }
    }

    private fun updateWorkspaceFilterStats(source: List<ConversationItem>) {
        val visible = source.filter { !it.isSubagent && !it.isArchived }
        val archived = source.filter { !it.isSubagent && it.isArchived }
        _totalVisibleCount.value = visible.size
        _archivedCount.value = archived.size

        var chatCount = 0
        var chatLatest = 0L
        val workspaces = LinkedHashMap<String, Pair<Int, Long>>()

        for (item in visible) {
            val modified = item.lastModifiedEpochMs
            if (item.isPureChat) {
                chatCount += 1
                if (modified > chatLatest) chatLatest = modified
            } else {
                val existing = workspaces[item.workspaceName]
                if (existing != null) {
                    workspaces[item.workspaceName] = Pair(existing.first + 1, maxOf(existing.second, modified))
                } else {
                    workspaces[item.workspaceName] = Pair(1, modified)
                }
            }
        }

        val options = mutableListOf<WorkspaceFilterOption>()
        if (chatCount > 0) {
            options.add(WorkspaceFilterOption(WorkspaceFilter.Chat, chatCount, chatLatest))
        }
        val sortedProjects = workspaces
            .map { (name, stats) -> WorkspaceFilterOption(WorkspaceFilter.Workspace(name), stats.first, stats.second) }
            .sortedWith(
                compareByDescending<WorkspaceFilterOption> { it.latestModifiedMs }
                    .thenBy { it.filter.displayName.lowercase(Locale.getDefault()) }
            )
        options.addAll(sortedProjects)
        if (archived.isNotEmpty()) {
            val latestArchived = archived.maxOfOrNull { it.lastModifiedEpochMs } ?: 0L
            options.add(WorkspaceFilterOption(WorkspaceFilter.Archived, archived.size, latestArchived))
        }
        _workspaceFilterOptions.value = options

        val activeFilter = _selectedWorkspaceFilter.value
        _workspaceTotalCount.value = when (activeFilter) {
            null -> visible.size
            is WorkspaceFilter.Archived -> archived.size
            else -> visible.count { activeFilter.matches(it) }
        }
        _hiddenPendingApprovalCount.value = if (activeFilter == null) {
            0
        } else {
            visible.count { it.status == ConversationStatus.ACTION && !activeFilter.matches(it) }
        }
    }

    /** 输入停顿 350ms 后再查；少于 2 个字符不查（单字命中太泛）。 */
    private fun searchContent(query: String) {
        contentSearchJob?.cancel()
        // 当前 Antigravity 没有 SearchConversations 时只保留按标题过滤，不再发必然失败的请求
        if (!GatewayCompatStore.isAvailable(GatewayFeature.SEARCH) || query.codePointCount(0, query.length) < 2) {
            _contentResults.value = emptyList()
            _isSearchingContent.value = false
            return
        }
        _isSearchingContent.value = true
        contentSearchJob = viewModelScope.launch {
            kotlinx.coroutines.delay(350)
            apiClient.searchConversations(query)
                .onSuccess { results ->
                    // 子代理会话不在列表里展示，也不作为搜索结果；同时遵守当前项目筛选
                    val activeFilter = _selectedWorkspaceFilter.value
                    val scopedIds = rawConversations
                        .filter { !it.isSubagent && (activeFilter?.matches(it) ?: true) }
                        .map { it.id }
                        .toSet()
                    _contentResults.value = if (activeFilter != null) {
                        results.filter { it.cascadeId in scopedIds }
                    } else {
                        results.filter { scopedIds.isEmpty() || it.cascadeId in scopedIds }
                    }
                }
                .onFailure {
                    Log.w("ConvListVM", "Content search failed: ${it.message}")
                    _contentResults.value = emptyList()
                }
            _isSearchingContent.value = false
        }
    }

    private fun applyFilter() {
        updateWorkspaceFilterStats(rawConversations)
        val activeFilter = _selectedWorkspaceFilter.value
        val scoped = if (activeFilter == null) {
            rawConversations.filter { !it.isArchived }
        } else {
            rawConversations.filter { activeFilter.matches(it) }
        }
        val query = _searchQuery.value.trim().lowercase()
        val filtered = if (query.isEmpty()) {
            arrangeBranchTree(scoped)
        } else {
            arrangeBranchTree(
                scoped.filter {
                    it.title.lowercase().contains(query) ||
                        it.workspaceName.lowercase().contains(query) ||
                        (it.branchParentTitle?.lowercase()?.contains(query) == true)
                }
            )
        }
        _uiState.value = ConversationListUiState.Success(filtered)
    }

    /** Unfiltered (ignores the search box) list of top-level conversations, for share-target pickers. */
    fun allConversations(): List<ConversationItem> = rawConversations.filter { !it.isSubagent }

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
        _selectedWorkspaceFilter.value = null
        _workspaceFilterOptions.value = emptyList()
        _totalVisibleCount.value = 0
        _workspaceTotalCount.value = 0
        lastReorderAtMs = 0L
        _projects.value = emptyList()
        _projectsError.value = null
        _quotaData.value = null
        _tokenHistory.value = null
        lastTokenHistoryFetchAtMs = 0L
        lastTokenHistorySessionKey = null
        lastModelCatalogFetchAtMs = 0L
        lastModelCatalogBaseUrl = null
        onComplete()
    }

    override fun onCleared() {
        connectionManager?.removeOnRouteChangedListener(routeChangeListener)
        stopAutoRefresh()
        eventsClient.close()
        super.onCleared()
    }
}
