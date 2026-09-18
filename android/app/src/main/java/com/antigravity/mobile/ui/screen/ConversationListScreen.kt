package com.antigravity.mobile.ui.screen

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.ui.res.painterResource
import com.antigravity.mobile.R
import com.antigravity.mobile.ui.util.rememberHaptic
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.material3.pulltorefresh.PullToRefreshContainer
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import kotlin.math.roundToInt
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ConversationListUiState
import com.antigravity.mobile.ui.viewmodel.ConversationListViewModel

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun ConversationListScreen(
    viewModel: ConversationListViewModel,
    onSelectConversation: (cascadeId: String, title: String, isNew: Boolean, isUnread: Boolean, status: com.antigravity.mobile.data.model.ConversationStatus, lastModifiedTime: String?) -> Unit,
    onNavigateToPair: () -> Unit,
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val searchQuery by viewModel.searchQuery.collectAsStateWithLifecycle()
    val quotaData by viewModel.quotaData.collectAsStateWithLifecycle()
    val isRefreshingQuota by viewModel.isRefreshingQuota.collectAsStateWithLifecycle()
    val projects by viewModel.projects.collectAsStateWithLifecycle()
    val isLoadingProjects by viewModel.isLoadingProjects.collectAsStateWithLifecycle()
    val projectsError by viewModel.projectsError.collectAsStateWithLifecycle()

    var showQuotaSheet by remember { mutableStateOf(false) }
    var showNewConvSheet by remember { mutableStateOf(false) }
    var showSettingsSheet by remember { mutableStateOf(false) }

    var easterEggTapCount by remember { mutableIntStateOf(0) }
    var lastEasterEggTapTime by remember { mutableLongStateOf(0L) }
    var showEasterEgg by remember { mutableStateOf(false) }
    val haptic = rememberHaptic()

    val handleEasterEggTap: () -> Unit = {
        if (!showEasterEgg) {
            val now = System.currentTimeMillis()
            if (now - lastEasterEggTapTime > 2000L) {
                easterEggTapCount = 1
            } else {
                easterEggTapCount++
            }
            lastEasterEggTapTime = now
            haptic.light()

            if (easterEggTapCount >= 10) {
                easterEggTapCount = 0
                haptic.success()
                showEasterEgg = true
            }
        }
    }

    var renamingItem by remember { mutableStateOf<ConversationItem?>(null) }
    var renameText by remember { mutableStateOf("") }
    var deletingItem by remember { mutableStateOf<ConversationItem?>(null) }

    val colors = AntigravityTheme.colors
    val isRefreshing by viewModel.isRefreshing.collectAsStateWithLifecycle()
    val pullRefreshState = rememberPullToRefreshState()
    val listState = rememberLazyListState()
    val coroutineScope = rememberCoroutineScope()

    LaunchedEffect(searchQuery) {
        if (listState.firstVisibleItemIndex > 0) {
            listState.scrollToItem(0)
        }
    }

    DisposableEffect(Unit) {
        viewModel.startAutoRefresh()
        onDispose {
            viewModel.stopAutoRefresh()
        }
    }

    LaunchedEffect(Unit) {
        if (projects.isEmpty()) {
            viewModel.loadProjects()
        }
    }

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

    val density = LocalDensity.current
    val statusBarTop = WindowInsets.statusBars.asPaddingValues().calculateTopPadding()
    val topBarHeight = statusBarTop + 52.dp
    val scrollThresholdPx = with(density) { 48.dp.toPx() }

    val scrollProgress by remember {
        derivedStateOf {
            if (listState.firstVisibleItemIndex > 0) {
                1f
            } else {
                val offset = listState.firstVisibleItemScrollOffset
                (offset / scrollThresholdPx).coerceIn(0f, 1f)
            }
        }
    }

    val inlineTitleAlpha = remember(scrollProgress) {
        if (scrollProgress < 0.25f) 0f else ((scrollProgress - 0.25f) / 0.75f).coerceIn(0f, 1f)
    }

    val largeTitleAlpha = remember(scrollProgress) {
        (1f - scrollProgress * 1.5f).coerceIn(0f, 1f)
    }

    val topBarDividerAlpha = remember(scrollProgress) {
        (scrollProgress * 1.2f).coerceIn(0f, 1f)
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(colors.background)
    ) {
        // Content Area with PullToRefresh and Bottom Floating Search Bar
        Box(
            modifier = Modifier
                .fillMaxSize()
                .nestedScroll(pullRefreshState.nestedScrollConnection)
                .clipToBounds()
        ) {
            when (val state = uiState) {
                is ConversationListUiState.Loading -> {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(top = topBarHeight),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator(color = colors.accentIndigo)
                    }
                }
                is ConversationListUiState.Error -> {
                    if (!viewModel.prefs.isPaired()) {
                        LaunchedEffect(Unit) {
                            onNavigateToPair()
                        }
                    } else {
                        Column(
                            modifier = Modifier
                                .fillMaxSize()
                                .padding(top = topBarHeight)
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
                }
                is ConversationListUiState.Success -> {
                    if (!viewModel.prefs.isPaired()) {
                        LaunchedEffect(Unit) {
                            onNavigateToPair()
                        }
                    } else {
                        ConversationListContent(
                            conversations = state.conversations,
                            listState = listState,
                            quotaData = quotaData,
                            searchQuery = searchQuery,
                            largeTitleAlpha = largeTitleAlpha,
                            topPadding = topBarHeight,
                            onEasterEggTap = handleEasterEggTap,
                            onQuotaTap = { showQuotaSheet = true },
                            onRefresh = { viewModel.loadConversations() },
                            hasDraftFor = { viewModel.prefs.hasDraft(it) },
                            onSelect = onSelectConversation,
                            onRename = { item ->
                                if (!item.isDraft) {
                                    renamingItem = item
                                    renameText = item.title
                                }
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
                    modifier = Modifier
                        .align(Alignment.TopCenter)
                        .padding(top = topBarHeight),
                    containerColor = colors.surface,
                    contentColor = colors.accentIndigo
                )
            }

            // iOS-Style Floating Bottom Search Bar (as specified in session_list.jpg)
            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .navigationBarsPadding()
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

        // 2. Sticky iOS-Style Top Bar (Header)
        Box(
            modifier = Modifier
                .align(Alignment.TopCenter)
                .fillMaxWidth()
                .background(colors.background)
                .statusBarsPadding()
                .height(52.dp)
        ) {
            // Settings Button (Left)
            Box(
                modifier = Modifier
                    .align(Alignment.CenterStart)
                    .padding(start = 12.dp)
                    .size(44.dp)
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        onClick = { showSettingsSheet = true }
                    ),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(38.dp)
                        .clip(CircleShape)
                        .background(colors.surface)
                        .border(0.8.dp, colors.border, CircleShape),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Settings,
                        contentDescription = "Settings",
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(20.dp)
                    )
                }
            }

            // Centered Title (Transitions in when scrolling up)
            Text(
                text = "Multigravity",
                fontWeight = FontWeight.Bold,
                fontSize = 18.sp,
                color = colors.textPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier
                    .align(Alignment.Center)
                    .graphicsLayer {
                        alpha = inlineTitleAlpha
                        translationY = (1f - inlineTitleAlpha) * 8.dp.toPx()
                    }
                    .clip(RoundedCornerShape(8.dp))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        enabled = inlineTitleAlpha > 0.5f,
                        onClick = handleEasterEggTap
                    )
            )

            // New Conversation Button (Right)
            Box(
                modifier = Modifier
                    .align(Alignment.CenterEnd)
                    .padding(end = 12.dp)
                    .size(44.dp)
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        onClick = {
                            viewModel.loadProjects()
                            showNewConvSheet = true
                        }
                    ),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(38.dp)
                        .clip(CircleShape)
                        .background(colors.surface)
                        .border(0.8.dp, colors.border, CircleShape),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Add,
                        contentDescription = "New Conversation",
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(20.dp)
                    )
                }
            }

            // Bottom hairline border when scrolled
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(0.6.dp)
                    .align(Alignment.BottomCenter)
                    .background(colors.border.copy(alpha = topBarDividerAlpha))
            )
        }
    }

    // Sheets & Dialogs
    if (showEasterEgg) {
        EasterEggDialog(
            onDismiss = { showEasterEgg = false }
        )
    }

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
            errorMessage = projectsError,
            onRefreshProjects = { viewModel.loadProjects() },
            onSelectProject = { project ->
                showNewConvSheet = false
                val draftSession = viewModel.createLocalDraftSession(project)
                val title = if (project.isPureChat) "新对话" else project.name
                onSelectConversation(draftSession.id, title, true, false, com.antigravity.mobile.data.model.ConversationStatus.IDLE, null)
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
    listState: LazyListState,
    quotaData: CockpitQuotaResponse?,
    searchQuery: String,
    largeTitleAlpha: Float,
    topPadding: Dp,
    onEasterEggTap: () -> Unit,
    onQuotaTap: () -> Unit,
    onRefresh: () -> Unit,
    hasDraftFor: (String) -> Boolean,
    onSelect: (cascadeId: String, title: String, isNew: Boolean, isUnread: Boolean, status: com.antigravity.mobile.data.model.ConversationStatus, lastModifiedTime: String?) -> Unit,
    onRename: (ConversationItem) -> Unit,
    onDelete: (ConversationItem) -> Unit
) {
    val colors = AntigravityTheme.colors

    LazyColumn(
        state = listState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(
            top = topPadding + 4.dp,
            bottom = 84.dp,
            start = 16.dp,
            end = 16.dp
        ),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        // Large Title Header (matches iOS large title below settings button)
        item(key = "header_large_title") {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 2.dp, bottom = 2.dp)
                    .graphicsLayer {
                        alpha = largeTitleAlpha
                    }
            ) {
                Text(
                    text = "Multigravity",
                    fontSize = 34.sp,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = (-0.5).sp,
                    color = colors.textPrimary,
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null,
                            enabled = largeTitleAlpha > 0.5f,
                            onClick = onEasterEggTap
                        )
                )
            }
        }

        // Quota Status Bar (matches iOS QuotaStatusBarView directly below large title)
        quotaData?.currentAccount?.let { acc ->
            if (acc.gemini5h != null) {
                item(key = "header_quota") {
                    QuotaStatusBar(
                        account = acc,
                        onTap = onQuotaTap,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }
        }

        if (conversations.isEmpty()) {
            item(key = "empty_placeholder") {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 48.dp, horizontal = 16.dp),
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
                    if (searchQuery.isEmpty()) {
                        Spacer(modifier = Modifier.height(16.dp))
                        Button(
                            onClick = onRefresh,
                            colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                        ) {
                            Text("刷新列表")
                        }
                    }
                }
            }
        } else {
            items(conversations, key = { it.id }) { conversation ->
                val hasDraft = conversation.isDraft || hasDraftFor(conversation.id)
                SwipeableConversationCard(
                    conversation = conversation,
                    hasDraft = hasDraft,
                    onClick = { onSelect(conversation.id, conversation.displayTitle, false, conversation.isUnread, conversation.status, conversation.lastModifiedTime) },
                    onLongClick = { onRename(conversation) },
                    onDelete = { onDelete(conversation) }
                )
            }
        }
    }
}

@Composable
private fun SwipeableConversationCard(
    conversation: ConversationItem,
    hasDraft: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    onDelete: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val coroutineScope = rememberCoroutineScope()
    val maxRevealPx = with(density) { 72.dp.toPx() }
    val offsetX = remember { Animatable(0f) }
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp)),
        contentAlignment = Alignment.CenterEnd
    ) {
        // Background: iOS-style circular red delete button (appears on transparent background)
        Box(
            modifier = Modifier.matchParentSize(),
            contentAlignment = Alignment.CenterEnd
        ) {
            Box(
                modifier = Modifier
                    .width(72.dp)
                    .fillMaxHeight(),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(40.dp)
                        .clip(CircleShape)
                        .background(colors.accentRed)
                        .clickable {
                            haptic.medium()
                            coroutineScope.launch {
                                offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                            }
                            onDelete()
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Delete,
                        contentDescription = "Delete",
                        tint = Color.White,
                        modifier = Modifier.size(20.dp)
                    )
                }
            }
        }

        // Foreground Card
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .offset { IntOffset(offsetX.value.roundToInt(), 0) }
                .draggable(
                    state = rememberDraggableState { delta ->
                        coroutineScope.launch {
                            val newOffset = (offsetX.value + delta).coerceIn(-maxRevealPx, 0f)
                            offsetX.snapTo(newOffset)
                        }
                    },
                    orientation = Orientation.Horizontal,
                    onDragStopped = { velocity ->
                        coroutineScope.launch {
                            val targetOffset = if (velocity < -500f || offsetX.value < -maxRevealPx / 2) {
                                haptic.light()
                                -maxRevealPx
                            } else {
                                0f
                            }
                            offsetX.animateTo(targetOffset, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    }
                )
        ) {
            ConversationCard(
                conversation = conversation,
                hasDraft = hasDraft,
                onClick = {
                    if (offsetX.value != 0f) {
                        coroutineScope.launch {
                            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    } else {
                        haptic.light()
                        onClick()
                    }
                },
                onLongClick = {
                    if (!conversation.isDraft) {
                        haptic.longPress()
                        onLongClick()
                    }
                }
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ConversationCard(
    conversation: ConversationItem,
    hasDraft: Boolean = false,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors

    Card(
        modifier = modifier
            .fillMaxWidth()
            .shadow(
                elevation = 1.5.dp,
                shape = RoundedCornerShape(14.dp),
                ambientColor = Color.Black.copy(alpha = 0.04f),
                spotColor = Color.Black.copy(alpha = 0.08f)
            )
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
                } else if (hasDraft || conversation.isDraft) {
                    DraftBadge()
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
                val metaText = if (conversation.isDraft) {
                    if (timeStr.isNotBlank()) "草稿 · $timeStr" else "草稿"
                } else {
                    if (timeStr.isNotBlank()) "${conversation.stepCount} 步骤 · $timeStr" else "${conversation.stepCount} 步骤"
                }

                Text(
                    text = metaText,
                    color = colors.textSecondary,
                    fontSize = 12.sp
                )
            }
        }
    }
}
