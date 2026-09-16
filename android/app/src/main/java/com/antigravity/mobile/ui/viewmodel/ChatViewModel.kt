package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.data.model.PendingInteraction
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.data.service.StreamWebSocketClient
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class ChatUiState(
    val cascadeId: String = "",
    val title: String = "会话详情",
    val messages: List<GatewayMessageItem> = emptyList(),
    val isRunning: Boolean = false,
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val connectionStatus: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val errorMessage: String? = null
)

class ChatViewModel(
    private val apiClient: ApiClient,
    private val wsClient: StreamWebSocketClient
) : ViewModel() {

    private val _uiState = MutableStateFlow(ChatUiState())
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    private val _inputText = MutableStateFlow("")
    val inputText: StateFlow<String> = _inputText.asStateFlow()

    fun initSession(cascadeId: String, initialTitle: String? = null) {
        _uiState.value = _uiState.value.copy(
            cascadeId = cascadeId,
            title = initialTitle?.takeIf { it.isNotBlank() } ?: "会话 $cascadeId"
        )

        wsClient.connect(cascadeId)
        observeWebSocket()
    }

    private fun observeWebSocket() {
        viewModelScope.launch {
            wsClient.connectionStatus.collect { status ->
                _uiState.value = _uiState.value.copy(connectionStatus = status)
            }
        }

        viewModelScope.launch {
            wsClient.streamUpdates.collect { payload ->
                if (payload == null) return@collect
                if (payload.cascadeId != _uiState.value.cascadeId) return@collect

                val isRunning = payload.status.equals("RUNNING", ignoreCase = true)
                _uiState.value = _uiState.value.copy(
                    title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                    messages = payload.messages ?: _uiState.value.messages,
                    isRunning = isRunning,
                    canProceed = payload.canProceed,
                    proceedArtifactUri = payload.proceedArtifactUri,
                    pendingInteraction = payload.pendingInteraction,
                    errorMessage = if (payload.hasError) payload.errorMessage else null
                )
            }
        }
    }

    fun onInputTextChanged(text: String) {
        _inputText.value = text
    }

    fun sendCurrentMessage() {
        val text = _inputText.value.trim()
        if (text.isEmpty() || _uiState.value.isRunning) return

        val cascadeId = _uiState.value.cascadeId
        _inputText.value = ""

        // Optimistically add user message bubble
        val optimisticUserMsg = GatewayMessageItem(
            id = "opt_${System.currentTimeMillis()}",
            role = "user",
            content = text
        )
        _uiState.value = _uiState.value.copy(
            messages = _uiState.value.messages + optimisticUserMsg,
            isRunning = true
        )

        viewModelScope.launch {
            val result = apiClient.sendMessage(cascadeId, text)
            result.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    errorMessage = "发送失败: ${err.message}",
                    isRunning = false
                )
            }
        }
    }

    fun cancelExecution() {
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.cancelInvocation(cascadeId)
        }
    }

    fun approveInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.submitInteraction(
                cascadeId = cascadeId,
                stepIndex = interaction.stepIndex,
                responseType = interaction.type,
                confirmed = true
            )
            _uiState.value = _uiState.value.copy(pendingInteraction = null)
        }
    }

    fun rejectInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.submitInteraction(
                cascadeId = cascadeId,
                stepIndex = interaction.stepIndex,
                responseType = interaction.type,
                confirmed = false
            )
            _uiState.value = _uiState.value.copy(pendingInteraction = null)
        }
    }

    fun proceedArtifact() {
        val cascadeId = _uiState.value.cascadeId
        val uri = _uiState.value.proceedArtifactUri ?: ""
        viewModelScope.launch {
            apiClient.proceedArtifact(cascadeId, uri)
            _uiState.value = _uiState.value.copy(canProceed = false)
        }
    }

    override fun onCleared() {
        super.onCleared()
        wsClient.disconnect()
    }
}
