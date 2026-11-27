package com.antigravity.mobile.ui.components

import android.animation.ValueAnimator
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import com.antigravity.mobile.ui.util.loc
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelEffortRank
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.modelProviderAccent

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun QuickActionChips(
    activeModel: String,
    onToggleModel: () -> Unit,
    onSelectModel: (String) -> Unit = {},
    modelCatalog: ModelsResponse? = null,
    modelSlidePickerState: ModelSlidePickerState? = null,
    onAddImage: () -> Unit,
    onAddImageLongClick: (() -> Unit)? = null,
    suggestedResponses: List<String> = emptyList(),
    isPredictedSuggestion: Boolean = false,
    onSelectSuggestion: (String) -> Unit = {},
    onSendSuggestion: (String) -> Unit = {},
    onDismissSuggestion: (String) -> Unit = {},
    selectedSlashCommands: List<SlashCommandOption> = emptyList(),
    onRemoveSlashCommand: (SlashCommandOption) -> Unit = {},
    onCommitAndPush: () -> Unit,
    commitLabel: String = "Commit",
    showCommit: Boolean = true,
    canCommit: Boolean = false,
    commitSlidePickerState: CommitSlidePickerState? = null,
    onSelectCommitAction: (String) -> Unit = {},
    onShowChanges: () -> Unit,
    showChanges: Boolean = true,
    onShowWorkflowTemplates: (() -> Unit)? = null,
    workflowSlidePickerState: WorkflowSlidePickerState? = null,
    workflowTemplates: List<WorkflowTemplateRecord> = emptyList(),
    onSelectWorkflowPrompt: (String) -> Unit = {},
    onOpenWorkflowSetup: () -> Unit = {},
    showContinue: Boolean,
    onContinue: () -> Unit,
    showProceed: Boolean,
    onProceed: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val compactName = remember(activeModel, modelCatalog) {
        ModelDefaults.compactDisplayName(activeModel, modelCatalog?.models.orEmpty())
    }
    val providerKind = remember(activeModel) {
        ModelDefaults.providerKind(activeModel)
    }
    val chipTextColor = colors.modelProviderAccent(providerKind)
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
    val hapticFeedback = LocalHapticFeedback.current
    val density = LocalDensity.current
    var chipBoundsInRoot by remember { mutableStateOf(Rect.Zero) }
    var workflowChipBoundsInRoot by remember { mutableStateOf(Rect.Zero) }
    var commitChipBoundsInRoot by remember { mutableStateOf(Rect.Zero) }
    var rowViewportWidthDp by remember { mutableStateOf(0.dp) }
    val latestWorkflowTemplates by rememberUpdatedState(workflowTemplates)
    val maxSuggestionBubbleWidth = if (rowViewportWidthDp > 0.dp) {
        // 34dp (+) + 8dp 间距 + 6dp 右上角小叉留白；优先单行向右延伸挤出模型胶囊，占满可视宽度后再折行
        (rowViewportWidthDp - 34.dp - 8.dp - 6.dp).coerceAtLeast(180.dp)
    } else {
        300.dp
    }

    val context = androidx.compose.ui.platform.LocalContext.current
    val tipsPrefs = remember { context.getSharedPreferences("antigravity_tips", android.content.Context.MODE_PRIVATE) }
    var showLongPressTip by remember { mutableStateOf(false) }

    val dismissTip: () -> Unit = {
        if (showLongPressTip) {
            showLongPressTip = false
            tipsPrefs.edit().putBoolean("has_seen_model_long_press_tooltip", true).apply()
        }
    }

    LaunchedEffect(Unit) {
        val hasSeen = tipsPrefs.getBoolean("has_seen_model_long_press_tooltip", false)
        if (!hasSeen) {
            kotlinx.coroutines.delay(600)
            showLongPressTip = true
            kotlinx.coroutines.delay(5000)
            dismissTip()
        }
    }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .onSizeChanged { size ->
                val measured = with(density) { size.width.toDp() }
                if (measured > 0.dp && kotlin.math.abs((measured - rowViewportWidthDp).value) > 0.5f) {
                    rowViewportWidthDp = measured
                }
            }
            .horizontalScroll(rememberScrollState())
            .padding(top = 8.dp, bottom = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.Top
    ) {
        // 1. Add Image ➕ (long press for @ Mention)
        Box(
            modifier = Modifier
                .height(32.dp)
                .width(34.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(colors.surface)
                .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                .combinedClickable(
                    onClick = {
                        haptic.light()
                        onAddImage()
                    },
                    onLongClick = {
                        haptic.medium()
                        onAddImageLongClick?.invoke()
                    }
                ),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.Add,
                contentDescription = "Add Media",
                tint = colors.accentIndigo,
                modifier = Modifier.size(16.dp)
            )
        }

        // 1b. 建议回复气泡（短按填入输入框，长按 1.5 秒带进度条防呆直发；优先单行向右延伸挤出模型胶囊，占满可视宽度后再折行）
        suggestedResponses.forEach { suggestion ->
            SuggestedReplyChip(
                text = suggestion,
                isPredicted = isPredictedSuggestion,
                maxBubbleWidth = maxSuggestionBubbleWidth,
                onTapFill = {
                    onSelectSuggestion(suggestion)
                },
                onHoldSend = {
                    onSendSuggestion(suggestion)
                },
                onDismiss = {
                    haptic.light()
                    onDismissSuggestion(suggestion)
                }
            )
        }

        // 2. Gemini / Claude / Orchestrator Model Switcher (Tap to cycle, Long-press + slide up/right for iOS-style picker)
        val isXHighActive = remember(activeModel, modelCatalog) {
            ModelDefaults.effortRank(activeModel, modelCatalog?.models.orEmpty()) == ModelEffortRank.EXTRA
        }
        val (xhighCosmicStart, xhighGlowColor, xhighRimColors) = remember(providerKind) {
            when (providerKind.lowercase()) {
                "gemini" -> Triple(
                    Color(0xFF13174A),
                    Color(0xFF38BDF8),
                    listOf(Color(0xFF38BDF8), Color(0xFF6366F1), Color(0xFF2563EB), Color(0xFF38BDF8))
                )
                "claude" -> Triple(
                    Color(0xFF451A03),
                    Color(0xFFFB923C),
                    listOf(Color(0xFFFBBF24), Color(0xFFF97316), Color(0xFFC2410C), Color(0xFFFBBF24))
                )
                "orchestrator" -> Triple(
                    Color(0xFF0B101C),
                    Color(0xFFCBD5E1),
                    listOf(Color(0xFFE2E8F0), Color(0xFF94A3B8), Color(0xFF475569), Color(0xFFE2E8F0))
                )
                else -> Triple(
                    Color(0xFF042F2E),
                    Color(0xFF2DD4BF),
                    listOf(Color(0xFF5EEAD4), Color(0xFF14B8A6), Color(0xFF0F766E), Color(0xFF5EEAD4))
                )
            }
        }
        val xhighRimAngle: Float = if (isXHighActive && ValueAnimator.areAnimatorsEnabled()) {
            val transition = rememberInfiniteTransition(label = "chip_xhigh_rim")
            val a by transition.animateFloat(
                initialValue = 0f,
                targetValue = 360f,
                animationSpec = infiniteRepeatable(
                    animation = tween(durationMillis = 7200, easing = LinearEasing),
                    repeatMode = RepeatMode.Restart
                ),
                label = "chip_rim_angle"
            )
            a
        } else {
            35f
        }
        val chipShape = RoundedCornerShape(10.dp)
        Box(contentAlignment = Alignment.TopStart) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .onGloballyPositioned { coords ->
                        val rect = coords.boundsInRoot()
                        chipBoundsInRoot = rect
                        modelSlidePickerState?.anchorRect = rect
                    }
                    .then(
                        if (isXHighActive) {
                            Modifier.shadow(
                                elevation = 4.dp,
                                shape = chipShape,
                                ambientColor = xhighGlowColor,
                                spotColor = xhighGlowColor
                            )
                        } else {
                            Modifier
                        }
                    )
                    .clip(chipShape)
                    .then(
                        if (isXHighActive) {
                            // Pre-composite the translucent tint onto the opaque surface color so
                            // Android's RenderNode elevation shadow (Modifier.shadow) cannot bleed
                            // through the interior and expose its opaque-occluder cutout rectangle.
                            Modifier.background(
                                Brush.linearGradient(
                                    colors = listOf(
                                        xhighCosmicStart
                                            .copy(alpha = if (colors.isDark) 0.34f else 0.14f)
                                            .compositeOver(colors.surface),
                                        xhighGlowColor
                                            .copy(alpha = if (colors.isDark) 0.22f else 0.12f)
                                            .compositeOver(colors.surface)
                                    )
                                )
                            )
                        } else {
                            Modifier.background(chipTextColor.copy(alpha = 0.12f))
                        }
                    )
                    .then(
                        if (isXHighActive) {
                            Modifier.drawWithContent {
                                drawContent()
                                val strokePx = 1.35.dp.toPx()
                                val radiusPx = 10.dp.toPx()
                                val innerRadius = (radiusPx - strokePx / 2f).coerceAtLeast(0f)
                                val rad = xhighRimAngle * (PI / 180.0)
                                val dx = (cos(rad) * size.width * 0.65).toFloat()
                                val dy = (sin(rad) * size.height * 1.4).toFloat()
                                val rimBrush = Brush.linearGradient(
                                    colors = xhighRimColors,
                                    start = Offset(center.x - dx, center.y - dy),
                                    end = Offset(center.x + dx, center.y + dy)
                                )
                                drawRoundRect(
                                    brush = rimBrush,
                                    topLeft = Offset(strokePx / 2f, strokePx / 2f),
                                    size = androidx.compose.ui.geometry.Size(
                                        size.width - strokePx,
                                        size.height - strokePx
                                    ),
                                    cornerRadius = CornerRadius(innerRadius, innerRadius),
                                    style = Stroke(width = strokePx)
                                )
                            }
                        } else {
                            Modifier.border(
                                1.dp,
                                chipTextColor.copy(alpha = 0.35f),
                                chipShape
                            )
                        }
                    )
                    .clickable {
                        dismissTip()
                        haptic.light()
                        if (modelSlidePickerState?.isPresented == true) {
                            modelSlidePickerState.dismiss()
                        } else {
                            onToggleModel()
                        }
                    }
                    .pointerInput(activeModel, modelCatalog, modelSlidePickerState) {
                        if (modelSlidePickerState == null) return@pointerInput
                        detectDragGesturesAfterLongPress(
                            onDragStart = {
                                dismissTip()
                                haptic.medium()
                                workflowSlidePickerState?.dismiss()
                                modelSlidePickerState.open(
                                    activeModel = activeModel,
                                    options = modelCatalog?.models.orEmpty()
                                )
                            },
                            onDrag = { change, _ ->
                                change.consume()
                                val rootPoint = Offset(
                                    x = chipBoundsInRoot.left + change.position.x,
                                    y = chipBoundsInRoot.top + change.position.y
                                )
                                modelSlidePickerState.updateDrag(rootPoint, density, hapticFeedback)
                            },
                            onDragEnd = {
                                modelSlidePickerState.endDrag(onSelect = onSelectModel)
                            },
                            onDragCancel = {
                                modelSlidePickerState.dismiss()
                            }
                        )
                    }
                    .padding(horizontal = 11.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = compactName,
                    color = chipTextColor,
                    fontSize = 13.sp,
                    fontWeight = if (isXHighActive) FontWeight.SemiBold else FontWeight.Medium
                )
            }

            if (showLongPressTip) {
                val gapPx = with(density) { 8.dp.roundToPx() }
                val edgePaddingPx = with(density) { 12.dp.roundToPx() }
                val tooltipPositionProvider = remember(gapPx, edgePaddingPx) {
                    object : PopupPositionProvider {
                        override fun calculatePosition(
                            anchorBounds: IntRect,
                            windowSize: IntSize,
                            layoutDirection: LayoutDirection,
                            popupContentSize: IntSize
                        ): IntOffset {
                            val maxX = (windowSize.width - popupContentSize.width - edgePaddingPx)
                                .coerceAtLeast(edgePaddingPx)
                            val x = anchorBounds.left.coerceIn(edgePaddingPx, maxX)
                            val y = (anchorBounds.top - popupContentSize.height - gapPx)
                                .coerceAtLeast(edgePaddingPx)
                            return IntOffset(x, y)
                        }
                    }
                }
                Popup(
                    popupPositionProvider = tooltipPositionProvider,
                    properties = PopupProperties(
                        focusable = false,
                        dismissOnBackPress = false,
                        dismissOnClickOutside = false
                    )
                ) {
                    Box(
                        modifier = Modifier
                            .shadow(4.dp, RoundedCornerShape(8.dp))
                            .clip(RoundedCornerShape(8.dp))
                            .background(Color(0xE6222222))
                            .clickable { dismissTip() }
                            .padding(horizontal = 9.dp, vertical = 6.dp)
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.ArrowUpward,
                                contentDescription = null,
                                tint = Color.White,
                                modifier = Modifier.size(13.dp)
                            )
                            Text(
                                text = "长按上滑可切换更多模型".loc,
                                color = Color.White,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Medium,
                                maxLines = 1,
                                softWrap = false
                            )
                        }
                    }
                }
            }
        }

        // 2a. 已选中的斜杠命令胶囊（按选择顺序排在模型与 Workflow 之间，样式同高对齐，右上角带小叉，轻点移除单个）
        selectedSlashCommands.forEach { command ->
            SlashCommandChip(
                command = command,
                onClear = {
                    haptic.light()
                    onRemoveSlashCommand(command)
                }
            )
        }

        // 2b. Workflow: 单击打开满屏模板页，长按上滑快捷输入（纯文字无 logo，视觉与 Commit/Changes 对齐）
        if (onShowWorkflowTemplates != null) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .onGloballyPositioned { coords ->
                        val rect = coords.boundsInRoot()
                        workflowChipBoundsInRoot = rect
                        workflowSlidePickerState?.anchorRect = rect
                    }
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(
                        1.dp,
                        colors.border,
                        RoundedCornerShape(10.dp)
                    )
                    .clickable {
                        haptic.light()
                        if (workflowSlidePickerState?.isPresented == true) {
                            workflowSlidePickerState.dismiss()
                        } else {
                            onShowWorkflowTemplates()
                        }
                    }
                    .pointerInput(workflowSlidePickerState) {
                        if (workflowSlidePickerState == null) return@pointerInput
                        detectDragGesturesAfterLongPress(
                            onDragStart = {
                                haptic.medium()
                                modelSlidePickerState?.dismiss()
                                workflowSlidePickerState.open(latestWorkflowTemplates)
                            },
                            onDrag = { change, _ ->
                                change.consume()
                                val rootPoint = Offset(
                                    x = workflowChipBoundsInRoot.left + change.position.x,
                                    y = workflowChipBoundsInRoot.top + change.position.y
                                )
                                workflowSlidePickerState.updateDrag(rootPoint, density, hapticFeedback)
                            },
                            onDragEnd = {
                                workflowSlidePickerState.endDrag(
                                    onSelectTemplate = onSelectWorkflowPrompt,
                                    onOpenSetup = onOpenWorkflowSetup
                                )
                            },
                            onDragCancel = {
                                workflowSlidePickerState.dismiss()
                            }
                        )
                    }
                    .padding(horizontal = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "Workflow",
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        // 3. Commit：打开 Git 提交浮窗（长按上滑快捷填充 Commit 命令，仅在有工作区变动时可上滑）
        if (showCommit) {
            val commitModifier = Modifier
                .height(32.dp)
                .onGloballyPositioned { coords ->
                    val rect = coords.boundsInRoot()
                    commitChipBoundsInRoot = rect
                    commitSlidePickerState?.anchorRect = rect
                }
                .clip(RoundedCornerShape(10.dp))
                .background(colors.surface)
                .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                .clickable {
                    haptic.light()
                    if (commitSlidePickerState?.isPresented == true) {
                        commitSlidePickerState.dismiss()
                    } else {
                        onCommitAndPush()
                    }
                }
                .then(
                    if (canCommit && commitSlidePickerState != null) {
                        Modifier.pointerInput(canCommit, commitSlidePickerState) {
                            detectDragGesturesAfterLongPress(
                                onDragStart = {
                                    haptic.medium()
                                    modelSlidePickerState?.dismiss()
                                    workflowSlidePickerState?.dismiss()
                                    commitSlidePickerState.open()
                                },
                                onDrag = { change, _ ->
                                    change.consume()
                                    val rootPoint = Offset(
                                        x = commitChipBoundsInRoot.left + change.position.x,
                                        y = commitChipBoundsInRoot.top + change.position.y
                                    )
                                    commitSlidePickerState.updateDrag(rootPoint, density, hapticFeedback)
                                },
                                onDragEnd = {
                                    commitSlidePickerState.endDrag(onSelect = onSelectCommitAction)
                                },
                                onDragCancel = {
                                    commitSlidePickerState.dismiss()
                                }
                            )
                        }
                    } else {
                        Modifier
                    }
                )
                .padding(horizontal = 14.dp)

            Box(
                modifier = commitModifier,
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = commitLabel,
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        // 3b. Changes：本会话累计改动的 diff（当前 Antigravity 缺少所需接口时隐藏）
        if (showChanges) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                    .clickable {
                        haptic.light()
                        onShowChanges()
                    }
                    .padding(horizontal = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "Changes",
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        // 4. Continue Button (when error occurred)
        if (showContinue) {
            Row(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.accentBlue)
                    .clickable {
                        haptic.medium()
                        onContinue()
                    }
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "Continue",
                    tint = Color.White,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "Continue",
                    color = Color.White,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }
        }

        // 5. Proceed Button (when plan ready)
        if (showProceed) {
            Row(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.accentBlue)
                    .clickable {
                        haptic.heavy()
                        onProceed()
                    }
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "Proceed",
                    tint = Color.White,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "Proceed",
                    color = Color.White,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }
        }
    }
}

/** 建议回复气泡按钮：优先沿首行向右延伸（可挤出模型胶囊），占满可视宽度后再折成多行完整展示，右上角带小叉；短按填入输入框，长按 1 秒带实时进度条防呆直发。 */
@Composable
private fun SuggestedReplyChip(
    text: String,
    isPredicted: Boolean,
    maxBubbleWidth: Dp,
    onTapFill: () -> Unit,
    onHoldSend: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
    val shape = RoundedCornerShape(10.dp)
    val holdProgress = remember { Animatable(0f) }
    var isHoldEngaged by remember { mutableStateOf(false) }
    val latestOnTapFill by rememberUpdatedState(onTapFill)
    val latestOnHoldSend by rememberUpdatedState(onHoldSend)
    val scope = rememberCoroutineScope()

    Box(
        modifier = modifier.padding(end = 4.dp)
    ) {
        Row(
            modifier = Modifier
                .defaultMinSize(minHeight = 32.dp)
                .widthIn(max = maxBubbleWidth)
                .graphicsLayer {
                    val s = if (isHoldEngaged) 0.98f else 1f
                    scaleX = s
                    scaleY = s
                }
                .clip(shape)
                .background(colors.accentIndigo.copy(alpha = 0.12f))
                .drawBehind {
                    val p = holdProgress.value
                    if (p > 0.001f) {
                        val w = size.width * p
                        drawRect(
                            color = colors.accentIndigo.copy(alpha = 0.22f),
                            topLeft = Offset.Zero,
                            size = androidx.compose.ui.geometry.Size(w, size.height)
                        )
                        val barH = 2.5.dp.toPx()
                        drawRect(
                            color = colors.accentIndigo,
                            topLeft = Offset(0f, size.height - barH),
                            size = androidx.compose.ui.geometry.Size(w, barH)
                        )
                    }
                }
                .border(
                    width = if (isHoldEngaged) 1.25.dp else 1.dp,
                    color = colors.accentIndigo.copy(alpha = if (isHoldEngaged) 0.78f else 0.35f),
                    shape = shape
                )
                .pointerInput(text) {
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        val touchSlop = viewConfiguration.touchSlop
                        var cancelled = false
                        var completed = false
                        var engaged = false

                        val holdJob = scope.launch {
                            delay(180L)
                            engaged = true
                            isHoldEngaged = true
                            haptic.light()
                            holdProgress.snapTo(0f)
                            holdProgress.animateTo(
                                targetValue = 1f,
                                animationSpec = tween(durationMillis = 1320, easing = LinearEasing)
                            )
                            completed = true
                            isHoldEngaged = false
                            haptic.medium()
                            latestOnHoldSend()
                            holdProgress.snapTo(0f)
                        }

                        while (true) {
                            val event = awaitPointerEvent()
                            val change = event.changes.firstOrNull { it.id == down.id }
                            if (change == null || !change.pressed) {
                                break
                            }
                            if (change.isConsumed || (change.position - down.position).getDistance() > touchSlop) {
                                cancelled = true
                                break
                            }
                            if (completed) {
                                change.consume()
                                break
                            }
                        }

                        holdJob.cancel()
                        if (!completed) {
                            isHoldEngaged = false
                            scope.launch {
                                holdProgress.animateTo(0f, animationSpec = tween(durationMillis = 160))
                            }
                            if (!cancelled && !engaged) {
                                haptic.light()
                                latestOnTapFill()
                            }
                        }
                    }
                }
                .padding(start = 11.dp, end = 14.dp, top = 7.dp, bottom = 7.dp),
            verticalAlignment = Alignment.Top,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            if (isPredicted) {
                Icon(
                    imageVector = Icons.Default.AutoAwesome,
                    contentDescription = null,
                    tint = colors.accentIndigo.copy(alpha = 0.95f),
                    modifier = Modifier
                        .padding(top = 2.5.dp)
                        .size(13.dp)
                )
            }
            Text(
                text = text,
                color = colors.accentIndigo,
                fontSize = 13.sp,
                lineHeight = 18.sp,
                fontWeight = FontWeight.Medium
            )
        }

        Box(
            modifier = Modifier
                .size(20.dp)
                .align(Alignment.TopEnd)
                .offset(x = 6.dp, y = (-6).dp)
                .clip(CircleShape)
                .clickable(onClick = onDismiss),
            contentAlignment = Alignment.Center
        ) {
            Box(
                modifier = Modifier
                    .size(16.dp)
                    .clip(CircleShape)
                    .background(colors.surface),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(14.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo.copy(alpha = 0.85f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = when (com.antigravity.mobile.ui.util.L10n.currentLang) {
                            com.antigravity.mobile.ui.util.L10n.AppLang.EN -> "Dismiss suggestion"
                            com.antigravity.mobile.ui.util.L10n.AppLang.JA -> "提案を閉じる"
                            com.antigravity.mobile.ui.util.L10n.AppLang.ZH -> "关闭建议"
                        },
                        tint = Color.White,
                        modifier = Modifier.size(10.dp)
                    )
                }
            }
        }
    }
}
