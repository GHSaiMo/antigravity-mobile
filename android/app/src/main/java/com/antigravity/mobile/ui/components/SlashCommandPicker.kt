package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

/** 输入框里键入 "/" 后弹出的命令列表（系统命令 + 技能），与桌面端的斜杠菜单同源。 */
@Composable
fun SlashCommandPicker(
    commands: List<SlashCommandOption>,
    isLoading: Boolean,
    onSelect: (SlashCommandOption) -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val shape = RoundedCornerShape(14.dp)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp)
            .clip(shape)
            .background(colors.surface)
            .border(0.5.dp, colors.border, shape)
    ) {
        if (commands.isEmpty()) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (isLoading) {
                    CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                }
                Text(
                    text = if (isLoading) "正在读取命令..." else "没有匹配的命令",
                    color = colors.textMuted,
                    fontSize = 13.sp
                )
            }
        } else {
            LazyColumn(modifier = Modifier.heightIn(max = 264.dp)) {
                items(commands, key = { it.name }) { command ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable {
                                haptic.light()
                                onSelect(command)
                            }
                            .padding(horizontal = 14.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.Top,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(
                            imageVector = if (command.kind == "skill") Icons.Default.AutoAwesome else Icons.Default.Bolt,
                            contentDescription = null,
                            tint = if (command.kind == "skill") colors.accentOrange else colors.accentIndigo,
                            modifier = Modifier.padding(top = 2.dp).size(16.dp)
                        )
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = "/${command.name}",
                                color = colors.textPrimary,
                                fontSize = 14.5.sp,
                                fontWeight = FontWeight.SemiBold,
                                fontFamily = FontFamily.Monospace
                            )
                            if (command.description.isNotBlank()) {
                                Text(
                                    text = command.description.lineSequence().first().trim(),
                                    color = colors.textSecondary,
                                    fontSize = 12.5.sp,
                                    lineHeight = 17.sp,
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

/** 已选中的命令标签："/plan ✕"，发送时随消息一起带上。 */
@Composable
fun SlashCommandChip(
    command: SlashCommandOption,
    onClear: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val shape = RoundedCornerShape(10.dp)
    Row(
        modifier = modifier
            .padding(horizontal = 16.dp)
            .clip(shape)
            .background(colors.accentIndigo.copy(alpha = 0.12f))
            .border(1.dp, colors.accentIndigo.copy(alpha = 0.35f), shape)
            .clickable(onClick = onClear)
            .padding(start = 10.dp, end = 8.dp, top = 5.dp, bottom = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Text(
            text = "/${command.name}",
            color = colors.accentIndigo,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace
        )
        Icon(
            imageVector = Icons.Default.Close,
            contentDescription = "移除命令",
            tint = colors.accentIndigo,
            modifier = Modifier.size(14.dp)
        )
    }
}
