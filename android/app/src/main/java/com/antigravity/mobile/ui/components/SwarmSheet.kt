package com.antigravity.mobile.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
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
import androidx.compose.ui.graphics.Color
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
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    var selectedTab by remember { mutableIntStateOf(1) } // 0: 概览, 1: 协作板, 2: 成员, 3: 拓扑
    val tabs = listOf("概览", "协作板", "成员", "拓扑")

    var snapshot by remember { mutableStateOf<SwarmSnapshot?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

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
            val res = apiClient.fetchSwarmSnapshot(cascadeId)
            res.onSuccess {
                snapshot = it
                if (!silent) errorMessage = null
            }.onFailure {
                if (!silent || snapshot == null) {
                    errorMessage = it.message ?: "加载快照失败"
                }
            }
            if (!silent) isLoading = false
        }
    }

    LaunchedEffect(cascadeId) {
        loadData(silent = false)
        while (true) {
            delay(3000)
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
                // Top Header Bar (No "完成" Button)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = snapshot?.title?.ifBlank { "蜂群协同 (Swarm)" } ?: "蜂群协同",
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
                val boardCount = snapshot?.messages?.size ?: 0
                val memberCount = snapshot?.members?.size ?: 0

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
                                    Text("加载蜂群快照失败", color = SwarmColors.ink(isDark), fontSize = 15.sp, fontWeight = FontWeight.Bold)
                                    Text(errorMessage ?: "", color = SwarmColors.muted(isDark), fontSize = 12.5.sp)
                                    Button(
                                        onClick = { loadData(silent = false) },
                                        colors = ButtonDefaults.buttonColors(containerColor = SwarmColors.accent(isDark))
                                    ) {
                                        Text("重试", color = Color.White)
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
                                    }
                                )
                                1 -> SwarmBoardTabNative(
                                    snap = snap,
                                    search = boardSearch,
                                    agentFilter = boardAgentFilter,
                                    expandedSeqs = expandedSeqs,
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

// MARK: - Overview Tab (Warm Colors)

@Composable
private fun SwarmOverviewTab(
    snap: SwarmSnapshot,
    feedbackText: String,
    isSendingFeedback: Boolean,
    onFeedbackChange: (String) -> Unit,
    onSendFeedback: () -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val phaseLabel = snap.view?.phase?.label ?: snap.brief.phase

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(SwarmColors.cardBg(isDark))
                    .border(0.7.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = snap.title.ifBlank { "蜂群协同任务" },
                        color = SwarmColors.ink(isDark),
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.weight(1f)
                    )

                    if (phaseLabel.isNotBlank()) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(SwarmColors.accent(isDark).copy(alpha = 0.12f))
                                .padding(horizontal = 8.dp, vertical = 3.dp)
                        ) {
                            Text(
                                text = phaseLabel,
                                color = SwarmColors.accent(isDark),
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }
                    }
                }

                val headline = snap.view?.headline ?: snap.brief.headline
                if (headline.isNotBlank()) {
                    Text(
                        text = headline,
                        color = SwarmColors.muted(isDark),
                        fontSize = 13.5.sp,
                        lineHeight = 18.sp
                    )
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    StatBox(
                        title = "活跃成员",
                        value = "${snap.brief.workingCount} / ${snap.brief.totalMembers}",
                        color = SwarmColors.liveGreen(isDark),
                        modifier = Modifier.weight(1f)
                    )
                    StatBox(
                        title = "协作消息",
                        value = "${maxOf(snap.messages.size, snap.brief.latestSeq)}",
                        color = SwarmColors.infoBlue(isDark),
                        modifier = Modifier.weight(1f)
                    )
                    StatBox(
                        title = "待关注",
                        value = "${snap.brief.attentionCount}",
                        color = if (snap.brief.attentionCount > 0) SwarmColors.warnAmber(isDark) else SwarmColors.muted(isDark),
                        modifier = Modifier.weight(1f)
                    )
                }
            }
        }

        item {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(SwarmColors.cardBg(isDark))
                    .border(0.7.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        imageVector = Icons.Default.Campaign,
                        contentDescription = null,
                        tint = SwarmColors.accent(isDark),
                        modifier = Modifier.size(17.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "协同指挥 / 用户反馈",
                        color = SwarmColors.ink(isDark),
                        fontSize = 14.5.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }

                Text(
                    text = "直接向蜂群公共协作板留言（发送至协调者）。",
                    color = SwarmColors.muted(isDark),
                    fontSize = 12.sp
                )

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedTextField(
                        value = feedbackText,
                        onValueChange = onFeedbackChange,
                        placeholder = { Text("输入对蜂群的反馈或补充指示...", fontSize = 13.sp) },
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
                            Text("发板", color = Color.White)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun StatBox(title: String, value: String, color: Color, modifier: Modifier = Modifier) {
    val isDark = AntigravityTheme.colors.isDark
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .background(color.copy(alpha = 0.09f))
            .padding(vertical = 10.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Text(text = value, color = SwarmColors.ink(isDark), fontSize = 14.5.sp, fontWeight = FontWeight.Bold)
        Text(text = title, color = SwarmColors.muted(isDark), fontSize = 11.sp)
    }
}

// MARK: - Board Tab Native

@Composable
private fun SwarmBoardTabNative(
    snap: SwarmSnapshot,
    search: String,
    agentFilter: String?,
    expandedSeqs: List<Int>,
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
                    DropdownMenuItem(text = { Text("全部成员") }, onClick = { onFilterChange(null); showFilterMenu = false })
                    DropdownMenuItem(text = { Text("仅看 @all 广播") }, onClick = { onFilterChange("@all"); showFilterMenu = false })
                    DropdownMenuItem(text = { Text("未指定接收者") }, onClick = { onFilterChange("@none"); showFilterMenu = false })
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
                Text(if (snap.messages.isEmpty()) "暂无协作留言" else "没有匹配筛选条件的消息", color = SwarmColors.muted(isDark), fontSize = 14.sp)
            }
        } else {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(bottom = 16.dp)
            ) {
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
                    text = if (isExpanded) "收起" else "展开更多",
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

    val matching = remember(snap.members, search) {
        val q = search.trim().lowercase()
        snap.members.filter { m ->
            if (q.isEmpty()) true
            else "${m.name} ${m.role} ${m.model} ${m.lastActivity ?: ""}".lowercase().contains(q)
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
                Text("未找到匹配的成员", color = SwarmColors.muted(isDark), fontSize = 14.sp)
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
                                text = "当前",
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
                            Text("催促 / 唤醒", fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold, color = SwarmColors.accent(isDark))
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
                            Text("进入子会话", fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold, color = SwarmColors.infoBlue(isDark))
                        }
                    }
                }

                Spacer(modifier = Modifier.weight(1f))

                if (member.unreadCount > 0) {
                    Text("未读 ${member.unreadCount}", fontSize = 11.sp, color = SwarmColors.faint(isDark))
                }
            }
        }
    }
}
