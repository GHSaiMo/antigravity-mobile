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
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

@Stable
class CommitSlidePickerState {
    var isPresented by mutableStateOf(false)
    var isInteractiveTapMode by mutableStateOf(false)
    var anchorRect by mutableStateOf(Rect.Zero)
    var containerSize by mutableStateOf(IntSize.Zero)
    var hoveredIndex by mutableStateOf<Int?>(null)
    var hasEnteredMenu by mutableStateOf(false)

    val options = listOf("Commit and Push", "Commit Only")

    object Layout {
        val rowHeight = 34.dp
        val rowSpacing = 3.dp
        val cardPadding = 5.dp
        val cardWidth = 172.dp
        val gapBetweenButtonAndMenu = 8.dp
        val horizontalMargin = 12.dp
        val topSafeMargin = 12.dp
    }

    val rowCount: Int get() = options.size

    fun open() {
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

    fun cardRectPx(density: Density): Rect = with(density) {
        val count = rowCount
        val rowH = Layout.rowHeight.toPx()
        val rowS = Layout.rowSpacing.toPx()
        val pad = Layout.cardPadding.toPx()
        val height = count * rowH + (count - 1) * rowS + pad * 2f

        val containerW = containerSize.width.toFloat()
        val hMargin = Layout.horizontalMargin.toPx()
        val width = Layout.cardWidth.toPx()

        val maxLeftX = if (containerW > 0f) max(hMargin, containerW - width - hMargin) else hMargin
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
            val step = Layout.rowHeight.toPx() + Layout.rowSpacing.toPx()
            val hitRect = Rect(
                left = rect.left - 28.dp.toPx(),
                top = rect.top - 24.dp.toPx(),
                right = rect.right + 28.dp.toPx(),
                bottom = rect.bottom + 10.dp.toPx()
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

    fun endDrag(onSelect: (String) -> Unit) {
        if (!isPresented) return
        val idx = hoveredIndex
        if (idx != null) {
            val chosen = options.getOrNull(idx)
            dismiss()
            if (chosen != null) {
                onSelect(chosen)
            }
            return
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
fun CommitSlidePickerOverlay(
    state: CommitSlidePickerState,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current

    AnimatedVisibility(
        visible = state.isPresented,
        enter = fadeIn(spring(stiffness = Spring.StiffnessMedium)),
        exit = fadeOut(spring(stiffness = Spring.StiffnessMedium)),
        modifier = modifier
            .fillMaxSize()
            .zIndex(500f)
    ) {
        val rect = state.cardRectPx(density)
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
                    .height(with(density) { rect.height.toDp() })
                    .border(1.dp, colors.border, shape),
                shape = shape,
                color = colors.surface.copy(alpha = 0.96f),
                shadowElevation = 10.dp,
                tonalElevation = 0.dp
            ) {
                Column(
                    modifier = Modifier.padding(CommitSlidePickerState.Layout.cardPadding),
                    verticalArrangement = Arrangement.spacedBy(CommitSlidePickerState.Layout.rowSpacing)
                ) {
                    state.options.forEachIndexed { index, title ->
                        val isHovered = state.hoveredIndex == index
                        val rowShape = RoundedCornerShape(8.dp)
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(CommitSlidePickerState.Layout.rowHeight)
                                .clip(rowShape)
                                .background(if (isHovered) colors.textPrimary.copy(alpha = 0.12f) else Color.Transparent)
                                .border(
                                    if (isHovered) 1.dp else 0.dp,
                                    if (isHovered) colors.textPrimary.copy(alpha = 0.25f) else Color.Transparent,
                                    rowShape
                                )
                                .clickable {
                                    state.dismiss()
                                    onSelect(title)
                                }
                                .padding(horizontal = 12.dp),
                            contentAlignment = Alignment.CenterStart
                        ) {
                            Text(
                                text = title,
                                fontSize = 13.5.sp,
                                fontWeight = if (isHovered) FontWeight.SemiBold else FontWeight.Medium,
                                color = colors.textPrimary
                            )
                        }
                    }
                }
            }
        }
    }
}
