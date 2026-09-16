package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.ConversationItem
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
    private val prefs: PreferencesManager
) : ViewModel() {

    private val _uiState = MutableStateFlow<ConversationListUiState>(ConversationListUiState.Loading)
    val uiState: StateFlow<ConversationListUiState> = _uiState.asStateFlow()

    private val _searchQuery = MutableStateFlow("")
    val searchQuery: StateFlow<String> = _searchQuery.asStateFlow()

    private var rawConversations = listOf<ConversationItem>()

    init {
        loadConversations()
    }

    fun loadConversations() {
        _uiState.value = ConversationListUiState.Loading
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
                (it.title?.lowercase()?.contains(query) == true) ||
                        (it.workspaceFolder?.lowercase()?.contains(query) == true)
            }
        }
        _uiState.value = ConversationListUiState.Success(filtered)
    }

    fun unpair() {
        prefs.clear()
    }
}
