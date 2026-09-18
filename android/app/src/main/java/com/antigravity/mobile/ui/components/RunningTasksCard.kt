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
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.RunningTaskItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

@Composable
fun RunningTasksCard(
    tasks: List<RunningTaskItem>,
    onStopTask: (RunningTaskItem) -> Unit,
    onToggleExpand: ((Boolean) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    if (tasks.isEmpty()) return

    var isExpanded by remember { mutableStateOf(true) }
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()

    // iOS 对齐：展开时箭头朝下(0度)，折叠时箭头朝上(180度)，弹性动画旋转
    val arrowRotation by animateFloatAsState(
        targetValue = if (isExpanded) 0f else 180f,
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
                        isExpanded = !isExpanded
                        onToggleExpand?.invoke(isExpanded)
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
                    contentDescription = if (isExpanded) "折叠任务" else "展开任务",
                    tint = colors.textSecondary,
                    modifier = Modifier
                        .size(20.dp)
                        .graphicsLayer(rotationZ = arrowRotation)
                )
            }

            AnimatedVisibility(
                visible = isExpanded,
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
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    tasks.forEachIndexed { index, task ->
                        val desc = task.displayTitle
                        val cmdText = task.displayCommand

                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 4.dp),
                            verticalAlignment = Alignment.Bottom,
                            horizontalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Column(
                                modifier = Modifier.weight(1f),
                                verticalArrangement = Arrangement.spacedBy(3.dp)
                            ) {
                                if (desc.isNotBlank() && desc != cmdText) {
                                    Text(
                                        text = desc,
                                        color = colors.textPrimary,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }

                                val bubbleText = cmdText.ifBlank { desc.ifBlank { "run_command" } }
                                Box(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(6.dp))
                                        .background(colors.surfaceVariant)
                                        .padding(horizontal = 6.dp, vertical = 3.dp)
                                ) {
                                    Text(
                                        text = bubbleText,
                                        color = colors.textSecondary,
                                        fontSize = 12.sp,
                                        fontFamily = FontFamily.Monospace,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }

                            // Stop Task Button (对齐 iOS: 缩小并放置在命令气泡外部)
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
                                        .size(22.dp)
                                        .clip(CircleShape)
                                        .background(colors.accentRed.copy(alpha = 0.9f)),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Box(
                                        modifier = Modifier
                                            .size(7.5.dp)
                                            .clip(RoundedCornerShape(1.5.dp))
                                            .background(colors.surface)
                                    )
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
