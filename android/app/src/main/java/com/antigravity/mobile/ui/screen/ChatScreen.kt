package com.antigravity.mobile.ui.screen

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
import com.antigravity.mobile.ui.components.InteractionCard
import com.antigravity.mobile.ui.components.MessageBubble
import com.antigravity.mobile.ui.components.ProceedBanner
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
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                items(uiState.messages, key = { it.id.ifBlank { "${it.timestamp}_${it.content.hashCode()}" } }) { msg ->
                    MessageBubble(message = msg)
                }

                // Interactive Decision Card (if pending)
                uiState.pendingInteraction?.let { interaction ->
                    item {
                        InteractionCard(
                            interaction = interaction,
                            onApprove = { viewModel.approveInteraction() },
                            onReject = { viewModel.rejectInteraction() },
                            modifier = Modifier.padding(vertical = 8.dp)
                        )
                    }
                }

                // Proceed Banner (if ready)
                if (uiState.canProceed) {
                    item {
                        ProceedBanner(
                            onProceed = { viewModel.proceedArtifact() },
                            modifier = Modifier.padding(vertical = 8.dp)
                        )
                    }
                }
            }

            // Input Bar
            Surface(
                color = DarkSurface,
                modifier = Modifier.fillMaxWidth()
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedTextField(
                        value = inputText,
                        onValueChange = { viewModel.onInputTextChanged(it) },
                        placeholder = {
                            Text(
                                text = if (uiState.isRunning) "Agent 正在执行，指令将进入排队..." else "向 Antigravity 发送指令...",
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
