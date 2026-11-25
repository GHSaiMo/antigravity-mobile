package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.CheckCircleOutline
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Extension
import androidx.compose.material.icons.filled.PauseCircleOutline
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.StepPlanItem
import com.antigravity.mobile.data.model.ThinkingBlockItem
import com.antigravity.mobile.data.model.ToolCallItem
import com.antigravity.mobile.data.model.ToolDetailItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.ToolLocalization
import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.OffsetDateTime
import kotlin.math.PI
import kotlin.math.cos

/**
 * 2×2 pulsing 4-dot grid spinner matching Antigravity desktop `$vc`.
 */
@Composable
fun StepPlanDotSpinner(
    color: Color,
    size: Dp = 15.dp,
    modifier: Modifier = Modifier
) {
    val infiniteTransition = rememberInfiniteTransition(label = "stepPlanDots")
    val phase by infiniteTransition.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 1400, easing = LinearEasing),
            repeatMode = RepeatMode.Restart
        ),
        label = "stepPlanDotPhase"
    )
    val dotSize = 3.5.dp
    val gap = 2.dp
    val offsets = remember { floatArrayOf(0f, 0.2f / 1.4f, 0.4f / 1.4f, 0.6f / 1.4f) }

    fun dotAlpha(idx: Int): Float {
        val shifted = ((phase - offsets[idx]) % 1f + 1f) % 1f
        val wave = (0.5 - 0.5 * cos(shifted * 2.0 * PI)).toFloat()
        return 0.26f + 0.62f * wave
    }

    Box(modifier = modifier.size(size), contentAlignment = Alignment.Center) {
        Column(verticalArrangement = Arrangement.spacedBy(gap)) {
            Row(horizontalArrangement = Arrangement.spacedBy(gap)) {
                Box(
                    modifier = Modifier
                        .size(dotSize)
                        .clip(CircleShape)
                        .background(color.copy(alpha = dotAlpha(0)))
                )
                Box(
                    modifier = Modifier
                        .size(dotSize)
                        .clip(CircleShape)
                        .background(color.copy(alpha = dotAlpha(1)))
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(gap)) {
                Box(
                    modifier = Modifier
                        .size(dotSize)
                        .clip(CircleShape)
                        .background(color.copy(alpha = dotAlpha(2)))
                )
                Box(
                    modifier = Modifier
                        .size(dotSize)
                        .clip(CircleShape)
                        .background(color.copy(alpha = dotAlpha(3)))
                )
            }
        }
    }
}

/**
 * Circular progress ring matching Antigravity desktop `lSc`.
 */
@Composable
fun StepPlanProgressRing(
    done: Int,
    total: Int,
    color: Color,
    size: Dp = 13.dp,
    modifier: Modifier = Modifier
) {
    val ratio = if (total > 0) (done.toFloat() / total.toFloat()).coerceIn(0f, 1f) else 0f
    Canvas(modifier = modifier.size(size)) {
        val strokeWidth = 1.8.dp.toPx()
        drawCircle(
            color = color.copy(alpha = 0.25f),
            style = Stroke(width = strokeWidth)
        )
        if (ratio > 0f) {
            drawArc(
                color = color,
                startAngle = -90f,
                sweepAngle = 360f * ratio,
                useCenter = false,
                style = Stroke(width = strokeWidth, cap = StrokeCap.Round)
            )
        }
    }
}

/**
 * Scheme A native StepPlan card:
 * - While running (`isActivePlan == true`): shows the full rounded checklist card.
 * - Once completed (`isActivePlan == false`): auto-collapses into a compact progress chip
 *   (`4/4 步`) that expands smoothly on tap.
 */
@Composable
fun StepPlanCard(
    plan: StepPlanItem,
    isActivePlan: Boolean,
    modifier: Modifier = Modifier
) {
    if (plan.items.isEmpty()) return

    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    var isManuallyExpanded by remember(plan.anchorStepIndex) { mutableStateOf(false) }

    LaunchedEffect(isActivePlan) {
        if (!isActivePlan) {
            isManuallyExpanded = false
        }
    }

    val done = plan.effectiveDoneCount
    val total = plan.effectiveTotalCount
    val failed = plan.effectiveFailedCount
    val skipped = plan.effectiveSkippedCount

    val chipLabel = remember(done, total, failed, skipped) {
        buildList {
            if (total > 0) add("$done/$total 步".loc)
            if (failed > 0) add("$failed 失败".loc)
            if (skipped > 0) add("$skipped 跳过".loc)
        }.joinToString(" · ")
    }

    val chevronAngle by animateFloatAsState(
        targetValue = if (isManuallyExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "stepPlanChevron"
    )

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        if (isActivePlan) {
            StepPlanChecklistBox(plan = plan, isRunning = true)
        } else {
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(9.dp))
                    .background(colors.surfaceVariant)
                    .border(0.7.dp, colors.border, RoundedCornerShape(9.dp))
                    .clickable {
                        haptic.light()
                        isManuallyExpanded = !isManuallyExpanded
                    }
                    .padding(horizontal = 10.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                StepPlanProgressRing(
                    done = done,
                    total = total,
                    color = colors.textSecondary
                )
                Text(
                    text = chipLabel,
                    color = if (isManuallyExpanded) colors.textPrimary else colors.textSecondary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.Medium,
                    style = androidx.compose.ui.text.TextStyle(fontFeatureSettings = "tnum")
                )
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                    contentDescription = if (isManuallyExpanded) "Collapse" else "Expand",
                    tint = colors.textSecondary.copy(alpha = 0.7f),
                    modifier = Modifier
                        .size(14.dp)
                        .rotate(chevronAngle)
                )
            }

            AnimatedVisibility(
                visible = isManuallyExpanded,
                enter = expandVertically(
                    animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
                ) + fadeIn(),
                exit = shrinkVertically(
                    animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
                ) + fadeOut()
            ) {
                StepPlanChecklistBox(plan = plan, isRunning = false)
            }
        }
    }
}

@Composable
private fun StepPlanChecklistBox(
    plan: StepPlanItem,
    isRunning: Boolean
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface.copy(alpha = if (colors.isDark) 0.85f else 0.92f))
            .border(0.9.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(horizontal = 14.dp, vertical = 11.dp),
        verticalArrangement = Arrangement.spacedBy(9.dp)
    ) {
        plan.items.forEach { item ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 16.dp, height = 19.dp),
                    contentAlignment = Alignment.Center
                ) {
                    when (item.status) {
                        "done" -> Icon(
                            imageVector = Icons.Default.CheckCircleOutline,
                            contentDescription = "Done",
                            tint = colors.textSecondary,
                            modifier = Modifier.size(15.dp)
                        )
                        "failed" -> Icon(
                            imageVector = Icons.Default.ErrorOutline,
                            contentDescription = "Failed",
                            tint = colors.accentRed,
                            modifier = Modifier.size(15.dp)
                        )
                        "in_progress" -> if (isRunning) {
                            StepPlanDotSpinner(color = colors.textPrimary, size = 14.dp)
                        } else {
                            Icon(
                                imageVector = Icons.Default.PauseCircleOutline,
                                contentDescription = "Paused",
                                tint = colors.textSecondary,
                                modifier = Modifier.size(15.dp)
                            )
                        }
                        "skipped" -> Icon(
                            imageVector = Icons.Default.RadioButtonUnchecked,
                            contentDescription = "Skipped",
                            tint = colors.textSecondary.copy(alpha = 0.45f),
                            modifier = Modifier.size(15.dp)
                        )
                        else -> Icon(
                            imageVector = Icons.Default.RadioButtonUnchecked,
                            contentDescription = "Pending",
                            tint = colors.textSecondary.copy(alpha = 0.75f),
                            modifier = Modifier.size(15.dp)
                        )
                    }
                }

                val textColor = when (item.status) {
                    "done", "skipped" -> colors.textSecondary
                    "pending" -> colors.textPrimary.copy(alpha = 0.85f)
                    else -> colors.textPrimary
                }
                Text(
                    text = item.title,
                    color = textColor,
                    fontSize = 13.5.sp,
                    lineHeight = 19.sp,
                    fontWeight = if (item.status == "in_progress") FontWeight.Medium else FontWeight.Normal,
                    textDecoration = if (item.status == "skipped") TextDecoration.LineThrough else TextDecoration.None,
                    modifier = Modifier.weight(1f)
                )
            }
        }
    }
}

internal fun parseIsoTimestampMs(raw: String?): Long? {
    val trimmed = raw?.trim()?.takeIf { it.isNotEmpty() } ?: return null
    return try {
        Instant.parse(trimmed).toEpochMilli()
    } catch (_: Exception) {
        try {
            OffsetDateTime.parse(trimmed).toInstant().toEpochMilli()
        } catch (_: Exception) {
            null
        }
    }
}

internal fun formatElapsedDuration(elapsedMs: Long): String {
    val secs = maxOf(0L, elapsedMs / 1000L)
    val h = secs / 3600L
    val m = (secs % 3600L) / 60L
    val s = secs % 60L
    return when {
        h > 0L -> "${h}h ${m}m ${s}s"
        m > 0L -> "${m}m ${s}s"
        else -> "${s}s"
    }
}

/**
 * Level-2 collapsible thinking process block used inside ActiveFocusToolRow,
 * ToolStepCollapseCard, and standalone reasoning sections.
 */
@Composable
fun CollapsibleThinkingBlock(
    thinkingText: String,
    initiallyExpanded: Boolean = false,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    var isExpanded by remember { mutableStateOf(initiallyExpanded) }
    val chevronAngle by animateFloatAsState(
        targetValue = if (isExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "thinkingChevron"
    )

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(colors.accentIndigo.copy(alpha = 0.08f))
            .border(0.5.dp, colors.accentIndigo.copy(alpha = 0.16f), RoundedCornerShape(8.dp))
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable {
                    haptic.light()
                    isExpanded = !isExpanded
                }
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Psychology,
                contentDescription = "Thinking",
                tint = colors.accentIndigo,
                modifier = Modifier.size(13.dp)
            )
            Text(
                text = "思考过程".loc,
                color = colors.accentIndigo,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f)
            )
            Icon(
                imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                contentDescription = if (isExpanded) "Collapse" else "Expand",
                tint = colors.accentIndigo.copy(alpha = 0.7f),
                modifier = Modifier
                    .size(14.dp)
                    .rotate(chevronAngle)
            )
        }

        AnimatedVisibility(
            visible = isExpanded,
            enter = expandVertically(
                animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
            ) + fadeIn(),
            exit = shrinkVertically(
                animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
            ) + fadeOut()
        ) {
            SelectionContainer {
                Text(
                    text = thinkingText,
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    lineHeight = 16.sp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(start = 12.dp, end = 12.dp, top = 2.dp, bottom = 8.dp)
                )
            }
        }
    }
}

/**
 * Live focus status line + 1-second elapsed timer + expandable tool details,
 * matching Antigravity desktop `tSc` + `rSc` right underneath the StepPlan card.
 */
@Composable
fun ActiveFocusToolRow(
    focusText: String,
    startedAt: String? = null,
    thinkingText: String? = null,
    thinkingDuration: String? = null,
    thinkingBlocks: List<ThinkingBlockItem>? = null,
    toolDetails: List<ToolDetailItem>? = null,
    toolCalls: List<ToolCallItem> = emptyList(),
    toolNames: List<String> = emptyList(),
    isExpandable: Boolean = true,
    onFileClick: ((uri: String, title: String) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    var isExpanded by remember { mutableStateOf(false) }

    val hasTools = !toolDetails.isNullOrEmpty() || toolCalls.isNotEmpty() || toolNames.isNotEmpty()
    val hasDetails = isExpandable && (!thinkingText.isNullOrBlank() || !thinkingBlocks.isNullOrEmpty() || hasTools)

    val startMs = remember(startedAt) { parseIsoTimestampMs(startedAt) }
    var nowMs by remember(startMs) { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(startMs) {
        if (startMs == null) return@LaunchedEffect
        while (true) {
            nowMs = System.currentTimeMillis()
            delay(1000L)
        }
    }

    val infiniteTransition = rememberInfiniteTransition(label = "focusBreath")
    val textAlpha by infiniteTransition.animateFloat(
        initialValue = 0.62f,
        targetValue = 1.0f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 1200, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "focusBreathAlpha"
    )

    val chevronAngle by animateFloatAsState(
        targetValue = if (isExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "focusChevron"
    )

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 2.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(8.dp))
                .then(
                    if (hasDetails) {
                        Modifier.clickable {
                            haptic.light()
                            isExpanded = !isExpanded
                        }
                    } else {
                        Modifier
                    }
                )
                .padding(horizontal = 4.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = focusText,
                color = colors.textSecondary.copy(alpha = textAlpha),
                fontSize = 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )
            if (startMs != null) {
                Text(
                    text = formatElapsedDuration(nowMs - startMs),
                    color = colors.textSecondary.copy(alpha = 0.85f),
                    fontSize = 12.5.sp,
                    style = androidx.compose.ui.text.TextStyle(fontFeatureSettings = "tnum")
                )
            }
            if (hasDetails) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                    contentDescription = if (isExpanded) "Collapse" else "Expand",
                    tint = colors.textSecondary.copy(alpha = 0.65f),
                    modifier = Modifier
                        .size(14.dp)
                        .rotate(chevronAngle)
                )
            }
        }

        AnimatedVisibility(
            visible = isExpanded && hasDetails,
            enter = expandVertically(
                animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
            ) + fadeIn(),
            exit = shrinkVertically(
                animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
            ) + fadeOut()
        ) {
            DesktopToolTimelineList(
                thinkingText = thinkingText,
                thinkingDuration = thinkingDuration,
                thinkingBlocks = thinkingBlocks,
                toolDetails = toolDetails,
                toolCalls = toolCalls,
                toolNames = toolNames,
                onFileClick = onFileClick
            )
        }
    }
}
