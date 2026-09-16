package com.antigravity.mobile.ui.screen

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.*
import com.antigravity.mobile.ui.viewmodel.ConversationListUiState
import com.antigravity.mobile.ui.viewmodel.ConversationListViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationListScreen(
    viewModel: ConversationListViewModel,
    onSelectConversation: (cascadeId: String, title: String) -> Unit,
    onNavigateToPair: () -> Unit,
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()
    val searchQuery by viewModel.searchQuery.collectAsState()
    val quotaData by viewModel.quotaData.collectAsState()
    val isRefreshingQuota by viewModel.isRefreshingQuota.collectAsState()
    val projects by viewModel.projects.collectAsState()
    val isLoadingProjects by viewModel.isLoadingProjects.collectAsState()

    var showQuotaSheet by remember { mutableStateOf(false) }
    var showNewConvSheet by remember { mutableStateOf(false) }
    var showSettingsSheet by remember { mutableStateOf(false) }

    var renamingItem by remember { mutableStateOf<ConversationItem?>(null) }
    var renameText by remember { mutableStateOf("") }
    var deletingItem by remember { mutableStateOf<ConversationItem?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        text = "Antigravity",
                        fontWeight = FontWeight.Bold,
                        fontSize = 20.sp
                    )
                },
                navigationIcon = {
                    IconButton(onClick = { showSettingsSheet = true }) {
                        Icon(
                            imageVector = Icons.Default.Settings,
                            contentDescription = "Settings",
                            tint = TextPrimary
                        )
                    }
                },
                actions = {
                    IconButton(onClick = { showNewConvSheet = true }) {
                        Icon(
                            imageVector = Icons.Default.Add,
                            contentDescription = "New Cascade",
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
            // Cockpit 5h Status Bar
            quotaData?.let { qd ->
                QuotaStatusBar(
                    account = qd.currentAccount,
                    onTap = { showQuotaSheet = true },
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp)
                )
            }

            // Search Bar
            OutlinedTextField(
                value = searchQuery,
                onValueChange = { viewModel.onSearchQueryChanged(it) },
                placeholder = { Text("搜索会话或工作区...") },
                leadingIcon = {
                    Icon(
                        imageVector = Icons.Default.Search,
                        contentDescription = "Search",
                        tint = TextSecondary
                    )
                },
                singleLine = true,
                shape = RoundedCornerShape(20.dp),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedContainerColor = DarkSurface,
                    unfocusedContainerColor = DarkSurface,
                    focusedBorderColor = AccentBlue,
                    unfocusedBorderColor = DarkBorder
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 6.dp)
            )

            // Content List
            when (val state = uiState) {
                is ConversationListUiState.Loading -> {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator(color = AccentBlue)
                    }
                }
                is ConversationListUiState.Error -> {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(24.dp),
                        verticalArrangement = Arrangement.Center,
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        Text(
                            text = state.message,
                            color = AccentRed,
                            fontSize = 14.sp,
                            modifier = Modifier.padding(bottom = 12.dp)
                        )
                        Button(
                            onClick = { viewModel.loadConversations() },
                            colors = ButtonDefaults.buttonColors(containerColor = AccentBlue)
                        ) {
                            Text("重试连接")
                        }
                    }
                }
                is ConversationListUiState.Success -> {
                    if (state.conversations.isEmpty()) {
                        Box(
                            modifier = Modifier.fillMaxSize(),
                            contentAlignment = Alignment.Center
                        ) {
                            Text(
                                text = "暂无活跃会话，点击右上角 ➕ 发起新对话",
                                color = TextMuted,
                                fontSize = 14.sp
                            )
                        }
                    } else {
                        LazyColumn(
                            modifier = Modifier.fillMaxSize(),
                            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 6.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            items(state.conversations, key = { it.id }) { conversation ->
                                ConversationCard(
                                    conversation = conversation,
                                    onClick = {
                                        onSelectConversation(conversation.id, conversation.displayTitle)
                                    },
                                    onLongClick = {
                                        renamingItem = conversation
                                        renameText = conversation.title
                                    }
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    // Account Quota Sheet
    if (showQuotaSheet) {
        AccountQuotaSheet(
            quotaData = quotaData,
            isRefreshing = isRefreshingQuota,
            onRefresh = { viewModel.refreshQuotas() },
            onSwitchAccount = { viewModel.switchCockpitAccount(it) },
            onDismiss = { showQuotaSheet = false }
        )
    }

    // New Conversation Sheet
    if (showNewConvSheet) {
        NewConversationSheet(
            projects = projects,
            isLoading = isLoadingProjects,
            onSelectProject = { project, prompt, model ->
                showNewConvSheet = false
                viewModel.createConversation(project, prompt, model) { cascadeId ->
                    onSelectConversation(cascadeId, project.name)
                }
            },
            onDismiss = { showNewConvSheet = false }
        )
    }

    // Settings Sheet
    if (showSettingsSheet) {
        SettingsSheet(
            gatewayUrl = viewModel.prefs.gatewayBaseUrl ?: "未配置",
            deviceId = viewModel.prefs.deviceId ?: "未知",
            deviceToken = viewModel.prefs.deviceToken ?: "未生成",
            onUnpair = {
                showSettingsSheet = false
                onNavigateToPair()
            },
            onDismiss = { showSettingsSheet = false }
        )
    }

    // Rename Dialog
    renamingItem?.let { item ->
        AlertDialog(
            onDismissRequest = { renamingItem = null },
            title = { Text("重命名会话") },
            text = {
                OutlinedTextField(
                    value = renameText,
                    onValueChange = { renameText = it },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        if (renameText.isNotBlank()) {
                            viewModel.renameConversation(item.id, renameText)
                        }
                        renamingItem = null
                    }
                ) {
                    Text("保存")
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        deletingItem = item
                        renamingItem = null
                    },
                    colors = ButtonDefaults.textButtonColors(contentColor = AccentRed)
                ) {
                    Text("删除此会话")
                }
            }
        )
    }

    // Delete Confirmation Dialog
    deletingItem?.let { item ->
        AlertDialog(
            onDismissRequest = { deletingItem = null },
            title = { Text("确认删除会话？") },
            text = { Text("此操作将从 Antigravity 工作区永久移除会话记录及历史轨迹。") },
            confirmButton = {
                Button(
                    onClick = {
                        viewModel.deleteConversation(item.id)
                        deletingItem = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = AccentRed)
                ) {
                    Text("删除")
                }
            },
            dismissButton = {
                TextButton(onClick = { deletingItem = null }) {
                    Text("取消")
                }
            }
        )
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ConversationCard(
    conversation: ConversationItem,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Card(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .combinedClickable(
                onClick = onClick,
                onLongClick = onLongClick
            ),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = DarkSurface)
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    Text(
                        text = conversation.displayTitle,
                        color = TextPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                if (conversation.status.isRunning || conversation.status.needsAction || conversation.status.isError) {
                    StatusBadge(status = conversation.status)
                } else if (conversation.isUnread) {
                    UnreadDot()
                }
            }

            // Workspace & Step counts & Relative Time
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(5.dp),
                    modifier = Modifier.weight(1f, fill = false)
                ) {
                    Icon(
                        imageVector = Icons.Default.Folder,
                        contentDescription = "Workspace",
                        tint = TextMuted,
                        modifier = Modifier.size(13.dp)
                    )
                    Text(
                        text = conversation.workspaceName,
                        color = TextSecondary,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                val timeStr = conversation.relativeTimeString
                val metaText = if (timeStr.isNotBlank()) "${conversation.stepCount} 步骤 · $timeStr" else "${conversation.stepCount} 步骤"

                Text(
                    text = metaText,
                    color = TextMuted,
                    fontSize = 12.sp
                )
            }
        }
    }
}
