package com.antigravity.mobile.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SwarmBoardMessage
import com.antigravity.mobile.data.model.SwarmMemberInfo
import com.antigravity.mobile.data.model.SwarmSnapshot
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay
import kotlin.math.*

@Composable
fun SwarmGraphTab(
    snap: SwarmSnapshot,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val density = LocalDensity.current

    val allMessages = remember(snap.messages) {
        snap.messages.filter { !it.text.startsWith("[channel]") }
    }

    var replayIndex by remember { mutableStateOf<Int?>(null) } // null == Live
    var isPlaying by remember { mutableStateOf(false) }
    var hideInactive by remember { mutableStateOf(false) }
    var expandAll by remember { mutableStateOf(false) }
    var selectedNodeId by remember { mutableStateOf<String?>(null) }

    val visibleMessages = remember(allMessages, replayIndex) {
        val idx = replayIndex ?: allMessages.size
        allMessages.take(idx.coerceIn(0, allMessages.size))
    }

    val currentReplayMessage = remember(allMessages, replayIndex) {
        val idx = replayIndex ?: return@remember null
        if (idx in 1..allMessages.size) allMessages[idx - 1] else null
    }

    val displayDetailMessage = remember(currentReplayMessage, selectedNodeId, allMessages) {
        if (currentReplayMessage != null) return@remember currentReplayMessage
        val sel = selectedNodeId
        if (sel != null) {
            allMessages.lastOrNull { it.agent.equals(sel, ignoreCase = true) }
                ?: allMessages.lastOrNull { m -> m.mentions.any { it.equals(sel, ignoreCase = true) } }
        } else {
            allMessages.lastOrNull()
        }
    }

    val memberByName = remember(snap.members) {
        snap.members.associateBy { it.name }
    }

    val taskMembers = remember(snap.members) {
        snap.members.filter { !isRoleWorker(it.name, it.role) }.sortedWith { a, b ->
            if (a.name.equals("coordinator", ignoreCase = true)) -1
            else if (b.name.equals("coordinator", ignoreCase = true)) 1
            else a.name.compareTo(b.name)
        }
    }

    LaunchedEffect(isPlaying) {
        if (isPlaying) {
            val total = allMessages.size
            if (total == 0) {
                isPlaying = false
                return@LaunchedEffect
            }
            if (replayIndex == null || (replayIndex ?: 0) >= total) {
                replayIndex = 1
            }
            while (isPlaying) {
                delay(950)
                val cur = replayIndex ?: total
                if (cur < total) {
                    replayIndex = cur + 1
                } else {
                    isPlaying = false
                }
            }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Replay Controls
        ReplayControlsBar(
            total = allMessages.size,
            currentPos = replayIndex ?: allMessages.size,
            isLive = replayIndex == null,
            isPlaying = isPlaying,
            currentMsg = currentReplayMessage,
            firstMsg = allMessages.firstOrNull(),
            hideInactive = hideInactive,
            expandAll = expandAll,
            onPlayPause = {
                if (isPlaying) {
                    isPlaying = false
                } else {
                    if (replayIndex == null || (replayIndex ?: 0) >= allMessages.size) {
                        replayIndex = 1
                    }
                    isPlaying = true
                }
            },
            onStepBack = {
                isPlaying = false
                val cur = replayIndex ?: allMessages.size
                replayIndex = max(1, cur - 1)
            },
            onStepForward = {
                isPlaying = false
                val cur = replayIndex ?: allMessages.size
                if (cur + 1 >= allMessages.size) {
                    replayIndex = null
                } else {
                    replayIndex = cur + 1
                }
            },
            onStart = {
                isPlaying = false
                replayIndex = if (allMessages.isNotEmpty()) 1 else 0
            },
            onSeek = { target ->
                isPlaying = false
                if (target >= allMessages.size) {
                    replayIndex = null
                } else {
                    replayIndex = max(1, target)
                }
            },
            onGoLive = {
                isPlaying = false
                replayIndex = null
            },
            onToggleHideInactive = { hideInactive = !hideInactive },
            onToggleExpandAll = { expandAll = !expandAll }
        )

        // Topology Canvas
        GraphCanvasCard(
            messages = visibleMessages,
            members = taskMembers,
            memberByName = memberByName,
            currentReplayMessage = currentReplayMessage,
            selectedNodeId = selectedNodeId,
            hideInactive = hideInactive,
            expandAll = expandAll,
            onSelectNode = {
                selectedNodeId = if (selectedNodeId == it) null else it
            }
        )

        // Legend Row
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "→ @提及 (线宽 = 次数)",
                fontSize = 11.sp,
                color = SwarmColors.muted(isDark)
            )
            Text(
                text = "⇢ 经 @all",
                fontSize = 11.sp,
                color = SwarmColors.muted(isDark)
            )
            Box(
                modifier = Modifier
                    .clip(CircleShape)
                    .background(SwarmColors.codeBg(isDark))
                    .padding(horizontal = 5.dp, vertical = 1.dp)
            ) {
                Text(
                    text = "@all",
                    fontSize = 9.5.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.SemiBold,
                    color = SwarmColors.ink(isDark)
                )
            }
        }

        // Selected / Current Message Detail Card
        displayDetailMessage?.let { detailMsg ->
            DetailMessageCard(
                msg = detailMsg,
                selectedMember = selectedNodeId?.let { memberByName[it] },
                onSelectConversation = onSelectConversation
            )
        }
    }
}

internal fun isRoleWorker(name: String, role: String): Boolean {
    val r = role.trim().lowercase()
    if (r.isNotEmpty() && r != "worker" && r != "coordinator") return true
    val n = name.lowercase()
    return n == "narrator" || n == "monitor" || n.startsWith("narrator-") || n.startsWith("monitor-")
}

internal fun extractTargets(msg: SwarmBoardMessage): List<String> {
    return msg.mentions.filter { !it.equals(msg.agent, ignoreCase = true) }.distinct()
}

internal fun stripLeadingMentions(text: String): String {
    val trimmed = text.trim()
    if (!trimmed.startsWith("@")) return text
    val regex = Regex("""^(?:\s*@[A-Za-z0-9_\-\.]+[,:]?)+\s*""")
    val stripped = regex.replace(trimmed, "")
    return if (stripped.isNotBlank()) stripped else text
}

internal fun formatClock(raw: String): String {
    val parts = raw.split(" ")
    if (parts.size >= 2) {
        val comps = parts[1].split(":")
        if (comps.size >= 2) {
            return "${comps[0].toIntOrNull() ?: comps[0]}:${comps[1]}"
        }
    }
    return raw
}

@Composable
private fun ReplayControlsBar(
    total: Int,
    currentPos: Int,
    isLive: Boolean,
    isPlaying: Boolean,
    currentMsg: SwarmBoardMessage?,
    firstMsg: SwarmBoardMessage?,
    hideInactive: Boolean,
    expandAll: Boolean,
    onPlayPause: () -> Unit,
    onStepBack: () -> Unit,
    onStepForward: () -> Unit,
    onStart: () -> Unit,
    onSeek: (Int) -> Unit,
    onGoLive: () -> Unit,
    onToggleHideInactive: () -> Unit,
    onToggleExpandAll: () -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 14.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            IconButton(
                onClick = onStart,
                enabled = total > 0,
                modifier = Modifier.size(28.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.FirstPage,
                    contentDescription = "起始",
                    tint = if (total > 0) SwarmColors.ink(isDark) else SwarmColors.faint(isDark),
                    modifier = Modifier.size(16.dp)
                )
            }

            IconButton(
                onClick = onStepBack,
                enabled = total > 0 && currentPos > 1,
                modifier = Modifier.size(28.dp)
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "上一条",
                    tint = if (currentPos > 1) SwarmColors.ink(isDark) else SwarmColors.faint(isDark),
                    modifier = Modifier.size(14.dp)
                )
            }

            IconButton(
                onClick = onPlayPause,
                enabled = total > 0,
                modifier = Modifier
                    .size(28.dp)
                    .clip(RoundedCornerShape(6.dp))
                    .background(SwarmColors.fieldBg(isDark))
            ) {
                Icon(
                    imageVector = if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                    contentDescription = if (isPlaying) "暂停" else "播放",
                    tint = SwarmColors.ink(isDark),
                    modifier = Modifier.size(15.dp)
                )
            }

            IconButton(
                onClick = onStepForward,
                enabled = !isLive && total > 0,
                modifier = Modifier.size(28.dp)
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowForward,
                    contentDescription = "下一条",
                    tint = if (!isLive) SwarmColors.ink(isDark) else SwarmColors.faint(isDark),
                    modifier = Modifier.size(14.dp)
                )
            }

            Slider(
                value = currentPos.toFloat(),
                onValueChange = { onSeek(it.roundToInt()) },
                valueRange = 0f..max(1, total).toFloat(),
                steps = max(0, total - 1),
                colors = SliderDefaults.colors(
                    thumbColor = SwarmColors.ink(isDark),
                    activeTrackColor = SwarmColors.ink(isDark),
                    inactiveTrackColor = SwarmColors.border(isDark)
                ),
                modifier = Modifier.weight(1f)
            )

            Row(
                modifier = Modifier
                    .clip(CircleShape)
                    .background(if (isLive) SwarmColors.fieldBg(isDark) else Color.Transparent)
                    .clickable { onGoLive() }
                    .padding(horizontal = 8.dp, vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(6.dp)
                        .clip(CircleShape)
                        .background(if (isLive) SwarmColors.liveGreen(isDark) else SwarmColors.faint(isDark))
                )
                Text(
                    text = "Live",
                    fontSize = 11.5.sp,
                    fontWeight = if (isLive) FontWeight.SemiBold else FontWeight.Normal,
                    color = if (isLive) SwarmColors.ink(isDark) else SwarmColors.muted(isDark)
                )
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            if (currentMsg != null) {
                Text(
                    text = "$currentPos / $total · #${currentMsg.seq} · ${formatClock(currentMsg.time)}",
                    fontSize = 11.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = SwarmColors.muted(isDark)
                )
            } else {
                Text(
                    text = "Live · 共 $total 条消息",
                    fontSize = 11.5.sp,
                    color = SwarmColors.muted(isDark)
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                ToggleChip(title = "隐藏未活跃", isOn = hideInactive, onClick = onToggleHideInactive)
                ToggleChip(title = "展开 @all", isOn = expandAll, onClick = onToggleExpandAll)
            }
        }
    }
}

@Composable
private fun ToggleChip(
    title: String,
    isOn: Boolean,
    onClick: () -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    Row(
        modifier = Modifier
            .clip(CircleShape)
            .background(if (isOn) SwarmColors.cardBg(isDark) else SwarmColors.fieldBg(isDark).copy(alpha = 0.6f))
            .border(
                width = 0.7.dp,
                color = if (isOn) SwarmColors.accent(isDark).copy(alpha = 0.5f) else SwarmColors.border(isDark),
                shape = CircleShape
            )
            .clickable { onClick() }
            .padding(horizontal = 8.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Box(
            modifier = Modifier
                .size(6.dp)
                .clip(CircleShape)
                .background(if (isOn) SwarmColors.accent(isDark) else SwarmColors.faint(isDark).copy(alpha = 0.6f))
        )
        Text(
            text = title,
            fontSize = 11.sp,
            fontWeight = FontWeight.Medium,
            color = if (isOn) SwarmColors.ink(isDark) else SwarmColors.muted(isDark)
        )
    }
}

private data class NodeLayoutPos(
    val id: String,
    val center: Offset,
    val w: Float,
    val h: Float,
    val member: SwarmMemberInfo?
)

@Composable
private fun GraphCanvasCard(
    messages: List<SwarmBoardMessage>,
    members: List<SwarmMemberInfo>,
    memberByName: Map<String, SwarmMemberInfo>,
    currentReplayMessage: SwarmBoardMessage?,
    selectedNodeId: String?,
    hideInactive: Boolean,
    expandAll: Boolean,
    onSelectNode: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val density = LocalDensity.current

    val memberSet = remember(members) { members.map { it.name }.toSet() }
    val allNamesForBroadcast = remember(members) { members.filter { !it.killed }.map { it.name } }

    val activeSet = remember(messages, expandAll, hideInactive) {
        val set = mutableSetOf("coordinator")
        messages.forEach { m ->
            if (!isRoleWorker(m.agent, "")) {
                set.add(m.agent)
                val targets = extractTargets(m)
                targets.forEach { set.add(it) }
            }
        }
        set
    }

    val edges = remember(messages, expandAll) {
        val map = mutableMapOf<Pair<String, String>, Pair<Int, Int>>() // Pair(direct, viaAll)
        messages.forEach { m ->
            if (!isRoleWorker(m.agent, "")) {
                val targets = extractTargets(m)
                val isAll = m.mentions.any { it.equals("all", ignoreCase = true) }
                targets.forEach { t ->
                    val key = m.agent to t
                    val cur = map[key] ?: (0 to 0)
                    map[key] = (cur.first + 1) to cur.second
                }
                if (isAll && expandAll) {
                    allNamesForBroadcast.forEach { t ->
                        if (!t.equals(m.agent, ignoreCase = true) && !targets.contains(t)) {
                            val key = m.agent to t
                            val cur = map[key] ?: (0 to 0)
                            map[key] = cur.first to (cur.second + 1)
                        }
                    }
                }
            }
        }
        map
    }

    val broadcasts = remember(messages) {
        val map = mutableMapOf<String, Int>()
        messages.forEach { m ->
            if (m.mentions.any { it.equals("all", ignoreCase = true) }) {
                map[m.agent] = (map[m.agent] ?: 0) + 1
            }
        }
        map
    }

    val outsiders = remember(messages, memberSet) {
        messages.map { it.agent }.filter { !memberSet.contains(it) && !isRoleWorker(it, "") }.distinct()
    }

    var allNodeIds = remember(members, outsiders, hideInactive, currentReplayMessage, activeSet) {
        var list = members.map { it.name } + outsiders
        if (hideInactive) {
            val curDirect = mutableSetOf<String>()
            if (currentReplayMessage != null && !isRoleWorker(currentReplayMessage.agent, "")) {
                curDirect.add(currentReplayMessage.agent)
                extractTargets(currentReplayMessage).forEach { curDirect.add(it) }
            }
            list = list.filter { id ->
                val isKilled = memberByName[id]?.killed == true
                curDirect.contains(id) || (activeSet.contains(id) && !isKilled)
            }
        }
        list
    }

    val currentTargets = remember(currentReplayMessage, expandAll, allNamesForBroadcast) {
        if (currentReplayMessage == null) emptySet<String>()
        else {
            val set = extractTargets(currentReplayMessage).toMutableSet()
            if (expandAll && currentReplayMessage.mentions.any { it.equals("all", ignoreCase = true) }) {
                allNamesForBroadcast.filter { !it.equals(currentReplayMessage.agent, ignoreCase = true) }.forEach { set.add(it) }
            }
            set
        }
    }

    val canvasHeightDp: Dp = if (allNodeIds.size <= 2) 200.dp else if (allNodeIds.size <= 10) 300.dp else 350.dp

    BoxWithConstraints(
        modifier = Modifier
            .fillMaxWidth()
            .height(canvasHeightDp)
            .background(SwarmColors.bg(isDark))
    ) {
        val widthPx = constraints.maxWidth.toFloat().coerceAtLeast(300f)
        val heightPx = with(density) { canvasHeightDp.toPx() }

        val positions = remember(allNodeIds, widthPx, heightPx) {
            computeNodeLayout(allNodeIds, widthPx, heightPx, memberByName)
        }
        val posMap = remember(positions) { positions.associateBy { it.id } }

        val maxEdgeTotal = remember(edges, posMap) {
            edges.filter { posMap.containsKey(it.key.first) && posMap.containsKey(it.key.second) }
                .map { it.value.first + it.value.second }
                .maxOrNull()?.coerceAtLeast(1) ?: 1
        }

        // Draw curved Bezier edges, arrows and count badges
        Canvas(modifier = Modifier.fillMaxSize()) {
            edges.forEach { (key, stat) ->
                val a = posMap[key.first] ?: return@forEach
                val b = posMap[key.second] ?: return@forEach
                val totalCount = stat.first + stat.second

                val isCurEdge = currentReplayMessage?.let {
                    it.agent == key.first && currentTargets.contains(key.second)
                } ?: false
                val isSelectedEdge = selectedNodeId?.let {
                    it == key.first || it == key.second
                } ?: false

                val strokeW = if (isCurEdge || isSelectedEdge) {
                    max(2.2f * density.density, (1.2f + 2.8f * totalCount / maxEdgeTotal) * density.density)
                } else {
                    (1.0f + 2.4f * totalCount / maxEdgeTotal) * density.density
                }

                val edgeColor = when {
                    isCurEdge -> SwarmColors.ink(isDark)
                    isSelectedEdge -> SwarmColors.accent(isDark)
                    selectedNodeId != null -> SwarmColors.faint(isDark).copy(alpha = 0.28f)
                    else -> SwarmColors.muted(isDark).copy(alpha = 0.45f)
                }

                val geom = calcEdgeGeometry(a, b)

                val path = Path().apply {
                    moveTo(geom.start.x, geom.start.y)
                    quadraticTo(geom.ctrl.x, geom.ctrl.y, geom.end.x, geom.end.y)
                }

                val pathEffect = if (stat.first > 0) null else PathEffect.dashPathEffect(floatArrayOf(12f, 8f))
                drawPath(
                    path = path,
                    color = edgeColor,
                    style = Stroke(width = strokeW, cap = StrokeCap.Round, pathEffect = pathEffect)
                )

                // Arrowhead
                val arrowPath = createArrowhead(tip = geom.end, from = geom.ctrl, size = if (isCurEdge) 18f else 14f)
                drawPath(arrowPath, color = edgeColor)

                // Current message travelling green dot at arrowhead
                if (isCurEdge) {
                    drawCircle(color = SwarmColors.liveGreen(isDark), radius = 5f * density.density, center = geom.end)
                    drawCircle(color = SwarmColors.bg(isDark), radius = 5f * density.density, center = geom.end, style = Stroke(width = 1.5f * density.density))
                }

                // Midpoint count
                if (totalCount > 1) {
                    val mid = Offset(
                        x = 0.25f * geom.start.x + 0.5f * geom.ctrl.x + 0.25f * geom.end.x,
                        y = 0.25f * geom.start.y + 0.5f * geom.ctrl.y + 0.25f * geom.end.y - 10f
                    )
                    drawContext.canvas.nativeCanvas.apply {
                        val paint = android.graphics.Paint().apply {
                            color = (if (isCurEdge) SwarmColors.ink(isDark) else SwarmColors.muted(isDark)).hashCode()
                            textSize = 10f * density.density
                            textAlign = android.graphics.Paint.Align.CENTER
                            isFakeBoldText = true
                        }
                        drawText(totalCount.toString(), mid.x, mid.y, paint)
                    }
                }
            }
        }

        // Tappable node pills
        positions.forEach { node ->
            val status = node.member?.status ?: ""
            val isHot = currentReplayMessage?.let { it.agent == node.id || currentTargets.contains(node.id) } ?: false
            val isSelected = selectedNodeId == node.id
            val isActiveNode = activeSet.contains(node.id)
            val bcast = if (expandAll) 0 else (broadcasts[node.id] ?: 0)

            val nodeWidthDp = with(density) { node.w.toDp() }
            val nodeHeightDp = with(density) { node.h.toDp() }
            val xDp = with(density) { (node.center.x - node.w / 2f).toDp() }
            val yDp = with(density) { (node.center.y - node.h / 2f).toDp() }

            Box(
                modifier = Modifier
                    .offset(x = xDp, y = yDp)
                    .size(nodeWidthDp, nodeHeightDp)
                    .clip(CircleShape)
                    .background(if (isHot || isSelected) SwarmColors.surfaceBg(isDark) else SwarmColors.cardBg(isDark))
                    .border(
                        width = if (isSelected || isHot) 1.6.dp else 0.9.dp,
                        color = when {
                            isSelected -> SwarmColors.accent(isDark)
                            isHot -> SwarmColors.ink(isDark)
                            isActiveNode -> Color(0xFF4A4D54).copy(alpha = 0.7f)
                            else -> SwarmColors.border(isDark)
                        },
                        shape = CircleShape
                    )
                    .clickable { onSelectNode(node.id) }
                    .padding(horizontal = 8.dp),
                contentAlignment = Alignment.Center
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(5.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(6.5.dp)
                            .clip(CircleShape)
                            .background(
                                when (status.lowercase()) {
                                    "working" -> SwarmColors.liveGreen(isDark)
                                    "error", "stuck" -> SwarmColors.errorRed(isDark)
                                    else -> Color.Transparent
                                }
                            )
                            .border(
                                width = 1.2.dp,
                                color = if (status.lowercase() == "working" || status.lowercase() == "error") Color.Transparent
                                else SwarmColors.muted(isDark),
                                shape = CircleShape
                            )
                    )

                    Text(
                        text = node.id,
                        fontSize = 11.5.sp,
                        fontWeight = if (isHot || isSelected || node.id == "coordinator") FontWeight.SemiBold else FontWeight.Medium,
                        color = if (isActiveNode) SwarmColors.ink(isDark) else SwarmColors.faint(isDark),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )

                    if (bcast > 0) {
                        Text(
                            text = "@all $bcast",
                            fontSize = 8.5.sp,
                            fontFamily = FontFamily.Monospace,
                            fontWeight = FontWeight.SemiBold,
                            color = SwarmColors.ink(isDark),
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(SwarmColors.codeBg(isDark))
                                .border(0.6.dp, SwarmColors.border(isDark), CircleShape)
                                .padding(horizontal = 4.dp, vertical = 1.dp)
                        )
                    }
                }
            }
        }
    }
}

private fun computeNodeLayout(
    nodeIds: List<String>,
    width: Float,
    height: Float,
    memberByName: Map<String, SwarmMemberInfo>
): List<NodeLayoutPos> {
    val n = nodeIds.size
    if (n == 0) return emptyList()
    val nodeH = 28f * 2.5f

    if (n <= 16) {
        val maxPillW = min(136f * 2.5f, max(100f * 2.5f, width / max(3.2f, ceil(n / 2.0f) + 0.6f) - 10f))
        val rx = max(56f * 2.5f, width / 2f - maxPillW / 2f - 14f)
        val ry = max(50f * 2.5f, height / 2f - nodeH / 2f - 24f)
        val cx = width / 2f
        val cy = height / 2f

        return nodeIds.mapIndexed { idx, id ->
            val angle = -PI / 2.0 + (2.0 * PI * idx.toDouble()) / n.toDouble()
            val w = min(maxPillW, max(74f * 2.5f, 32f * 2.5f + id.length * 6.5f * 2.5f))
            NodeLayoutPos(
                id = id,
                center = Offset(cx + rx * cos(angle).toFloat(), cy + ry * sin(angle).toFloat()),
                w = w,
                h = nodeH,
                member = memberByName[id]
            )
        }
    }

    val perRow = max(2, (width / 260f).toInt())
    val result = mutableListOf<NodeLayoutPos>()
    val rows = ceil(n.toDouble() / perRow.toDouble()).toInt()
    val rowStep = max(110f, (height - 90f) / max(1, rows))

    nodeIds.forEachIndexed { idx, id ->
        val row = idx / perRow
        val col = idx % perRow
        val countInRow = min(perRow, n - row * perRow)
        val gap = width / (countInRow + 1f)
        val w = min(320f, max(180f, gap - 20f))
        result.add(
            NodeLayoutPos(
                id = id,
                center = Offset(gap * (col + 1), 60f + row * rowStep),
                w = w,
                h = nodeH,
                member = memberByName[id]
            )
        )
    }
    return result
}

private data class EdgeGeom(val start: Offset, val ctrl: Offset, val end: Offset)

private fun calcEdgeGeometry(a: NodeLayoutPos, b: NodeLayoutPos): EdgeGeom {
    val dx = b.center.x - a.center.x
    val dy = b.center.y - a.center.y
    val len = max(1f, hypot(dx, dy))
    val nx = -dy / len
    val ny = dx / len
    val bend = 30f
    val mx = (a.center.x + b.center.x) / 2f + nx * bend
    val my = (a.center.y + b.center.y) / 2f + ny * bend
    val ctrl = Offset(mx, my)

    fun trim(node: NodeLayoutPos, toward: Offset): Offset {
        val tx = toward.x - node.center.x
        val ty = toward.y - node.center.y
        val l = max(1f, hypot(tx, ty))
        val ux = tx / l
        val uy = ty / l
        val bx = if (abs(ux) > 0.001f) (node.w / 2f) / abs(ux) else Float.POSITIVE_INFINITY
        val by = if (abs(uy) > 0.001f) (node.h / 2f) / abs(uy) else Float.POSITIVE_INFINITY
        val t = min(0.45f * l, min(bx, by) + 6f)
        return Offset(node.center.x + ux * t, node.center.y + uy * t)
    }

    return EdgeGeom(
        start = trim(a, ctrl),
        ctrl = ctrl,
        end = trim(b, ctrl)
    )
}

private fun createArrowhead(tip: Offset, from: Offset, size: Float): Path {
    val angle = atan2(tip.y - from.y, tip.x - from.x)
    val spread = PI.toFloat() / 6.5f
    val p1 = Offset(tip.x - size * cos(angle - spread), tip.y - size * sin(angle - spread))
    val p2 = Offset(tip.x - size * cos(angle + spread), tip.y - size * sin(angle + spread))
    return Path().apply {
        moveTo(tip.x, tip.y)
        lineTo(p1.x, p1.y)
        lineTo(p2.x, p2.y)
        close()
    }
}

@Composable
private fun DetailMessageCard(
    msg: SwarmBoardMessage,
    selectedMember: SwarmMemberInfo?,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val targets = extractTargets(msg)
    val cleanBody = if (targets.isEmpty()) msg.text else stripLeadingMentions(msg.text)

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 14.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(SwarmColors.cardBg(isDark))
            .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = msg.agent,
                fontSize = 13.5.sp,
                fontWeight = FontWeight.Bold,
                color = SwarmColors.ink(isDark)
            )

            if (targets.isNotEmpty()) {
                Text(text = "→", fontSize = 12.sp, color = SwarmColors.faint(isDark))
                Text(
                    text = targets.joinToString(" ") { "@$it" },
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.Medium,
                    color = SwarmColors.muted(isDark),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }

            Spacer(modifier = Modifier.weight(1f))

            if (selectedMember != null && selectedMember.conversationId.isNotBlank() && !selectedMember.isCurrent) {
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(SwarmColors.accent(isDark).copy(alpha = 0.12f))
                        .clickable { onSelectConversation(selectedMember.conversationId) }
                        .padding(horizontal = 8.dp, vertical = 3.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                        Text(
                            text = "进入子会话",
                            fontSize = 11.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = SwarmColors.accent(isDark)
                        )
                        Icon(
                            imageVector = Icons.Default.OpenInNew,
                            contentDescription = null,
                            tint = SwarmColors.accent(isDark),
                            modifier = Modifier.size(10.dp)
                        )
                    }
                }
            }

            Text(
                text = "#${msg.seq} · ${formatClock(msg.time)}",
                fontSize = 11.sp,
                fontFamily = FontFamily.Monospace,
                color = SwarmColors.faint(isDark)
            )
        }

        Text(
            text = cleanBody,
            fontSize = 12.5.sp,
            lineHeight = 17.sp,
            color = SwarmColors.ink(isDark)
        )
    }
}
