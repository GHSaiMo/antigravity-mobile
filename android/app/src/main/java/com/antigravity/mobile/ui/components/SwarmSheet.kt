package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.material3.TabRowDefaults.tabIndicatorOffset
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SwarmSheet(
    cascadeId: String,
    apiClient: ApiClient,
    onSelectConversation: (String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val isDark = AntigravityTheme.colors.isDark
    val scope = rememberCoroutineScope()
    val lifecycleOwner = androidx.lifecycle.compose.LocalLifecycleOwner.current
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    var selectedTab by remember { mutableIntStateOf(0) } // 0: 概览, 1: 协作板, 2: 成员, 3: 拓扑
    val tabs = listOf("概览".loc, "协作板".loc, "成员".loc, "拓扑".loc)

    var snapshot by remember { mutableStateOf<SwarmSnapshot?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var isLoadingEarlier by remember { mutableStateOf(false) }
    var lastETag by remember { mutableStateOf<String?>(null) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    val swarmPageSize = 80

    // Overview feedback
    var feedbackText by remember { mutableStateOf("") }
    var isSendingFeedback by remember { mutableStateOf(false) }
    val nudgingMembers = remember { mutableStateListOf<String>() }

    // Board filter & expand
    var boardSearch by remember { mutableStateOf("") }
    var boardAgentFilter by remember { mutableStateOf<String?>(null) }
    val expandedSeqs = remember { mutableStateListOf<Int>() }

    // Members filter
    var memberSearch by remember { mutableStateOf("") }
    var showRetiredMembers by remember { mutableStateOf(false) }
    var selectedMemberForActions by remember { mutableStateOf<String?>(null) }

    fun loadData(silent: Boolean = false) {
        scope.launch {
            if (!silent) {
                isLoading = true
                errorMessage = null
            }
            val existing = snapshot
            val latestSeq = existing?.messages?.lastOrNull()?.seq ?: 0
            if (silent && existing != null && latestSeq > 0) {
                val res = apiClient.fetchSwarmSnapshotIfModified(
                    cascadeId = cascadeId,
                    afterSeq = latestSeq,
                    etag = lastETag
                )
                res.onSuccess { delta ->
                    if (delta != null) {
                        if (delta.etag.isNotBlank()) lastETag = delta.etag
                        snapshot = existing.mergingMessages(delta, isOlderPage = false)
                    }
                }.onFailure {
                    if (snapshot == null) {
                        errorMessage = it.message ?: "加载快照失败".loc
                    }
                }
            } else {
                val res = apiClient.fetchSwarmSnapshot(
                    cascadeId = cascadeId,
                    limit = swarmPageSize
                )
                res.onSuccess {
                    if (it.etag.isNotBlank()) lastETag = it.etag
                    snapshot = it
                    if (!silent) errorMessage = null
                }.onFailure {
                    if (!silent || snapshot == null) {
                        errorMessage = it.message ?: "加载快照失败".loc
                    }
                }
            }
            if (!silent) isLoading = false
        }
    }

    fun loadEarlierMessages() {
        val existing = snapshot ?: return
        if (isLoadingEarlier || !existing.hasMoreBefore) return
        val oldest = if (existing.oldestSeq > 0) existing.oldestSeq else (existing.messages.firstOrNull()?.seq ?: 0)
        if (oldest <= 0) return
        scope.launch {
            isLoadingEarlier = true
            val res = apiClient.fetchSwarmSnapshot(
                cascadeId = cascadeId,
                beforeSeq = oldest,
                limit = swarmPageSize
            )
            res.onSuccess { older ->
                snapshot = existing.mergingMessages(older, isOlderPage = true)
            }
            isLoadingEarlier = false
        }
    }

    LaunchedEffect(cascadeId) {
        lastETag = null
        loadData(silent = false)
        while (true) {
            delay(3000)
            if (!lifecycleOwner.lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.RESUMED)) {
                continue
            }
            loadData(silent = true)
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = SwarmColors.bg(isDark), onDismiss = onDismiss) {
            BackHandler { onDismiss() }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .fillMaxHeight()
                    .background(SwarmColors.bg(isDark))
            ) {
                SheetGrabHandle(color = SwarmColors.border(isDark))

                // Top Header Bar (No "完成" Button)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 12.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = snapshot?.title?.ifBlank { "蜂群协同 (Swarm)".loc } ?: "蜂群协同".loc,
                        color = SwarmColors.ink(isDark),
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )

                    Spacer(modifier = Modifier.width(8.dp))

                    Row(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(SwarmColors.cardBg(isDark))
                            .clickable { loadData(silent = false) }
                            .padding(horizontal = 8.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(7.dp)
                                .clip(CircleShape)
                                .background(if (isLoading) SwarmColors.warnAmber(isDark) else SwarmColors.liveGreen(isDark))
                        )
                        Text(
                            text = "Live",
                            color = SwarmColors.muted(isDark),
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium
                        )
                    }
                }

                // Desktop-style Underline Tab Row
                val boardCount = snapshot?.messages?.count { !it.text.startsWith("[channel]") } ?: 0
                val memberCount = snapshot?.members?.count { !it.roleWorker && !isRoleWorker(it.name, it.role) } ?: 0

                TabRow(
                    selectedTabIndex = selectedTab,
                    containerColor = SwarmColors.bg(isDark),
                    contentColor = SwarmColors.accent(isDark),
                    indicator = { tabPositions ->
                        TabRowDefaults.SecondaryIndicator(
                            Modifier.tabIndicatorOffset(tabPositions[selectedTab]),
                            color = SwarmColors.accent(isDark),
                            height = 2.dp
                        )
                    },
                    divider = {
                        HorizontalDivider(thickness = 0.8.dp, color = SwarmColors.border(isDark))
                    },
                    modifier = Modifier.fillMaxWidth()
                ) {
                    tabs.forEachIndexed { index, title ->
                        val countText = when (index) {
                            1 -> if (boardCount > 0) " $boardCount" else ""
                            2 -> if (memberCount > 0) " $memberCount" else ""
                            else -> ""
                        }
                        Tab(
                            selected = selectedTab == index,
                            selectedContentColor = SwarmColors.ink(isDark),
                            unselectedContentColor = SwarmColors.muted(isDark),
                            onClick = { selectedTab = index },
                            text = {
                                Text(
                                    text = "$title$countText",
                                    fontSize = 13.5.sp,
                                    fontWeight = if (selectedTab == index) FontWeight.SemiBold else FontWeight.Normal
                                )
                            }
                        )
                    }
                }

                // Content Views
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                ) {
                    when {
                        isLoading && snapshot == null -> {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator(color = SwarmColors.accent(isDark), strokeWidth = 2.5.dp)
                            }
                        }
                        errorMessage != null && snapshot == null -> {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                    Text("加载蜂群快照失败".loc, color = SwarmColors.ink(isDark), fontSize = 15.sp, fontWeight = FontWeight.Bold)
                                    Text(errorMessage ?: "", color = SwarmColors.muted(isDark), fontSize = 12.5.sp)
                                    Button(
                                        onClick = { loadData(silent = false) },
                                        colors = ButtonDefaults.buttonColors(containerColor = SwarmColors.accent(isDark))
                                    ) {
                                        Text("重试".loc, color = Color.White)
                                    }
                                }
                            }
                        }
                        snapshot != null -> {
                            val snap = snapshot!!
                            when (selectedTab) {
                                0 -> SwarmOverviewTab(
                                    snap = snap,
                                    feedbackText = feedbackText,
                                    isSendingFeedback = isSendingFeedback,
                                    onFeedbackChange = { feedbackText = it },
                                    onSendFeedback = {
                                        scope.launch {
                                            isSendingFeedback = true
                                            apiClient.sendSwarmFeedback(cascadeId, feedbackText, snap.coordinatorId)
                                            feedbackText = ""
                                            loadData(silent = true)
                                            isSendingFeedback = false
                                        }
                                    },
                                    onSelectConversation = { convId ->
                                        onDismiss()
                                        onSelectConversation(convId)
                                    },
                                    onSwitchToTab = { selectedTab = it }
                                )
                                1 -> SwarmBoardTabNative(
                                    snap = snap,
                                    search = boardSearch,
                                    agentFilter = boardAgentFilter,
                                    expandedSeqs = expandedSeqs,
                                    isLoadingEarlier = isLoadingEarlier,
                                    onLoadEarlier = { loadEarlierMessages() },
                                    onSearchChange = { boardSearch = it },
                                    onFilterChange = { boardAgentFilter = it },
                                    onToggleExpand = { seq ->
                                        if (expandedSeqs.contains(seq)) expandedSeqs.remove(seq) else expandedSeqs.add(seq)
                                    }
                                )
                                2 -> SwarmMembersTabNative(
                                    snap = snap,
                                    search = memberSearch,
                                    showRetired = showRetiredMembers,
                                    selectedMember = selectedMemberForActions,
                                    nudgingMembers = nudgingMembers,
                                    onSearchChange = { memberSearch = it },
                                    onToggleRetired = { showRetiredMembers = !showRetiredMembers },
                                    onSelectMemberRow = { name ->
                                        selectedMemberForActions = if (selectedMemberForActions == name) null else name
                                    },
                                    onNudge = { name ->
                                        scope.launch {
                                            nudgingMembers.add(name)
                                            apiClient.nudgeSwarmMember(cascadeId, name, snap.coordinatorId)
                                            delay(1000)
                                            nudgingMembers.remove(name)
                                            loadData(silent = true)
                                        }
                                    },
                                    onSelectConversation = onSelectConversation
                                )
                                3 -> SwarmGraphTab(
                                    snap = snap,
                                    onSelectConversation = onSelectConversation
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

// MARK: - Overview Tab (Goal, Running, Progress Ring, Threads, The Team + Feedback)

@Composable
private fun SwarmOverviewTab(
    snap: SwarmSnapshot,
    feedbackText: String,
    isSendingFeedback: Boolean,
    onFeedbackChange: (String) -> Unit,
    onSendFeedback: () -> Unit,
    onSelectConversation: (String) -> Unit,
    onSwitchToTab: (Int) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    var filterMember by remember { mutableStateOf<String?>(null) }
    var showAllRunning by remember { mutableStateOf(false) }
    var showDoneTasks by remember { mutableStateOf(false) }
    val expandedThreadIds = remember { mutableStateListOf<String>() }

    val synth = remember(snap, filterMember) {
        SwarmHiveEngine.synthesize(snap, filterMember)
    }
    val runningItems = remember(synth.tasks) {
        synth.tasks.filter { it.state == SwarmTaskBucketState.RUNNING || it.state == SwarmTaskBucketState.BLOCKED }
            .sortedBy { if (it.state == SwarmTaskBucketState.BLOCKED) 0 else 1 }
    }
    val doneItems = remember(synth.tasks) {
        synth.tasks.filter { it.state == SwarmTaskBucketState.DONE }.sortedByDescending { it.seq }
    }
    val queuedItems = remember(synth.tasks) {
        synth.tasks.filter { it.state == SwarmTaskBucketState.QUEUED }
    }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        // 1. Stall Banner
        if (synth.stallText != null && synth.stallSeq != null) {
            item(key = "stall") {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(11.dp))
                        .background(SwarmColors.warnAmber(isDark).copy(alpha = 0.10f))
                        .border(0.8.dp, SwarmColors.warnAmber(isDark).copy(alpha = 0.45f), RoundedCornerShape(11.dp))
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(5.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                        Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(SwarmColors.warnAmber(isDark)))
                        Text("Swarm stalled — no agents working", color = SwarmColors.ink(isDark), fontSize = 13.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(SwarmColors.warnAmber(isDark).copy(alpha = 0.16f))
                                .clickable { onSwitchToTab(1) }
                                .padding(horizontal = 7.dp, vertical = 2.dp)
                        ) {
                            Text("@${synth.stallAgent ?: "monitor"} #${synth.stallSeq}", color = SwarmColors.warnAmber(isDark), fontSize = 11.sp, fontFamily = FontFamily.Monospace)
                        }
                    }
                    Text(synth.stallText, color = SwarmColors.muted(isDark), fontSize = 12.sp, modifier = Modifier.padding(start = 15.dp))
                }
            }
        }

        // 2. GOAL Card
        if (synth.goalTitle.isNotBlank() || synth.goalLine.isNotBlank()) {
            item(key = "goal") {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(SwarmColors.surfaceBg(isDark))
                        .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
                        .padding(horizontal = 14.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Text("GOAL", color = SwarmColors.muted(isDark), fontSize = 11.sp, fontWeight = FontWeight.SemiBold, letterSpacing = 0.6.sp)
                    if (synth.goalTitle.isNotBlank()) {
                        Text(synth.goalTitle, color = SwarmColors.ink(isDark), fontSize = 15.5.sp, fontWeight = FontWeight.SemiBold, lineHeight = 21.sp)
                    }
                    if (synth.goalLine.isNotBlank()) {
                        Text(synth.goalLine, color = SwarmColors.muted(isDark), fontSize = 13.sp, lineHeight = 18.sp)
                    }
                }
            }
        }

        // 3. Running Tasks Card
        if (runningItems.isNotEmpty() && !synth.finished) {
            item(key = "running") {
                val visibleRun = if (showAllRunning) runningItems else runningItems.take(8)
                val extraRun = (runningItems.size - visibleRun.size).coerceAtLeast(0)
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(SwarmColors.surfaceBg(isDark))
                        .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
                        .padding(horizontal = 14.dp, vertical = 12.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
                        Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(SwarmColors.accent(isDark)))
                        Text("Running", color = SwarmColors.ink(isDark), fontSize = 13.5.sp, fontWeight = FontWeight.SemiBold)
                        Text("${runningItems.size}", color = SwarmColors.muted(isDark), fontSize = 12.5.sp, fontFamily = FontFamily.Monospace)
                        Spacer(modifier = Modifier.weight(1f))
                        if (!synth.phaseLabel.isNullOrBlank()) {
                            Box(modifier = Modifier.clip(CircleShape).background(SwarmColors.fieldBg(isDark)).padding(horizontal = 8.dp, vertical = 2.dp)) {
                                Text(synth.phaseLabel, color = SwarmColors.muted(isDark), fontSize = 11.sp, fontWeight = FontWeight.Medium)
                            }
                        }
                    }

                    if (synth.runningSummary.isNotBlank()) {
                        Text(synth.runningSummary, color = SwarmColors.muted(isDark), fontSize = 12.5.sp, modifier = Modifier.padding(bottom = 8.dp))
                    }

                    visibleRun.forEachIndexed { idx, task ->
                        if (idx > 0) {
                            HorizontalDivider(thickness = 0.6.dp, color = SwarmColors.border(isDark).copy(alpha = 0.65f))
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(vertical = 8.5.dp),
                            verticalAlignment = Alignment.Top,
                            horizontalArrangement = Arrangement.spacedBy(9.dp)
                        ) {
                            OverviewTaskIndicatorAndroid(state = task.state, modifier = Modifier.padding(top = 3.dp))
                            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                    Text(task.title, color = SwarmColors.ink(isDark), fontSize = 13.5.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                                    if (task.rawState == "review") {
                                        Box(modifier = Modifier.clip(CircleShape).background(SwarmColors.accent(isDark).copy(alpha = 0.12f)).padding(horizontal = 6.5.dp, vertical = 2.dp)) {
                                            Text("In review", color = SwarmColors.accent(isDark), fontSize = 11.sp, fontWeight = FontWeight.Medium)
                                        }
                                    }
                                    WorkerBeeChipsAndroid(owners = task.owners, synth = synth, snap = snap, onSelectConversation = onSelectConversation)
                                }
                                val sub = task.waitingOn?.let { "Waiting on @$it" } ?: (task.liveActivity ?: task.note)
                                if (sub.isNotBlank()) {
                                    Text(
                                        text = sub,
                                        color = if (task.waitingOn != null) SwarmColors.warnAmber(isDark) else if (task.liveActivity != null) SwarmColors.muted(isDark) else SwarmColors.faint(isDark),
                                        fontSize = 12.sp,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }
                        }
                    }

                    if (extraRun > 0) {
                        Text(
                            text = "+$extraRun more",
                            color = SwarmColors.muted(isDark),
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium,
                            modifier = Modifier.padding(top = 4.dp).clickable { showAllRunning = true }
                        )
                    }
                }
            }
        }

        // 4. Compact Expandable "X of Y tasks done" Progress Card
        if (synth.totalTasksCount > 0) {
            item(key = "progress") {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(11.dp))
                        .background(SwarmColors.surfaceBg(isDark))
                        .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(11.dp))
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { showDoneTasks = !showDoneTasks }
                            .padding(horizontal = 13.dp, vertical = 10.5.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(9.dp)
                    ) {
                        OverviewProgressRingAndroid(
                            done = synth.doneCount,
                            running = synth.runningCount,
                            blocked = synth.blockedCount,
                            total = synth.totalTasksCount
                        )
                        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text("${synth.doneCount} of ${synth.totalTasksCount}", color = SwarmColors.ink(isDark), fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                            Text("tasks done", color = SwarmColors.muted(isDark), fontSize = 13.sp)
                        }
                        Spacer(modifier = Modifier.weight(1f))
                        if (synth.stuckCount > 0) {
                            ProgressStatusDotLabel("${synth.stuckCount} stuck", SwarmColors.errorRed(isDark))
                        } else if (synth.waitingChips.isNotEmpty()) {
                            ProgressStatusDotLabel(synth.waitingChips.first(), SwarmColors.warnAmber(isDark))
                        }
                        if (synth.reviewCount > 0) {
                            ProgressStatusDotLabel("${synth.reviewCount} in review", SwarmColors.accent(isDark))
                        }
                        if (synth.openThreadsCount > 0) {
                            Text("${synth.openThreadsCount} ${if (synth.openThreadsCount == 1) "thread" else "threads"} open", color = SwarmColors.muted(isDark), fontSize = 12.sp, maxLines = 1)
                        }
                        Icon(
                            imageVector = if (showDoneTasks) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                            contentDescription = null,
                            tint = SwarmColors.faint(isDark),
                            modifier = Modifier.size(16.dp)
                        )
                    }

                    AnimatedVisibility(visible = showDoneTasks) {
                        Column(modifier = Modifier.fillMaxWidth()) {
                            HorizontalDivider(thickness = 0.7.dp, color = SwarmColors.border(isDark))
                            Column(modifier = Modifier.padding(horizontal = 13.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                if (doneItems.isEmpty()) {
                                    Text("No tasks completed yet.", color = SwarmColors.muted(isDark), fontSize = 12.5.sp)
                                } else {
                                    doneItems.forEach { item ->
                                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                            OverviewTaskIndicatorAndroid(SwarmTaskBucketState.DONE)
                                            Column(modifier = Modifier.weight(1f)) {
                                                Text(item.title, color = SwarmColors.ink(isDark), fontSize = 12.5.sp, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                                                if (item.note.isNotBlank()) {
                                                    Text(item.note, color = SwarmColors.muted(isDark), fontSize = 11.5.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                                }
                                            }
                                            WorkerBeeChipsAndroid(item.owners, synth, snap, onSelectConversation)
                                        }
                                    }
                                }
                                if (queuedItems.isNotEmpty()) {
                                    HorizontalDivider(thickness = 0.6.dp, color = SwarmColors.border(isDark).copy(alpha = 0.6f), modifier = Modifier.padding(vertical = 2.dp))
                                    Text("UP NEXT · ${queuedItems.size}", color = SwarmColors.muted(isDark), fontSize = 10.5.sp, fontWeight = FontWeight.SemiBold)
                                    queuedItems.take(6).forEach { item ->
                                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                            OverviewTaskIndicatorAndroid(SwarmTaskBucketState.QUEUED)
                                            Text(item.title, color = SwarmColors.ink(isDark), fontSize = 12.5.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                                            WorkerBeeChipsAndroid(item.owners, synth, snap, onSelectConversation)
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        // 5. Threads Section
        if (synth.threads.isNotEmpty()) {
            item(key = "threads_header") {
                Text("Threads", color = SwarmColors.ink(isDark), fontSize = 13.5.sp, fontWeight = FontWeight.SemiBold)
            }
            items(synth.threads, key = { "th_${it.id}" }) { thread ->
                val defaultOpen = thread.state == SwarmThreadState.NEEDS || thread.state == SwarmThreadState.WAITING || thread.state == SwarmThreadState.STUCK
                val isOpen = expandedThreadIds.contains(thread.id) || (defaultOpen && !expandedThreadIds.contains("closed:${thread.id}"))
                val dotCol = when (thread.state) {
                    SwarmThreadState.NEEDS, SwarmThreadState.WAITING -> SwarmColors.warnAmber(isDark)
                    SwarmThreadState.STUCK -> SwarmColors.errorRed(isDark)
                    SwarmThreadState.RUNNING -> SwarmColors.accent(isDark)
                    SwarmThreadState.DONE -> SwarmColors.liveGreen(isDark)
                    SwarmThreadState.QUEUED -> SwarmColors.faint(isDark)
                }
                val borderCol = when (thread.state) {
                    SwarmThreadState.NEEDS, SwarmThreadState.WAITING -> SwarmColors.warnAmber(isDark).copy(alpha = 0.45f)
                    SwarmThreadState.STUCK -> SwarmColors.errorRed(isDark).copy(alpha = 0.40f)
                    else -> SwarmColors.border(isDark)
                }

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(11.dp))
                        .background(if (thread.state == SwarmThreadState.DONE) Color.Transparent else SwarmColors.surfaceBg(isDark))
                        .border(0.8.dp, borderCol, RoundedCornerShape(11.dp))
                        .padding(horizontal = 13.dp, vertical = 11.dp)
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable {
                                if (isOpen) {
                                    expandedThreadIds.remove(thread.id)
                                    if (defaultOpen) expandedThreadIds.add("closed:${thread.id}")
                                } else {
                                    expandedThreadIds.remove("closed:${thread.id}")
                                    expandedThreadIds.add(thread.id)
                                }
                            },
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(dotCol))
                        Row(modifier = Modifier.weight(1f), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            Text(thread.title, color = SwarmColors.ink(isDark), fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            Text(
                                text = thread.statusLabel,
                                color = if (thread.state == SwarmThreadState.WAITING || thread.state == SwarmThreadState.NEEDS) SwarmColors.warnAmber(isDark)
                                else if (thread.state == SwarmThreadState.STUCK) SwarmColors.errorRed(isDark)
                                else SwarmColors.muted(isDark),
                                fontSize = 12.sp,
                                maxLines = 1
                            )
                        }
                        OverviewMiniSegmentedBarAndroid(thread.doneCount, thread.runningCount, thread.blockedCount, thread.queuedCount)
                        Text("${thread.doneCount}/${thread.totalCount}", color = SwarmColors.muted(isDark), fontSize = 12.sp, fontFamily = FontFamily.Monospace)
                        Icon(
                            imageVector = if (isOpen) Icons.Default.KeyboardArrowDown else Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = SwarmColors.faint(isDark),
                            modifier = Modifier.size(15.dp)
                        )
                    }

                    if (thread.goal.isNotBlank()) {
                        Text(thread.goal, color = SwarmColors.muted(isDark), fontSize = 13.sp, modifier = Modifier.padding(start = 16.dp, top = 3.dp))
                    }
                    if (thread.now.isNotBlank()) {
                        Row(modifier = Modifier.padding(start = 16.dp, top = 4.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            if (thread.state != SwarmThreadState.DONE) {
                                Text("Now", color = SwarmColors.ink(isDark).copy(alpha = 0.85f), fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                            }
                            Text(thread.now, color = SwarmColors.muted(isDark), fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    }

                    AnimatedVisibility(visible = isOpen && thread.tasks.isNotEmpty()) {
                        Column(modifier = Modifier.padding(start = 16.dp, top = 8.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
                            HorizontalDivider(thickness = 0.6.dp, color = SwarmColors.border(isDark).copy(alpha = 0.75f))
                            thread.tasks.forEach { task ->
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    OverviewTaskIndicatorAndroid(task.state)
                                    Text(
                                        text = task.title,
                                        color = if (task.state == SwarmTaskBucketState.DONE) SwarmColors.muted(isDark) else SwarmColors.ink(isDark),
                                        fontSize = 12.5.sp,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f)
                                    )
                                    if (task.rawState == "review") {
                                        Box(modifier = Modifier.clip(CircleShape).background(SwarmColors.accent(isDark).copy(alpha = 0.12f)).padding(horizontal = 6.dp, vertical = 1.5.dp)) {
                                            Text("In review", color = SwarmColors.accent(isDark), fontSize = 10.5.sp, fontWeight = FontWeight.Medium)
                                        }
                                    }
                                    WorkerBeeChipsAndroid(task.owners, synth, snap, onSelectConversation)
                                }
                            }
                        }
                    }
                }
            }
        }

        // 6. The Team Section
        item(key = "team") {
            val coord = synth.members["coordinator"]
            val coordPose = if (!synth.finished && coord?.status == "working") SwarmBeePose.WORKING else SwarmBeePose.LANDED
            val crew = synth.orderedCrew
            val maxTiles = 5
            val shownCrew = if (crew.size <= maxTiles + 1) crew else {
                val top = crew.take(maxTiles).toMutableList()
                val sel = filterMember
                if (sel != null && top.none { it.name == sel }) {
                    crew.firstOrNull { it.name == sel }?.let { top[maxTiles - 1] = it }
                }
                top
            }
            val extraCrew = (crew.size - shownCrew.size).coerceAtLeast(0)

            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("The team", color = SwarmColors.ink(isDark), fontSize = 13.5.sp, fontWeight = FontWeight.SemiBold)

                // Coordinator Card
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(11.dp))
                        .background(SwarmColors.surfaceBg(isDark))
                        .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(11.dp))
                        .clickable {
                            val cid = coord?.conversationId.orEmpty()
                            if (cid.isNotEmpty()) onSelectConversation(cid)
                        }
                        .padding(10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    SwarmPixelBee(name = "coordinator", pose = coordPose, size = 54.dp)
                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text("Coordinator", color = SwarmColors.ink(isDark), fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                        if (!coord?.now.isNullOrBlank()) {
                            Text(coord!!.now, color = SwarmColors.muted(isDark), fontSize = 12.5.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    }
                }

                // Crew Pixel-Bee Filter Tiles
                if (crew.isNotEmpty()) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        val allSelected = filterMember == null
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                            modifier = Modifier.width(52.dp).clickable { filterMember = null }
                        ) {
                            SwarmMiniTrioTile(
                                names = crew.take(3).map { it.name },
                                size = 46.dp,
                                isActive = !synth.finished,
                                modifier = Modifier.border(
                                    width = if (allSelected) 1.8.dp else 0.8.dp,
                                    color = if (allSelected) SwarmColors.ink(isDark) else SwarmColors.border(isDark),
                                    shape = RoundedCornerShape(10.dp)
                                )
                            )
                            Text("All", color = if (allSelected) SwarmColors.ink(isDark) else SwarmColors.muted(isDark), fontSize = 11.sp, fontWeight = if (allSelected) FontWeight.SemiBold else FontWeight.Normal)
                        }

                        shownCrew.forEach { member ->
                            val isSelected = filterMember == member.name
                            val isWorking = !synth.finished && member.status == "working"
                            val dotCol: Color? = when {
                                synth.finished -> null
                                member.hasIssue || member.status == "needs_you" -> SwarmColors.errorRed(isDark)
                                isWorking -> SwarmColors.liveGreen(isDark)
                                member.status == "waiting_on" -> SwarmColors.warnAmber(isDark)
                                else -> null
                            }
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(4.dp),
                                modifier = Modifier.width(54.dp).clickable {
                                    filterMember = if (filterMember == member.name) null else member.name
                                }
                            ) {
                                Box(contentAlignment = Alignment.TopEnd) {
                                    SwarmPixelBee(
                                        name = member.name,
                                        pose = if (isWorking) SwarmBeePose.WORKING else SwarmBeePose.LANDED,
                                        size = 46.dp,
                                        modifier = if (isSelected) Modifier.border(1.8.dp, SwarmColors.ink(isDark), RoundedCornerShape(11.dp)) else Modifier
                                    )
                                    if (dotCol != null) {
                                        Box(
                                            modifier = Modifier
                                                .size(9.5.dp)
                                                .offset(x = 2.dp, y = (-2).dp)
                                                .clip(CircleShape)
                                                .background(dotCol)
                                                .border(1.6.dp, SwarmColors.bg(isDark), CircleShape)
                                        )
                                    }
                                }
                                Text("@${member.name}", color = if (isSelected) SwarmColors.ink(isDark) else SwarmColors.muted(isDark), fontSize = 11.sp, fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }

                        if (extraCrew > 0) {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(4.dp),
                                modifier = Modifier.width(50.dp).clickable { onSwitchToTab(2) }
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(46.dp)
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(SwarmColors.fieldBg(isDark))
                                        .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(10.dp)),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Text("+$extraCrew", color = SwarmColors.muted(isDark), fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
                                }
                                Text(" ", fontSize = 11.sp)
                            }
                        }
                    }
                }
            }
        }

        // 7. Mobile Feedback Composer
        item(key = "feedback") {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(SwarmColors.surfaceBg(isDark))
                    .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
                    .padding(13.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        imageVector = Icons.Default.Campaign,
                        contentDescription = null,
                        tint = SwarmColors.accent(isDark),
                        modifier = Modifier.size(16.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "协同指挥 / 用户反馈".loc,
                        color = SwarmColors.ink(isDark),
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedTextField(
                        value = feedbackText,
                        onValueChange = onFeedbackChange,
                        placeholder = { Text("输入对蜂群的反馈或补充指示...".loc, fontSize = 13.sp) },
                        modifier = Modifier.weight(1f),
                        singleLine = true
                    )

                    Button(
                        onClick = onSendFeedback,
                        enabled = feedbackText.isNotBlank() && !isSendingFeedback,
                        colors = ButtonDefaults.buttonColors(containerColor = SwarmColors.accent(isDark))
                    ) {
                        if (isSendingFeedback) {
                            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = Color.White)
                        } else {
                            Text("发板".loc, color = Color.White)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ProgressStatusDotLabel(text: String, color: Color) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.5.dp)) {
        Box(modifier = Modifier.size(5.5.dp).clip(CircleShape).background(color))
        Text(text, color = color, fontSize = 12.sp, fontWeight = FontWeight.Medium, maxLines = 1)
    }
}

@Composable
private fun WorkerBeeChipsAndroid(
    owners: List<String>,
    synth: SwarmSynthesizedModel,
    snap: SwarmSnapshot,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val shown = owners.take(2)
    val extra = (owners.size - shown.size).coerceAtLeast(0)
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
        shown.forEach { owner ->
            val m = synth.members[owner]
            val working = !synth.finished && m?.killed != true && m?.status == "working"
            Row(
                modifier = Modifier
                    .height(22.dp)
                    .clip(CircleShape)
                    .background(SwarmColors.fieldBg(isDark))
                    .clickable {
                        val cid = snap.members.firstOrNull { it.name == owner }?.conversationId.orEmpty()
                        if (cid.isNotEmpty()) onSelectConversation(cid)
                    }
                    .padding(start = 4.dp, end = 7.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(3.5.dp)
            ) {
                SwarmPixelBeeSprite(
                    name = owner,
                    pose = if (working) SwarmBeePose.WORKING else SwarmBeePose.LANDED,
                    width = 19.dp,
                    height = 15.5.dp
                )
                Text(owner, color = SwarmColors.ink(isDark).copy(alpha = 0.85f), fontSize = 11.sp, fontWeight = FontWeight.Medium, maxLines = 1)
            }
        }
        if (extra > 0) {
            Box(
                modifier = Modifier.height(22.dp).clip(CircleShape).background(SwarmColors.fieldBg(isDark)).padding(horizontal = 6.dp),
                contentAlignment = Alignment.Center
            ) {
                Text("+$extra", color = SwarmColors.muted(isDark), fontSize = 11.sp, fontWeight = FontWeight.Medium)
            }
        }
    }
}

@Composable
private fun OverviewTaskIndicatorAndroid(state: SwarmTaskBucketState, modifier: Modifier = Modifier) {
    val isDark = AntigravityTheme.colors.isDark
    when (state) {
        SwarmTaskBucketState.DONE -> {
            Box(
                modifier = modifier.size(15.dp).clip(CircleShape).background(SwarmColors.liveGreen(isDark).copy(alpha = 0.22f)),
                contentAlignment = Alignment.Center
            ) {
                Icon(Icons.Default.Check, contentDescription = null, tint = SwarmColors.liveGreen(isDark), modifier = Modifier.size(10.dp))
            }
        }
        SwarmTaskBucketState.RUNNING -> {
            val infinite = rememberInfiniteTransition(label = "spinTask")
            val angle by infinite.animateFloat(
                initialValue = 0f,
                targetValue = 360f,
                animationSpec = infiniteRepeatable(tween(1150, easing = LinearEasing)),
                label = "spinDeg"
            )
            val col = SwarmColors.accent(isDark)
            Canvas(modifier = modifier.size(14.dp).rotate(angle)) {
                drawArc(color = col, startAngle = 0f, sweepAngle = 260f, useCenter = false, style = Stroke(width = 2.dp.toPx(), cap = StrokeCap.Round))
            }
        }
        SwarmTaskBucketState.BLOCKED -> {
            Box(modifier = modifier.size(14.dp).border(2.dp, SwarmColors.warnAmber(isDark), CircleShape))
        }
        SwarmTaskBucketState.QUEUED -> {
            Box(modifier = modifier.size(14.dp).border(1.4.dp, SwarmColors.faint(isDark), CircleShape))
        }
    }
}

@Composable
private fun OverviewProgressRingAndroid(done: Int, running: Int, blocked: Int, total: Int) {
    val isDark = AntigravityTheme.colors.isDark
    if (total > 0 && done == total) {
        Box(
            modifier = Modifier.size(18.dp).clip(CircleShape).background(SwarmColors.liveGreen(isDark).copy(alpha = 0.22f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(Icons.Default.Check, contentDescription = null, tint = SwarmColors.liveGreen(isDark), modifier = Modifier.size(11.dp))
        }
    } else {
        val safeTotal = maxOf(1, total).toFloat()
        val sDone = 360f * (done / safeTotal)
        val sRun = 360f * (running / safeTotal)
        val sBlk = 360f * (blocked / safeTotal)
        val trackCol = SwarmColors.fieldBg(isDark)
        val doneCol = SwarmColors.liveGreen(isDark)
        val runCol = SwarmColors.accent(isDark)
        val blkCol = SwarmColors.warnAmber(isDark)
        Canvas(modifier = Modifier.size(18.dp)) {
            val sw = 2.3.dp.toPx()
            val inset = sw / 2f
            val arcSize = Size(size.width - sw, size.height - sw)
            val tl = androidx.compose.ui.geometry.Offset(inset, inset)
            drawArc(trackCol, 0f, 360f, false, tl, arcSize, style = Stroke(sw))
            if (sDone > 0f) drawArc(doneCol, -90f, sDone, false, tl, arcSize, style = Stroke(sw, cap = StrokeCap.Butt))
            if (sRun > 0f) drawArc(runCol, -90f + sDone, sRun, false, tl, arcSize, style = Stroke(sw, cap = StrokeCap.Butt))
            if (sBlk > 0f) drawArc(blkCol, -90f + sDone + sRun, sBlk, false, tl, arcSize, style = Stroke(sw, cap = StrokeCap.Butt))
        }
    }
}

@Composable
private fun OverviewMiniSegmentedBarAndroid(done: Int, running: Int, blocked: Int, queued: Int) {
    val isDark = AntigravityTheme.colors.isDark
    val total = maxOf(1, done + running + blocked + queued).toFloat()
    Row(
        modifier = Modifier.width(28.dp).height(4.5.dp).clip(CircleShape).background(SwarmColors.fieldBg(isDark)),
        horizontalArrangement = Arrangement.spacedBy(1.dp)
    ) {
        if (done > 0) Box(modifier = Modifier.weight(done / total).fillMaxHeight().background(SwarmColors.liveGreen(isDark)))
        if (running > 0) Box(modifier = Modifier.weight(running / total).fillMaxHeight().background(SwarmColors.accent(isDark)))
        if (blocked > 0) Box(modifier = Modifier.weight(blocked / total).fillMaxHeight().background(SwarmColors.warnAmber(isDark)))
        if (queued > 0) Box(modifier = Modifier.weight(queued / total).fillMaxHeight().background(SwarmColors.faint(isDark).copy(alpha = 0.45f)))
    }
}

// MARK: - Board Tab Native

@Composable
private fun SwarmBoardTabNative(
    snap: SwarmSnapshot,
    search: String,
    agentFilter: String?,
    expandedSeqs: List<Int>,
    isLoadingEarlier: Boolean = false,
    onLoadEarlier: () -> Unit = {},
    onSearchChange: (String) -> Unit,
    onFilterChange: (String?) -> Unit,
    onToggleExpand: (Int) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    var showFilterMenu by remember { mutableStateOf(false) }

    val agents = remember(snap.messages) {
        snap.messages.map { it.agent }.distinct().sortedWith { a, b ->
            if (a.equals("coordinator", ignoreCase = true)) -1
            else if (b.equals("coordinator", ignoreCase = true)) 1
            else a.compareTo(b)
        }
    }

    val filteredMessages = remember(snap.messages, search, agentFilter) {
        val q = search.trim().lowercase()
        snap.messages.filter { msg ->
            if (agentFilter != null) {
                if (agentFilter == "@all") {
                    if (!msg.mentions.any { it.equals("all", ignoreCase = true) }) return@filter false
                } else if (agentFilter == "@none") {
                    if (msg.mentions.isNotEmpty()) return@filter false
                } else {
                    val matchSender = msg.agent.equals(agentFilter, ignoreCase = true)
                    val matchMention = msg.mentions.any { it.equals(agentFilter, ignoreCase = true) }
                    if (!matchSender && !matchMention) return@filter false
                }
            }
            if (q.isNotEmpty()) {
                val hay = "${msg.agent} ${msg.mentions.joinToString(" ")} ${msg.text}".lowercase()
                if (!hay.contains(q)) return@filter false
            }
            true
        }
    }

    Column(modifier = Modifier.fillMaxSize()) {
        // Search & Filter header
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Row(
                modifier = Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(9.dp))
                    .background(SwarmColors.fieldBg(isDark))
                    .padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Search,
                    contentDescription = null,
                    tint = SwarmColors.faint(isDark),
                    modifier = Modifier.size(15.dp)
                )
                OutlinedTextField(
                    value = search,
                    onValueChange = onSearchChange,
                    placeholder = { Text("Search", fontSize = 13.5.sp, color = SwarmColors.faint(isDark)) },
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = Color.Transparent,
                        unfocusedBorderColor = Color.Transparent
                    ),
                    modifier = Modifier.weight(1f)
                )
            }

            Box {
                IconButton(
                    onClick = { showFilterMenu = true },
                    modifier = Modifier
                        .size(36.dp)
                        .clip(RoundedCornerShape(9.dp))
                        .background(SwarmColors.fieldBg(isDark))
                ) {
                    Icon(
                        imageVector = Icons.Default.FilterList,
                        contentDescription = "Filter",
                        tint = if (agentFilter == null) SwarmColors.muted(isDark) else SwarmColors.accent(isDark),
                        modifier = Modifier.size(17.dp)
                    )
                }

                DropdownMenu(
                    expanded = showFilterMenu,
                    onDismissRequest = { showFilterMenu = false }
                ) {
                    DropdownMenuItem(text = { Text("全部成员".loc) }, onClick = { onFilterChange(null); showFilterMenu = false })
                    DropdownMenuItem(text = { Text("仅看 @all 广播".loc) }, onClick = { onFilterChange("@all"); showFilterMenu = false })
                    DropdownMenuItem(text = { Text("未指定接收者".loc) }, onClick = { onFilterChange("@none"); showFilterMenu = false })
                    if (agents.isNotEmpty()) {
                        HorizontalDivider()
                        agents.forEach { ag ->
                            DropdownMenuItem(text = { Text("@$ag") }, onClick = { onFilterChange(ag); showFilterMenu = false })
                        }
                    }
                }
            }
        }

        if (filteredMessages.isEmpty()) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(if (snap.messages.isEmpty()) "暂无协作留言".loc else "没有匹配筛选条件的消息".loc, color = SwarmColors.muted(isDark), fontSize = 14.sp)
            }
        } else {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(bottom = 16.dp)
            ) {
                if (snap.hasMoreBefore) {
                    item(key = "swarm-load-earlier") {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 10.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            TextButton(
                                onClick = onLoadEarlier,
                                enabled = !isLoadingEarlier
                            ) {
                                if (isLoadingEarlier) {
                                    CircularProgressIndicator(
                                        modifier = Modifier.size(14.dp),
                                        strokeWidth = 2.dp,
                                        color = SwarmColors.accent(isDark)
                                    )
                                    Spacer(modifier = Modifier.width(6.dp))
                                }
                                Text(
                                    text = if (isLoadingEarlier) "正在加载更早消息...".loc else "加载更早消息".loc,
                                    color = SwarmColors.accent(isDark),
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.Medium
                                )
                            }
                        }
                    }
                }
                items(filteredMessages, key = { it.seq }) { msg ->
                    BoardMessageRowNative(
                        msg = msg,
                        isExpanded = expandedSeqs.contains(msg.seq),
                        onToggleExpand = { onToggleExpand(msg.seq) }
                    )
                    HorizontalDivider(
                        modifier = Modifier.padding(start = 56.dp),
                        thickness = 0.6.dp,
                        color = SwarmColors.border(isDark).copy(alpha = 0.6f)
                    )
                }
            }
        }
    }
}

@Composable
private fun BoardMessageRowNative(
    msg: SwarmBoardMessage,
    isExpanded: Boolean,
    onToggleExpand: () -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val targets = remember(msg) {
        msg.mentions.filter { !it.equals(msg.agent, ignoreCase = true) }.distinct()
    }
    val cleanBody = remember(msg, targets) {
        if (targets.isEmpty()) msg.text else stripLeadingMentions(msg.text)
    }

    val needsClamp = cleanBody.length > 160 || cleanBody.count { it == '\n' } >= 4

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(11.dp),
        verticalAlignment = Alignment.Top
    ) {
        SwarmPixelBee(
            name = msg.agent,
            pose = SwarmBeePose.LANDED,
            size = 30.dp,
            modifier = Modifier.padding(top = 2.dp)
        )

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(5.dp)
            ) {
                Text(
                    text = "@${msg.agent}",
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = SwarmColors.ink(isDark)
                )

                if (targets.isNotEmpty()) {
                    Text(text = "→", fontSize = 12.sp, color = SwarmColors.faint(isDark))
                    Text(
                        text = targets.joinToString(" ") { "@$it" },
                        fontSize = 13.sp,
                        color = SwarmColors.muted(isDark),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false)
                    )
                }

                Spacer(modifier = Modifier.weight(1f))

                Text(
                    text = "#${msg.seq} · ${formatClock(msg.time)}",
                    fontSize = 11.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = SwarmColors.faint(isDark)
                )
            }

            Text(
                text = cleanBody,
                fontSize = 13.sp,
                lineHeight = 18.sp,
                color = SwarmColors.ink(isDark),
                maxLines = if (needsClamp && !isExpanded) 4 else Int.MAX_VALUE,
                overflow = TextOverflow.Ellipsis
            )

            if (needsClamp) {
                Text(
                    text = if (isExpanded) "收起".loc else "展开更多".loc,
                    fontSize = 11.5.sp,
                    fontWeight = FontWeight.Medium,
                    color = SwarmColors.accent(isDark),
                    modifier = Modifier
                        .clickable { onToggleExpand() }
                        .padding(top = 2.dp)
                )
            }
        }
    }
}

// MARK: - Members Tab Native

@Composable
private fun SwarmMembersTabNative(
    snap: SwarmSnapshot,
    search: String,
    showRetired: Boolean,
    selectedMember: String?,
    nudgingMembers: List<String>,
    onSearchChange: (String) -> Unit,
    onToggleRetired: () -> Unit,
    onSelectMemberRow: (String) -> Unit,
    onNudge: (String) -> Unit,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark

    val matching = remember(snap.members, search, snap.plan) {
        val q = search.trim().lowercase()
        snap.members.filter { m ->
            if (m.roleWorker || isRoleWorker(m.name, m.role)) return@filter false
            if (q.isEmpty()) true
            else "${m.name} ${m.role} ${snap.plan?.tasks?.get(m.name).orEmpty()} ${m.model} ${m.lastActivity ?: ""}".lowercase().contains(q)
        }
    }

    val coordinator = matching.firstOrNull { it.name.equals("coordinator", ignoreCase = true) || it.role.equals("coordinator", ignoreCase = true) }
    val workers = matching.filter { !it.name.equals("coordinator", ignoreCase = true) && !it.role.equals("coordinator", ignoreCase = true) }
    val liveWorkers = workers.filter { !it.killed }
    val activeWorkers = liveWorkers.filter {
        val st = it.status.lowercase()
        st == "working" || st == "error" || st == "stuck"
    }
    val standbyWorkers = liveWorkers.filter {
        val st = it.status.lowercase()
        st != "working" && st != "error" && st != "stuck"
    }
    val retiredWorkers = workers.filter { it.killed }

    Column(modifier = Modifier.fillMaxSize()) {
        // Search bar
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 10.dp)
                .clip(RoundedCornerShape(9.dp))
                .background(SwarmColors.fieldBg(isDark))
                .padding(horizontal = 10.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Search,
                contentDescription = null,
                tint = SwarmColors.faint(isDark),
                modifier = Modifier.size(15.dp)
            )
            OutlinedTextField(
                value = search,
                onValueChange = onSearchChange,
                placeholder = { Text("Search members", fontSize = 13.5.sp, color = SwarmColors.faint(isDark)) },
                colors = OutlinedTextFieldDefaults.colors(
                    focusedBorderColor = Color.Transparent,
                    unfocusedBorderColor = Color.Transparent
                ),
                modifier = Modifier.weight(1f)
            )
        }

        if (matching.isEmpty()) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text("未找到匹配的成员".loc, color = SwarmColors.muted(isDark), fontSize = 14.sp)
            }
        } else {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                coordinator?.let { coord ->
                    item(key = "coord") {
                        MemberCardRow(
                            member = coord,
                            snap = snap,
                            highlighted = false,
                            isSelected = selectedMember == coord.name,
                            isNudging = nudgingMembers.contains(coord.name),
                            onRowClick = { onSelectMemberRow(coord.name) },
                            onNudge = { onNudge(coord.name) },
                            onSelectConversation = onSelectConversation
                        )
                    }
                }

                if (activeWorkers.isNotEmpty()) {
                    item(key = "hdr_active") {
                        MemberGroupHeader(title = "ACTIVE", count = activeWorkers.size)
                    }
                    items(activeWorkers, key = { it.name }) { m ->
                        MemberCardRow(
                            member = m,
                            snap = snap,
                            highlighted = true,
                            isSelected = selectedMember == m.name,
                            isNudging = nudgingMembers.contains(m.name),
                            onRowClick = { onSelectMemberRow(m.name) },
                            onNudge = { onNudge(m.name) },
                            onSelectConversation = onSelectConversation
                        )
                    }
                }

                if (standbyWorkers.isNotEmpty()) {
                    item(key = "hdr_standby") {
                        MemberGroupHeader(title = "STANDING BY", count = standbyWorkers.size)
                    }
                    items(standbyWorkers, key = { it.name }) { m ->
                        MemberCardRow(
                            member = m,
                            snap = snap,
                            highlighted = false,
                            isSelected = selectedMember == m.name,
                            isNudging = nudgingMembers.contains(m.name),
                            onRowClick = { onSelectMemberRow(m.name) },
                            onNudge = { onNudge(m.name) },
                            onSelectConversation = onSelectConversation
                        )
                    }
                }

                if (retiredWorkers.isNotEmpty()) {
                    item(key = "hdr_retired") {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { onToggleRetired() }
                                .padding(vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Text(
                                text = "RETIRED",
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Bold,
                                color = SwarmColors.faint(isDark)
                            )
                            Text(
                                text = "${retiredWorkers.size}",
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = SwarmColors.faint(isDark)
                            )
                            Icon(
                                imageVector = if (showRetired) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                                contentDescription = null,
                                tint = SwarmColors.faint(isDark),
                                modifier = Modifier.size(14.dp)
                            )
                        }
                    }
                    if (showRetired) {
                        items(retiredWorkers, key = { it.name }) { m ->
                            MemberCardRow(
                                member = m,
                                snap = snap,
                                highlighted = false,
                                isSelected = selectedMember == m.name,
                                isNudging = false,
                                onRowClick = { onSelectMemberRow(m.name) },
                                onNudge = {},
                                onSelectConversation = onSelectConversation
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun MemberGroupHeader(title: String, count: Int) {
    val isDark = AntigravityTheme.colors.isDark
    Row(
        modifier = Modifier.padding(top = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = title,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold,
            color = SwarmColors.faint(isDark)
        )
        Text(
            text = "$count",
            fontSize = 11.sp,
            fontFamily = FontFamily.Monospace,
            color = SwarmColors.faint(isDark)
        )
    }
}

@Composable
private fun MemberCardRow(
    member: SwarmMemberInfo,
    snap: SwarmSnapshot,
    highlighted: Boolean,
    isSelected: Boolean,
    isNudging: Boolean,
    onRowClick: () -> Unit,
    onNudge: () -> Unit,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val isCoord = member.name.equals("coordinator", ignoreCase = true) || member.role.equals("coordinator", ignoreCase = true)
    val st = member.status.lowercase()
    val isWorking = !member.killed && st == "working"
    val isError = !member.killed && (st == "error" || st == "stuck")

    val pose = when {
        member.killed -> SwarmBeePose.LANDED
        isWorking -> SwarmBeePose.WORKING
        else -> SwarmBeePose.ASLEEP
    }
    val dotColor = when {
        isWorking -> SwarmColors.liveGreen(isDark)
        isError -> SwarmColors.errorRed(isDark)
        else -> null
    }

    val lastPost = remember(snap.messages, member.name) {
        snap.messages.lastOrNull { it.agent.equals(member.name, ignoreCase = true) }
    }

    val bodyPreview = when {
        isWorking && !member.lastActivity.isNullOrBlank() -> member.lastActivity
        isError && !member.lastActivity.isNullOrBlank() -> member.lastActivity
        lastPost != null -> stripLeadingMentions(lastPost.text).replace('\n', ' ')
        else -> member.lastActivity ?: ""
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(if (highlighted) SwarmColors.cardBg(isDark) else Color.Transparent)
            .border(
                width = if (highlighted) 0.7.dp else 0.dp,
                color = if (highlighted) SwarmColors.border(isDark) else Color.Transparent,
                shape = RoundedCornerShape(10.dp)
            )
            .clickable { onRowClick() }
            .padding(horizontal = if (highlighted) 12.dp else 4.dp, vertical = if (highlighted) 10.dp else 6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(11.dp),
            verticalAlignment = Alignment.Top
        ) {
            SwarmPixelBee(
                name = member.name,
                pose = pose,
                size = 36.dp,
                statusDotColor = dotColor
            )

            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(3.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = "@${member.name}",
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = if (member.killed) SwarmColors.faint(isDark) else SwarmColors.ink(isDark)
                    )

                    val statusLabel = when {
                        isCoord -> if (isWorking) "Coordinator · Working" else "Coordinator"
                        member.killed -> "Retired"
                        isWorking -> "Working"
                        isError -> "Error"
                        else -> member.role.ifBlank { "worker" }
                    }

                    Text(
                        text = statusLabel,
                        fontSize = 12.sp,
                        fontWeight = if (isWorking) FontWeight.SemiBold else FontWeight.Medium,
                        color = when {
                            isWorking -> SwarmColors.accent(isDark)
                            isError -> SwarmColors.errorRed(isDark)
                            isCoord -> SwarmColors.muted(isDark)
                            else -> SwarmColors.faint(isDark)
                        }
                    )

                    if (member.isCurrent) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(SwarmColors.infoBlue(isDark).copy(alpha = 0.12f))
                                .padding(horizontal = 5.dp, vertical = 1.dp)
                        ) {
                            Text(
                                text = "当前".loc,
                                fontSize = 10.sp,
                                fontWeight = FontWeight.Bold,
                                color = SwarmColors.infoBlue(isDark)
                            )
                        }
                    }

                    Spacer(modifier = Modifier.weight(1f))

                    val modelLabel = member.model.replace(Regex("-(?:dropdown|high|medium)$", RegexOption.IGNORE_CASE), "")
                    val meta = listOfNotNull(
                        modelLabel.ifBlank { null },
                        lastPost?.seq?.let { "#$it" }
                    ).joinToString(" · ")

                    if (meta.isNotBlank()) {
                        Text(
                            text = meta,
                            fontSize = 11.sp,
                            fontFamily = FontFamily.Monospace,
                            color = SwarmColors.faint(isDark)
                        )
                    }
                }

                if (!bodyPreview.isNullOrBlank()) {
                    Text(
                        text = bodyPreview,
                        fontSize = 12.5.sp,
                        color = if (isWorking || isCoord) SwarmColors.ink(isDark).copy(alpha = 0.88f) else SwarmColors.muted(isDark),
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }

        AnimatedVisibility(visible = isSelected) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 47.dp, top = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (!member.killed) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(SwarmColors.accent(isDark).copy(alpha = 0.12f))
                            .clickable(enabled = !isNudging) { onNudge() }
                            .padding(horizontal = 10.dp, vertical = 5.dp)
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            if (isNudging) {
                                CircularProgressIndicator(modifier = Modifier.size(10.dp), strokeWidth = 1.5.dp, color = SwarmColors.accent(isDark))
                            } else {
                                Icon(imageVector = Icons.Default.Bolt, contentDescription = null, tint = SwarmColors.accent(isDark), modifier = Modifier.size(11.dp))
                            }
                            Text("催促 / 唤醒".loc, fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold, color = SwarmColors.accent(isDark))
                        }
                    }
                }

                if (member.conversationId.isNotBlank() && !member.isCurrent) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(SwarmColors.infoBlue(isDark).copy(alpha = 0.12f))
                            .clickable { onSelectConversation(member.conversationId) }
                            .padding(horizontal = 10.dp, vertical = 5.dp)
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            Icon(imageVector = Icons.Default.OpenInNew, contentDescription = null, tint = SwarmColors.infoBlue(isDark), modifier = Modifier.size(11.dp))
                            Text("进入子会话".loc, fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold, color = SwarmColors.infoBlue(isDark))
                        }
                    }
                }

                Spacer(modifier = Modifier.weight(1f))

                if (member.unreadCount > 0) {
                    Text("未读 ${member.unreadCount}".loc, fontSize = 11.sp, color = SwarmColors.faint(isDark))
                }
            }
        }
    }
}
