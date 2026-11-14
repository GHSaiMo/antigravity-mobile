package com.antigravity.mobile.ui.screen

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
import androidx.compose.material3.pulltorefresh.PullToRefreshDefaults
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
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
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.LocalTextToolbar
import androidx.compose.ui.platform.LocalView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import java.net.URLDecoder

import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChatViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import com.antigravity.mobile.ui.viewmodel.addImagesFromUris
import com.antigravity.mobile.ui.viewmodel.cancelExecution
import com.antigravity.mobile.ui.viewmodel.clearSlashCommand
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
    onStartNewSession: () -> Unit = {}
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
    val shouldShowThinkingBubble = uiState.isAwaitingResponse || uiState.isRunning
    val coroutineScope = rememberCoroutineScope()
    var adaptiveScrollJob by remember { mutableStateOf<Job?>(null) }
    var isProgrammaticScrolling by remember { mutableStateOf(false) }
    var isInputFocused by remember { mutableStateOf(false) }
    val isImeVisible = WindowInsets.isImeVisible

    // 对话导轨与章节大纲状态 (Antigravity 2.28.1 对齐)
    var showNavigatorSheet by remember { mutableStateOf(false) }
    var showMcpSkillsSheet by remember { mutableStateOf(false) }
    var showSwarmSheet by remember { mutableStateOf(false) }
    var showContextAuditSheet by remember { mutableStateOf(false) }
    var showSendQueuedNowConfirmDialog by remember { mutableStateOf(false) }
    var queuedItemToSendNow by remember { mutableStateOf<com.antigravity.mobile.data.model.QueuedMessageItem?>(null) }
    var lockedTurnIndex by remember { mutableStateOf<Int?>(null) }
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
    val turns = remember(uiState.messages, uiState.isRunning, uiState.isAwaitingResponse) {
        ConversationTurnBuilder.buildTurns(uiState.messages, uiState.isRunning, uiState.isAwaitingResponse)
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

    val currentTurnIndex by remember(turns) {
        derivedStateOf {
            if (turns.isEmpty()) {
                1
            } else if (lockedTurnIndex != null && turns.any { it.turnIndex == lockedTurnIndex }) {
                lockedTurnIndex!!
            } else if (!hasInitiallyAligned || isNearBottom || !listState.canScrollForward) {
                turns.lastOrNull()?.turnIndex ?: 1
            } else if (!listState.canScrollBackward) {
                turns.firstOrNull()?.turnIndex ?: 1
            } else {
                val layoutInfo = listState.layoutInfo
                val anchorY = (layoutInfo.viewportEndOffset * 0.45f).toInt()
                val messageItems = layoutInfo.visibleItemsInfo.filter { item ->
                    val k = item.key as? String ?: return@filter false
                    turns.any { t -> t.messages.any { m -> m.id == k } }
                }
                val bestItem = messageItems.minByOrNull { item ->
                    val top = item.offset
                    val bottom = item.offset + item.size
                    if (anchorY in top..bottom) 0 else minOf(kotlin.math.abs(top - anchorY), kotlin.math.abs(bottom - anchorY))
                }
                val targetId = bestItem?.key as? String
                if (targetId != null) {
                    turns.firstOrNull { turn -> turn.messages.any { it.id == targetId } }?.turnIndex
                        ?: turns.lastOrNull()?.turnIndex ?: 1
                } else {
                    turns.lastOrNull()?.turnIndex ?: 1
                }
            }
        }
    }

    val openSubagent: (com.antigravity.mobile.data.model.SubagentItem) -> Unit = { sub ->
        savedScrollIndex = listState.firstVisibleItemIndex
        savedScrollOffset = listState.firstVisibleItemScrollOffset
        preserveScrollOnReturn = hasInitiallyAligned
        onOpenSubagent(sub)
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
            android.widget.Toast.makeText(context, "无法打开相机", android.widget.Toast.LENGTH_SHORT).show()
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
            android.util.Log.e("ChatScreen", "解析文档扫描结果异常", t)
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
                            android.widget.Toast.makeText(context, "无法启动文档扫描", android.widget.Toast.LENGTH_SHORT).show()
                        }
                    }
                    .addOnFailureListener { e ->
                        android.widget.Toast.makeText(context, "启动文档扫描失败: ${e.localizedMessage ?: "服务不可用"}", android.widget.Toast.LENGTH_SHORT).show()
                    }
            } else {
                android.widget.Toast.makeText(context, "无法获取界面上下文", android.widget.Toast.LENGTH_SHORT).show()
            }
        } catch (t: Throwable) {
            android.util.Log.e("ChatScreen", "初始化文档扫描异常", t)
            android.widget.Toast.makeText(context, "当前设备暂不支持原生文档扫描", android.widget.Toast.LENGTH_SHORT).show()
        }
    }

    val audioRecorder = remember { AudioRecorderManager(context) }
    val isRecordingAudio by audioRecorder.isRecording.collectAsState()
    val voiceCall = remember { VoiceCallManager(context) }
    val isInVoiceCall by voiceCall.isInCall.collectAsState()
    var showVoiceTroubleshootBubble by remember { mutableStateOf(false) }
    var lastVoiceTroubleshootBubbleAt by remember { mutableStateOf(0L) }

    // 语音明确失败（此时通话已被自动挂断）后弹出排查气泡；30 秒内不重复弹
    LaunchedEffect(voiceCall) {
        voiceCall.failureEvents.collect {
            val now = System.currentTimeMillis()
            if (now - lastVoiceTroubleshootBubbleAt > 30_000L) {
                lastVoiceTroubleshootBubbleAt = now
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
    val voicePrefs = remember { viewModel.prefs ?: com.antigravity.mobile.data.service.PreferencesManager(context) }
    val enableVoiceMode by voicePrefs.enableVoiceModeFlow.collectAsState()
    val fontSizeScaleSetting by voicePrefs.fontSizeScaleFlow.collectAsState()
    val isKnowledgeModeEnabled by voicePrefs.knowledgeModeFlow.collectAsState()
    val modelCatalog by voicePrefs.modelCatalogFlow.collectAsState()

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
                android.widget.Toast.makeText(context, "无法启动实时语音通话", android.widget.Toast.LENGTH_SHORT).show()
            }
        }
    }

    val voiceCallPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { granted ->
        if (granted) {
            startVoiceCallInternal()
        } else {
            android.widget.Toast.makeText(context, "需要麦克风权限以开启实时语音对话", android.widget.Toast.LENGTH_SHORT).show()
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
                android.widget.Toast.makeText(context, "无法启动录音", android.widget.Toast.LENGTH_SHORT).show()
            }
        } else {
            android.widget.Toast.makeText(context, "需要麦克风权限以录制音频", android.widget.Toast.LENGTH_SHORT).show()
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
                android.widget.Toast.makeText(context, "无法启动录音", android.widget.Toast.LENGTH_SHORT).show()
            }
        } else {
            audioPermissionLauncher.launch(android.Manifest.permission.RECORD_AUDIO)
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            audioRecorder.cancelRecording()
            voiceCall.endCall()
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
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE || event == Lifecycle.Event.ON_STOP) {
                textToolbar.hide()
                viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
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
                        listState.animateScrollToItem(total - 1)
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

    // 在有选项状态下默认收起软键盘
    LaunchedEffect(uiState.pendingInteraction) {
        if (uiState.pendingInteraction != null) {
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
        val rawName = title.ifEmpty { clean.substringAfterLast('/') }
        val decodedFileName = try {
            URLDecoder.decode(rawName, "UTF-8")
        } catch (_: Exception) {
            rawName
        }

        // Script and code files do not need to be accessible (aligned 1:1 with iOS)
        if (!com.antigravity.mobile.data.service.FileIconResolver.isScriptFile(clean) &&
            !com.antigravity.mobile.data.service.FileIconResolver.isScriptFile(decodedFileName)
        ) {
            // 1. Office Documents, Presentations, PDFs & Images (matching iOS allPreviewExtensions)
            if (com.antigravity.mobile.data.service.FileIconResolver.isPreviewableDocument(clean) ||
                com.antigravity.mobile.data.service.FileIconResolver.isPreviewableDocument(decodedFileName)
            ) {
                haptic.medium()
                viewModel.downloadAndPreviewDocument(clean, decodedFileName)
            } else if (lower.endsWith(".md") || lower.endsWith(".markdown") ||
                lower.contains("/brain/") || lower.contains("/static/artifacts/") ||
                lower.contains("implementation_plan") || lower.contains("walkthrough")
            ) {
                // 2. Markdown & Plan Artifacts
                val docTitle = when {
                    title.isNotBlank() && !title.endsWith(".md", ignoreCase = true) && !title.endsWith(".markdown", ignoreCase = true) -> title
                    lower.contains("walkthrough") -> "Walkthrough"
                    lower.contains("implementation_plan") -> "Implementation Plan"
                    title.isNotBlank() -> title
                    else -> decodedFileName
                }
                viewModel.openMarkdownViewer(clean, docTitle)
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
        com.antigravity.mobile.ui.components.LocalKnowledgeModeEnabled provides isKnowledgeModeEnabled
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
                    if (turns.size >= 2) {
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
                                        text = "第 $currentTurnIndex/${turns.size} 轮",
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentIndigo
                                    )
                                    Spacer(modifier = Modifier.width(2.dp))
                                    Icon(
                                        imageVector = Icons.Default.KeyboardArrowDown,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(11.dp)
                                    )
                                }
                                ContextRingCapsule(
                                    meter = uiState.contextMeter,
                                    isRunning = uiState.isRunning || uiState.isAwaitingResponse,
                                    onClick = {
                                        haptic.light()
                                        showContextAuditSheet = true
                                    }
                                )
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
                            if (uiState.contextMeter != null) {
                                Spacer(modifier = Modifier.height(2.dp))
                                ContextRingCapsule(
                                    meter = uiState.contextMeter,
                                    isRunning = uiState.isRunning || uiState.isAwaitingResponse,
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
                    IconButton(onClick = {
                        haptic.light()
                        showMcpSkillsSheet = true
                    }) {
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
                                contentDescription = "MCP 工具与技能看板",
                                tint = colors.accentBlue
                            )
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
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
        ) {
            val swarm = uiState.swarmBrief
            if (swarm != null && swarm.active) {
                SwarmBanner(
                    brief = swarm,
                    onClick = { showSwarmSheet = true },
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp)
                )
            }

            // Content Area (Loading / Error / Empty / Messages)
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
            ) {
                when {
                    uiState.isLoading && uiState.messages.isEmpty() && !uiState.isNewConversation -> {
                        ChatLoadingStateView()
                    }
                    uiState.errorMessage != null && uiState.messages.isEmpty() && !uiState.isNewConversation -> {
                        ChatErrorStateView(
                            errorMessage = uiState.errorMessage ?: "同步会话历史失败",
                            onRetry = { viewModel.retryLoadMessages() }
                        )
                    }
                    uiState.messages.isEmpty() && !shouldShowThinkingBubble -> {
                        ChatEmptyStateView(title = uiState.title)
                    }
                    else -> {
                        PullToRefreshBox(
                            isRefreshing = isRefreshing,
                            onRefresh = { viewModel.refresh() },
                            modifier = Modifier.fillMaxSize(),
                            state = pullRefreshState,
                            indicator = {
                                PullToRefreshDefaults.Indicator(
                                    state = pullRefreshState,
                                    isRefreshing = isRefreshing,
                                    modifier = Modifier.align(Alignment.TopCenter),
                                    containerColor = colors.surface,
                                    color = colors.accentIndigo
                                )
                            }
                        ) {
                            LazyColumn(
                                state = listState,
                                modifier = Modifier.fillMaxSize(),
                                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                                verticalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                if (uiState.hasMore) {
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
                                                            contentDescription = "查看更早的消息",
                                                            modifier = Modifier.size(14.dp),
                                                            tint = MaterialTheme.colorScheme.onSurfaceVariant
                                                        )
                                                        Text(
                                                            text = "查看更早的消息",
                                                            style = MaterialTheme.typography.labelMedium,
                                                            color = MaterialTheme.colorScheme.onSurfaceVariant
                                                        )
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }

                                items(
                                    uiState.messages,
                                    key = { if (it.id.isNotBlank()) it.id else "${it.timestamp}_${it.role}_${it.stepIndex ?: 0}" }
                                ) { rawMsg ->
                                    val msg = com.antigravity.mobile.ui.util.rememberPacedMessage(rawMsg)
                                    MessageBubble(
                                        message = msg,
                                        onPlanClick = { uri, title ->
                                            handleFileOrLinkClick(uri, title)
                                        },
                                        urlResolver = { raw -> viewModel.resolveMediaUrl(raw) },
                                        onImageClick = { url, bitmap ->
                                            if (!url.isNullOrEmpty() && com.antigravity.mobile.data.service.FileIconResolver.isVideo(url)) {
                                                handleFileOrLinkClick(url, "")
                                            } else {
                                                viewModel.openImageViewer(bitmap = bitmap, url = url)
                                            }
                                        },
                                        onImageGroupClick = { items, index ->
                                            viewModel.openImageViewer(items = items, initialIndex = index)
                                        },
                                        onSubagentClick = openSubagent,
                                        onCompactClick = { viewModel.sendCompactCommand() },
                                        onStartNewSessionClick = onStartNewSession,
                                        onUndoClick = if (compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.REVERT)) {
                                            { target -> viewModel.requestUndo(target) }
                                        } else null,
                                        onExportMarkdownClick = if (compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.EXPORT)) {
                                            {
                                                viewModel.exportMarkdown { title, markdown ->
                                                    coroutineScope.launch {
                                                        com.antigravity.mobile.ui.util.MarkdownExportUtils.shareMarkdown(context, title, markdown)
                                                    }
                                                }
                                            }
                                        } else null,
                                        shareContextProvider = { pacedTarget ->
                                            val list = uiState.messages
                                            // 播放中的消息是副本，按 id 还原回原始消息再取上下文
                                            val target = list.firstOrNull { it.id.isNotBlank() && it.id == pacedTarget.id } ?: pacedTarget
                                            val idx = list.indexOfFirst { it === target }.takeIf { it >= 0 } ?: list.size
                                            val question = list.take(idx).lastOrNull { it.isUser }
                                            val badge = ShareCardContext.resolveModel(
                                                target = target,
                                                messages = list,
                                                activeModel = uiState.activeModel,
                                                activeModelName = uiState.activeModelName
                                            )
                                            ShareCardContext(
                                                sessionTitle = uiState.title,
                                                modelName = badge.first,
                                                modelIsClaude = badge.second,
                                                startedAtIso = uiState.startedAt,
                                                previousMessage = question
                                            )
                                        }
                                    )
                                }

                                // Active Thinking Animation Card
                                if (shouldShowThinkingBubble) {
                                    item(key = "agent_thinking_bubble") {
                                        AgentThinkingBubble()
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
                if (slashQuery != null) {
                    SlashCommandPicker(
                        commands = com.antigravity.mobile.data.model.SlashCommandFilter.filter(uiState.slashCommands, slashQuery),
                        isLoading = uiState.isLoadingSlashCommands,
                        onSelect = {
                            isSlashPickerDismissed = false
                            viewModel.selectSlashCommand(it)
                        },
                        onDismiss = {
                            isSlashPickerDismissed = true
                        },
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .padding(top = 8.dp, bottom = 8.dp)
                    )

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
                                lockedTurnIndex = targetTurn
                                val matchTurn = turns.firstOrNull { it.turnIndex == targetTurn }
                                if (matchTurn != null) {
                                    val targetIndex = uiState.messages.indexOfFirst { it.id == matchTurn.firstMessageId }
                                    if (targetIndex >= 0) {
                                        coroutineScope.launch {
                                            isProgrammaticScrolling = true
                                            try {
                                                listState.animateScrollToItem(targetIndex)
                                            } finally {
                                                isProgrammaticScrolling = false
                                            }
                                        }
                                    }
                                }
                            },
                            onJumpNextOrBottom = {
                                stepperAlphaTarget = 1.0f
                                if (currentTurnIndex < turns.size) {
                                    val targetTurn = currentTurnIndex + 1
                                    lockedTurnIndex = targetTurn
                                    val matchTurn = turns.firstOrNull { it.turnIndex == targetTurn }
                                    if (matchTurn != null) {
                                        val targetIndex = uiState.messages.indexOfFirst { it.id == matchTurn.firstMessageId }
                                        if (targetIndex >= 0) {
                                            coroutineScope.launch {
                                                isProgrammaticScrolling = true
                                                try {
                                                    listState.animateScrollToItem(targetIndex)
                                                } finally {
                                                    isProgrammaticScrolling = false
                                                }
                                            }
                                        }
                                    }
                                } else {
                                    lockedTurnIndex = null
                                    coroutineScope.launch {
                                        if (uiState.messages.isNotEmpty()) {
                                            isProgrammaticScrolling = true
                                            try {
                                                listState.animateScrollToItem(uiState.messages.size - 1)
                                            } finally {
                                                isProgrammaticScrolling = false
                                            }
                                        }
                                    }
                                }
                            },
                            onForceJumpBottom = {
                                stepperAlphaTarget = 1.0f
                                lockedTurnIndex = null
                                coroutineScope.launch {
                                    if (uiState.messages.isNotEmpty()) {
                                        isProgrammaticScrolling = true
                                        try {
                                            listState.animateScrollToItem(uiState.messages.size - 1)
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
                        val hasActiveBackgroundWork = isActivelyRunningForCards ||
                                uiState.runningTasks.isNotEmpty() ||
                                runningSubagents.isNotEmpty()
                        QueuedMessagesCard(
                            items = uiState.queuedMessages,
                            hasActiveBackgroundWork = hasActiveBackgroundWork,
                            onSendNow = { item ->
                                dismissKeyboard()
                                if (hasActiveBackgroundWork) {
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
                        // Quick Action Chips
                        Box(modifier = Modifier.padding(horizontal = 16.dp)) {
                            QuickActionChips(
                                activeModel = uiState.activeModel,
                                onToggleModel = { viewModel.toggleModel() },
                                onSelectModel = { viewModel.selectModel(it) },
                                modelCatalog = modelCatalog,
                                onAddImage = {
                                    dismissKeyboard()
                                    showAttachmentSheet = true
                                },
                                selectedSlashCommand = uiState.selectedSlashCommand,
                                onClearSlashCommand = { viewModel.clearSlashCommand() },
                                onCommitAndPush = {
                                    dismissKeyboard()
                                    viewModel.openGitSheet()
                                },
                                commitLabel = uiState.gitChipLabel,
                                onShowChanges = {
                                    dismissKeyboard()
                                    viewModel.openChangesSheet()
                                },
                                showCommit = uiState.hasGitChanges,
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
                                value = inputText,
                                onValueChange = { viewModel.onInputTextChanged(it) },
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
                                        if (inputText.isNotBlank() || uiState.selectedImages.isNotEmpty() || uiState.selectedFiles.isNotEmpty() || uiState.selectedSlashCommand != null) {
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
                                            Text(
                                                text = if (isActivelyRunning) "向队列添加指令..." else "发送对 Agent 的指令...",
                                                color = colors.textMuted,
                                                fontSize = 16.sp,
                                                lineHeight = 22.sp
                                            )
                                        }
                                        innerTextField()
                                    }
                                }
                            )

                            val isActivelyRunning = uiState.isRunning || uiState.isAwaitingResponse || uiState.runningTasks.hasBlockingTasks
                            val isInputBlank = inputText.isBlank()
                            val hasAttachments = uiState.selectedImages.isNotEmpty() || uiState.selectedFiles.isNotEmpty() || uiState.selectedSlashCommand != null
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
                                            contentDescription = "挂断实时语音通话",
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
                                                viewModel.sendCurrentMessage()
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
                                            .background(Color(0xFF475558))
                                            .border(0.8.dp, Color.White.copy(alpha = 0.14f), CircleShape)
                                            .combinedClickable(
                                                onClick = {
                                                    haptic.medium()
                                                    dismissKeyboard()
                                                    launchVoiceCall()
                                                },
                                                onLongClick = {
                                                    haptic.heavy()
                                                    dismissKeyboard()
                                                    launchAudioRecording()
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
                lockedTurnIndex = turn.turnIndex
                val targetIndex = uiState.messages.indexOfFirst { it.id == turn.firstMessageId }
                if (targetIndex >= 0) {
                    coroutineScope.launch {
                        isProgrammaticScrolling = true
                        try {
                            listState.animateScrollToItem(targetIndex)
                        } finally {
                            isProgrammaticScrolling = false
                        }
                    }
                }
            },
            onJumpToTop = {
                showNavigatorSheet = false
                lockedTurnIndex = 1
                coroutineScope.launch {
                    isProgrammaticScrolling = true
                    try {
                        listState.animateScrollToItem(0)
                    } finally {
                        isProgrammaticScrolling = false
                    }
                }
            },
            onJumpToBottom = {
                showNavigatorSheet = false
                lockedTurnIndex = null
                coroutineScope.launch {
                    if (uiState.messages.isNotEmpty()) {
                        isProgrammaticScrolling = true
                        try {
                            listState.animateScrollToItem(uiState.messages.size - 1)
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
                canCompact = !uiState.cascadeId.startsWith("local_draft_") && uiState.parentConversationId.isNullOrBlank(),
                onCompact = { viewModel.sendCompactCommand() },
                onStartNewSession = onStartNewSession,
                onDismiss = { showContextAuditSheet = false }
            )
        }
    }

    // MCP 工具与技能看板 / 全局后台任务中心
    if (showMcpSkillsSheet) {
        val hasAnyBgTasks = uiState.runningTasks.isNotEmpty() || uiState.globalBackgroundTasks.isNotEmpty()
        McpSkillsSheet(
            initialTab = if (hasAnyBgTasks) 4 else 0,
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
            onDismiss = { showMcpSkillsSheet = false }
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

    // 会话切换、Agent 运行状态翻转（跑完了）、提交/改动面板关闭后重新探测 Commit / Changes 是否该显示
    val changesSupported = compat.isAvailable(com.antigravity.mobile.data.model.GatewayFeature.CHANGES)
    LaunchedEffect(uiState.cascadeId, uiState.isRunning, uiState.gitSheet == null, changesSupported) {
        viewModel.refreshActionAvailability(changesSupported)
    }

    // 本会话改动（累计 diff）
    uiState.changesSheet?.let { sheet ->
        CascadeChangesSheet(state = sheet, onDismiss = { viewModel.closeChangesSheet() })
    }

    // Git 直接提交
    uiState.gitSheet?.let { sheet ->
        GitCommitSheet(
            state = sheet,
            onCommit = { message, paths, push -> viewModel.commitGit(message, paths, push) },
            onRetryPush = { viewModel.retryGitPush() },
            onDelegateToAgent = { viewModel.delegateCommitToAgent() },
            onDismiss = { viewModel.closeGitSheet() }
        )
    }

    // Confirm Undo Bottom Sheet
    if (uiState.showConfirmUndoSheet) {
        ConfirmUndoBottomSheet(
            message = uiState.activeUndoMessage,
            preview = uiState.revertPreview,
            isLoadingPreview = uiState.isLoadingRevertPreview,
            isReverting = uiState.isReverting,
            onConfirm = { viewModel.confirmUndo() },
            onDismiss = { viewModel.dismissConfirmUndo() }
        )
    }

    // Downloading Document Progress Dialog
    if (uiState.isDownloadingDocument) {
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
                    if (uiState.downloadProgress > 0f) {
                        LinearProgressIndicator(
                            progress = { uiState.downloadProgress },
                            color = colors.accentIndigo,
                            modifier = Modifier.fillMaxWidth()
                        )
                    } else {
                        CircularProgressIndicator(
                            color = colors.accentIndigo,
                            modifier = Modifier.size(32.dp)
                        )
                    }
                    Text(
                        text = "正在下载 ${uiState.downloadingDocumentName}...",
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Medium,
                        textAlign = androidx.compose.ui.text.style.TextAlign.Center
                    )
                    TextButton(onClick = { viewModel.cancelDocumentDownload() }) {
                        Text("取消", color = colors.textMuted)
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
                    text = "后台任务正在执行",
                    fontWeight = FontWeight.Bold,
                    fontSize = 17.sp,
                    color = colors.textPrimary
                )
            },
            text = {
                Text(
                    text = "当前有正在运行的后台命令或子代理。立即发送将提前触发下一轮对话，可能会导致后台任务被中断或产生冲突。确定仍要立即发送吗？",
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
                    Text("仍要立即发送", fontWeight = FontWeight.SemiBold)
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
                    Text("取消")
                }
            },
            containerColor = colors.surface,
            shape = RoundedCornerShape(16.dp)
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
            text = "正在同步会话历史与步骤...",
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
            Text("点击重试")
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
            text = if (title == "新对话") "新对话模式，在下方输入指令开启对话" else "已连接工作区，在下方输入指令开启对话",
            color = colors.textSecondary,
            fontSize = 13.sp,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun AgentThinkingBubble() {
    val colors = AntigravityTheme.colors

    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        modifier = Modifier.padding(vertical = 4.dp)
    ) {
        // Agent Avatar
        Box(
            modifier = Modifier
                .size(28.dp)
                .clip(CircleShape)
                .background(colors.accentBlue.copy(alpha = 0.12f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.AutoAwesome,
                contentDescription = "Agent",
                tint = colors.accentBlue,
                modifier = Modifier.size(14.dp)
            )
        }

        // Bubble with 3 pulsing dots
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier
                .clip(RoundedCornerShape(14.dp))
                .background(colors.surface)
                .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                .padding(horizontal = 12.dp, vertical = 8.dp)
        ) {
            Text(
                text = "Agent 正在思考与执行",
                color = colors.textPrimary,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold
            )

            PulsingDots(color = colors.accentBlue)
        }
    }
}

@Composable
private fun PulsingDots(color: Color) {
    val infiniteTransition = rememberInfiniteTransition(label = "dots")

    val dot1Alpha by infiniteTransition.animateFloat(
        initialValue = 0.25f,
        targetValue = 0.9f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 600, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "dot1"
    )

    val dot2Alpha by infiniteTransition.animateFloat(
        initialValue = 0.25f,
        targetValue = 0.9f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 600, delayMillis = 200, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "dot2"
    )

    val dot3Alpha by infiniteTransition.animateFloat(
        initialValue = 0.25f,
        targetValue = 0.9f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 600, delayMillis = 400, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "dot3"
    )

    Row(
        horizontalArrangement = Arrangement.spacedBy(3.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(
            modifier = Modifier
                .size(4.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = dot1Alpha))
        )
        Box(
            modifier = Modifier
                .size(4.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = dot2Alpha))
        )
        Box(
            modifier = Modifier
                .size(4.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = dot3Alpha))
        )
    }
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
