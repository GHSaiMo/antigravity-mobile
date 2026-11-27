package com.antigravity.mobile.ui.screen

import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.IntentSenderRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.core.*
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.CallEnd
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.GraphicEq
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.LinearScale
import androidx.compose.material.icons.filled.Warning
import com.antigravity.mobile.data.model.ConversationTurnBuilder
import com.antigravity.mobile.data.model.hasBlockingTasks
import com.antigravity.mobile.ui.components.ConversationNavigatorSheet
import androidx.compose.material3.*
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
import androidx.compose.material.icons.automirrored.outlined.LiveHelp
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import android.net.Uri
import com.antigravity.mobile.ui.util.rememberHaptic
import com.antigravity.mobile.ui.util.DocumentScannerHelper
import com.google.mlkit.vision.documentscanner.GmsDocumentScannerOptions
import com.google.mlkit.vision.documentscanner.GmsDocumentScanning
import com.google.mlkit.vision.documentscanner.GmsDocumentScanningResult
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.LocalTextToolbar
import androidx.compose.ui.platform.LocalView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import java.net.URLDecoder

import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChatViewModel
import com.antigravity.mobile.ui.viewmodel.handleAppBackgrounded
import com.antigravity.mobile.ui.viewmodel.handleAppForegrounded
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import com.antigravity.mobile.ui.viewmodel.addImagesFromUris
import com.antigravity.mobile.ui.viewmodel.cancelExecution
import com.antigravity.mobile.ui.viewmodel.clearSlashCommands
import com.antigravity.mobile.ui.viewmodel.removeSlashCommand
import com.antigravity.mobile.ui.viewmodel.stripTrailingSlashQuery
import com.antigravity.mobile.ui.viewmodel.ensureSlashCommands
import com.antigravity.mobile.ui.viewmodel.selectSlashCommand
import com.antigravity.mobile.ui.viewmodel.closeChangesSheet
import com.antigravity.mobile.ui.viewmodel.closeGitSheet
import com.antigravity.mobile.ui.viewmodel.commitGit
import com.antigravity.mobile.ui.viewmodel.delegateCommitToAgent
import com.antigravity.mobile.ui.viewmodel.exportMarkdown
import com.antigravity.mobile.ui.viewmodel.openChangesSheet
import com.antigravity.mobile.ui.viewmodel.openGitSheet
import com.antigravity.mobile.ui.viewmodel.refreshActionAvailability
import com.antigravity.mobile.ui.viewmodel.refreshChapterTitles
import com.antigravity.mobile.ui.viewmodel.refreshGitStatus
import com.antigravity.mobile.ui.viewmodel.retryGitPush
import com.antigravity.mobile.ui.viewmodel.confirmUndo
import com.antigravity.mobile.ui.viewmodel.deleteQueuedMessage
import com.antigravity.mobile.ui.viewmodel.downloadAndPreviewDocument
import com.antigravity.mobile.ui.viewmodel.editQueuedMessage
import com.antigravity.mobile.ui.viewmodel.initSession
import com.antigravity.mobile.ui.viewmodel.loadMessagesThrough
import com.antigravity.mobile.ui.viewmodel.loadOlderMessages
import com.antigravity.mobile.ui.viewmodel.openAttachmentImageViewer
import com.antigravity.mobile.ui.viewmodel.openMarkdownViewer
import com.antigravity.mobile.ui.viewmodel.proceedArtifact
import com.antigravity.mobile.ui.viewmodel.refresh
import com.antigravity.mobile.ui.viewmodel.removeImage
import com.antigravity.mobile.ui.viewmodel.removeFile
import com.antigravity.mobile.ui.viewmodel.retryFileUpload
import com.antigravity.mobile.ui.viewmodel.addAttachmentsFromUris
import com.antigravity.mobile.ui.viewmodel.consumeAttachmentNotice
import com.antigravity.mobile.ui.components.AttachmentPickerSheet
import com.antigravity.mobile.ui.components.AttachmentFileChip
import com.antigravity.mobile.ui.components.AudioRecordingCard
import com.antigravity.mobile.ui.components.ContextMentionPicker
import com.antigravity.mobile.ui.components.VoiceCallCard
import com.antigravity.mobile.ui.components.VoiceTroubleshootBubble
import com.antigravity.mobile.ui.util.AudioRecorderManager
import com.antigravity.mobile.ui.util.VoiceCallManager
import com.antigravity.mobile.ui.viewmodel.bindVoiceCascadeId
import com.antigravity.mobile.ui.viewmodel.ensureCascadeForVoiceCall
import com.antigravity.mobile.ui.viewmodel.requestUndo
import com.antigravity.mobile.ui.viewmodel.retryLoadMessages
import com.antigravity.mobile.ui.viewmodel.saveDraftFor
import com.antigravity.mobile.ui.viewmodel.sendQueuedMessageNow
import com.antigravity.mobile.ui.viewmodel.skipInteraction
import com.antigravity.mobile.ui.viewmodel.stopSubagent
import com.antigravity.mobile.ui.viewmodel.stopTask
import com.antigravity.mobile.ui.viewmodel.submitInteraction
import com.antigravity.mobile.ui.viewmodel.updateGlobalBackgroundTasks
import com.antigravity.mobile.ui.viewmodel.addContextMention
import com.antigravity.mobile.ui.viewmodel.closeMentionPicker
import com.antigravity.mobile.ui.viewmodel.forkConversationAt
import com.antigravity.mobile.ui.viewmodel.openMentionPicker
import com.antigravity.mobile.ui.viewmodel.prepareSession
import com.antigravity.mobile.ui.viewmodel.refreshPermissionSettings
import com.antigravity.mobile.ui.viewmodel.refreshProjectTrust
import com.antigravity.mobile.ui.viewmodel.removeContextMention
import com.antigravity.mobile.ui.viewmodel.searchContextMentions
import com.antigravity.mobile.ui.viewmodel.sendTaskToBackground
import com.antigravity.mobile.ui.viewmodel.trustCurrentProject
import com.antigravity.mobile.ui.viewmodel.startBtwAndOpen
import com.antigravity.mobile.ui.viewmodel.insertConclusionIntoParent
import com.antigravity.mobile.ui.viewmodel.setPermissionPreset
import com.antigravity.mobile.ui.components.BranchOriginBanner
import com.antigravity.mobile.ui.components.BranchForkDivider
import com.antigravity.mobile.ui.components.SideQuestionInlineCardsSection

private val BracketPlaceholderRegex = Regex("\\[[^\\[\\]\\n]+\\]")

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class, ExperimentalFoundationApi::class)
@Composable
fun ChatScreen(
    cascadeId: String,
    initialTitle: String,
    viewModel: ChatViewModel,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    isNewConversation: Boolean = false,
    isUnreadOnEntry: Boolean = false,
    initialStatus: com.antigravity.mobile.data.model.ConversationStatus? = null,
    onOpenSubagent: (com.antigravity.mobile.data.model.SubagentItem) -> Unit = {},
    onOpenConversation: (cascadeId: String, title: String) -> Unit = { _, _ -> },
    onStartNewSession: () -> Unit = {},
    onCreateAutomationWithPrompt: () -> Unit = {},
    dismissModalsTrigger: Int = 0
) {
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val focusRequester = remember { FocusRequester() }
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val textToolbar = LocalTextToolbar.current
    val dismissKeyboard: () -> Unit = {
        focusManager.clearFocus()
        keyboardController?.hide()
    }
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val inputText by viewModel.inputText.collectAsStateWithLifecycle()
    // 升级自检：当前 Antigravity 缺少某接口时，对应功能的入口直接不显示
    val compat by com.antigravity.mobile.data.model.GatewayCompatStore.state.collectAsStateWithLifecycle()
    val handleBackNavigation: () -> Unit = {
        textToolbar.hide()
        focusManager.clearFocus()
        keyboardController?.hide()
        viewModel.handleBack(cascadeId, inputText)
        onNavigateBack()
    }
    val scrollToBottomTrigger by viewModel.scrollToBottomTrigger.collectAsStateWithLifecycle()
    val listState = rememberLazyListState()
    val colors = AntigravityTheme.colors
    val isStreamingOrRunning = uiState.isAwaitingResponse || uiState.isRunning
    val lastMsg = uiState.messages.lastOrNull()
    val hasActiveToolTail = isStreamingOrRunning && lastMsg?.isTools == true
    val shouldShowThinkingBubble = isStreamingOrRunning && !hasActiveToolTail
    val activePlanId = remember(uiState.messages, isStreamingOrRunning) {
        if (!isStreamingOrRunning) return@remember null
        val lastUserIdx = uiState.messages.indexOfLast { it.isUser }
        val tailStart = lastUserIdx + 1
        if (tailStart >= uiState.messages.size) return@remember null
        var found: String? = null
        for (i in uiState.messages.lastIndex downTo tailStart) {
            val m = uiState.messages[i]
            if (m.isAgent) break
            if (m.isStepPlan) {
                found = m.id
                break
            }
        }
        found
    }
    val coroutineScope = rememberCoroutineScope()
    var adaptiveScrollJob by remember { mutableStateOf<Job?>(null) }
    var isProgrammaticScrolling by remember { mutableStateOf(false) }
    var isInputFocused by remember { mutableStateOf(false) }
    val isImeVisible = WindowInsets.isImeVisible

    // 对话导轨与章节大纲状态 (Antigravity 2.28.1 对齐)
    var showNavigatorSheet by remember { mutableStateOf(false) }
    var showMcpSkillsSheet by remember { mutableStateOf(false) }
    var mcpSkillsInitialTabOverride by remember { mutableStateOf<McpSkillsTab?>(null) }
    var showSwarmSheet by remember { mutableStateOf(false) }
    var showContextAuditSheet by remember { mutableStateOf(false) }
    var showSendQueuedNowConfirmDialog by remember { mutableStateOf(false) }
    var queuedItemToSendNow by remember { mutableStateOf<com.antigravity.mobile.data.model.QueuedMessageItem?>(null) }
    var showWorkflowTemplateSheet by remember { mutableStateOf(false) }
    var workflowSheetStartEditing by remember { mutableStateOf(false) }
    var pendingSelectFirstPlaceholder by remember { mutableStateOf(false) }
    val modelSlidePickerState = remember { com.antigravity.mobile.ui.components.ModelSlidePickerState() }
    val workflowSlidePickerState = remember { com.antigravity.mobile.ui.components.WorkflowSlidePickerState() }
    val commitSlidePickerState = remember { com.antigravity.mobile.ui.components.CommitSlidePickerState() }
    var workflowTemplates by remember { mutableStateOf(com.antigravity.mobile.ui.components.WorkflowTemplateLocalCache.load(context)) }
    LaunchedEffect(Unit) {
        val apiClient = com.antigravity.mobile.data.service.ApiClient.defaultInstance
            ?: com.antigravity.mobile.data.service.ApiClient(
                context.applicationContext,
                com.antigravity.mobile.data.service.PreferencesManager(context.applicationContext)
            )
        apiClient.fetchWorkflowTemplates().onSuccess { resp ->
            val partitioned = com.antigravity.mobile.data.model.WorkflowTemplateDefaults.partition(resp.templates)
            workflowTemplates = partitioned
            com.antigravity.mobile.ui.components.WorkflowTemplateLocalCache.save(context, partitioned)
        }
    }
    LaunchedEffect(modelSlidePickerState.isPresented) {
        if (modelSlidePickerState.isPresented && workflowSlidePickerState.isPresented) {
            workflowSlidePickerState.dismiss()
        }
    }
    LaunchedEffect(workflowSlidePickerState.isPresented) {
        if (workflowSlidePickerState.isPresented && modelSlidePickerState.isPresented) {
            modelSlidePickerState.dismiss()
        }
    }
    var inputFieldValue by remember { mutableStateOf(TextFieldValue(inputText)) }
    LaunchedEffect(inputText) {
        if (inputFieldValue.text != inputText) {
            inputFieldValue = TextFieldValue(
                text = inputText,
                selection = TextRange(inputText.length)
            )
        }
    }
    LaunchedEffect(pendingSelectFirstPlaceholder, inputText) {
        if (pendingSelectFirstPlaceholder) {
            pendingSelectFirstPlaceholder = false
            val firstMatch = BracketPlaceholderRegex.find(inputText)
            if (firstMatch != null) {
                kotlinx.coroutines.delay(80)
                inputFieldValue = TextFieldValue(
                    text = inputText,
                    selection = TextRange(firstMatch.range.first, firstMatch.range.last + 1)
                )
                focusRequester.requestFocus()
                keyboardController?.show()
            } else {
                focusRequester.requestFocus()
                keyboardController?.show()
            }
        }
    }
    var selectedWorkflowSubagent by remember { mutableStateOf<com.antigravity.mobile.data.model.SubagentItem?>(null) }
    var lockedTurnIndex by remember { mutableStateOf<Int?>(null) }
    var jumpingToTurnIndex by remember { mutableStateOf<Int?>(null) }
    var jumpTransitionDimmed by remember { mutableStateOf(false) }
    var highlightedTurnMessageId by remember { mutableStateOf<String?>(null) }
    var activeTurnJumpJob by remember { mutableStateOf<kotlinx.coroutines.Job?>(null) }
    val listJumpAlpha by androidx.compose.animation.core.animateFloatAsState(
        targetValue = if (jumpTransitionDimmed) 0.35f else 1.0f,
        animationSpec = androidx.compose.animation.core.tween(durationMillis = 200),
        label = "listJumpAlpha"
    )
    LaunchedEffect(highlightedTurnMessageId) {
        val currentId = highlightedTurnMessageId ?: return@LaunchedEffect
        delay(1800L)
        if (highlightedTurnMessageId == currentId) {
            highlightedTurnMessageId = null
        }
    }
    var isSlashPickerDismissed by remember { mutableStateOf(false) }
    var previousInputTextForSlash by remember { mutableStateOf(inputText) }
    LaunchedEffect(inputText) {
        val q = com.antigravity.mobile.data.model.SlashCommandFilter.queryOf(inputText)
        if (q == null) {
            isSlashPickerDismissed = false
        } else if (inputText.endsWith("/") && !previousInputTextForSlash.endsWith("/")) {
            isSlashPickerDismissed = false
        }
        previousInputTextForSlash = inputText
    }
    val turns = remember(uiState.messages, uiState.turnOutline, uiState.isRunning, uiState.isAwaitingResponse) {
        ConversationTurnBuilder.buildTurns(
            uiState.messages,
            uiState.turnOutline.takeIf { it.isNotEmpty() },
            uiState.isRunning,
            uiState.isAwaitingResponse
        )
    }
    val chapters = remember(turns, uiState.chapterTitles) {
        ConversationTurnBuilder.buildChapters(turns, aiTitles = uiState.chapterTitles)
    }

    // Scroll state tracking: distinguish initial logical entry alignment vs in-session incremental scroll
    // 对齐 iOS preserveScrollOnReturn：压入子页面（如子代理会话）后返回，保持原滚动位置，不重新对齐到底部。
    // 这些状态用 rememberSaveable，才能在页面离开组合后随回退栈条目恢复。
    var hasInitiallyAligned by rememberSaveable(cascadeId) { mutableStateOf(false) }
    var hasUserInteracted by rememberSaveable(cascadeId) { mutableStateOf(false) }
    var preserveScrollOnReturn by rememberSaveable(cascadeId) { mutableStateOf(false) }
    var savedScrollIndex by rememberSaveable(cascadeId) { mutableIntStateOf(0) }
    var savedScrollOffset by rememberSaveable(cascadeId) { mutableIntStateOf(0) }
    var previousMessageCount by remember(cascadeId) { mutableIntStateOf(uiState.messages.size) }
    var lastTrigger by remember(cascadeId) { mutableIntStateOf(scrollToBottomTrigger) }

    val isNearBottom by remember {
        derivedStateOf {
            val layoutInfo = listState.layoutInfo
            val visibleItems = layoutInfo.visibleItemsInfo
            if (visibleItems.isEmpty()) true
            else {
                val lastVisibleIndex = visibleItems.last().index
                lastVisibleIndex >= layoutInfo.totalItemsCount - 2
            }
        }
    }

    LaunchedEffect(listState.isScrollInProgress, isProgrammaticScrolling) {
        if (listState.isScrollInProgress && !isProgrammaticScrolling && lockedTurnIndex != null) {
            lockedTurnIndex = null
        }
    }

    var stepperAlphaTarget by remember { mutableFloatStateOf(0f) }
    val animatedStepperAlpha by androidx.compose.animation.core.animateFloatAsState(
        targetValue = stepperAlphaTarget,
        animationSpec = androidx.compose.animation.core.tween(durationMillis = 250),
        label = "stepperAlpha"
    )
    var lastFirstVisibleItemIndex by remember { mutableIntStateOf(0) }
    var lastFirstVisibleItemScrollOffset by remember { mutableIntStateOf(0) }

    LaunchedEffect(
        listState.firstVisibleItemIndex,
        listState.firstVisibleItemScrollOffset,
        isNearBottom,
        isInputFocused,
        turns.size
    ) {
        if (isNearBottom || isInputFocused || turns.size < 2) {
            stepperAlphaTarget = 0f
            return@LaunchedEffect
        }

        val currentIndex = listState.firstVisibleItemIndex
        val currentOffset = listState.firstVisibleItemScrollOffset

        val isScrollingDown = when {
            currentIndex > lastFirstVisibleItemIndex -> true
            currentIndex < lastFirstVisibleItemIndex -> false
            else -> currentOffset > lastFirstVisibleItemScrollOffset + 4
        }
        val isScrollingUp = when {
            currentIndex < lastFirstVisibleItemIndex -> true
            currentIndex > lastFirstVisibleItemIndex -> false
            else -> currentOffset < lastFirstVisibleItemScrollOffset - 4
        }

        lastFirstVisibleItemIndex = currentIndex
        lastFirstVisibleItemScrollOffset = currentOffset

        if (isScrollingUp) {
            stepperAlphaTarget = 1.0f
            kotlinx.coroutines.delay(3000L)
            stepperAlphaTarget = 0.3f
            kotlinx.coroutines.delay(7000L)
            stepperAlphaTarget = 0f
        } else if (isScrollingDown) {
            stepperAlphaTarget = 0.3f
            kotlinx.coroutines.delay(10000L)
            stepperAlphaTarget = 0f
        }
    }

    val turnIndexByMessageId = remember(turns) {
        buildMap<String, Int> {
            for (t in turns) {
                for (m in t.messages) {
                    if (m.id.isNotBlank()) {
                        put(m.id, t.turnIndex)
                    }
                }
            }
        }
    }
    val validTurnIndices = remember(turns) {
        turns.mapTo(HashSet(turns.size)) { it.turnIndex }
    }

    val currentTurnIndex by remember(turns, turnIndexByMessageId, validTurnIndices) {
        derivedStateOf {
            if (turns.isEmpty()) {
                1
            } else if (lockedTurnIndex != null && validTurnIndices.contains(lockedTurnIndex)) {
                lockedTurnIndex!!
            } else if (!hasInitiallyAligned || isNearBottom || !listState.canScrollForward) {
                turns.lastOrNull()?.turnIndex ?: 1
            } else if (!listState.canScrollBackward) {
                val firstMsgId = uiState.messages.firstOrNull()?.id
                val firstLoadedTurn = firstMsgId?.let { turnIndexByMessageId[it] }
                firstLoadedTurn ?: turns.firstOrNull()?.turnIndex ?: 1
            } else {
                val layoutInfo = listState.layoutInfo
                val anchorY = (layoutInfo.viewportEndOffset * 0.45f).toInt()
                val messageItems = layoutInfo.visibleItemsInfo.filter { item ->
                    val k = item.key as? String
                    k != null && turnIndexByMessageId.containsKey(k)
                }
                val bestItem = messageItems.minByOrNull { item ->
                    val top = item.offset
                    val bottom = item.offset + item.size
                    if (anchorY in top..bottom) 0 else minOf(kotlin.math.abs(top - anchorY), kotlin.math.abs(bottom - anchorY))
                }
                val targetId = bestItem?.key as? String
                if (targetId != null) {
                    turnIndexByMessageId[targetId] ?: turns.lastOrNull()?.turnIndex ?: 1
                } else {
                    turns.lastOrNull()?.turnIndex ?: 1
                }
            }
        }
    }

    val latestOnOpenSubagent by rememberUpdatedState(onOpenSubagent)
    val latestHasInitiallyAligned by rememberUpdatedState(hasInitiallyAligned)
    val openSubagent: (com.antigravity.mobile.data.model.SubagentItem) -> Unit = remember(listState) {
        { sub ->
            savedScrollIndex = listState.firstVisibleItemIndex
            savedScrollOffset = listState.firstVisibleItemScrollOffset
            preserveScrollOnReturn = latestHasInitiallyAligned
            latestOnOpenSubagent(sub)
        }
    }

    fun jumpToTurn(turn: com.antigravity.mobile.data.model.ConversationTurn) {
        activeTurnJumpJob?.cancel()
        lockedTurnIndex = turn.turnIndex
        hasUserInteracted = true
        activeTurnJumpJob = coroutineScope.launch {
            isProgrammaticScrolling = true
            try {
                val currentState = viewModel.uiState.value
                val existingIdx = currentState.messages.indexOfFirst { it.id == turn.firstMessageId }
                if (existingIdx >= 0) {
                    jumpTransitionDimmed = false
                    jumpingToTurnIndex = null
                    val hasHeader = if (currentState.hasMore) 1 else 0
                    val scrollIdx = existingIdx + hasHeader
                    val distance = kotlin.math.abs(listState.firstVisibleItemIndex - scrollIdx)
                    if (distance > 30) {
                        listState.scrollToItem(scrollIdx)
                    } else {
                        listState.animateScrollToItem(scrollIdx)
                    }
                    highlightedTurnMessageId = turn.firstMessageId
                    return@launch
                }

                jumpingToTurnIndex = turn.turnIndex
                jumpTransitionDimmed = true
                viewModel.loadMessagesThrough(turn.firstMessageId, turn.messageOffset)
                val updatedState = viewModel.uiState.value
                val updatedMsgs = updatedState.messages
                if (updatedMsgs.isNotEmpty()) {
                    val targetIdx = updatedMsgs.indexOfFirst { it.id == turn.firstMessageId }
                        .takeIf { it >= 0 }
                        ?: 0
                    val hasHeader = if (updatedState.hasMore) 1 else 0
                    val scrollIdx = targetIdx + hasHeader
                    delay(40L)
                    listState.scrollToItem(scrollIdx)
                    delay(60L)
                    highlightedTurnMessageId = updatedMsgs.getOrNull(targetIdx)?.id ?: turn.firstMessageId
                }
            } finally {
                jumpTransitionDimmed = false
                jumpingToTurnIndex = null
                delay(50L)
                isProgrammaticScrolling = false
            }
        }
    }

    // 对齐 iOS performAdaptiveCardScroll：多阶段弹性阻尼自适应滚动与贴边回弹
    fun performAdaptiveCardScroll() {
        hasUserInteracted = false
        adaptiveScrollJob?.cancel()
        adaptiveScrollJob = coroutineScope.launch {
            isProgrammaticScrolling = true
            try {
                val total = listState.layoutInfo.totalItemsCount
                if (total <= 0) return@launch

                // 阶段 1: 立即使用弹性动画启动滚动，卡片展开向上弹起，卡片折叠直接贴着卡片边缘回弹
                try {
                    listState.animateScrollToItem(total - 1)
                } catch (_: Exception) {}

                // 阶段 2: 80ms 连续多阶段布局微调吸附
                delay(80)
                try {
                    val t1 = listState.layoutInfo.totalItemsCount
                    if (t1 > 0) listState.animateScrollToItem(t1 - 1)
                } catch (_: Exception) {}

                // 阶段 3: 200ms (80 + 120ms) 布局中间态贴边
                delay(120)
                try {
                    val t2 = listState.layoutInfo.totalItemsCount
                    if (t2 > 0) listState.animateScrollToItem(t2 - 1)
                } catch (_: Exception) {}

                // 阶段 4: 360ms (200 + 160ms) 接近终态吸附
                delay(160)
                try {
                    val t3 = listState.layoutInfo.totalItemsCount
                    if (t3 > 0) listState.animateScrollToItem(t3 - 1)
                } catch (_: Exception) {}

                // 阶段 5: 480ms (360 + 120ms) 消除折叠卡片后的悬空留白，终态贴边回弹
                delay(120)
                try {
                    val t4 = listState.layoutInfo.totalItemsCount
                    if (t4 > 0) listState.scrollToItem(t4 - 1)
                } catch (_: Exception) {}
            } finally {
                delay(50)
                isProgrammaticScrolling = false
            }
        }
    }

    fun handleFloatingCardToggle(isExpanded: Boolean) {
        hasUserInteracted = false
        performAdaptiveCardScroll()
    }

    val isRefreshing by viewModel.isRefreshing.collectAsStateWithLifecycle()
    val pullRefreshState = rememberPullToRefreshState()
    var refreshTriggerToken by remember { mutableIntStateOf(0) }



    // "+" panel state, system file picker (SAF) and camera capture
    var showAttachmentSheet by remember { mutableStateOf(false) }
    val filePickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.OpenMultipleDocuments()
    ) { uris ->
        if (uris.isNotEmpty()) {
            showAttachmentSheet = false
            viewModel.addAttachmentsFromUris(context, uris)
        }
    }
    var pendingCameraUri by remember { mutableStateOf<android.net.Uri?>(null) }
    val cameraLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.TakePicture()
    ) { success ->
        val captured = pendingCameraUri
        pendingCameraUri = null
        if (success && captured != null) {
            showAttachmentSheet = false
            viewModel.addImagesFromUris(context, listOf(captured))
        }
    }
    val launchCamera: () -> Unit = {
        try {
            val dir = java.io.File(context.cacheDir, "camera").apply { mkdirs() }
            val photo = java.io.File(dir, "capture_${System.currentTimeMillis()}.jpg")
            val uri = androidx.core.content.FileProvider.getUriForFile(
                context, "${context.packageName}.fileprovider", photo
            )
            pendingCameraUri = uri
            cameraLauncher.launch(uri)
        } catch (e: Exception) {
            android.widget.Toast.makeText(context, "无法打开相机".loc, android.widget.Toast.LENGTH_SHORT).show()
        }
    }
    // The manifest declares CAMERA, so the system camera intent requires the permission to be granted.
    val cameraPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted -> if (granted) launchCamera() }

    val isDocumentScannerSupported = remember(context) { DocumentScannerHelper.isSupported(context) }
    val documentScannerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.StartIntentSenderForResult()
    ) { activityResult ->
        try {
            if (activityResult.resultCode == android.app.Activity.RESULT_OK) {
                val result = GmsDocumentScanningResult.fromActivityResultIntent(activityResult.data)
                val pageUris = result?.pages?.mapNotNull { it.imageUri } ?: emptyList()
                if (pageUris.isNotEmpty()) {
                    showAttachmentSheet = false
                    viewModel.addImagesFromUris(context, pageUris)
                }
            }
        } catch (t: Throwable) {
            android.util.Log.e("ChatScreen", "解析文档扫描结果异常".loc, t)
        }
    }
    val launchDocumentScanner: () -> Unit = {
        try {
            val activity = DocumentScannerHelper.findActivity(context)
            if (activity != null) {
                val remainingSlots = (9 - uiState.selectedImages.size).coerceIn(1, 9)
                val options = GmsDocumentScannerOptions.Builder()
                    .setGalleryImportAllowed(false)
                    .setPageLimit(remainingSlots)
                    .setResultFormats(GmsDocumentScannerOptions.RESULT_FORMAT_JPEG)
                    .setScannerMode(GmsDocumentScannerOptions.SCANNER_MODE_FULL)
                    .build()
                val scanner = GmsDocumentScanning.getClient(options)
                scanner.getStartScanIntent(activity)
                    .addOnSuccessListener { intentSender ->
                        try {
                            val request = IntentSenderRequest.Builder(intentSender).build()
                            documentScannerLauncher.launch(request)
                        } catch (e: Exception) {
                            android.widget.Toast.makeText(context, "无法启动文档扫描".loc, android.widget.Toast.LENGTH_SHORT).show()
                        }
                    }
                    .addOnFailureListener { e ->
                        android.widget.Toast.makeText(context, "启动文档扫描失败: %s".loc(e.localizedMessage ?: "服务不可用".loc), android.widget.Toast.LENGTH_SHORT).show()
                    }
            } else {
                android.widget.Toast.makeText(context, "无法获取界面上下文".loc, android.widget.Toast.LENGTH_SHORT).show()
            }
        } catch (t: Throwable) {
            android.util.Log.e("ChatScreen", "初始化文档扫描异常".loc, t)
            android.widget.Toast.makeText(context, "当前设备暂不支持原生文档扫描".loc, android.widget.Toast.LENGTH_SHORT).show()
        }
    }

    val voicePrefs = remember { viewModel.prefs ?: com.antigravity.mobile.data.service.PreferencesManager(context) }
    val audioRecorder = remember { AudioRecorderManager(context) }
    val isRecordingAudio by audioRecorder.isRecording.collectAsState()
    val voiceCall = remember { VoiceCallManager(context) }
    val isInVoiceCall by voiceCall.isInCall.collectAsState()
    var showVoiceTroubleshootBubble by remember { mutableStateOf(false) }

    // 语音明确失败（此时通话已被自动挂断）后弹出排查气泡；安装后永久只弹出一次
    LaunchedEffect(voiceCall) {
        voiceCall.failureEvents.collect {
            if (!voicePrefs.hasShownVoiceTroubleshootBubble) {
                voicePrefs.hasShownVoiceTroubleshootBubble = true
                showVoiceTroubleshootBubble = true
            }
        }
    }
    LaunchedEffect(isInVoiceCall) {
        if (isInVoiceCall) showVoiceTroubleshootBubble = false
    }

    // 语音通话期间保持屏幕常亮活跃，避免手机自动息屏断流
    val currentLocalView = LocalView.current
    DisposableEffect(isInVoiceCall) {
        if (isInVoiceCall) {
            val previous = currentLocalView.keepScreenOn
            currentLocalView.keepScreenOn = true
            onDispose {
                currentLocalView.keepScreenOn = previous
            }
        } else {
            onDispose {}
        }
    }
    val enableVoiceMode by voicePrefs.enableVoiceModeFlow.collectAsState()
    val fontSizeScaleSetting by voicePrefs.fontSizeScaleFlow.collectAsState()
    val isKnowledgeModeEnabled by voicePrefs.knowledgeModeFlow.collectAsState()
    val compactCollapseTools by voicePrefs.compactCollapseToolsFlow.collectAsState()
    val compactCollapseTasks by voicePrefs.compactCollapseTasksFlow.collectAsState()
    val compactHideHealthyContext by voicePrefs.compactHideHealthyContextFlow.collectAsState()
    val modelCatalog by voicePrefs.modelCatalogFlow.collectAsState()

    val shouldCollapseTools = isKnowledgeModeEnabled && compactCollapseTools
    val shouldCollapseTasks = isKnowledgeModeEnabled && compactCollapseTasks
    val isSessionRunning = uiState.isRunning || uiState.isAwaitingResponse
    val hasPendingApproval = uiState.pendingInteraction != null || uiState.canProceed
    val shouldShowRealtimeContext = uiState.contextMeter?.let { meter ->
        !(isKnowledgeModeEnabled && compactHideHealthyContext) ||
            meter.isOverflow == true ||
            meter.percentage >= 70.0 ||
            meter.hasCacheWarning(isRunning = isSessionRunning, hasPendingInteraction = hasPendingApproval)
    } ?: false

    LaunchedEffect(enableVoiceMode) {
        if (!enableVoiceMode && isInVoiceCall) {
            voiceCall.endCall()
        }
    }

    val startVoiceCallInternal: () -> Unit = {
        if (isRecordingAudio) {
            audioRecorder.cancelRecording()
        }
        voiceCall.onCascadeBound = { newCascadeId ->
            viewModel.bindVoiceCascadeId(newCascadeId)
        }
        viewModel.ensureCascadeForVoiceCall { readyCascadeId, baseUrl, token ->
            val ok = voiceCall.startCall(
                cascadeId = readyCascadeId,
                modelIdOrEnum = uiState.activeModel,
                baseUrl = baseUrl,
                deviceToken = token
            )
            if (!ok) {
                android.widget.Toast.makeText(context, "无法启动实时语音通话".loc, android.widget.Toast.LENGTH_SHORT).show()
            }
        }
    }

    val voiceCallPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            startVoiceCallInternal()
        } else {
            android.widget.Toast.makeText(context, "需要麦克风权限以开启实时语音对话".loc, android.widget.Toast.LENGTH_SHORT).show()
        }
    }

    val launchVoiceCall: () -> Unit = {
        if (androidx.core.content.ContextCompat.checkSelfPermission(
                context, android.Manifest.permission.RECORD_AUDIO
            ) == android.content.pm.PackageManager.PERMISSION_GRANTED
        ) {
            startVoiceCallInternal()
        } else {
            voiceCallPermissionLauncher.launch(android.Manifest.permission.RECORD_AUDIO)
        }
    }

    LaunchedEffect(uiState.activeModel, isInVoiceCall) {
        if (isInVoiceCall) {
            voiceCall.updatePlanModel(uiState.activeModel)
        }
    }

    val audioPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            showAttachmentSheet = false
            if (isInVoiceCall) {
                voiceCall.endCall()
            }
            val success = audioRecorder.startRecording()
            if (!success) {
                android.widget.Toast.makeText(context, "无法启动录音".loc, android.widget.Toast.LENGTH_SHORT).show()
            }
        } else {
            android.widget.Toast.makeText(context, "需要麦克风权限以录制音频".loc, android.widget.Toast.LENGTH_SHORT).show()
        }
    }
    val launchAudioRecording: () -> Unit = {
        if (androidx.core.content.ContextCompat.checkSelfPermission(
                context, android.Manifest.permission.RECORD_AUDIO
            ) == android.content.pm.PackageManager.PERMISSION_GRANTED
        ) {
            showAttachmentSheet = false
            if (isInVoiceCall) {
                voiceCall.endCall()
            }
            val success = audioRecorder.startRecording()
            if (!success) {
                android.widget.Toast.makeText(context, "无法启动录音".loc, android.widget.Toast.LENGTH_SHORT).show()
            }
        } else {
            audioPermissionLauncher.launch(android.Manifest.permission.RECORD_AUDIO)
        }
    }

    val narrationPlayer = remember(context) {
        val client = com.antigravity.mobile.data.service.ApiClient.defaultInstance
            ?: com.antigravity.mobile.data.service.ApiClient(context.applicationContext, voicePrefs)
        com.antigravity.mobile.ui.util.NarrationAudioPlayer(context.applicationContext, client)
    }
    val dictationManager = remember(context) {
        val client = com.antigravity.mobile.data.service.ApiClient.defaultInstance
            ?: com.antigravity.mobile.data.service.ApiClient(context.applicationContext, voicePrefs)
        com.antigravity.mobile.ui.util.AudioDictationManager(client)
    }
    val isDictating by dictationManager.isDictating.collectAsState()
    val isNarrating by narrationPlayer.isPlaying.collectAsState()
    val narrationLabel by narrationPlayer.statusLabel.collectAsState()
    var showPermissionMenu by remember { mutableStateOf(false) }
    var pendingTurboModeConfirm by remember { mutableStateOf(false) }
    var showBtwSheet by remember { mutableStateOf(false) }
    var btwAnchorStepIndex by remember { mutableStateOf<Int?>(null) }
    var btwAnchorPreview by remember { mutableStateOf<String?>(null) }
    var showSharedBranchContext by remember(cascadeId) { mutableStateOf(false) }
    var selectedGitHubPRUrl by remember { mutableStateOf<String?>(null) }
    var phase3Notice by remember { mutableStateOf<String?>(null) }
    var dictationBaseText by remember { mutableStateOf("") }

    var lastSeenDismissTrigger by remember { mutableIntStateOf(dismissModalsTrigger) }
    LaunchedEffect(dismissModalsTrigger) {
        if (dismissModalsTrigger != lastSeenDismissTrigger) {
            lastSeenDismissTrigger = dismissModalsTrigger
            showNavigatorSheet = false
            showMcpSkillsSheet = false
            mcpSkillsInitialTabOverride = null
            showSwarmSheet = false
            showContextAuditSheet = false
            showSendQueuedNowConfirmDialog = false
            queuedItemToSendNow = null
            showWorkflowTemplateSheet = false
            workflowSheetStartEditing = false
            showAttachmentSheet = false
            showVoiceTroubleshootBubble = false
            showPermissionMenu = false
            pendingTurboModeConfirm = false
            showBtwSheet = false
            btwAnchorStepIndex = null
            btwAnchorPreview = null
            selectedWorkflowSubagent = null
            selectedGitHubPRUrl = null
            viewModel.closeMarkdownViewer()
            viewModel.closeImageViewer()
            viewModel.closeDocumentPreview()
            viewModel.closeWeChatArchive()
            viewModel.closeChangesSheet()
            viewModel.closeGitSheet()
            viewModel.dismissConfirmUndo()
        }
    }

    val startDictation: () -> Unit = {
        dictationManager.start(
            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
            initialText = inputText,
            onTranscriptUpdate = { fullText ->
                viewModel.onInputTextChanged(fullText)
            },
            onError = { err -> phase3Notice = err }
        )
    }
    val dictationPermissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) startDictation()
        else phase3Notice = "需要麦克风权限以开启流式听写".loc
    }
    val toggleDictation: () -> Unit = {
        if (isDictating) {
            dictationManager.stop()
        } else if (androidx.core.content.ContextCompat.checkSelfPermission(
                context, android.Manifest.permission.RECORD_AUDIO
            ) == android.content.pm.PackageManager.PERMISSION_GRANTED
        ) {
            if (isInVoiceCall) voiceCall.endCall()
            startDictation()
        } else {
            dictationPermissionLauncher.launch(android.Manifest.permission.RECORD_AUDIO)
        }
    }

    LaunchedEffect(phase3Notice) {
        if (phase3Notice != null) {
            kotlinx.coroutines.delay(2600)
            phase3Notice = null
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            audioRecorder.cancelRecording()
            voiceCall.endCall()
            dictationManager.stop()
            narrationPlayer.stop()
        }
    }

    LaunchedEffect(uiState.attachmentNotice) {
        uiState.attachmentNotice?.let {
            android.widget.Toast.makeText(context, it, android.widget.Toast.LENGTH_LONG).show()
            viewModel.consumeAttachmentNotice()
        }
    }

    val lifecycleOwner = LocalLifecycleOwner.current

    BackHandler {
        handleBackNavigation()
    }

    DisposableEffect(lifecycleOwner, cascadeId) {
        var hasLeftForeground = false
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_PAUSE -> {
                    textToolbar.hide()
                    viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
                }
                Lifecycle.Event.ON_STOP -> {
                    hasLeftForeground = true
                    textToolbar.hide()
                    viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
                    viewModel.handleAppBackgrounded()
                }
                Lifecycle.Event.ON_START, Lifecycle.Event.ON_RESUME -> {
                    if (hasLeftForeground) {
                        hasLeftForeground = false
                        viewModel.handleAppForegrounded()
                    }
                }
                else -> {}
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            textToolbar.hide()
            focusManager.clearFocus()
            lifecycleOwner.lifecycle.removeObserver(observer)
            viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
        }
    }

    // 共享的 ChatViewModel 同一时刻只装载一个会话：子代理会话压栈后，返回时必须把本会话重新装回来
    // （并还原进入子页面前的滚动位置）。进入时和「回到前台却发现装载的不是自己」时共用这一段。
    suspend fun enterSession() {
        val returningFromChild = preserveScrollOnReturn
        if (!returningFromChild) {
            hasInitiallyAligned = false
            hasUserInteracted = false
            previousMessageCount = 0
        }
        lastTrigger = scrollToBottomTrigger
        viewModel.initSession(
            cascadeId = cascadeId,
            initialTitle = initialTitle,
            isNewConversation = isNewConversation,
            isUnread = isUnreadOnEntry,
            conversationStatus = initialStatus
        )
        if (returningFromChild) {
            isProgrammaticScrolling = true
            try {
                withTimeoutOrNull(1500) {
                    snapshotFlow { uiState.cascadeId == cascadeId && uiState.messages.isNotEmpty() }.first { it }
                }
                previousMessageCount = uiState.messages.size
                listState.scrollToItem(savedScrollIndex, savedScrollOffset)
            } finally {
                delay(50)
                isProgrammaticScrolling = false
            }
            delay(1500)
            preserveScrollOnReturn = false
        }
    }

    LaunchedEffect(cascadeId, isNewConversation) {
        enterSession()
        viewModel.refreshPermissionSettings()
        viewModel.refreshProjectTrust()
        // Automatically focus the input field and pop up soft keyboard ONLY on new conversation creation without pending options
        if (isNewConversation && uiState.pendingInteraction == null) {
            delay(250)
            try {
                focusRequester.requestFocus()
                keyboardController?.show()
            } catch (_: Exception) {}
        }
    }

    // 兜底：本页回到前台（RESUMED）时，若共享 ViewModel 装载的仍是别的会话（典型：从子代理会话返回），重新装载本会话
    val lifecycleState by lifecycleOwner.lifecycle.currentStateFlow.collectAsState()
    val isScreenResumed = lifecycleState == Lifecycle.State.RESUMED
    val isSessionMismatched = uiState.cascadeId != cascadeId
    LaunchedEffect(isScreenResumed, isSessionMismatched) {
        if (isScreenResumed && isSessionMismatched && !cascadeId.startsWith("local_draft_") && !isNewConversation) {
            enterSession()
        }
    }

    LaunchedEffect(uiState.focusInputTrigger) {
        if (uiState.focusInputTrigger > 0) {
            delay(150)
            try {
                focusRequester.requestFocus()
                keyboardController?.show()
            } catch (_: Exception) {}
        }
    }

    fun computeTargetPosition(): Int {
        val totalCount = listState.layoutInfo.totalItemsCount
        val shouldScrollToTurnStart = viewModel.shouldScrollToTurnStartOnEntry ||
            isUnreadOnEntry ||
            (initialStatus?.isError == true) ||
            (initialStatus?.needsAction == true)

        val hasHeader = if (uiState.hasMore) 1 else 0
        val fallbackLastIndex = maxOf(0, uiState.messages.size + hasHeader)
        val defaultBottomIndex = if (totalCount > 0) totalCount - 1 else fallbackLastIndex

        if (shouldScrollToTurnStart) {
            val targetId = viewModel.latestAgentMessageId ?: viewModel.latestTurnStartMessageId
            val msgIndex = if (targetId != null) {
                uiState.messages.indexOfFirst { it.id == targetId }
            } else -1

            return if (msgIndex >= 0) {
                msgIndex + hasHeader
            } else {
                defaultBottomIndex
            }
        }
        return defaultBottomIndex
    }

    suspend fun performInitialAlignment() {
        if (uiState.messages.isNotEmpty()) {
            val targetIndex = computeTargetPosition()
            if (targetIndex >= 0) {
                isProgrammaticScrolling = true
                try {
                    listState.scrollToItem(targetIndex, 0)
                } finally {
                    delay(50)
                    isProgrammaticScrolling = false
                }
            }
        }
    }

    // Phase 1: Instant alignment when messages become available (no animation to prevent slider bounce)
    LaunchedEffect(uiState.messages.isNotEmpty(), cascadeId) {
        if (uiState.messages.isNotEmpty() && !hasInitiallyAligned) {
            performInitialAlignment()
            delay(50)
            if (!hasUserInteracted) {
                performInitialAlignment()
            }
            delay(150)
            if (!hasUserInteracted) {
                performInitialAlignment()
                hasInitiallyAligned = true
            }
        }
    }

    // Phase 2: Calibration after network sync completes (if user hasn't scrolled manually)
    LaunchedEffect(uiState.isLoading) {
        if (!uiState.isLoading && uiState.messages.isNotEmpty() && !preserveScrollOnReturn) {
            if (!hasUserInteracted) {
                delay(30)
                performInitialAlignment()
                hasInitiallyAligned = true
            }
        }
    }

    // Phase 3: In-session incremental scrolling (sending new message, thinking bubble, or explicit triggers)
    LaunchedEffect(uiState.messages.size, shouldShowThinkingBubble, scrollToBottomTrigger) {
        val isExplicitTrigger = scrollToBottomTrigger != lastTrigger
        lastTrigger = scrollToBottomTrigger

        val msgSizeChanged = uiState.messages.size != previousMessageCount
        previousMessageCount = uiState.messages.size

        if (!hasInitiallyAligned || preserveScrollOnReturn) {
            // Guard: Initial alignment handles entry positioning with instant scrollToItem
            return@LaunchedEffect
        }

        if (isExplicitTrigger) {
            performAdaptiveCardScroll()
            return@LaunchedEffect
        }

        if (msgSizeChanged || shouldShowThinkingBubble) {
            if (isNearBottom || !hasUserInteracted) {
                delay(20)
                val total = listState.layoutInfo.totalItemsCount
                if (total > 0) {
                    isProgrammaticScrolling = true
                    try {
                        // 新 Agent 气泡进入流式传输时无动画贴底，避免与 rememberPacedMessage 的生长跟随冲突抖动
                        if (msgSizeChanged && uiState.isRunning && uiState.messages.lastOrNull()?.isAgent == true) {
                            listState.scrollToItem(total - 1)
                        } else {
                            listState.animateScrollToItem(total - 1)
                        }
                    } finally {
                        delay(50)
                        isProgrammaticScrolling = false
                    }
                }
            }
        }
    }

    // Phase 4: 对齐 iOS：任务或队列列表数量变化时触发自适应吸附滚动
    LaunchedEffect(uiState.runningTasks.size, uiState.queuedMessages.size) {
        if (hasInitiallyAligned && !preserveScrollOnReturn && (isNearBottom || !hasUserInteracted)) {
            performAdaptiveCardScroll()
        }
    }

    // Phase 5: 对齐 iOS isInputFocused：输入框获焦或键盘弹出时，会话内容自动往上顶，让用户可以看到会话的最底端
    LaunchedEffect(isInputFocused, isImeVisible) {
        if (preserveScrollOnReturn) {
            // 从子页面返回时保持原位置，不做贴底
        } else if (isInputFocused || isImeVisible) {
            hasUserInteracted = false
            performAdaptiveCardScroll()
        } else if (hasInitiallyAligned) {
            // 键盘完全收起并恢复完整视口高度后，做底部对齐校准，消除悬空留白
            delay(280)
            if (isNearBottom || !hasUserInteracted) {
                performAdaptiveCardScroll()
            }
        }
    }

    // 在新选项卡片出现时默认收起旧的底部软键盘；按交互稳定标识去重，避免流式/轮询刷新误关自定义回答键盘
    val pendingInteractionKey = uiState.pendingInteraction?.let {
        "${it.trajectoryId}:${it.stepIndex}:${it.type}:${it.title.hashCode()}"
    }
    LaunchedEffect(pendingInteractionKey) {
        if (pendingInteractionKey != null) {
            dismissKeyboard()
        }
    }

    // Auto-dismiss keyboard when user manually scrolls through messages and track user manual scroll
    LaunchedEffect(listState.isScrollInProgress) {
        if (listState.isScrollInProgress && !isProgrammaticScrolling) {
            // 仅在软键盘显示或输入框聚焦时收起软键盘，绝不在此处清除全局焦点，确保选中的文字选框 (SelectionContainer) 在移动屏幕时稳稳保留
            if (isImeVisible || isInputFocused) {
                keyboardController?.hide()
            }
            if (hasInitiallyAligned) {
                if (isNearBottom) {
                    hasUserInteracted = false
                } else {
                    hasUserInteracted = true
                }
            }
        }
    }

    val handleFileOrLinkClick: (String, String) -> Unit = { uri, title ->
        dismissKeyboard()
        val clean = uri.trim()
        val lower = clean.lowercase()
        val pathLower = lower.substringBefore('#').substringBefore('?').replace(Regex(""":l?\d+(?:[:-]l?\d+)?$"""), "")
        val uriFileName = com.antigravity.mobile.data.service.FileIconResolver.extractFileNameFromUri(clean)
        val decodedTitle = try {
            URLDecoder.decode(title.replace("+", "%2B"), "UTF-8")
        } catch (_: Exception) {
            title
        }.trim()
        val displayTitle = decodedTitle.ifBlank { uriFileName }
        val effectiveFileName = uriFileName.ifBlank { displayTitle }

        // 0. Native Sidecar UI links (sidecar://<id>/...)
        if (lower.startsWith("sidecar://")) {
            val rest = clean.substring("sidecar://".length).trimStart('/')
            val sidecarId = rest.substringBefore('/').trim()
            if (sidecarId.equals("swarm", ignoreCase = true) || rest.contains("swarm_board", ignoreCase = true)) {
                haptic.light()
                showSwarmSheet = true
            } else if (sidecarId.lowercase() in setOf("dashboard", "automations", "sidecars", "schedule", "schedules")) {
                haptic.light()
                mcpSkillsInitialTabOverride = McpSkillsTab.AUTOMATIONS
                showMcpSkillsSheet = true
            } else if (rest.isNotEmpty()) {
                val base = viewModel.apiClient.currentBaseUrl?.trimEnd('/').orEmpty()
                if (base.isNotEmpty()) {
                    haptic.light()
                    val rawTarget = "$base/gateway/sidecar-ui/$rest"
                    coroutineScope.launch {
                        val authedUrl = viewModel.apiClient.getWsTicket().getOrNull()?.let { ticket ->
                            val sep = if (rawTarget.contains('?')) "&" else "?"
                            "${rawTarget}${sep}ticket=${java.net.URLEncoder.encode(ticket, "UTF-8")}"
                        } ?: rawTarget
                        com.antigravity.mobile.ui.util.InAppBrowserUtils.openUrl(context, authedUrl)
                    }
                }
            }
        } else if (!com.antigravity.mobile.data.service.FileIconResolver.isScriptFile(clean) &&
            (uriFileName.isNotEmpty() || !com.antigravity.mobile.data.service.FileIconResolver.isScriptFile(displayTitle))
        ) {
            val isZipFile = lower.endsWith(".zip") || effectiveFileName.lowercase().endsWith(".zip") ||
                lower.contains(".zip?") || lower.contains(".zip#")
            if (isZipFile) {
                haptic.medium()
                if (com.antigravity.mobile.data.service.FileIconResolver.isWeChatChatArchive(effectiveFileName) ||
                    com.antigravity.mobile.data.service.FileIconResolver.isWeChatChatArchive(clean)
                ) {
                    viewModel.openWeChatArchive(clean, effectiveFileName)
                } else {
                    viewModel.downloadAndPreviewDocument(clean, effectiveFileName, displayTitle)
                }
            } else if (com.antigravity.mobile.data.service.FileIconResolver.isPreviewableDocument(clean) ||
                com.antigravity.mobile.data.service.FileIconResolver.isPreviewableDocument(effectiveFileName)
            ) {
                haptic.medium()
                viewModel.downloadAndPreviewDocument(clean, effectiveFileName, displayTitle)
            } else if (pathLower.endsWith(".md") || pathLower.endsWith(".markdown") ||
                lower.contains("/brain/") || lower.contains("/static/artifacts/") ||
                lower.contains("implementation_plan") || lower.contains("walkthrough")
            ) {
                // 2. Markdown & Plan Artifacts
                val docTitle = when {
                    title.isNotBlank() && !title.endsWith(".md", ignoreCase = true) && !title.endsWith(".markdown", ignoreCase = true) -> title
                    lower.contains("walkthrough") -> "Walkthrough"
                    lower.contains("implementation_plan") -> "Implementation Plan"
                    title.isNotBlank() -> title
                    else -> effectiveFileName
                }
                viewModel.openMarkdownViewer(clean, docTitle)
            } else if ((clean.startsWith("http://") || clean.startsWith("https://")) &&
                GitHubPRLinkExtractor.parseUrl(clean) != null
            ) {
                haptic.light()
                selectedGitHubPRUrl = clean
            } else if (clean.startsWith("http://") || clean.startsWith("https://")) {
                // 3. External Web links (Open in immersive in-app Chrome Custom Tabs)
                haptic.light()
                com.antigravity.mobile.ui.util.InAppBrowserUtils.openUrl(context, clean)
            }
        }
    }

    CompositionLocalProvider(
        com.antigravity.mobile.ui.components.LocalMarkdownBodyScale provides com.antigravity.mobile.data.service.PreferencesManager.bodyScaleFactor(fontSizeScaleSetting),
        com.antigravity.mobile.ui.components.LocalMarkdownCodeScale provides com.antigravity.mobile.data.service.PreferencesManager.codeScaleFactor(fontSizeScaleSetting),
        com.antigravity.mobile.ui.components.LocalCurrentCascadeId provides uiState.cascadeId,
        com.antigravity.mobile.ui.components.LocalKnowledgeModeEnabled provides shouldCollapseTools
    ) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .onGloballyPositioned {
                modelSlidePickerState.containerSize = it.size
                workflowSlidePickerState.containerSize = it.size
                commitSlidePickerState.containerSize = it.size
            }
    ) {
    Scaffold(
        modifier = modifier.fillMaxSize(),
        // Bottom insets are applied inside the input surface so its background reaches the screen
        // bottom (no empty strip above the gesture bar).
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        topBar = {
            TopAppBar(
                modifier = Modifier.clickable(
                    interactionSource = remember { androidx.compose.foundation.interaction.MutableInteractionSource() },
                    indication = null
                ) { focusManager.clearFocus() },
                title = {
                    val branchOrigin = uiState.branchOrigin
                    if (branchOrigin != null && branchOrigin.isSideQuestion) {
                        Column {
                            Text(
                                text = uiState.title,
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 15.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                color = colors.textPrimary
                            )
                            Spacer(modifier = Modifier.height(2.dp))
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(3.dp),
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.accentIndigo.copy(alpha = 0.11f))
                                        .clickable {
                                            if (branchOrigin.parentCascadeId.isNotBlank()) {
                                                haptic.light()
                                                onOpenConversation(branchOrigin.parentCascadeId, branchOrigin.parentTitle.orEmpty())
                                            }
                                        }
                                        .padding(horizontal = 7.dp, vertical = 1.5.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.AutoMirrored.Outlined.LiveHelp,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(11.dp)
                                    )
                                    Text(
                                        text = "旁路快问 · 回主会话".loc,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentIndigo,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }
                        }
                    } else if (turns.size >= 2) {
                        Column(
                            modifier = Modifier.clickable(
                                interactionSource = remember { androidx.compose.foundation.interaction.MutableInteractionSource() },
                                indication = null
                            ) {
                                haptic.light()
                                showNavigatorSheet = true
                            }
                        ) {
                            Text(
                                text = uiState.title,
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 15.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                color = colors.textPrimary
                            )
                            Spacer(modifier = Modifier.height(2.dp))
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(10.dp)
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.accentIndigo.copy(alpha = 0.09f))
                                        .padding(horizontal = 6.dp, vertical = 1.5.dp)
                                ) {
                                    Text(
                                        text = "第 %1\$d/%2\$d 轮".loc(currentTurnIndex, turns.size),
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentIndigo,
                                        maxLines = 1,
                                        softWrap = false
                                    )
                                    Spacer(modifier = Modifier.width(2.dp))
                                    Icon(
                                        imageVector = Icons.Default.KeyboardArrowDown,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(11.dp)
                                    )
                                }
                                if (shouldShowRealtimeContext) {
                                    ContextRingCapsule(
                                        meter = uiState.contextMeter,
                                        isRunning = isSessionRunning,
                                        hasPendingInteraction = hasPendingApproval,
                                        onClick = {
                                            haptic.light()
                                            showContextAuditSheet = true
                                        }
                                    )
                                }
                            }
                        }
                    } else {
                        Column {
                            Text(
                                text = uiState.title,
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 16.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                color = colors.textPrimary
                            )
                            if (shouldShowRealtimeContext && uiState.contextMeter != null) {
                                Spacer(modifier = Modifier.height(2.dp))
                                ContextRingCapsule(
                                    meter = uiState.contextMeter,
                                    isRunning = isSessionRunning,
                                    hasPendingInteraction = hasPendingApproval,
                                    onClick = {
                                        haptic.light()
                                        showContextAuditSheet = true
                                    }
                                )
                            }
                        }
                    }
                },
                navigationIcon = {
                    IconButton(onClick = handleBackNavigation) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back",
                            tint = colors.accentIndigo
                        )
                    }
                },
                actions = {
                    val hasAnyBgTasks = uiState.runningTasks.isNotEmpty() || uiState.globalBackgroundTasks.isNotEmpty()
                    Box {
                        Box(
                            modifier = Modifier
                                .size(40.dp)
                                .clip(CircleShape)
                                .combinedClickable(
                                    onClick = {
                                        haptic.light()
                                        showMcpSkillsSheet = true
                                    },
                                    onLongClick = {
                                        haptic.medium()
                                        showPermissionMenu = true
                                    }
                                ),
                            contentAlignment = Alignment.Center
                        ) {
                            if (hasAnyBgTasks) {
                                Box(
                                    modifier = Modifier
                                        .size(26.dp)
                                        .clip(RoundedCornerShape(6.dp))
                                        .border(1.5.dp, colors.accentBlue, RoundedCornerShape(6.dp)),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Text(
                                        text = ">_",
                                        fontSize = 11.sp,
                                        fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                                        fontWeight = FontWeight.Bold,
                                        color = colors.accentBlue
                                    )
                                }
                            } else {
                                Icon(
                                    imageVector = Icons.Default.Build,
                                    contentDescription = "MCP 工具与技能看板".loc,
                                    tint = colors.accentBlue
                                )
                            }
                        }

                        DropdownMenu(
                            expanded = showPermissionMenu,
                            onDismissRequest = { showPermissionMenu = false },
                            containerColor = colors.surface
                        ) {
                            DropdownMenuItem(
                                text = { Text("旁路快问".loc) },
                                leadingIcon = {
                                    Icon(
                                        imageVector = Icons.AutoMirrored.Outlined.LiveHelp,
                                        contentDescription = "旁路快问".loc,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(18.dp)
                                    )
                                },
                                onClick = {
                                    showPermissionMenu = false
            pendingTurboModeConfirm = false
                                    btwAnchorStepIndex = null
                                    btwAnchorPreview = null
                                    showBtwSheet = true
                                }
                            )
                            HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            if (uiState.isRunning) {
                                DropdownMenuItem(
                                    text = {
                                        Text(
                                            text = "运行中 · 权限切换下轮生效".loc,
                                            fontSize = 11.5.sp,
                                            color = colors.accentOrange,
                                            fontWeight = FontWeight.Medium
                                        )
                                    },
                                    enabled = false,
                                    onClick = {}
                                )
                            }
                            listOf(
                                Triple("strict", "严格审批".loc, "每步高危命令需确认".loc),
                                Triple("default", "默认沙箱".loc, "工作区安全操作自动放行".loc),
                                Triple("auto", "智能自动 · 沙箱".loc, "Antigravity 2.35 自动模式".loc),
                                Triple("turbo", "全自动 · 无沙箱".loc, "命令与编辑全自动放行".loc)
                            ).forEach { (mode, label, _) ->
                                val selected = uiState.permissionState.effectiveMode == mode
                                val selectedTint = when (mode) {
                                    "strict" -> colors.accentIndigo
                                    "default" -> colors.accentBlue
                                    "turbo" -> colors.accentRed
                                    else -> Color(0xFFFF9F0A)
                                }
                                DropdownMenuItem(
                                    text = {
                                        Text(
                                            text = label,
                                            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
                                            color = if (selected && mode != "strict") selectedTint else colors.textPrimary
                                        )
                                    },
                                    trailingIcon = if (selected) {
                                        {
                                            Icon(
                                                imageVector = Icons.Default.Check,
                                                contentDescription = null,
                                                tint = selectedTint,
                                                modifier = Modifier.size(16.dp)
                                            )
                                        }
                                    } else null,
                                    onClick = {
                                        showPermissionMenu = false
            pendingTurboModeConfirm = false
                                        if (mode == "turbo") {
                                            pendingTurboModeConfirm = true
                                        } else {
                                            viewModel.setPermissionPreset(mode) { msg -> phase3Notice = msg }
                                        }
                                    }
                                )
                            }
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = colors.background,
                    titleContentColor = colors.textPrimary
                )
            )
        },
        containerColor = colors.background
    ) { padding ->
        val density = LocalDensity.current
        var chatScaffoldBodyHeight by remember { mutableStateOf<Dp?>(null) }
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
                .onSizeChanged { size ->
                    if (size.height > 0) {
                        val nextHeight = with(density) { size.height.toDp() }
                        if (chatScaffoldBodyHeight != nextHeight) {
                            chatScaffoldBodyHeight = nextHeight
                        }
                    }
                }
        ) {
            val swarm = uiState.swarmBrief
            if (swarm != null && swarm.active) {
                SwarmBanner(
                    brief = swarm,
                    onClick = { showSwarmSheet = true },
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp)
                )
            }

            uiState.trustStatus?.takeIf { !it.isProjectTrusted }?.let { trust ->
                WorkspaceTrustBanner(
                    status = trust,
                    isUpdating = uiState.isUpdatingTrust,
                    onTrust = {
                        viewModel.trustCurrentProject { msg -> phase3Notice = msg }
                    }
                )
            }

            // Content Area (Loading / Error / Empty / Messages)
            BoxWithConstraints(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
            ) {
                val floatingPopupMaxHeight = (maxHeight - 12.dp).coerceIn(240.dp, 500.dp)
                when {
                    uiState.isLoading && uiState.messages.isEmpty() && !uiState.isNewConversation -> {
                        ChatLoadingStateView()
                    }
                    uiState.errorMessage != null && uiState.messages.isEmpty() && !uiState.isNewConversation -> {
                        ChatErrorStateView(
                            errorMessage = uiState.errorMessage ?: "同步会话历史失败".loc,
                            onRetry = { viewModel.retryLoadMessages() }
                        )
                    }
                    uiState.messages.isEmpty() && !shouldShowThinkingBubble -> {
                        ChatEmptyStateView(title = uiState.title)
                    }
                    else -> {
                        PullToRefreshBox(
                            isRefreshing = isRefreshing,
                            onRefresh = {
                                refreshTriggerToken++
                                viewModel.refresh()
                            },
                            modifier = Modifier
                                .fillMaxSize()
                                .clipToBounds(),
                            state = pullRefreshState,
                            indicator = {
                                MgyPullToRefreshIndicator(
                                    state = pullRefreshState,
                                    isRefreshing = isRefreshing,
                                    refreshTriggerToken = refreshTriggerToken,
                                    modifier = Modifier.align(Alignment.TopCenter),
                                    containerColor = colors.surface,
                                    color = colors.accentIndigo
                                )
                            }
                        ) {
                            val lastFollowScrollAtMs = remember { longArrayOf(0L) }
                            val latestUiState by rememberUpdatedState(uiState)
                            val latestViewModel by rememberUpdatedState(viewModel)
                            val latestHandleFileOrLinkClick by rememberUpdatedState(handleFileOrLinkClick)
                            val latestNarrationPlayer by rememberUpdatedState(narrationPlayer)
                            val latestContext by rememberUpdatedState(context)

                            val stableOnPlanClick = remember<((String, String) -> Unit)> {
                                { uri, title -> latestHandleFileOrLinkClick(uri, title) }
                            }
                            val stableUrlResolver = remember<((String) -> String)> {
                                { raw -> latestViewModel.resolveMediaUrl(raw) }
                            }
                            val stableOnImageClick = remember<((String?, android.graphics.Bitmap?) -> Unit)> {
                                { url, bitmap ->
                                    if (!url.isNullOrEmpty() && com.antigravity.mobile.data.service.FileIconResolver.isVideo(url)) {
                                        latestHandleFileOrLinkClick(url, "")
                                    } else {
                                        latestViewModel.openImageViewer(bitmap = bitmap, url = url)
                                    }
                                }
                            }
                            val stableOnImageGroupClick = remember<((List<ImageViewerItem>, Int) -> Unit)> {
                                { items, index -> latestViewModel.openImageViewer(items = items, initialIndex = index) }
                            }
                            val stableOnStopWorkflow = remember<(() -> Unit)> {
                                { latestViewModel.cancelExecution() }
                            }
                            val stableOnSelectWorkflowSubagent = remember<((com.antigravity.mobile.data.model.SubagentItem) -> Unit)> {
                                { sa -> selectedWorkflowSubagent = sa }
                            }
                            val stableOnForkClick = remember<((com.antigravity.mobile.data.model.GatewayMessageItem) -> Unit)> {
                                { target ->
                                    val snapshot = latestUiState
                                    latestViewModel.forkConversationAt(target.stepIndex ?: 0, false) { newId ->
                                        latestViewModel.prepareSession(
                                            cascadeId = newId,
                                            initialTitle = "${snapshot.title} (分支)".loc,
                                            isNewConversation = false,
                                            workspaceName = snapshot.workspaceName
                                        )
                                    }
                                }
                            }
                            val stableOnNarrateClick = remember<((String) -> Unit)> {
                                { text -> latestNarrationPlayer.play(text) }
                            }
                            val stableOnAskBtwClick = remember<((Int?, String) -> Unit)> {
                                { stepIdx, preview ->
                                    btwAnchorStepIndex = stepIdx
                                    btwAnchorPreview = preview
                                    showBtwSheet = true
                                }
                            }
                            val stableOnOpenBranch = remember<((String, String) -> Unit)> {
                                { targetId, targetTitle ->
                                    preserveScrollOnReturn = true
                                    savedScrollIndex = listState.firstVisibleItemIndex
                                    savedScrollOffset = listState.firstVisibleItemScrollOffset
                                    onOpenConversation(targetId, targetTitle)
                                }
                            }
                            val stableOnInsertToParent = remember<((String) -> Unit)> {
                                { text ->
                                    latestViewModel.insertConclusionIntoParent(text) { parentId, parentTitle ->
                                        onOpenConversation(parentId, parentTitle)
                                    }
                                }
                            }
                            val stableOnCompactClick = remember<(() -> Unit)> {
                                { latestViewModel.sendCompactCommand() }
                            }
                            val canRevert = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.REVERT)
                            val stableOnUndoClick = remember<((com.antigravity.mobile.data.model.GatewayMessageItem) -> Unit)?>(canRevert) {
                                if (canRevert) {
                                    { target -> latestViewModel.requestUndo(target) }
                                } else null
                            }
                            val canExport = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.EXPORT)
                            val stableOnExportMarkdownClick = remember<(() -> Unit)?>(canExport, coroutineScope) {
                                if (canExport) {
                                    {
                                        latestViewModel.exportMarkdown { title, markdown ->
                                            coroutineScope.launch {
                                                com.antigravity.mobile.ui.util.MarkdownExportUtils.shareMarkdown(latestContext, title, markdown)
                                            }
                                        }
                                    }
                                } else null
                            }
                            val stableShareContextProvider = remember<((com.antigravity.mobile.data.model.GatewayMessageItem) -> ShareCardContext)> {
                                { pacedTarget ->
                                    val snapshot = latestUiState
                                    val list = snapshot.messages
                                    // 播放中的消息是副本，按 id 还原回原始消息再取上下文
                                    val target = list.firstOrNull { it.id.isNotBlank() && it.id == pacedTarget.id } ?: pacedTarget
                                    val idx = list.indexOfFirst { it === target }.takeIf { it >= 0 } ?: list.size
                                    val question = list.take(idx).lastOrNull { it.isUser }
                                    val badge = ShareCardContext.resolveModel(
                                        target = target,
                                        messages = list,
                                        activeModel = snapshot.activeModel,
                                        activeModelName = snapshot.activeModelName
                                    )
                                    ShareCardContext(
                                        sessionTitle = snapshot.title,
                                        modelName = badge.first,
                                        modelIsClaude = badge.second,
                                        startedAtIso = snapshot.startedAt,
                                        previousMessage = question
                                    )
                                }
                            }

                            val branchOrigin = uiState.branchOrigin
                            val (sharedPrefixMessages, branchOnlyMessages) = remember(uiState.messages, branchOrigin) {
                                if (branchOrigin == null || branchOrigin.forkStepIndex < 0) {
                                    emptyList<com.antigravity.mobile.data.model.GatewayMessageItem>() to uiState.messages
                                } else {
                                    var firstBranchIdx = -1
                                    var lastSharedIdx = -1
                                    for (idx in uiState.messages.indices) {
                                        val si = uiState.messages[idx].effectiveStepIndex ?: continue
                                        if (si > branchOrigin.forkStepIndex) {
                                            if (firstBranchIdx < 0) firstBranchIdx = idx
                                        } else {
                                            lastSharedIdx = idx
                                        }
                                    }
                                    val split = when {
                                        firstBranchIdx >= 0 -> firstBranchIdx
                                        lastSharedIdx >= 0 -> lastSharedIdx + 1
                                        else -> 0
                                    }
                                    uiState.messages.subList(0, split) to uiState.messages.subList(split, uiState.messages.size)
                                }
                            }
                            val sideQuestionsByMessageId = remember(uiState.sideQuestions, uiState.messages) {
                                if (uiState.sideQuestions.isEmpty() || uiState.messages.isEmpty()) emptyMap()
                                else {
                                    val loadedIds = uiState.messages.mapTo(HashSet(uiState.messages.size)) { it.id }
                                    val firstLoadedStep = uiState.messages.mapNotNull { it.effectiveStepIndex }.minOrNull() ?: 0
                                    val map = LinkedHashMap<String, MutableList<com.antigravity.mobile.data.model.SideQuestionCardItem>>()
                                    for (card in uiState.sideQuestions) {
                                        val anchorId = card.anchorMessageId
                                        if (!anchorId.isNullOrBlank() && loadedIds.contains(anchorId)) {
                                            map.getOrPut(anchorId) { mutableListOf() }.add(card)
                                            continue
                                        }
                                        val forkStep = card.forkStepIndex
                                        if (forkStep != null && forkStep >= firstLoadedStep) {
                                            val match = uiState.messages.lastOrNull { m ->
                                                val si = m.effectiveStepIndex
                                                si != null && si <= forkStep && (m.isAgent || m.isUser)
                                            } ?: uiState.messages.lastOrNull { (it.effectiveStepIndex ?: Int.MAX_VALUE) <= forkStep }
                                            if (match != null) {
                                                map.getOrPut(match.id) { mutableListOf() }.add(card)
                                                continue
                                            }
                                        }
                                        if (forkStep == null) {
                                            uiState.messages.lastOrNull()?.id?.let { lastId ->
                                                map.getOrPut(lastId) { mutableListOf() }.add(card)
                                            }
                                        }
                                    }
                                    map
                                }
                            }

                            LazyColumn(
                                state = listState,
                                modifier = Modifier
                                    .fillMaxSize()
                                    .alpha(listJumpAlpha),
                                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                                verticalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                if (branchOrigin != null) {
                                    item(key = "branch_origin_banner") {
                                        BranchOriginBanner(
                                            origin = branchOrigin,
                                            isExpanded = showSharedBranchContext,
                                            onToggleExpand = { showSharedBranchContext = !showSharedBranchContext },
                                            onReturnToParent = {
                                                if (branchOrigin.parentCascadeId.isNotBlank()) {
                                                    onOpenConversation(branchOrigin.parentCascadeId, branchOrigin.parentTitle.orEmpty())
                                                }
                                            }
                                        )
                                    }
                                }

                                val showOlderBtn = uiState.hasMore && (branchOrigin == null || showSharedBranchContext)
                                if (showOlderBtn) {
                                    item(key = "load_older_messages") {
                                        Box(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .padding(vertical = 4.dp),
                                            contentAlignment = Alignment.Center
                                        ) {
                                            if (uiState.isLoadingOlder) {
                                                CircularProgressIndicator(
                                                    modifier = Modifier.size(20.dp),
                                                    strokeWidth = 2.dp,
                                                    color = MaterialTheme.colorScheme.primary
                                                )
                                            } else {
                                                Surface(
                                                    onClick = { viewModel.loadOlderMessages() },
                                                    shape = RoundedCornerShape(16.dp),
                                                    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.7f)
                                                ) {
                                                    Row(
                                                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 7.dp),
                                                        verticalAlignment = Alignment.CenterVertically,
                                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                                    ) {
                                                        Icon(
                                                            imageVector = Icons.Default.ArrowUpward,
                                                            contentDescription = "查看更早的消息".loc,
                                                            modifier = Modifier.size(14.dp),
                                                            tint = MaterialTheme.colorScheme.onSurfaceVariant
                                                        )
                                                        Text(
                                                            text = "查看更早的消息".loc,
                                                            style = MaterialTheme.typography.labelMedium,
                                                            color = MaterialTheme.colorScheme.onSurfaceVariant
                                                        )
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }

                                if (branchOrigin != null && showSharedBranchContext && sharedPrefixMessages.isNotEmpty()) {
                                    items(
                                        sharedPrefixMessages,
                                        key = { "shared_${if (it.id.isNotBlank()) it.id else "${it.timestamp}_${it.role}_${it.stepIndex ?: 0}"}" }
                                    ) { sharedMsg ->
                                        Box(modifier = Modifier.fillMaxWidth().alpha(0.82f)) {
                                            MessageBubble(
                                                message = sharedMsg,
                                                isActiveToolBatch = false,
                                                isActivePlan = false,
                                                isScheduledTaskPrompt = uiState.isScheduledTaskPromptMessage(sharedMsg),
                                                onPlanClick = stableOnPlanClick,
                                                urlResolver = stableUrlResolver,
                                                onImageClick = stableOnImageClick,
                                                onImageGroupClick = stableOnImageGroupClick,
                                                onSubagentClick = openSubagent,
                                                onStopWorkflow = stableOnStopWorkflow,
                                                onSelectWorkflowSubagent = stableOnSelectWorkflowSubagent,
                                                onForkClick = stableOnForkClick,
                                                onNarrateClick = stableOnNarrateClick,
                                                onAskBtwClick = stableOnAskBtwClick,
                                                onCompactClick = stableOnCompactClick,
                                                onStartNewSessionClick = onStartNewSession,
                                                onUndoClick = null,
                                                onExportMarkdownClick = stableOnExportMarkdownClick,
                                                shareContextProvider = stableShareContextProvider
                                            )
                                        }
                                    }
                                    item(key = "branch_fork_divider") {
                                        BranchForkDivider(onCollapse = { showSharedBranchContext = false })
                                    }
                                }

                                val subagentGroupByHeadId = run {
                                    val byHead = HashMap<String, List<com.antigravity.mobile.data.model.SubagentItem>>()
                                    val absorbed = HashSet<String>()
                                    var idx = 0
                                    while (idx < branchOnlyMessages.size) {
                                        val m = branchOnlyMessages[idx]
                                        val firstSub = m.subagent?.takeIf { m.isSubagent }
                                        if (firstSub != null) {
                                            val runList = mutableListOf(firstSub)
                                            var j = idx + 1
                                            while (j < branchOnlyMessages.size) {
                                                val nextSub = branchOnlyMessages[j].subagent?.takeIf { branchOnlyMessages[j].isSubagent }
                                                if (nextSub == null) break
                                                runList.add(nextSub)
                                                j++
                                            }
                                            if (runList.size >= 2) {
                                                byHead[m.id] = runList
                                                for (k in (idx + 1) until j) {
                                                    absorbed.add(branchOnlyMessages[k].id)
                                                }
                                            }
                                            idx = j
                                        } else {
                                            idx++
                                        }
                                    }
                                    byHead to absorbed
                                }
                                val displayBranchMessages = if (subagentGroupByHeadId.second.isEmpty()) {
                                    branchOnlyMessages
                                } else {
                                    branchOnlyMessages.filterNot { subagentGroupByHeadId.second.contains(it.id) }
                                }

                                items(
                                    displayBranchMessages,
                                    key = { if (it.id.isNotBlank()) it.id else "${it.timestamp}_${it.role}_${it.stepIndex ?: 0}" }
                                ) { rawMsg ->
                                    val groupedSubagents = subagentGroupByHeadId.first[rawMsg.id]
                                    if (groupedSubagents != null) {
                                        SubagentGroupInlineCard(
                                            items = groupedSubagents,
                                            onClick = openSubagent
                                        )
                                        return@items
                                    }
                                    val isLastAgentStreaming = uiState.isRunning && rawMsg.isAgent && rawMsg.id == uiState.messages.lastOrNull()?.id
                                    val msg = com.antigravity.mobile.ui.util.rememberPacedMessage(
                                        message = rawMsg,
                                        onGrow = if (isLastAgentStreaming) {
                                            {
                                                if (hasInitiallyAligned && !preserveScrollOnReturn && !listState.isScrollInProgress && (isNearBottom || !hasUserInteracted)) {
                                                    val now = android.os.SystemClock.uptimeMillis()
                                                    if (now - lastFollowScrollAtMs[0] >= 60L) {
                                                        lastFollowScrollAtMs[0] = now
                                                        coroutineScope.launch {
                                                            val total = listState.layoutInfo.totalItemsCount
                                                            if (total > 0) {
                                                                isProgrammaticScrolling = true
                                                                try {
                                                                    listState.scrollToItem(total - 1)
                                                                } finally {
                                                                    delay(30)
                                                                    isProgrammaticScrolling = false
                                                                }
                                                            }
                                                        }
                                                    }
                                                }
                                            }
                                        } else null
                                    )
                                    val isHighlighted = highlightedTurnMessageId != null && rawMsg.id == highlightedTurnMessageId
                                    val highlightBorderColor by androidx.compose.animation.animateColorAsState(
                                        targetValue = if (isHighlighted) colors.accentIndigo else Color.Transparent,
                                        animationSpec = androidx.compose.animation.core.tween(durationMillis = 280),
                                        label = "turnHighlightBorder"
                                    )
                                    val attachedSides = sideQuestionsByMessageId[rawMsg.id].orEmpty()
                                    val allowInsertParent = branchOrigin != null && branchOrigin.parentCascadeId.isNotBlank() && rawMsg.isAgent
                                    Column(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .then(
                                                if (highlightBorderColor.alpha > 0.01f) {
                                                    Modifier.border(
                                                        width = 1.8.dp,
                                                        color = highlightBorderColor,
                                                        shape = RoundedCornerShape(18.dp)
                                                    )
                                                } else {
                                                    Modifier
                                                }
                                            )
                                    ) {
                                        MessageBubble(
                                            message = msg,
                                            isActiveToolBatch = isStreamingOrRunning && rawMsg.id == lastMsg?.id,
                                            isActivePlan = rawMsg.id == activePlanId,
                                            isStreaming = isLastAgentStreaming,
                                            isScheduledTaskPrompt = uiState.isScheduledTaskPromptMessage(rawMsg),
                                            onPlanClick = stableOnPlanClick,
                                            urlResolver = stableUrlResolver,
                                            onImageClick = stableOnImageClick,
                                            onImageGroupClick = stableOnImageGroupClick,
                                            onSubagentClick = openSubagent,
                                            onStopWorkflow = stableOnStopWorkflow,
                                            onSelectWorkflowSubagent = stableOnSelectWorkflowSubagent,
                                            onForkClick = stableOnForkClick,
                                            onNarrateClick = stableOnNarrateClick,
                                            onAskBtwClick = stableOnAskBtwClick,
                                            onInsertToParentClick = if (allowInsertParent) stableOnInsertToParent else null,
                                            onCompactClick = stableOnCompactClick,
                                            onStartNewSessionClick = onStartNewSession,
                                            onUndoClick = stableOnUndoClick,
                                            onExportMarkdownClick = stableOnExportMarkdownClick,
                                            shareContextProvider = stableShareContextProvider
                                        )
                                        if (attachedSides.isNotEmpty()) {
                                            SideQuestionInlineCardsSection(
                                                items = attachedSides,
                                                onOpen = stableOnOpenBranch
                                            )
                                        }
                                    }
                                }

                                // Active Thinking Animation Card
                                if (shouldShowThinkingBubble) {
                                    item(key = "agent_thinking_bubble") {
                                        AgentThinkingBubble(
                                            focusText = uiState.activeFocus,
                                            startedAt = uiState.activeFocusStartedAt
                                        )
                                    }
                                }

                                // Bottom breathing room spacer ensuring bubble is fully clear of input bar
                                item(key = "chat_bottom_spacer") {
                                    Spacer(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .height(10.dp)
                                            .clickable(
                                                interactionSource = remember { androidx.compose.foundation.interaction.MutableInteractionSource() },
                                                indication = null
                                            ) { focusManager.clearFocus() }
                                    )
                                }
                            }

                        }
                    }
                }

                // 斜杠命令悬浮气泡：完全浮在输入栏横线上方的会话区域底部
                val slashQuery = com.antigravity.mobile.data.model.SlashCommandFilter.queryOf(inputText)
                    .takeIf { compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.SLASH) && !isSlashPickerDismissed }
                LaunchedEffect(slashQuery != null) {
                    if (slashQuery != null) viewModel.ensureSlashCommands()
                }
                if (slashQuery != null && !uiState.showMentionPicker) {
                    SlashCommandPicker(
                        commands = com.antigravity.mobile.data.model.SlashCommandFilter.filter(uiState.slashCommands, slashQuery),
                        selectedNames = uiState.selectedSlashCommands.map { it.name }.toSet(),
                        isLoading = uiState.isLoadingSlashCommands,
                        onSelect = {
                            isSlashPickerDismissed = false
                            if (it.name.equals("workflow", ignoreCase = true)) {
                                viewModel.stripTrailingSlashQuery()
                                showWorkflowTemplateSheet = true
                            } else {
                                viewModel.selectSlashCommand(it)
                            }
                        },
                        onDismiss = {
                            isSlashPickerDismissed = true
                        },
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .padding(top = 8.dp, bottom = 8.dp)
                    )
                }

                // 定位未加载早期轮次时的轻量过渡胶囊提示
                jumpingToTurnIndex?.let { jumpIdx ->
                    Surface(
                        shape = RoundedCornerShape(999.dp),
                        color = colors.surface.copy(alpha = 0.94f),
                        tonalElevation = 4.dp,
                        shadowElevation = 6.dp,
                        modifier = Modifier.align(Alignment.Center)
                    ) {
                        Row(
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(15.dp),
                                strokeWidth = 2.dp,
                                color = colors.accentIndigo
                            )
                            Text(
                                text = "第 $jumpIdx 轮".loc,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary
                            )
                        }
                    }
                }

                // 浮动跳跃双钮导航条 (Prompt Jump Stepper)
                if (animatedStepperAlpha > 0.01f) {
                    com.antigravity.mobile.ui.components.PromptJumpStepper(
                        currentTurn = currentTurnIndex,
                        totalTurns = turns.size,
                        isAtTop = currentTurnIndex <= 1,
                        isAtLastTurn = currentTurnIndex >= turns.size,
                        onJumpPrevious = {
                            stepperAlphaTarget = 1.0f
                            val targetTurn = (currentTurnIndex - 1).coerceAtLeast(1)
                            val matchTurn = turns.firstOrNull { it.turnIndex == targetTurn }
                            if (matchTurn != null) {
                                jumpToTurn(matchTurn)
                            }
                        },
                        onJumpNextOrBottom = {
                            stepperAlphaTarget = 1.0f
                            if (currentTurnIndex < turns.size) {
                                val targetTurn = currentTurnIndex + 1
                                val matchTurn = turns.firstOrNull { it.turnIndex == targetTurn }
                                if (matchTurn != null) {
                                    jumpToTurn(matchTurn)
                                }
                            } else {
                                activeTurnJumpJob?.cancel()
                                lockedTurnIndex = null
                                coroutineScope.launch {
                                    val total = listState.layoutInfo.totalItemsCount
                                    if (total > 0) {
                                        isProgrammaticScrolling = true
                                        try {
                                            listState.animateScrollToItem(total - 1)
                                        } finally {
                                            isProgrammaticScrolling = false
                                        }
                                    }
                                }
                            }
                        },
                        onForceJumpBottom = {
                            stepperAlphaTarget = 1.0f
                            activeTurnJumpJob?.cancel()
                            lockedTurnIndex = null
                            coroutineScope.launch {
                                val total = listState.layoutInfo.totalItemsCount
                                if (total > 0) {
                                    isProgrammaticScrolling = true
                                    try {
                                        listState.animateScrollToItem(total - 1)
                                    } finally {
                                        isProgrammaticScrolling = false
                                    }
                                }
                            }
                        },
                        modifier = Modifier
                            .align(Alignment.BottomEnd)
                            .padding(end = 14.dp, bottom = 12.dp)
                            .alpha(animatedStepperAlpha)
                    )
                }

                // @ 上下文引用悬浮弹窗：悬浮在会话区域底部，避免挤占输入栏高度
                if (uiState.showMentionPicker) {
                    ContextMentionPicker(
                        candidates = uiState.mentionCandidates,
                        selectedItems = uiState.selectedContextItems,
                        query = uiState.mentionQuery,
                        isLoading = uiState.isLoadingMentions,
                        maxHeight = floatingPopupMaxHeight,
                        onQueryChange = { viewModel.searchContextMentions(it) },
                        onSelect = { item ->
                            val stripped = inputText.replace(Regex("""(?:^|\s)@[^\s@]*$"""), "").trimEnd()
                            if (stripped != inputText) viewModel.onInputTextChanged(stripped)
                            viewModel.addContextMention(item)
                        },
                        onDismiss = { viewModel.closeMentionPicker() },
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .padding(top = 8.dp, bottom = 8.dp)
                    )
                }
            }

            // Floating Cards (RunningTasksCard, QueuedMessagesCard, InteractionCard, AudioRecordingCard) 对齐 iOS floatingCards
            // 底部只放仍在运行的子代理（方便随时关停）；已结束的留在消息流里的内联卡片中，不占底部位置
            val runningSubagents = uiState.subagents.filter { it.isRunning }
            val isActivelyRunningForCards = uiState.isRunning || uiState.isAwaitingResponse || uiState.runningTasks.hasBlockingTasks
            val hasFloatingCards = uiState.runningTasks.isNotEmpty() ||
                    runningSubagents.isNotEmpty() ||
                    uiState.queuedMessages.isNotEmpty() ||
                    uiState.pendingInteraction != null ||
                    isRecordingAudio ||
                    isInVoiceCall ||
                    showVoiceTroubleshootBubble

            if (hasFloatingCards) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 12.dp)
                        .padding(bottom = 6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (showVoiceTroubleshootBubble && !isInVoiceCall) {
                        VoiceTroubleshootBubble(
                            onRetry = { showVoiceTroubleshootBubble = false },
                            onTroubleshoot = {
                                showVoiceTroubleshootBubble = false
                                viewModel.sendPresetMessage(VoiceCallManager.TROUBLESHOOT_PROMPT)
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    if (isInVoiceCall) {
                        VoiceCallCard(
                            voiceCall = voiceCall,
                            isAgentRunning = isActivelyRunningForCards,
                            onStopAgentTask = {
                                dismissKeyboard()
                                viewModel.cancelExecution()
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    if (isRecordingAudio) {
                        AudioRecordingCard(
                            recorder = audioRecorder,
                            onCancel = {
                                audioRecorder.cancelRecording()
                            },
                            onDone = {
                                val uri = audioRecorder.stopRecording()
                                if (uri != null) {
                                    viewModel.addAttachmentsFromUris(context, listOf(uri))
                                }
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    if (uiState.runningTasks.isNotEmpty()) {
                        RunningTasksCard(
                            tasks = uiState.runningTasks,
                            onStopTask = { viewModel.stopTask(it) },
                            onSendToBackground = { task ->
                                viewModel.sendTaskToBackground(task.stepIndex) { msg -> phase3Notice = msg }
                            },
                            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
                            compactMode = shouldCollapseTasks,
                            onToggleExpand = { isExpanded ->
                                handleFloatingCardToggle(isExpanded)
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    if (runningSubagents.isNotEmpty()) {
                        SubagentsCard(
                            items = runningSubagents,
                            canStop = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.SUBAGENTS),
                            onOpen = openSubagent,
                            onStop = { viewModel.stopSubagent(it.conversationId) },
                            onToggleExpand = { isExpanded ->
                                handleFloatingCardToggle(isExpanded)
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    if (uiState.queuedMessages.isNotEmpty()) {
                        val nonDaemonTasks = uiState.runningTasks.filter { !it.isDaemon }
                        val effectiveTasks = nonDaemonTasks.ifEmpty { uiState.runningTasks }
                        val hasActiveBackgroundWork = isActivelyRunningForCards ||
                                effectiveTasks.isNotEmpty() ||
                                runningSubagents.isNotEmpty()
                        val blockingWorkNames = remember(runningSubagents, effectiveTasks) {
                            buildList {
                                runningSubagents.forEach { sa ->
                                    val label = (sa.role?.takeIf { it.isNotBlank() } ?: sa.typeName).orEmpty().trim()
                                    if (label.isNotEmpty()) add(label)
                                }
                                effectiveTasks.forEach { task ->
                                    val raw = task.toolSummary?.takeIf { it.isNotBlank() }
                                        ?: task.commandLine.takeIf { it.isNotBlank() }
                                        ?: task.toolName.orEmpty()
                                    val clean = raw.trim()
                                    if (clean.isNotEmpty()) {
                                        add(if (clean.length > 18) clean.take(18) + "…" else clean)
                                    }
                                }
                            }
                        }
                        QueuedMessagesCard(
                            items = uiState.queuedMessages,
                            hasActiveBackgroundWork = hasActiveBackgroundWork,
                            blockingWorkNames = blockingWorkNames,
                            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
                            onSendNow = { item ->
                                if (hasActiveBackgroundWork) {
                                    dismissKeyboard()
                                    queuedItemToSendNow = item
                                    showSendQueuedNowConfirmDialog = true
                                } else {
                                    viewModel.sendQueuedMessageNow(item)
                                }
                            },
                            onEdit = { viewModel.editQueuedMessage(it) },
                            onDelete = { viewModel.deleteQueuedMessage(it) },
                            onToggleExpand = { isExpanded ->
                                handleFloatingCardToggle(isExpanded)
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    uiState.pendingInteraction?.let { interaction ->
                        val bannerReserve = if (uiState.swarmBrief?.active == true) 52.dp else 0.dp
                        val interactionMaxAvailableHeight = chatScaffoldBodyHeight?.let { bodyH ->
                            (bodyH - 124.dp - bannerReserve).coerceAtLeast(220.dp)
                        }
                        InteractionCard(
                            interaction = interaction,
                            onSubmit = { optId, writeIn, qResponses ->
                                dismissKeyboard()
                                viewModel.submitInteraction(
                                    optionId = optId,
                                    writeInText = writeIn,
                                    questionResponses = qResponses
                                )
                            },
                            onSkip = { qResponses ->
                                dismissKeyboard()
                                viewModel.skipInteraction(questionResponses = qResponses)
                            },
                            onToggleExpand = { isExpanded ->
                                handleFloatingCardToggle(isExpanded)
                            },
                            maxAvailableHeight = interactionMaxAvailableHeight,
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }

            // Bottom Control Area: Divider + Chips + Attached Images + Input Bar
            val isSubagentSession = !uiState.parentConversationId.isNullOrBlank()
            if (isSubagentSession) {
                SubagentReadOnlyBar(
                    role = uiState.subagentRole,
                    isRunning = uiState.isRunning,
                    canStop = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.SUBAGENTS),
                    onStop = { viewModel.stopSubagent(cascadeId) }
                )
            } else {
            Surface(
                color = colors.surface,
                shadowElevation = 4.dp,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .navigationBarsPadding()
                ) {
                    HorizontalDivider(
                        color = colors.separator.copy(alpha = 0.5f),
                        thickness = 0.5.dp
                    )

                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 4.dp, bottom = 12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        // Suggested Reply Bubbles (dedicated full-width wrapping row above Quick Action Chips) + Quick Action Chips
                        val aiAssistStatus by com.antigravity.mobile.data.service.AIAssistCache.statusFlow.collectAsState()
                        LaunchedEffect(uiState.isPredictedSuggestion) {
                            if (uiState.isPredictedSuggestion) {
                                com.antigravity.mobile.data.service.AIAssistCache.ensureFetched(com.antigravity.mobile.data.service.ApiClient.defaultInstance)
                            }
                        }
                        val isActivelyRunningForSuggestions = uiState.isRunning || uiState.isAwaitingResponse || uiState.runningTasks.hasBlockingTasks
                        val visibleSuggestions = if (
                            uiState.suggestedResponses.isNotEmpty() &&
                            inputText.isBlank() &&
                            !isActivelyRunningForSuggestions &&
                            !uiState.showMentionPicker
                        ) {
                            uiState.suggestedResponses.filter { s ->
                                s.isNotBlank() && s != uiState.dismissedPredictedSuggestion
                            }
                        } else {
                            emptyList()
                        }
                        Column(modifier = Modifier.fillMaxWidth()) {
                            SuggestedReplyChipsBar(
                                suggestedResponses = visibleSuggestions,
                                isPredictedSuggestion = uiState.isPredictedSuggestion,
                                onSelectSuggestion = { chosen ->
                                    if (aiAssistStatus.prefs.nextPromptTapAction == "send") {
                                        dismissKeyboard()
                                        viewModel.sendPresetMessage(chosen)
                                    } else {
                                        viewModel.dismissPredictedSuggestion(chosen)
                                        viewModel.onInputTextChanged(chosen)
                                        focusRequester.requestFocus()
                                    }
                                },
                                onDismissSuggestion = { chosen ->
                                    viewModel.dismissPredictedSuggestion(chosen)
                                }
                            )
                            Box(modifier = Modifier.padding(horizontal = 16.dp)) {
                                QuickActionChips(
                                    activeModel = uiState.activeModel,
                                    onToggleModel = {
                                        workflowSlidePickerState.dismiss()
                                        commitSlidePickerState.dismiss()
                                        viewModel.toggleModel()
                                    },
                                    onSelectModel = { viewModel.selectModel(it) },
                                    modelCatalog = modelCatalog,
                                    modelSlidePickerState = modelSlidePickerState,
                                    onAddImage = {
                                        dismissKeyboard()
                                        showAttachmentSheet = true
                                    },
                                    onAddImageLongClick = {
                                        dismissKeyboard()
                                        viewModel.openMentionPicker("")
                                    },
                                    selectedSlashCommands = uiState.selectedSlashCommands,
                                    onRemoveSlashCommand = { viewModel.removeSlashCommand(it) },
                                    onCommitAndPush = {
                                        dismissKeyboard()
                                        viewModel.openGitSheet()
                                    },
                                    commitLabel = uiState.gitChipLabel,
                                    showCommit = uiState.hasGitChanges,
                                    canCommit = uiState.hasUncommittedFiles,
                                    commitSlidePickerState = commitSlidePickerState,
                                    onSelectCommitAction = { cmd ->
                                        viewModel.onInputTextChanged(cmd)
                                    },
                                    onShowChanges = {
                                        dismissKeyboard()
                                        viewModel.openChangesSheet()
                                    },
                                    onShowWorkflowTemplates = {
                                        dismissKeyboard()
                                        modelSlidePickerState.dismiss()
                                        workflowSlidePickerState.dismiss()
                                        commitSlidePickerState.dismiss()
                                        workflowSheetStartEditing = false
                                        showWorkflowTemplateSheet = true
                                    },
                                    workflowSlidePickerState = workflowSlidePickerState,
                                    workflowTemplates = workflowTemplates,
                                    onSelectWorkflowPrompt = { prompt ->
                                        viewModel.onInputTextChanged(prompt)
                                        pendingSelectFirstPlaceholder = true
                                    },
                                    onOpenWorkflowSetup = {
                                        dismissKeyboard()
                                        modelSlidePickerState.dismiss()
                                        workflowSlidePickerState.dismiss()
                                        commitSlidePickerState.dismiss()
                                        workflowSheetStartEditing = true
                                        showWorkflowTemplateSheet = true
                                    },
                                    showChanges = uiState.hasSessionChanges,
                                    showContinue = uiState.isLatestMessageError,
                                    onContinue = {
                                        dismissKeyboard()
                                        viewModel.handleContinue()
                                    },
                                    showProceed = uiState.canProceed,
                                    onProceed = {
                                        dismissKeyboard()
                                        viewModel.proceedArtifact()
                                    }
                                )
                            }
                        }

                        // Phase 3: Transient notice pill
                        phase3Notice?.let { noticeText ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp)
                                    .clip(RoundedCornerShape(10.dp))
                                    .background(colors.accentIndigo.copy(alpha = 0.12f))
                                    .padding(horizontal = 12.dp, vertical = 6.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = noticeText,
                                    fontSize = 12.5.sp,
                                    color = colors.accentIndigo,
                                    modifier = Modifier.weight(1f)
                                )
                            }
                        }

                        // Phase 3: Voice Narration playback bar
                        if (isNarrating) {
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp)
                                    .clip(RoundedCornerShape(10.dp))
                                    .background(colors.surfaceVariant)
                                    .padding(horizontal = 12.dp, vertical = 6.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.GraphicEq,
                                    contentDescription = null,
                                    tint = colors.accentIndigo,
                                    modifier = Modifier.size(16.dp)
                                )
                                Text(
                                    text = narrationLabel.ifBlank { "语音播报中...".loc },
                                    fontSize = 12.5.sp,
                                    color = colors.textPrimary,
                                    modifier = Modifier.weight(1f),
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                TextButton(
                                    onClick = { narrationPlayer.stop() },
                                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                                ) {
                                    Text("停止".loc, fontSize = 12.sp, color = colors.accentRed)
                                }
                            }
                        }

                        // Phase 3: Selected @ Context Mention chips
                        if (uiState.selectedContextItems.isNotEmpty()) {
                            LazyRow(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp, vertical = 2.dp),
                                horizontalArrangement = Arrangement.spacedBy(6.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                items(uiState.selectedContextItems, key = { it.id }) { item ->
                                    Row(
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(12.dp))
                                            .background(colors.accentIndigo.copy(alpha = 0.14f))
                                            .padding(horizontal = 8.dp, vertical = 4.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                                    ) {
                                        Text(
                                            text = "@${item.label}",
                                            fontSize = 12.sp,
                                            color = colors.accentIndigo,
                                            maxLines = 1
                                        )
                                        Icon(
                                            imageVector = Icons.Default.Close,
                                            contentDescription = "移除引用".loc,
                                            tint = colors.accentIndigo,
                                            modifier = Modifier
                                                .size(13.dp)
                                                .clickable { viewModel.removeContextMention(item.id) }
                                        )
                                    }
                                }
                            }
                        }

                        // Attached Image Previews Strip (displayed directly above the input box)
                        if (uiState.selectedImages.isNotEmpty()) {
                            LazyRow(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp, vertical = 2.dp),
                                horizontalArrangement = Arrangement.spacedBy(10.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                itemsIndexed(uiState.selectedImages, key = { _, img -> img.id }) { index, img ->
                                    Box(
                                        modifier = Modifier.padding(top = 4.dp, end = 6.dp)
                                    ) {
                                        if (img.bitmap != null) {
                                            Image(
                                                bitmap = img.bitmap.asImageBitmap(),
                                                contentDescription = "Attachment preview",
                                                contentScale = ContentScale.Crop,
                                                modifier = Modifier
                                                    .size(52.dp)
                                                    .clip(RoundedCornerShape(10.dp))
                                                    .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                                                    .clickable { viewModel.openAttachmentImageViewer(img) }
                                            )
                                        } else {
                                            SubcomposeAsyncImage(
                                                model = ImageRequest.Builder(context)
                                                    .data(img.byteArray)
                                                    .crossfade(true)
                                                    .build(),
                                                contentDescription = "Attachment preview",
                                                contentScale = ContentScale.Crop,
                                                modifier = Modifier
                                                    .size(52.dp)
                                                    .clip(RoundedCornerShape(10.dp))
                                                    .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                                                    .clickable { viewModel.openAttachmentImageViewer(img) }
                                            )
                                        }

                                        Box(
                                            modifier = Modifier
                                                .size(20.dp)
                                                .align(Alignment.TopEnd)
                                                .offset(x = 6.dp, y = (-6).dp)
                                                .clip(CircleShape)
                                                .background(Color.Black.copy(alpha = 0.65f))
                                                .clickable { viewModel.removeImage(index) },
                                            contentAlignment = Alignment.Center
                                        ) {
                                            Icon(
                                                imageVector = Icons.Default.Close,
                                                contentDescription = "Remove image",
                                                tint = Color.White,
                                                modifier = Modifier.size(12.dp)
                                            )
                                        }
                                    }
                                }
                            }
                        }

                        // Attached file chips (documents / archives / source files)
                        if (uiState.selectedFiles.isNotEmpty()) {
                            LazyRow(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp, vertical = 2.dp),
                                horizontalArrangement = Arrangement.spacedBy(6.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                items(uiState.selectedFiles, key = { it.id }) { file ->
                                    AttachmentFileChip(
                                        file = file,
                                        onRemove = { viewModel.removeFile(file.id) },
                                        onRetry = { viewModel.retryFileUpload(file.id) }
                                    )
                                }
                            }
                        }

                        // Input Field & iOS Circular Action Button (1:1 iOS Alignment & Style)
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 16.dp),
                            verticalAlignment = Alignment.Bottom,
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            BasicTextField(
                                value = inputFieldValue,
                                onValueChange = { newValue ->
                                    val expandedValue = if (newValue.text == inputFieldValue.text && newValue.selection.collapsed) {
                                        val pos = newValue.selection.start
                                        val matched = BracketPlaceholderRegex.findAll(newValue.text).firstOrNull { match ->
                                            pos in match.range.first..(match.range.last + 1)
                                        }
                                        if (matched != null) {
                                            newValue.copy(selection = TextRange(matched.range.first, matched.range.last + 1))
                                        } else {
                                            newValue
                                        }
                                    } else {
                                        newValue
                                    }
                                    inputFieldValue = expandedValue
                                    if (expandedValue.text != inputText) {
                                        viewModel.onInputTextChanged(expandedValue.text)
                                        val m = Regex("""(?:^|\s)@([^\s@]*)$""").find(expandedValue.text)
                                        if (m != null) {
                                            viewModel.openMentionPicker(m.groupValues[1])
                                        } else if (uiState.showMentionPicker) {
                                            viewModel.closeMentionPicker()
                                        }
                                    }
                                },
                                modifier = Modifier
                                    .weight(1f)
                                    .focusRequester(focusRequester)
                                    .onFocusChanged { focusState ->
                                        if (focusState.isFocused != isInputFocused) {
                                            isInputFocused = focusState.isFocused
                                        }
                                    }
                                    .heightIn(min = 44.dp),
                                textStyle = TextStyle(
                                    color = colors.textPrimary,
                                    fontSize = 16.sp,
                                    lineHeight = 22.sp
                                ),
                                cursorBrush = SolidColor(colors.accentIndigo),
                                maxLines = 5,
                                keyboardActions = KeyboardActions(
                                    onSend = {
                                        if (inputText.isNotBlank() || uiState.selectedImages.isNotEmpty() || uiState.selectedFiles.isNotEmpty() || uiState.selectedSlashCommands.isNotEmpty()) {
                                            dismissKeyboard()
                                            viewModel.sendCurrentMessage()
                                        }
                                    }
                                ),
                                decorationBox = { innerTextField ->
                                    Box(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .heightIn(min = 44.dp)
                                            .clip(RoundedCornerShape(22.dp))
                                            .background(colors.surfaceVariant)
                                            .padding(horizontal = 16.dp, vertical = 8.dp),
                                        contentAlignment = Alignment.CenterStart
                                    ) {
                                        val isActivelyRunning = uiState.isRunning || uiState.isAwaitingResponse || uiState.runningTasks.hasBlockingTasks
                                        if (inputText.isEmpty()) {
                                            val placeholderText = when {
                                                uiState.selectedSlashCommands.any { it.name.equals("btw", ignoreCase = true) } -> "输入旁路快问（不打断主对话）...".loc
                                                isActivelyRunning -> "向队列添加指令...".loc
                                                else -> "发送对 Agent 的指令...".loc
                                            }
                                            Text(
                                                text = placeholderText,
                                                color = colors.textMuted,
                                                fontSize = 16.sp,
                                                lineHeight = 22.sp,
                                                maxLines = 1,
                                                overflow = TextOverflow.Ellipsis
                                            )
                                        }
                                        innerTextField()
                                    }
                                }
                            )

                            val isActivelyRunning = uiState.isRunning || uiState.isAwaitingResponse || uiState.runningTasks.hasBlockingTasks
                            val isInputBlank = inputText.isBlank()
                            val hasAttachments = uiState.selectedImages.isNotEmpty() || uiState.selectedFiles.isNotEmpty() || uiState.selectedSlashCommands.isNotEmpty() || uiState.selectedContextItems.isNotEmpty()
                            val hasSendableDraft = !isInputBlank || hasAttachments
                            val hasUnuploadedFiles = uiState.selectedFiles.any { !it.isUploaded }

                            when {
                                isInVoiceCall && !hasSendableDraft -> {
                                    // Hang-up button during active voice call when draft is empty
                                    Box(
                                        modifier = Modifier
                                            .size(44.dp)
                                            .shadow(
                                                elevation = 3.dp,
                                                shape = CircleShape,
                                                ambientColor = Color.Black.copy(alpha = 0.08f),
                                                spotColor = colors.accentRed.copy(alpha = 0.35f)
                                            )
                                            .clip(CircleShape)
                                            .background(colors.accentRed)
                                            .clickable {
                                                haptic.medium()
                                                voiceCall.endCall()
                                            },
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Icon(
                                            imageVector = Icons.Default.CallEnd,
                                            contentDescription = "挂断实时语音通话".loc,
                                            tint = Color.White,
                                            modifier = Modifier.size(19.dp)
                                        )
                                    }
                                }
                                hasSendableDraft || (!isActivelyRunning && !enableVoiceMode) -> {
                                    // Send button: 44.dp circle, Apple Indigo with white up arrow
                                    val isEnabled = hasSendableDraft && !hasUnuploadedFiles
                                    Box(
                                        modifier = Modifier
                                            .size(44.dp)
                                            .shadow(
                                                elevation = if (isEnabled) 3.dp else 0.dp,
                                                shape = CircleShape,
                                                ambientColor = Color.Black.copy(alpha = 0.05f),
                                                spotColor = colors.accentIndigo.copy(alpha = 0.35f)
                                            )
                                            .clip(CircleShape)
                                            .background(if (isEnabled) colors.accentIndigo else colors.surfaceVariant)
                                            .then(
                                                if (!isEnabled) Modifier.border(0.8.dp, colors.border, CircleShape)
                                                else Modifier
                                            )
                                            .clickable(enabled = isEnabled) {
                                                dismissKeyboard()
                                                val trimmedSend = inputText.trim()
                                                val slashCmds = uiState.selectedSlashCommands
                                                val isSlashBtw = slashCmds.size == 1 && slashCmds[0].name.equals("btw", ignoreCase = true)
                                                val isTypedBtw = slashCmds.isEmpty() && (trimmedSend == "/btw" || trimmedSend.startsWith("/btw "))
                                                if (uiState.selectedImages.isEmpty() && uiState.selectedFiles.isEmpty() && (isSlashBtw || isTypedBtw)) {
                                                    val q = if (isSlashBtw) trimmedSend else trimmedSend.drop(4).trim()
                                                    viewModel.onInputTextChanged("")
                                                    viewModel.clearSlashCommands()
                                                    val cid = uiState.cascadeId
                                                    if (q.isEmpty() || cid.isBlank() || cid.startsWith("local_draft_")) {
                                                        btwAnchorStepIndex = null
                                                        btwAnchorPreview = null
                                                        showBtwSheet = true
                                                    } else {
                                                        coroutineScope.launch {
                                                            viewModel.startBtwAndOpen(question = q) { newId, newTitle ->
                                                                preserveScrollOnReturn = true
                                                                savedScrollIndex = listState.firstVisibleItemIndex
                                                                savedScrollOffset = listState.firstVisibleItemScrollOffset
                                                                onOpenConversation(newId, newTitle)
                                                            }
                                                        }
                                                    }
                                                } else {
                                                    viewModel.sendCurrentMessage()
                                                }
                                            },
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Icon(
                                            imageVector = Icons.Default.ArrowUpward,
                                            contentDescription = "Send",
                                            tint = if (isEnabled) Color.White else colors.textSecondary.copy(alpha = 0.5f),
                                            modifier = Modifier.size(19.dp)
                                        )
                                    }
                                }
                                isActivelyRunning -> {
                                    // Stop button: 44.dp circle, matches iOS stop button (gray circle with red stop square)
                                    Box(
                                        modifier = Modifier
                                            .size(44.dp)
                                            .shadow(
                                                elevation = 2.dp,
                                                shape = CircleShape,
                                                ambientColor = Color.Black.copy(alpha = 0.05f),
                                                spotColor = colors.accentRed.copy(alpha = 0.25f)
                                            )
                                            .clip(CircleShape)
                                            .background(colors.surfaceVariant)
                                            .border(0.8.dp, colors.border, CircleShape)
                                            .clickable {
                                                dismissKeyboard()
                                                viewModel.cancelExecution()
                                            },
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .size(15.dp)
                                                .clip(RoundedCornerShape(3.dp))
                                                .background(colors.accentRed)
                                        )
                                    }
                                }
                                else -> {
                                    // Voice Mode button: 1:1 Desktop Antigravity styling parity
                                    Box(
                                        modifier = Modifier
                                            .size(44.dp)
                                            .shadow(
                                                elevation = 2.dp,
                                                shape = CircleShape,
                                                ambientColor = Color.Black.copy(alpha = 0.2f),
                                                spotColor = Color.Black.copy(alpha = 0.3f)
                                            )
                                            .clip(CircleShape)
                                            .background(if (isDictating) colors.accentRed else Color(0xFF475558))
                                            .border(0.8.dp, Color.White.copy(alpha = 0.14f), CircleShape)
                                            .combinedClickable(
                                                onClick = {
                                                    haptic.medium()
                                                    dismissKeyboard()
                                                    if (isDictating) {
                                                        dictationManager.stop()
                                                    } else {
                                                        launchVoiceCall()
                                                    }
                                                },
                                                onLongClick = {
                                                    haptic.heavy()
                                                    dismissKeyboard()
                                                    toggleDictation()
                                                }
                                            ),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        AntigravityVoiceIcon(
                                            modifier = Modifier.size(22.dp),
                                            tint = Color.White
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
            }
        }
    }

    // 对话导轨与大纲导航抽屉 (Antigravity 2.30.0 对齐)
    LaunchedEffect(showNavigatorSheet) {
        if (showNavigatorSheet) {
            viewModel.refreshChapterTitles(chapters)
        }
    }
    if (showNavigatorSheet) {
        ConversationNavigatorSheet(
            turns = turns,
            chapters = chapters,
            currentTurnIndex = currentTurnIndex,
            isLoadingChapterTitles = uiState.isLoadingChapterTitles,
            onSelectTurn = { turn ->
                showNavigatorSheet = false
                jumpToTurn(turn)
            },
            onJumpToTop = {
                showNavigatorSheet = false
                val firstTurn = turns.firstOrNull()
                if (firstTurn != null) {
                    jumpToTurn(firstTurn)
                } else {
                    lockedTurnIndex = 1
                    coroutineScope.launch {
                        isProgrammaticScrolling = true
                        try {
                            listState.animateScrollToItem(0)
                        } finally {
                            isProgrammaticScrolling = false
                        }
                    }
                }
            },
            onJumpToBottom = {
                showNavigatorSheet = false
                activeTurnJumpJob?.cancel()
                lockedTurnIndex = null
                coroutineScope.launch {
                    val total = listState.layoutInfo.totalItemsCount
                    if (total > 0) {
                        isProgrammaticScrolling = true
                        try {
                            listState.animateScrollToItem(total - 1)
                        } finally {
                            isProgrammaticScrolling = false
                        }
                    }
                }
            },
            onDismiss = { showNavigatorSheet = false }
        )
    }

    if (showContextAuditSheet) {
        uiState.contextMeter?.let { meter ->
            ContextAuditSheet(
                meter = meter,
                isRunning = isSessionRunning,
                hasPendingInteraction = hasPendingApproval,
                canCompact = !uiState.cascadeId.startsWith("local_draft_") && uiState.parentConversationId.isNullOrBlank(),
                onCompact = { viewModel.sendCompactCommand() },
                onStartNewSession = onStartNewSession,
                onJumpToMessage = { targetMsgId ->
                    showContextAuditSheet = false
                    activeTurnJumpJob?.cancel()
                    hasUserInteracted = true
                    activeTurnJumpJob = coroutineScope.launch {
                        isProgrammaticScrolling = true
                        try {
                            val currentState = viewModel.uiState.value
                            val existingIdx = currentState.messages.indexOfFirst { it.id == targetMsgId }
                            if (existingIdx >= 0) {
                                val hasHeader = if (currentState.hasMore) 1 else 0
                                val scrollIdx = existingIdx + hasHeader
                                listState.animateScrollToItem(scrollIdx)
                                highlightedTurnMessageId = targetMsgId
                            } else {
                                viewModel.loadMessagesThrough(targetMsgId, 0)
                                val updatedState = viewModel.uiState.value
                                val targetIdx = updatedState.messages.indexOfFirst { it.id == targetMsgId }
                                if (targetIdx >= 0) {
                                    val hasHeader = if (updatedState.hasMore) 1 else 0
                                    delay(40L)
                                    listState.scrollToItem(targetIdx + hasHeader)
                                    highlightedTurnMessageId = targetMsgId
                                }
                            }
                        } finally {
                            delay(50L)
                            isProgrammaticScrolling = false
                        }
                    }
                },
                onDismiss = { showContextAuditSheet = false }
            )
        }
    }

    // MCP 工具与技能看板 / 全局后台任务中心
    if (showMcpSkillsSheet) {
        val hasAnyBgTasks = uiState.runningTasks.isNotEmpty() || uiState.globalBackgroundTasks.isNotEmpty()
        McpSkillsSheet(
            initialTab = mcpSkillsInitialTabOverride ?: if (hasAnyBgTasks) McpSkillsTab.TASKS else McpSkillsTab.MCP,
            currentConversationId = uiState.cascadeId,
            hasActiveSwarm = uiState.swarmBrief?.active == true,
            onSelectConversation = { targetId, targetTitle ->
                openSubagent(
                    com.antigravity.mobile.data.model.SubagentItem(
                        conversationId = targetId,
                        title = targetTitle,
                        status = "running"
                    )
                )
            },
            onTasksChanged = { viewModel.updateGlobalBackgroundTasks(it) },
            onOpenSwarm = { showSwarmSheet = true },
            onCreateAutomationWithPrompt = {
                showMcpSkillsSheet = false
                mcpSkillsInitialTabOverride = null
                onCreateAutomationWithPrompt()
            },
            onDismiss = {
                showMcpSkillsSheet = false
                mcpSkillsInitialTabOverride = null
            }
        )
    }

    // Swarm 蜂群协同看板
    if (showSwarmSheet) {
        SwarmSheet(
            cascadeId = uiState.cascadeId,
            apiClient = viewModel.apiClient,
            onSelectConversation = { targetId ->
                showSwarmSheet = false
                onOpenSubagent(
                    com.antigravity.mobile.data.model.SubagentItem(
                        conversationId = targetId,
                        status = "running"
                    )
                )
            },
            onDismiss = { showSwarmSheet = false }
        )
    }

    // "+" attachment panel (camera, recent photos, add file)
    if (showAttachmentSheet) {
        AttachmentPickerSheet(
            remainingImageSlots = (9 - uiState.selectedImages.size).coerceAtLeast(0),
            onDismiss = { showAttachmentSheet = false },
            onPhotosPicked = { uris ->
                showAttachmentSheet = false
                viewModel.addImagesFromUris(context, uris)
            },
            isDocumentScannerSupported = isDocumentScannerSupported,
            onScanDocument = launchDocumentScanner,
            onRecordAudio = launchAudioRecording,
            onOpenCamera = {
                if (androidx.core.content.ContextCompat.checkSelfPermission(
                        context, android.Manifest.permission.CAMERA
                    ) == android.content.pm.PackageManager.PERMISSION_GRANTED
                ) {
                    launchCamera()
                } else {
                    cameraPermissionLauncher.launch(android.Manifest.permission.CAMERA)
                }
            },
            onPickFiles = { filePickerLauncher.launch(arrayOf("*/*")) }
        )
    }

    // Markdown File Viewer Sheet
    uiState.markdownViewerData?.let { viewerData ->
        MarkdownViewerSheet(
            data = viewerData,
            onProceed = { viewModel.proceedFromViewer() },
            onDismiss = { viewModel.closeMarkdownViewer() },
            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
            urlResolver = { raw -> viewModel.resolveMediaUrl(raw) },
            onImageClick = { url ->
                if (com.antigravity.mobile.data.service.FileIconResolver.isVideo(url)) {
                    handleFileOrLinkClick(url, "")
                } else {
                    viewModel.openImageViewer(url = url)
                }
            }
        )
    }

    // Fullscreen Image Viewer Sheet
    uiState.imageViewerData?.let { viewerData ->
        ImageViewerSheet(
            data = viewerData,
            onDismiss = { viewModel.closeImageViewer() }
        )
    }

    // Document Preview Sheet (PDF, HTML, Office, Text, Jupyter Notebook)
    uiState.previewDocumentFile?.let { docFile ->
        if (docFile.name.endsWith(".ipynb", ignoreCase = true)) {
            val notebookContent = remember(docFile) {
                try { docFile.readText() } catch (_: Exception) { "" }
            }
            NotebookPreviewDialog(
                jsonContent = notebookContent,
                fileName = uiState.previewDocumentTitle.ifBlank { docFile.name },
                onDismiss = { viewModel.closeDocumentPreview() }
            )
        } else {
            DocumentPreviewSheet(
                file = docFile,
                title = uiState.previewDocumentTitle,
                onDismiss = { viewModel.closeDocumentPreview() }
            )
        }
    }

    // WeChat Chat Archive Sheet
    uiState.weChatArchiveItem?.let { archiveItem ->
        WeChatChatArchiveSheet(
            item = archiveItem,
            apiClient = viewModel.apiClient,
            cascadeId = uiState.cascadeId,
            onFallbackToDocumentPreview = {
                viewModel.downloadAndPreviewDocument(archiveItem.uri, archiveItem.fileName)
            },
            onDismiss = { viewModel.closeWeChatArchive() }
        )
    }

    // 会话切换、Agent 运行状态翻转（跑完了）、提交/改动面板关闭后重新探测 Commit / Changes 是否该显示
    val changesSupported = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.CHANGES)
    LaunchedEffect(uiState.cascadeId, uiState.isRunning, uiState.gitSheet == null, changesSupported) {
        viewModel.refreshActionAvailability(changesSupported)
    }

    // 本会话改动（累计 diff）+ 工作区代码/文件检索
    uiState.changesSheet?.let { sheet ->
        CascadeChangesSheet(
            state = sheet,
            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
            onDismiss = { viewModel.closeChangesSheet() }
        )
    }

    // Git 直接提交
    uiState.gitSheet?.let { sheet ->
        GitCommitSheet(
            state = sheet,
            cascadeId = uiState.cascadeId.ifBlank { cascadeId },
            onRefreshGit = { viewModel.openGitSheet() },
            onCommit = { message, paths, push -> viewModel.commitGit(message, paths, push) },
            onRetryPush = { viewModel.retryGitPush() },
            onDelegateToAgent = { viewModel.delegateCommitToAgent() },
            onDismiss = { viewModel.closeGitSheet() }
        )
    }

    if (pendingTurboModeConfirm) {
        AlertDialog(
            onDismissRequest = { pendingTurboModeConfirm = false },
            containerColor = colors.surface,
            title = { Text("开启全自动 · 无沙箱？".loc, fontWeight = FontWeight.Bold) },
            text = {
                Text(
                    "开启后 Agent 将自动执行终端命令并访问工作区外文件（同步作用于桌面端全部会话）。建议仅在可信或隔离环境中使用。".loc,
                    fontSize = 13.5.sp,
                    color = colors.textSecondary
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        pendingTurboModeConfirm = false
                        viewModel.setPermissionPreset("turbo") { msg -> phase3Notice = msg }
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) { Text("确认开启".loc) }
            },
            dismissButton = {
                TextButton(onClick = { pendingTurboModeConfirm = false }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            }
        )
    }

    // Confirm Undo Bottom Sheet
    if (uiState.showConfirmUndoSheet) {
        ConfirmUndoBottomSheet(
            message = uiState.activeUndoMessage,
            preview = uiState.revertPreview,
            isLoadingPreview = uiState.isLoadingRevertPreview,
            isReverting = uiState.isReverting,
            errorMessage = uiState.undoErrorMessage,
            onConfirm = { viewModel.confirmUndo() },
            onDismiss = { viewModel.dismissConfirmUndo() }
        )
    }

    // Downloading Document Progress Dialog
    if (uiState.isDownloadingDocument) {
        val downloadProgress by viewModel.documentDownloadProgress.collectAsState()
        androidx.compose.ui.window.Dialog(onDismissRequest = { viewModel.cancelDocumentDownload() }) {
            Surface(
                shape = RoundedCornerShape(14.dp),
                color = colors.surface,
                tonalElevation = 6.dp,
                modifier = Modifier.padding(24.dp)
            ) {
                Column(
                    modifier = Modifier.padding(20.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    if (downloadProgress > 0f) {
                        val animatedProgress by animateFloatAsState(
                            targetValue = downloadProgress.coerceIn(0f, 1f),
                            animationSpec = tween(durationMillis = 120, easing = FastOutSlowInEasing),
                            label = "documentDownloadProgress"
                        )
                        LinearProgressIndicator(
                            progress = { animatedProgress },
                            color = colors.accentIndigo,
                            modifier = Modifier.fillMaxWidth()
                        )
                    } else {
                        LinearProgressIndicator(
                            color = colors.accentIndigo,
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                    Text(
                        text = "正在下载 %s...".loc(uiState.downloadingDocumentName.orEmpty()),
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Medium,
                        textAlign = androidx.compose.ui.text.style.TextAlign.Center
                    )
                    TextButton(onClick = { viewModel.cancelDocumentDownload() }) {
                        Text("取消".loc, color = colors.textMuted)
                    }
                }
            }
        }
    }

    // Confirm send queued message now while background work is active
    if (showSendQueuedNowConfirmDialog) {
        AlertDialog(
            onDismissRequest = {
                showSendQueuedNowConfirmDialog = false
                queuedItemToSendNow = null
            },
            title = {
                Text(
                    text = "后台任务正在执行".loc,
                    fontWeight = FontWeight.Bold,
                    fontSize = 17.sp,
                    color = colors.textPrimary
                )
            },
            text = {
                Text(
                    text = "当前有正在运行的后台命令或子代理。立即发送将提前触发下一轮对话，可能会导致后台任务被中断或产生冲突。确定仍要立即发送吗？".loc,
                    fontSize = 14.sp,
                    color = colors.textSecondary,
                    lineHeight = 20.sp
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        val item = queuedItemToSendNow
                        showSendQueuedNowConfirmDialog = false
                        queuedItemToSendNow = null
                        if (item != null) {
                            viewModel.sendQueuedMessageNow(item)
                        }
                    },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.accentRed)
                ) {
                    Text("仍要立即发送".loc, fontWeight = FontWeight.SemiBold)
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        showSendQueuedNowConfirmDialog = false
                        queuedItemToSendNow = null
                    },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.textSecondary)
                ) {
                    Text("取消".loc)
                }
            },
            containerColor = colors.surface,
            shape = RoundedCornerShape(16.dp)
        )
    }

    // 动态多智能体工作流预设模板选择抽屉 (WorkflowTemplateSheet)
    if (showWorkflowTemplateSheet) {
        WorkflowTemplateSheet(
            onDismissRequest = {
                showWorkflowTemplateSheet = false
                workflowSheetStartEditing = false
            },
            onSelectTemplate = { prompt ->
                showWorkflowTemplateSheet = false
                workflowSheetStartEditing = false
                viewModel.onInputTextChanged(prompt)
                pendingSelectFirstPlaceholder = true
            },
            initialEditing = workflowSheetStartEditing,
            onTemplatesChanged = { workflowTemplates = it }
        )
    }

    // 工作流子智能体原位详情半屏抽屉 (SubagentDetailBottomSheet)
    selectedWorkflowSubagent?.let { sa ->
        SubagentDetailBottomSheet(
            subagent = sa,
            onDismissRequest = { selectedWorkflowSubagent = null },
            onStopSubagent = { item ->
                viewModel.stopSubagent(item.conversationId)
            },
            onAskBtw = { quote ->
                btwAnchorStepIndex = null
                btwAnchorPreview = quote
                showBtwSheet = true
            }
        )
    }

    selectedGitHubPRUrl?.let { prUrl ->
        GitHubPRDetailBottomSheet(
            prUrl = prUrl,
            onDismiss = { selectedGitHubPRUrl = null },
            onOpenInBrowser = { targetUrl ->
                com.antigravity.mobile.ui.util.InAppBrowserUtils.openUrl(context, targetUrl)
            }
        )
    }

    if (showBtwSheet) {
        BtwSideQuestionBottomSheet(
            parentCascadeId = uiState.cascadeId,
            parentTitle = uiState.title,
            anchorStepIndex = btwAnchorStepIndex,
            anchorPreview = btwAnchorPreview,
            existingBranches = uiState.sideQuestions,
            onOpenBranch = { targetId, targetTitle ->
                preserveScrollOnReturn = true
                savedScrollIndex = listState.firstVisibleItemIndex
                savedScrollOffset = listState.firstVisibleItemScrollOffset
                onOpenConversation(targetId, targetTitle)
            },
            onStartBranch = { question, stepIdx ->
                viewModel.startBtwAndOpen(question = question, stepIndex = stepIdx) { newId, newTitle ->
                    preserveScrollOnReturn = true
                    savedScrollIndex = listState.firstVisibleItemIndex
                    savedScrollOffset = listState.firstVisibleItemScrollOffset
                    onOpenConversation(newId, newTitle)
                }
            },
            onDismiss = {
                showBtwSheet = false
                btwAnchorStepIndex = null
                btwAnchorPreview = null
            }
        )
    }

    // iOS 对齐的模型上拉右滑二级选择器
    com.antigravity.mobile.ui.components.ModelSlidePickerOverlay(
        state = modelSlidePickerState,
        onSelectModel = { viewModel.selectModel(it) },
        isRunning = uiState.isRunning
    )

    // Workflow 长按上滑快捷模板菜单
    com.antigravity.mobile.ui.components.WorkflowSlidePickerOverlay(
        state = workflowSlidePickerState,
        onSelectTemplate = { prompt ->
            viewModel.onInputTextChanged(prompt)
            pendingSelectFirstPlaceholder = true
        },
        onOpenSetup = {
            dismissKeyboard()
            workflowSheetStartEditing = true
            showWorkflowTemplateSheet = true
        }
    )

    // Commit / PR 长按上滑快捷提交命令菜单
    com.antigravity.mobile.ui.components.CommitSlidePickerOverlay(
        state = commitSlidePickerState,
        onSelect = { cmd ->
            viewModel.onInputTextChanged(cmd)
        }
    )
    }
    }
}

@Composable
private fun ChatLoadingStateView() {
    val colors = AntigravityTheme.colors

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        CircularProgressIndicator(
            color = colors.accentIndigo,
            strokeWidth = 3.dp,
            modifier = Modifier.size(34.dp)
        )

        Spacer(modifier = Modifier.height(14.dp))

        Text(
            text = "正在同步会话历史与步骤...".loc,
            color = colors.textSecondary,
            fontSize = 13.5.sp,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun ChatErrorStateView(
    errorMessage: String,
    onRetry: () -> Unit
) {
    val colors = AntigravityTheme.colors

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Icon(
            imageVector = Icons.Default.Warning,
            contentDescription = "Error",
            tint = colors.accentOrange,
            modifier = Modifier.size(36.dp)
        )

        Spacer(modifier = Modifier.height(14.dp))

        Text(
            text = errorMessage,
            color = colors.textSecondary,
            fontSize = 13.5.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(horizontal = 24.dp)
        )

        Spacer(modifier = Modifier.height(16.dp))

        Button(
            onClick = onRetry,
            colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo),
            shape = RoundedCornerShape(10.dp)
        ) {
            Text("点击重试".loc)
        }
    }
}

@Composable
private fun ChatEmptyStateView(title: String) {
    val colors = AntigravityTheme.colors

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Box(
            modifier = Modifier
                .size(58.dp)
                .clip(CircleShape)
                .background(colors.accentIndigo.copy(alpha = 0.12f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.AutoAwesome,
                contentDescription = "Sparkles",
                tint = colors.accentIndigo,
                modifier = Modifier.size(26.dp)
            )
        }

        Spacer(modifier = Modifier.height(16.dp))

        Text(
            text = title,
            color = colors.textPrimary,
            fontSize = 17.sp,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )

        Spacer(modifier = Modifier.height(6.dp))

        Text(
            text = if (title == "新对话".loc) "新对话模式，在下方输入指令开启对话".loc else "已连接工作区，在下方输入指令开启对话".loc,
            color = colors.textSecondary,
            fontSize = 13.sp,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun AgentThinkingBubble(
    focusText: String? = null,
    startedAt: String? = null
) {
    val resolvedFocus = focusText?.trim().orEmpty().ifEmpty { "工作中".loc }
    ActiveFocusToolRow(
        focusText = resolvedFocus,
        startedAt = startedAt,
        isExpandable = false
    )
}

/**
 * 1:1 对齐 Antigravity 桌面端（main.js tuc() 组件）的 3 波形条 + 4 芒星矢量图标
 */
@Composable
fun AntigravityVoiceIcon(
    modifier: Modifier = Modifier,
    tint: Color = Color.White
) {
    androidx.compose.foundation.Canvas(modifier = modifier) {
        val w = size.width
        val h = size.height

        val r1 = CornerRadius(w * 0.04271f, w * 0.04271f)
        // Bar 1
        drawRoundRect(
            color = tint,
            topLeft = Offset(w * 0.21021f, h * 0.35083f),
            size = Size(w * 0.08542f, h * 0.36458f),
            cornerRadius = r1
        )

        // Bar 2
        drawRoundRect(
            color = tint,
            topLeft = Offset(w * 0.41021f, h * 0.25969f),
            size = Size(w * 0.08542f, h * 0.54688f),
            cornerRadius = r1
        )

        // Bar 3
        drawRoundRect(
            color = tint,
            topLeft = Offset(w * 0.61021f, h * 0.41594f),
            size = Size(w * 0.08542f, h * 0.23438f),
            cornerRadius = r1
        )

        // Sparkle 4-point star
        val star = Path().apply {
            moveTo(w * 0.78958f, h * 0.37400f)
            quadraticBezierTo(w * 0.78358f, h * 0.37400f, w * 0.78158f, h * 0.36800f)
            quadraticBezierTo(w * 0.76635f, h * 0.30969f, w * 0.72416f, h * 0.26750f)
            quadraticBezierTo(w * 0.68197f, h * 0.22531f, w * 0.62366f, h * 0.21008f)
            quadraticBezierTo(w * 0.61766f, h * 0.20808f, w * 0.61766f, h * 0.20208f)
            quadraticBezierTo(w * 0.61766f, h * 0.19508f, w * 0.62366f, h * 0.19408f)
            quadraticBezierTo(w * 0.68197f, h * 0.17885f, w * 0.72416f, h * 0.13666f)
            quadraticBezierTo(w * 0.76635f, h * 0.09447f, w * 0.78158f, h * 0.03616f)
            quadraticBezierTo(w * 0.78358f, h * 0.03016f, w * 0.78958f, h * 0.03016f)
            quadraticBezierTo(w * 0.79658f, h * 0.03016f, w * 0.79758f, h * 0.03616f)
            quadraticBezierTo(w * 0.81343f, h * 0.09447f, w * 0.85538f, h * 0.13669f)
            quadraticBezierTo(w * 0.89733f, h * 0.17891f, w * 0.95550f, h * 0.19409f)
            quadraticBezierTo(w * 0.96150f, h * 0.19509f, w * 0.96150f, h * 0.20209f)
            quadraticBezierTo(w * 0.96150f, h * 0.20809f, w * 0.95550f, h * 0.21009f)
            quadraticBezierTo(w * 0.89719f, h * 0.22532f, w * 0.85500f, h * 0.26751f)
            quadraticBezierTo(w * 0.81281f, h * 0.30970f, w * 0.79758f, h * 0.36801f)
            quadraticBezierTo(w * 0.79658f, h * 0.37401f, w * 0.78958f, h * 0.37401f)
            close()
        }
        drawPath(star, tint)
    }
}
