package com.antigravity.mobile.ui.components

import android.animation.ValueAnimator
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelEffortRank
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.model.ModelVariant
import com.antigravity.mobile.data.model.WeeklyPaceInfo
import com.antigravity.mobile.data.service.ModelPickerQuotaCache
import com.antigravity.mobile.data.service.ModelQuotaPoolKind
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.modelProviderAccent
import com.antigravity.mobile.ui.util.loc
import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sin

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
) {
    val hasInlineEffort: Boolean
        get() = variantsTopToBottom.size > 1
}

// MARK: - Shared Effort Visual Template & Provider Palettes

private data class CosmicStarSpec(
    val x: Float,
    val y: Float,
    val radiusDp: Float,
    val phase: Float,
    val speed: Float,
    val isSparkle: Boolean
)

private val SharedCosmicStars = listOf(
    CosmicStarSpec(0.07f, 0.28f, 1.05f, 0.2f, 1.4f, false),
    CosmicStarSpec(0.13f, 0.72f, 0.85f, 1.7f, 1.1f, false),
    CosmicStarSpec(0.21f, 0.34f, 1.45f, 3.1f, 1.7f, true),
    CosmicStarSpec(0.28f, 0.66f, 0.90f, 4.5f, 1.2f, false),
    CosmicStarSpec(0.35f, 0.22f, 1.10f, 0.9f, 1.5f, false),
    CosmicStarSpec(0.43f, 0.76f, 1.25f, 2.3f, 1.3f, false),
    CosmicStarSpec(0.50f, 0.30f, 0.85f, 5.1f, 1.8f, false),
    CosmicStarSpec(0.57f, 0.62f, 1.55f, 1.2f, 1.6f, true),
    CosmicStarSpec(0.64f, 0.24f, 0.95f, 3.8f, 1.1f, false),
    CosmicStarSpec(0.70f, 0.74f, 1.10f, 0.4f, 1.4f, false),
    CosmicStarSpec(0.76f, 0.38f, 0.80f, 2.8f, 1.9f, false),
    CosmicStarSpec(0.82f, 0.68f, 1.45f, 4.1f, 1.5f, true),
    CosmicStarSpec(0.88f, 0.26f, 1.00f, 5.6f, 1.2f, false),
    CosmicStarSpec(0.93f, 0.54f, 1.20f, 1.9f, 1.6f, false),
    CosmicStarSpec(0.46f, 0.48f, 0.75f, 2.6f, 1.3f, false),
    CosmicStarSpec(0.61f, 0.44f, 0.85f, 4.9f, 1.7f, false)
)

internal data class EffortStylePalette(
    val accent: Color,
    val lowFill: Color,
    val medColors: List<Color>,
    val highColors: List<Color>,
    val cosmicColors: List<Color>,
    val starColor: Color,
    val glowColor: Color,
    val chipRimColors: List<Color>
) {
    companion object {
        fun forProvider(kind: String, isDark: Boolean, fallbackAccent: Color): EffortStylePalette {
            return when (kind.lowercase()) {
                "gemini" -> EffortStylePalette(
                    accent = fallbackAccent,
                    lowFill = Color(0xFF2563EB).copy(alpha = if (isDark) 0.24f else 0.15f),
                    medColors = if (isDark) {
                        listOf(Color(0xFF1E40AF).copy(alpha = 0.46f), Color(0xFF3B82F6).copy(alpha = 0.62f))
                    } else {
                        listOf(Color(0xFF60A5FA).copy(alpha = 0.28f), Color(0xFF2563EB).copy(alpha = 0.42f))
                    },
                    highColors = listOf(Color(0xFF1D4ED8), Color(0xFF2563EB), Color(0xFF38BDF8)),
                    cosmicColors = listOf(Color(0xFF13174A), Color(0xFF1E3A8A), Color(0xFF4F46E5), Color(0xFF38BDF8)),
                    starColor = Color.White,
                    glowColor = Color(0xFF38BDF8),
                    chipRimColors = listOf(Color(0xFF38BDF8), Color(0xFF6366F1), Color(0xFF2563EB), Color(0xFF38BDF8))
                )
                "claude" -> EffortStylePalette(
                    accent = fallbackAccent,
                    lowFill = Color(0xFFEA580C).copy(alpha = if (isDark) 0.24f else 0.15f),
                    medColors = if (isDark) {
                        listOf(Color(0xFF9A3412).copy(alpha = 0.46f), Color(0xFFF97316).copy(alpha = 0.62f))
                    } else {
                        listOf(Color(0xFFFDBA74).copy(alpha = 0.34f), Color(0xFFEA580C).copy(alpha = 0.42f))
                    },
                    highColors = listOf(Color(0xFFC2410C), Color(0xFFEA580C), Color(0xFFFB923C)),
                    cosmicColors = listOf(Color(0xFF451A03), Color(0xFF9A3412), Color(0xFFEA580C), Color(0xFFFBBF24)),
                    starColor = Color(0xFFFEF3C7),
                    glowColor = Color(0xFFFB923C),
                    chipRimColors = listOf(Color(0xFFFBBF24), Color(0xFFF97316), Color(0xFFC2410C), Color(0xFFFBBF24))
                )
                "orchestrator" -> EffortStylePalette(
                    accent = fallbackAccent,
                    lowFill = Color(0xFF475569).copy(alpha = if (isDark) 0.28f else 0.14f),
                    medColors = if (isDark) {
                        listOf(Color(0xFF334155).copy(alpha = 0.55f), Color(0xFF64748B).copy(alpha = 0.68f))
                    } else {
                        listOf(Color(0xFF94A3B8).copy(alpha = 0.30f), Color(0xFF475569).copy(alpha = 0.42f))
                    },
                    highColors = if (isDark) {
                        listOf(Color(0xFF334155), Color(0xFF475569), Color(0xFF94A3B8))
                    } else {
                        listOf(Color(0xFF1E293B), Color(0xFF475569), Color(0xFF64748B))
                    },
                    cosmicColors = listOf(Color(0xFF0B101C), Color(0xFF1E293B), Color(0xFF475569), Color(0xFF94A3B8)),
                    starColor = Color(0xFFF1F5F9),
                    glowColor = Color(0xFFCBD5E1),
                    chipRimColors = listOf(Color(0xFFE2E8F0), Color(0xFF94A3B8), Color(0xFF475569), Color(0xFFE2E8F0))
                )
                else -> EffortStylePalette(
                    accent = fallbackAccent,
                    lowFill = Color(0xFF0D9488).copy(alpha = if (isDark) 0.24f else 0.15f),
                    medColors = if (isDark) {
                        listOf(Color(0xFF115E59).copy(alpha = 0.48f), Color(0xFF14B8A6).copy(alpha = 0.62f))
                    } else {
                        listOf(Color(0xFF5EEAD4).copy(alpha = 0.30f), Color(0xFF0D9488).copy(alpha = 0.42f))
                    },
                    highColors = listOf(Color(0xFF0F766E), Color(0xFF0D9488), Color(0xFF2DD4BF)),
                    cosmicColors = listOf(Color(0xFF042F2E), Color(0xFF0F766E), Color(0xFF0D9488), Color(0xFF5EEAD4)),
                    starColor = Color(0xFFCCFBF1),
                    glowColor = Color(0xFF2DD4BF),
                    chipRimColors = listOf(Color(0xFF5EEAD4), Color(0xFF14B8A6), Color(0xFF0F766E), Color(0xFF5EEAD4))
                )
            }
        }

        fun fillFraction(index: Int, count: Int): Float {
            if (count <= 1) return 1f
            val c = index.coerceIn(0, count - 1)
            return when (count) {
                2 -> if (c == 0) 0.50f else 1f
                3 -> when (c) {
                    0 -> 0.35f
                    1 -> 0.68f
                    else -> 1f
                }
                4 -> when (c) {
                    0 -> 0.28f
                    1 -> 0.52f
                    2 -> 0.76f
                    else -> 1f
                }
                else -> 0.24f + 0.76f * (c.toFloat() / (count - 1).toFloat())
            }
        }
    }
}

@Composable
private fun CosmicStarfieldLayer(
    palette: EffortStylePalette,
    animated: Boolean,
    isSubtleResting: Boolean = false,
    modifier: Modifier = Modifier
) {
    val animatorsEnabled = remember { ValueAnimator.areAnimatorsEnabled() }
    val shouldAnimate = animated && animatorsEnabled
    val phase: Float = if (shouldAnimate) {
        val transition = rememberInfiniteTransition(label = "xhigh_cosmic")
        val value by transition.animateFloat(
            initialValue = 0f,
            targetValue = 1f,
            animationSpec = infiniteRepeatable(
                animation = tween(durationMillis = 2800, easing = LinearEasing),
                repeatMode = RepeatMode.Restart
            ),
            label = "cosmic_phase"
        )
        value
    } else {
        0.25f
    }

    Canvas(modifier = modifier) {
        if (size.width <= 1f || size.height <= 1f) return@Canvas
        drawRect(
            brush = Brush.horizontalGradient(palette.cosmicColors),
            alpha = if (isSubtleResting) 0.78f else 1f
        )

        if (shouldAnimate) {
            val sweepX = size.width * (phase * 1.8f - 0.4f)
            val bandW = max(36.dp.toPx(), size.width * 0.34f)
            drawRect(
                brush = Brush.linearGradient(
                    colors = listOf(
                        Color.Transparent,
                        palette.starColor.copy(alpha = 0.20f),
                        palette.glowColor.copy(alpha = 0.14f),
                        Color.Transparent
                    ),
                    start = Offset(sweepX - bandW / 2f, 0f),
                    end = Offset(sweepX + bandW / 2f, size.height * 0.45f)
                )
            )
        }

        val t = phase * (2.0 * PI)
        for (star in SharedCosmicStars) {
            val cx = star.x * size.width
            val cy = star.y * size.height
            val rPx = star.radiusDp.dp.toPx()
            val wave = if (shouldAnimate) {
                (0.45 + 0.55 * (0.5 + 0.5 * sin(t * star.speed + star.phase))).toFloat()
            } else {
                0.85f
            }
            val baseAlpha = (if (isSubtleResting) 0.55f else 0.95f) * wave

            drawCircle(
                color = palette.glowColor.copy(alpha = baseAlpha * 0.28f),
                radius = rPx * 2.2f,
                center = Offset(cx, cy)
            )
            drawCircle(
                color = palette.starColor.copy(alpha = baseAlpha),
                radius = rPx,
                center = Offset(cx, cy)
            )
            if (star.isSparkle && !isSubtleResting) {
                val arm = rPx * 2.5f
                val stroke = 0.7.dp.toPx()
                drawLine(
                    color = palette.starColor.copy(alpha = baseAlpha * 0.65f),
                    start = Offset(cx - arm, cy),
                    end = Offset(cx + arm, cy),
                    strokeWidth = stroke
                )
                drawLine(
                    color = palette.starColor.copy(alpha = baseAlpha * 0.65f),
                    start = Offset(cx, cy - arm),
                    end = Offset(cx, cy + arm),
                    strokeWidth = stroke
                )
            }
        }
    }
}

// MARK: - Picker State & Gesture Physics

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

    private var lastDragPoint: Offset? = null
    private var isHorizontalRowLocked: Boolean = false

    object Layout {
        val defaultRowHeight = 36.dp
        val compactRowHeight = 31.dp
        val minRowHeight = 25.dp
        val defaultRowSpacing = 3.dp
        val compactRowSpacing = 2.dp
        val cardPadding = 5.dp
        val leftCardCompactMinWidth = 184.dp
        val leftCardMinWidth = 212.dp
        val leftCardMaxWidth = 248.dp
        val rightCardCompactWidth = 104.dp
        val rightCardWidth = 116.dp
        val rightCardWideWidth = 124.dp
        val compactWidthThreshold = 420.dp
        val gapBetweenButtonAndMenu = 8.dp
        val gapBetweenColumns = 8.dp
        val horizontalMargin = 12.dp
        val topSafeMargin = 12.dp
        val tierBoundaryHysteresis = 6.dp
        val horizontalRowLockVerticalPad = 15.dp
    }

    fun isCompactWidth(density: Density): Boolean = with(density) {
        val wPx = containerSize.width.toFloat()
        wPx <= 0f || wPx < Layout.compactWidthThreshold.toPx()
    }

    fun rightQuotaWidthDp(density: Density): Dp = with(density) {
        val wDp = containerSize.width.toDp()
        when {
            containerSize.width > 0 && wDp < 380.dp -> Layout.rightCardCompactWidth
            containerSize.width > 0 && wDp >= 420.dp -> Layout.rightCardWideWidth
            else -> Layout.rightCardWidth
        }
    }

    fun activeVariantIndex(row: ModelPickerRow): Int? {
        val idx = row.variantsTopToBottom.indexOfFirst { it.id.equals(activeModelId, ignoreCase = true) }
        return if (idx >= 0) idx else null
    }

    fun highlightedQuotaPool(): ModelQuotaPoolKind? {
        val hIdx = hoveredRowIndex
        if (hIdx != null && hIdx in rows.indices) {
            val row = rows[hIdx]
            return ModelPickerQuotaCache.quotaPool(
                providerKind = row.providerKind,
                title = row.title,
                variantIds = row.variantsTopToBottom.map { it.id },
                variantRawNames = row.variantsTopToBottom.map { it.rawName },
                quotaPools = row.variantsTopToBottom.map { it.quotaPool }
            )
        }
        val activeRow = rows.firstOrNull { activeVariantIndex(it) != null } ?: return null
        return ModelPickerQuotaCache.quotaPool(
            providerKind = activeRow.providerKind,
            title = activeRow.title,
            variantIds = activeRow.variantsTopToBottom.map { it.id },
            variantRawNames = activeRow.variantsTopToBottom.map { it.rawName },
            quotaPools = activeRow.variantsTopToBottom.map { it.quotaPool }
        )
    }

    /** 长按打开全模型单列菜单：按 Gemini(蓝) -> Claude(橙) -> Orchestrator(钛灰) -> 其他 分组，
     *  行内横滑调节思考强度，右侧伴随展示 Gemini / Claude 共享额度。 */
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
                val fullTitle = if (kind == "orchestrator") {
                    val source = family.name.ifBlank { ordered.firstOrNull()?.rawName.orEmpty().ifBlank { "Orchestrator" } }
                    ModelDefaults.formatSlidePickerTitle(source).ifEmpty { "Orch" }
                } else {
                    ModelDefaults.formatSlidePickerTitle(family.name).ifEmpty { family.name }
                }
                builtRows.add(
                    ModelPickerRow(
                        id = family.id,
                        title = fullTitle,
                        compactTitle = null,
                        providerKind = kind,
                        isGroupStart = isFirstInGroup,
                        variantsTopToBottom = ordered,
                        defaultVariantIndex = family.defaultVariantIndexTopToBottom,
                        hasSubmenu = ordered.size > 1,
                        usesEffortLabels = family.hasEffortVariants || ordered.any { it.effortRank != null }
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
        lastDragPoint = null
        isHorizontalRowLocked = false
        isPresented = true
    }

    fun dismiss() {
        isPresented = false
        isInteractiveTapMode = false
        hoveredRowIndex = null
        hoveredVariantIndex = null
        isInRightColumn = false
        hasEnteredMenu = false
        lastDragPoint = null
        isHorizontalRowLocked = false
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
        val rightW = rightQuotaWidthDp(density).toPx()
        val colGap = Layout.gapBetweenColumns.toPx()
        val maxRoom = if (containerW > 0f) containerW - rightW - colGap - hMargin * 2f else Layout.leftCardMaxWidth.toPx()
        val minLeft = min(Layout.leftCardMinWidth.toPx(), max(Layout.leftCardCompactMinWidth.toPx(), maxRoom))
        val upperW = min(Layout.leftCardMaxWidth.toPx(), max(minLeft, maxRoom))
        val preferredW = if (containerW > 0f) maxRoom else Layout.leftCardMaxWidth.toPx()
        val width = min(upperW, max(minLeft, preferredW))

        val maxLeftX = max(hMargin, containerW - width - rightW - colGap - hMargin)
        val minX = if (containerW > 0f && containerW < 520.dp.toPx()) {
            hMargin
        } else if (containerW > 0f) {
            min(max(hMargin, anchorRect.left), maxLeftX)
        } else {
            hMargin
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

    fun variantIndexForX(
        rowIndex: Int,
        localX: Float,
        trackWidth: Float,
        currentVariant: Int?,
        hysteresisPx: Float
    ): Int {
        val row = rows.getOrNull(rowIndex) ?: return 0
        val count = row.variantsTopToBottom.size
        if (count <= 1 || trackWidth <= 1f) return 0
        if (localX <= 0f) return 0
        if (localX >= trackWidth) return count - 1

        val segW = trackWidth / count.toFloat()
        if (currentVariant != null && currentVariant in 0 until count) {
            val curMin = currentVariant * segW - hysteresisPx
            val curMax = (currentVariant + 1) * segW + hysteresisPx
            if (localX in curMin..curMax) {
                return currentVariant
            }
        }
        return floor(localX / segW).toInt().coerceIn(0, count - 1)
    }

    fun triggerHaptic(row: ModelPickerRow, variantIndex: Int, haptic: HapticFeedback) {
        val rank = row.variantsTopToBottom.getOrNull(variantIndex)?.effortRank
        if (rank == ModelEffortRank.EXTRA) {
            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
        } else {
            haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
        }
    }

    fun updateDrag(point: Offset, density: Density, haptic: HapticFeedback) {
        if (!isPresented || rows.isEmpty()) return
        val leftRect = leftCardRectPx(density)
        val prevPoint = lastDragPoint
        lastDragPoint = point

        with(density) {
            val pad = Layout.cardPadding.toPx()
            val rowH = leftRowHeightDp(density).toPx()
            val leftStep = rowH + leftRowSpacingDp().toPx()
            val hysteresisPx = Layout.tierBoundaryHysteresis.toPx()
            val lockPadPx = Layout.horizontalRowLockVerticalPad.toPx()

            val anchorExpanded = anchorRect.inflate(16.dp.toPx())
            if (
                anchorExpanded.contains(point) ||
                point.y > leftRect.bottom + 18.dp.toPx() ||
                point.y < leftRect.top - 52.dp.toPx() ||
                point.x < leftRect.left - 48.dp.toPx()
            ) {
                isHorizontalRowLocked = false
                clearHover()
                return
            }

            val rightLimit = if (containerSize.width > 0) containerSize.width + 24.dp.toPx() else leftRect.right + 180.dp.toPx()
            if (point.x < leftRect.left - 32.dp.toPx() || point.x > rightLimit) {
                isHorizontalRowLocked = false
                clearHover()
                return
            }

            hasEnteredMenu = true

            val curIdx = hoveredRowIndex
            val resolvedRowIdx: Int = if (curIdx != null && curIdx in rows.indices && rows[curIdx].hasInlineEffort) {
                if (prevPoint != null) {
                    val dx = abs(point.x - prevPoint.x)
                    val dy = abs(point.y - prevPoint.y)
                    if (dx > dy * 1.35f && dx > 2.dp.toPx()) {
                        isHorizontalRowLocked = true
                    }
                }
                val centerY = leftRowCenterYPx(curIdx, leftRect, density)
                val tolerance = if (isHorizontalRowLocked) (rowH * 0.5f + lockPadPx) else (rowH * 0.5f + 1.5.dp.toPx())
                if (abs(point.y - centerY) <= tolerance) {
                    curIdx
                } else {
                    isHorizontalRowLocked = false
                    val localY = point.y - (leftRect.top + pad)
                    floor(localY / leftStep).toInt().coerceIn(0, rows.lastIndex)
                }
            } else {
                isHorizontalRowLocked = false
                val localY = point.y - (leftRect.top + pad)
                floor(localY / leftStep).toInt().coerceIn(0, rows.lastIndex)
            }

            val row = rows[resolvedRowIdx]
            val trackMinX = leftRect.left + pad
            val trackWidth = max(1f, leftRect.width - pad * 2f)
            val localX = point.x - trackMinX
            val sameRow = hoveredRowIndex == resolvedRowIdx
            val nextVariantIdx = variantIndexForX(
                rowIndex = resolvedRowIdx,
                localX = localX,
                trackWidth = trackWidth,
                currentVariant = if (sameRow) hoveredVariantIndex else null,
                hysteresisPx = hysteresisPx
            )

            val changed = hoveredRowIndex != resolvedRowIdx || hoveredVariantIndex != nextVariantIdx
            hoveredRowIndex = resolvedRowIdx
            hoveredVariantIndex = nextVariantIdx
            if (changed) {
                triggerHaptic(row, nextVariantIdx, haptic)
            }
        }
    }

    fun endDrag(onSelect: (String) -> Unit) {
        if (!isPresented) return
        lastDragPoint = null
        isHorizontalRowLocked = false
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

// MARK: - Main Overlay Composable

@Composable
fun ModelSlidePickerOverlay(
    state: ModelSlidePickerState,
    onSelectModel: (String) -> Unit,
    modifier: Modifier = Modifier,
    isRunning: Boolean = false
) {
    val colors = AntigravityTheme.colors
    val isDark = colors.isDark
    val density = LocalDensity.current
    val haptic = LocalHapticFeedback.current
    val quotaAccount by ModelPickerQuotaCache.account.collectAsState()
    val quotaEnabled by ModelPickerQuotaCache.enabled.collectAsState()
    val paceCursorEnabled by ModelPickerQuotaCache.weeklyPaceCursorEnabled.collectAsState()
    var rightQuotaHeightPx by remember { mutableIntStateOf(0) }

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
        val acc = quotaAccount
        val showQuotaCard = quotaEnabled && acc != null && (
            acc.gemini5h != null || acc.geminiWeekly != null ||
                acc.claude5h != null || acc.claudeWeekly != null
            )

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

            // Top floating pills: running hint + live drag HUD preview (anti-thumb-occlusion)
            val showLivePreview = !state.isInteractiveTapMode && hoveredIdx != null && hoveredIdx in state.rows.indices
            if (isRunning || showLivePreview) {
                val stackHeightDp = when {
                    isRunning && showLivePreview -> 60.dp
                    showLivePreview -> 32.dp
                    else -> 28.dp
                }
                val topY = (leftRect.top - with(density) { (stackHeightDp + 6.dp).toPx() })
                    .coerceAtLeast(with(density) { 10.dp.toPx() })
                Column(
                    modifier = Modifier
                        .offset { IntOffset(leftRect.left.roundToInt(), topY.roundToInt()) }
                        .width(with(density) { leftRect.width.toDp() }),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(5.dp)
                ) {
                    if (isRunning) {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(999.dp))
                                .background(colors.surface.copy(alpha = 0.96f))
                                .border(0.8.dp, colors.accentOrange.copy(alpha = 0.35f), RoundedCornerShape(999.dp))
                                .padding(horizontal = 10.dp, vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Text(
                                text = "运行中 · 切换下轮生效".loc,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.accentOrange,
                                maxLines = 1
                            )
                        }
                    }

                    if (showLivePreview) {
                        val row = state.rows[hoveredIdx!!]
                        val vIdx = (state.hoveredVariantIndex ?: row.defaultVariantIndex)
                            .coerceIn(0, row.variantsTopToBottom.lastIndex.coerceAtLeast(0))
                        val variant = row.variantsTopToBottom.getOrNull(vIdx)
                        val rank = variant?.effortRank
                        val accent = providerAccentColor(row.providerKind)
                        val palette = EffortStylePalette.forProvider(row.providerKind, isDark, accent)
                        val effortText = if (row.hasInlineEffort && variant != null) {
                            if (row.usesEffortLabels) (rank?.shortLabel ?: variant.effortDisplayLabel) else variant.label
                        } else null
                        val usesDarkBackdrop = rank == ModelEffortRank.HIGH || rank == ModelEffortRank.EXTRA
                        val pillShape = RoundedCornerShape(999.dp)

                        Box(
                            modifier = Modifier
                                .shadow(if (rank == ModelEffortRank.EXTRA) 8.dp else 4.dp, pillShape)
                                .clip(pillShape)
                                .then(
                                    when (rank) {
                                        ModelEffortRank.EXTRA -> Modifier
                                        ModelEffortRank.HIGH -> Modifier.background(Brush.horizontalGradient(palette.highColors))
                                        ModelEffortRank.MEDIUM -> Modifier
                                            .background(colors.surface.copy(alpha = 0.96f))
                                            .background(Brush.horizontalGradient(palette.medColors))
                                        else -> Modifier
                                            .background(colors.surface.copy(alpha = 0.96f))
                                            .background(palette.lowFill)
                                    }
                                )
                                .border(
                                    width = if (rank == ModelEffortRank.EXTRA) 1.1.dp else 0.85.dp,
                                    color = when (rank) {
                                        ModelEffortRank.EXTRA -> palette.glowColor.copy(alpha = 0.78f)
                                        ModelEffortRank.HIGH -> Color.White.copy(alpha = 0.28f)
                                        else -> palette.accent.copy(alpha = 0.45f)
                                    },
                                    shape = pillShape
                                )
                        ) {
                            if (rank == ModelEffortRank.EXTRA) {
                                CosmicStarfieldLayer(
                                    palette = palette,
                                    animated = true,
                                    modifier = Modifier.matchParentSize()
                                )
                            }
                            Row(
                                modifier = Modifier.padding(horizontal = 11.dp, vertical = 5.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(5.dp)
                            ) {
                                val textColor = if (usesDarkBackdrop) Color.White else palette.accent
                                Text(
                                    text = row.title,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = textColor,
                                    maxLines = 1
                                )
                                if (!effortText.isNullOrBlank()) {
                                    Text(
                                        text = "·",
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Bold,
                                        color = textColor.copy(alpha = 0.65f)
                                    )
                                    Text(
                                        text = effortText,
                                        fontSize = 11.5.sp,
                                        fontWeight = FontWeight.Bold,
                                        color = textColor,
                                        maxLines = 1
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // Left column: Single-column model picker with inline horizontal effort scrub
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
                        val dividerColor = colors.separator.copy(alpha = 0.45f)
                        val spacingPx = with(density) { leftRowSpacingDp.toPx() }
                        val groupDividerModifier = if (row.isGroupStart) {
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

                        InlineEffortModelRow(
                            index = index,
                            row = row,
                            state = state,
                            rowHeight = leftRowHeightDp,
                            isDark = isDark,
                            density = density,
                            haptic = haptic,
                            onSelectModel = onSelectModel,
                            modifier = groupDividerModifier
                        )
                    }
                }
            }

            // Right companion card: Consolidated Gemini & Claude quotas (5h + Weekly + pace cursor)
            if (showQuotaCard) {
                val quotaAcc = acc!!
                val rightWidthDp = state.rightQuotaWidthDp(density)
                val rightWidthPx = with(density) { rightWidthDp.toPx() }
                val hMarginPx = with(density) { ModelSlidePickerState.Layout.horizontalMargin.toPx() }
                val gapPx = with(density) { ModelSlidePickerState.Layout.gapBetweenColumns.toPx() }
                val topSafePx = with(density) { ModelSlidePickerState.Layout.topSafeMargin.toPx() }
                val maxRightX = if (state.containerSize.width > 0) {
                    max(hMarginPx, state.containerSize.width - rightWidthPx - hMarginPx)
                } else {
                    leftRect.right + gapPx
                }
                val rightX = min(leftRect.right + gapPx, maxRightX)
                val estH = if (rightQuotaHeightPx > 0) rightQuotaHeightPx.toFloat() else with(density) { 150.dp.toPx() }
                val rightY = max(topSafePx, leftRect.bottom - estH)

                RightQuotaCompanionCard(
                    account = quotaAcc,
                    highlightedPool = state.highlightedQuotaPool(),
                    hasHoverHighlight = state.hoveredRowIndex != null && state.highlightedQuotaPool() != null,
                    showPaceCursor = paceCursorEnabled,
                    widthDp = rightWidthDp,
                    isDark = isDark,
                    modifier = Modifier
                        .offset { IntOffset(rightX.roundToInt(), rightY.roundToInt()) }
                        .onSizeChanged { rightQuotaHeightPx = it.height }
                )
            }
        }
    }
}

@Composable
private fun InlineEffortModelRow(
    index: Int,
    row: ModelPickerRow,
    state: ModelSlidePickerState,
    rowHeight: Dp,
    isDark: Boolean,
    density: Density,
    haptic: HapticFeedback,
    onSelectModel: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    val accent = providerAccentColor(row.providerKind)
    val palette = remember(row.providerKind, isDark, accent) {
        EffortStylePalette.forProvider(row.providerKind, isDark, accent)
    }
    val isHovered = state.hoveredRowIndex == index
    val activeVIdx = state.activeVariantIndex(row)
    val isCurrentActive = activeVIdx != null

    val displayVariantIdx: Int? = when {
        isHovered -> (state.hoveredVariantIndex ?: row.defaultVariantIndex)
            .coerceIn(0, row.variantsTopToBottom.lastIndex.coerceAtLeast(0))
        isCurrentActive -> activeVIdx
        else -> null
    }
    val displayVariant = displayVariantIdx?.let { row.variantsTopToBottom.getOrNull(it) }
    val effortRank = displayVariant?.effortRank
    val count = row.variantsTopToBottom.size
    val targetFillFraction = when {
        !isHovered && !isCurrentActive -> 0f
        !row.hasInlineEffort -> 1f
        else -> EffortStylePalette.fillFraction(displayVariantIdx ?: 0, count)
    }
    val fillFraction by animateFloatAsState(
        targetValue = targetFillFraction,
        animationSpec = spring(dampingRatio = 0.88f, stiffness = Spring.StiffnessMedium),
        label = "effort_fill"
    )

    val isRichDarkFill = isHovered && (effortRank == ModelEffortRank.HIGH || effortRank == ModelEffortRank.EXTRA)
    val isRestingCosmic = !isHovered && isCurrentActive && effortRank == ModelEffortRank.EXTRA
    val titleColor = when {
        isRichDarkFill || isRestingCosmic -> Color.White
        isHovered && effortRank == ModelEffortRank.MEDIUM && isDark -> Color.White
        else -> palette.accent
    }

    val effortBadgeText = if (row.hasInlineEffort && displayVariant != null) {
        if (row.usesEffortLabels) (effortRank?.shortLabel ?: displayVariant.effortDisplayLabel) else displayVariant.label
    } else null

    val borderColor = when {
        isHovered && effortRank == ModelEffortRank.EXTRA -> palette.glowColor.copy(alpha = 0.88f)
        isHovered && effortRank == ModelEffortRank.HIGH -> palette.accent.copy(alpha = 0.85f)
        isHovered -> palette.accent.copy(alpha = 0.70f)
        isCurrentActive && effortRank == ModelEffortRank.EXTRA -> palette.glowColor.copy(alpha = 0.62f)
        isCurrentActive -> palette.accent.copy(alpha = 0.40f)
        else -> Color.Transparent
    }
    val borderWidth = when {
        isHovered && effortRank == ModelEffortRank.EXTRA -> 1.35.dp
        isHovered -> 1.15.dp
        isCurrentActive -> 0.9.dp
        else -> 0.dp
    }
    val shape = RoundedCornerShape(9.dp)
    val hysteresisPx = with(density) { ModelSlidePickerState.Layout.tierBoundaryHysteresis.toPx() }
    var rowWidthPx by remember { mutableIntStateOf(0) }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(rowHeight)
            .onSizeChanged { rowWidthPx = it.width }
            .then(
                if (isHovered && effortRank == ModelEffortRank.EXTRA) {
                    Modifier.shadow(6.dp, shape, ambientColor = palette.glowColor, spotColor = palette.glowColor)
                } else {
                    Modifier
                }
            )
            .clip(shape)
            .background(
                when {
                    isHovered && row.hasInlineEffort -> palette.accent.copy(alpha = if (isDark) 0.11f else 0.07f)
                    isCurrentActive && !isHovered && !isRestingCosmic -> palette.accent.copy(alpha = if (isDark) 0.13f else 0.08f)
                    else -> Color.Transparent
                }
            )
            .then(
                if (borderColor != Color.Transparent) Modifier.border(borderWidth, borderColor, shape) else Modifier
            )
            .pointerInput(state.isInteractiveTapMode, index, row.id, rowWidthPx) {
                if (!state.isInteractiveTapMode) return@pointerInput
                detectTapGestures { offset ->
                    if (!row.hasInlineEffort) {
                        row.variantsTopToBottom.firstOrNull()?.let { only ->
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            onSelectModel(only.id)
                            state.dismiss()
                        }
                        return@detectTapGestures
                    }
                    if (state.hoveredRowIndex == index) {
                        val w = max(1f, rowWidthPx.toFloat())
                        val chosenIdx = state.variantIndexForX(index, offset.x, w, null, 0f)
                        row.variantsTopToBottom.getOrNull(chosenIdx)?.let { chosen ->
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            onSelectModel(chosen.id)
                            state.dismiss()
                        }
                    } else {
                        val initialIdx = activeVIdx ?: row.defaultVariantIndex
                        state.hoveredRowIndex = index
                        state.hoveredVariantIndex = initialIdx
                        state.triggerHaptic(row, initialIdx, haptic)
                    }
                }
            }
            .pointerInput(state.isInteractiveTapMode, index, row.id, rowWidthPx) {
                if (!state.isInteractiveTapMode || !row.hasInlineEffort) return@pointerInput
                var lastX = 0f
                detectHorizontalDragGestures(
                    onDragStart = { startOffset ->
                        lastX = startOffset.x
                        val w = max(1f, rowWidthPx.toFloat())
                        val vIdx = state.variantIndexForX(index, lastX, w, null, hysteresisPx)
                        val changed = state.hoveredRowIndex != index || state.hoveredVariantIndex != vIdx
                        state.hoveredRowIndex = index
                        state.hoveredVariantIndex = vIdx
                        if (changed) state.triggerHaptic(row, vIdx, haptic)
                    },
                    onHorizontalDrag = { change, dragAmount ->
                        change.consume()
                        lastX += dragAmount
                        val w = max(1f, rowWidthPx.toFloat())
                        val vIdx = state.variantIndexForX(
                            index,
                            lastX,
                            w,
                            if (state.hoveredRowIndex == index) state.hoveredVariantIndex else null,
                            hysteresisPx
                        )
                        val changed = state.hoveredRowIndex != index || state.hoveredVariantIndex != vIdx
                        state.hoveredRowIndex = index
                        state.hoveredVariantIndex = vIdx
                        if (changed) state.triggerHaptic(row, vIdx, haptic)
                    },
                    onDragEnd = {
                        val chosenIdx = state.hoveredVariantIndex ?: row.defaultVariantIndex
                        row.variantsTopToBottom.getOrNull(chosenIdx)?.let { chosen ->
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            onSelectModel(chosen.id)
                            state.dismiss()
                        }
                    }
                )
            }
    ) {
        // Effort progress fill layer
        if (fillFraction > 0.01f) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(fillFraction.coerceIn(0f, 1f))
                    .fillMaxHeight()
                    .clip(shape)
            ) {
                when {
                    !row.hasInlineEffort -> {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(
                                    palette.accent.copy(
                                        alpha = if (isHovered) (if (isDark) 0.28f else 0.20f) else (if (isDark) 0.14f else 0.10f)
                                    )
                                )
                        )
                    }
                    !isHovered && isCurrentActive -> {
                        when (effortRank) {
                            ModelEffortRank.EXTRA -> CosmicStarfieldLayer(
                                palette = palette,
                                animated = false,
                                isSubtleResting = true,
                                modifier = Modifier.fillMaxSize()
                            )
                            ModelEffortRank.HIGH -> Box(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .background(Brush.horizontalGradient(palette.highColors))
                                    .alpha(if (isDark) 0.34f else 0.24f)
                            )
                            ModelEffortRank.MEDIUM -> Box(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .background(Brush.horizontalGradient(palette.medColors))
                                    .alpha(0.65f)
                            )
                            else -> Box(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .background(palette.lowFill.copy(alpha = palette.lowFill.alpha * 0.75f))
                            )
                        }
                    }
                    effortRank == ModelEffortRank.EXTRA -> {
                        CosmicStarfieldLayer(
                            palette = palette,
                            animated = true,
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                    effortRank == ModelEffortRank.HIGH -> {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(Brush.horizontalGradient(palette.highColors))
                        )
                    }
                    effortRank == ModelEffortRank.MEDIUM -> {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(Brush.horizontalGradient(palette.medColors))
                        )
                    }
                    else -> {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(palette.lowFill)
                        )
                    }
                }

                // Sleek luminous edge cap when partially filled
                if (isHovered && row.hasInlineEffort && fillFraction < 0.99f) {
                    Box(
                        modifier = Modifier
                            .align(Alignment.CenterEnd)
                            .padding(end = 1.5.dp, top = 5.dp, bottom = 5.dp)
                            .width(2.5.dp)
                            .fillMaxHeight()
                            .clip(RoundedCornerShape(1.5.dp))
                            .background(
                                if (effortRank == ModelEffortRank.HIGH) Color.White.copy(alpha = 0.88f)
                                else palette.accent.copy(alpha = 0.85f)
                            )
                    )
                }
            }
        }

        // Foreground row content: checkmark + model title + effort badge or idle dots
        Row(
            modifier = Modifier
                .fillMaxSize()
                .padding(
                    start = 10.dp,
                    end = 10.dp,
                    bottom = if (isHovered && row.hasInlineEffort) 3.dp else 0.dp
                ),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            if (isCurrentActive) {
                Icon(
                    imageVector = Icons.Default.Check,
                    contentDescription = null,
                    tint = titleColor,
                    modifier = Modifier.size(12.5.dp)
                )
            }
            val rowTitle = if (state.isCompactWidth(density)) (row.compactTitle ?: row.title) else row.title
            Text(
                text = rowTitle,
                fontSize = if (rowHeight < 29.dp) 12.5.sp else 13.5.sp,
                fontWeight = if (isHovered || isCurrentActive) FontWeight.SemiBold else FontWeight.Medium,
                color = titleColor,
                style = if (isRichDarkFill || isRestingCosmic) {
                    TextStyle(shadow = Shadow(Color.Black.copy(alpha = 0.28f), Offset(0f, 1f), 2f))
                } else {
                    TextStyle.Default
                },
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )

            if (effortBadgeText != null) {
                val badgeOnFill = targetFillFraction >= 0.96f || isRestingCosmic
                val whiteBadge = badgeOnFill && (isRichDarkFill || isRestingCosmic)
                val badgeTextColor = when {
                    whiteBadge -> Color.White
                    isHovered && effortRank == ModelEffortRank.MEDIUM && isDark && targetFillFraction >= 0.96f -> Color.White
                    else -> palette.accent
                }
                val badgeBg = if (whiteBadge) {
                    Color.White.copy(alpha = 0.22f)
                } else {
                    palette.accent.copy(alpha = if (isDark) 0.22f else 0.14f)
                }
                Text(
                    text = effortBadgeText,
                    fontSize = 10.5.sp,
                    fontWeight = if (effortRank == ModelEffortRank.EXTRA) FontWeight.Bold else FontWeight.SemiBold,
                    color = badgeTextColor,
                    maxLines = 1,
                    modifier = Modifier
                        .clip(RoundedCornerShape(50))
                        .background(badgeBg)
                        .padding(horizontal = 6.dp, vertical = 1.dp)
                )
            } else if (row.hasInlineEffort) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(2.5.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    repeat(min(count, 4)) {
                        Box(
                            modifier = Modifier
                                .size(3.dp)
                                .clip(CircleShape)
                                .background(palette.accent.copy(alpha = 0.32f))
                        )
                    }
                }
            }
        }

        // Bottom micro-node rail when hovered on a multi-tier row
        if (isHovered && row.hasInlineEffort) {
            val activeStop = displayVariantIdx ?: 0
            Row(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .padding(horizontal = 6.dp, vertical = 2.5.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                for (stopIdx in 0 until count) {
                    val isSelectedStop = stopIdx == activeStop
                    val isReached = stopIdx <= activeStop
                    val dotColor = if (isRichDarkFill) {
                        when {
                            isSelectedStop -> Color.White
                            isReached -> Color.White.copy(alpha = 0.55f)
                            else -> palette.accent.copy(alpha = 0.35f)
                        }
                    } else {
                        when {
                            isSelectedStop -> palette.accent
                            isReached -> palette.accent.copy(alpha = 0.55f)
                            else -> palette.accent.copy(alpha = 0.24f)
                        }
                    }
                    Box(
                        modifier = Modifier.weight(1f),
                        contentAlignment = Alignment.Center
                    ) {
                        Box(
                            modifier = Modifier
                                .width(if (isSelectedStop) 12.dp else 4.dp)
                                .height(if (isSelectedStop) 3.dp else 2.5.dp)
                                .clip(CircleShape)
                                .background(dotColor)
                        )
                    }
                }
            }
        }
    }
}

// MARK: - Right Companion Quota Card (Gemini + Claude, 5h + Weekly + Pace Cursor)

@Composable
private fun RightQuotaCompanionCard(
    account: CockpitAccountQuota,
    highlightedPool: ModelQuotaPoolKind?,
    hasHoverHighlight: Boolean,
    showPaceCursor: Boolean,
    widthDp: Dp,
    isDark: Boolean,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val isFree = account.isFreeTier
    val shape = RoundedCornerShape(13.dp)

    Surface(
        modifier = modifier
            .width(widthDp)
            .border(1.dp, colors.border, shape),
        shape = shape,
        color = colors.surface.copy(alpha = 0.96f),
        shadowElevation = 10.dp,
        tonalElevation = 0.dp
    ) {
        Column(
            modifier = Modifier.padding(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            if (account.gemini5h != null || account.geminiWeekly != null) {
                QuotaPoolModule(
                    title = "Gemini",
                    accent = colors.modelProviderAccent("gemini"),
                    bucket5h = if (isFree) null else account.gemini5h,
                    bucketWeekly = account.geminiWeekly,
                    showPaceCursor = showPaceCursor,
                    isFocused = highlightedPool == ModelQuotaPoolKind.GEMINI,
                    isDimmed = hasHoverHighlight && highlightedPool != ModelQuotaPoolKind.GEMINI,
                    isDark = isDark
                )
            }
            if (account.claude5h != null || account.claudeWeekly != null) {
                QuotaPoolModule(
                    title = "Claude",
                    accent = colors.modelProviderAccent("claude"),
                    bucket5h = if (isFree) null else account.claude5h,
                    bucketWeekly = account.claudeWeekly,
                    showPaceCursor = showPaceCursor,
                    isFocused = highlightedPool == ModelQuotaPoolKind.CLAUDE,
                    isDimmed = hasHoverHighlight && highlightedPool != ModelQuotaPoolKind.CLAUDE,
                    isDark = isDark
                )
            }
        }
    }
}

@Composable
private fun QuotaPoolModule(
    title: String,
    accent: Color,
    bucket5h: CockpitQuotaBucket?,
    bucketWeekly: CockpitQuotaBucket?,
    showPaceCursor: Boolean,
    isFocused: Boolean,
    isDimmed: Boolean,
    isDark: Boolean
) {
    val colors = AntigravityTheme.colors
    val shape = RoundedCornerShape(9.dp)
    val bgColor = if (isFocused) {
        accent.copy(alpha = if (isDark) 0.16f else 0.10f)
    } else {
        colors.textPrimary.copy(alpha = if (isDark) 0.05f else 0.03f)
    }
    val borderColor = if (isFocused) {
        accent.copy(alpha = 0.68f)
    } else {
        colors.border.copy(alpha = 0.45f)
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .alpha(if (isDimmed) 0.48f else 1f)
            .clip(shape)
            .background(bgColor)
            .border(if (isFocused) 1.dp else 0.6.dp, borderColor, shape)
            .padding(horizontal = 7.dp, vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(5.5.dp)
                    .clip(CircleShape)
                    .background(accent)
            )
            Text(
                text = title,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                color = accent,
                maxLines = 1,
                modifier = Modifier.weight(1f)
            )
            val resetShort = remember(bucket5h?.resetFriendly) {
                compactResetLabel(bucket5h?.resetFriendly)
            }
            if (resetShort != null) {
                Text(
                    text = resetShort,
                    fontSize = 9.sp,
                    fontWeight = FontWeight.Medium,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary,
                    maxLines = 1
                )
            }
        }

        if (bucket5h != null) {
            QuotaMiniBarRow(
                label = "5h",
                bucket = bucket5h,
                accent = accent,
                paceInfo = null,
                isDark = isDark
            )
        }

        if (bucketWeekly != null) {
            val pace = remember(bucketWeekly, showPaceCursor) {
                if (showPaceCursor) bucketWeekly.calculateWeeklyPace(168.0) else null
            }
            QuotaMiniBarRow(
                label = "周".loc,
                bucket = bucketWeekly,
                accent = accent,
                paceInfo = pace,
                isDark = isDark
            )
        }
    }
}

@Composable
private fun QuotaMiniBarRow(
    label: String,
    bucket: CockpitQuotaBucket,
    accent: Color,
    paceInfo: WeeklyPaceInfo?,
    isDark: Boolean
) {
    val colors = AntigravityTheme.colors
    val pct = bucket.remainingPercent.takeIf { it.isFinite() }?.coerceIn(0.0, 100.0) ?: 0.0
    val isLow = pct < 20.0
    val barColor = if (isLow) Color(0xFFE5484D) else accent

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(2.5.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = label,
                fontSize = 9.5.sp,
                fontWeight = FontWeight.Medium,
                color = colors.textSecondary,
                maxLines = 1
            )
            Spacer(modifier = Modifier.weight(1f))
            Text(
                text = "${pct.roundToInt()}%",
                fontSize = 10.5.sp,
                fontWeight = FontWeight.SemiBold,
                fontFamily = FontFamily.Monospace,
                color = if (isLow) Color(0xFFE5484D) else colors.textPrimary,
                maxLines = 1
            )
        }

        BoxWithConstraints(
            modifier = Modifier
                .fillMaxWidth()
                .height(7.5.dp),
            contentAlignment = Alignment.CenterStart
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(4.5.dp)
                    .clip(CircleShape)
                    .background(colors.textPrimary.copy(alpha = if (isDark) 0.14f else 0.09f))
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth((pct / 100.0).toFloat().coerceIn(0f, 1f))
                        .fillMaxHeight()
                        .clip(CircleShape)
                        .background(barColor)
                )
            }

            if (paceInfo != null) {
                val expectedFrac = (paceInfo.expectedRemainingPercent / 100.0).toFloat().coerceIn(0f, 1f)
                val needleX = (maxWidth * expectedFrac - 0.75.dp).coerceIn(0.dp, (maxWidth - 1.5.dp).coerceAtLeast(0.dp))
                val needleColor = when {
                    paceInfo.isOverPacing -> Color(0xFFFF9F0A)
                    isDark -> Color.White.copy(alpha = 0.92f)
                    else -> colors.textPrimary.copy(alpha = 0.78f)
                }
                Box(
                    modifier = Modifier
                        .padding(start = needleX)
                        .size(width = 1.5.dp, height = 7.5.dp)
                        .clip(RoundedCornerShape(0.75.dp))
                        .background(needleColor)
                )
            }
        }
    }
}

private val COMPACT_RESET_REGEX = Regex("""(\d+)\s*(天|日|d|小时|小時|h|分钟|分鐘|分|m)""", RegexOption.IGNORE_CASE)

private fun compactResetLabel(raw: String?): String? {
    val text = raw?.trim().orEmpty()
    if (text.isEmpty()) return null
    val lower = text.lowercase()
    if (lower in setOf("已就绪", "就绪", "未知", "配额充足", "ready", "unknown")) return null
    val match = COMPACT_RESET_REGEX.find(text) ?: return null
    val num = match.groupValues[1]
    val rawUnit = match.groupValues[2].lowercase()
    val unit = when (rawUnit) {
        "天", "日", "d" -> "d"
        "小时", "小時", "h" -> "h"
        else -> "m"
    }
    return "$num$unit"
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
    val shape = RoundedCornerShape(13.dp)
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
