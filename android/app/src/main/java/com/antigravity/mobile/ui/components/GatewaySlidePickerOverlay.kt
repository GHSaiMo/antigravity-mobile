package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DesktopWindows
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.LaptopMac
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.data.model.GatewayProfile
import com.antigravity.mobile.data.model.GatewaySummarySnapshot
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import kotlin.math.abs
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

@Stable
class GatewaySlidePickerState {
    var isPresented by mutableStateOf(false)
    var isInteractiveTapMode by mutableStateOf(false)
    var anchorRect by mutableStateOf(Rect.Zero)
    var containerSize by mutableStateOf(IntSize.Zero)
    var preferCenteredAnchor by mutableStateOf(false)
    /** 0..<profiles.size = host row, profiles.size = "+ 添加主机" row */
    var hoveredRowIndex by mutableStateOf<Int?>(null)
    var hasEnteredMenu by mutableStateOf(false)

    object Layout {
        val hostRowHeight = 48.dp
        val addRowHeight = 44.dp
        val rowSpacing = 4.dp
        val dividerBlockHeight = 9.dp
        val cardPadding = 6.dp
        val cardMinWidth = 236.dp
        val cardMaxWidth = 268.dp
        val horizontalMargin = 14.dp
        val gapBelowTitle = 8.dp
        val bottomSafeMargin = 28.dp
    }

    fun isCenteredAnchor(density: Density): Boolean = with(density) {
        if (preferCenteredAnchor) return@with true
        val containerW = containerSize.width.toFloat()
        containerW > 0f
            && abs(anchorRect.center.x - containerW / 2f) < 48.dp.toPx()
            && anchorRect.left > 44.dp.toPx()
    }

    fun open(anchor: Rect, centered: Boolean = true) {
        anchorRect = anchor
        preferCenteredAnchor = centered
        hoveredRowIndex = null
        hasEnteredMenu = false
        isInteractiveTapMode = false
        isPresented = true
    }

    fun openInteractive(anchor: Rect, centered: Boolean = true) {
        anchorRect = anchor
        preferCenteredAnchor = centered
        hoveredRowIndex = null
        hasEnteredMenu = false
        isInteractiveTapMode = true
        isPresented = true
    }

    fun dismiss() {
        isPresented = false
        isInteractiveTapMode = false
        hoveredRowIndex = null
        hasEnteredMenu = false
    }

    fun cardRectPx(density: Density, profileCount: Int): Rect = with(density) {
        val containerW = containerSize.width.toFloat()
        val containerH = containerSize.height.toFloat()
        val hMargin = Layout.horizontalMargin.toPx()
        val availableW = if (containerW > 0f) max(180f, containerW - hMargin * 2f) else Layout.cardMaxWidth.toPx()
        val width = min(Layout.cardMaxWidth.toPx(), max(min(Layout.cardMinWidth.toPx(), availableW), min(availableW, containerW * 0.70f)))

        val hostCount = max(0, profileCount)
        val hostRowPx = Layout.hostRowHeight.toPx()
        val addRowPx = Layout.addRowHeight.toPx()
        val rowSpacePx = Layout.rowSpacing.toPx()
        val dividerPx = if (hostCount > 0) Layout.dividerBlockHeight.toPx() else 0f
        val padPx = Layout.cardPadding.toPx()
        val hostsHeight = hostCount * hostRowPx + max(0, hostCount - 1) * rowSpacePx
        val naturalHeight = padPx * 2f + hostsHeight + dividerPx + addRowPx

        val gapBelowPx = Layout.gapBelowTitle.toPx()
        val bottomMarginPx = Layout.bottomSafeMargin.toPx()
        val preferredY = max(56.dp.toPx(), anchorRect.bottom + gapBelowPx)
        val maxAvailH = if (containerH > 160.dp.toPx()) max(160.dp.toPx(), containerH - preferredY - bottomMarginPx) else 440.dp.toPx()
        val height = min(naturalHeight, maxAvailH)

        val rawMinX = if (containerW > 0f) {
            (containerW - width) / 2f
        } else {
            anchorRect.center.x - width / 2f
        }
        val maxMinX = if (containerW > 0f) max(hMargin, containerW - width - hMargin) else rawMinX
        val minX = min(max(hMargin, rawMinX), maxMinX)

        val maxMinY = if (containerH > 0f) max(56.dp.toPx(), containerH - height - bottomMarginPx) else preferredY
        val minY = min(preferredY, maxMinY)

        Rect(left = minX, top = minY, right = minX + width, bottom = minY + height)
    }

    fun updateDrag(
        pointInRoot: Offset,
        density: Density,
        profileCount: Int,
        haptic: HapticFeedback
    ) {
        if (!isPresented) return
        val rect = cardRectPx(density, profileCount)
        with(density) {
            // Match rows purely by vertical height even when the finger slides outside the card
            // horizontally, so one-handed slide-down never obscures host names.
            val verticalTop = max(anchorRect.bottom + 4.dp.toPx(), rect.top - 2.dp.toPx())
            val verticalBottom = rect.bottom + 20.dp.toPx()

            if (pointInRoot.y < verticalTop || pointInRoot.y > verticalBottom) {
                hoveredRowIndex = null
                return
            }

            hasEnteredMenu = true
            val padPx = Layout.cardPadding.toPx()
            val hostRowPx = Layout.hostRowHeight.toPx()
            val rowSpacePx = Layout.rowSpacing.toPx()
            val dividerPx = Layout.dividerBlockHeight.toPx()
            val localY = pointInRoot.y - (rect.top + padPx)
            val hostCount = max(0, profileCount)
            val hostsHeight = hostCount * hostRowPx + max(0, hostCount - 1) * rowSpacePx

            val targetIdx = when {
                hostCount == 0 -> 0
                localY < hostsHeight + dividerPx * 0.45f -> {
                    val step = hostRowPx + rowSpacePx
                    floor(max(0f, localY) / step).toInt().coerceIn(0, hostCount - 1)
                }
                else -> hostCount
            }

            if (hoveredRowIndex != targetIdx) {
                hoveredRowIndex = targetIdx
                haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
            }
        }
    }

    fun endDrag(
        profiles: List<GatewayProfile>,
        haptic: HapticFeedback,
        onSelectProfile: (GatewayProfile) -> Unit,
        onAddGateway: () -> Unit
    ) {
        if (!isPresented) return
        val idx = hoveredRowIndex
        if (idx != null) {
            haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
            if (idx == profiles.size) {
                dismiss()
                onAddGateway()
                return
            } else if (idx in profiles.indices) {
                val chosen = profiles[idx]
                dismiss()
                onSelectProfile(chosen)
                return
            }
        }
        if (!hasEnteredMenu) {
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }
}

/**
 * Attaches the mirrored Hero long-press (220ms) + slide-down gesture and optional horizontal swipe
 * quick-cycling to a title composable while preserving short taps and normal vertical scrolling.
 */
@Composable
fun Modifier.heroGatewayLongPressGesture(
    enabled: Boolean,
    pickerState: GatewaySlidePickerState,
    anchorRectProvider: () -> Rect,
    profileCountProvider: () -> Int,
    profilesProvider: () -> List<GatewayProfile>,
    density: Density,
    haptic: HapticFeedback,
    centeredAnchor: Boolean = false,
    canHorizontalSwipe: () -> Boolean = { false },
    onHorizontalDragOffsetChanged: ((Float) -> Unit)? = null,
    onHorizontalSwipe: ((Int) -> Unit)? = null,
    onShortTap: () -> Unit,
    onLongPressOpened: () -> Unit,
    onSelectProfile: (GatewayProfile) -> Unit,
    onAddGateway: () -> Unit
): Modifier {
    val latestAnchorRectProvider by rememberUpdatedState(anchorRectProvider)
    val latestProfileCountProvider by rememberUpdatedState(profileCountProvider)
    val latestProfilesProvider by rememberUpdatedState(profilesProvider)
    val latestCenteredAnchor by rememberUpdatedState(centeredAnchor)
    val latestCanHorizontalSwipe by rememberUpdatedState(canHorizontalSwipe)
    val latestOnDragOffset by rememberUpdatedState(onHorizontalDragOffsetChanged)
    val latestOnHorizontalSwipe by rememberUpdatedState(onHorizontalSwipe)
    val latestOnShortTap by rememberUpdatedState(onShortTap)
    val latestOnLongPressOpened by rememberUpdatedState(onLongPressOpened)
    val latestOnSelectProfile by rememberUpdatedState(onSelectProfile)
    val latestOnAddGateway by rememberUpdatedState(onAddGateway)

    return this.pointerInput(enabled) {
        if (!enabled) return@pointerInput
        val slopPx = with(density) { 12.dp.toPx() }
        val swipeTriggerPx = with(density) { 36.dp.toPx() }
        val quickFlickPx = with(density) { 20.dp.toPx() }
        val maxDampedPx = with(density) { 14.dp.toPx() }
        awaitEachGesture {
            val down = awaitFirstDown(requireUnconsumed = false)
            val downPos = down.position
            val downTimeMs = down.uptimeMillis
            var movedBeyondSlop = false
            var startedHorizontalSwipe = false
            var firstHorizDx = 0f
            var releasedEarly = false

            val holdOutcome = withTimeoutOrNull(220L) {
                while (true) {
                    val event = awaitPointerEvent(PointerEventPass.Main)
                    val change = event.changes.firstOrNull { it.id == down.id } ?: break
                    if (!change.pressed) {
                        releasedEarly = true
                        break
                    }
                    val delta = change.position - downPos
                    if (delta.getDistance() > slopPx) {
                        movedBeyondSlop = true
                        if (abs(delta.x) > abs(delta.y) * 1.35f && latestCanHorizontalSwipe() && !pickerState.isPresented) {
                            startedHorizontalSwipe = true
                            firstHorizDx = delta.x
                            change.consume()
                        }
                        break
                    }
                }
            }

            if (startedHorizontalSwipe) {
                var triggered = false
                var lastDx = firstHorizDx
                latestOnDragOffset?.invoke((firstHorizDx * 0.35f).coerceIn(-maxDampedPx, maxDampedPx))
                if (abs(firstHorizDx) >= swipeTriggerPx) {
                    triggered = true
                    haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                    latestOnHorizontalSwipe?.invoke(if (firstHorizDx < 0f) 1 else -1)
                }
                try {
                    while (true) {
                        val event = awaitPointerEvent(PointerEventPass.Main)
                        val change = event.changes.firstOrNull { it.id == down.id } ?: break
                        val dx = change.position.x - downPos.x
                        lastDx = dx
                        if (!change.pressed) {
                            change.consume()
                            val elapsed = change.uptimeMillis - downTimeMs
                            if (!triggered && abs(dx) >= quickFlickPx && elapsed <= 260L) {
                                haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                latestOnHorizontalSwipe?.invoke(if (dx < 0f) 1 else -1)
                            }
                            break
                        }
                        change.consume()
                        latestOnDragOffset?.invoke((dx * 0.35f).coerceIn(-maxDampedPx, maxDampedPx))
                        if (!triggered && abs(lastDx) >= swipeTriggerPx) {
                            triggered = true
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            latestOnHorizontalSwipe?.invoke(if (lastDx < 0f) 1 else -1)
                        }
                    }
                } finally {
                    latestOnDragOffset?.invoke(0f)
                }
                return@awaitEachGesture
            }

            if (releasedEarly) {
                if (!movedBeyondSlop) {
                    latestOnShortTap()
                }
                return@awaitEachGesture
            }
            if (movedBeyondSlop || holdOutcome != null) {
                return@awaitEachGesture
            }

            // Long press triggered after 220ms hold!
            val anchor = latestAnchorRectProvider()
            if (anchor == Rect.Zero) return@awaitEachGesture
            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
            pickerState.open(anchor, centered = latestCenteredAnchor)
            latestOnLongPressOpened()

            // Track continuous slide-down movement until release or cancellation.
            while (true) {
                val event = awaitPointerEvent(PointerEventPass.Main)
                val change = event.changes.firstOrNull { it.id == down.id } ?: break
                val pointInRoot = Offset(
                    x = anchor.left + change.position.x,
                    y = anchor.top + change.position.y
                )
                if (!change.pressed) {
                    change.consume()
                    pickerState.endDrag(
                        profiles = latestProfilesProvider(),
                        haptic = haptic,
                        onSelectProfile = latestOnSelectProfile,
                        onAddGateway = latestOnAddGateway
                    )
                    break
                }
                change.consume()
                pickerState.updateDrag(
                    pointInRoot = pointInRoot,
                    density = density,
                    profileCount = latestProfileCountProvider(),
                    haptic = haptic
                )
            }
        }
    }
}

@Composable
fun GatewaySlidePickerOverlay(
    state: GatewaySlidePickerState,
    profiles: List<GatewayProfile>,
    activeProfileId: String?,
    summaries: Map<String, GatewaySummarySnapshot>,
    onSelectProfile: (GatewayProfile) -> Unit,
    onAddGateway: () -> Unit,
    onRenameProfile: (GatewayProfile) -> Unit,
    onRemoveProfile: (GatewayProfile) -> Unit
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val haptic = LocalHapticFeedback.current

    AnimatedVisibility(
        visible = state.isPresented,
        enter = fadeIn(animationSpec = spring(stiffness = Spring.StiffnessMedium)) + scaleIn(
            initialScale = 0.94f,
            transformOrigin = TransformOrigin(0.5f, 0f),
            animationSpec = spring(stiffness = Spring.StiffnessMedium)
        ),
        exit = fadeOut(animationSpec = spring(stiffness = Spring.StiffnessHigh)),
        modifier = Modifier
            .fillMaxSize()
            .zIndex(550f)
    ) {
        val rect = state.cardRectPx(density, profiles.size)
        val cardWidthDp = with(density) { rect.width.toDp() }
        val cardHeightDp = with(density) { rect.height.toDp() }

        Box(modifier = Modifier.fillMaxSize()) {
            // Subtle dim scrim to dismiss on outside tap
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black.copy(alpha = if (state.isInteractiveTapMode) 0.14f else 0.05f))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        onClick = { state.dismiss() }
                    )
            )

            Surface(
                modifier = Modifier
                    .offset { IntOffset(rect.left.roundToInt(), rect.top.roundToInt()) }
                    .width(cardWidthDp)
                    .height(cardHeightDp),
                shape = RoundedCornerShape(14.dp),
                color = colors.surface.copy(alpha = 0.96f),
                shadowElevation = 14.dp,
                tonalElevation = 3.dp
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .border(1.dp, colors.border.copy(alpha = 0.85f), RoundedCornerShape(14.dp))
                ) {
                    val scrollState = rememberScrollState()
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(scrollState)
                            .padding(GatewaySlidePickerState.Layout.cardPadding)
                    ) {
                        profiles.forEachIndexed { index, profile ->
                            if (index > 0) {
                                Spacer(modifier = Modifier.height(GatewaySlidePickerState.Layout.rowSpacing))
                            }
                            GatewayHostRow(
                                profile = profile,
                                isActive = profile.id == activeProfileId,
                                isHovered = state.hoveredRowIndex == index,
                                isInteractiveTapMode = state.isInteractiveTapMode,
                                summary = summaries[profile.id],
                                onSelect = {
                                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                    state.dismiss()
                                    onSelectProfile(profile)
                                },
                                onRename = {
                                    state.dismiss()
                                    onRenameProfile(profile)
                                },
                                onRemove = {
                                    state.dismiss()
                                    onRemoveProfile(profile)
                                }
                            )
                        }

                        if (profiles.isNotEmpty()) {
                            HorizontalDivider(
                                modifier = Modifier.padding(horizontal = 6.dp, vertical = 4.dp),
                                thickness = 0.75.dp,
                                color = colors.border.copy(alpha = 0.65f)
                            )
                        }

                        AddGatewayPickerRow(
                            isHovered = state.hoveredRowIndex == profiles.size,
                            isInteractiveTapMode = state.isInteractiveTapMode,
                            onClick = {
                                haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                state.dismiss()
                                onAddGateway()
                            }
                        )
                    }
                }
            }
        }
    }
}

private fun platformIcon(platform: String?): ImageVector = when (platform?.lowercase()) {
    "windows", "win" -> Icons.Default.DesktopWindows
    "linux" -> Icons.Default.Dns
    "darwin", "macos", "mac" -> Icons.Default.LaptopMac
    else -> Icons.Default.Computer
}

@Composable
private fun GatewayHostRow(
    profile: GatewayProfile,
    isActive: Boolean,
    isHovered: Boolean,
    isInteractiveTapMode: Boolean,
    summary: GatewaySummarySnapshot?,
    onSelect: () -> Unit,
    onRename: () -> Unit,
    onRemove: () -> Unit
) {
    val colors = AntigravityTheme.colors
    var menuExpanded by remember { mutableStateOf(false) }

    val rowBg = when {
        isHovered -> colors.accentIndigo.copy(alpha = 0.22f)
        isActive -> colors.accentIndigo.copy(alpha = 0.11f)
        else -> Color.Transparent
    }
    val rowBorder = when {
        isHovered -> colors.accentIndigo.copy(alpha = 0.78f)
        isActive -> colors.accentIndigo.copy(alpha = 0.36f)
        else -> Color.Transparent
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(GatewaySlidePickerState.Layout.hostRowHeight)
            .clip(RoundedCornerShape(9.dp))
            .background(rowBg)
            .border(if (isHovered) 1.25.dp else 1.dp, rowBorder, RoundedCornerShape(9.dp))
            .clickable(enabled = isInteractiveTapMode, onClick = onSelect)
            .padding(start = 9.dp, end = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Box(
            modifier = Modifier
                .size(28.dp)
                .clip(RoundedCornerShape(7.dp))
                .background(
                    if (isActive || isHovered) colors.accentIndigo.copy(alpha = 0.16f)
                    else colors.textPrimary.copy(alpha = 0.06f)
                ),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = platformIcon(profile.platform),
                contentDescription = null,
                tint = if (isActive || isHovered) colors.accentIndigo else colors.textSecondary,
                modifier = Modifier.size(16.dp)
            )
        }

        Row(
            modifier = Modifier.weight(1f),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = profile.resolvedDisplayName,
                fontSize = 13.5.sp,
                fontWeight = if (isActive || isHovered) FontWeight.SemiBold else FontWeight.Medium,
                color = colors.textPrimary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )
            if (isActive) {
                Icon(
                    imageVector = Icons.Default.Check,
                    contentDescription = null,
                    tint = colors.accentIndigo,
                    modifier = Modifier.size(14.dp)
                )
            }
        }

        GatewayStatusPills(summary = summary, isActive = isActive)

        Box {
            Box(
                modifier = Modifier
                    .size(26.dp)
                    .clip(CircleShape)
                    .clickable(enabled = isInteractiveTapMode) { menuExpanded = true },
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = Icons.Default.MoreVert,
                    contentDescription = "更多".loc,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(16.dp)
                )
            }
            DropdownMenu(
                expanded = menuExpanded,
                onDismissRequest = { menuExpanded = false }
            ) {
                DropdownMenuItem(
                    text = { Text("重命名主机".loc) },
                    leadingIcon = { Icon(Icons.Default.Edit, contentDescription = null, modifier = Modifier.size(16.dp)) },
                    onClick = {
                        menuExpanded = false
                        onRename()
                    }
                )
                DropdownMenuItem(
                    text = { Text("移除此主机".loc, color = colors.accentRed) },
                    leadingIcon = {
                        Icon(
                            Icons.Default.Delete,
                            contentDescription = null,
                            tint = colors.accentRed,
                            modifier = Modifier.size(16.dp)
                        )
                    },
                    onClick = {
                        menuExpanded = false
                        onRemove()
                    }
                )
            }
        }
    }
}

@Composable
private fun GatewayStatusPills(
    summary: GatewaySummarySnapshot?,
    isActive: Boolean
) {
    val colors = AntigravityTheme.colors
    if (summary == null) return

    when {
        !summary.isReachable && !isActive -> {
            Text(
                text = "离线".loc,
                fontSize = 10.5.sp,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary,
                modifier = Modifier
                    .clip(RoundedCornerShape(999.dp))
                    .background(colors.textPrimary.copy(alpha = 0.07f))
                    .padding(horizontal = 6.dp, vertical = 2.dp)
            )
        }
        summary.isRefreshing && summary.updatedAtMs == 0L && !isActive -> {
            CircularProgressIndicator(
                modifier = Modifier.size(12.dp),
                strokeWidth = 1.6.dp,
                color = colors.textSecondary
            )
        }
        summary.actionCount > 0 || summary.runningCount > 0 || summary.unreadCount > 0 -> {
            Row(
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (summary.actionCount > 0) {
                    CompactStatusPill(
                        text = "%d 待确认".loc(summary.actionCount),
                        tint = colors.accentOrange
                    )
                }
                if (summary.runningCount > 0) {
                    CompactStatusPill(
                        text = "%d 工作中".loc(summary.runningCount),
                        tint = Color(0xFF3B82F6)
                    )
                }
                if (summary.unreadCount > 0 && summary.actionCount == 0) {
                    CompactStatusPill(
                        text = "%d 未读".loc(summary.unreadCount),
                        tint = colors.accentIndigo
                    )
                }
            }
        }
        summary.updatedAtMs > 0L || isActive -> {
            Text(
                text = "空闲".loc,
                fontSize = 10.5.sp,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary.copy(alpha = 0.78f)
            )
        }
    }
}

@Composable
private fun CompactStatusPill(text: String, tint: Color) {
    Text(
        text = text,
        fontSize = 10.5.sp,
        fontWeight = FontWeight.SemiBold,
        color = tint,
        maxLines = 1,
        modifier = Modifier
            .clip(RoundedCornerShape(999.dp))
            .background(tint.copy(alpha = 0.14f))
            .padding(horizontal = 6.dp, vertical = 2.dp)
    )
}

internal fun isCameraUnavailableOrRestricted(context: android.content.Context): Boolean {
    val hasCamera = runCatching {
        context.packageManager.hasSystemFeature(android.content.pm.PackageManager.FEATURE_CAMERA_ANY)
    }.getOrDefault(true)
    if (!hasCamera) return true
    val disabledByPolicy = runCatching {
        val dpm = context.getSystemService(android.content.Context.DEVICE_POLICY_SERVICE) as? android.app.admin.DevicePolicyManager
        dpm?.getCameraDisabled(null) == true
    }.getOrDefault(false)
    return disabledByPolicy
}

@Composable
private fun AddGatewayPickerRow(
    isHovered: Boolean,
    isInteractiveTapMode: Boolean,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val context = androidx.compose.ui.platform.LocalContext.current
    val cameraUnavailable = remember(context) { isCameraUnavailableOrRestricted(context) }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(GatewaySlidePickerState.Layout.addRowHeight)
            .clip(RoundedCornerShape(9.dp))
            .background(if (isHovered) colors.accentIndigo.copy(alpha = 0.18f) else Color.Transparent)
            .border(
                width = 1.2.dp,
                color = if (isHovered) colors.accentIndigo.copy(alpha = 0.75f) else Color.Transparent,
                shape = RoundedCornerShape(9.dp)
            )
            .clickable(enabled = isInteractiveTapMode, onClick = onClick)
            .padding(horizontal = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Icon(
            imageVector = Icons.Default.AddCircle,
            contentDescription = null,
            tint = colors.accentIndigo,
            modifier = Modifier.size(20.dp)
        )
        Text(
            text = "添加主机".loc,
            fontSize = 13.5.sp,
            fontWeight = if (isHovered) FontWeight.SemiBold else FontWeight.Medium,
            color = colors.accentIndigo,
            modifier = Modifier.weight(1f)
        )
        Text(
            text = if (cameraUnavailable) "链接配对".loc else "扫码配对".loc,
            fontSize = 11.sp,
            fontWeight = FontWeight.Medium,
            color = colors.textSecondary
        )
    }
}
