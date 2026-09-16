package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
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
    val workspaceFolder: String? = null,
    val messages: List<GatewayMessageItem> = emptyList(),
    val runningTasks: List<RunningTaskItem> = emptyList(),
    val queuedMessages: List<QueuedMessageItem> = emptyList(),
    val isRunning: Boolean = false,
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val activeModel: String = "gemini-3.8-flash-high",
    val connectionStatus: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val errorMessage: String? = null,
    val isLatestMessageError: Boolean = false,
    val markdownViewerData: MarkdownFileViewerData? = null
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

        viewModelScope.launch {
            apiClient.markConversationAsRead(cascadeId)
        }

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
                val msgs = payload.messages ?: _uiState.value.messages
                val lastMsg = msgs.lastOrNull()
                val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError

                _uiState.value = _uiState.value.copy(
                    title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                    messages = msgs,
                    runningTasks = payload.runningTasks ?: emptyList(),
                    queuedMessages = payload.queuedMessages ?: emptyList(),
                    isRunning = isRunning,
                    canProceed = payload.canProceed,
                    proceedArtifactUri = payload.proceedArtifactUri,
                    pendingInteraction = payload.pendingInteraction,
                    activeModel = payload.activeModel?.let { raw ->
                        if (raw.contains("claude", ignoreCase = true) || raw.contains("m26", ignoreCase = true)) {
                            "claude-opus-4-6-thinking"
                        } else {
                            "gemini-3.8-flash-high"
                        }
                    } ?: _uiState.value.activeModel,
                    errorMessage = if (payload.hasError) payload.errorMessage else null,
                    isLatestMessageError = isError
                )
            }
        }
    }

    fun onInputTextChanged(text: String) {
        _inputText.value = text
    }

    fun toggleModel() {
        val nextModel = if (_uiState.value.activeModel.contains("claude", ignoreCase = true)) {
            "gemini-3.8-flash-high"
        } else {
            "claude-opus-4-6-thinking"
        }
        _uiState.value = _uiState.value.copy(activeModel = nextModel)
    }

    fun syncModel(model: String) {
        val lower = model.lowercase()
        val canonical = if (lower.contains("claude") || lower.contains("m26")) {
            "claude-opus-4-6-thinking"
        } else {
            "gemini-3.8-flash-high"
        }
        _uiState.value = _uiState.value.copy(activeModel = canonical)
    }

    fun insertCommitAndPush() {
        val cmd = "Commit and Push"
        val current = _inputText.value.trim()
        _inputText.value = if (current.isEmpty()) cmd else "$current\n$cmd"
    }

    fun handleContinue() {
        sendMessage("Continue")
    }

    fun sendCurrentMessage() {
        val text = _inputText.value.trim()
        if (text.isEmpty()) return
        _inputText.value = ""
        sendMessage(text)
    }

    private fun sendMessage(text: String) {
        val cascadeId = _uiState.value.cascadeId
        val model = _uiState.value.activeModel

        // Optimistically add user bubble
        val optimisticUserMsg = GatewayMessageItem(
            id = "opt_${System.currentTimeMillis()}",
            role = "user",
            content = text
        )
        _uiState.value = _uiState.value.copy(
            messages = _uiState.value.messages + optimisticUserMsg,
            isRunning = true,
            isLatestMessageError = false
        )

        viewModelScope.launch {
            val result = apiClient.sendMessage(cascadeId, text, model)
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

    fun stopTask(task: RunningTaskItem) {
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.stopTask(cascadeId, task.id)
        }
    }

    fun sendQueuedMessageNow(item: QueuedMessageItem) {
        sendMessage(item.text)
        deleteQueuedMessage(item)
    }

    fun editQueuedMessage(item: QueuedMessageItem) {
        _inputText.value = item.text
        deleteQueuedMessage(item)
    }

    fun deleteQueuedMessage(item: QueuedMessageItem) {
        _uiState.value = _uiState.value.copy(
            queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
        )
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

    fun openMarkdownViewer(uri: String, title: String) {
        _uiState.value = _uiState.value.copy(
            markdownViewerData = MarkdownFileViewerData(
                uri = uri,
                title = title,
                filename = uri.substringAfterLast('/'),
                content = "",
                summary = null,
                canProceed = _uiState.value.canProceed,
                isLoading = true
            )
        )

        viewModelScope.launch {
            val res = apiClient.fetchFileContent(uri, _uiState.value.cascadeId)
            res.onSuccess { resp ->
                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        filename = resp.filename.ifBlank { uri.substringAfterLast('/') },
                        content = resp.content,
                        summary = resp.summary,
                        isLoading = false
                    )
                )
            }.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        errorMessage = err.message ?: "无法加载文档内容",
                        isLoading = false
                    )
                )
            }
        }
    }

    fun closeMarkdownViewer() {
        _uiState.value = _uiState.value.copy(markdownViewerData = null)
    }

    fun proceedFromViewer() {
        closeMarkdownViewer()
        proceedArtifact()
    }

    override fun onCleared() {
        super.onCleared()
        wsClient.disconnect()
    }
}
