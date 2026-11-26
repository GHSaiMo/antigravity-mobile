package com.antigravity.mobile.ui.screen

import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.loc
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalHapticFeedback
import com.antigravity.mobile.data.model.GatewayProfile
import com.antigravity.mobile.data.service.GatewayProfileStore
import com.antigravity.mobile.data.service.GatewaySwitcher
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
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.AppUpdateInstaller
import com.antigravity.mobile.data.service.AppUpdateState
import com.antigravity.mobile.data.service.cloneGitHubRepo
import com.antigravity.mobile.data.service.createProjectWorkspace
import com.antigravity.mobile.data.service.createWorktreeSession
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
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalTextToolbar
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import kotlinx.coroutines.launch
import kotlin.math.roundToInt
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.ui.components.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ConversationListUiState
import com.antigravity.mobile.ui.viewmodel.ConversationListViewModel
import com.antigravity.mobile.ui.viewmodel.WorkspaceFilter
import com.antigravity.mobile.ui.viewmodel.WorkspaceFilterOption

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun ConversationListScreen(
    viewModel: ConversationListViewModel,
    onSelectConversation: (cascadeId: String, title: String, isNew: Boolean, isUnread: Boolean, status: com.antigravity.mobile.data.model.ConversationStatus, lastModifiedTime: String?) -> Unit,
    onNavigateToPair: () -> Unit,
    modifier: Modifier = Modifier,
    dismissModalsTrigger: Int = 0,
    openQuotaRequested: Boolean = false,
    onConsumeOpenQuota: () -> Unit = {},
    onSelectSideQuestionFromSearch: ((parentId: String, sideId: String, sideTitle: String, lastModifiedTime: String?) -> Unit)? = null,
    onAddGateway: () -> Unit = {}
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val searchQuery by viewModel.searchQuery.collectAsStateWithLifecycle()
    val expandedBranchParentIds by viewModel.expandedBranchParentIds.collectAsStateWithLifecycle()
    val searchCollapsedBranchParentIds by viewModel.searchCollapsedBranchParentIds.collectAsStateWithLifecycle()
    val manuallyCollapsedActionBranchParentIds by viewModel.manuallyCollapsedActionBranchParentIds.collectAsStateWithLifecycle()
    val selectedWorkspaceFilter by viewModel.selectedWorkspaceFilter.collectAsStateWithLifecycle()
    val workspaceFilterOptions by viewModel.workspaceFilterOptions.collectAsStateWithLifecycle()
    val totalVisibleCount by viewModel.totalVisibleCount.collectAsStateWithLifecycle()
    val archivedCount by viewModel.archivedCount.collectAsStateWithLifecycle()
    val workspaceTotalCount by viewModel.workspaceTotalCount.collectAsStateWithLifecycle()
    val hiddenPendingApprovalCount by viewModel.hiddenPendingApprovalCount.collectAsStateWithLifecycle()
    val contentResults by viewModel.contentResults.collectAsStateWithLifecycle()
    val compat by com.antigravity.mobile.data.model.GatewayCompatStore.state.collectAsStateWithLifecycle()
    val linkSnapshot by viewModel.linkSnapshot.collectAsStateWithLifecycle()
    val isSearchingContent by viewModel.isSearchingContent.collectAsStateWithLifecycle()
    val quotaData by viewModel.quotaData.collectAsStateWithLifecycle()
    val isRefreshingQuota by viewModel.isRefreshingQuota.collectAsStateWithLifecycle()
    val tokenHistory by viewModel.tokenHistory.collectAsStateWithLifecycle()
    val isLoadingTokenHistory by viewModel.isLoadingTokenHistory.collectAsStateWithLifecycle()
    val globalBackgroundTasks by viewModel.globalBackgroundTasks.collectAsStateWithLifecycle()
    val projects by viewModel.projects.collectAsStateWithLifecycle()
    val isLoadingProjects by viewModel.isLoadingProjects.collectAsStateWithLifecycle()
    val projectsError by viewModel.projectsError.collectAsStateWithLifecycle()
    val showWeeklyPaceCursor by viewModel.prefs.weeklyPaceCursorFlow.collectAsStateWithLifecycle()
    val selectedQuotaBucket by viewModel.prefs.selectedQuotaBucketFlow.collectAsStateWithLifecycle()
    val isEmailMasked by viewModel.prefs.cockpitEmailMaskedFlow.collectAsStateWithLifecycle()
    val textToolbar = LocalTextToolbar.current
    val screenContext = LocalContext.current
    val appUpdateState by AppUpdateInstaller.state.collectAsStateWithLifecycle()
    val hasPendingAppUpdate = AppUpdateInstaller.isSupported && (
        appUpdateState is AppUpdateState.Available ||
            appUpdateState is AppUpdateState.ReadyToInstall ||
            appUpdateState is AppUpdateState.NeedsInstallPermission
        )
    LaunchedEffect(Unit) {
        textToolbar.hide()
        if (AppUpdateInstaller.isSupported) {
            AppUpdateInstaller.checkInBackgroundIfNeeded(screenContext)
        }
    }

    var showQuotaSheet by remember { mutableStateOf(false) }
    var showBackgroundTasksSheet by remember { mutableStateOf(false) }
    var showNewConvSheet by remember { mutableStateOf(false) }
    var showSettingsSheet by remember { mutableStateOf(false) }
    val p3Scope = rememberCoroutineScope()

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
    var pinningItem by remember { mutableStateOf<ConversationItem?>(null) }
    var archivingItem by remember { mutableStateOf<ConversationItem?>(null) }
    var longPressedItem by remember { mutableStateOf<ConversationItem?>(null) }
    var swipedConversationId by remember { mutableStateOf<String?>(null) }
    var editingAliasProject by remember { mutableStateOf<ProjectItem?>(null) }
    var aliasInputText by remember { mutableStateOf("") }

    val gwStore = remember { GatewayProfileStore.sharedOrNull() }
    val emptyProfilesFlow = remember { kotlinx.coroutines.flow.MutableStateFlow<List<GatewayProfile>>(emptyList()) }
    val emptyActiveIdFlow = remember { kotlinx.coroutines.flow.MutableStateFlow<String?>(null) }
    val emptySummariesFlow = remember { kotlinx.coroutines.flow.MutableStateFlow<Map<String, com.antigravity.mobile.data.model.GatewaySummarySnapshot>>(emptyMap()) }
    val gatewayProfiles by (gwStore?.profilesFlow ?: emptyProfilesFlow).collectAsStateWithLifecycle()
    val activeGatewayId by (gwStore?.activeProfileIdFlow ?: emptyActiveIdFlow).collectAsStateWithLifecycle()
    val gatewaySummaries by (gwStore?.summariesFlow ?: emptySummariesFlow).collectAsStateWithLifecycle()
    val switchBannerText by GatewaySwitcher.switchBannerText.collectAsStateWithLifecycle()
    val gatewayPickerState = remember { GatewaySlidePickerState() }
    val composeHaptic = LocalHapticFeedback.current
    var inlineTitleBounds by remember { mutableStateOf(Rect.Zero) }
    var largeTitleBounds by remember { mutableStateOf(Rect.Zero) }
    var inlineTitleDragOffsetPx by remember { mutableFloatStateOf(0f) }
    var largeTitleDragOffsetPx by remember { mutableFloatStateOf(0f) }
    val animatedInlineDragX by animateFloatAsState(
        targetValue = inlineTitleDragOffsetPx,
        animationSpec = spring(dampingRatio = 0.72f, stiffness = Spring.StiffnessMedium),
        label = "inlineTitleDragX"
    )
    val animatedLargeDragX by animateFloatAsState(
        targetValue = largeTitleDragOffsetPx,
        animationSpec = spring(dampingRatio = 0.72f, stiffness = Spring.StiffnessMedium),
        label = "largeTitleDragX"
    )
    var renamingGateway by remember { mutableStateOf<GatewayProfile?>(null) }
    var renameGatewayText by remember { mutableStateOf("") }
    var removingGateway by remember { mutableStateOf<GatewayProfile?>(null) }

    val activeGateway = remember(gatewayProfiles, activeGatewayId) {
        gatewayProfiles.firstOrNull { it.id == activeGatewayId } ?: gatewayProfiles.firstOrNull()
    }
    val hasMultipleGateways = gatewayProfiles.size > 1
    val hasInactivePendingAction = remember(gatewayProfiles, activeGatewayId, gatewaySummaries) {
        val aid = activeGateway?.id
        gatewayProfiles.size > 1 && gatewaySummaries.any { (pid, snap) ->
            pid != aid && snap.actionCount > 0
        }
    }

    val handleSelectGatewayProfile: (GatewayProfile) -> Unit = { profile ->
        val convs = (uiState as? ConversationListUiState.Success)?.conversations
        GatewaySwitcher.switchTo(
            profileId = profile.id,
            currentConversations = convs,
            showToast = true
        )
    }

    val handleHorizontalCycleGateway: (Int) -> Unit = { step ->
        swipedConversationId = null
        val convs = (uiState as? ConversationListUiState.Success)?.conversations
        GatewaySwitcher.cycle(
            step = step,
            currentConversations = convs,
            showToast = true
        )
    }

    val handleHeroLongPressOpened: () -> Unit = {
        swipedConversationId = null
        val convs = (uiState as? ConversationListUiState.Success)?.conversations
        if (convs != null) {
            gwStore?.updateActiveSummary(convs)
        }
        gwStore?.refreshInactiveSummaries(force = false, minIntervalMs = 8_000L)
    }

    val density = LocalDensity.current
    val handleCollapsedTitleTap: () -> Unit = {
        if (hasMultipleGateways && viewModel.prefs.isPaired()) {
            if (gatewayPickerState.isPresented) {
                gatewayPickerState.dismiss()
            } else {
                haptic.light()
                handleHeroLongPressOpened()
                val anchor = if (inlineTitleBounds != Rect.Zero) {
                    inlineTitleBounds
                } else {
                    val cx = gatewayPickerState.containerSize.width / 2f
                    val topPx = with(density) { 52.dp.toPx() }
                    Rect(cx - 80f, topPx - 32f, cx + 80f, topPx)
                }
                gatewayPickerState.openInteractive(anchor, centered = true)
            }
        } else {
            handleEasterEggTap()
        }
    }

    var lastSeenDismissTrigger by remember { mutableIntStateOf(dismissModalsTrigger) }
    LaunchedEffect(dismissModalsTrigger) {
        if (dismissModalsTrigger != lastSeenDismissTrigger) {
            lastSeenDismissTrigger = dismissModalsTrigger
            gatewayPickerState.dismiss()
            renamingGateway = null
            removingGateway = null
            showQuotaSheet = false
            showBackgroundTasksSheet = false
            showNewConvSheet = false
            showSettingsSheet = false
            showEasterEgg = false
            renamingItem = null
            deletingItem = null
            pinningItem = null
            archivingItem = null
            longPressedItem = null
            editingAliasProject = null
        }
    }
    LaunchedEffect(openQuotaRequested) {
        if (openQuotaRequested) {
            onConsumeOpenQuota()
            showBackgroundTasksSheet = false
            showNewConvSheet = false
            showSettingsSheet = false
            showEasterEgg = false
            renamingItem = null
            deletingItem = null
            pinningItem = null
            archivingItem = null
            longPressedItem = null
            editingAliasProject = null
            showQuotaSheet = true
            viewModel.refreshQuotas()
        }
    }

    val colors = AntigravityTheme.colors
    val isRefreshing by viewModel.isRefreshing.collectAsStateWithLifecycle()
    val pullRefreshState = rememberPullToRefreshState()
    var refreshTriggerToken by remember { mutableIntStateOf(0) }
    val listState = rememberLazyListState()
    val coroutineScope = rememberCoroutineScope()
    var previousSearchQuery by rememberSaveable { mutableStateOf(searchQuery) }
    var previousFilterKey by rememberSaveable { mutableStateOf(selectedWorkspaceFilter?.let { when { it.isPureChat -> "__chat__"; it.isStarred -> "__starred__"; it.isArchived -> "__archived__"; else -> "ws:${it.displayName}" } }) }

    // 仅在搜索词真正发生变化时回到顶部，避免从详情页返回重建组合后把已恢复的列表滚动位置重置回 0
    LaunchedEffect(searchQuery) {
        swipedConversationId = null
        if (searchQuery != previousSearchQuery) {
            previousSearchQuery = searchQuery
            if (listState.firstVisibleItemIndex > 0 || listState.firstVisibleItemScrollOffset > 0) {
                listState.scrollToItem(0)
            }
        }
    }

    LaunchedEffect(selectedWorkspaceFilter) {
        swipedConversationId = null
        val currentKey = selectedWorkspaceFilter?.let { when { it.isPureChat -> "__chat__"; it.isStarred -> "__starred__"; it.isArchived -> "__archived__"; else -> "ws:${it.displayName}" } }
        if (currentKey != previousFilterKey) {
            previousFilterKey = currentKey
            if (listState.firstVisibleItemIndex > 0 || listState.firstVisibleItemScrollOffset > 0) {
                listState.scrollToItem(0)
            }
        }
    }

    LaunchedEffect(uiState) {
        swipedConversationId = null
    }

    var previousActiveGatewayId by rememberSaveable { mutableStateOf(activeGatewayId) }
    LaunchedEffect(activeGatewayId) {
        if (activeGatewayId != previousActiveGatewayId) {
            val hadPrevious = previousActiveGatewayId != null
            previousActiveGatewayId = activeGatewayId
            if (hadPrevious && (listState.firstVisibleItemIndex > 0 || listState.firstVisibleItemScrollOffset > 0)) {
                listState.scrollToItem(0)
            }
        }
    }

    val lifecycleOwner = androidx.lifecycle.compose.LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = androidx.lifecycle.LifecycleEventObserver { _, event ->
            when (event) {
                androidx.lifecycle.Lifecycle.Event.ON_START, androidx.lifecycle.Lifecycle.Event.ON_RESUME -> {
                    viewModel.startAutoRefresh()
                    // Don't trust an old green dot after the app was suspended.
                    viewModel.reconnectEventsIfStale()
                    gwStore?.refreshInactiveSummaries(force = false, minIntervalMs = 45_000L)
                }
                androidx.lifecycle.Lifecycle.Event.ON_STOP, androidx.lifecycle.Lifecycle.Event.ON_PAUSE -> {
                    viewModel.stopAutoRefresh()
                }
                else -> {}
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            viewModel.stopAutoRefresh()
        }
    }

    LaunchedEffect(Unit) {
        if (projects.isEmpty()) {
            viewModel.loadProjects()
        }
    }



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
            .onSizeChanged { gatewayPickerState.containerSize = it }
    ) {
        // Content Area with PullToRefresh and Bottom Floating Search Bar
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
                    modifier = Modifier
                        .align(Alignment.TopCenter)
                        .padding(top = topBarHeight),
                    containerColor = colors.surface,
                    color = colors.accentIndigo
                )
            }
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
                            if (compat.isIncompatible) {
                                CompatBanner(text = compat.bannerText)
                                Spacer(modifier = Modifier.height(20.dp))
                            }
                            Icon(
                                imageVector = Icons.Default.Warning,
                                contentDescription = "Error",
                                tint = colors.accentOrange,
                                modifier = Modifier.size(48.dp)
                            )
                            Spacer(modifier = Modifier.height(12.dp))
                            Text(
                                text = if (hasMultipleGateways && activeGateway != null) {
                                    "无法连接「%s」".loc(activeGateway.resolvedDisplayName)
                                } else {
                                    "无法连接网关".loc
                                },
                                color = colors.textPrimary,
                                fontSize = 17.sp,
                                fontWeight = FontWeight.SemiBold,
                                textAlign = TextAlign.Center
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
                                Text("重试连接".loc)
                            }
                            if (hasMultipleGateways) {
                                Spacer(modifier = Modifier.height(6.dp))
                                TextButton(
                                    onClick = {
                                        haptic.light()
                                        handleHeroLongPressOpened()
                                        val cx = gatewayPickerState.containerSize.width / 2f
                                        val topPx = with(density) { (topBarHeight + 48.dp).toPx() }
                                        val anchor = Rect(cx - 120f, topPx - 36f, cx + 120f, topPx)
                                        gatewayPickerState.openInteractive(anchor, centered = true)
                                    }
                                ) {
                                    Text(
                                        text = "切换其他主机".loc,
                                        color = colors.accentIndigo,
                                        fontSize = 14.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                }
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
                            expandedBranchParentIds = expandedBranchParentIds,
                            searchCollapsedBranchParentIds = searchCollapsedBranchParentIds,
                            manuallyCollapsedActionBranchParentIds = manuallyCollapsedActionBranchParentIds,
                            onToggleBranchGroup = { parentId, hasAction ->
                                swipedConversationId = null
                                haptic.light()
                                viewModel.toggleBranchGroup(parentId, hasAction)
                            },
                            listState = listState,
                            quotaData = quotaData,
                            linkSnapshot = linkSnapshot,
                            globalBackgroundTasks = globalBackgroundTasks,
                            compat = compat,
                            searchQuery = searchQuery,
                            selectedWorkspaceFilter = selectedWorkspaceFilter,
                            workspaceFilterOptions = workspaceFilterOptions,
                            totalVisibleCount = totalVisibleCount,
                            archivedCount = archivedCount,
                            workspaceTotalCount = workspaceTotalCount,
                            hiddenPendingApprovalCount = hiddenPendingApprovalCount,
                            onWorkspaceTagTap = { item ->
                                swipedConversationId = null
                                haptic.light()
                                viewModel.toggleWorkspaceFilter(item)
                            },
                            onSelectWorkspaceFilter = { filter ->
                                swipedConversationId = null
                                haptic.light()
                                viewModel.selectWorkspaceFilter(filter)
                            },
                            onClearWorkspaceFilter = {
                                swipedConversationId = null
                                haptic.light()
                                viewModel.clearWorkspaceFilter()
                            },
                            contentResults = contentResults.filter { hit ->
                                val matchesFilter = selectedWorkspaceFilter?.let { filter ->
                                    val known = viewModel.getConversation(hit.cascadeId)
                                        ?: hit.parentConversationId?.let { viewModel.getConversation(it) }
                                    known != null && filter.matches(known)
                                } ?: true
                                matchesFilter && state.conversations.none { it.id == hit.cascadeId }
                            },
                            isSearchingContent = isSearchingContent,
                            onSelectContentHit = { hit ->
                                val known = viewModel.getConversation(hit.cascadeId)
                                val resolvedTitle = known?.displayTitle ?: hit.title.ifBlank { "未命名会话".loc }
                                val parentId = (hit.parentConversationId ?: known?.branchParentId)?.trim().orEmpty()
                                val isSide = hit.isSideQuestion || known?.isSideQuestion == true
                                if (isSide && parentId.isNotEmpty() && parentId != hit.cascadeId && onSelectSideQuestionFromSearch != null) {
                                    onSelectSideQuestionFromSearch(
                                        parentId,
                                        hit.cascadeId,
                                        resolvedTitle,
                                        known?.lastModifiedTime ?: hit.lastModifiedTime
                                    )
                                } else {
                                    onSelectConversation(
                                        hit.cascadeId,
                                        resolvedTitle,
                                        false,
                                        known?.isUnread ?: false,
                                        known?.status ?: com.antigravity.mobile.data.model.ConversationStatus.UNKNOWN,
                                        known?.lastModifiedTime ?: hit.lastModifiedTime
                                    )
                                }
                            },
                            largeTitleAlpha = largeTitleAlpha,
                            largeTitleDragOffsetX = animatedLargeDragX,
                            topPadding = topBarHeight,
                            onEasterEggTap = handleEasterEggTap,
                            gatewayPickerState = gatewayPickerState,
                            largeTitleBoundsProvider = { largeTitleBounds },
                            onLargeTitleBoundsChanged = { largeTitleBounds = it },
                            gatewayProfilesProvider = { gatewayProfiles },
                            onLargeTitleHorizontalDragOffsetChanged = { largeTitleDragOffsetPx = it },
                            onHorizontalCycleGateway = handleHorizontalCycleGateway,
                            onHeroLongPressOpened = handleHeroLongPressOpened,
                            onSelectGatewayProfile = handleSelectGatewayProfile,
                            onAddGateway = onAddGateway,
                            onQuotaTap = {
                                swipedConversationId = null
                                showQuotaSheet = true
                            },
                            onBackgroundTasksTap = {
                                swipedConversationId = null
                                showBackgroundTasksSheet = true
                            },
                            onRefresh = { viewModel.loadConversations() },
                            hasDraftFor = { viewModel.prefs.hasDraft(it) },
                            onSelect = onSelectConversation,
                            onRename = { item ->
                                swipedConversationId = null
                                if (!item.isDraft) {
                                    longPressedItem = item
                                }
                            },
                            onDelete = { item ->
                                swipedConversationId = null
                                deletingItem = item
                            },
                            onPin = { item ->
                                swipedConversationId = null
                                pinningItem = item
                            },
                            onArchive = { item ->
                                swipedConversationId = null
                                archivingItem = item
                            },
                            swipedConversationId = swipedConversationId,
                            showWeeklyPaceCursor = showWeeklyPaceCursor,
                            selectedQuotaBucket = selectedQuotaBucket,
                            onSwipedConversationIdChange = { swipedConversationId = it }
                        )
                    }
                }
            }


            // iOS-Style Floating Bottom Search Bar (as specified in session_list.jpg)
            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .windowInsetsPadding(WindowInsets.navigationBars.union(WindowInsets.ime))
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
                        contentDescription = "搜索".loc,
                        tint = colors.textSecondary,
                        modifier = Modifier.size(18.dp)
                    )
                    BasicTextField(
                        value = searchQuery,
                        onValueChange = {
                            swipedConversationId = null
                            viewModel.onSearchQueryChanged(it)
                        },
                        singleLine = true,
                        textStyle = TextStyle(
                            color = colors.textPrimary,
                            fontSize = 15.sp
                        ),
                        cursorBrush = SolidColor(colors.accentIndigo),
                        decorationBox = { innerTextField ->
                            if (searchQuery.isEmpty()) {
                                val placeholderText = selectedWorkspaceFilter?.let {
                                    "在「%s」中搜索...".loc(it.displayName)
                                } ?: "搜索会话或工作区...".loc
                                Text(
                                    text = placeholderText,
                                    color = colors.textSecondary,
                                    fontSize = 15.sp,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
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
                                .combinedClickable(onClick = {
                                    swipedConversationId = null
                                    viewModel.onSearchQueryChanged("")
                                })
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
                        onClick = {
                            swipedConversationId = null
                            showSettingsSheet = true
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
                        imageVector = Icons.Default.Settings,
                        contentDescription = "Settings",
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(20.dp)
                    )
                }
                if (hasPendingAppUpdate) {
                    Box(
                        modifier = Modifier
                            .align(Alignment.TopEnd)
                            .padding(top = 4.dp, end = 4.dp)
                            .size(8.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo)
                            .border(1.5.dp, colors.background, CircleShape)
                    )
                }
            }

            // Centered Title (Transitions in when scrolling up; switches to [Host ▾] with 2+ gateways)
            val chevronRotation by animateFloatAsState(
                targetValue = if (gatewayPickerState.isPresented) 180f else 0f,
                animationSpec = spring(dampingRatio = 0.82f, stiffness = Spring.StiffnessMediumLow),
                label = "inlineHostChevronRotation"
            )
            Box(
                modifier = Modifier
                    .align(Alignment.Center)
                    .padding(horizontal = 66.dp)
                    .onGloballyPositioned { inlineTitleBounds = it.boundsInRoot() }
                    .graphicsLayer {
                        alpha = inlineTitleAlpha
                        translationX = animatedInlineDragX
                        translationY = (1f - inlineTitleAlpha) * 8.dp.toPx()
                    }
                    .clip(RoundedCornerShape(10.dp))
                    .heroGatewayLongPressGesture(
                        enabled = inlineTitleAlpha > 0.5f && viewModel.prefs.isPaired(),
                        pickerState = gatewayPickerState,
                        anchorRectProvider = { inlineTitleBounds },
                        profileCountProvider = { gatewayProfiles.size },
                        profilesProvider = { gatewayProfiles },
                        density = density,
                        haptic = composeHaptic,
                        centeredAnchor = true,
                        canHorizontalSwipe = { gatewayProfiles.size > 1 && viewModel.prefs.isPaired() },
                        onHorizontalDragOffsetChanged = { inlineTitleDragOffsetPx = it },
                        onHorizontalSwipe = handleHorizontalCycleGateway,
                        onShortTap = handleCollapsedTitleTap,
                        onLongPressOpened = handleHeroLongPressOpened,
                        onSelectProfile = handleSelectGatewayProfile,
                        onAddGateway = onAddGateway
                    )
                    .padding(horizontal = 8.dp, vertical = 5.dp),
                contentAlignment = Alignment.Center
            ) {
                if (hasMultipleGateways && activeGateway != null) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        Text(
                            text = activeGateway.resolvedDisplayName,
                            fontWeight = FontWeight.Bold,
                            fontSize = 17.sp,
                            color = colors.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false)
                        )
                        Icon(
                            imageVector = Icons.Default.KeyboardArrowDown,
                            contentDescription = "切换主机".loc,
                            tint = colors.textSecondary,
                            modifier = Modifier
                                .size(17.dp)
                                .graphicsLayer { rotationZ = chevronRotation }
                        )
                        if (hasInactivePendingAction) {
                            Box(
                                modifier = Modifier
                                    .padding(start = 1.dp)
                                    .size(5.5.dp)
                                    .clip(CircleShape)
                                    .background(colors.accentOrange)
                            )
                        }
                    }
                } else {
                    Text(
                        text = "Multigravity",
                        fontWeight = FontWeight.Bold,
                        fontSize = 18.sp,
                        color = colors.textPrimary,
                        textAlign = TextAlign.Center,
                        maxLines = 1
                    )
                }
            }

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
                            swipedConversationId = null
                            val activeFilter = selectedWorkspaceFilter
                            val directProject = activeFilter?.let { viewModel.resolveProjectForFilter(it) }
                            if (directProject != null) {
                                haptic.light()
                                val draftSession = viewModel.createLocalDraftSession(directProject)
                                val title = if (directProject.isPureChat) "新对话".loc else directProject.displayName
                                onSelectConversation(draftSession.id, title, true, false, com.antigravity.mobile.data.model.ConversationStatus.IDLE, null)
                            } else {
                                viewModel.loadProjects()
                                showNewConvSheet = true
                            }
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

        // 3. Hidden Hero Long-Press Multi-Gateway Switcher Overlay (mirrored to bottom model picker)
        GatewaySlidePickerOverlay(
            state = gatewayPickerState,
            profiles = gatewayProfiles,
            activeProfileId = activeGatewayId,
            summaries = gatewaySummaries,
            onSelectProfile = handleSelectGatewayProfile,
            onAddGateway = onAddGateway,
            onRenameProfile = { profile ->
                renamingGateway = profile
                renameGatewayText = profile.resolvedDisplayName
            },
            onRemoveProfile = { profile ->
                removingGateway = profile
            }
        )

        // 4. Subtle 1.5s switch toast banner at top
        AnimatedVisibility(
            visible = !switchBannerText.isNullOrBlank(),
            enter = slideInVertically { -it } + fadeIn(),
            exit = slideOutVertically { -it } + fadeOut(),
            modifier = Modifier
                .align(Alignment.TopCenter)
                .statusBarsPadding()
                .padding(top = 10.dp)
                .zIndex(600f)
        ) {
            Surface(
                shape = CircleShape,
                color = colors.surface.copy(alpha = 0.94f),
                shadowElevation = 8.dp,
                tonalElevation = 2.dp
            ) {
                Row(
                    modifier = Modifier
                        .border(0.8.dp, colors.border.copy(alpha = 0.85f), CircleShape)
                        .padding(horizontal = 14.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Sync,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(14.dp)
                    )
                    Text(
                        text = switchBannerText.orEmpty(),
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textPrimary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }
    }

    // Rename Gateway Dialog
    renamingGateway?.let { target ->
        AlertDialog(
            onDismissRequest = { renamingGateway = null },
            title = { Text("重命名主机".loc, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = {
                OutlinedTextField(
                    value = renameGatewayText,
                    onValueChange = { renameGatewayText = it },
                    placeholder = { Text("输入主机名称".loc, color = colors.textSecondary) },
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
                        if (renameGatewayText.isNotBlank()) {
                            gwStore?.renameProfile(target.id, renameGatewayText)
                        }
                        renamingGateway = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                ) {
                    Text("保存".loc)
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { renamingGateway = null },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.textSecondary)
                ) {
                    Text("取消".loc)
                }
            },
            containerColor = colors.surface
        )
    }

    // Remove Gateway Confirmation Dialog
    removingGateway?.let { target ->
        AlertDialog(
            onDismissRequest = { removingGateway = null },
            title = { Text("移除主机".loc, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = {
                Text(
                    text = "确定移除「%s」吗？将通知该网关解绑此设备并清除该主机的本地缓存。".loc(target.resolvedDisplayName),
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        removingGateway = null
                        if (target.id == gwStore?.activeProfileId) {
                            onNavigateToPair()
                        } else {
                            gwStore?.removeProfile(target.id)
                        }
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("移除".loc)
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { removingGateway = null },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.textSecondary)
                ) {
                    Text("取消".loc)
                }
            },
            containerColor = colors.surface
        )
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
            showWeeklyPaceCursor = showWeeklyPaceCursor,
            selectedQuotaBucket = selectedQuotaBucket,
            onSelectQuotaBucket = { viewModel.prefs.selectedQuotaBucket = it },
            isMasked = isEmailMasked,
            onToggleMask = { viewModel.prefs.cockpitEmailMasked = !isEmailMasked },
            tokenHistory = tokenHistory,
            isLoadingTokenHistory = isLoadingTokenHistory,
            onLoadTokenHistory = { force -> viewModel.loadTokenHistory(force) },
            onRefresh = { viewModel.refreshQuotas() },
            onSwitchAccount = { viewModel.switchCockpitAccount(it) },
            onDismiss = { showQuotaSheet = false }
        )
    }

    if (showBackgroundTasksSheet) {
        McpSkillsSheet(
            initialTab = McpSkillsTab.TASKS,
            onSelectConversation = { cascadeId, title ->
                val known = viewModel.getConversation(cascadeId)
                onSelectConversation(
                    cascadeId,
                    known?.displayTitle ?: title.ifBlank { "会话详情".loc },
                    false,
                    known?.isUnread ?: false,
                    known?.status ?: com.antigravity.mobile.data.model.ConversationStatus.RUNNING,
                    known?.lastModifiedTime
                )
            },
            onTasksChanged = { viewModel.updateGlobalBackgroundTasks(it) },
            onDismiss = { showBackgroundTasksSheet = false }
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
                val title = if (project.isPureChat) "新对话".loc else project.displayName
                onSelectConversation(draftSession.id, title, true, false, com.antigravity.mobile.data.model.ConversationStatus.IDLE, null)
            },
            onEditProjectAlias = { project ->
                editingAliasProject = project
                aliasInputText = project.alias ?: ""
            },
            onStartWorktreeSession = { project ->
                showNewConvSheet = false
                p3Scope.launch {
                    val resp = ApiClient.defaultInstance?.createWorktreeSession(
                        repoUri = project.uri,
                        workspacePath = project.path
                    )?.getOrNull()
                    if (resp?.ok == true && !resp.cascadeId.isNullOrBlank()) {
                        viewModel.refresh()
                        onSelectConversation(
                            resp.cascadeId,
                            "${project.displayName} (Worktree)",
                            false,
                            false,
                            com.antigravity.mobile.data.model.ConversationStatus.IDLE,
                            null
                        )
                    } else {
                        val draftSession = viewModel.createLocalDraftSession(project)
                        onSelectConversation(draftSession.id, project.displayName, true, false, com.antigravity.mobile.data.model.ConversationStatus.IDLE, null)
                    }
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
            onDismiss = { showSettingsSheet = false },
            onLoadModels = { viewModel.loadModelCatalog(force = true) }
        )
    }

    // Unified Long-Press Conversation Context Dialog (Rename / Star / Archive / Delete)
    longPressedItem?.let { item ->
        AlertDialog(
            onDismissRequest = { longPressedItem = null },
            containerColor = colors.surface,
            title = {
                Text(
                    text = item.displayTitle,
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    fontSize = 16.sp
                )
            },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .clickable {
                                val target = item
                                longPressedItem = null
                                renamingItem = target
                                renameText = target.title
                            }
                            .padding(horizontal = 8.dp, vertical = 11.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(Icons.Default.Edit, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                        Text("重命名会话".loc, fontSize = 14.5.sp, color = colors.textPrimary)
                    }
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .clickable {
                                val target = item
                                longPressedItem = null
                                haptic.light()
                                viewModel.toggleStarConversation(target.id)
                            }
                            .padding(horizontal = 8.dp, vertical = 11.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(
                            imageVector = if (item.isStarred) Icons.Default.Star else Icons.Default.StarBorder,
                            contentDescription = null,
                            tint = Color(0xFFF59E0B),
                            modifier = Modifier.size(18.dp)
                        )
                        Text(if (item.isStarred) "取消星标".loc else "星标会话".loc, fontSize = 14.5.sp, color = colors.textPrimary)
                    }
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .clickable {
                                val target = item
                                longPressedItem = null
                                archivingItem = target
                            }
                            .padding(horizontal = 8.dp, vertical = 11.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(Icons.Default.Inventory2, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                        Text(if (item.isArchived) "取消归档".loc else "归档会话".loc, fontSize = 14.5.sp, color = colors.textPrimary)
                    }
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .clickable {
                                val target = item
                                longPressedItem = null
                                deletingItem = target
                            }
                            .padding(horizontal = 8.dp, vertical = 11.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(Icons.Default.Delete, contentDescription = null, tint = colors.accentRed, modifier = Modifier.size(18.dp))
                        Text("删除".loc, fontSize = 14.5.sp, color = colors.accentRed)
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { longPressedItem = null }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            }
        )
    }

    // Rename Conversation Dialog
    renamingItem?.let { item ->
        AlertDialog(
            onDismissRequest = { renamingItem = null },
            title = { Text("重命名会话".loc, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
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
                    Text("保存".loc)
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { renamingItem = null },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.textSecondary)
                ) {
                    Text("取消".loc)
                }
            },
            containerColor = colors.surface
        )
    }

    // Workspace Alias Dialog (matches rename conversation dialog structure)
    editingAliasProject?.let { project ->
        AlertDialog(
            onDismissRequest = { editingAliasProject = null },
            title = { Text("设置工作区备注".loc, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "原文件夹：%s".loc(project.name),
                        fontSize = 12.5.sp,
                        color = colors.textSecondary
                    )
                    OutlinedTextField(
                        value = aliasInputText,
                        onValueChange = { aliasInputText = it },
                        placeholder = { Text("输入中文备注（留空恢复默认）".loc, color = colors.textMuted) },
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
                }
            },
            confirmButton = {
                Button(
                    onClick = {
                        viewModel.updateProjectAlias(project, aliasInputText)
                        editingAliasProject = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                ) {
                    Text("保存".loc)
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { editingAliasProject = null },
                    colors = ButtonDefaults.textButtonColors(contentColor = colors.textSecondary)
                ) {
                    Text("取消".loc)
                }
            },
            containerColor = colors.surface
        )
    }

    // Delete Confirmation Dialog
    deletingItem?.let { item ->
        AlertDialog(
            onDismissRequest = { deletingItem = null },
            title = { Text("确认删除此会话吗？".loc, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = { Text("此操作将永久删除会话记录且无法撤销。".loc, color = colors.textSecondary) },
            confirmButton = {
                Button(
                    onClick = {
                        viewModel.deleteConversation(item.id)
                        deletingItem = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("删除".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { deletingItem = null }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }

    // Pin / Unpin Confirmation Dialog
    pinningItem?.let { item ->
        val isCurrentlyPinned = item.isPinned
        val pinTitle = if (isCurrentlyPinned) "取消置顶".loc else "置顶会话".loc
        val pinMessage = if (isCurrentlyPinned) {
            "确定取消置顶此会话吗？".loc
        } else {
            "确定置顶此会话吗？\n置顶后该会话将固定显示在会话列表顶部。".loc
        }
        AlertDialog(
            onDismissRequest = { pinningItem = null },
            title = { Text(pinTitle, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = { Text(pinMessage, color = colors.textSecondary) },
            confirmButton = {
                Button(
                    onClick = {
                        haptic.medium()
                        viewModel.togglePinConversation(item.id)
                        pinningItem = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentOrange)
                ) {
                    Text(if (isCurrentlyPinned) "取消置顶".loc else "置顶".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { pinningItem = null }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }

    // Archive / Unarchive Confirmation Dialog
    archivingItem?.let { item ->
        val isCurrentlyArchived = item.isArchived
        val archiveTitle = if (isCurrentlyArchived) "取消归档".loc else "归档会话".loc
        val archiveMessage = if (isCurrentlyArchived) {
            "确定取消归档此会话吗？".loc
        } else {
            "确定归档此会话吗？\n归档后该会话将移入「已归档」，随时可查看或恢复。".loc
        }
        AlertDialog(
            onDismissRequest = { archivingItem = null },
            title = { Text(archiveTitle, color = colors.textPrimary, fontWeight = FontWeight.Bold) },
            text = { Text(archiveMessage, color = colors.textSecondary) },
            confirmButton = {
                Button(
                    onClick = {
                        haptic.medium()
                        viewModel.toggleArchiveConversation(item.id)
                        archivingItem = null
                    },
                    colors = ButtonDefaults.buttonColors(
                        containerColor = if (isCurrentlyArchived) colors.accentIndigo else Color(0xFF64748B),
                        contentColor = Color.White
                    )
                ) {
                    Text(if (isCurrentlyArchived) "取消归档".loc else "归档".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { archivingItem = null }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }
}

/** 升级自检提示条：当前 Antigravity 版本缺少基础接口，需要升级网关。 */
@Composable
private fun CompatBanner(text: String, modifier: Modifier = Modifier) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.accentRed.copy(alpha = 0.12f))
            .border(1.dp, colors.accentRed.copy(alpha = 0.35f), RoundedCornerShape(12.dp))
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Icon(
            imageVector = Icons.Default.Warning,
            contentDescription = null,
            tint = colors.accentRed,
            modifier = Modifier.padding(top = 1.dp).size(18.dp)
        )
        Text(text = text, color = colors.textPrimary, fontSize = 13.5.sp, lineHeight = 19.sp)
    }
}

private enum class BranchAggregateStatus {
    ACTION, ERROR, WORKING, DRAFT, UNREAD, NONE
}

@Composable
private fun ConversationListContent(
    conversations: List<ConversationItem>,
    expandedBranchParentIds: Set<String>,
    searchCollapsedBranchParentIds: Set<String>,
    manuallyCollapsedActionBranchParentIds: Set<String>,
    onToggleBranchGroup: (parentId: String, hasAction: Boolean) -> Unit,
    listState: LazyListState,
    quotaData: CockpitQuotaResponse?,
    linkSnapshot: com.antigravity.mobile.data.service.LinkSnapshot?,
    globalBackgroundTasks: List<com.antigravity.mobile.data.model.GlobalBackgroundTaskItem>,
    compat: com.antigravity.mobile.data.model.GatewayCompat,
    searchQuery: String,
    selectedWorkspaceFilter: WorkspaceFilter?,
    workspaceFilterOptions: List<WorkspaceFilterOption>,
    totalVisibleCount: Int,
    archivedCount: Int = 0,
    workspaceTotalCount: Int,
    hiddenPendingApprovalCount: Int = 0,
    onWorkspaceTagTap: (ConversationItem) -> Unit,
    onSelectWorkspaceFilter: (WorkspaceFilter) -> Unit,
    onClearWorkspaceFilter: () -> Unit,
    contentResults: List<com.antigravity.mobile.data.model.ConversationSearchResult>,
    isSearchingContent: Boolean,
    onSelectContentHit: (com.antigravity.mobile.data.model.ConversationSearchResult) -> Unit,
    largeTitleAlpha: Float,
    largeTitleDragOffsetX: Float = 0f,
    topPadding: Dp,
    onEasterEggTap: () -> Unit,
    gatewayPickerState: GatewaySlidePickerState,
    largeTitleBoundsProvider: () -> Rect,
    onLargeTitleBoundsChanged: (Rect) -> Unit,
    gatewayProfilesProvider: () -> List<GatewayProfile>,
    onLargeTitleHorizontalDragOffsetChanged: (Float) -> Unit = {},
    onHorizontalCycleGateway: (Int) -> Unit = {},
    onHeroLongPressOpened: () -> Unit,
    onSelectGatewayProfile: (GatewayProfile) -> Unit,
    onAddGateway: () -> Unit,
    onQuotaTap: () -> Unit,
    onBackgroundTasksTap: () -> Unit,
    onRefresh: () -> Unit,
    hasDraftFor: (String) -> Boolean,
    onSelect: (cascadeId: String, title: String, isNew: Boolean, isUnread: Boolean, status: com.antigravity.mobile.data.model.ConversationStatus, lastModifiedTime: String?) -> Unit,
    onRename: (ConversationItem) -> Unit,
    onDelete: (ConversationItem) -> Unit,
    onPin: (ConversationItem) -> Unit,
    onArchive: (ConversationItem) -> Unit = {},
    swipedConversationId: String?,
    showWeeklyPaceCursor: Boolean = true,
    selectedQuotaBucket: com.antigravity.mobile.data.model.QuotaBucketKind = com.antigravity.mobile.data.model.QuotaBucketKind.GEMINI_5H,
    onSwipedConversationIdChange: (String?) -> Unit
) {
    val colors = AntigravityTheme.colors
    val hasGlobalTasks = globalBackgroundTasks.isNotEmpty()
    val globalTasksAppearAnim = remember { Animatable(if (hasGlobalTasks) 1f else 0f) }

    val context = androidx.compose.ui.platform.LocalContext.current
    val tipsPrefs = remember { context.getSharedPreferences("antigravity_tips", android.content.Context.MODE_PRIVATE) }
    var firstCardPeekOffset by remember { mutableStateOf(0f) }

    LaunchedEffect(Unit) {
        val openCount = tipsPrefs.getInt("conversation_list_open_count", 0) + 1
        tipsPrefs.edit().putInt("conversation_list_open_count", openCount).apply()
        val hasSeenSwipe = tipsPrefs.getBoolean("has_seen_swipe_gesture_hint", false)
        if (openCount >= 2 && !hasSeenSwipe) {
            kotlinx.coroutines.delay(700)
            if (conversations.isNotEmpty()) {
                androidx.compose.animation.core.animate(
                    initialValue = 0f,
                    targetValue = -60f,
                    animationSpec = androidx.compose.animation.core.spring(
                        dampingRatio = androidx.compose.animation.core.Spring.DampingRatioMediumBouncy,
                        stiffness = androidx.compose.animation.core.Spring.StiffnessMedium
                    )
                ) { value, _ -> firstCardPeekOffset = value }
                kotlinx.coroutines.delay(350)
                androidx.compose.animation.core.animate(
                    initialValue = -60f,
                    targetValue = 0f,
                    animationSpec = androidx.compose.animation.core.spring(
                        dampingRatio = androidx.compose.animation.core.Spring.DampingRatioMediumBouncy,
                        stiffness = androidx.compose.animation.core.Spring.StiffnessMedium
                    )
                ) { value, _ -> firstCardPeekOffset = value }
                tipsPrefs.edit().putBoolean("has_seen_swipe_gesture_hint", true).apply()
            }
        }
    }

    LaunchedEffect(hasGlobalTasks) {
        if (hasGlobalTasks) {
            if (globalTasksAppearAnim.value < 1f) {
                globalTasksAppearAnim.animateTo(
                    targetValue = 1f,
                    animationSpec = spring(
                        dampingRatio = Spring.DampingRatioMediumBouncy,
                        stiffness = Spring.StiffnessLow
                    )
                )
            }
        } else {
            globalTasksAppearAnim.snapTo(0f)
        }
    }

    LaunchedEffect(listState.isScrollInProgress) {
        if (listState.isScrollInProgress && swipedConversationId != null) {
            onSwipedConversationIdChange(null)
        }
    }

    val navBottomPadding = WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
    val displayGroups = remember(conversations) {
        ConversationListViewModel.groupConversationsWithBranches(conversations)
    }
    val isSearchingBranches = searchQuery.trim().isNotEmpty()
    val firstConversationId = conversations.firstOrNull()?.id

    LazyColumn(
        state = listState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(
            top = topPadding + 4.dp,
            bottom = navBottomPadding + 92.dp,
            start = 16.dp,
            end = 16.dp
        ),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        // Large Title Header (matches iOS large title below settings button)
        item(key = "header_large_title") {
            val titleDensity = LocalDensity.current
            val titleHaptic = LocalHapticFeedback.current
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
                        .onGloballyPositioned { onLargeTitleBoundsChanged(it.boundsInRoot()) }
                        .graphicsLayer {
                            translationX = largeTitleDragOffsetX
                        }
                        .clip(RoundedCornerShape(8.dp))
                        .heroGatewayLongPressGesture(
                            enabled = largeTitleAlpha > 0.5f,
                            pickerState = gatewayPickerState,
                            anchorRectProvider = largeTitleBoundsProvider,
                            profileCountProvider = { gatewayProfilesProvider().size },
                            profilesProvider = gatewayProfilesProvider,
                            density = titleDensity,
                            haptic = titleHaptic,
                            centeredAnchor = false,
                            canHorizontalSwipe = { gatewayProfilesProvider().size > 1 },
                            onHorizontalDragOffsetChanged = onLargeTitleHorizontalDragOffsetChanged,
                            onHorizontalSwipe = onHorizontalCycleGateway,
                            onShortTap = onEasterEggTap,
                            onLongPressOpened = onHeroLongPressOpened,
                            onSelectProfile = onSelectGatewayProfile,
                            onAddGateway = onAddGateway
                        )
                )
            }
        }

        // 升级自检：基础能力缺失时在首页顶部提示（可选功能缺失只会隐藏对应入口，不打扰）
        if (compat.isIncompatible) {
            item(key = "header_compat_banner") {
                CompatBanner(
                    text = compat.bannerText,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }
        }

        // Quota Status Bar (matches iOS QuotaStatusBarView directly below large title)
        quotaData?.currentAccount?.let { acc ->
            if (acc.resolvedQuotaDisplay(selectedQuotaBucket) != null) {
                item(key = "header_quota") {
                    QuotaStatusBar(
                        account = acc,
                        onTap = onQuotaTap,
                        selectedBucket = selectedQuotaBucket,
                        showPaceCursor = showWeeklyPaceCursor,
                        linkSnapshot = linkSnapshot,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 6.dp)
                    )
                }
            }
        }

        // Global Background Tasks Capsule (appears right below QuotaStatusBar when count > 0)
        if (hasGlobalTasks) {
            item(key = "header_global_tasks") {
                val count = globalBackgroundTasks.size
                val daemonCount = globalBackgroundTasks.count { it.isDaemon }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 6.dp)
                        .zIndex(-1f)
                        .animateItem(
                            fadeInSpec = null,
                            placementSpec = spring(
                                dampingRatio = Spring.DampingRatioMediumBouncy,
                                stiffness = Spring.StiffnessLow
                            ),
                            fadeOutSpec = spring(
                                dampingRatio = Spring.DampingRatioNoBouncy,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                        .graphicsLayer {
                            val p = globalTasksAppearAnim.value
                            val scale = 0.92f + 0.08f * p
                            scaleX = scale
                            scaleY = scale
                            translationY = -12.dp.toPx() * (1f - p)
                            alpha = p.coerceIn(0f, 1f)
                        }
                        .clip(CircleShape)
                        .background(colors.surface)
                        .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.28f), CircleShape)
                        .clickable { onBackgroundTasksTap() }
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = ">_",
                        fontSize = 11.sp,
                        fontFamily = FontFamily.Monospace,
                        fontWeight = FontWeight.Bold,
                        color = colors.accentIndigo
                    )
                    Text(
                        text = if (daemonCount > 0) "${count} 个后台任务运行中（${daemonCount} 个常驻）".loc else "${count} 个后台任务运行中".loc,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.textPrimary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    Text(
                        text = "管理 ›".loc,
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.accentIndigo,
                        maxLines = 1,
                        softWrap = false
                    )
                }
            }
        }

        // Workspace Filter Capsule (appears below quota / background tasks when filtering by project or Chat)
        selectedWorkspaceFilter?.let { activeFilter ->
            item(key = "header_workspace_filter") {
                WorkspaceFilterCapsule(
                    activeFilter = activeFilter,
                    options = workspaceFilterOptions,
                    totalVisibleCount = totalVisibleCount,
                    workspaceTotalCount = workspaceTotalCount,
                    onSelectFilter = onSelectWorkspaceFilter,
                    onClear = onClearWorkspaceFilter,
                    modifier = Modifier
                        .padding(top = 6.dp)
                        .animateItem(
                            placementSpec = spring(
                                dampingRatio = Spring.DampingRatioMediumBouncy,
                                stiffness = Spring.StiffnessLow
                            )
                        )
                )
            }
            if (hiddenPendingApprovalCount > 0) {
                item(key = "header_hidden_pending_approvals") {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 6.dp)
                            .animateItem(
                                placementSpec = spring(
                                    dampingRatio = Spring.DampingRatioMediumBouncy,
                                    stiffness = Spring.StiffnessLow
                                )
                            )
                            .clip(CircleShape)
                            .background(colors.accentOrange.copy(alpha = if (colors.isDark) 0.14f else 0.10f))
                            .border(0.8.dp, colors.accentOrange.copy(alpha = 0.34f), CircleShape)
                            .clickable { onClearWorkspaceFilter() }
                            .padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Warning,
                            contentDescription = null,
                            tint = colors.accentOrange,
                            modifier = Modifier.size(14.dp)
                        )
                        Text(
                            text = when (L10n.currentLang) {
                                L10n.AppLang.EN -> if (hiddenPendingApprovalCount == 1) "1 approval waiting in other workspaces" else "$hiddenPendingApprovalCount approvals waiting in other workspaces"
                                L10n.AppLang.JA -> "他ワークスペースに承認待ち ${hiddenPendingApprovalCount} 件"
                                L10n.AppLang.ZH -> "其他工作区有 $hiddenPendingApprovalCount 个待审批"
                            },
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium,
                            color = colors.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f)
                        )
                        Text(
                            text = when (L10n.currentLang) {
                                L10n.AppLang.EN -> "Show ›"
                                L10n.AppLang.JA -> "表示 ›"
                                L10n.AppLang.ZH -> "查看 ›"
                            },
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.accentOrange,
                            maxLines = 1,
                            softWrap = false
                        )
                    }
                }
            }
        }

        if (conversations.isEmpty() && contentResults.isEmpty() && !isSearchingContent) {
            item(key = "empty_placeholder") {
                val activeFilter = selectedWorkspaceFilter
                Column(
                    modifier = Modifier
                        .animateItem(
                            placementSpec = spring(
                                dampingRatio = Spring.DampingRatioMediumBouncy,
                                stiffness = Spring.StiffnessLow
                            )
                        )
                        .fillMaxWidth()
                        .padding(vertical = 48.dp, horizontal = 16.dp),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Icon(
                        imageVector = when {
                            searchQuery.isNotEmpty() -> Icons.Default.Search
                            activeFilter != null && !activeFilter.isPureChat -> Icons.Default.Folder
                            else -> Icons.Default.ChatBubbleOutline
                        },
                        contentDescription = "Empty",
                        tint = colors.textMuted,
                        modifier = Modifier.size(44.dp)
                    )
                    Spacer(modifier = Modifier.height(12.dp))
                    Text(
                        text = when {
                            activeFilter != null && searchQuery.isNotEmpty() -> "在「%s」中未找到匹配会话".loc(activeFilter.displayName)
                            activeFilter != null -> "「%s」暂无会话".loc(activeFilter.displayName)
                            searchQuery.isEmpty() -> "暂无会话".loc
                            else -> "未找到匹配会话".loc
                        },
                        color = colors.textPrimary,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.SemiBold,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(4.dp))
                    Text(
                        text = when {
                            activeFilter != null -> "可点击右上角 + 新建会话，或返回全部会话".loc
                            searchQuery.isEmpty() -> "可点击右上角 + 开启新会话，或下拉刷新同步".loc
                            else -> "请尝试其他关键词搜索".loc
                        },
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        textAlign = TextAlign.Center
                    )
                    if (activeFilter != null) {
                        Spacer(modifier = Modifier.height(16.dp))
                        Button(
                            onClick = onClearWorkspaceFilter,
                            colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                        ) {
                            Text("查看全部会话".loc)
                        }
                    } else if (searchQuery.isEmpty()) {
                        Spacer(modifier = Modifier.height(16.dp))
                        Button(
                            onClick = onRefresh,
                            colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                        ) {
                            Text("刷新列表".loc)
                        }
                    }
                }
            }
        } else {
            items(displayGroups, key = { it.root.id }) { group ->
                val conversation = group.root
                val hasDraft = conversation.isDraft || hasDraftFor(conversation.id)
                val isFirst = conversation.id == firstConversationId
                val hasBranches = group.branches.isNotEmpty()

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 6.dp)
                        .animateItem(
                            placementSpec = spring(
                                dampingRatio = Spring.DampingRatioNoBouncy,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                ) {
                    SwipeableConversationCard(
                        modifier = Modifier
                            .offset { androidx.compose.ui.unit.IntOffset(if (isFirst) firstCardPeekOffset.toInt() else 0, 0) },
                        conversation = conversation,
                        hasDraft = hasDraft,
                        isEmbeddedBranchChild = false,
                        isSwiped = (swipedConversationId == conversation.id),
                        onSwipeOpened = {
                            tipsPrefs.edit().putBoolean("has_seen_swipe_gesture_hint", true).apply()
                            onSwipedConversationIdChange(conversation.id)
                        },
                        onSwipeClosed = {
                            if (swipedConversationId == conversation.id) {
                                onSwipedConversationIdChange(null)
                            }
                        },
                        onStartDrag = {
                            if (swipedConversationId != null && swipedConversationId != conversation.id) {
                                onSwipedConversationIdChange(null)
                            }
                        },
                        onClick = {
                            if (swipedConversationId != null) {
                                onSwipedConversationIdChange(null)
                            } else {
                                onSelect(
                                    conversation.id,
                                    conversation.displayTitle,
                                    false,
                                    conversation.isUnread,
                                    conversation.status,
                                    conversation.lastModifiedTime
                                )
                            }
                        },
                        onWorkspaceTap = {
                            if (swipedConversationId != null) {
                                onSwipedConversationIdChange(null)
                            } else {
                                onWorkspaceTagTap(conversation)
                            }
                        },
                        onLongClick = { onRename(conversation) },
                        onDelete = {
                            onSwipedConversationIdChange(null)
                            onDelete(conversation)
                        },
                        onPin = {
                            onSwipedConversationIdChange(null)
                            onPin(conversation)
                        },
                        onArchive = {
                            onSwipedConversationIdChange(null)
                            onArchive(conversation)
                        }
                    )

                    if (hasBranches) {
                        val hasAction = group.branches.any { it.status.needsAction }
                        val isExpanded = when {
                            isSearchingBranches -> conversation.id !in searchCollapsedBranchParentIds
                            hasAction -> conversation.id !in manuallyCollapsedActionBranchParentIds
                            else -> conversation.id in expandedBranchParentIds
                        }
                        val aggStatus = when {
                            hasAction -> BranchAggregateStatus.ACTION
                            group.branches.any { it.status.isError } -> BranchAggregateStatus.ERROR
                            group.branches.any { it.isWorking } -> BranchAggregateStatus.WORKING
                            group.branches.any { it.isDraft || hasDraftFor(it.id) } -> BranchAggregateStatus.DRAFT
                            group.branches.any { it.isUnread } -> BranchAggregateStatus.UNREAD
                            else -> BranchAggregateStatus.NONE
                        }

                        BranchGroupDrawer(
                            branches = group.branches,
                            isExpanded = isExpanded,
                            aggregateStatus = aggStatus,
                            hasDraftFor = hasDraftFor,
                            swipedConversationId = swipedConversationId,
                            onSwipedConversationIdChange = { id ->
                                if (id != null) {
                                    tipsPrefs.edit().putBoolean("has_seen_swipe_gesture_hint", true).apply()
                                }
                                onSwipedConversationIdChange(id)
                            },
                            onToggle = {
                                onSwipedConversationIdChange(null)
                                onToggleBranchGroup(conversation.id, hasAction)
                            },
                            onSelectChild = { child ->
                                if (swipedConversationId != null) {
                                    onSwipedConversationIdChange(null)
                                } else {
                                    onSelect(
                                        child.id,
                                        child.displayTitle,
                                        false,
                                        child.isUnread,
                                        child.status,
                                        child.lastModifiedTime
                                    )
                                }
                            },
                            onLongClickChild = { child -> onRename(child) },
                            onDeleteChild = { child ->
                                onSwipedConversationIdChange(null)
                                onDelete(child)
                            },
                            onPinChild = { child ->
                                onSwipedConversationIdChange(null)
                                onPin(child)
                            },
                            onArchiveChild = { child ->
                                onSwipedConversationIdChange(null)
                                onArchive(child)
                            }
                        )
                    }
                }
            }
        }

        if (selectedWorkspaceFilter == null && searchQuery.isBlank() && archivedCount > 0) {
            item(key = "archived_conversations_footer") {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 6.dp)
                        .clip(RoundedCornerShape(12.dp))
                        .clickable { onSelectWorkspaceFilter(WorkspaceFilter.Archived) }
                        .padding(vertical = 10.dp),
                    horizontalArrangement = Arrangement.Center,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Icon(
                        imageVector = Icons.Default.Inventory2,
                        contentDescription = null,
                        tint = colors.textSecondary,
                        modifier = Modifier.size(14.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "已归档会话 (%d)".loc(archivedCount),
                        color = colors.textSecondary,
                        fontSize = 12.5.sp,
                        fontWeight = FontWeight.Medium
                    )
                    Spacer(modifier = Modifier.width(4.dp))
                    Icon(
                        imageVector = Icons.Default.ChevronRight,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(14.dp)
                    )
                }
            }
        }

        // 对话内容命中（language_server 全文搜索），与上面的标题/工作区匹配并列
        if (searchQuery.isNotBlank() && (contentResults.isNotEmpty() || isSearchingContent)) {
            item(key = "content_hits_header") {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = "对话内容匹配".loc,
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                    if (isSearchingContent) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(12.dp),
                            strokeWidth = 1.5.dp,
                            color = colors.textMuted
                        )
                    }
                }
            }
            items(contentResults, key = { "hit_${it.cascadeId}" }) { hit ->
                ContentHitCard(
                    hit = hit,
                    onClick = {
                        if (swipedConversationId != null) {
                            onSwipedConversationIdChange(null)
                        } else {
                            onSelectContentHit(hit)
                        }
                    }
                )
            }
        }
    }
}

@Composable
private fun ContentHitCard(
    hit: com.antigravity.mobile.data.model.ConversationSearchResult,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val highlight = colors.accentOrange.copy(alpha = 0.28f)
    val annotated = remember(hit.snippet, hit.snippetMatchRanges, highlight) {
        androidx.compose.ui.text.buildAnnotatedString {
            append(hit.snippet)
            com.antigravity.mobile.ui.util.highlightRanges(hit.snippet, hit.snippetMatchRanges).forEach { r ->
                addStyle(
                    androidx.compose.ui.text.SpanStyle(background = highlight, fontWeight = FontWeight.SemiBold),
                    r.first,
                    r.last + 1
                )
            }
        }
    }
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 6.dp)
            .clip(RoundedCornerShape(14.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Text(
            text = hit.title.ifBlank { "未命名会话".loc },
            color = colors.textPrimary,
            fontSize = 15.sp,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        if (hit.snippet.isNotBlank()) {
            Text(
                text = annotated,
                color = colors.textSecondary,
                fontSize = 13.sp,
                lineHeight = 18.sp,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis
            )
        }
        val wsName = hit.workspaceName?.trim().orEmpty()
        val metaText = when {
            hit.isSideQuestion && wsName.isNotEmpty() -> "↳ /btw · $wsName"
            hit.isSideQuestion -> "↳ /btw"
            else -> wsName
        }
        if (metaText.isNotEmpty()) {
            Text(text = metaText, color = colors.textMuted, fontSize = 11.5.sp, maxLines = 1)
        }
    }
}

@Composable
private fun WorkspaceFilterCapsule(
    activeFilter: WorkspaceFilter,
    options: List<WorkspaceFilterOption>,
    totalVisibleCount: Int,
    archivedCount: Int = 0,
    workspaceTotalCount: Int,
    hiddenPendingApprovalCount: Int = 0,
    onSelectFilter: (WorkspaceFilter) -> Unit,
    onClear: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    var menuExpanded by remember { mutableStateOf(false) }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(CircleShape)
            .background(colors.surface)
            .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.28f), CircleShape)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Box(modifier = Modifier.weight(1f)) {
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null
                    ) { menuExpanded = true }
                    .padding(vertical = 1.dp, horizontal = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(5.dp)
            ) {
                Icon(
                    imageVector = when {
                        activeFilter.isStarred -> Icons.Default.Star
                        activeFilter.isArchived -> Icons.Default.Inventory2
                        activeFilter.isPureChat -> Icons.Default.ChatBubbleOutline
                        else -> Icons.Default.Folder
                    },
                    contentDescription = null,
                    tint = if (activeFilter.isStarred) Color(0xFFF59E0B) else colors.accentIndigo,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = activeFilter.displayName,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = if (activeFilter.isPureChat || activeFilter.isArchived || activeFilter.isStarred) FontFamily.Default else FontFamily.Monospace,
                    color = colors.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = "切换工作区".loc,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(16.dp)
                )
            }

            DropdownMenu(
                expanded = menuExpanded,
                onDismissRequest = { menuExpanded = false },
                containerColor = colors.surface
            ) {
                DropdownMenuItem(
                    text = {
                        Text(
                            text = "全部会话 (%d)".loc(totalVisibleCount),
                            color = colors.textPrimary,
                            fontSize = 14.sp
                        )
                    },
                    leadingIcon = {
                        Icon(
                            imageVector = Icons.Default.Inbox,
                            contentDescription = null,
                            tint = colors.textSecondary,
                            modifier = Modifier.size(18.dp)
                        )
                    },
                    onClick = {
                        menuExpanded = false
                        onClear()
                    }
                )
                if (options.isNotEmpty()) {
                    HorizontalDivider(thickness = 0.5.dp, color = colors.border)
                    options.forEach { opt ->
                        val isCurrent = opt.filter == activeFilter
                        DropdownMenuItem(
                            text = {
                                Text(
                                    text = "${opt.filter.displayName} (${opt.count})",
                                    color = if (isCurrent) colors.accentIndigo else colors.textPrimary,
                                    fontWeight = if (isCurrent) FontWeight.SemiBold else FontWeight.Normal,
                                    fontSize = 14.sp,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            },
                            leadingIcon = {
                                Icon(
                                    imageVector = when {
                                        opt.filter.isStarred -> Icons.Default.Star
                                        opt.filter.isArchived -> Icons.Default.Inventory2
                                        opt.filter.isPureChat -> Icons.Default.ChatBubbleOutline
                                        else -> Icons.Default.Folder
                                    },
                                    contentDescription = null,
                                    tint = when {
                                        opt.filter.isStarred -> Color(0xFFF59E0B)
                                        isCurrent -> colors.accentIndigo
                                        else -> colors.textSecondary
                                    },
                                    modifier = Modifier.size(18.dp)
                                )
                            },
                            trailingIcon = if (isCurrent) {
                                {
                                    Icon(
                                        imageVector = Icons.Default.Check,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(17.dp)
                                    )
                                }
                            } else null,
                            onClick = {
                                menuExpanded = false
                                onSelectFilter(opt.filter)
                            }
                        )
                    }
                }
            }
        }

        Text(
            text = if (workspaceTotalCount == 1) "1 个会话".loc else "%d 个会话".loc(workspaceTotalCount),
            fontSize = 11.5.sp,
            fontWeight = FontWeight.Medium,
            color = colors.textSecondary,
            maxLines = 1,
            softWrap = false
        )

        Icon(
            imageVector = Icons.Default.Cancel,
            contentDescription = "退出项目筛选".loc,
            tint = colors.textMuted,
            modifier = Modifier
                .size(18.dp)
                .clip(CircleShape)
                .clickable(onClick = onClear)
        )
    }
}

@Composable
private fun BranchGroupDrawer(
    branches: List<ConversationItem>,
    isExpanded: Boolean,
    aggregateStatus: BranchAggregateStatus,
    hasDraftFor: (String) -> Boolean,
    swipedConversationId: String?,
    onSwipedConversationIdChange: (String?) -> Unit,
    onToggle: () -> Unit,
    onSelectChild: (ConversationItem) -> Unit,
    onLongClickChild: (ConversationItem) -> Unit,
    onDeleteChild: (ConversationItem) -> Unit,
    onPinChild: (ConversationItem) -> Unit,
    onArchiveChild: (ConversationItem) -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val countLabel = if (branches.size == 1) "1 个旁路".loc else "%d 个旁路".loc(branches.size)
    val previewTitle = branches.firstOrNull()?.displayTitle?.trim().orEmpty()
    val chevronRotation by animateFloatAsState(
        targetValue = if (isExpanded) 180f else 0f,
        animationSpec = tween(durationMillis = 240, easing = FastOutSlowInEasing),
        label = "BranchDrawerChevron"
    )
    val stemAlpha by animateFloatAsState(
        targetValue = if (isExpanded) 1f else 0f,
        animationSpec = tween(durationMillis = 200, easing = FastOutSlowInEasing),
        label = "BranchDrawerStemAlpha"
    )
    val drawerShape = RoundedCornerShape(
        topStart = 0.dp,
        topEnd = 0.dp,
        bottomStart = 12.dp,
        bottomEnd = 12.dp
    )
    val trayBg = when {
        colors.useNord -> Color(0xFF333947)
        colors.isDark -> Color(0xFF151519)
        else -> Color(0xFFECEDF4)
    }
    val borderColor = colors.accentIndigo.copy(alpha = if (colors.isDark) 0.24f else 0.17f)
    // Opaque pre-blended connector tone eliminates any alpha double-compositing at T-junctions
    val connectorColor = when {
        colors.useNord -> Color(0xFF577281)
        colors.isDark -> Color(0xFF3A3A73)
        else -> Color(0xFFB8B8E6)
    }
    val topHairlineColor = if (colors.isDark) Color.White.copy(alpha = 0.08f) else Color.Black.copy(alpha = 0.07f)
    val topShadowColor = Color.Black.copy(alpha = if (colors.isDark) 0.28f else 0.055f)

    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .clip(drawerShape)
            .background(trayBg)
            .drawWithContent {
                drawContent()
                val strokePx = 0.75.dp.toPx()
                val inset = strokePx / 2f
                val r = 12.dp.toPx().coerceAtMost((size.width - strokePx) / 2f).coerceAtMost(size.height - inset)
                val borderPath = Path().apply {
                    moveTo(inset, 0f)
                    lineTo(inset, size.height - inset - r)
                    quadraticTo(inset, size.height - inset, inset + r, size.height - inset)
                    lineTo(size.width - inset - r, size.height - inset)
                    quadraticTo(size.width - inset, size.height - inset, size.width - inset, size.height - inset - r)
                    lineTo(size.width - inset, 0f)
                }
                drawPath(
                    path = borderPath,
                    color = borderColor,
                    style = Stroke(width = strokePx, cap = StrokeCap.Butt, join = StrokeJoin.Round)
                )
            }
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            // Fixed-height header row (prevents any vertical height jitter between collapsed & expanded)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(36.dp)
                    .clickable(onClick = onToggle)
                    .drawBehind {
                        if (stemAlpha > 0.01f) {
                            val spineX = 16.dp.toPx()
                            val stemH = 7.dp.toPx()
                            drawLine(
                                color = connectorColor.copy(alpha = stemAlpha),
                                start = androidx.compose.ui.geometry.Offset(spineX, size.height - stemH),
                                end = androidx.compose.ui.geometry.Offset(spineX, size.height + 1f),
                                strokeWidth = 1.5.dp.toPx(),
                                cap = StrokeCap.Round
                            )
                        }
                    }
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(horizontal = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(5.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.CallSplit,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(12.dp)
                    )
                    Text(
                        text = countLabel,
                        color = colors.accentIndigo,
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        softWrap = false
                    )
                    if (!isExpanded && previewTitle.isNotEmpty()) {
                        Text(
                            text = "·",
                            color = colors.textSecondary.copy(alpha = 0.75f),
                            fontSize = 11.5.sp
                        )
                        Text(
                            text = previewTitle,
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f)
                        )
                    } else {
                        Spacer(modifier = Modifier.weight(1f))
                    }

                    if (!isExpanded) {
                        when (aggregateStatus) {
                            BranchAggregateStatus.ACTION -> {
                                Text(
                                    text = "待审批".loc,
                                    color = colors.accentBlue,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Bold,
                                    maxLines = 1,
                                    softWrap = false,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(5.dp))
                                        .background(colors.accentBlue.copy(alpha = 0.15f))
                                        .padding(horizontal = 5.5.dp, vertical = 1.5.dp)
                                )
                            }
                            BranchAggregateStatus.ERROR -> {
                                Text(
                                    text = "错误".loc,
                                    color = colors.accentRed,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Bold,
                                    maxLines = 1,
                                    softWrap = false,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(5.dp))
                                        .background(colors.accentRed.copy(alpha = 0.15f))
                                        .padding(horizontal = 5.5.dp, vertical = 1.5.dp)
                                )
                            }
                            BranchAggregateStatus.WORKING -> {
                                Row(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(5.dp))
                                        .background(colors.accentGreen.copy(alpha = 0.15f))
                                        .padding(horizontal = 5.5.dp, vertical = 1.5.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(3.5.dp)
                                ) {
                                    CircularProgressIndicator(
                                        modifier = Modifier.size(9.dp),
                                        strokeWidth = 1.4.dp,
                                        color = colors.accentGreen
                                    )
                                    Text(
                                        text = "工作中".loc,
                                        color = colors.accentGreen,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.Bold,
                                        maxLines = 1,
                                        softWrap = false
                                    )
                                }
                            }
                            BranchAggregateStatus.DRAFT -> {
                                Text(
                                    text = "草稿".loc,
                                    color = colors.accentYellow,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Bold,
                                    maxLines = 1,
                                    softWrap = false,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(5.dp))
                                        .background(colors.accentYellow.copy(alpha = 0.15f))
                                        .padding(horizontal = 5.dp, vertical = 1.5.dp)
                                )
                            }
                            BranchAggregateStatus.UNREAD -> {
                                UnreadDot(modifier = Modifier.size(14.dp))
                            }
                            BranchAggregateStatus.NONE -> Unit
                        }
                    } else {
                        Text(
                            text = "收起".loc,
                            color = colors.textSecondary,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Medium,
                            maxLines = 1,
                            softWrap = false
                        )
                    }

                    Icon(
                        imageVector = Icons.Default.KeyboardArrowDown,
                        contentDescription = null,
                        tint = colors.textSecondary.copy(alpha = 0.85f),
                        modifier = Modifier
                            .size(15.dp)
                            .graphicsLayer { rotationZ = chevronRotation }
                    )
                }
            }

            AnimatedVisibility(
                visible = isExpanded,
                enter = expandVertically(
                    animationSpec = tween(durationMillis = 260, easing = FastOutSlowInEasing),
                    expandFrom = Alignment.Top
                ) + fadeIn(animationSpec = tween(durationMillis = 200)),
                exit = shrinkVertically(
                    animationSpec = tween(durationMillis = 220, easing = FastOutSlowInEasing),
                    shrinkTowards = Alignment.Top
                ) + fadeOut(animationSpec = tween(durationMillis = 150))
            ) {
                Column(modifier = Modifier.fillMaxWidth()) {
                    branches.forEachIndexed { idx, child ->
                        val isLast = idx == branches.lastIndex
                        val bottomPad = if (isLast) 8.dp else 6.dp
                        val childHasDraft = child.isDraft || hasDraftFor(child.id)

                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .drawBehind {
                                    val spineX = 16.dp.toPx()
                                    val endX = 27.dp.toPx()
                                    val strokePx = 1.5.dp.toPx()
                                    val bottomPadPx = bottomPad.toPx()
                                    val cardHeight = (size.height - bottomPadPx).coerceAtLeast(1f)
                                    val midY = cardHeight / 2f
                                    val r = 6.dp.toPx().coerceAtMost((midY - 1f).coerceAtLeast(0f)).coerceAtMost(endX - spineX)

                                    val elbowPath = Path().apply {
                                        if (!isLast) {
                                            moveTo(spineX, (midY - r).coerceAtLeast(0f))
                                            quadraticTo(spineX, midY, spineX + r, midY)
                                            lineTo(endX, midY)
                                        } else {
                                            moveTo(spineX, -0.5f)
                                            lineTo(spineX, (midY - r).coerceAtLeast(0f))
                                            quadraticTo(spineX, midY, spineX + r, midY)
                                            lineTo(endX, midY)
                                        }
                                    }
                                    if (!isLast) {
                                        drawLine(
                                            color = connectorColor,
                                            start = androidx.compose.ui.geometry.Offset(spineX, -0.5f),
                                            end = androidx.compose.ui.geometry.Offset(spineX, size.height + 0.5f),
                                            strokeWidth = strokePx,
                                            cap = StrokeCap.Butt
                                        )
                                    }
                                    drawPath(
                                        path = elbowPath,
                                        color = connectorColor,
                                        style = Stroke(width = strokePx, cap = StrokeCap.Round, join = StrokeJoin.Round)
                                    )
                                }
                                .padding(start = 28.dp, end = 8.dp, bottom = bottomPad)
                        ) {
                            SwipeableConversationCard(
                                conversation = child,
                                hasDraft = childHasDraft,
                                isEmbeddedBranchChild = true,
                                isSwiped = (swipedConversationId == child.id),
                                onSwipeOpened = { onSwipedConversationIdChange(child.id) },
                                onSwipeClosed = {
                                    if (swipedConversationId == child.id) {
                                        onSwipedConversationIdChange(null)
                                    }
                                },
                                onStartDrag = {
                                    if (swipedConversationId != null && swipedConversationId != child.id) {
                                        onSwipedConversationIdChange(null)
                                    }
                                },
                                onClick = { onSelectChild(child) },
                                onWorkspaceTap = { onSelectChild(child) },
                                onLongClick = { onLongClickChild(child) },
                                onDelete = { onDeleteChild(child) },
                                onPin = { onPinChild(child) },
                                onArchive = { onArchiveChild(child) }
                            )
                        }
                    }
                }
            }
        }

        // Top tucked lip hairline + subtle recess shadow directly beneath the parent card
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .align(Alignment.TopCenter)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(0.5.dp)
                    .background(topHairlineColor)
            )
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(3.5.dp)
                    .background(
                        Brush.verticalGradient(
                            colors = listOf(topShadowColor, Color.Transparent)
                        )
                    )
            )
        }
    }
}

@Composable
private fun SwipeableConversationCard(
    conversation: ConversationItem,
    hasDraft: Boolean,
    isEmbeddedBranchChild: Boolean = false,
    isSwiped: Boolean,
    onSwipeOpened: () -> Unit,
    onSwipeClosed: () -> Unit,
    onStartDrag: () -> Unit,
    onClick: () -> Unit,
    onWorkspaceTap: () -> Unit,
    onLongClick: () -> Unit,
    onDelete: () -> Unit,
    onPin: () -> Unit,
    onArchive: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val coroutineScope = rememberCoroutineScope()
    val leadingWidthDp = if (isEmbeddedBranchChild) 54.dp else 72.dp
    val trailingWidthDp = if (isEmbeddedBranchChild) {
        if (conversation.isDraft) 54.dp else 96.dp
    } else {
        if (conversation.isDraft) 72.dp else 124.dp
    }
    val actionCircleSize = if (isEmbeddedBranchChild) 30.dp else 40.dp
    val actionIconSize = if (isEmbeddedBranchChild) 15.dp else 20.dp
    val clipShape = RoundedCornerShape(if (isEmbeddedBranchChild) 10.dp else 14.dp)
    val maxLeftRevealPx = with(density) { leadingWidthDp.toPx() }
    val maxRightRevealPx = with(density) { trailingWidthDp.toPx() }
    val offsetX = remember { Animatable(0f) }
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()

    LaunchedEffect(isSwiped) {
        if (!isSwiped && offsetX.value != 0f) {
            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
        }
    }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(clipShape),
        contentAlignment = Alignment.Center
    ) {
        // Background Left: iOS-style circular orange pin button (only composed while swiping right)
        if (offsetX.value > 0.5f) {
            Box(
                modifier = Modifier.matchParentSize(),
                contentAlignment = Alignment.CenterStart
            ) {
                Box(
                    modifier = Modifier
                        .width(leadingWidthDp)
                        .fillMaxHeight(),
                    contentAlignment = Alignment.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(actionCircleSize)
                            .clip(CircleShape)
                            .background(if (conversation.isPinned) colors.textMuted else colors.accentOrange)
                            .clickable {
                                haptic.medium()
                                onSwipeClosed()
                                coroutineScope.launch {
                                    offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                }
                                onPin()
                            },
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.PushPin,
                            contentDescription = if (conversation.isPinned) "取消置顶".loc else "置顶".loc,
                            tint = Color.White,
                            modifier = Modifier.size(actionIconSize)
                        )
                    }
                }
            }
        }

        // Background Right: archive + delete circular buttons (only composed while swiping left)
        if (offsetX.value < -0.5f) {
            Box(
                modifier = Modifier.matchParentSize(),
                contentAlignment = Alignment.CenterEnd
            ) {
                Row(
                    modifier = Modifier
                        .width(trailingWidthDp)
                        .fillMaxHeight()
                        .padding(horizontal = if (isEmbeddedBranchChild) 6.dp else 10.dp),
                    horizontalArrangement = Arrangement.spacedBy(if (isEmbeddedBranchChild) 8.dp else 10.dp, Alignment.CenterHorizontally),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    if (!conversation.isDraft) {
                        Box(
                            modifier = Modifier
                                .size(actionCircleSize)
                                .clip(CircleShape)
                                .background(if (conversation.isArchived) colors.accentIndigo else Color(0xFF64748B))
                                .clickable {
                                    haptic.medium()
                                    onSwipeClosed()
                                    coroutineScope.launch {
                                        offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                    }
                                    onArchive()
                                },
                            contentAlignment = Alignment.Center
                        ) {
                            Icon(
                                imageVector = if (conversation.isArchived) Icons.Default.Unarchive else Icons.Default.Archive,
                                contentDescription = if (conversation.isArchived) "取消归档".loc else "归档".loc,
                                tint = Color.White,
                                modifier = Modifier.size(if (isEmbeddedBranchChild) 15.dp else 19.dp)
                            )
                        }
                    }
                    Box(
                        modifier = Modifier
                            .size(actionCircleSize)
                            .clip(CircleShape)
                            .background(colors.accentRed)
                            .clickable {
                                haptic.medium()
                                onSwipeClosed()
                                coroutineScope.launch {
                                    offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                }
                                onDelete()
                            },
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Delete,
                            contentDescription = "删除".loc,
                            tint = Color.White,
                            modifier = Modifier.size(actionIconSize)
                        )
                    }
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
                        onStartDrag()
                        coroutineScope.launch {
                            val newOffset = (offsetX.value + delta).coerceIn(-maxRightRevealPx, maxLeftRevealPx)
                            offsetX.snapTo(newOffset)
                        }
                    },
                    orientation = Orientation.Horizontal,
                    onDragStarted = {
                        onStartDrag()
                    },
                    onDragStopped = { velocity ->
                        coroutineScope.launch {
                            val targetOffset = when {
                                velocity < -500f || offsetX.value < -maxRightRevealPx / 2 -> {
                                    haptic.light()
                                    -maxRightRevealPx
                                }
                                velocity > 500f || offsetX.value > maxLeftRevealPx / 2 -> {
                                    haptic.light()
                                    maxLeftRevealPx
                                }
                                else -> 0f
                            }
                            if (targetOffset != 0f) {
                                onSwipeOpened()
                            } else {
                                onSwipeClosed()
                            }
                            offsetX.animateTo(targetOffset, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    }
                )
        ) {
            ConversationCard(
                conversation = conversation,
                hasDraft = hasDraft,
                isEmbeddedBranchChild = isEmbeddedBranchChild,
                onClick = {
                    if (offsetX.value != 0f) {
                        onSwipeClosed()
                        coroutineScope.launch {
                            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    } else {
                        onClick()
                    }
                },
                onWorkspaceTap = {
                    if (offsetX.value != 0f) {
                        onSwipeClosed()
                        coroutineScope.launch {
                            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    } else {
                        onWorkspaceTap()
                    }
                },
                onLongClick = {
                    if (offsetX.value != 0f) {
                        onSwipeClosed()
                        coroutineScope.launch {
                            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                        }
                    } else if (!conversation.isDraft) {
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
    isEmbeddedBranchChild: Boolean = false,
    onClick: () -> Unit,
    onWorkspaceTap: (() -> Unit)? = null,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors

    if (conversation.isSideQuestion) {
        if (isEmbeddedBranchChild) {
            val innerCardBg = if (colors.isDark) colors.surfaceVariant else colors.surface
            val innerCardBorder = if (colors.isDark) {
                Color.White.copy(alpha = 0.08f)
            } else {
                colors.border.copy(alpha = 0.45f)
            }
            Card(
                modifier = modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .border(0.5.dp, innerCardBorder, RoundedCornerShape(10.dp))
                    .combinedClickable(
                        onClick = onClick,
                        onLongClick = onLongClick
                    ),
                shape = RoundedCornerShape(10.dp),
                colors = CardDefaults.cardColors(containerColor = innerCardBg),
                elevation = CardDefaults.cardElevation(defaultElevation = if (colors.isDark) 0.dp else 0.5.dp)
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 10.dp, vertical = 8.5.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(5.dp)
                ) {
                    if (conversation.isPinned) {
                        Icon(
                            imageVector = Icons.Default.PushPin,
                            contentDescription = "置顶".loc,
                            tint = colors.accentOrange,
                            modifier = Modifier.size(11.dp)
                        )
                    }
                    if (conversation.isStarred) {
                        Icon(
                            imageVector = Icons.Default.Star,
                            contentDescription = "星标".loc,
                            tint = colors.accentYellow,
                            modifier = Modifier.size(11.dp)
                        )
                    }
                    Text(
                        text = conversation.displayTitle,
                        color = colors.textPrimary,
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.Medium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    val timeStr = conversation.relativeTimeString
                    if (timeStr.isNotBlank()) {
                        Text(
                            text = timeStr,
                            color = colors.textSecondary,
                            fontSize = 11.sp,
                            maxLines = 1,
                            softWrap = false
                        )
                    }
                    if (conversation.isWorking || conversation.status.needsAction || conversation.status.isError) {
                        StatusBadge(status = conversation.status, hasRunningTasks = conversation.hasRunningTasks)
                    } else if (hasDraft || conversation.isDraft) {
                        DraftBadge()
                    } else if (conversation.isUnread) {
                        UnreadDot(modifier = Modifier.size(14.dp))
                    }
                }
            }
            return
        }

        Card(
            modifier = modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.24f), RoundedCornerShape(12.dp))
                .combinedClickable(
                    onClick = onClick,
                    onLongClick = onLongClick
                ),
            shape = RoundedCornerShape(12.dp),
            colors = CardDefaults.cardColors(containerColor = colors.surface)
        ) {
            Column(
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
                verticalArrangement = Arrangement.spacedBy(5.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Row(
                        modifier = Modifier.weight(1f).padding(end = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.CallSplit,
                            contentDescription = "旁路快问".loc,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(13.dp)
                        )
                        Text(
                            text = "旁路".loc,
                            color = colors.accentIndigo,
                            fontSize = 10.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            softWrap = false,
                            modifier = Modifier
                                .clip(RoundedCornerShape(999.dp))
                                .background(colors.accentIndigo.copy(alpha = 0.12f))
                                .padding(horizontal = 5.dp, vertical = 1.dp)
                        )
                        Text(
                            text = conversation.displayTitle,
                            color = colors.textPrimary,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    if (conversation.isWorking || conversation.status.needsAction || conversation.status.isError) {
                        StatusBadge(status = conversation.status, hasRunningTasks = conversation.hasRunningTasks)
                    } else if (hasDraft || conversation.isDraft) {
                        DraftBadge()
                    } else if (conversation.isUnread) {
                        UnreadDot()
                    }
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    val subLabel = if (!conversation.branchParentTitle.isNullOrBlank()) {
                        "来自「%s」".loc(conversation.branchParentTitle)
                    } else {
                        conversation.workspaceName
                    }
                    Text(
                        text = subLabel,
                        color = colors.textSecondary,
                        fontSize = 11.5.sp,
                        fontFamily = FontFamily.Default,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false)
                    )
                    val timeStr = conversation.relativeTimeString
                    if (timeStr.isNotBlank()) {
                        Text(
                            text = timeStr,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp
                        )
                    }
                }
            }
        }
        return
    }

    val indentPadding = (conversation.nestingDepth.coerceAtMost(3) * 14).dp

    Card(
        modifier = modifier
            .fillMaxWidth()
            .padding(start = indentPadding)
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
                Row(
                    modifier = Modifier.weight(1f).padding(end = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    if (conversation.isPinned) {
                        Icon(
                            imageVector = Icons.Default.PushPin,
                            contentDescription = "已置顶".loc,
                            tint = colors.accentOrange,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                    if (conversation.isStarred) {
                        Icon(
                            imageVector = Icons.Default.Star,
                            contentDescription = "已星标".loc,
                            tint = Color(0xFFF59E0B),
                            modifier = Modifier.size(13.dp)
                        )
                    }
                    if (conversation.nestingDepth > 0 || !conversation.parentConversationId.isNullOrBlank()) {
                        Icon(
                            imageVector = Icons.Default.SubdirectoryArrowRight,
                            contentDescription = "子代理".loc,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(15.dp)
                        )
                    }
                    if (conversation.hasSwarm) {
                        SwarmPixelBeeSprite(
                            name = "coordinator",
                            pose = if (conversation.isSwarmWorking) SwarmBeePose.WORKING else SwarmBeePose.LANDED,
                            width = 22.dp,
                            height = 18.dp,
                            contentDescription = "蜂群会话".loc
                        )
                    }
                    Text(
                        text = conversation.displayTitle,
                        color = colors.textPrimary,
                        fontSize = 15.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                if (conversation.isWorking || conversation.status.needsAction || conversation.status.isError) {
                    StatusBadge(status = conversation.status, hasRunningTasks = conversation.hasRunningTasks)
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
                    modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp),
                        modifier = Modifier.weight(1f, fill = false)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(24.dp)
                                .then(
                                    if (onWorkspaceTap != null) {
                                        Modifier.clickable(
                                            interactionSource = remember { MutableInteractionSource() },
                                            indication = null,
                                            onClick = onWorkspaceTap
                                        )
                                    } else {
                                        Modifier
                                    }
                                ),
                            contentAlignment = Alignment.Center
                        ) {
                            Icon(
                                imageVector = if (conversation.isPureChat) Icons.Default.ChatBubbleOutline else Icons.Default.Folder,
                                contentDescription = "Workspace",
                                tint = colors.textMuted,
                                modifier = Modifier.size(13.dp)
                            )
                        }
                        Text(
                            text = conversation.workspaceName,
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            fontFamily = FontFamily.Monospace,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    conversation.pullRequest?.let { pr ->
                        GitHubPRPillBadge(item = pr, compact = true)
                    }
                }

                val timeStr = conversation.relativeTimeString
                val metaText = if (conversation.isDraft) {
                    if (timeStr.isNotBlank()) "草稿 · %s".loc(timeStr) else "草稿".loc
                } else {
                    if (timeStr.isNotBlank()) "${conversation.stepCount} 步骤 · $timeStr".loc else "${conversation.stepCount} 步骤".loc
                }

                Text(
                    text = metaText,
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    maxLines = 1,
                    softWrap = false
                )
            }
        }
    }
}
