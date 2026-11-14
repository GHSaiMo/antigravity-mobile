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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Icon
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
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.model.ModelVariant
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

data class ModelPickerRow(
    val id: String,
    val title: String,
    val variantsTopToBottom: List<ModelVariant>,
    val defaultVariantIndex: Int,
    val hasSubmenu: Boolean,
    val usesEffortLabels: Boolean
)

@Stable
class ModelSlidePickerState {
    var isPresented by mutableStateOf(false)
    var isInteractiveTapMode by mutableStateOf(false)
    var anchorRect by mutableStateOf(Rect.Zero)
    var containerSize by mutableStateOf(IntSize.Zero)
    var providerKind by mutableStateOf("gemini")
    var rows by mutableStateOf<List<ModelPickerRow>>(emptyList())
    var hoveredRowIndex by mutableStateOf<Int?>(null)
    var hoveredVariantIndex by mutableStateOf<Int?>(null)
    var isInRightColumn by mutableStateOf(false)
    var hasEnteredMenu by mutableStateOf(false)
    var activeModelId by mutableStateOf("")

    object Layout {
        val rowHeight = 36.dp
        val rowSpacing = 4.dp
        val cardPadding = 5.dp
        val leftCardMinWidth = 176.dp
        val leftCardMaxWidth = 220.dp
        val rightCardWidth = 112.dp
        val gapBetweenButtonAndMenu = 8.dp
        val gapBetweenColumns = 8.dp
        val horizontalMargin = 12.dp
        val topSafeMargin = 12.dp
        val columnLockHysteresis = 6.dp
    }

    private fun displayMenuTitle(rawTitle: String): String {
        val trimmed = rawTitle.trim()
        for (prefix in listOf("Gemini ", "Claude ")) {
            if (trimmed.length > prefix.length && trimmed.startsWith(prefix, ignoreCase = true)) {
                return trimmed.substring(prefix.length)
            }
        }
        return trimmed
    }

    fun open(provider: String, activeModel: String, options: List<ModelOption>) {
        val families = ModelDefaults.modelFamilies(options, provider = provider)
        val builtRows: List<ModelPickerRow> = if (
            families.size == 1 &&
            !families.first().hasEffortVariants &&
            families.first().variants.size > 1
        ) {
            // Single non-effort family (e.g. Orchestrator Carbon / Argon): show options directly in upward menu
            families.first().variants.map { variant ->
                ModelPickerRow(
                    id = variant.id,
                    title = displayMenuTitle(variant.rawName),
                    variantsTopToBottom = listOf(variant),
                    defaultVariantIndex = 0,
                    hasSubmenu = false,
                    usesEffortLabels = false
                )
            }
        } else {
            families.map { family ->
                val ordered = family.variantsTopToBottom
                ModelPickerRow(
                    id = family.id,
                    title = displayMenuTitle(family.name),
                    variantsTopToBottom = ordered,
                    defaultVariantIndex = family.defaultVariantIndexTopToBottom,
                    hasSubmenu = ordered.size > 1,
                    usesEffortLabels = family.hasEffortVariants
                )
            }
        }
        if (builtRows.isEmpty()) return

        providerKind = provider
        activeModelId = activeModel
        rows = builtRows
        hoveredRowIndex = null
        hoveredVariantIndex = null
        isInRightColumn = false
        hasEnteredMenu = false
        isInteractiveTapMode = false
        isPresented = true
    }

    fun dismiss() {
        isPresented = false
        isInteractiveTapMode = false
        hoveredRowIndex = null
        hoveredVariantIndex = null
        isInRightColumn = false
        hasEnteredMenu = false
    }

    fun leftCardRectPx(density: Density): Rect = with(density) {
        val count = max(rows.size, 1)
        val rowH = Layout.rowHeight.toPx()
        val rowS = Layout.rowSpacing.toPx()
        val pad = Layout.cardPadding.toPx()
        val height = count * rowH + max(0, count - 1) * rowS + pad * 2f

        val containerW = containerSize.width.toFloat()
        val preferredW = if (containerW > 0f) containerW * 0.52f else Layout.leftCardMinWidth.toPx()
        val width = min(Layout.leftCardMaxWidth.toPx(), max(Layout.leftCardMinWidth.toPx(), preferredW))

        val hMargin = Layout.horizontalMargin.toPx()
        val rightW = Layout.rightCardWidth.toPx()
        val colGap = Layout.gapBetweenColumns.toPx()
        val maxLeftX = max(hMargin, containerW - width - rightW - colGap - hMargin)
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

    private fun leftRowCenterYPx(index: Int, leftRect: Rect, density: Density): Float = with(density) {
        val pad = Layout.cardPadding.toPx()
        val rowH = Layout.rowHeight.toPx()
        val rowS = Layout.rowSpacing.toPx()
        leftRect.top + pad + index * (rowH + rowS) + rowH / 2f
    }

    fun rightCardRectPx(rowIndex: Int, leftRect: Rect, density: Density): Rect? = with(density) {
        val row = rows.getOrNull(rowIndex) ?: return@with null
        if (!row.hasSubmenu) return@with null
        val count = row.variantsTopToBottom.size
        if (count <= 1) return@with null

        val pad = Layout.cardPadding.toPx()
        val rowH = Layout.rowHeight.toPx()
        val rowS = Layout.rowSpacing.toPx()
        val height = count * rowH + max(0, count - 1) * rowS + pad * 2f
        val anchorIdx = row.defaultVariantIndex.coerceIn(0, count - 1)
        val anchorOffset = pad + anchorIdx * (rowH + rowS) + rowH / 2f
        val rawMinY = leftRowCenterYPx(rowIndex, leftRect, density) - anchorOffset
        val topSafe = Layout.topSafeMargin.toPx()
        val containerH = containerSize.height.toFloat()
        val maxMinY = if (containerH > 0f) max(topSafe, containerH - height - 6.dp.toPx()) else rawMinY
        val minY = min(max(topSafe, rawMinY), maxMinY)
        val minX = leftRect.right + Layout.gapBetweenColumns.toPx()
        val width = Layout.rightCardWidth.toPx()
        Rect(minX, minY, minX + width, minY + height)
    }

    fun updateDrag(point: Offset, density: Density, haptic: HapticFeedback) {
        if (!isPresented || rows.isEmpty()) return
        val leftRect = leftCardRectPx(density)
        with(density) {
            val pad = Layout.cardPadding.toPx()
            val rowH = Layout.rowHeight.toPx()
            val rowS = Layout.rowSpacing.toPx()
            val step = rowH + rowS
            val hysteresis = Layout.columnLockHysteresis.toPx()

            val activeRowIdx = hoveredRowIndex
            if (isInRightColumn && activeRowIdx != null) {
                val rightRect = rightCardRectPx(activeRowIdx, leftRect, density)
                if (rightRect != null) {
                    if (point.x < leftRect.right - hysteresis) {
                        isInRightColumn = false
                    } else if (
                        point.x > rightRect.right + 64.dp.toPx() ||
                        point.y < rightRect.top - 56.dp.toPx() ||
                        point.y > rightRect.bottom + 56.dp.toPx()
                    ) {
                        clearHover()
                        return
                    } else {
                        val count = rows[activeRowIdx].variantsTopToBottom.size
                        val localY = point.y - (rightRect.top + pad)
                        val vIdx = floor(localY / step).toInt().coerceIn(0, count - 1)
                        if (hoveredVariantIndex != vIdx) {
                            hoveredVariantIndex = vIdx
                            haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                        }
                        return
                    }
                }
            }

            if (activeRowIdx != null && rows[activeRowIdx].hasSubmenu) {
                val rightRect = rightCardRectPx(activeRowIdx, leftRect, density)
                if (
                    rightRect != null &&
                    point.x > leftRect.right + 2.dp.toPx() &&
                    point.x <= rightRect.right + 48.dp.toPx() &&
                    point.y >= rightRect.top - 24.dp.toPx() &&
                    point.y <= rightRect.bottom + 24.dp.toPx()
                ) {
                    isInRightColumn = true
                    hasEnteredMenu = true
                    val count = rows[activeRowIdx].variantsTopToBottom.size
                    val localY = point.y - (rightRect.top + pad)
                    val vIdx = floor(localY / step).toInt().coerceIn(0, count - 1)
                    if (hoveredVariantIndex != vIdx) {
                        hoveredVariantIndex = vIdx
                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                    }
                    return
                }
            }

            val leftHitRect = Rect(
                left = leftRect.left - 28.dp.toPx(),
                top = leftRect.top - 24.dp.toPx(),
                right = leftRect.right + 12.dp.toPx(),
                bottom = leftRect.bottom + 6.dp.toPx()
            )
            if (leftHitRect.contains(point)) {
                hasEnteredMenu = true
                isInRightColumn = false
                val localY = point.y - (leftRect.top + pad)
                val idx = floor(localY / step).toInt().coerceIn(0, rows.lastIndex)
                if (hoveredRowIndex != idx) {
                    hoveredRowIndex = idx
                    hoveredVariantIndex = rows[idx].defaultVariantIndex
                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                } else if (hoveredVariantIndex != rows[idx].defaultVariantIndex) {
                    hoveredVariantIndex = rows[idx].defaultVariantIndex
                }
                return
            }

            val anchorExpanded = anchorRect.inflate(16.dp.toPx())
            if (anchorExpanded.contains(point)) {
                isInRightColumn = false
                clearHover()
                return
            }

            isInRightColumn = false
            clearHover()
        }
    }

    fun endDrag(onSelect: (String) -> Unit) {
        if (!isPresented) return
        val rowIdx = hoveredRowIndex
        if (rowIdx != null && rowIdx in rows.indices) {
            val row = rows[rowIdx]
            val variantIdx = hoveredVariantIndex?.takeIf { it in row.variantsTopToBottom.indices }
                ?: row.defaultVariantIndex.coerceIn(0, row.variantsTopToBottom.lastIndex)
            row.variantsTopToBottom.getOrNull(variantIdx)?.let { chosen ->
                onSelect(chosen.id)
            }
            dismiss()
            return
        }

        if (!hasEnteredMenu) {
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }

    private fun clearHover() {
        if (hoveredRowIndex != null || hoveredVariantIndex != null) {
            hoveredRowIndex = null
            hoveredVariantIndex = null
        }
    }
}

@Composable
fun ModelSlidePickerOverlay(
    state: ModelSlidePickerState,
    onSelectModel: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val density = LocalDensity.current
    val haptic = LocalHapticFeedback.current

    val accentColor = when (state.providerKind) {
        "claude" -> colors.accentOrange
        "orchestrator" -> colors.accentPurple
        "other" -> Color(0xFF30B0C7)
        else -> colors.accentBlue
    }

    AnimatedVisibility(
        visible = state.isPresented,
        enter = fadeIn(spring(stiffness = Spring.StiffnessMedium)),
        exit = fadeOut(spring(stiffness = Spring.StiffnessMedium)),
        modifier = modifier
            .fillMaxSize()
            .zIndex(500f)
    ) {
        val leftRect = state.leftCardRectPx(density)
        val hoveredIdx = state.hoveredRowIndex
        val rightRect = if (hoveredIdx != null) state.rightCardRectPx(hoveredIdx, leftRect, density) else null

        Box(modifier = Modifier.fillMaxSize()) {
            // Backdrop scrim matching iOS opacity
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black.copy(alpha = if (state.isInteractiveTapMode) 0.08f else 0.03f))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null
                    ) { state.dismiss() }
            )

            // Left column: Model families (or direct options for single non-effort family like Orchestrator)
            ModelPickerCardSurface(
                widthDp = with(density) { leftRect.width.toDp() },
                accentColor = accentColor,
                baseSurfaceColor = colors.surface,
                modifier = Modifier
                    .offset { IntOffset(leftRect.left.roundToInt(), leftRect.top.roundToInt()) }
                    .animateEnterExit(
                        enter = scaleIn(
                            initialScale = 0.94f,
                            transformOrigin = TransformOrigin(0f, 1f)
                        )
                    )
            ) {
                Column(
                    verticalArrangement = Arrangement.spacedBy(ModelSlidePickerState.Layout.rowSpacing)
                ) {
                    state.rows.forEachIndexed { index, row ->
                        val isHovered = state.hoveredRowIndex == index
                        val isCurrentActive = row.variantsTopToBottom.any {
                            it.id.equals(state.activeModelId, ignoreCase = true)
                        }
                        ModelPickerOptionRow(
                            isHovered = isHovered,
                            isCurrentActive = isCurrentActive,
                            accentColor = accentColor,
                            onClick = {
                                if (!row.hasSubmenu) {
                                    row.variantsTopToBottom.firstOrNull()?.let { only ->
                                        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                        onSelectModel(only.id)
                                        state.dismiss()
                                    }
                                } else if (state.hoveredRowIndex == index) {
                                    val defIdx = row.defaultVariantIndex.coerceIn(0, row.variantsTopToBottom.lastIndex)
                                    row.variantsTopToBottom.getOrNull(defIdx)?.let { chosen ->
                                        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                        onSelectModel(chosen.id)
                                        state.dismiss()
                                    }
                                } else {
                                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                                    state.hoveredRowIndex = index
                                    state.hoveredVariantIndex = row.defaultVariantIndex
                                }
                            }
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                if (isCurrentActive) {
                                    Icon(
                                        imageVector = Icons.Default.Check,
                                        contentDescription = null,
                                        tint = accentColor,
                                        modifier = Modifier.size(13.dp)
                                    )
                                }
                                Text(
                                    text = row.title,
                                    fontSize = 13.5.sp,
                                    fontWeight = if (isHovered || isCurrentActive) FontWeight.SemiBold else FontWeight.Medium,
                                    color = accentColor,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f, fill = false)
                                )
                                Spacer(modifier = Modifier.weight(1f))
                                if (row.hasSubmenu) {
                                    Icon(
                                        imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                        contentDescription = null,
                                        tint = accentColor.copy(alpha = if (isHovered) 0.95f else 0.65f),
                                        modifier = Modifier.size(14.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // Right column: Thinking effort tiers (Extra -> High -> Medium -> Low top-to-bottom)
            if (hoveredIdx != null && hoveredIdx in state.rows.indices && rightRect != null) {
                val row = state.rows[hoveredIdx]
                ModelPickerCardSurface(
                    widthDp = ModelSlidePickerState.Layout.rightCardWidth,
                    accentColor = accentColor,
                    baseSurfaceColor = colors.surface,
                    modifier = Modifier.offset {
                        IntOffset(rightRect.left.roundToInt(), rightRect.top.roundToInt())
                    }
                ) {
                    Column(
                        verticalArrangement = Arrangement.spacedBy(ModelSlidePickerState.Layout.rowSpacing)
                    ) {
                        row.variantsTopToBottom.forEachIndexed { vIdx, variant ->
                            val isHovered = state.hoveredRowIndex == hoveredIdx && state.hoveredVariantIndex == vIdx
                            val isCurrentActive = variant.id.equals(state.activeModelId, ignoreCase = true)
                            val label = if (row.usesEffortLabels) variant.effortDisplayLabel else variant.label
                            ModelPickerOptionRow(
                                isHovered = isHovered,
                                isCurrentActive = isCurrentActive,
                                accentColor = accentColor,
                                onClick = {
                                    haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                                    onSelectModel(variant.id)
                                    state.dismiss()
                                }
                            ) {
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Text(
                                        text = label,
                                        fontSize = 13.5.sp,
                                        fontWeight = if (isHovered || isCurrentActive) FontWeight.SemiBold else FontWeight.Medium,
                                        color = accentColor,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Spacer(modifier = Modifier.weight(1f))
                                    if (isCurrentActive) {
                                        Icon(
                                            imageVector = Icons.Default.Check,
                                            contentDescription = null,
                                            tint = accentColor,
                                            modifier = Modifier.size(12.dp)
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
}

@Composable
private fun ModelPickerCardSurface(
    widthDp: androidx.compose.ui.unit.Dp,
    accentColor: Color,
    baseSurfaceColor: Color,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit
) {
    val shape = RoundedCornerShape(12.dp)
    Surface(
        modifier = modifier
            .width(widthDp)
            .border(1.dp, accentColor.copy(alpha = 0.35f), shape),
        shape = shape,
        color = baseSurfaceColor.copy(alpha = 0.95f),
        shadowElevation = 10.dp,
        tonalElevation = 0.dp
    ) {
        Box(
            modifier = Modifier
                .background(accentColor.copy(alpha = 0.08f))
                .padding(ModelSlidePickerState.Layout.cardPadding)
        ) {
            content()
        }
    }
}

@Composable
private fun ModelPickerOptionRow(
    isHovered: Boolean,
    isCurrentActive: Boolean,
    accentColor: Color,
    onClick: () -> Unit,
    content: @Composable () -> Unit
) {
    val bgColor = when {
        isHovered -> accentColor.copy(alpha = 0.24f)
        isCurrentActive -> accentColor.copy(alpha = 0.11f)
        else -> Color.Transparent
    }
    val borderColor = when {
        isHovered -> accentColor.copy(alpha = 0.80f)
        isCurrentActive -> accentColor.copy(alpha = 0.42f)
        else -> Color.Transparent
    }
    val borderWidth = if (isHovered) 1.25.dp else 1.dp
    val shape = RoundedCornerShape(8.dp)

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(ModelSlidePickerState.Layout.rowHeight)
            .clip(shape)
            .background(bgColor)
            .then(
                if (borderColor != Color.Transparent) {
                    Modifier.border(borderWidth, borderColor, shape)
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
