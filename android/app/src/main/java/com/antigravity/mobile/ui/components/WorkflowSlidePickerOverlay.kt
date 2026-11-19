package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

@Stable
class WorkflowSlidePickerState {
    var isPresented by mutableStateOf(false)
    var isInteractiveTapMode by mutableStateOf(false)
    var anchorRect by mutableStateOf(Rect.Zero)
    var containerSize by mutableStateOf(IntSize.Zero)
    var quickTemplates by mutableStateOf<List<WorkflowTemplateRecord>>(emptyList())
    var hoveredIndex by mutableStateOf<Int?>(null)
    var hasEnteredMenu by mutableStateOf(false)

    object Layout {
        val defaultRowHeight = 34.dp
        val compactRowHeight = 30.dp
        val minRowHeight = 24.dp
        val defaultRowSpacing = 3.dp
        val compactRowSpacing = 2.dp
        val cardPadding = 5.dp
        val cardMinWidth = 208.dp
        val cardMaxWidth = 276.dp
        val gapBetweenButtonAndMenu = 8.dp
        val horizontalMargin = 12.dp
        val topSafeMargin = 12.dp
    }

    val isEmptyState: Boolean get() = quickTemplates.isEmpty()
    val rowCount: Int get() = max(quickTemplates.size, 1)

    fun open(allTemplates: List<WorkflowTemplateRecord>) {
        quickTemplates = allTemplates.filter { it.inQuickMenu }
        hoveredIndex = null
        hasEnteredMenu = false
        isInteractiveTapMode = false
        isPresented = true
    }

    fun dismiss() {
        isPresented = false
        isInteractiveTapMode = false
        hoveredIndex = null
        hasEnteredMenu = false
    }

    private fun availableHeightPx(density: Density): Float = with(density) {
        val topSafe = Layout.topSafeMargin.toPx()
        val btnGap = Layout.gapBetweenButtonAndMenu.toPx()
        if (anchorRect.top > topSafe + btnGap + 60.dp.toPx()) {
            return@with anchorRect.top - btnGap - topSafe
        }
        val containerH = containerSize.height.toFloat()
        if (containerH > 120.dp.toPx()) {
            return@with containerH - topSafe * 2f
        }
        420.dp.toPx()
    }

    fun rowSpacingDp(): Dp =
        if (rowCount > 7) Layout.compactRowSpacing else Layout.defaultRowSpacing

    fun rowHeightDp(density: Density): Dp = with(density) {
        if (isEmptyState) return@with 36.dp
        val count = rowCount
        val preferredPx = (if (count > 7) Layout.compactRowHeight else Layout.defaultRowHeight).toPx()
        val rowS = rowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val avail = availableHeightPx(density)
        val overhead = pad * 2f + max(0, count - 1) * rowS
        val maxRowPx = (avail - overhead) / count.toFloat()
        val chosenPx = min(preferredPx, max(Layout.minRowHeight.toPx(), floor(maxRowPx)))
        chosenPx.toDp()
    }

    fun contentNeedsScroll(density: Density): Boolean = with(density) {
        val count = rowCount
        val rowH = rowHeightDp(density).toPx()
        val rowS = rowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val naturalHeight = count * rowH + max(0, count - 1) * rowS + pad * 2f
        naturalHeight > availableHeightPx(density) + 1f
    }

    fun cardRectPx(density: Density): Rect = with(density) {
        val count = rowCount
        val rowH = rowHeightDp(density).toPx()
        val rowS = rowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val naturalHeight = count * rowH + max(0, count - 1) * rowS + pad * 2f
        val avail = availableHeightPx(density)
        val height = min(naturalHeight, avail)

        val containerW = containerSize.width.toFloat()
        val hMargin = Layout.horizontalMargin.toPx()
        val minW = if (isEmptyState) 236.dp.toPx() else Layout.cardMinWidth.toPx()
        val maxW = Layout.cardMaxWidth.toPx()
        val maxRoom = if (containerW > 0f) containerW - hMargin * 2f else maxW
        val upperW = min(maxW, max(minW, maxRoom))
        val preferredW = if (containerW > 0f) containerW * (if (isEmptyState) 0.68f else 0.56f) else minW
        val width = min(upperW, max(minW, preferredW))

        val maxLeftX = max(hMargin, containerW - width - hMargin)
        val minX = if (containerW > 0f) {
            min(max(hMargin, anchorRect.left), maxLeftX)
        } else {
            max(hMargin, anchorRect.left)
        }
        val topSafe = Layout.topSafeMargin.toPx()
        val btnGap = Layout.gapBetweenButtonAndMenu.toPx()
        val minY = max(topSafe, anchorRect.top - btnGap - height)
        Rect(minX, minY, minX + width, minY + height)
    }

    fun updateDrag(point: Offset, density: Density, haptic: HapticFeedback) {
        if (!isPresented) return
        val rect = cardRectPx(density)
        with(density) {
            val pad = Layout.cardPadding.toPx()
            val step = rowHeightDp(density).toPx() + rowSpacingDp().toPx()
            val hitRect = Rect(
                left = rect.left - 28.dp.toPx(),
                top = rect.top - 24.dp.toPx(),
                right = rect.right + 28.dp.toPx(),
                bottom = rect.bottom + 6.dp.toPx()
            )
            if (hitRect.contains(point)) {
                hasEnteredMenu = true
                val localY = point.y - (rect.top + pad)
                val idx = floor(localY / step).toInt().coerceIn(0, rowCount - 1)
                if (hoveredIndex != idx) {
                    hoveredIndex = idx
                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                }
                return
            }

            val anchorExpanded = anchorRect.inflate(16.dp.toPx())
            if (anchorExpanded.contains(point)) {
                clearHover()
                return
            }

            clearHover()
        }
    }

    fun endDrag(onSelectTemplate: (String) -> Unit, onOpenSetup: () -> Unit) {
        if (!isPresented) return
        val idx = hoveredIndex
        if (idx != null) {
            if (isEmptyState) {
                dismiss()
                onOpenSetup()
                return
            }
            quickTemplates.getOrNull(idx)?.let { chosen ->
                onSelectTemplate(chosen.templatePrompt)
                dismiss()
                return
            }
        }

        if (!hasEnteredMenu) {
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }

    private fun clearHover() {
        if (hoveredIndex != null) {
            hoveredIndex = null
        }
    }
}

@Composable
fun WorkflowSlidePickerOverlay(
    state: WorkflowSlidePickerState,
    onSelectTemplate: (String) -> Unit,
    onOpenSetup: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val haptic = LocalHapticFeedback.current

    AnimatedVisibility(
        visible = state.isPresented,
        enter = fadeIn(spring(stiffness = Spring.StiffnessMedium)),
        exit = fadeOut(spring(stiffness = Spring.StiffnessMedium)),
        modifier = modifier
            .fillMaxSize()
            .zIndex(500f)
    ) {
        val rect = state.cardRectPx(density)
        val rowHeightDp = state.rowHeightDp(density)
        val rowSpacingDp = state.rowSpacingDp()
        val needsScroll = state.contentNeedsScroll(density)
        val shape = RoundedCornerShape(12.dp)

        Box(modifier = Modifier.fillMaxSize()) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black.copy(alpha = if (state.isInteractiveTapMode) 0.08f else 0.03f))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null
                    ) { state.dismiss() }
            )

            Surface(
                modifier = Modifier
                    .offset { IntOffset(rect.left.roundToInt(), rect.top.roundToInt()) }
                    .animateEnterExit(
                        enter = scaleIn(
                            initialScale = 0.94f,
                            transformOrigin = TransformOrigin(0f, 1f)
                        )
                    )
                    .width(with(density) { rect.width.toDp() })
                    .heightIn(max = with(density) { rect.height.toDp() })
                    .border(1.dp, colors.border, shape),
                shape = shape,
                color = colors.surface.copy(alpha = 0.96f),
                shadowElevation = 10.dp,
                tonalElevation = 0.dp
            ) {
                val listModifier = if (needsScroll) {
                    Modifier.verticalScroll(rememberScrollState())
                } else {
                    Modifier
                }

                Column(
                    modifier = Modifier
                        .padding(WorkflowSlidePickerState.Layout.cardPadding)
                        .then(listModifier),
                    verticalArrangement = Arrangement.spacedBy(rowSpacingDp)
                ) {
                    if (state.isEmptyState) {
                        val isHovered = state.hoveredIndex == 0
                        WorkflowPickerNeutralRow(
                            isHovered = isHovered,
                            rowHeight = rowHeightDp,
                            textColor = colors.textPrimary,
                            onClick = {
                                haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                state.dismiss()
                                onOpenSetup()
                            }
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = if (isHovered) "暂未配置快捷模板 · 松手去设置".loc else "暂未配置快捷模板 · 点击去设置".loc,
                                    fontSize = 13.sp,
                                    fontWeight = if (isHovered) FontWeight.SemiBold else FontWeight.Medium,
                                    color = colors.textPrimary,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f)
                                )
                                Text(
                                    text = "›",
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.Medium,
                                    color = colors.textPrimary.copy(alpha = 0.65f)
                                )
                            }
                        }
                    } else {
                        state.quickTemplates.forEachIndexed { index, item ->
                            val isHovered = state.hoveredIndex == index
                            val iconColor = WorkflowStyleCatalog.resolveColor(item.color, colors)
                            val badgeSize = if (rowHeightDp < 29.dp) 18.dp else 22.dp
                            val iconSize = if (rowHeightDp < 29.dp) 11.dp else 13.dp
                            WorkflowPickerNeutralRow(
                                isHovered = isHovered,
                                rowHeight = rowHeightDp,
                                textColor = colors.textPrimary,
                                onClick = {
                                    haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                    onSelectTemplate(item.templatePrompt)
                                    state.dismiss()
                                }
                            ) {
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Box(
                                        modifier = Modifier
                                            .size(badgeSize)
                                            .clip(RoundedCornerShape(6.dp))
                                            .background(iconColor.copy(alpha = 0.15f)),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        WorkflowVectorIcon(
                                            iconKey = item.icon,
                                            tint = iconColor,
                                            size = iconSize
                                        )
                                    }
                                    Text(
                                        text = item.title,
                                        fontSize = if (rowHeightDp < 29.dp) 12.5.sp else 13.5.sp,
                                        fontWeight = if (isHovered) FontWeight.SemiBold else FontWeight.Medium,
                                        color = colors.textPrimary,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f)
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun WorkflowPickerNeutralRow(
    isHovered: Boolean,
    rowHeight: Dp,
    textColor: Color,
    onClick: () -> Unit,
    content: @Composable () -> Unit
) {
    val bgColor = if (isHovered) textColor.copy(alpha = 0.12f) else Color.Transparent
    val borderColor = if (isHovered) textColor.copy(alpha = 0.32f) else Color.Transparent
    val shape = RoundedCornerShape(8.dp)

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(rowHeight)
            .clip(shape)
            .background(bgColor)
            .then(
                if (borderColor != Color.Transparent) {
                    Modifier.border(1.25.dp, borderColor, shape)
                } else {
                    Modifier
                }
            )
            .clickable(onClick = onClick)
            .padding(horizontal = 10.dp),
        contentAlignment = Alignment.CenterStart
    ) {
        content()
    }
}
