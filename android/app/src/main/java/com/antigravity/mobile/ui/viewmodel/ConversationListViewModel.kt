package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

sealed interface ConversationListUiState {
    data object Loading : ConversationListUiState
    data class Success(val conversations: List<ConversationItem>) : ConversationListUiState
    data class Error(val message: String) : ConversationListUiState
}

class ConversationListViewModel(
    private val apiClient: ApiClient,
    val prefs: PreferencesManager
) : ViewModel() {

    private val _uiState = MutableStateFlow<ConversationListUiState>(ConversationListUiState.Loading)
    val uiState: StateFlow<ConversationListUiState> = _uiState.asStateFlow()

    private val _searchQuery = MutableStateFlow("")
    val searchQuery: StateFlow<String> = _searchQuery.asStateFlow()

    private val _quotaData = MutableStateFlow<CockpitQuotaResponse?>(null)
    val quotaData: StateFlow<CockpitQuotaResponse?> = _quotaData.asStateFlow()

    private val _isRefreshingQuota = MutableStateFlow(false)
    val isRefreshingQuota: StateFlow<Boolean> = _isRefreshingQuota.asStateFlow()

    private val _projects = MutableStateFlow<List<ProjectItem>>(emptyList())
    val projects: StateFlow<List<ProjectItem>> = _projects.asStateFlow()

    private val _isLoadingProjects = MutableStateFlow(false)
    val isLoadingProjects: StateFlow<Boolean> = _isLoadingProjects.asStateFlow()

    private var rawConversations = listOf<ConversationItem>()

    init {
        loadConversations()
        loadQuotas()
        loadProjects()
    }

    fun loadConversations() {
        viewModelScope.launch {
            val result = apiClient.fetchConversations()
            result.onSuccess { list ->
                rawConversations = list
                applyFilter()
            }.onFailure { err ->
                _uiState.value = ConversationListUiState.Error(err.message ?: "无法获取会话列表")
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
        _isLoadingProjects.value = true
        viewModelScope.launch {
            apiClient.fetchProjects().onSuccess {
                _projects.value = it
            }
            _isLoadingProjects.value = false
        }
    }

    fun createConversation(project: ProjectItem, prompt: String, model: String = "gemini-3.8-flash-high", onCreated: (String) -> Unit) {
        viewModelScope.launch {
            val res = apiClient.createCascade(
                workspaceUri = project.uri,
                prompt = prompt,
                model = model,
                projectId = project.rawId
            )
            res.onSuccess { cascadeId ->
                loadConversations()
                onCreated(cascadeId)
            }
        }
    }

    fun deleteConversation(cascadeId: String) {
        viewModelScope.launch {
            apiClient.deleteConversation(cascadeId).onSuccess {
                loadConversations()
            }
        }
    }

    fun renameConversation(cascadeId: String, newTitle: String) {
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

    fun unpair() {
        prefs.clear()
    }
}
