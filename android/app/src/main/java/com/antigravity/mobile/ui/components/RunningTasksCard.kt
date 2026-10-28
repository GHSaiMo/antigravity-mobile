package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.RunningTaskItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.delay

@Composable
fun RunningTasksCard(
    tasks: List<RunningTaskItem>,
    onStopTask: (RunningTaskItem) -> Unit,
    onToggleExpand: ((Boolean) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    if (tasks.isEmpty()) return

    var isCardExpanded by remember { mutableStateOf(true) }
    var expandedTaskIds by remember { mutableStateOf(setOf<String>()) }
    var copiedTaskId by remember { mutableStateOf<String?>(null) }

    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val clipboardManager = LocalClipboardManager.current

    LaunchedEffect(copiedTaskId) {
        if (copiedTaskId != null) {
            delay(1500)
            copiedTaskId = null
        }
    }

    val arrowRotation by animateFloatAsState(
        targetValue = if (isCardExpanded) 0f else 180f,
        animationSpec = spring(
            dampingRatio = 0.82f,
            stiffness = Spring.StiffnessMediumLow
        ),
        label = "tasksArrowRotation"
    )

    Card(
        modifier = modifier
            .fillMaxWidth()
            .shadow(
                elevation = 4.dp,
                shape = RoundedCornerShape(18.dp),
                ambientColor = Color.Black.copy(alpha = 0.04f),
                spotColor = Color.Black.copy(alpha = 0.08f)
            ),
        shape = RoundedCornerShape(18.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surface),
        border = CardDefaults.outlinedCardBorder().copy(brush = SolidColor(colors.border.copy(alpha = 0.5f)))
    ) {
        Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp)) {
            // Header
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable {
                        isCardExpanded = !isCardExpanded
                        onToggleExpand?.invoke(isCardExpanded)
                    },
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(16.dp),
                        strokeWidth = 2.dp,
                        color = colors.accentIndigo,
                        trackColor = colors.accentIndigo.copy(alpha = 0.2f)
                    )

                    Text(
                        text = "${tasks.size} 个任务正在执行",
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold
                    )

                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(colors.surfaceVariant)
                            .padding(horizontal = 7.dp, vertical = 2.5.dp)
                    ) {
                        Text(
                            text = "${tasks.size}",
                            color = colors.textSecondary,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Bold
                        )
                    }
                }

                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = if (isCardExpanded) "折叠任务" else "展开任务",
                    tint = colors.textSecondary,
                    modifier = Modifier
                        .size(20.dp)
                        .graphicsLayer(rotationZ = arrowRotation)
                )
            }

            AnimatedVisibility(
                visible = isCardExpanded,
                enter = expandVertically(
                    animationSpec = spring(
                        dampingRatio = 0.82f,
                        stiffness = Spring.StiffnessMediumLow
                    )
                ) + fadeIn(
                    animationSpec = spring(dampingRatio = 0.82f)
                ),
                exit = shrinkVertically(
                    animationSpec = spring(
                        dampingRatio = 0.82f,
                        stiffness = Spring.StiffnessMediumLow
                    )
                ) + fadeOut(
                    animationSpec = spring(dampingRatio = 0.82f)
                )
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    tasks.forEachIndexed { index, task ->
                        val desc = task.displayTitle
                        val rawCmd = task.commandLine.ifBlank { task.displayCommand }
                        val isCmdExpanded = expandedTaskIds.contains(task.id)
                        val isCopied = copiedTaskId == task.id
                        val isLong = rawCmd.length > 100 || rawCmd.lines().size > 3

                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 2.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            // Top bar of task row: summary + copy + stop
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween
                            ) {
                                if (desc.isNotBlank() && desc != rawCmd) {
                                    Text(
                                        text = desc,
                                        color = colors.textPrimary,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f)
                                    )
                                } else {
                                    Spacer(modifier = Modifier.weight(1f))
                                }

                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    // Copy Command Button
                                    Row(
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(6.dp))
                                            .background(colors.surfaceVariant)
                                            .clickable {
                                                haptic.light()
                                                clipboardManager.setText(AnnotatedString(rawCmd))
                                                copiedTaskId = task.id
                                            }
                                            .padding(horizontal = 6.dp, vertical = 3.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                                    ) {
                                        Icon(
                                            imageVector = if (isCopied) Icons.Default.Check else Icons.Outlined.ContentCopy,
                                            contentDescription = "复制命令行",
                                            tint = if (isCopied) Color(0xFF4CAF50) else colors.textSecondary,
                                            modifier = Modifier.size(12.dp)
                                        )
                                        Text(
                                            text = if (isCopied) "已复制" else "复制",
                                            fontSize = 10.sp,
                                            color = if (isCopied) Color(0xFF4CAF50) else colors.textSecondary,
                                            fontWeight = FontWeight.Medium
                                        )
                                    }

                                    // Stop Task Button
                                    Box(
                                        modifier = Modifier
                                            .size(28.dp)
                                            .clip(CircleShape)
                                            .clickable {
                                                haptic.medium()
                                                onStopTask(task)
                                            },
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .size(20.dp)
                                                .clip(CircleShape)
                                                .background(colors.accentRed.copy(alpha = 0.9f)),
                                            contentAlignment = Alignment.Center
                                        ) {
                                            Box(
                                                modifier = Modifier
                                                    .size(7.dp)
                                                    .clip(RoundedCornerShape(1.dp))
                                                    .background(colors.surface)
                                            )
                                        }
                                    }
                                }
                            }

                            // Terminal-style command bubble
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(colors.surfaceVariant.copy(alpha = 0.7f))
                                    .padding(horizontal = 8.dp, vertical = 6.dp),
                                verticalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Text(
                                    text = highlightCommand(rawCmd, colors.accentIndigo, colors.textPrimary),
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    maxLines = if (isCmdExpanded) Int.MAX_VALUE else 3,
                                    overflow = TextOverflow.Ellipsis
                                )

                                if (isLong) {
                                    Row(
                                        modifier = Modifier
                                            .clickable {
                                                expandedTaskIds = if (isCmdExpanded) {
                                                    expandedTaskIds - task.id
                                                } else {
                                                    expandedTaskIds + task.id
                                                }
                                            }
                                            .padding(top = 2.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(2.dp)
                                    ) {
                                        Text(
                                            text = if (isCmdExpanded) "收起" else "展开完整命令",
                                            color = colors.accentIndigo,
                                            fontSize = 10.sp,
                                            fontWeight = FontWeight.Medium
                                        )
                                        Icon(
                                            imageVector = if (isCmdExpanded) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                                            contentDescription = null,
                                            tint = colors.accentIndigo,
                                            modifier = Modifier.size(12.dp)
                                        )
                                    }
                                }
                            }
                        }

                        if (index < tasks.size - 1) {
                            HorizontalDivider(
                                modifier = Modifier.fillMaxWidth(),
                                thickness = 0.5.dp,
                                color = colors.border.copy(alpha = 0.4f)
                            )
                        }
                    }
                }
            }
        }
    }
}

private fun highlightCommand(command: String, accentColor: Color, defaultTextColor: Color): AnnotatedString {
    return buildAnnotatedString {
        withStyle(SpanStyle(color = Color(0xFF4CAF50), fontWeight = FontWeight.Bold)) {
            append("$ ")
        }
        val parts = command.split(" ")
        var isFirstWord = true
        parts.forEachIndexed { idx, part ->
            if (idx > 0) append(" ")
            when {
                part.startsWith("-") -> {
                    withStyle(SpanStyle(color = Color(0xFFFF9800))) {
                        append(part)
                    }
                }
                part in listOf("&&", "||", "|", ";") -> {
                    withStyle(SpanStyle(color = Color(0xFFBA68C8))) {
                        append(part)
                    }
                    isFirstWord = true
                }
                isFirstWord && part.isNotEmpty() -> {
                    withStyle(SpanStyle(color = accentColor, fontWeight = FontWeight.SemiBold)) {
                        append(part)
                    }
                    isFirstWord = false
                }
                else -> {
                    withStyle(SpanStyle(color = defaultTextColor.copy(alpha = 0.85f))) {
                        append(part)
                    }
                }
            }
        }
    }
}
