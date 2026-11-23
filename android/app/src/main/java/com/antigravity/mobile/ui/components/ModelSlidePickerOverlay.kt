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
import androidx.compose.ui.draw.drawBehind
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
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.model.ModelVariant
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.modelProviderAccent
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

data class ModelPickerRow(
    val id: String,
    val title: String,
    val compactTitle: String? = null,
    val providerKind: String,
    val isGroupStart: Boolean,
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
        val defaultRowHeight = 34.dp
        val compactRowHeight = 30.dp
        val minRowHeight = 24.dp
        val defaultRowSpacing = 3.dp
        val compactRowSpacing = 2.dp
        val cardPadding = 5.dp
        val leftCardCompactMinWidth = 148.dp
        val leftCardMinWidth = 186.dp
        val leftCardMaxWidth = 226.dp
        val rightCardWidth = 104.dp
        val compactWidthThreshold = 420.dp
        val gapBetweenButtonAndMenu = 8.dp
        val gapBetweenColumns = 8.dp
        val horizontalMargin = 12.dp
        val topSafeMargin = 12.dp
        val columnLockHysteresis = 6.dp
    }

    fun isCompactWidth(density: Density): Boolean = with(density) {
        val wPx = containerSize.width.toFloat()
        wPx <= 0f || wPx < Layout.compactWidthThreshold.toPx()
    }

    /** 长按打开全模型一级菜单：按 Gemini(蓝) -> Claude(橙) -> Orchestrator(钛灰) -> 其他 列出全部可用模型。 */
    fun open(activeModel: String, options: List<ModelOption>) {
        val families = ModelDefaults.modelFamilies(options, provider = null)
        val builtRows = mutableListOf<ModelPickerRow>()
        var prevProvider: String? = null

        for (family in families) {
            val firstId = family.variants.firstOrNull()?.id ?: family.provider
            val kind = if (family.provider in setOf("gemini", "claude", "orchestrator", "other")) {
                family.provider.lowercase()
            } else {
                ModelDefaults.providerKind(firstId)
            }
            val isFirstInGroup = prevProvider != null && prevProvider != kind

            if (!family.hasEffortVariants && family.variants.size > 1 && kind != "orchestrator") {
                family.variants.forEachIndexed { vIndex, variant ->
                    val rawTitle = variant.rawName.trim().ifEmpty { family.name }
                    val displayTitle = ModelDefaults.formatSlidePickerTitle(rawTitle)
                    builtRows.add(
                        ModelPickerRow(
                            id = variant.id,
                            title = displayTitle.ifEmpty { family.name },
                            providerKind = kind,
                            isGroupStart = if (vIndex == 0) isFirstInGroup else false,
                            variantsTopToBottom = listOf(variant),
                            defaultVariantIndex = 0,
                            hasSubmenu = false,
                            usesEffortLabels = false
                        )
                    )
                }
            } else {
                val ordered = family.variantsTopToBottom
                val fullTitle: String
                val compactTitle: String?
                if (kind == "orchestrator") {
                    if (ordered.size > 1) {
                        fullTitle = family.name.ifBlank { "Orchestrator" }
                        compactTitle = "Orch"
                    } else {
                        val rawTitle = ordered.firstOrNull()?.rawName?.trim().orEmpty().ifEmpty { family.name }
                        fullTitle = rawTitle.ifEmpty { "Orchestrator" }
                        compactTitle = ModelDefaults.formatSlidePickerTitle(rawTitle).ifEmpty { "Orch" }
                    }
                } else {
                    fullTitle = ModelDefaults.formatSlidePickerTitle(family.name).ifEmpty { family.name }
                    compactTitle = null
                }
                builtRows.add(
                    ModelPickerRow(
                        id = family.id,
                        title = fullTitle,
                        compactTitle = compactTitle,
                        providerKind = kind,
                        isGroupStart = isFirstInGroup,
                        variantsTopToBottom = ordered,
                        defaultVariantIndex = family.defaultVariantIndexTopToBottom,
                        hasSubmenu = ordered.size > 1,
                        usesEffortLabels = family.hasEffortVariants
                    )
                )
            }
            prevProvider = kind
        }
        if (builtRows.isEmpty()) return

        providerKind = ModelDefaults.providerKind(activeModel)
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

    private fun availableLeftHeightPx(density: Density): Float = with(density) {
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

    fun leftRowSpacingDp(): Dp =
        if (rows.size > 7) Layout.compactRowSpacing else Layout.defaultRowSpacing

    fun leftRowHeightDp(density: Density): Dp = with(density) {
        val count = max(rows.size, 1)
        val preferredPx = (if (rows.size > 7) Layout.compactRowHeight else Layout.defaultRowHeight).toPx()
        val rowS = leftRowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val avail = availableLeftHeightPx(density)
        val overhead = pad * 2f + max(0, count - 1) * rowS
        val maxRowPx = (avail - overhead) / count.toFloat()
        val chosenPx = min(preferredPx, max(Layout.minRowHeight.toPx(), floor(maxRowPx)))
        chosenPx.toDp()
    }

    fun leftContentNeedsScroll(density: Density): Boolean = with(density) {
        val count = max(rows.size, 1)
        val rowH = leftRowHeightDp(density).toPx()
        val rowS = leftRowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val naturalHeight = count * rowH + max(0, count - 1) * rowS + pad * 2f
        naturalHeight > availableLeftHeightPx(density) + 1f
    }

    fun leftCardRectPx(density: Density): Rect = with(density) {
        val count = max(rows.size, 1)
        val rowH = leftRowHeightDp(density).toPx()
        val rowS = leftRowSpacingDp().toPx()
        val pad = Layout.cardPadding.toPx()
        val naturalHeight = count * rowH + max(0, count - 1) * rowS + pad * 2f
        val avail = availableLeftHeightPx(density)
        val height = min(naturalHeight, avail)

        val containerW = containerSize.width.toFloat()
        val hMargin = Layout.horizontalMargin.toPx()
        val rightW = Layout.rightCardWidth.toPx()
        val colGap = Layout.gapBetweenColumns.toPx()
        val maxRoom = if (containerW > 0f) containerW - rightW - colGap - hMargin * 2f else Layout.leftCardMaxWidth.toPx()
        val minLeft = min(Layout.leftCardMinWidth.toPx(), max(Layout.leftCardCompactMinWidth.toPx(), maxRoom))
        val upperW = min(Layout.leftCardMaxWidth.toPx(), max(minLeft, maxRoom))
        val preferredW = if (containerW > 0f) containerW * 0.54f else minLeft
        val width = min(upperW, max(minLeft, preferredW))

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
        val rowH = leftRowHeightDp(density).toPx()
        val rowS = leftRowSpacingDp().toPx()
        val rawY = leftRect.top + pad + index * (rowH + rowS) + rowH / 2f
        rawY.coerceIn(leftRect.top + rowH / 2f, leftRect.bottom - rowH / 2f)
    }

    fun rightCardRectPx(rowIndex: Int, leftRect: Rect, density: Density): Rect? = with(density) {
        val row = rows.getOrNull(rowIndex) ?: return@with null
        if (!row.hasSubmenu) return@with null
        val count = row.variantsTopToBottom.size
        if (count <= 1) return@with null

        val pad = Layout.cardPadding.toPx()
        val rowH = Layout.defaultRowHeight.toPx()
        val rowS = Layout.defaultRowSpacing.toPx()
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
            val leftStep = leftRowHeightDp(density).toPx() + leftRowSpacingDp().toPx()
            val rightStep = Layout.defaultRowHeight.toPx() + Layout.defaultRowSpacing.toPx()
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
                        val vIdx = floor(localY / rightStep).toInt().coerceIn(0, count - 1)
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
                    val vIdx = floor(localY / rightStep).toInt().coerceIn(0, count - 1)
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
                val idx = floor(localY / leftStep).toInt().coerceIn(0, rows.lastIndex)
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
private fun providerAccentColor(kind: String): Color = AntigravityTheme.colors.modelProviderAccent(kind)

@Composable
fun ModelSlidePickerOverlay(
    state: ModelSlidePickerState,
    onSelectModel: (String) -> Unit,
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
        val leftRect = state.leftCardRectPx(density)
        val leftRowHeightDp = state.leftRowHeightDp(density)
        val leftRowSpacingDp = state.leftRowSpacingDp()
        val needsScroll = state.leftContentNeedsScroll(density)
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

            // Left column: All models color-coded Blue (Gemini) / Orange (Claude) / Purple (Orchestrator)
            ModelPickerCardSurface(
                widthDp = with(density) { leftRect.width.toDp() },
                maxHeightDp = with(density) { leftRect.height.toDp() },
                accentColor = null,
                baseSurfaceColor = colors.surface,
                borderColor = colors.border,
                modifier = Modifier
                    .offset { IntOffset(leftRect.left.roundToInt(), leftRect.top.roundToInt()) }
                    .animateEnterExit(
                        enter = scaleIn(
                            initialScale = 0.94f,
                            transformOrigin = TransformOrigin(0f, 1f)
                        )
                    )
            ) {
                val leftColumnModifier = if (needsScroll) {
                    Modifier.verticalScroll(rememberScrollState())
                } else {
                    Modifier
                }
                Column(
                    modifier = leftColumnModifier,
                    verticalArrangement = Arrangement.spacedBy(leftRowSpacingDp)
                ) {
                    state.rows.forEachIndexed { index, row ->
                        val rowColor = providerAccentColor(row.providerKind)
                        val isHovered = state.hoveredRowIndex == index
                        val isCurrentActive = row.variantsTopToBottom.any {
                            it.id.equals(state.activeModelId, ignoreCase = true)
                        }
                        val dividerColor = colors.separator.copy(alpha = 0.45f)
                        val spacingPx = with(density) { leftRowSpacingDp.toPx() }
                        val rowModifier = if (row.isGroupStart) {
                            Modifier.drawBehind {
                                val y = -(spacingPx / 2f)
                                drawLine(
                                    color = dividerColor,
                                    start = Offset(6.dp.toPx(), y),
                                    end = Offset(size.width - 6.dp.toPx(), y),
                                    strokeWidth = 0.75.dp.toPx()
                                )
                            }
                        } else {
                            Modifier
                        }

                        ModelPickerOptionRow(
                            isHovered = isHovered,
                            isCurrentActive = isCurrentActive,
                            accentColor = rowColor,
                            rowHeight = leftRowHeightDp,
                            modifier = rowModifier,
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
                                        tint = rowColor,
                                        modifier = Modifier.size(13.dp)
                                    )
                                }
                                val rowTitle = if (state.isCompactWidth(density)) {
                                    row.compactTitle ?: row.title
                                } else {
                                    row.title
                                }
                                Text(
                                    text = rowTitle,
                                    fontSize = if (leftRowHeightDp < 29.dp) 12.5.sp else 13.5.sp,
                                    fontWeight = if (isHovered || isCurrentActive) FontWeight.SemiBold else FontWeight.Medium,
                                    color = rowColor,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f)
                                )
                                if (row.hasSubmenu) {
                                    Icon(
                                        imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                        contentDescription = null,
                                        tint = rowColor.copy(alpha = if (isHovered) 0.95f else 0.65f),
                                        modifier = Modifier.size(14.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // Right column: Thinking effort tiers Low -> Medium -> High -> Extra (top-to-bottom), default Medium aligned
            if (hoveredIdx != null && hoveredIdx in state.rows.indices && rightRect != null) {
                val row = state.rows[hoveredIdx]
                val rowColor = providerAccentColor(row.providerKind)
                ModelPickerCardSurface(
                    widthDp = ModelSlidePickerState.Layout.rightCardWidth,
                    maxHeightDp = null,
                    accentColor = rowColor,
                    baseSurfaceColor = colors.surface,
                    borderColor = rowColor.copy(alpha = 0.35f),
                    modifier = Modifier.offset {
                        IntOffset(rightRect.left.roundToInt(), rightRect.top.roundToInt())
                    }
                ) {
                    Column(
                        verticalArrangement = Arrangement.spacedBy(ModelSlidePickerState.Layout.defaultRowSpacing)
                    ) {
                        row.variantsTopToBottom.forEachIndexed { vIdx, variant ->
                            val isHovered = state.hoveredRowIndex == hoveredIdx && state.hoveredVariantIndex == vIdx
                            val isCurrentActive = variant.id.equals(state.activeModelId, ignoreCase = true)
                            val label = if (row.usesEffortLabels) variant.effortDisplayLabel else variant.label
                            ModelPickerOptionRow(
                                isHovered = isHovered,
                                isCurrentActive = isCurrentActive,
                                accentColor = rowColor,
                                rowHeight = ModelSlidePickerState.Layout.defaultRowHeight,
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
                                        color = rowColor,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f)
                                    )
                                    if (isCurrentActive) {
                                        Icon(
                                            imageVector = Icons.Default.Check,
                                            contentDescription = null,
                                            tint = rowColor,
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
    widthDp: Dp,
    maxHeightDp: Dp?,
    accentColor: Color?,
    baseSurfaceColor: Color,
    borderColor: Color,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit
) {
    val shape = RoundedCornerShape(12.dp)
    val sizedModifier = modifier
        .width(widthDp)
        .let { m -> if (maxHeightDp != null) m.heightIn(max = maxHeightDp) else m }
        .border(1.dp, borderColor, shape)
    Surface(
        modifier = sizedModifier,
        shape = shape,
        color = baseSurfaceColor.copy(alpha = 0.96f),
        shadowElevation = 10.dp,
        tonalElevation = 0.dp
    ) {
        Box(
            modifier = Modifier
                .then(
                    if (accentColor != null) Modifier.background(accentColor.copy(alpha = 0.08f)) else Modifier
                )
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
    rowHeight: Dp,
    modifier: Modifier = Modifier,
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
        modifier = modifier
            .fillMaxWidth()
            .height(rowHeight)
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
