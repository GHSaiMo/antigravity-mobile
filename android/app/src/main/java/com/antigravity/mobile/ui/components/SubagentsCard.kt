package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
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
import androidx.compose.material.icons.filled.KeyboardArrowRight
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SubagentItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

/** 当前会话派发的子代理：点一行进入子会话（只读），运行中的可单独关停。 */
@Composable
fun SubagentsCard(
    items: List<SubagentItem>,
    canStop: Boolean,
    onOpen: (SubagentItem) -> Unit,
    onStop: (SubagentItem) -> Unit,
    onToggleExpand: ((Boolean) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    if (items.isEmpty()) return

    var isExpanded by remember { mutableStateOf(true) }
    var pendingStop by remember { mutableStateOf<SubagentItem?>(null) }
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val runningCount = items.count { it.isRunning }

    Card(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(18.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surface),
        border = CardDefaults.outlinedCardBorder().copy(brush = SolidColor(colors.border.copy(alpha = 0.5f)))
    ) {
        Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp)) {
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
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (runningCount > 0) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(16.dp),
                            strokeWidth = 2.dp,
                            color = colors.accentIndigo,
                            trackColor = colors.accentIndigo.copy(alpha = 0.2f)
                        )
                    }
                    Text(
                        text = if (runningCount > 0) "$runningCount 个子代理运行中" else "${items.size} 个子代理",
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
                        Text(text = "${items.size}", color = colors.textSecondary, fontSize = 11.sp, fontWeight = FontWeight.Bold)
                    }
                }
                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = if (isExpanded) "折叠子代理" else "展开子代理",
                    tint = colors.textSecondary,
                    modifier = Modifier.size(20.dp).rotate(if (isExpanded) 0f else 180f)
                )
            }

            AnimatedVisibility(
                visible = isExpanded,
                enter = expandVertically() + fadeIn(),
                exit = shrinkVertically() + fadeOut()
            ) {
                Column(
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    items.forEachIndexed { index, item ->
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Row(
                                modifier = Modifier
                                    .weight(1f)
                                    .then(if (item.isGone) Modifier else Modifier.clickable { onOpen(item) }),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                SubagentInfo(item)
                                if (!item.isGone) {
                                    Icon(
                                        imageVector = Icons.Default.KeyboardArrowRight,
                                        contentDescription = null,
                                        tint = colors.textMuted,
                                        modifier = Modifier.size(16.dp)
                                    )
                                }
                            }
                            if (item.isRunning && canStop) {
                                Box(
                                    modifier = Modifier
                                        .size(36.dp)
                                        .clip(CircleShape)
                                        .clickable {
                                            haptic.medium()
                                            pendingStop = item
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
                        }
                        if (index < items.size - 1) {
                            HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.4f))
                        }
                    }
                }
            }
        }
    }

    pendingStop?.let { target ->
        ConfirmStopSubagentDialog(
            name = target.displayName,
            onConfirm = {
                pendingStop = null
                onStop(target)
            },
            onDismiss = { pendingStop = null }
        )
    }
}

@Composable
private fun SubagentInfo(item: SubagentItem) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            Box(
                modifier = Modifier
                    .size(7.dp)
                    .clip(CircleShape)
                    .background(if (item.isRunning) colors.accentGreen else colors.textMuted.copy(alpha = 0.6f))
            )
            Text(
                text = item.displayName,
                color = if (item.isGone) colors.textSecondary else colors.textPrimary,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )
            item.typeName?.takeIf { it.isNotBlank() && it != item.displayName }?.let { type ->
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                ) {
                    Text(text = type, color = colors.textSecondary, fontSize = 10.sp, fontWeight = FontWeight.Medium, maxLines = 1)
                }
            }
        }
        val meta = buildString {
            append(item.statusText)
            item.stepCount?.takeIf { it > 0 }?.let { if (isNotEmpty()) append(" · "); append("$it 步") }
        }
        if (meta.isNotEmpty()) {
            Text(text = meta, color = colors.textSecondary, fontSize = 11.sp)
        }
        item.prompt?.takeIf { it.isNotBlank() }?.let {
            Text(text = it, color = colors.textSecondary, fontSize = 12.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

@Composable
private fun ConfirmStopSubagentDialog(name: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("关停子代理？") },
        text = { Text("「$name」正在进行的工作会被中断，已完成的内容会保留。") },
        confirmButton = {
            TextButton(onClick = onConfirm) { Text("关停", color = AntigravityTheme.colors.accentRed) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } }
    )
}

/** 子代理会话底部的只读提示条（替代输入栏）。 */
@Composable
fun SubagentReadOnlyBar(
    role: String?,
    isRunning: Boolean,
    canStop: Boolean,
    onStop: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    var confirm by remember { mutableStateOf(false) }
    Surface(color = colors.surface, shadowElevation = 4.dp, modifier = modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text("子代理会话 · 只读", color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                role?.takeIf { it.isNotBlank() }?.let {
                    Text(it, color = colors.textSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            if (isRunning && canStop) {
                TextButton(onClick = { confirm = true }) {
                    Text("关停", color = colors.accentRed, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
    if (confirm) {
        ConfirmStopSubagentDialog(
            name = role?.takeIf { it.isNotBlank() } ?: "子代理",
            onConfirm = { confirm = false; onStop() },
            onDismiss = { confirm = false }
        )
    }
}
