package com.antigravity.mobile.ui.screen

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.*
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
                            modifier = Modifier.weight(1f, fill = false)
                        )

                        // Connection indicator dot
                        val dotColor = when (uiState.connectionStatus) {
                            ConnectionStatus.CONNECTED -> AccentGreen
                            ConnectionStatus.CONNECTING -> AccentYellow
                            else -> AccentRed
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
                            tint = TextPrimary
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = DarkBackground,
                    titleContentColor = TextPrimary
                )
            )
        },
        containerColor = DarkBackground,
        modifier = modifier
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            // Chat Message Stream
            LazyColumn(
                state = listState,
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth(),
                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                items(uiState.messages, key = { it.id.ifBlank { "${it.timestamp}_${it.content.hashCode()}" } }) { msg ->
                    MessageBubble(message = msg)
                }

                // Active Thinking Animation Card
                if (uiState.isRunning) {
                    item {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(8.dp))
                                .background(DarkSurfaceVariant)
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.AutoAwesome,
                                contentDescription = "Thinking",
                                tint = AccentBlue,
                                modifier = Modifier.size(16.dp)
                            )
                            Text(
                                text = "Agent 正在思考与执行...",
                                color = TextSecondary,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }
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

            // Bottom Control Area: Chips + Input Bar
            Surface(
                color = DarkSurface,
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 12.dp, vertical = 8.dp),
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

                    // Input Field & Action Button
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        OutlinedTextField(
                            value = inputText,
                            onValueChange = { viewModel.onInputTextChanged(it) },
                            placeholder = {
                                Text(
                                    text = if (uiState.isRunning) "向队列添加指令..." else "向 Antigravity 发送指令...",
                                    color = TextMuted,
                                    fontSize = 14.sp
                                )
                            },
                            maxLines = 4,
                            shape = RoundedCornerShape(20.dp),
                            colors = OutlinedTextFieldDefaults.colors(
                                focusedContainerColor = DarkBackground,
                                unfocusedContainerColor = DarkBackground,
                                focusedBorderColor = AccentBlue,
                                unfocusedBorderColor = DarkBorder
                            ),
                            modifier = Modifier.weight(1f)
                        )

                        if (uiState.isRunning) {
                            IconButton(
                                onClick = { viewModel.cancelExecution() },
                                modifier = Modifier
                                    .size(44.dp)
                                    .clip(CircleShape)
                                    .background(AccentRed)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.Stop,
                                    contentDescription = "Stop",
                                    tint = TextPrimary
                                )
                            }
                        } else {
                            IconButton(
                                onClick = { viewModel.sendCurrentMessage() },
                                enabled = inputText.isNotBlank(),
                                modifier = Modifier
                                    .size(44.dp)
                                    .clip(CircleShape)
                                    .background(if (inputText.isNotBlank()) AccentBlue else DarkSurfaceVariant)
                            ) {
                                Icon(
                                    imageVector = Icons.AutoMirrored.Filled.Send,
                                    contentDescription = "Send",
                                    tint = if (inputText.isNotBlank()) TextPrimary else TextMuted
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
