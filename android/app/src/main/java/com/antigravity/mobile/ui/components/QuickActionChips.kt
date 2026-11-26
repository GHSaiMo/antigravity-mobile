package com.antigravity.mobile.ui.components

import android.animation.ValueAnimator
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
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
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
    val latestWorkflowTemplates by rememberUpdatedState(workflowTemplates)

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
            .horizontalScroll(rememberScrollState())
            .padding(top = 6.dp, bottom = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically
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

        // 2. Gemini / Claude / Orchestrator Model Switcher (Tap to cycle, Long-press + slide up & horizontal scrub)
        val isXHighActive = remember(activeModel, modelCatalog) {
            ModelDefaults.effortRank(activeModel, modelCatalog?.models.orEmpty()) == ModelEffortRank.EXTRA
        }
        val effortPalette = remember(providerKind, colors.isDark, chipTextColor) {
            EffortStylePalette.forProvider(providerKind, colors.isDark, chipTextColor)
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
                                ambientColor = effortPalette.glowColor,
                                spotColor = effortPalette.glowColor
                            )
                        } else {
                            Modifier
                        }
                    )
                    .clip(chipShape)
                    .then(
                        if (isXHighActive) {
                            Modifier.background(
                                Brush.linearGradient(
                                    colors = listOf(
                                        effortPalette.cosmicColors.first().copy(alpha = if (colors.isDark) 0.34f else 0.14f),
                                        effortPalette.glowColor.copy(alpha = if (colors.isDark) 0.22f else 0.12f)
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
                                val rad = xhighRimAngle * (PI / 180.0)
                                val dx = (cos(rad) * size.width * 0.65).toFloat()
                                val dy = (sin(rad) * size.height * 1.4).toFloat()
                                val rimBrush = Brush.linearGradient(
                                    colors = effortPalette.chipRimColors,
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
                                    cornerRadius = CornerRadius(radiusPx, radiusPx),
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
