package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
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

    var graphMode by remember { mutableStateOf("hive") } // "hive" | "mentions"
    var replayIndex by remember { mutableStateOf<Int?>(null) } // null == Live
    var isPlaying by remember { mutableStateOf(false) }
    var replayPacing by remember { mutableStateOf("even") } // "even" | "real"
    var replaySpeed by remember { mutableDoubleStateOf(1.0) }
    var selectedCellId by remember { mutableStateOf<String?>(null) }
    var hiveZoom by remember { mutableFloatStateOf(1f) }
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

    val displayDetailMessage = remember(currentReplayMessage, selectedNodeId, allMessages, graphMode) {
        if (currentReplayMessage != null) return@remember currentReplayMessage
        if (graphMode == "mentions") {
            val sel = selectedNodeId
            if (sel != null) {
                allMessages.lastOrNull { it.agent.equals(sel, ignoreCase = true) }
                    ?: allMessages.lastOrNull { m -> m.mentions.any { it.equals(sel, ignoreCase = true) } }
            } else {
                allMessages.lastOrNull()
            }
        } else null
    }

    val memberByName = remember(snap.members) {
        snap.members.associateBy { it.name }
    }

    val taskMembers = remember(snap.members) {
        snap.members.filter { !it.roleWorker && !isRoleWorker(it.name, it.role) }.sortedWith { a, b ->
            if (a.name.equals("coordinator", ignoreCase = true)) -1
            else if (b.name.equals("coordinator", ignoreCase = true)) 1
            else a.name.compareTo(b.name)
        }
    }

    val synth = remember(snap) { SwarmHiveEngine.synthesize(snap) }
    val replaySeq = if (replayIndex != null) (currentReplayMessage?.seq ?: 0) else null
    val hiveModel = remember(snap, synth, replaySeq) {
        SwarmHiveEngine.buildHiveMap(snap, synth, replaySeq)
    }

    LaunchedEffect(isPlaying, replayPacing, replaySpeed) {
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
                val cur = replayIndex ?: total
                val spd = replaySpeed.coerceAtLeast(0.25)
                val waitMs = if (replayPacing == "even" || cur <= 0 || cur >= allMessages.size) {
                    (1400.0 / spd).toLong()
                } else {
                    val t1 = SwarmHiveEngine.parseSwarmTimeMs(allMessages[cur - 1].time)
                    val t2 = SwarmHiveEngine.parseSwarmTimeMs(allMessages[cur].time)
                    if (t1 != null && t2 != null) {
                        val gap = (t2 - t1).coerceAtLeast(0L).toDouble()
                        (gap / 60.0 / spd).coerceIn(250.0, 4000.0).toLong()
                    } else (1100.0 / spd).toLong()
                }
                delay(waitMs)
                val latestTotal = allMessages.size
                val nowPos = replayIndex ?: latestTotal
                if (nowPos < latestTotal) {
                    replayIndex = nowPos + 1
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
            .padding(top = 12.dp, bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Replay & Hive/Mentions Controls
        ReplayControlsBar(
            total = allMessages.size,
            currentPos = replayIndex ?: allMessages.size,
            isLive = replayIndex == null,
            isPlaying = isPlaying,
            currentMsg = currentReplayMessage,
            firstMsg = allMessages.firstOrNull(),
            lastMsg = allMessages.lastOrNull(),
            graphMode = graphMode,
            replayPacing = replayPacing,
            replaySpeed = replaySpeed,
            onGraphModeChange = { graphMode = it },
            onPacingChange = { replayPacing = it },
            onSpeedChange = { replaySpeed = it },
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
            }
        )

        if (graphMode == "hive") {
            HiveHoneycombSectionAndroid(
                snap = snap,
                synth = synth,
                hiveModel = hiveModel,
                selectedCellId = selectedCellId,
                hiveZoom = hiveZoom,
                onZoomChange = { hiveZoom = it },
                onSelectCell = { id -> selectedCellId = if (selectedCellId == id) null else id },
                onSelectConversation = onSelectConversation
            )

            // Hive Legend
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(14.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Box(modifier = Modifier.width(16.dp).height(2.dp).clip(CircleShape).background(SwarmColors.liveGreen(isDark)))
                    Text("Tasks done", fontSize = 11.5.sp, color = SwarmColors.muted(isDark))
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Box(modifier = Modifier.width(16.dp).height(2.dp).clip(CircleShape).background(SwarmColors.accent(isDark).copy(alpha = 0.75f)))
                    Text("Message just sent", fontSize = 11.5.sp, color = SwarmColors.muted(isDark))
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
                    Row(horizontalArrangement = Arrangement.spacedBy(2.dp), verticalAlignment = Alignment.CenterVertically) {
                        Box(modifier = Modifier.width(4.dp).height(2.dp).background(SwarmColors.warnAmber(isDark)))
                        Box(modifier = Modifier.width(4.dp).height(2.dp).background(SwarmColors.warnAmber(isDark)))
                        Icon(Icons.Default.PlayArrow, contentDescription = null, tint = SwarmColors.warnAmber(isDark), modifier = Modifier.size(9.dp))
                    }
                    Text("Waiting on", fontSize = 11.5.sp, color = SwarmColors.muted(isDark))
                }
            }

            // Selected Thread Card
            val selCell = hiveModel.cells.firstOrNull { it.id == selectedCellId }
            if (selCell != null) {
                SelectedHiveCellCardAndroid(
                    cell = selCell,
                    snap = snap,
                    synth = synth,
                    hiveModel = hiveModel,
                    onClose = { selectedCellId = null },
                    onSelectConversation = onSelectConversation
                )
            }
        } else {
            // Mentions filter chips
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                ToggleChip(title = "隐藏未活跃".loc, isOn = hideInactive, onClick = { hideInactive = !hideInactive })
                ToggleChip(title = "展开 @all".loc, isOn = expandAll, onClick = { expandAll = !expandAll })
            }

            // Mentions Topology Canvas
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

            // Mentions Legend Row
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(14.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "→ @提及 (线宽 = 次数)".loc,
                    fontSize = 11.sp,
                    color = SwarmColors.muted(isDark)
                )
                Text(
                    text = "⇢ 经 @all".loc,
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
    lastMsg: SwarmBoardMessage?,
    graphMode: String,
    replayPacing: String,
    replaySpeed: Double,
    onGraphModeChange: (String) -> Unit,
    onPacingChange: (String) -> Unit,
    onSpeedChange: (Double) -> Unit,
    onPlayPause: () -> Unit,
    onStepBack: () -> Unit,
    onStepForward: () -> Unit,
    onStart: () -> Unit,
    onSeek: (Int) -> Unit,
    onGoLive: () -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    var showPacingMenu by remember { mutableStateOf(false) }
    var showSpeedMenu by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp),
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
                    contentDescription = "起始".loc,
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
                    contentDescription = "上一条".loc,
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
                    contentDescription = if (isPlaying) "暂停".loc else "播放".loc,
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
                    contentDescription = "下一条".loc,
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
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            if (currentMsg != null) {
                Text(
                    text = "$currentPos of $total · #${currentMsg.seq} · ${formatClock(currentMsg.time)}",
                    fontSize = 11.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = SwarmColors.muted(isDark),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f)
                )
            } else {
                val span = SwarmHiveEngine.formatSpanDuration(firstMsg?.time, lastMsg?.time)
                Row(modifier = Modifier.weight(1f), horizontalArrangement = Arrangement.spacedBy(3.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = "$total ${if (total == 1) "post" else "posts"}",
                        fontSize = 12.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = SwarmColors.ink(isDark),
                        maxLines = 1
                    )
                    if (span != null) {
                        Text(
                            text = "over $span",
                            fontSize = 12.sp,
                            color = SwarmColors.muted(isDark),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }

            // Segmented Hive | Mentions Pill
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .background(SwarmColors.fieldBg(isDark))
                    .padding(2.dp),
                horizontalArrangement = Arrangement.spacedBy(2.dp)
            ) {
                listOf("hive" to "Hive", "mentions" to "Mentions").forEach { (key, label) ->
                    val sel = graphMode == key
                    Box(
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(if (sel) SwarmColors.surfaceBg(isDark) else Color.Transparent)
                            .clickable { onGraphModeChange(key) }
                            .padding(horizontal = 9.dp, vertical = 4.5.dp)
                    ) {
                        Text(
                            text = label,
                            fontSize = 11.5.sp,
                            fontWeight = if (sel) FontWeight.SemiBold else FontWeight.Medium,
                            color = if (sel) SwarmColors.ink(isDark) else SwarmColors.muted(isDark)
                        )
                    }
                }
            }

            // Pacing Dropdown
            Box {
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(6.dp))
                        .background(SwarmColors.fieldBg(isDark).copy(alpha = 0.75f))
                        .clickable { showPacingMenu = true }
                        .padding(horizontal = 7.dp, vertical = 5.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(2.dp)
                ) {
                    Text(
                        text = if (replayPacing == "even") "Even" else "Real-time",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        color = SwarmColors.muted(isDark)
                    )
                    Icon(Icons.Default.KeyboardArrowDown, contentDescription = null, tint = SwarmColors.muted(isDark), modifier = Modifier.size(13.dp))
                }
                DropdownMenu(expanded = showPacingMenu, onDismissRequest = { showPacingMenu = false }) {
                    DropdownMenuItem(text = { Text("Even pacing") }, onClick = { onPacingChange("even"); showPacingMenu = false })
                    DropdownMenuItem(text = { Text("Real-time gaps") }, onClick = { onPacingChange("real"); showPacingMenu = false })
                }
            }

            // Speed Dropdown
            val speedFmt = { s: Double -> if (s == floor(s)) "${s.toInt()}×" else "${s}×" }
            Box {
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(6.dp))
                        .background(SwarmColors.fieldBg(isDark).copy(alpha = 0.75f))
                        .clickable { showSpeedMenu = true }
                        .padding(horizontal = 6.5.dp, vertical = 5.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(2.dp)
                ) {
                    Text(
                        text = speedFmt(replaySpeed),
                        fontSize = 11.sp,
                        fontFamily = FontFamily.Monospace,
                        fontWeight = FontWeight.Medium,
                        color = SwarmColors.muted(isDark)
                    )
                    Icon(Icons.Default.KeyboardArrowDown, contentDescription = null, tint = SwarmColors.muted(isDark), modifier = Modifier.size(13.dp))
                }
                DropdownMenu(expanded = showSpeedMenu, onDismissRequest = { showSpeedMenu = false }) {
                    listOf(0.5, 1.0, 2.0, 4.0, 8.0).forEach { sp ->
                        DropdownMenuItem(text = { Text(speedFmt(sp)) }, onClick = { onSpeedChange(sp); showSpeedMenu = false })
                    }
                }
            }
        }
    }
}

// MARK: - Honeycomb Hive Map Composable (Ported from swarm_board/hive.js)

private data class HiveBeePosAndroid(
    val name: String,
    val cx: Float,
    val cy: Float,
    val spriteW: Float,
    val spriteH: Float,
    val padRadius: Float,
    val showName: Boolean,
    val isQueen: Boolean
)

private data class HiveCellPosAndroid(
    val id: String,
    val cell: SwarmHiveCell?,
    val cx: Float,
    val cy: Float,
    val fractionDone: Float,
    val isSelected: Boolean
)

@Composable
private fun HiveHoneycombSectionAndroid(
    snap: SwarmSnapshot,
    synth: SwarmSynthesizedModel,
    hiveModel: SwarmHiveMapModel,
    selectedCellId: String?,
    hiveZoom: Float,
    onZoomChange: (Float) -> Unit,
    onSelectCell: (String) -> Unit,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    val density = LocalDensity.current

    val beamAnim = remember { Animatable(1f) }
    var lastBeamSeq by remember { mutableStateOf<Int?>(null) }
    LaunchedEffect(hiveModel.beamSeq) {
        val s = hiveModel.beamSeq
        if (s != null && s != lastBeamSeq) {
            lastBeamSeq = s
            beamAnim.snapTo(0f)
            beamAnim.animateTo(1f, tween(680, easing = FastOutSlowInEasing))
        }
    }

    val infinite = rememberInfiniteTransition(label = "hiveMarch")
    val waitPhase by infinite.animateFloat(
        initialValue = 0f,
        targetValue = 16f,
        animationSpec = infiniteRepeatable(tween(1000, easing = LinearEasing)),
        label = "waitDash"
    )

    val viewportHeight = when {
        hiveModel.cells.isEmpty() -> 220.dp
        hiveModel.cells.size == 1 -> 290.dp
        hiveModel.cells.size <= 3 -> 340.dp
        hiveModel.cells.size <= 6 -> 375.dp
        else -> 390.dp
    }

    BoxWithConstraints(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 10.dp)
            .height(viewportHeight)
    ) {
        val availWDp = maxWidth.value.coerceAtLeast(300f)
        val k = (sqrt(3.0) / 2.0).toFloat()
        fun ux(q: Int) = 1.5f * q
        fun uy(q: Int, r: Int) = 2f * k * (r + q / 2f)

        val xs = listOf(0f) + hiveModel.cells.map { ux(it.q) }
        val ys = listOf(0f) + hiveModel.cells.map { uy(it.q, it.r) }
        val minX = xs.minOrNull() ?: 0f
        val maxX = xs.maxOrNull() ?: 0f
        val minY = ys.minOrNull() ?: 0f
        val maxY = ys.maxOrNull() ?: 0f
        val spanX = maxOf(2f, (maxX - minX) + 2f)
        val spanY = maxOf(2f * k, (maxY - minY) + 2f * k)

        val fit = (availWDp - 4f) / spanX
        val gap = if (fit < 44f) 3.5f else 5.5f
        val rFit = (fit - gap / 2f).coerceIn(18f, 104f)
        val maxZ = maxOf(1f, 104f / rFit)
        val clampedZoom = hiveZoom.coerceIn(1f, maxZ)
        val rDp = minOf(104f, rFit * clampedZoom)
        val stepG = rDp + gap / 2f

        val canvasWDp = maxOf(availWDp, ceil(spanX * stepG + 8f))
        val canvasHDp = ceil(spanY * stepG + 14f)
        val ox = canvasWDp / 2f - ((maxX + minX) / 2f) * stepG
        val oy = 7f + k * rDp - minY * stepG

        val cellPlacements = mutableListOf<HiveCellPosAndroid>()
        val beePlacements = mutableListOf<HiveBeePosAndroid>()

        val hubFrac = if (hiveModel.allTotal > 0) hiveModel.allDone.toFloat() / hiveModel.allTotal.toFloat() else 0f
        cellPlacements.add(HiveCellPosAndroid("_hub", null, ox, oy, hubFrac, false))

        val queenW = if (rDp >= 82f) 56f else if (rDp >= 52f) 42f else 26f
        val queenH = queenW * 18f / 22f
        beePlacements.add(HiveBeePosAndroid("coordinator", ox, oy - (if (rDp >= 52f) rDp * 0.14f else 4f), queenW, queenH, queenW * 0.56f, false, true))

        val showNames = rDp >= 56f
        val workerW = if (rDp >= 82f) 38f else if (rDp >= 56f) 28f else 20f
        val workerH = workerW * 18f / 22f

        for (cell in hiveModel.cells) {
            val cx = ox + ux(cell.q) * stepG
            val cy = oy + uy(cell.q, cell.r) * stepG
            val frac = if (cell.countOnly != null) (if (cell.done > 0) 1f else 0f) else (if (cell.total > 0) cell.done.toFloat() / cell.total.toFloat() else 0f)
            cellPlacements.add(HiveCellPosAndroid(cell.id, cell, cx, cy, frac, selectedCellId == cell.id))

            val born = cell.bees.filter { (hiveModel.beeStates[it] ?: SwarmHiveBeeState.IDLE) != SwarmHiveBeeState.UNBORN }
                .sortedWith(compareBy<String> {
                    when (hiveModel.beeStates[it]) {
                        SwarmHiveBeeState.ISSUE -> 0
                        SwarmHiveBeeState.WAIT -> 1
                        SwarmHiveBeeState.WORKING -> 2
                        else -> 3
                    }
                }.thenBy { it })

            if (born.isEmpty()) continue
            val perRow = if (rDp >= 56f) (1.7f * rDp / 38f).toInt().coerceIn(2, 3) else 2
            val shown = born.take(perRow * 2)
            val rows = if (shown.size > perRow) listOf(shown.take(perRow), shown.drop(perRow)) else listOf(shown)
            val horizStep = minOf(if (rDp >= 82f) 50f else 34f, (1.62f * rDp) / maxOf(1, perRow))
            val rowGap = if (showNames) (if (rDp >= 82f) 44f else 33f) else 24f
            val topLim = cy - k * rDp + (if (rDp >= 82f) 46f else 36f)
            val botLim = cy + k * rDp - (if (showNames) 16f else 8f)
            val baseY = (topLim + botLim) / 2f - (rows.size - 1) * rowGap / 2f

            rows.forEachIndexed { ri, row ->
                row.forEachIndexed { ci, name ->
                    val bx = cx + (ci - (row.size - 1) / 2f) * horizStep
                    val by = baseY + ri * rowGap
                    beePlacements.add(HiveBeePosAndroid(name, bx, by, workerW, workerH, workerW * 0.56f, showNames, false))
                }
            }
        }
        val beeMap = beePlacements.associateBy { it.name }

        val hScroll = rememberScrollState()
        Box(modifier = Modifier.fillMaxSize()) {
            Box(
                modifier = if (clampedZoom > 1.01f) Modifier.fillMaxSize().horizontalScroll(hScroll) else Modifier.fillMaxSize()
            ) {
                Box(modifier = Modifier.size(canvasWDp.dp, canvasHDp.dp)) {
                    val surfCol = SwarmColors.surfaceBg(isDark)
                    val borderCol = SwarmColors.border(isDark)
                    val greenCol = SwarmColors.liveGreen(isDark)
                    val accentCol = SwarmColors.accent(isDark)
                    val warnCol = SwarmColors.warnAmber(isDark)

                    Canvas(modifier = Modifier.fillMaxSize()) {
                        val pxPerDp = this.density
                        fun buildHex(cxPx: Float, cyPx: Float, rPx: Float, startDeg: Double = 0.0): Path {
                            val p = Path()
                            for (idx in 0 until 6) {
                                val a = (startDeg + idx * 60.0) * PI / 180.0
                                val x = cxPx + rPx * cos(a).toFloat()
                                val y = cyPx + rPx * sin(a).toFloat()
                                if (idx == 0) p.moveTo(x, y) else p.lineTo(x, y)
                            }
                            p.close()
                            return p
                        }

                        val rPx = rDp * pxPerDp
                        for (cp in cellPlacements) {
                            val cxPx = cp.cx * pxPerDp
                            val cyPx = cp.cy * pxPerDp
                            val hex = buildHex(cxPx, cyPx, rPx)
                            val isFull = cp.fractionDone >= 1f && (cp.cell?.total ?: hiveModel.allTotal) > 0
                            val fill = when {
                                cp.isSelected -> accentCol.copy(alpha = 0.08f)
                                isFull -> greenCol.copy(alpha = 0.08f)
                                else -> surfCol
                            }
                            drawPath(hex, color = fill)
                            drawPath(hex, color = borderCol, style = Stroke(width = 1f * pxPerDp))

                            if (cp.fractionDone > 0f) {
                                val progFull = buildHex(cxPx, cyPx, rPx - 2f * pxPerDp, -120.0)
                                val pm = PathMeasure()
                                pm.setPath(progFull, false)
                                val seg = Path()
                                pm.getSegment(0f, pm.length * cp.fractionDone.coerceIn(0f, 1f), seg, true)
                                drawPath(
                                    seg,
                                    color = greenCol,
                                    style = Stroke(width = 2.4f * pxPerDp, cap = StrokeCap.Round, join = StrokeJoin.Round)
                                )
                            }

                            if (cp.isSelected) {
                                val selHex = buildHex(cxPx, cyPx, rPx + 1.5f * pxPerDp)
                                drawPath(selHex, color = accentCol, style = Stroke(width = 2.2f * pxPerDp))
                            }
                        }

                        fun quadCurve(a: HiveBeePosAndroid, b: HiveBeePosAndroid, cutDp: Float): Triple<Offset, Offset, Pair<Offset, Offset>>? {
                            val dx = b.cx - a.cx
                            val dy = b.cy - a.cy
                            val len = hypot(dx, dy)
                            if (len < a.padRadius + b.padRadius + 6f) return null
                            val vx = dx / len; val vy = dy / len
                            val x1 = a.cx + vx * a.padRadius; val y1 = a.cy + vy * a.padRadius
                            val x2 = b.cx - vx * b.padRadius; val y2 = b.cy - vy * b.padRadius
                            val bend = minOf(24f, (len - a.padRadius - b.padRadius) * 0.2f)
                            val mx = (x1 + x2) / 2f - vy * bend; val my = (y1 + y2) / 2f + vx * bend
                            val hl = maxOf(1f, hypot(x2 - mx, y2 - my))
                            val ex = x2 - ((x2 - mx) / hl) * cutDp
                            val ey = y2 - ((y2 - my) / hl) * cutDp
                            return Triple(
                                Offset(x1 * pxPerDp, y1 * pxPerDp),
                                Offset(mx * pxPerDp, my * pxPerDp),
                                Offset(ex * pxPerDp, ey * pxPerDp) to Offset(x2 * pxPerDp, y2 * pxPerDp)
                            )
                        }

                        // Waiting dashed lines
                        for ((fromN, toN) in hiveModel.waitLinks) {
                            val a = beeMap[fromN] ?: continue
                            val b = beeMap[toN] ?: continue
                            val (s, ctrl, ends) = quadCurve(a, b, 8f) ?: continue
                            val p = Path().apply {
                                moveTo(s.x, s.y)
                                quadraticBezierTo(ctrl.x, ctrl.y, ends.first.x, ends.first.y)
                            }
                            drawPath(
                                p,
                                color = warnCol.copy(alpha = 0.7f),
                                style = Stroke(
                                    width = 1.6f * pxPerDp,
                                    cap = StrokeCap.Round,
                                    pathEffect = PathEffect.dashPathEffect(floatArrayOf(4f * pxPerDp, 4f * pxPerDp), waitPhase * pxPerDp)
                                )
                            )
                            val arrow = createArrowhead(ends.second, ctrl, 7.5f * pxPerDp)
                            drawPath(arrow, color = warnCol.copy(alpha = 0.75f))
                        }

                        // Recent / Replay Message Beam
                        val bFrom = hiveModel.beamFrom?.let { beeMap[it] }
                        if (bFrom != null) {
                            for (tName in hiveModel.beamTargets) {
                                val bTo = beeMap[tName] ?: continue
                                val (s, ctrl, ends) = quadCurve(bFrom, bTo, 0f) ?: continue
                                val endPt = ends.first
                                val p = Path().apply {
                                    moveTo(s.x, s.y)
                                    quadraticBezierTo(ctrl.x, ctrl.y, endPt.x, endPt.y)
                                }
                                drawPath(p, color = accentCol.copy(alpha = 0.62f), style = Stroke(width = 1.6f * pxPerDp, cap = StrokeCap.Round))
                                val t = beamAnim.value
                                val u = 1f - t
                                val dotX = u * u * s.x + 2f * u * t * ctrl.x + t * t * endPt.x
                                val dotY = u * u * s.y + 2f * u * t * ctrl.y + t * t * endPt.y
                                drawCircle(color = accentCol, radius = 3f * pxPerDp, center = Offset(dotX, dotY))
                            }
                        }
                    }

                    // Cell Titles & Hit Areas
                    cellPlacements.forEach { cp ->
                        val cell = cp.cell
                        if (cell != null) {
                            Box(
                                modifier = Modifier
                                    .offset(x = (cp.cx - rDp).dp, y = (cp.cy - k * rDp).dp)
                                    .size((rDp * 2f).dp, (k * rDp * 2f).dp)
                                    .clickable { onSelectCell(cell.id) },
                                contentAlignment = Alignment.TopCenter
                            ) {
                                if (rDp >= 38f) {
                                    Column(
                                        horizontalAlignment = Alignment.CenterHorizontally,
                                        modifier = Modifier.padding(top = if (rDp >= 80f) 12.dp else 9.dp).widthIn(max = (rDp * 1.32f).dp)
                                    ) {
                                        Text(
                                            text = cell.title,
                                            fontSize = if (rDp >= 80f) 12.sp else 10.5.sp,
                                            fontWeight = FontWeight.SemiBold,
                                            color = SwarmColors.ink(isDark),
                                            textAlign = TextAlign.Center,
                                            maxLines = if (rDp >= 56f) 2 else 1,
                                            overflow = TextOverflow.Ellipsis,
                                            lineHeight = 13.sp
                                        )
                                        Text(
                                            text = cell.countOnly?.toString() ?: "${cell.done}/${cell.total}",
                                            fontSize = if (rDp >= 80f) 10.5.sp else 9.5.sp,
                                            color = SwarmColors.muted(isDark)
                                        )
                                    }
                                }
                            }
                        } else {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                modifier = Modifier
                                    .offset(x = (cp.cx - rDp * 0.7f).dp, y = (cp.cy + rDp * 0.34f).dp)
                                    .width((rDp * 1.4f).dp)
                            ) {
                                if (rDp >= 52f) {
                                    Text("Coordinator", fontSize = if (rDp >= 80f) 12.sp else 11.sp, fontWeight = FontWeight.SemiBold, color = SwarmColors.ink(isDark))
                                }
                                Text(
                                    text = if (hiveModel.coordinatorNeedsYou) "Needs you" else if (rDp >= 52f) "${hiveModel.allDone} of ${hiveModel.allTotal} done" else "${hiveModel.allDone}/${hiveModel.allTotal}",
                                    fontSize = if (rDp >= 80f) 10.5.sp else 9.5.sp,
                                    color = if (hiveModel.coordinatorNeedsYou) SwarmColors.warnAmber(isDark) else SwarmColors.muted(isDark)
                                )
                            }
                        }
                    }

                    // Animated Pixel Bees inside Honeycomb Cells
                    beePlacements.forEach { bp ->
                        val st = hiveModel.beeStates[bp.name] ?: SwarmHiveBeeState.IDLE
                        if (st != SwarmHiveBeeState.UNBORN) {
                            val dotCol: Color? = when (st) {
                                SwarmHiveBeeState.WAIT, SwarmHiveBeeState.NEEDS -> SwarmColors.warnAmber(isDark)
                                SwarmHiveBeeState.ISSUE -> SwarmColors.errorRed(isDark)
                                SwarmHiveBeeState.DONE -> SwarmColors.liveGreen(isDark)
                                else -> null
                            }
                            val boxW = bp.spriteW + 20f
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                modifier = Modifier
                                    .offset(x = (bp.cx - boxW / 2f).dp, y = (bp.cy - bp.spriteH / 2f).dp)
                                    .width(boxW.dp)
                                    .clickable {
                                        val cid = snap.members.firstOrNull { it.name == bp.name || (bp.isQueen && it.role == "coordinator") }?.conversationId.orEmpty()
                                        if (cid.isNotEmpty()) onSelectConversation(cid)
                                    }
                            ) {
                                SwarmPixelBeeSprite(
                                    name = bp.name,
                                    pose = if (st == SwarmHiveBeeState.WORKING) SwarmBeePose.WORKING else SwarmBeePose.LANDED,
                                    width = bp.spriteW.dp,
                                    height = bp.spriteH.dp,
                                    statusDotColor = dotCol,
                                    dotBorderColor = SwarmColors.surfaceBg(isDark),
                                    isTilted = st == SwarmHiveBeeState.WAIT,
                                    isRetired = st == SwarmHiveBeeState.RETIRED,
                                    bobbing = st == SwarmHiveBeeState.WORKING
                                )
                                if (bp.showName && !bp.isQueen) {
                                    Text(
                                        text = bp.name,
                                        fontSize = if (rDp >= 80f) 9.5.sp else 8.5.sp,
                                        color = SwarmColors.muted(isDark),
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // Zoom Pill
            if (maxZ > 1.15f && (clampedZoom > 1.01f || rFit < 68f)) {
                Row(
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(6.dp)
                        .clip(CircleShape)
                        .background(SwarmColors.surfaceBg(isDark).copy(alpha = 0.92f))
                        .border(0.8.dp, SwarmColors.border(isDark), CircleShape)
                        .padding(horizontal = 4.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    IconButton(onClick = { onZoomChange((clampedZoom / 1.35f).coerceAtLeast(1f)) }, enabled = clampedZoom > 1.01f, modifier = Modifier.size(24.dp)) {
                        Icon(Icons.Default.Remove, contentDescription = null, tint = SwarmColors.ink(isDark), modifier = Modifier.size(13.dp))
                    }
                    IconButton(onClick = { onZoomChange((clampedZoom * 1.35f).coerceAtMost(maxZ)) }, enabled = clampedZoom < maxZ - 0.02f, modifier = Modifier.size(24.dp)) {
                        Icon(Icons.Default.Add, contentDescription = null, tint = SwarmColors.ink(isDark), modifier = Modifier.size(13.dp))
                    }
                    if (clampedZoom > 1.01f) {
                        Text(
                            text = "Fit",
                            fontSize = 10.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = SwarmColors.ink(isDark),
                            modifier = Modifier.clickable { onZoomChange(1f) }.padding(horizontal = 5.dp)
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SelectedHiveCellCardAndroid(
    cell: SwarmHiveCell,
    snap: SwarmSnapshot,
    synth: SwarmSynthesizedModel,
    hiveModel: SwarmHiveMapModel,
    onClose: () -> Unit,
    onSelectConversation: (String) -> Unit
) {
    val isDark = AntigravityTheme.colors.isDark
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(SwarmColors.surfaceBg(isDark))
            .border(0.8.dp, SwarmColors.border(isDark), RoundedCornerShape(12.dp))
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(cell.title, color = SwarmColors.ink(isDark), fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                if (cell.goal.isNotBlank()) {
                    Text(cell.goal, color = SwarmColors.muted(isDark), fontSize = 12.5.sp)
                }
            }
            Text(
                text = if (cell.countOnly == null && cell.total > 0) "${cell.done} of ${cell.total} done" else "${cell.bees.size} ${if (cell.bees.size == 1) "bee" else "bees"}",
                color = SwarmColors.muted(isDark),
                fontSize = 11.5.sp,
                fontFamily = FontFamily.Monospace
            )
            IconButton(onClick = onClose, modifier = Modifier.size(24.dp).clip(CircleShape).background(SwarmColors.fieldBg(isDark))) {
                Icon(Icons.Default.Close, contentDescription = null, tint = SwarmColors.muted(isDark), modifier = Modifier.size(13.dp))
            }
        }

        if (cell.bees.isEmpty()) {
            Text("No one is on this thread yet.", color = SwarmColors.muted(isDark), fontSize = 12.5.sp)
        } else {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                cell.bees.forEach { name ->
                    val sm = synth.members[name]
                    val st = hiveModel.beeStates[name] ?: SwarmHiveBeeState.IDLE
                    val stLabel = when (st) {
                        SwarmHiveBeeState.WORKING -> "Working"
                        SwarmHiveBeeState.WAIT -> hiveModel.beeWaitingOn[name]?.let { "Waiting on @$it" } ?: "Waiting"
                        SwarmHiveBeeState.ISSUE -> "Stuck"
                        SwarmHiveBeeState.DONE -> "Done"
                        SwarmHiveBeeState.RETIRED -> "Retired"
                        SwarmHiveBeeState.UNBORN -> "Not started"
                        else -> "Resting"
                    }
                    val stCol = when (st) {
                        SwarmHiveBeeState.WORKING -> SwarmColors.accent(isDark)
                        SwarmHiveBeeState.WAIT, SwarmHiveBeeState.NEEDS -> SwarmColors.warnAmber(isDark)
                        SwarmHiveBeeState.ISSUE -> SwarmColors.errorRed(isDark)
                        SwarmHiveBeeState.DONE -> SwarmColors.liveGreen(isDark)
                        else -> SwarmColors.muted(isDark)
                    }
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .clickable {
                                val cid = snap.members.firstOrNull { it.name == name }?.conversationId.orEmpty()
                                if (cid.isNotEmpty()) onSelectConversation(cid)
                            }
                            .padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        SwarmPixelBee(name = name, pose = if (st == SwarmHiveBeeState.WORKING) SwarmBeePose.WORKING else SwarmBeePose.LANDED, size = 36.dp)
                        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text("@$name", color = SwarmColors.ink(isDark), fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                                if (!sm?.title.isNullOrBlank()) {
                                    Text(sm!!.title, color = SwarmColors.muted(isDark), fontSize = 11.5.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                            }
                            if (!sm?.now.isNullOrBlank() && st != SwarmHiveBeeState.UNBORN) {
                                Text(sm!!.now, color = SwarmColors.faint(isDark), fontSize = 11.5.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                        Text(stLabel, color = stCol, fontSize = 11.5.sp, fontWeight = FontWeight.Medium)
                    }
                }
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
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    val dotCol = when (status.lowercase()) {
                        "working" -> SwarmColors.liveGreen(isDark)
                        "error", "stuck" -> SwarmColors.errorRed(isDark)
                        else -> null
                    }
                    SwarmPixelBeeSprite(
                        name = node.id,
                        pose = if (status.lowercase() == "working") SwarmBeePose.WORKING else SwarmBeePose.LANDED,
                        width = 20.dp,
                        height = 16.5.dp,
                        statusDotColor = dotCol,
                        dotBorderColor = if (isHot || isSelected) SwarmColors.surfaceBg(isDark) else SwarmColors.cardBg(isDark)
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
                            text = "进入子会话".loc,
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
