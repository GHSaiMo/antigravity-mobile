package com.antigravity.mobile.ui.screen

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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import java.net.URLDecoder

import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChatViewModel
import kotlinx.coroutines.delay

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    cascadeId: String,
    initialTitle: String,
    viewModel: ChatViewModel,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    isNewConversation: Boolean = false
) {
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val focusRequester = remember { FocusRequester() }
    val keyboardController = LocalSoftwareKeyboardController.current
    val uiState by viewModel.uiState.collectAsState()
    val inputText by viewModel.inputText.collectAsState()
    val scrollToBottomTrigger by viewModel.scrollToBottomTrigger.collectAsState()
    val listState = rememberLazyListState()
    val colors = AntigravityTheme.colors
    val shouldShowThinkingBubble = uiState.isAwaitingResponse || uiState.isRunning

    val isRefreshing by viewModel.isRefreshing.collectAsState()
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

    LaunchedEffect(cascadeId, isNewConversation) {
        viewModel.initSession(cascadeId, initialTitle, isNewConversation)
        // Automatically focus the input field and pop up soft keyboard on session entry
        delay(250)
        try {
            focusRequester.requestFocus()
            keyboardController?.show()
        } catch (_: Exception) {}
    }

    // Auto-scroll and bounce down to bottom on new messages, thinking state, or explicit triggers
    LaunchedEffect(uiState.messages.size, shouldShowThinkingBubble, scrollToBottomTrigger) {
        delay(25)
        val totalCount = listState.layoutInfo.totalItemsCount
        if (totalCount > 0) {
            listState.animateScrollToItem(totalCount - 1)
        }
        delay(60)
        val finalTotal = listState.layoutInfo.totalItemsCount
        if (finalTotal > 0) {
            listState.animateScrollToItem(finalTotal - 1)
        }
    }

    val handleFileOrLinkClick: (String, String) -> Unit = { uri, title ->
        val clean = uri.trim()
        val lower = clean.lowercase()
        val rawName = title.ifEmpty { clean.substringAfterLast('/') }
        val decodedFileName = try {
            URLDecoder.decode(rawName, "UTF-8")
        } catch (_: Exception) {
            rawName
        }

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
        } else {
            val previewExtensions = listOf(
                ".pdf", ".pptx", ".ppt", ".docx", ".doc", ".xlsx", ".xls",
                ".html", ".htm", ".txt", ".json", ".csv", ".log", ".xml",
                ".yaml", ".yml", ".py", ".js", ".ts", ".kt", ".swift", ".sh"
            )
            val isPreviewable = previewExtensions.any { ext ->
                lower.endsWith(ext) || lower.contains("$ext?") || lower.contains("$ext#")
            } || clean.startsWith("file://") || clean.startsWith("/")

            if (isPreviewable) {
                haptic.medium()
                viewModel.downloadAndPreviewDocument(clean, decodedFileName)
            } else if (clean.startsWith("http://") || clean.startsWith("https://")) {
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
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text(
                            text = uiState.title,
                            fontWeight = FontWeight.SemiBold,
                            fontSize = 16.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            color = colors.textPrimary,
                            modifier = Modifier.weight(1f, fill = false)
                        )

                        // Connection indicator dot
                        val dotColor = when (uiState.connectionStatus) {
                            ConnectionStatus.CONNECTED -> colors.accentGreen
                            ConnectionStatus.CONNECTING -> colors.accentYellow
                            else -> colors.accentRed
                        }
                        Box(
                            modifier = Modifier
                                .size(8.dp)
                                .clip(CircleShape)
                                .background(dotColor)
                        )
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onNavigateBack) {
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
                                        }
                                    )
                                }

                                // Active Thinking Animation Card
                                if (shouldShowThinkingBubble) {
                                    item(key = "agent_thinking_bubble") {
                                        AgentThinkingBubble()
                                    }
                                }

                                // Active Running Tasks Card
                                if (uiState.runningTasks.isNotEmpty()) {
                                    item {
                                        RunningTasksCard(
                                            tasks = uiState.runningTasks,
                                            onStopTask = { viewModel.stopTask(it) },
                                            modifier = Modifier.padding(vertical = 4.dp)
                                        )
                                    }
                                }

                                // Queued Messages Panel
                                if (uiState.queuedMessages.isNotEmpty()) {
                                    item {
                                        QueuedMessagesCard(
                                            items = uiState.queuedMessages,
                                            onSendNow = { viewModel.sendQueuedMessageNow(it) },
                                            onEdit = { viewModel.editQueuedMessage(it) },
                                            onDelete = { viewModel.deleteQueuedMessage(it) },
                                            modifier = Modifier.padding(vertical = 4.dp)
                                        )
                                    }
                                }

                                // Interactive Decision Card (if pending)
                                uiState.pendingInteraction?.let { interaction ->
                                    item {
                                        InteractionCard(
                                            interaction = interaction,
                                            onApprove = { viewModel.approveInteraction() },
                                            onReject = { viewModel.rejectInteraction() },
                                            modifier = Modifier.padding(vertical = 6.dp)
                                        )
                                    }
                                }

                                // Proceed Banner (if ready)
                                if (uiState.canProceed) {
                                    item {
                                        ProceedBanner(
                                            onProceed = {
                                                val planUri = uiState.proceedArtifactUri ?: "implementation_plan.md"
                                                viewModel.openMarkdownViewer(planUri, "实施方案 (Implementation Plan)")
                                            },
                                            modifier = Modifier.padding(vertical = 6.dp)
                                        )
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
                                onAddImage = { photoPickerLauncher.launch("image/*") },
                                onCommitAndPush = { viewModel.insertCommitAndPush() },
                                showContinue = uiState.isLatestMessageError,
                                onContinue = { viewModel.handleContinue() },
                                showProceed = uiState.canProceed,
                                onProceed = {
                                    val planUri = uiState.proceedArtifactUri ?: "implementation_plan.md"
                                    viewModel.openMarkdownViewer(planUri, "实施方案 (Implementation Plan)")
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
                                    .heightIn(min = 44.dp),
                                textStyle = TextStyle(
                                    color = colors.textPrimary,
                                    fontSize = 16.sp,
                                    lineHeight = 22.sp
                                ),
                                cursorBrush = SolidColor(colors.accentIndigo),
                                maxLines = 5,
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
                                        .clickable { viewModel.cancelExecution() },
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
                                        .clickable(enabled = isEnabled) { viewModel.sendCurrentMessage() },
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
            onDismiss = { viewModel.closeMarkdownViewer() }
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
            text = "已连接工作区，在下方输入指令开启对话",
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
