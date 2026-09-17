package com.antigravity.mobile.ui.screen

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.material3.pulltorefresh.PullToRefreshContainer
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
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

    val colors = AntigravityTheme.colors
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
        if (!isRefreshing && pullRefreshState.isRefreshing) {
            pullRefreshState.endRefresh()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        text = "Antigravity",
                        fontWeight = FontWeight.Bold,
                        fontSize = 20.sp,
                        color = colors.textPrimary
                    )
                },
                navigationIcon = {
                    IconButton(onClick = { showSettingsSheet = true }) {
                        Icon(
                            imageVector = Icons.Default.Settings,
                            contentDescription = "Settings",
                            tint = colors.accentIndigo
                        )
                    }
                },
                actions = {
                    IconButton(onClick = { showNewConvSheet = true }) {
                        Icon(
                            imageVector = Icons.Default.Add,
                            contentDescription = "New Conversation",
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
            // Quota Bar if available (directly under TopBar, exactly matching iOS layout)
            quotaData?.currentAccount?.let { acc ->
                QuotaStatusBar(
                    account = acc,
                    onTap = { showQuotaSheet = true },
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp)
                )
            }

            // Content List with PullToRefresh and Bottom Floating Search Bar
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .nestedScroll(pullRefreshState.nestedScrollConnection)
                    .clipToBounds()
            ) {
                when (val state = uiState) {
                    is ConversationListUiState.Loading -> {
                        if (state.conversations.isEmpty()) {
                            Box(
                                modifier = Modifier.fillMaxSize(),
                                contentAlignment = Alignment.Center
                            ) {
                                CircularProgressIndicator(color = colors.accentIndigo)
                            }
                        } else {
                            ConversationListContent(
                                conversations = state.conversations,
                                onSelect = onSelectConversation,
                                onRename = { item ->
                                    renamingItem = item
                                    renameText = item.title
                                },
                                onDelete = { item ->
                                    deletingItem = item
                                }
                            )
                        }
                    }
                    is ConversationListUiState.Error -> {
                        Column(
                            modifier = Modifier
                                .fillMaxSize()
                                .padding(32.dp),
                            verticalArrangement = Arrangement.Center,
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            Icon(
                                imageVector = Icons.Default.Warning,
                                contentDescription = "Error",
                                tint = colors.accentOrange,
                                modifier = Modifier.size(48.dp)
                            )
                            Spacer(modifier = Modifier.height(12.dp))
                            Text(
                                text = "无法连接网关",
                                color = colors.textPrimary,
                                fontSize = 17.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            Spacer(modifier = Modifier.height(6.dp))
                            Text(
                                text = state.message,
                                color = colors.textSecondary,
                                fontSize = 13.sp,
                                textAlign = TextAlign.Center
                            )
                            Spacer(modifier = Modifier.height(16.dp))
                            Button(
                                onClick = { viewModel.loadConversations() },
                                colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                            ) {
                                Text("重试连接")
                            }
                        }
                    }
                    is ConversationListUiState.Success -> {
                        if (state.conversations.isEmpty()) {
                            Column(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .padding(32.dp),
                                verticalArrangement = Arrangement.Center,
                                horizontalAlignment = Alignment.CenterHorizontally
                            ) {
                                Icon(
                                    imageVector = if (searchQuery.isEmpty()) Icons.Default.ChatBubbleOutline else Icons.Default.Search,
                                    contentDescription = "Empty",
                                    tint = colors.textMuted,
                                    modifier = Modifier.size(44.dp)
                                )
                                Spacer(modifier = Modifier.height(12.dp))
                                Text(
                                    text = if (searchQuery.isEmpty()) "暂无会话" else "未找到匹配会话",
                                    color = colors.textPrimary,
                                    fontSize = 17.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                                Spacer(modifier = Modifier.height(4.dp))
                                Text(
                                    text = if (searchQuery.isEmpty()) "可点击右上角 + 开启新会话，或下拉刷新同步" else "请尝试其他关键词搜索",
                                    color = colors.textSecondary,
                                    fontSize = 13.sp,
                                    textAlign = TextAlign.Center
                                )
                            }
                        } else {
                            ConversationListContent(
                                conversations = state.conversations,
                                onSelect = onSelectConversation,
                                onRename = { item ->
                                    renamingItem = item
                                    renameText = item.title
                                },
                                onDelete = { item ->
                                    deletingItem = item
                                }
                            )
                        }
                    }
                }

                // Pull to refresh spinner (only shown when actively pulled down or refreshing)
                if (pullRefreshState.verticalOffset > 0 || isRefreshing) {
                    PullToRefreshContainer(
                        state = pullRefreshState,
                        modifier = Modifier.align(Alignment.TopCenter),
                        containerColor = colors.surface,
                        contentColor = colors.accentIndigo
                    )
                }

                // iOS-Style Floating Bottom Search Bar (as specified in session_list.jpg)
                Box(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .fillMaxWidth()
                        .padding(horizontal = 24.dp, vertical = 14.dp)
                        .clip(RoundedCornerShape(26.dp))
                        .background(colors.surface)
                        .border(0.8.dp, colors.border, RoundedCornerShape(26.dp))
                        .padding(horizontal = 16.dp, vertical = 12.dp)
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Search,
                            contentDescription = "Search",
                            tint = colors.textSecondary,
                            modifier = Modifier.size(18.dp)
                        )
                        BasicTextField(
                            value = searchQuery,
                            onValueChange = { viewModel.onSearchQueryChanged(it) },
                            singleLine = true,
                            textStyle = TextStyle(
                                color = colors.textPrimary,
                                fontSize = 15.sp
                            ),
                            cursorBrush = SolidColor(colors.accentIndigo),
                            decorationBox = { innerTextField ->
                                if (searchQuery.isEmpty()) {
                                    Text(
                                        text = "搜索会话或工作区...",
                                        color = colors.textSecondary,
                                        fontSize = 15.sp
                                    )
                                }
                                innerTextField()
                            },
                            modifier = Modifier.weight(1f)
                        )
                        if (searchQuery.isNotEmpty()) {
                            Icon(
                                imageVector = Icons.Default.Close,
                                contentDescription = "Clear",
                                tint = colors.textMuted,
                                modifier = Modifier
                                    .size(18.dp)
                                    .clip(CircleShape)
                                    .combinedClickable(onClick = { viewModel.onSearchQueryChanged("") })
                            )
                        }
                    }
                }
            }
        }
    }

    // Sheets & Dialogs
    if (showQuotaSheet) {
        AccountQuotaSheet(
            quotaData = quotaData,
            isRefreshing = isRefreshingQuota,
            onRefresh = { viewModel.refreshQuotas() },
            onSwitchAccount = { viewModel.switchCockpitAccount(it) },
            onDismiss = { showQuotaSheet = false }
        )
    }

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

    if (showSettingsSheet) {
        SettingsSheet(
            prefs = viewModel.prefs,
            onThemeModeChange = { viewModel.prefs.themeMode = it },
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
            title = { Text("重命名会话", color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = {
                OutlinedTextField(
                    value = renameText,
                    onValueChange = { renameText = it },
                    singleLine = true,
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = colors.surface,
                        unfocusedContainerColor = colors.surface,
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border,
                        focusedTextColor = colors.textPrimary,
                        unfocusedTextColor = colors.textPrimary
                    ),
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
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
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
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.accentRed)
                ) {
                    Text("删除此会话")
                }
            },
            containerColor = colors.surface
        )
    }

    // Delete Confirmation Dialog
    deletingItem?.let { item ->
        AlertDialog(
            onDismissRequest = { deletingItem = null },
            title = { Text("确认删除此会话吗？", color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = { Text("此操作将永久删除会话记录且无法撤销。", color = colors.textSecondary) },
            confirmButton = {
                Button(
                    onClick = {
                        viewModel.deleteConversation(item.id)
                        deletingItem = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("删除")
                }
            },
            dismissButton = {
                TextButton(onClick = { deletingItem = null }) {
                    Text("取消", color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }
}

@Composable
private fun ConversationListContent(
    conversations: List<ConversationItem>,
    onSelect: (cascadeId: String, title: String) -> Unit,
    onRename: (ConversationItem) -> Unit,
    onDelete: (ConversationItem) -> Unit
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(top = 6.dp, bottom = 84.dp, start = 16.dp, end = 16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        items(conversations, key = { it.id }) { conversation ->
            SwipeableConversationCard(
                conversation = conversation,
                onClick = { onSelect(conversation.id, conversation.displayTitle) },
                onLongClick = { onRename(conversation) },
                onDelete = { onDelete(conversation) }
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SwipeableConversationCard(
    conversation: ConversationItem,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    onDelete: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val dismissState = rememberSwipeToDismissBoxState(
        confirmValueChange = { value ->
            if (value == SwipeToDismissBoxValue.EndToStart) {
                onDelete()
                false
            } else {
                false
            }
        }
    )

    SwipeToDismissBox(
        state = dismissState,
        enableDismissFromStartToEnd = false,
        backgroundContent = {
            val isSwiping = dismissState.targetValue == SwipeToDismissBoxValue.EndToStart
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(RoundedCornerShape(14.dp))
                    .background(colors.accentRed),
                contentAlignment = Alignment.CenterEnd
            ) {
                Row(
                    modifier = Modifier.padding(end = 20.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Delete,
                        contentDescription = "Delete",
                        tint = colors.textPrimary,
                        modifier = Modifier.size(22.dp)
                    )
                }
            }
        }
    ) {
        ConversationCard(
            conversation = conversation,
            onClick = onClick,
            onLongClick = onLongClick
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
    val colors = AntigravityTheme.colors

    Card(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
            .combinedClickable(
                onClick = onClick,
                onLongClick = onLongClick
            ),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surface)
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
                Text(
                    text = conversation.displayTitle,
                    color = colors.textPrimary,
                    fontSize = 15.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).padding(end = 8.dp)
                )

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
                        imageVector = if (conversation.isPureChat) Icons.Default.ChatBubbleOutline else Icons.Default.Folder,
                        contentDescription = "Workspace",
                        tint = colors.textMuted,
                        modifier = Modifier.size(13.dp)
                    )
                    Text(
                        text = conversation.workspaceName,
                        color = colors.textSecondary,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                val timeStr = conversation.relativeTimeString
                val metaText = if (timeStr.isNotBlank()) "${conversation.stepCount} 步骤 · $timeStr" else "${conversation.stepCount} 步骤"

                Text(
                    text = metaText,
                    color = colors.textSecondary,
                    fontSize = 12.sp
                )
            }
        }
    }
}
