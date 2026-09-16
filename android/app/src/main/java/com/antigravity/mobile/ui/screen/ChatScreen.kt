package com.antigravity.mobile.ui.screen

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChatViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    cascadeId: String,
    initialTitle: String,
    viewModel: ChatViewModel,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()
    val inputText by viewModel.inputText.collectAsState()
    val listState = rememberLazyListState()
    val colors = AntigravityTheme.colors

    // Photo picker launcher
    val photoPickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.GetMultipleContents()
    ) { uris ->
        if (uris.isNotEmpty()) {
            // Selected image attachments
        }
    }

    LaunchedEffect(cascadeId) {
        viewModel.initSession(cascadeId, initialTitle)
    }

    // Auto-scroll to bottom on new messages
    LaunchedEffect(uiState.messages.size) {
        if (uiState.messages.isNotEmpty()) {
            listState.animateScrollToItem(uiState.messages.size - 1)
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
            // Message List / Empty State
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
            ) {
                if (uiState.messages.isEmpty() && !uiState.isRunning) {
                    ChatEmptyStateView(title = uiState.title)
                } else {
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
                            MessageBubble(message = msg)
                        }

                        // Active Thinking Animation Card
                        if (uiState.isRunning) {
                            item {
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
                    }
                }
            }

            // Bottom Control Area: Divider + Chips + Input Bar
            Surface(
                color = colors.background,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 14.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    // Quick Action Chips
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

                    // Input Field & iOS Circular Action Button
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.Bottom,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        OutlinedTextField(
                            value = inputText,
                            onValueChange = { viewModel.onInputTextChanged(it) },
                            placeholder = {
                                Text(
                                    text = if (uiState.isRunning) "向队列添加指令..." else "向 Antigravity 发送指令...",
                                    color = colors.textMuted,
                                    fontSize = 15.sp
                                )
                            },
                            maxLines = 5,
                            shape = RoundedCornerShape(22.dp),
                            colors = OutlinedTextFieldDefaults.colors(
                                focusedContainerColor = colors.surface,
                                unfocusedContainerColor = colors.surface,
                                focusedBorderColor = colors.accentIndigo,
                                unfocusedBorderColor = colors.border,
                                focusedTextColor = colors.textPrimary,
                                unfocusedTextColor = colors.textPrimary
                            ),
                            modifier = Modifier
                                .weight(1f)
                                .heightIn(min = 44.dp)
                        )

                        val isRunning = uiState.isRunning
                        val isInputBlank = inputText.isBlank()

                        if (isRunning && isInputBlank) {
                            // Stop button: gray circle with red stop square
                            IconButton(
                                onClick = { viewModel.cancelExecution() },
                                modifier = Modifier
                                    .size(44.dp)
                                    .clip(CircleShape)
                                    .background(colors.surfaceVariant)
                                    .border(0.8.dp, colors.border, CircleShape)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.Stop,
                                    contentDescription = "Stop",
                                    tint = colors.accentRed,
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                        } else {
                            // Send button: 44.dp circle, Apple Indigo with white up arrow
                            val isEnabled = !isInputBlank
                            IconButton(
                                onClick = { viewModel.sendCurrentMessage() },
                                enabled = isEnabled,
                                modifier = Modifier
                                    .size(44.dp)
                                    .clip(CircleShape)
                                    .background(if (isEnabled) colors.accentIndigo else colors.surfaceVariant)
                                    .border(
                                        0.8.dp,
                                        if (isEnabled) colors.accentIndigo else colors.border,
                                        CircleShape
                                    )
                            ) {
                                Icon(
                                    imageVector = Icons.Default.ArrowUpward,
                                    contentDescription = "Send",
                                    tint = if (isEnabled) Color.White else colors.textMuted,
                                    modifier = Modifier.size(20.dp)
                                )
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
