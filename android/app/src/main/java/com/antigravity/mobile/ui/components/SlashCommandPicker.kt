package com.antigravity.mobile.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SlashCommandIconCatalog
import com.antigravity.mobile.data.model.SlashCommandMention
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.data.model.SlashCommandTextTokenizer
import com.antigravity.mobile.data.model.SlashTextSegment
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import java.util.concurrent.ConcurrentHashMap

object SlashCommandPathCache {
    private val cache = ConcurrentHashMap<String, Path>()

    fun getPath(iconKey: String): Path {
        val resolved = if (SlashCommandIconCatalog.svgPaths.containsKey(iconKey)) iconKey else "code_blocks"
        return cache.computeIfAbsent(resolved) { key ->
            val d = SlashCommandIconCatalog.svgPaths[key] ?: SlashCommandIconCatalog.svgPaths["code_blocks"]!!
            PathParser().parsePathString(d).toPath()
        }
    }
}

/** 桌面端（Antigravity Desktop）对齐的斜杠命令与技能图标 */
@Composable
fun SlashCommandIcon(
    name: String,
    icon: String? = null,
    kind: String? = null,
    tint: Color = Color.Unspecified,
    size: Dp = 16.dp,
    modifier: Modifier = Modifier
) {
    val key = remember(name, icon, kind) {
        SlashCommandIconCatalog.resolveIcon(name, icon, kind)
    }
    val path = remember(key) {
        SlashCommandPathCache.getPath(key)
    }
    val effectiveColor = if (tint != Color.Unspecified) tint else LocalContentColor.current
    Canvas(modifier = modifier.size(size)) {
        val s = this.size.minDimension / 960f
        scale(scaleX = s, scaleY = s, pivot = Offset.Zero) {
            drawPath(path = path, color = effectiveColor)
        }
    }
}

object SlashCommandTextRenderer {
    fun buildInlineText(
        text: String,
        mentions: List<SlashCommandMention> = emptyList(),
        availableCommands: List<SlashCommandOption> = emptyList(),
        iconColor: Color = Color.Unspecified
    ): Pair<AnnotatedString, Map<String, InlineTextContent>> {
        val segments = SlashCommandTextTokenizer.tokenize(text, mentions, availableCommands)
        if (segments.isEmpty()) {
            return AnnotatedString(text) to emptyMap()
        }
        val inlineMap = mutableMapOf<String, InlineTextContent>()
        val annotated = buildAnnotatedString {
            for (seg in segments) {
                when (seg) {
                    is SlashTextSegment.Text -> append(seg.content)
                    is SlashTextSegment.Slash -> {
                        val inlineId = "slash_${seg.mention.name}_${inlineMap.size}"
                        appendInlineContent(inlineId, alternateText = "/${seg.mention.name}")
                        append(" ")
                        withStyle(SpanStyle(fontWeight = FontWeight.SemiBold)) {
                            append(seg.mention.name)
                        }
                        inlineMap[inlineId] = InlineTextContent(
                            placeholder = Placeholder(
                                width = 16.sp,
                                height = 16.sp,
                                placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                            )
                        ) {
                            SlashCommandIcon(
                                name = seg.mention.name,
                                icon = seg.mention.icon,
                                kind = seg.mention.kind,
                                tint = iconColor,
                                size = 15.dp
                            )
                        }
                    }
                }
            }
        }
        return annotated to inlineMap
    }
}

/** 输入框里键入 "/" 后浮在会话界面底部的命令气泡弹窗（系统命令 + 技能），与桌面端的斜杠菜单同源。 */
@Composable
fun SlashCommandPicker(
    commands: List<SlashCommandOption>,
    isLoading: Boolean,
    onSelect: (SlashCommandOption) -> Unit,
    onDismiss: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val shape = RoundedCornerShape(18.dp)

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp),
        shape = shape,
        color = colors.surface,
        tonalElevation = 8.dp,
        shadowElevation = 14.dp,
        border = androidx.compose.foundation.BorderStroke(0.8.dp, colors.border)
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            // 顶部标题栏 + 右上角关闭按钮
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(18.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = "/",
                        color = Color.White,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace
                    )
                }

                Text(
                    text = "斜杠命令与技能",
                    color = colors.textPrimary,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold
                )

                if (commands.isNotEmpty()) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.14f))
                            .padding(horizontal = 6.dp, vertical = 1.5.dp)
                    ) {
                        Text(
                            text = "${commands.size}",
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
                        contentDescription = "关闭斜杠命令气泡",
                        tint = colors.textSecondary,
                        modifier = Modifier.size(14.dp)
                    )
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.65f))

            if (commands.isEmpty()) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 20.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    if (isLoading) {
                        CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                    }
                    Text(
                        text = if (isLoading) "正在读取命令与技能..." else "没有匹配的命令或技能",
                        color = colors.textMuted,
                        fontSize = 13.5.sp
                    )
                }
            } else {
                LazyColumn(
                    modifier = Modifier
                        .weight(1f, fill = false)
                        .heightIn(max = 260.dp)
                ) {
                    itemsIndexed(commands, key = { _, it -> it.name }) { index, command ->
                        val isSkill = command.kind == "skill"
                        val accentColor = if (isSkill) colors.accentIndigo else colors.accentBlue
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    haptic.light()
                                    onSelect(command)
                                }
                                .padding(horizontal = 14.dp, vertical = 11.dp),
                            verticalAlignment = Alignment.Top,
                            horizontalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .padding(top = 1.dp)
                                    .size(32.dp)
                                    .clip(RoundedCornerShape(9.dp))
                                    .background(accentColor.copy(alpha = 0.13f)),
                                contentAlignment = Alignment.Center
                            ) {
                                SlashCommandIcon(
                                    name = command.name,
                                    icon = command.icon,
                                    kind = command.kind,
                                    tint = accentColor,
                                    size = 16.dp
                                )
                            }
                            Column(modifier = Modifier.weight(1f)) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Text(
                                        text = "/${command.name}",
                                        color = colors.textPrimary,
                                        fontSize = 14.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        fontFamily = FontFamily.Monospace,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Box(
                                        modifier = Modifier
                                            .clip(CircleShape)
                                            .background(accentColor.copy(alpha = 0.12f))
                                            .padding(horizontal = 6.dp, vertical = 1.5.dp)
                                    ) {
                                        Text(
                                            text = if (isSkill) "技能" else "系统",
                                            color = accentColor,
                                            fontSize = 10.5.sp,
                                            fontWeight = FontWeight.SemiBold
                                        )
                                    }
                                }
                                if (command.description.isNotBlank()) {
                                    Spacer(modifier = Modifier.height(3.dp))
                                    Text(
                                        text = command.description.lineSequence().first().trim(),
                                        color = colors.textSecondary,
                                        fontSize = 12.5.sp,
                                        lineHeight = 17.5.sp,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }
                        }

                        if (index < commands.lastIndex) {
                            HorizontalDivider(
                                modifier = Modifier.padding(start = 58.dp),
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

/** 已选中的命令标签：与 Gemini 模型按钮同高同行的胶囊按钮，右上角带小叉，发送时随消息一起带上。 */
@Composable
fun SlashCommandChip(
    command: SlashCommandOption,
    onClear: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val shape = RoundedCornerShape(10.dp)
    Box(
        modifier = modifier.padding(end = 4.dp)
    ) {
        Row(
            modifier = Modifier
                .height(32.dp)
                .clip(shape)
                .background(colors.accentIndigo.copy(alpha = 0.12f))
                .border(1.dp, colors.accentIndigo.copy(alpha = 0.35f), shape)
                .clickable(onClick = onClear)
                .padding(horizontal = 11.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            SlashCommandIcon(
                name = command.name,
                icon = command.icon,
                kind = command.kind,
                tint = colors.accentIndigo.copy(alpha = 0.9f),
                size = 14.dp
            )
            Text(
                text = command.name,
                color = colors.accentIndigo,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium
            )
        }

        Box(
            modifier = Modifier
                .size(16.dp)
                .align(Alignment.TopEnd)
                .offset(x = 5.dp, y = (-5).dp)
                .clip(CircleShape)
                .background(colors.surface)
                .clickable(onClick = onClear),
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
                    contentDescription = "移除命令",
                    tint = Color.White,
                    modifier = Modifier.size(9.dp)
                )
            }
        }
    }
}
