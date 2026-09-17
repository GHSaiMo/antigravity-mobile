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
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Extension
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ToolCallItem
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun ToolStepCollapseCard(
    toolCalls: List<ToolCallItem>,
    modifier: Modifier = Modifier
) {
    if (toolCalls.isEmpty()) return

    var isExpanded by remember { mutableStateOf(false) }
    val colors = AntigravityTheme.colors

    val uniqueToolNames = remember(toolCalls) {
        toolCalls.map { it.name.ifBlank { "action" } }.distinct().joinToString(", ")
    }

    val summaryText = remember(toolCalls, uniqueToolNames) {
        if (uniqueToolNames.isNotBlank()) {
            "已思考并执行 ${toolCalls.size} 项操作 ($uniqueToolNames)"
        } else {
            "已思考并执行 ${toolCalls.size} 项操作"
        }
    }

    val rotationAngle by animateFloatAsState(
        targetValue = if (isExpanded) 180f else 0f,
        animationSpec = spring(
            dampingRatio = 0.82f,
            stiffness = Spring.StiffnessMediumLow
        ),
        label = "chevronRotation"
    )

    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        // iOS-style Tool Batch Capsule Header
        Box(
            modifier = Modifier
                .clip(CircleShape)
                .background(colors.surfaceVariant)
                .border(0.8.dp, colors.border, CircleShape)
                .clickable {
                    haptic.light()
                    isExpanded = !isExpanded
                }
                .padding(horizontal = 14.dp, vertical = 8.dp)
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Bolt,
                    contentDescription = "Tool Operation",
                    tint = colors.accentOrange,
                    modifier = Modifier.size(14.dp)
                )

                Text(
                    text = summaryText,
                    color = colors.textPrimary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.Medium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )

                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = if (isExpanded) "Collapse" else "Expand",
                    tint = colors.textSecondary,
                    modifier = Modifier
                        .size(14.dp)
                        .rotate(rotationAngle)
                )
            }
        }

        // Expandable tool call list
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
                    .padding(top = 2.dp),
                verticalArrangement = Arrangement.spacedBy(5.dp)
            ) {
                toolCalls.forEachIndexed { index, tool ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .background(colors.surfaceVariant.copy(alpha = 0.7f))
                            .border(0.5.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(8.dp))
                            .padding(horizontal = 12.dp, vertical = 7.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Extension,
                            contentDescription = "Tool",
                            tint = colors.accentGreen,
                            modifier = Modifier.size(13.dp)
                        )

                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = tool.name.ifBlank { tool.type.ifBlank { "action_${index + 1}" } },
                                color = colors.textPrimary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold,
                                fontFamily = FontFamily.Monospace,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                            tool.input?.takeIf { it.isNotBlank() }?.let { input ->
                                Text(
                                    text = input.take(200) + if (input.length > 200) "..." else "",
                                    color = colors.textSecondary,
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    maxLines = 2,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
