package com.antigravity.mobile.ui.screen

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.*
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
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
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.material3.pulltorefresh.PullToRefreshContainer
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import android.net.Uri
import com.antigravity.mobile.ui.util.rememberHaptic
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
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

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun ChatScreen(
    cascadeId: String,
    initialTitle: String,
    viewModel: ChatViewModel,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    isNewConversation: Boolean = false,
    isUnreadOnEntry: Boolean = false,
    initialStatus: com.antigravity.mobile.data.model.ConversationStatus? = null
) {
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val focusRequester = remember { FocusRequester() }
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val dismissKeyboard: () -> Unit = {
        focusManager.clearFocus()
        keyboardController?.hide()
    }
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val inputText by viewModel.inputText.collectAsStateWithLifecycle()
    val scrollToBottomTrigger by viewModel.scrollToBottomTrigger.collectAsStateWithLifecycle()
    val listState = rememberLazyListState()
    val colors = AntigravityTheme.colors
    val shouldShowThinkingBubble = uiState.isAwaitingResponse || uiState.isRunning
    val coroutineScope = rememberCoroutineScope()
    var adaptiveScrollJob by remember { mutableStateOf<Job?>(null) }
    var isProgrammaticScrolling by remember { mutableStateOf(false) }
    var isInputFocused by remember { mutableStateOf(false) }
    val isImeVisible = WindowInsets.isImeVisible

    // Scroll state tracking: distinguish initial logical entry alignment vs in-session incremental scroll
    var hasInitiallyAligned by remember(cascadeId) { mutableStateOf(false) }
    var hasUserInteracted by remember(cascadeId) { mutableStateOf(false) }
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

    LaunchedEffect(pullRefreshState.isRefreshing) {
        if (pullRefreshState.isRefreshing) {
            viewModel.refresh {
                pullRefreshState.endRefresh()
            }
        }
    }

    LaunchedEffect(isRefreshing) {
        if (isRefreshing) {
            pullRefreshState.startRefresh()
        } else {
            pullRefreshState.endRefresh()
        }
    }

    // Photo picker launcher
    val photoPickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.GetMultipleContents()
    ) { uris ->
        if (uris.isNotEmpty()) {
            viewModel.addImagesFromUris(context, uris)
        }
    }

    val lifecycleOwner = LocalLifecycleOwner.current

    BackHandler {
        viewModel.handleBack(cascadeId, inputText)
        onNavigateBack()
    }

    DisposableEffect(lifecycleOwner, cascadeId) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE || event == Lifecycle.Event.ON_STOP) {
                viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            viewModel.saveDraftFor(cascadeId, inputText, uiState.selectedImages.map { it.byteArray })
        }
    }

    LaunchedEffect(cascadeId, isNewConversation) {
        hasInitiallyAligned = false
        hasUserInteracted = false
        previousMessageCount = 0
        lastTrigger = scrollToBottomTrigger
        viewModel.initSession(
            cascadeId = cascadeId,
            initialTitle = initialTitle,
            isNewConversation = isNewConversation,
            isUnread = isUnreadOnEntry,
            conversationStatus = initialStatus
        )
        // Automatically focus the input field and pop up soft keyboard ONLY on new conversation creation
        if (isNewConversation) {
            delay(250)
            try {
                focusRequester.requestFocus()
                keyboardController?.show()
            } catch (_: Exception) {}
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
        if (!uiState.isLoading && uiState.messages.isNotEmpty()) {
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

        if (!hasInitiallyAligned) {
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
        if (hasInitiallyAligned && (isNearBottom || !hasUserInteracted)) {
            performAdaptiveCardScroll()
        }
    }

    // Phase 5: 对齐 iOS isInputFocused：输入框获焦或键盘弹出时，会话内容自动往上顶，让用户可以看到会话的最底端
    LaunchedEffect(isInputFocused, isImeVisible) {
        if (isInputFocused || isImeVisible) {
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

    // Auto-dismiss keyboard when user manually scrolls through messages and track user manual scroll
    LaunchedEffect(listState.isScrollInProgress) {
        if (listState.isScrollInProgress && !isProgrammaticScrolling) {
            dismissKeyboard()
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
            // 1. Markdown & Plan Artifacts
            if (lower.endsWith(".md") || lower.endsWith(".markdown") ||
                lower.contains("/brain/") || lower.contains("/static/artifacts/") ||
                lower.contains("implementation_plan") || lower.contains("walkthrough")
            ) {
                val docTitle = when {
                    lower.contains("walkthrough") -> "Walkthrough"
                    lower.contains("implementation_plan") -> "Implementation Plan"
                    else -> decodedFileName
                }
                viewModel.openMarkdownViewer(clean, docTitle)
            } else if (com.antigravity.mobile.data.service.FileIconResolver.isAccessibleDocument(clean) ||
                com.antigravity.mobile.data.service.FileIconResolver.isAccessibleDocument(decodedFileName)
            ) {
                // 2. Office Documents, Presentations, PDFs & Images (matching iOS allPreviewExtensions)
                haptic.medium()
                viewModel.downloadAndPreviewDocument(clean, decodedFileName)
            } else if (clean.startsWith("http://") || clean.startsWith("https://")) {
                // 3. External Web links
                try {
                    val intent = android.content.Intent(android.content.Intent.ACTION_VIEW, Uri.parse(clean)).apply {
                        flags = android.content.Intent.FLAG_ACTIVITY_NEW_TASK
                    }
                    context.startActivity(intent)
                } catch (_: Exception) {}
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        text = uiState.title,
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 16.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        color = colors.textPrimary
                    )
                },
                navigationIcon = {
                    IconButton(onClick = {
                        viewModel.handleBack(cascadeId, inputText)
                        onNavigateBack()
                    }) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back",
                            tint = colors.accentIndigo
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = colors.background,
                    titleContentColor = colors.textPrimary
                )
            )
        },
        containerColor = colors.background,
        modifier = modifier
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
        ) {
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
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .nestedScroll(pullRefreshState.nestedScrollConnection)
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
                                    key = { it.id.ifBlank { "${it.timestamp}_${it.content.hashCode()}" } }
                                ) { msg ->
                                    MessageBubble(
                                        message = msg,
                                        onPlanClick = { uri, title ->
                                            handleFileOrLinkClick(uri, title)
                                        },
                                        urlResolver = { raw -> viewModel.resolveMediaUrl(raw) },
                                        onImageClick = { url, bitmap ->
                                            viewModel.openImageViewer(bitmap = bitmap, url = url)
                                        },
                                        onImageGroupClick = { items, index ->
                                            viewModel.openImageViewer(items = items, initialIndex = index)
                                        },
                                        onUndoClick = { target ->
                                            viewModel.requestUndo(target)
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
                                    Spacer(modifier = Modifier.height(10.dp))
                                }
                            }

                            if (pullRefreshState.verticalOffset > 0 || isRefreshing) {
                                PullToRefreshContainer(
                                    state = pullRefreshState,
                                    modifier = Modifier.align(Alignment.TopCenter),
                                    containerColor = colors.surface,
                                    contentColor = colors.accentIndigo
                                )
                            }
                        }
                    }
                }
            }

            // Floating Cards (InteractionCard, RunningTasksCard, QueuedMessagesCard) 对齐 iOS floatingCards
            val hasFloatingCards = uiState.pendingInteraction != null ||
                    uiState.runningTasks.isNotEmpty() ||
                    uiState.queuedMessages.isNotEmpty()

            if (hasFloatingCards) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 12.dp)
                        .padding(bottom = 6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    uiState.pendingInteraction?.let { interaction ->
                        InteractionCard(
                            interaction = interaction,
                            onApprove = {
                                dismissKeyboard()
                                viewModel.approveInteraction()
                            },
                            onReject = {
                                dismissKeyboard()
                                viewModel.rejectInteraction()
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

                    if (uiState.queuedMessages.isNotEmpty()) {
                        QueuedMessagesCard(
                            items = uiState.queuedMessages,
                            onSendNow = {
                                dismissKeyboard()
                                viewModel.sendQueuedMessageNow(it)
                            },
                            onEdit = { viewModel.editQueuedMessage(it) },
                            onDelete = { viewModel.deleteQueuedMessage(it) },
                            onToggleExpand = { isExpanded ->
                                handleFloatingCardToggle(isExpanded)
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }

            // Bottom Control Area: Divider + Chips + Attached Images + Input Bar
            Surface(
                color = colors.surface,
                shadowElevation = 4.dp,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier.fillMaxWidth()
                ) {
                    HorizontalDivider(
                        color = colors.separator.copy(alpha = 0.5f),
                        thickness = 0.5.dp
                    )

                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 8.dp, bottom = 12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        // Quick Action Chips
                        Box(modifier = Modifier.padding(horizontal = 16.dp)) {
                            QuickActionChips(
                                activeModel = uiState.activeModel,
                                onToggleModel = { viewModel.toggleModel() },
                                onAddImage = {
                                    dismissKeyboard()
                                    photoPickerLauncher.launch("image/*")
                                },
                                onCommitAndPush = { viewModel.insertCommitAndPush() },
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
                                        if (inputText.isNotBlank() || uiState.selectedImages.isNotEmpty()) {
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
                                            .padding(horizontal = 16.dp, vertical = 11.dp),
                                        contentAlignment = Alignment.CenterStart
                                    ) {
                                        if (inputText.isEmpty()) {
                                            Text(
                                                text = if (uiState.isRunning || uiState.isAwaitingResponse) "向队列添加指令..." else "发送对 Agent 的指令...",
                                                color = colors.textMuted,
                                                fontSize = 16.sp,
                                                lineHeight = 22.sp
                                            )
                                        }
                                        innerTextField()
                                    }
                                }
                            )

                            val isRunning = uiState.isRunning || uiState.isAwaitingResponse
                            val isInputBlank = inputText.isBlank()
                            val hasAttachments = uiState.selectedImages.isNotEmpty()

                            if (isRunning && isInputBlank && !hasAttachments) {
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
                            } else {
                                // Send button: 44.dp circle, Apple Indigo with white up arrow, matches iOS send button
                                val isEnabled = !isInputBlank || hasAttachments
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
                        }
                    }
                }
            }
        }
    }

    // Markdown File Viewer Sheet
    uiState.markdownViewerData?.let { viewerData ->
        MarkdownViewerSheet(
            data = viewerData,
            onProceed = { viewModel.proceedFromViewer() },
            onDismiss = { viewModel.closeMarkdownViewer() },
            urlResolver = { raw -> viewModel.resolveMediaUrl(raw) },
            onImageClick = { url -> viewModel.openImageViewer(url = url) }
        )
    }

    // Fullscreen Image Viewer Sheet
    uiState.imageViewerData?.let { viewerData ->
        ImageViewerSheet(
            data = viewerData,
            onDismiss = { viewModel.closeImageViewer() }
        )
    }

    // Document Preview Sheet (PDF, HTML, Office, Text)
    uiState.previewDocumentFile?.let { docFile ->
        DocumentPreviewSheet(
            file = docFile,
            title = uiState.previewDocumentTitle,
            onDismiss = { viewModel.closeDocumentPreview() }
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
