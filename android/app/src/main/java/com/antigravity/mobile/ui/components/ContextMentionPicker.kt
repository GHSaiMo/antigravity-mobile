package com.antigravity.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Chat
import androidx.compose.material.icons.automirrored.filled.ManageSearch
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ContextMentionItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

/** 输入框里键入 "@" 后浮在会话界面底部的上下文引用浮窗（文件 / 目录 / 关联会话），与 iOS 端对齐。 */
@Composable
fun ContextMentionPicker(
    candidates: List<ContextMentionItem>,
    selectedItems: List<ContextMentionItem>,
    query: String,
    isLoading: Boolean,
    maxHeight: Dp = 380.dp,
    onQueryChange: (String) -> Unit,
    onSelect: (ContextMentionItem) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val shape = RoundedCornerShape(18.dp)
    val maxListHeight = (maxHeight - 98.dp).coerceAtLeast(160.dp)
    val minListHeight = minOf(200.dp, maxListHeight)

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 10.dp),
        shape = shape,
        color = colors.surface,
        tonalElevation = 8.dp,
        shadowElevation = 14.dp,
        border = BorderStroke(0.8.dp, colors.border)
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            // 顶部标题栏 + 数量徽标 + 关闭按钮
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(20.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = "@",
                        color = Color.White,
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace
                    )
                }

                Text(
                    text = "引用文件或关联会话",
                    color = colors.textPrimary,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold
                )

                if (candidates.isNotEmpty()) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.14f))
                            .padding(horizontal = 6.dp, vertical = 1.5.dp)
                    ) {
                        Text(
                            text = "${candidates.size}",
                            color = colors.accentIndigo,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }

                Spacer(modifier = Modifier.weight(1f))

                Box(
                    modifier = Modifier
                        .size(26.dp)
                        .clip(CircleShape)
                        .background(colors.surfaceVariant)
                        .clickable {
                            haptic.light()
                            onDismiss()
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = "关闭引用选择浮窗",
                        tint = colors.textSecondary,
                        modifier = Modifier.size(14.dp)
                    )
                }
            }

            // 搜索输入框
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 12.dp, end = 12.dp, bottom = 10.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surfaceVariant)
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Search,
                    contentDescription = null,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(16.dp)
                )
                Box(modifier = Modifier.weight(1f)) {
                    if (query.isEmpty()) {
                        Text(
                            text = "搜索文件、目录或关联会话...",
                            color = colors.textMuted,
                            fontSize = 14.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    BasicTextField(
                        value = query,
                        onValueChange = onQueryChange,
                        singleLine = true,
                        textStyle = TextStyle(
                            color = colors.textPrimary,
                            fontSize = 14.sp
                        ),
                        cursorBrush = SolidColor(colors.accentIndigo),
                        modifier = Modifier.fillMaxWidth()
                    )
                }
                if (isLoading) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = colors.accentIndigo
                    )
                } else if (query.isNotEmpty()) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = "清空搜索",
                        tint = colors.textSecondary,
                        modifier = Modifier
                            .size(16.dp)
                            .clickable { onQueryChange("") }
                    )
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.65f))

            if (candidates.isEmpty()) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(minListHeight),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center
                ) {
                    if (isLoading) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(22.dp),
                            strokeWidth = 2.5.dp,
                            color = colors.accentIndigo
                        )
                    } else {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ManageSearch,
                            contentDescription = null,
                            tint = colors.textMuted.copy(alpha = 0.7f),
                            modifier = Modifier.size(28.dp)
                        )
                    }
                    Spacer(modifier = Modifier.height(10.dp))
                    Text(
                        text = if (isLoading) "正在检索工作区上下文..." else "暂无匹配的文件、目录或关联会话",
                        color = colors.textMuted,
                        fontSize = 13.5.sp
                    )
                }
            } else {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = minListHeight, max = maxListHeight)
                ) {
                    itemsIndexed(candidates, key = { index, it -> "${it.id}_$index" }) { index, item ->
                        val style = mentionVisualStyle(item.type)
                        val isAlreadySelected = selectedItems.any { it.id == item.id && it.label == item.label }

                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    haptic.light()
                                    onSelect(item)
                                }
                                .padding(horizontal = 14.dp, vertical = 11.dp),
                            verticalAlignment = Alignment.Top,
                            horizontalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .padding(top = 1.dp)
                                    .size(34.dp)
                                    .clip(RoundedCornerShape(9.dp))
                                    .background(style.accentColor.copy(alpha = 0.13f)),
                                contentAlignment = Alignment.Center
                            ) {
                                Icon(
                                    imageVector = style.icon,
                                    contentDescription = style.tag,
                                    tint = style.accentColor,
                                    modifier = Modifier.size(17.dp)
                                )
                            }

                            Column(
                                modifier = Modifier.weight(1f),
                                verticalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Text(
                                        text = item.label,
                                        color = colors.textPrimary,
                                        fontSize = 14.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f, fill = false)
                                    )
                                    Box(
                                        modifier = Modifier
                                            .clip(CircleShape)
                                            .background(style.accentColor.copy(alpha = 0.12f))
                                            .padding(horizontal = 6.dp, vertical = 1.5.dp)
                                    ) {
                                        Text(
                                            text = style.tag,
                                            color = style.accentColor,
                                            fontSize = 10.5.sp,
                                            fontWeight = FontWeight.SemiBold
                                        )
                                    }
                                }

                                val detail = (item.detail ?: item.path).orEmpty()
                                if (detail.isNotBlank()) {
                                    Text(
                                        text = detail,
                                        color = colors.textSecondary,
                                        fontSize = 12.sp,
                                        fontFamily = FontFamily.Monospace,
                                        lineHeight = 16.sp,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }

                            if (isAlreadySelected) {
                                Icon(
                                    imageVector = Icons.Default.CheckCircle,
                                    contentDescription = "已选中",
                                    tint = colors.accentIndigo,
                                    modifier = Modifier
                                        .padding(top = 2.dp)
                                        .size(18.dp)
                                )
                            }
                        }

                        if (index < candidates.lastIndex) {
                            HorizontalDivider(
                                modifier = Modifier.padding(start = 60.dp),
                                thickness = 0.5.dp,
                                color = colors.border.copy(alpha = 0.45f)
                            )
                        }
                    }
                }
            }
        }
    }
}

private data class MentionVisualStyle(
    val icon: ImageVector,
    val accentColor: Color,
    val tag: String
)

@Composable
private fun mentionVisualStyle(type: String): MentionVisualStyle {
    val colors = AntigravityTheme.colors
    return when (type) {
        "directory" -> MentionVisualStyle(
            icon = Icons.Default.Folder,
            accentColor = colors.accentBlue,
            tag = "目录"
        )
        "conversation" -> MentionVisualStyle(
            icon = Icons.AutoMirrored.Filled.Chat,
            accentColor = colors.accentGreen,
            tag = "会话"
        )
        else -> MentionVisualStyle(
            icon = Icons.Default.Description,
            accentColor = colors.accentIndigo,
            tag = "文件"
        )
    }
}
