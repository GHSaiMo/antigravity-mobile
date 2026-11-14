package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.filled.Segment
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material.icons.filled.StopCircle
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SubagentItem
import com.antigravity.mobile.data.model.WorkflowItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

/**
 * 动态多智能体工作流拓扑看板 (Antigravity 2.32+ Dynamic Workflows)。
 * 展示阶段流转轴 (Stage Pipeline Stepper)、并发子智能体拓扑网格、Live 日志流与结构化 Schema 产物。
 */
@Composable
fun WorkflowStageCard(
    workflow: WorkflowItem,
    modifier: Modifier = Modifier,
    onStopWorkflow: (() -> Unit)? = null,
    onSelectSubagent: ((SubagentItem) -> Unit)? = null
) {
    var isExpanded by remember { mutableStateOf(true) }
    var selectedPhaseIndex by remember { mutableIntStateOf(0) }
    var isLogsExpanded by remember { mutableStateOf(false) }
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val clipboardManager = LocalClipboardManager.current

    val isRunning = workflow.status.equals("running", ignoreCase = true)
    val isError = workflow.status.equals("error", ignoreCase = true) || workflow.status.equals("failed", ignoreCase = true)

    val statusText = when {
        isRunning -> "编排执行中"
        isError -> "执行失败"
        else -> "工作流完成"
    }

    val statusColor = when {
        isRunning -> colors.accentGreen
        isError -> colors.accentRed
        else -> colors.textMuted
    }

    val allSubagents = remember(workflow.phases, selectedPhaseIndex) {
        if (workflow.phases.isNotEmpty() && selectedPhaseIndex < workflow.phases.size) {
            val p = workflow.phases[selectedPhaseIndex]
            if (p.subagents.isNotEmpty()) p.subagents else workflow.phases.flatMap { it.subagents }
        } else {
            workflow.phases.flatMap { it.subagents }
        }
    }

    Card(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surface),
        border = CardDefaults.outlinedCardBorder().copy(
            brush = SolidColor(if (isError) colors.accentRed.copy(alpha = 0.5f) else (if (isRunning) colors.accentIndigo.copy(alpha = 0.45f) else colors.border.copy(alpha = 0.6f)))
        )
    ) {
        Column(modifier = Modifier.padding(14.dp)) {
            // Header Row
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier
                        .weight(1f)
                        .clickable {
                            haptic.light()
                            isExpanded = !isExpanded
                        }
                ) {
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.15f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.AccountTree,
                            contentDescription = "Workflow",
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(16.dp)
                        )
                    }

                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = workflow.description?.takeIf { it.isNotBlank() } ?: "动态多智能体工作流",
                            color = colors.textPrimary,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(6.dp)
                                    .clip(CircleShape)
                                    .background(statusColor)
                            )
                            Text(
                                text = statusText,
                                color = colors.textMuted,
                                fontSize = 11.sp
                            )
                            workflow.duration?.takeIf { it.isNotBlank() }?.let { dur ->
                                Text(
                                    text = "• $dur",
                                    color = colors.textMuted,
                                    fontSize = 11.sp
                                )
                            }
                        }
                    }
                }

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    // Stop Workflow button
                    if (isRunning && onStopWorkflow != null) {
                        Surface(
                            shape = RoundedCornerShape(12.dp),
                            color = colors.accentRed.copy(alpha = 0.12f),
                            modifier = Modifier.clickable {
                                haptic.heavy()
                                onStopWorkflow()
                            }
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(3.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.StopCircle,
                                    contentDescription = null,
                                    tint = colors.accentRed,
                                    modifier = Modifier.size(12.dp)
                                )
                                Text(
                                    text = "终止",
                                    color = colors.accentRed,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                        }
                    }

                    IconButton(
                        onClick = {
                            haptic.light()
                            isExpanded = !isExpanded
                        },
                        modifier = Modifier.size(24.dp)
                    ) {
                        Icon(
                            imageVector = if (isExpanded) Icons.Default.KeyboardArrowDown else Icons.Default.KeyboardArrowRight,
                            contentDescription = if (isExpanded) "折叠" else "展开",
                            tint = colors.textMuted,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                }
            }

            AnimatedVisibility(
                visible = isExpanded,
                enter = fadeIn() + expandVertically(),
                exit = fadeOut() + shrinkVertically()
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 10.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    // 1. Stage Pipeline Stepper
                    if (workflow.phases.isNotEmpty()) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .horizontalScroll(rememberScrollState()),
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            workflow.phases.forEachIndexed { pIdx, phase ->
                                val isSelected = selectedPhaseIndex == pIdx
                                Surface(
                                    shape = RoundedCornerShape(8.dp),
                                    color = if (isSelected) colors.accentIndigo.copy(alpha = 0.15f) else colors.surfaceVariant.copy(alpha = 0.5f),
                                    border = CardDefaults.outlinedCardBorder().copy(
                                        brush = SolidColor(if (isSelected) colors.accentIndigo.copy(alpha = 0.5f) else Color.Transparent)
                                    ),
                                    modifier = Modifier.clickable {
                                        haptic.light()
                                        selectedPhaseIndex = pIdx
                                    }
                                ) {
                                    Row(
                                        modifier = Modifier.padding(horizontal = 9.dp, vertical = 6.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(5.dp)
                                    ) {
                                        when (phase.status) {
                                            "done" -> Icon(
                                                imageVector = Icons.Default.CheckCircle,
                                                contentDescription = null,
                                                tint = colors.accentGreen,
                                                modifier = Modifier.size(13.dp)
                                            )
                                            "running" -> CircularProgressIndicator(
                                                modifier = Modifier.size(12.dp),
                                                color = colors.accentIndigo,
                                                strokeWidth = 2.dp
                                            )
                                            "failed" -> Icon(
                                                imageVector = Icons.Default.Error,
                                                contentDescription = null,
                                                tint = colors.accentRed,
                                                modifier = Modifier.size(13.dp)
                                            )
                                            else -> Box(
                                                modifier = Modifier
                                                    .size(10.dp)
                                                    .border(1.dp, colors.textMuted.copy(alpha = 0.5f), CircleShape)
                                            )
                                        }

                                        Text(
                                            text = "P${pIdx + 1}: ${phase.name}",
                                            color = if (isSelected) colors.textPrimary else colors.textSecondary,
                                            fontSize = 11.5.sp,
                                            fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Medium
                                        )

                                        if (phase.subagents.isNotEmpty()) {
                                            Text(
                                                text = "(${phase.subagents.size})",
                                                color = colors.accentIndigo,
                                                fontSize = 10.sp,
                                                fontFamily = FontFamily.Monospace
                                            )
                                        }
                                    }
                                }
                            }
                        }
                    }

                    // 2. Subagent Matrix
                    if (allSubagents.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = "并发子代理 (Sub-Agents)",
                                    color = colors.textMuted,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.Bold
                                )
                                Text(
                                    text = "轻触穿透查看",
                                    color = colors.textMuted,
                                    fontSize = 10.sp
                                )
                            }

                            allSubagents.chunked(2).forEach { rowSubagents ->
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    rowSubagents.forEach { sa ->
                                        Surface(
                                            shape = RoundedCornerShape(8.dp),
                                            color = colors.surfaceVariant.copy(alpha = 0.6f),
                                            modifier = Modifier
                                                .weight(1f)
                                                .clickable {
                                                    haptic.light()
                                                    onSelectSubagent?.invoke(sa)
                                                }
                                        ) {
                                            Row(
                                                modifier = Modifier.padding(8.dp),
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                                            ) {
                                                Box(
                                                    modifier = Modifier
                                                        .size(22.dp)
                                                        .clip(CircleShape)
                                                        .background(if (sa.isRunning) colors.accentGreen.copy(alpha = 0.18f) else colors.textMuted.copy(alpha = 0.15f)),
                                                    contentAlignment = Alignment.Center
                                                ) {
                                                    Icon(
                                                        imageVector = Icons.Default.Memory,
                                                        contentDescription = null,
                                                        tint = if (sa.isRunning) colors.accentGreen else colors.textMuted,
                                                        modifier = Modifier.size(12.dp)
                                                    )
                                                }

                                                Column(modifier = Modifier.weight(1f)) {
                                                    Text(
                                                        text = sa.displayName,
                                                        color = colors.textPrimary,
                                                        fontSize = 11.5.sp,
                                                        fontWeight = FontWeight.SemiBold,
                                                        maxLines = 1,
                                                        overflow = TextOverflow.Ellipsis
                                                    )
                                                    Text(
                                                        text = if (sa.isRunning) "运行中" else "已就绪",
                                                        color = colors.textSecondary,
                                                        fontSize = 9.5.sp
                                                    )
                                                }

                                                Icon(
                                                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                                    contentDescription = null,
                                                    tint = colors.textMuted,
                                                    modifier = Modifier.size(12.dp)
                                                )
                                            }
                                        }
                                    }
                                    if (rowSubagents.size == 1) {
                                        Spacer(modifier = Modifier.weight(1f))
                                    }
                                }
                            }
                        }
                    }

                    // 3. Live Logs
                    val currentLogs = remember(workflow.phases, workflow.logs, selectedPhaseIndex) {
                        if (workflow.phases.isNotEmpty() && selectedPhaseIndex < workflow.phases.size) {
                            val pLogs = workflow.phases[selectedPhaseIndex].logs
                            if (pLogs.isNotEmpty()) pLogs else workflow.logs
                        } else {
                            workflow.logs
                        }
                    }

                    if (currentLogs.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable {
                                        haptic.light()
                                        isLogsExpanded = !isLogsExpanded
                                    },
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.Terminal,
                                        contentDescription = null,
                                        tint = colors.textMuted,
                                        modifier = Modifier.size(11.dp)
                                    )
                                    Text(
                                        text = if (isLogsExpanded) "收起日志输出" else "展开执行日志 (${currentLogs.size} 行)",
                                        color = colors.textSecondary,
                                        fontSize = 11.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                }

                                Icon(
                                    imageVector = if (isLogsExpanded) Icons.Default.KeyboardArrowDown else Icons.Default.KeyboardArrowRight,
                                    contentDescription = null,
                                    tint = colors.textMuted,
                                    modifier = Modifier.size(14.dp)
                                )
                            }

                            AnimatedVisibility(visible = isLogsExpanded) {
                                Surface(
                                    shape = RoundedCornerShape(8.dp),
                                    color = Color.Black.copy(alpha = 0.35f),
                                    modifier = Modifier.fillMaxWidth()
                                ) {
                                    Text(
                                        text = currentLogs.joinToString("\n"),
                                        modifier = Modifier.padding(8.dp),
                                        color = colors.textPrimary,
                                        fontSize = 10.5.sp,
                                        fontFamily = FontFamily.Monospace
                                    )
                                }
                            }
                        }
                    }

                    // 4. Structured Schema Output
                    if (!workflow.resultJson.isNullOrBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(Color.Black.copy(alpha = 0.35f))
                                .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                                .padding(10.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.AutoMirrored.Filled.Segment,
                                        contentDescription = null,
                                        tint = colors.accentOrange,
                                        modifier = Modifier.size(12.dp)
                                    )
                                    Text(
                                        text = "结构化产物 (Schema Output)",
                                        color = colors.accentOrange,
                                        fontSize = 10.5.sp,
                                        fontWeight = FontWeight.Bold
                                    )
                                }

                                IconButton(
                                    onClick = {
                                        haptic.light()
                                        clipboardManager.setText(AnnotatedString(workflow.resultJson))
                                    },
                                    modifier = Modifier.size(20.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.ContentCopy,
                                        contentDescription = "复制",
                                        tint = colors.textMuted,
                                        modifier = Modifier.size(12.dp)
                                    )
                                }
                            }

                            Spacer(modifier = Modifier.height(4.dp))

                            Text(
                                text = workflow.resultJson,
                                color = colors.textPrimary,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }
                    }

                    // 5. Error Banner
                    if (isError) {
                        Surface(
                            shape = RoundedCornerShape(8.dp),
                            color = colors.accentRed.copy(alpha = 0.12f),
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Row(
                                modifier = Modifier.padding(8.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.Error,
                                    contentDescription = null,
                                    tint = colors.accentRed,
                                    modifier = Modifier.size(14.dp)
                                )
                                Text(
                                    text = workflow.error?.ifBlank { null } ?: "工作流在执行中遇到未捕获异常或单节点失败",
                                    color = colors.accentRed,
                                    fontSize = 11.sp
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
