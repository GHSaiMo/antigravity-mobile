package com.antigravity.mobile.ui.components

import android.widget.Toast
import androidx.activity.compose.BackHandler
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
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.ui.theme.AntigravityTheme
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
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    var selectedTab by remember { mutableStateOf(0) } // 0: 概览, 1: 协作板, 2: 成员, 3: 拓扑
    val tabs = listOf("概览", "协作板", "成员", "拓扑")

    var snapshot by remember { mutableStateOf<SwarmSnapshot?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var feedbackText by remember { mutableStateOf("") }
    var isSendingFeedback by remember { mutableStateOf(false) }
    val nudgingMembers = remember { mutableStateListOf<String>() }

    fun loadData() {
        scope.launch {
            isLoading = true
            errorMessage = null
            val res = apiClient.fetchSwarmSnapshot(cascadeId)
            res.onSuccess {
                snapshot = it
            }.onFailure {
                errorMessage = it.message ?: "加载快照失败"
            }
            isLoading = false
        }
    }

    LaunchedEffect(cascadeId) {
        loadData()
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
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
            BackHandler { onDismiss() }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .fillMaxHeight()
            ) {
                // Top Header Bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = snapshot?.title?.ifBlank { "蜂群协同 (Swarm)" } ?: "蜂群协同",
                        color = colors.textPrimary,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Bold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )

                    IconButton(onClick = onDismiss) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = "关闭",
                            tint = colors.textSecondary
                        )
                    }
                }

                // Tab Row
                TabRow(
                    selectedTabIndex = selectedTab,
                    containerColor = colors.surface,
                    contentColor = colors.accentBlue,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    tabs.forEachIndexed { index, title ->
                        Tab(
                            selected = selectedTab == index,
                            onClick = { selectedTab = index },
                            text = {
                                Text(
                                    text = title,
                                    fontSize = 13.5.sp,
                                    fontWeight = if (selectedTab == index) FontWeight.SemiBold else FontWeight.Normal
                                )
                            }
                        )
                    }
                }

                // Content
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                ) {
                    if (isLoading && snapshot == null) {
                        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                            CircularProgressView("正在同步蜂群看板与成员状态...")
                        }
                    } else if (errorMessage != null && snapshot == null) {
                        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Text(text = errorMessage ?: "", color = colors.accentOrange, fontSize = 14.sp)
                                Button(onClick = { loadData() }) {
                                    Text("重试")
                                }
                            }
                        }
                    } else if (snapshot != null) {
                        val snap = snapshot!!
                        when (selectedTab) {
                            0 -> SwarmOverviewTab(
                                snap = snap,
                                feedbackText = feedbackText,
                                isSendingFeedback = isSendingFeedback,
                                onFeedbackChange = { feedbackText = it },
                                onSendFeedback = {
                                    val text = feedbackText.trim()
                                    if (text.isNotEmpty()) {
                                        scope.launch {
                                            isSendingFeedback = true
                                            val coordId = snap.coordinatorId.ifBlank { snap.brief.coordinatorId }
                                            val res = apiClient.sendSwarmFeedback(cascadeId, text, coordId)
                                            res.onSuccess {
                                                feedbackText = ""
                                                Toast.makeText(context, "反馈已发送至 @narrator", Toast.LENGTH_SHORT).show()
                                                loadData()
                                            }.onFailure {
                                                Toast.makeText(context, "发送失败: ${it.message}", Toast.LENGTH_SHORT).show()
                                            }
                                            isSendingFeedback = false
                                        }
                                    }
                                }
                            )
                            1 -> SwarmBoardTab(snap = snap)
                            2 -> SwarmMembersTab(
                                snap = snap,
                                nudgingMembers = nudgingMembers,
                                onNudge = { memberName ->
                                    scope.launch {
                                        nudgingMembers.add(memberName)
                                        val coordId = snap.coordinatorId.ifBlank { snap.brief.coordinatorId }
                                        val res = apiClient.nudgeSwarmMember(cascadeId, memberName, coordId)
                                        res.onSuccess {
                                            Toast.makeText(context, "已催促唤醒 @$memberName", Toast.LENGTH_SHORT).show()
                                            loadData()
                                        }.onFailure {
                                            Toast.makeText(context, "催促失败: ${it.message}", Toast.LENGTH_SHORT).show()
                                        }
                                        nudgingMembers.remove(memberName)
                                    }
                                },
                                onSelectConversation = { convId ->
                                    onDismiss()
                                    onSelectConversation(convId)
                                }
                            )
                            3 -> SwarmGraphTab(snap = snap)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SwarmOverviewTab(
    snap: SwarmSnapshot,
    feedbackText: String,
    isSendingFeedback: Boolean,
    onFeedbackChange: (String) -> Unit,
    onSendFeedback: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val phaseText = snap.view?.phase?.label?.ifBlank { snap.brief.phase } ?: snap.brief.phase
    val headlineText = snap.view?.headline?.ifBlank { snap.brief.headline } ?: snap.brief.headline

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item {
            // Task Header Card
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text(
                        text = snap.title.ifBlank { snap.brief.title.ifBlank { "无标题蜂群任务" } },
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.weight(1f)
                    )
                    if (phaseText.isNotBlank()) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(colors.accentBlue.copy(alpha = 0.12f))
                                .padding(horizontal = 8.dp, vertical = 3.dp)
                        ) {
                            Text(
                                text = phaseText,
                                color = colors.accentBlue,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Bold
                            )
                        }
                    }
                }

                if (headlineText.isNotBlank()) {
                    Text(
                        text = headlineText,
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        lineHeight = 18.sp
                    )
                }

                // Stats Row
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    StatPill(
                        title = "工作中成员",
                        value = "${snap.brief.workingCount} / ${snap.brief.totalMembers}",
                        modifier = Modifier.weight(1f)
                    )
                    StatPill(
                        title = "协作消息",
                        value = "${snap.brief.latestSeq}",
                        modifier = Modifier.weight(1f)
                    )
                    StatPill(
                        title = "待关注",
                        value = "${snap.brief.attentionCount}",
                        isWarning = snap.brief.attentionCount > 0,
                        modifier = Modifier.weight(1f)
                    )
                }
            }
        }

        val attentionItems = snap.view?.attention.orEmpty()
        if (attentionItems.isNotEmpty()) {
            item {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                        .padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = "待关注事项 (${attentionItems.size})",
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                    attentionItems.forEach { att ->
                        val sevColor = when {
                            att.severity.equals("high", ignoreCase = true) -> Color(0xFFEF4444)
                            att.severity.equals("low", ignoreCase = true) -> colors.accentBlue
                            else -> colors.accentOrange
                        }
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(sevColor.copy(alpha = 0.08f))
                                .border(0.5.dp, sevColor.copy(alpha = 0.25f), RoundedCornerShape(8.dp))
                                .padding(10.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(4.dp))
                                        .background(sevColor.copy(alpha = 0.16f))
                                        .padding(horizontal = 5.dp, vertical = 1.dp)
                                ) {
                                    Text(
                                        text = att.kind.uppercase(),
                                        color = sevColor,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.Bold,
                                        fontFamily = FontFamily.Monospace
                                    )
                                }
                                Text(
                                    text = att.title,
                                    color = colors.textPrimary,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                            if (!att.why.isNullOrBlank()) {
                                Text(
                                    text = att.why,
                                    color = colors.textSecondary,
                                    fontSize = 12.sp
                                )
                            }
                        }
                    }
                }
            }
        }

        val workstreams = snap.view?.workstreams.orEmpty()
        if (workstreams.isNotEmpty()) {
            item {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                        .padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = "工作流推进 (${workstreams.size})",
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                    workstreams.forEach { ws ->
                        val wsColor = when (ws.state.lowercase()) {
                            "active", "done" -> colors.accentGreen
                            "blocked", "failed" -> Color(0xFFEF4444)
                            "review" -> colors.accentOrange
                            else -> colors.textSecondary
                        }
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.surfaceVariant.copy(alpha = 0.45f))
                                .padding(10.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween,
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text(
                                    text = ws.title,
                                    color = colors.textPrimary,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.Medium,
                                    modifier = Modifier.weight(1f)
                                )
                                Text(
                                    text = ws.state,
                                    color = wsColor,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                            if (ws.owners.isNotEmpty()) {
                                Text(
                                    text = "负责人: ${ws.owners.joinToString(", ")}",
                                    color = colors.textSecondary,
                                    fontSize = 11.5.sp
                                )
                            }
                            if (!ws.note.isNullOrBlank()) {
                                Text(
                                    text = ws.note,
                                    color = colors.textSecondary,
                                    fontSize = 12.sp
                                )
                            }
                        }
                    }
                }
            }
        }

        item {
            // Feedback Relay
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        imageVector = Icons.Default.Campaign,
                        contentDescription = null,
                        tint = colors.accentBlue,
                        modifier = Modifier.size(18.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "协同指挥 / 用户反馈",
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }

                Text(
                    text = "直接向蜂群公共协作板留言（自动以 @narrator user feedback: 发送至协调员）。",
                    color = colors.textSecondary,
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
                        enabled = feedbackText.isNotBlank() && !isSendingFeedback
                    ) {
                        if (isSendingFeedback) {
                            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                        } else {
                            Text("发板")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun StatPill(
    title: String,
    value: String,
    modifier: Modifier = Modifier,
    isWarning: Boolean = false
) {
    val colors = AntigravityTheme.colors
    val accent = if (isWarning) colors.accentOrange else colors.accentBlue

    Column(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(accent.copy(alpha = 0.08f))
            .padding(vertical = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text(text = value, color = colors.textPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
        Text(text = title, color = colors.textSecondary, fontSize = 10.5.sp)
    }
}

@Composable
private fun SwarmBoardTab(snap: SwarmSnapshot) {
    val colors = AntigravityTheme.colors

    if (snap.messages.isEmpty()) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text("暂无协作留言", color = colors.textSecondary, fontSize = 14.sp)
        }
    } else {
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            items(snap.messages) { msg ->
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        val isCoordinator = msg.agent == "narrator" || msg.agent == "coordinator"
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(if (isCoordinator) colors.accentBlue.copy(alpha = 0.12f) else colors.surfaceVariant)
                                .padding(horizontal = 7.dp, vertical = 2.5.dp)
                        ) {
                            Text(
                                text = msg.agent,
                                color = if (isCoordinator) colors.accentBlue else colors.textPrimary,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }

                        Text(
                            text = "#${msg.seq}",
                            color = colors.textSecondary,
                            fontSize = 10.5.sp,
                            fontFamily = FontFamily.Monospace
                        )

                        msg.mentions.forEach { mention ->
                            Box(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(colors.accentBlue.copy(alpha = 0.08f))
                                    .padding(horizontal = 5.dp, vertical = 2.dp)
                            ) {
                                Text(
                                    text = "@$mention",
                                    color = colors.accentBlue,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.Medium
                                )
                            }
                        }

                        Spacer(modifier = Modifier.weight(1f))

                        if (msg.time.isNotBlank()) {
                            Text(text = msg.time, color = colors.textSecondary, fontSize = 10.sp)
                        }
                    }

                    Text(
                        text = msg.text,
                        color = colors.textPrimary,
                        fontSize = 13.sp,
                        lineHeight = 18.sp
                    )
                }
            }
        }
    }
}

@Composable
private fun SwarmMembersTab(
    snap: SwarmSnapshot,
    nudgingMembers: List<String>,
    onNudge: (String) -> Unit,
    onSelectConversation: (String) -> Unit
) {
    val colors = AntigravityTheme.colors

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        items(snap.members) { member ->
            val lowerStatus = member.status.lowercase()
            val isWorking = lowerStatus == "working"
            val isWaiting = lowerStatus == "waiting"
            val isKilled = member.killed || lowerStatus == "killed"
            val isError = lowerStatus == "error" || lowerStatus == "stuck"

            val stateColor = when {
                isKilled -> Color.Gray
                isError -> Color(0xFFEF4444)
                isWorking -> colors.accentGreen
                isWaiting -> colors.accentOrange
                else -> colors.textSecondary
            }

            val stateLabel = when {
                isKilled -> "已终止"
                lowerStatus == "error" -> "报错"
                lowerStatus == "stuck" -> "卡住"
                isWorking -> "工作中"
                isWaiting -> "等待中"
                else -> member.status
            }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(
                        if (member.isCurrent) 1.dp else 0.5.dp,
                        if (member.isCurrent) colors.accentBlue.copy(alpha = 0.5f) else colors.border,
                        RoundedCornerShape(10.dp)
                    )
                    .padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(7.dp)
                            .clip(CircleShape)
                            .background(stateColor)
                    )

                    Text(
                        text = member.name,
                        color = colors.textPrimary,
                        fontSize = 14.5.sp,
                        fontWeight = FontWeight.Bold
                    )

                    if (member.isCurrent) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(colors.accentBlue.copy(alpha = 0.14f))
                                .padding(horizontal = 6.dp, vertical = 2.dp)
                        ) {
                            Text(text = "当前", color = colors.accentBlue, fontSize = 10.sp, fontWeight = FontWeight.Bold)
                        }
                    }

                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(stateColor.copy(alpha = 0.12f))
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    ) {
                        Text(text = stateLabel, color = stateColor, fontSize = 10.5.sp, fontWeight = FontWeight.Medium)
                    }

                    if (member.unreadCount > 0) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(colors.accentOrange.copy(alpha = 0.14f))
                                .padding(horizontal = 5.dp, vertical = 1.5.dp)
                        ) {
                            Text(text = "${member.unreadCount} 未读", color = colors.accentOrange, fontSize = 10.sp)
                        }
                    }

                    Spacer(modifier = Modifier.weight(1f))

                    if (member.model.isNotBlank()) {
                        Text(
                            text = member.model,
                            color = colors.textSecondary,
                            fontSize = 10.5.sp,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                }

                if (member.role.isNotBlank()) {
                    Text(
                        text = member.role,
                        color = colors.textPrimary,
                        fontSize = 12.5.sp,
                        fontWeight = FontWeight.Medium
                    )
                }

                if (!member.lastActivity.isNullOrBlank()) {
                    Text(
                        text = member.lastActivity,
                        color = colors.textSecondary,
                        fontSize = 12.sp,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                // Action buttons: Nudge & Jump
                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    if (!isKilled) {
                        val isNudging = nudgingMembers.contains(member.name)
                        OutlinedButton(
                            onClick = { onNudge(member.name) },
                            enabled = !isNudging,
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(30.dp)
                        ) {
                            if (isNudging) {
                                CircularProgressIndicator(modifier = Modifier.size(12.dp), strokeWidth = 1.5.dp)
                            } else {
                                Icon(
                                    imageVector = Icons.Default.Bolt,
                                    contentDescription = null,
                                    tint = colors.accentOrange,
                                    modifier = Modifier.size(13.dp)
                                )
                                Spacer(modifier = Modifier.width(4.dp))
                                Text("催促 / 唤醒", fontSize = 11.5.sp, color = colors.accentOrange)
                            }
                        }
                    }

                    if (member.conversationId.isNotBlank() && !member.isCurrent) {
                        OutlinedButton(
                            onClick = { onSelectConversation(member.conversationId) },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(30.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.OpenInNew,
                                contentDescription = null,
                                modifier = Modifier.size(13.dp)
                            )
                            Spacer(modifier = Modifier.width(4.dp))
                            Text("进入子会话", fontSize = 11.5.sp)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SwarmGraphTab(snap: SwarmSnapshot) {
    val colors = AntigravityTheme.colors

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        items(snap.members) { m ->
            val mentionsOut = snap.messages.filter { it.agent == m.name }.flatMap { it.mentions }.distinct()
            val mentionsIn = snap.messages.filter { it.mentions.contains(m.name) }.map { it.agent }.distinct()

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                    .padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(text = m.name, color = colors.textPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                    Spacer(modifier = Modifier.width(8.dp))
                    Text(text = m.role, color = colors.textSecondary, fontSize = 12.sp)
                }

                if (mentionsOut.isNotEmpty()) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(text = "→ 提及:", color = colors.textSecondary, fontSize = 11.sp)
                        mentionsOut.forEach {
                            Text(text = "@$it", color = colors.accentBlue, fontSize = 11.sp, fontWeight = FontWeight.Medium)
                        }
                    }
                }

                if (mentionsIn.isNotEmpty()) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(text = "← 被提及:", color = colors.textSecondary, fontSize = 11.sp)
                        mentionsIn.forEach {
                            Text(text = "@$it", color = colors.accentIndigo, fontSize = 11.sp, fontWeight = FontWeight.Medium)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun CircularProgressView(message: String) {
    val colors = AntigravityTheme.colors
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        CircularProgressIndicator(color = colors.accentBlue, strokeWidth = 2.5.dp)
        Text(text = message, color = colors.textSecondary, fontSize = 13.sp)
    }
}
