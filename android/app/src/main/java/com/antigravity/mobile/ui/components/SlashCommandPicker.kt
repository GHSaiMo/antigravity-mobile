package com.antigravity.mobile.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
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
                        SlashCommandIcon(
                            name = command.name,
                            icon = command.icon,
                            kind = command.kind,
                            tint = colors.textSecondary,
                            size = 16.dp,
                            modifier = Modifier.padding(top = 2.dp)
                        )
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
                                    fontFamily = FontFamily.Monospace
                                )
                                Box(
                                    modifier = Modifier
                                        .clip(CircleShape)
                                        .background(colors.surfaceVariant)
                                        .padding(horizontal = 6.dp, vertical = 1.dp)
                                ) {
                                    Text(
                                        text = if (command.kind == "skill") "技能" else "系统",
                                        color = colors.textMuted,
                                        fontSize = 10.5.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                }
                            }
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
            .padding(start = 8.dp, end = 8.dp, top = 5.dp, bottom = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        SlashCommandIcon(
            name = command.name,
            icon = command.icon,
            kind = command.kind,
            tint = colors.accentIndigo.copy(alpha = 0.85f),
            size = 14.dp
        )
        Text(
            text = command.name,
            color = colors.accentIndigo,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace
        )
        Icon(
            imageVector = Icons.Default.Close,
            contentDescription = "移除命令",
            tint = colors.accentIndigo,
            modifier = Modifier.size(12.dp)
        )
    }
}
