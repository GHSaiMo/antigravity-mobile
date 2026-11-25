package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.FastOutSlowInEasing
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
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ThinkingBlockItem
import com.antigravity.mobile.data.model.ToolCallItem
import com.antigravity.mobile.data.model.ToolDetailItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.ToolLocalization
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.delay
import org.json.JSONObject
import kotlin.math.floor

internal data class NormalizedToolDetail(
    val kind: String,
    val prefix: String,
    val title: String,
    val fileUri: String?,
    val fileName: String?,
    val startLine: Int?,
    val endLine: Int?,
    val command: String?
)

internal object DesktopToolFormatter {
    fun shortThinkingDuration(raw: String?): String? {
        val trimmed = raw?.trim().orEmpty()
        if (trimmed.isEmpty()) return null
        if (trimmed.endsWith("s") && !trimmed.endsWith("ms") && !trimmed.contains("m")) {
            val secsDouble = trimmed.dropLast(1).trim().toDoubleOrNull()
            if (secsDouble != null) {
                val totalSecs = maxOf(1, floor(secsDouble).toInt())
                return if (totalSecs >= 60) {
                    val m = totalSecs / 60
                    val s = totalSecs % 60
                    if (s > 0) "${m}m ${s}s" else "${m}m"
                } else {
                    "${totalSecs}s"
                }
            }
        }
        return trimmed
    }

    fun thinkingTitle(duration: String?, isRunning: Boolean, lang: L10n.AppLang = L10n.currentLang): String {
        if (isRunning) {
            return when (lang) {
                L10n.AppLang.ZH -> "思考中..."
                L10n.AppLang.JA -> "思考中..."
                L10n.AppLang.EN -> "Thinking..."
            }
        }
        val dur = shortThinkingDuration(duration)
        if (!dur.isNullOrBlank()) {
            return when (lang) {
                L10n.AppLang.ZH -> "已思考 $dur"
                L10n.AppLang.JA -> "$dur 思考"
                L10n.AppLang.EN -> "Thought for $dur"
            }
        }
        return when (lang) {
            L10n.AppLang.ZH -> "思考过程"
            L10n.AppLang.JA -> "思考プロセス"
            L10n.AppLang.EN -> "Thought Process"
        }
    }

    fun localizedPrefix(raw: String?, lang: L10n.AppLang = L10n.currentLang): String {
        val p = raw?.trim().orEmpty()
        if (p.isEmpty() || lang == L10n.AppLang.EN) return p
        val zh = lang == L10n.AppLang.ZH
        return when (p.lowercase()) {
            "analyzed" -> if (zh) "已分析" else "分析済み"
            "analyzing" -> if (zh) "正在分析" else "分析中"
            "read" -> if (zh) "已读取" else "読み取り済み"
            "reading" -> if (zh) "正在读取" else "読み取り中"
            "edited" -> if (zh) "已编辑" else "編集済み"
            "editing" -> if (zh) "正在编辑" else "編集中"
            "created" -> if (zh) "已创建" else "作成済み"
            "creating" -> if (zh) "正在创建" else "作成中"
            "deleted" -> if (zh) "已删除" else "削除済み"
            "deleting" -> if (zh) "正在删除" else "削除中"
            "ran" -> if (zh) "已运行" else "実行済み"
            "running" -> if (zh) "正在运行" else "実行中"
            "run" -> if (zh) "运行" else "実行"
            "searched" -> if (zh) "已搜索" else "検索済み"
            "searching" -> if (zh) "正在搜索" else "検索中"
            "listed" -> if (zh) "已浏览" else "一覧取得済み"
            "listing" -> if (zh) "正在浏览" else "一覧取得中"
            "rejected" -> if (zh) "已拒绝" else "拒否済み"
            "canceled" -> if (zh) "已取消" else "キャンセル済み"
            "interrupted" -> if (zh) "已中断" else "中断"
            "errored" -> if (zh) "执行出错" else "エラー"
            "canceled edit to" -> if (zh) "已取消编辑" else "編集キャンセル"
            "canceled creation of" -> if (zh) "已取消创建" else "作成キャンセル"
            "canceled deletion of" -> if (zh) "已取消删除" else "削除キャンセル"
            else -> p
        }
    }

    fun resultsBadge(count: Int, lang: L10n.AppLang = L10n.currentLang): String = when (lang) {
        L10n.AppLang.ZH -> "$count 条结果"
        L10n.AppLang.JA -> "$count 件"
        L10n.AppLang.EN -> if (count == 1) "1 result" else "$count results"
    }

    fun hiddenLinesText(count: Int, lang: L10n.AppLang = L10n.currentLang): String = when (lang) {
        L10n.AppLang.ZH -> "…（上方已省略 $count 行）"
        L10n.AppLang.JA -> "…（上部の $count 行を省略）"
        L10n.AppLang.EN -> "… ($count ${if (count == 1) "line" else "lines"} above hidden)"
    }

    fun lineRangeSuffix(start: Int?, end: Int?): String {
        val s = start ?: 0
        val e = end ?: 0
        return when {
            s > 0 && e > 0 && e != s -> "#L$s-$e"
            s > 0 -> "#L$s"
            e > 0 -> "#L1-$e"
            else -> ""
        }
    }

    fun resolvedKind(item: ToolDetailItem): String {
        item.kind?.takeIf { it.isNotBlank() }?.let { return it }
        val lower = item.name.lowercase()
        return when {
            lower in setOf("run_command", "shell_command", "command", "command_status") -> "terminal"
            lower in setOf("read_file", "view_file", "read_url_content") -> "read"
            lower in setOf(
                "edit_file", "write_file", "write_to_file",
                "replace_file_content", "multi_replace_file_content",
                "create_file", "delete_file", "code_action"
            ) -> "edit"
            lower in setOf("list_dir", "list_directory") -> "list"
            lower.contains("search") || lower.contains("grep") || lower.contains("find") -> "search"
            lower.startsWith("mcp_") || lower in setOf("call_mcp_tool", "mcp_tool") -> "mcp"
            else -> "other"
        }
    }

    fun normalize(item: ToolDetailItem): NormalizedToolDetail {
        val kind = resolvedKind(item)
        var prefix = item.prefix.orEmpty().trim()
        var title = item.title.orEmpty().trim()
        var fileUri = item.fileUri?.takeIf { it.isNotBlank() }
        var fileName = item.fileName?.takeIf { it.isNotBlank() }
        var startLine = item.startLine
        var endLine = item.endLine
        var command = item.command?.takeIf { it.isNotBlank() }

        val rawSummary = item.effectiveSummary.trim()
        if (rawSummary.startsWith("{")) {
            runCatching {
                val obj = JSONObject(rawSummary)
                if (fileName == null) {
                    val path = listOf("Path", "AbsolutePath", "TargetFile", "path", "DirectoryPath")
                        .firstNotNullOfOrNull { k -> obj.optString(k).takeIf { it.isNotBlank() } }
                    if (!path.isNullOrBlank()) {
                        fileUri = fileUri ?: if (path.startsWith("file://")) path else "file://$path"
                        fileName = path.removePrefix("file://").trimEnd('/', '\\').substringAfterLast('/').substringAfterLast('\\')
                    }
                }
                if (startLine == null) {
                    val s = obj.optInt("StartLine", 0)
                    if (s > 0) startLine = s
                }
                if (endLine == null) {
                    val e = obj.optInt("EndLine", 0)
                    if (e > 0) endLine = e
                }
                if (command == null) {
                    command = listOf("CommandLine", "commandLine", "command")
                        .firstNotNullOfOrNull { k -> obj.optString(k).takeIf { it.isNotBlank() } }
                }
                if (title.isEmpty()) {
                    title = listOf("toolSummary", "Query", "query", "Pattern")
                        .firstNotNullOfOrNull { k -> obj.optString(k).takeIf { it.isNotBlank() } }
                        .orEmpty()
                }
            }
        } else if (title.isEmpty() && fileName == null && command == null && rawSummary.isNotEmpty()) {
            if (kind == "terminal") {
                command = rawSummary
            } else {
                title = rawSummary
            }
        }

        if (prefix.isEmpty()) {
            val running = item.status.equals("running", ignoreCase = true)
            prefix = when (kind) {
                "read", "list" -> if (running) "Analyzing" else "Analyzed"
                "edit" -> if (running) "Editing" else "Edited"
                "terminal" -> if (running) "Running" else "Ran"
                "search" -> if (running) "Searching" else "Searched"
                else -> ""
            }
        }

        return NormalizedToolDetail(kind, prefix, title, fileUri, fileName, startLine, endLine, command)
    }

    fun completedSummary(
        toolCount: Int,
        toolNames: List<String>,
        toolCalls: List<ToolCallItem>,
        toolDetails: List<ToolDetailItem>?,
        thinkingText: String?,
        thinkingDuration: String?,
        thinkingBlocks: List<ThinkingBlockItem>?,
        lang: L10n.AppLang = L10n.currentLang
    ): String {
        val details = toolDetails.orEmpty()
        val effectiveCount = maxOf(details.size, toolCount, toolNames.size, toolCalls.size)

        if (effectiveCount == 0) {
            val bestDur = thinkingBlocks?.lastOrNull { !it.duration.isNullOrBlank() }?.duration ?: thinkingDuration
            if (!thinkingText.isNullOrBlank() || !thinkingBlocks.isNullOrEmpty()) {
                return thinkingTitle(bestDur, isRunning = false, lang = lang)
            }
            return when (lang) {
                L10n.AppLang.ZH -> "已完成"
                L10n.AppLang.JA -> "完了"
                L10n.AppLang.EN -> "Done"
            }
        }

        var exploredCount = 0
        val exploredFileKeys = mutableSetOf<String>()
        var exploredFileCount = 0
        var exploredArtifactCount = 0

        var terminalCount = 0

        var editCount = 0
        val editFileKeys = mutableSetOf<String>()
        var editFileCount = 0
        var editArtifactCount = 0

        if (details.isNotEmpty()) {
            for (item in details) {
                if (item.status.equals("error", ignoreCase = true)) continue
                val kind = resolvedKind(item)
                when (kind) {
                    "terminal" -> terminalCount++
                    "edit" -> {
                        editCount++
                        val key = (item.fileUri ?: item.fileName).orEmpty().trim()
                        if (key.isNotEmpty()) {
                            if (editFileKeys.add(key)) {
                                if (item.isArtifact == true) editArtifactCount++ else editFileCount++
                            }
                        } else {
                            editFileCount++
                        }
                    }
                    else -> {
                        exploredCount++
                        if (kind == "read" || kind == "list") {
                            val key = (item.fileUri ?: item.fileName).orEmpty().trim()
                            if (key.isNotEmpty() && exploredFileKeys.add(key)) {
                                if (item.isArtifact == true) exploredArtifactCount++ else exploredFileCount++
                            }
                        }
                    }
                }
            }
        } else {
            val names = if (toolNames.isNotEmpty()) toolNames else toolCalls.map { it.name.ifBlank { it.type } }
            for (raw in names) {
                val lower = raw.lowercase()
                when {
                    lower.contains("command") || lower == "run_command" || lower == "shell_command" -> terminalCount++
                    lower.contains("edit") || lower.contains("write") || lower.contains("replace") ||
                        lower.contains("create_file") || lower.contains("delete_file") -> {
                        editCount++; editFileCount++
                    }
                    else -> {
                        exploredCount++
                        if (lower.contains("file") || lower.contains("read") || lower.contains("view") || lower.contains("dir")) {
                            exploredFileCount++
                        }
                    }
                }
            }
        }

        fun targetNoun(files: Int, artifacts: Int): String = when (lang) {
            L10n.AppLang.ZH -> when {
                files > 0 -> "文件"
                artifacts > 0 -> "文档"
                else -> ""
            }
            L10n.AppLang.JA -> when {
                files > 0 -> "ファイル"
                artifacts > 0 -> "ドキュメント"
                else -> ""
            }
            L10n.AppLang.EN -> when {
                files > 0 -> if (files + artifacts == 1) "file" else "files"
                artifacts > 0 -> if (artifacts == 1) "artifact" else "artifacts"
                else -> ""
            }
        }

        val parts = mutableListOf<String>()
        if (exploredCount > 0) {
            val noun = targetNoun(exploredFileCount, exploredArtifactCount)
            parts.add(
                when (lang) {
                    L10n.AppLang.ZH -> if (noun.isEmpty()) "已探索" else "已浏览$noun"
                    L10n.AppLang.JA -> if (noun.isEmpty()) "調査済み" else "${noun}を調査"
                    L10n.AppLang.EN -> if (noun.isEmpty()) "Explored" else "Explored $noun"
                }
            )
        }
        if (terminalCount > 0) {
            parts.add(
                when (lang) {
                    L10n.AppLang.ZH -> if (parts.isEmpty()) "已运行命令" else "运行命令"
                    L10n.AppLang.JA -> "コマンドを実行"
                    L10n.AppLang.EN -> "${if (parts.isEmpty()) "Ran" else "ran"} ${if (terminalCount == 1) "command" else "commands"}"
                }
            )
        }
        if (editCount > 0) {
            val noun = targetNoun(editFileCount, editArtifactCount)
            parts.add(
                when (lang) {
                    L10n.AppLang.ZH -> {
                        val verb = if (parts.isEmpty()) "已编辑" else "编辑"
                        if (noun.isEmpty()) "${verb}文件" else "$verb$noun"
                    }
                    L10n.AppLang.JA -> if (noun.isEmpty()) "ファイルを編集" else "${noun}を編集"
                    L10n.AppLang.EN -> {
                        val verb = if (parts.isEmpty()) "Edited" else "edited"
                        if (noun.isEmpty()) "$verb files" else "$verb $noun"
                    }
                }
            )
        }

        if (parts.isNotEmpty()) {
            val sep = when (lang) {
                L10n.AppLang.ZH -> "、"
                L10n.AppLang.JA -> "・"
                L10n.AppLang.EN -> ", "
            }
            return parts.joinToString(sep)
        }

        return when (lang) {
            L10n.AppLang.ZH -> "已完成 $effectiveCount 项操作"
            L10n.AppLang.JA -> "$effectiveCount 件の操作を完了"
            L10n.AppLang.EN -> if (effectiveCount == 1) "Completed 1 action" else "Completed $effectiveCount actions"
        }
    }
}

private sealed class DesktopTimelineItem(val key: String, val order: Int, val subOrder: Int) {
    class Thinking(
        key: String,
        order: Int,
        subOrder: Int,
        val text: String,
        val duration: String?,
        val isRunning: Boolean
    ) : DesktopTimelineItem(key, order, subOrder)

    class Tool(
        key: String,
        order: Int,
        subOrder: Int,
        val detail: ToolDetailItem
    ) : DesktopTimelineItem(key, order, subOrder)

    class FallbackTool(
        key: String,
        order: Int,
        subOrder: Int,
        val name: String
    ) : DesktopTimelineItem(key, order, subOrder)
}

@Composable
fun DesktopToolTimelineList(
    thinkingText: String? = null,
    thinkingDuration: String? = null,
    thinkingBlocks: List<ThinkingBlockItem>? = null,
    toolDetails: List<ToolDetailItem>? = null,
    toolCalls: List<ToolCallItem> = emptyList(),
    toolNames: List<String> = emptyList(),
    onFileClick: ((uri: String, title: String) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val entries = remember(thinkingText, thinkingDuration, thinkingBlocks, toolDetails, toolCalls, toolNames) {
        val list = mutableListOf<DesktopTimelineItem>()
        val blocks = thinkingBlocks.orEmpty().filter { it.text.isNotBlank() }
        if (blocks.isNotEmpty()) {
            blocks.forEachIndexed { i, blk ->
                list.add(
                    DesktopTimelineItem.Thinking(
                        key = "think-${blk.stepIndex}-$i",
                        order = blk.stepIndex,
                        subOrder = i * 2,
                        text = blk.text.trim(),
                        duration = blk.duration,
                        isRunning = blk.status.equals("running", ignoreCase = true)
                    )
                )
            }
        } else if (!thinkingText.isNullOrBlank()) {
            list.add(
                DesktopTimelineItem.Thinking(
                    key = "think-legacy",
                    order = -1,
                    subOrder = 0,
                    text = thinkingText.trim(),
                    duration = thinkingDuration,
                    isRunning = false
                )
            )
        }

        val details = toolDetails.orEmpty()
        if (details.isNotEmpty()) {
            details.forEachIndexed { i, d ->
                val stepIdx = d.stepIndex ?: (10_000 + i)
                list.add(
                    DesktopTimelineItem.Tool(
                        key = "tool-$stepIdx-$i",
                        order = stepIdx,
                        subOrder = i * 2 + 1,
                        detail = d
                    )
                )
            }
        } else {
            val fallbackNames = if (toolCalls.isNotEmpty()) {
                toolCalls.map { it.name.ifBlank { it.type } }
            } else {
                toolNames
            }
            fallbackNames.forEachIndexed { i, rawName ->
                list.add(
                    DesktopTimelineItem.FallbackTool(
                        key = "fb-$i",
                        order = 10_000 + i,
                        subOrder = i * 2 + 1,
                        name = rawName
                    )
                )
            }
        }

        list.sortedWith(compareBy<DesktopTimelineItem> { it.order }.thenBy { it.subOrder })
    }

    val hasTools = !toolDetails.isNullOrEmpty() || toolCalls.isNotEmpty() || toolNames.isNotEmpty()
    val colors = AntigravityTheme.colors

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(start = 12.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp)
    ) {
        entries.forEach { entry ->
            when (entry) {
                is DesktopTimelineItem.Thinking -> DesktopThinkingStepRow(
                    text = entry.text,
                    duration = entry.duration,
                    isRunning = entry.isRunning,
                    initiallyExpanded = !hasTools
                )
                is DesktopTimelineItem.Tool -> DesktopToolStepRow(
                    detail = entry.detail,
                    onFileClick = onFileClick
                )
                is DesktopTimelineItem.FallbackTool -> {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 4.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = ToolLocalization.localizedName(entry.name.ifBlank { "tool_call" }),
                            color = colors.textSecondary,
                            fontSize = 12.5.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun DesktopThinkingStepRow(
    text: String,
    duration: String?,
    isRunning: Boolean,
    initiallyExpanded: Boolean
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val currentLang = L10n.currentLang
    var isExpanded by remember { mutableStateOf(initiallyExpanded) }
    val chevronAngle by animateFloatAsState(
        targetValue = if (isExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "desktopThinkChevron"
    )

    val textAlpha = if (isRunning) {
        val transition = rememberInfiniteTransition(label = "thinkBreath")
        val a by transition.animateFloat(
            initialValue = 0.55f,
            targetValue = 1.0f,
            animationSpec = infiniteRepeatable(
                animation = tween(durationMillis = 1200, easing = FastOutSlowInEasing),
                repeatMode = RepeatMode.Reverse
            ),
            label = "thinkBreathAlpha"
        )
        a
    } else {
        1.0f
    }

    val title = remember(duration, isRunning, currentLang) {
        DesktopToolFormatter.thinkingTitle(duration, isRunning, currentLang)
    }

    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(6.dp))
                .clickable {
                    haptic.light()
                    isExpanded = !isExpanded
                }
                .padding(horizontal = 4.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            Text(
                text = title,
                color = colors.textSecondary.copy(alpha = textAlpha),
                fontSize = 12.5.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Icon(
                imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                contentDescription = if (isExpanded) "Collapse" else "Expand",
                tint = colors.textSecondary.copy(alpha = 0.65f),
                modifier = Modifier
                    .size(13.dp)
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
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(IntrinsicSize.Min)
                    .padding(start = 6.dp, end = 4.dp, top = 2.dp, bottom = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(
                    modifier = Modifier
                        .width(1.5.dp)
                        .fillMaxHeight()
                        .background(colors.textSecondary.copy(alpha = 0.24f))
                )
                SelectionContainer(modifier = Modifier.weight(1f)) {
                    Text(
                        text = text,
                        color = colors.textSecondary,
                        fontSize = 12.sp,
                        lineHeight = 17.sp
                    )
                }
            }
        }
    }
}

@Composable
private fun DesktopToolStepRow(
    detail: ToolDetailItem,
    onFileClick: ((uri: String, title: String) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val currentLang = L10n.currentLang
    val norm = remember(detail) { DesktopToolFormatter.normalize(detail) }

    val isRunning = detail.status.equals("running", ignoreCase = true)
    val isError = detail.status.lowercase().let { it.contains("err") || it.contains("fail") } ||
        (detail.exitCode != null && detail.exitCode != 0)
    val hasTerminalCard = !norm.command.isNullOrBlank() ||
        !detail.output.isNullOrBlank() ||
        (detail.numLinesAbove ?: 0) > 0
    val hasMarkdown = !detail.markdown.isNullOrBlank()
    val hasErrorBody = !detail.errorText.isNullOrBlank()
    val isExpandable = hasTerminalCard || hasMarkdown || hasErrorBody

    var isExpanded by remember(detail.stepIndex, detail.name) { mutableStateOf(false) }
    val chevronAngle by animateFloatAsState(
        targetValue = if (isExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "desktopStepChevron"
    )

    val textAlpha = if (isRunning) {
        val transition = rememberInfiniteTransition(label = "stepBreath")
        val a by transition.animateFloat(
            initialValue = 0.55f,
            targetValue = 1.0f,
            animationSpec = infiniteRepeatable(
                animation = tween(durationMillis = 1200, easing = FastOutSlowInEasing),
                repeatMode = RepeatMode.Reverse
            ),
            label = "stepBreathAlpha"
        )
        a
    } else {
        1.0f
    }

    val prefixLabel = remember(norm.prefix, currentLang) {
        DesktopToolFormatter.localizedPrefix(norm.prefix, currentLang)
    }

    val canOpenFile = !isExpandable && !norm.fileUri.isNullOrBlank() && onFileClick != null

    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(6.dp))
                .then(
                    when {
                        isExpandable -> Modifier.clickable {
                            haptic.light()
                            isExpanded = !isExpanded
                        }
                        canOpenFile -> Modifier.clickable {
                            haptic.light()
                            onFileClick?.invoke(norm.fileUri!!, norm.fileName ?: norm.fileUri)
                        }
                        else -> Modifier
                    }
                )
                .padding(horizontal = 4.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            if (prefixLabel.isNotEmpty()) {
                Text(
                    text = prefixLabel,
                    color = colors.textSecondary.copy(alpha = textAlpha),
                    fontSize = 12.5.sp,
                    maxLines = 1
                )
            }

            if (!norm.fileName.isNullOrBlank()) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(3.5.dp),
                    modifier = Modifier.weight(1f, fill = false)
                ) {
                    Icon(
                        imageVector = if (norm.kind == "list") Icons.Default.Folder else Icons.Default.Description,
                        contentDescription = null,
                        tint = colors.textSecondary.copy(alpha = 0.85f * textAlpha),
                        modifier = Modifier.size(13.dp)
                    )
                    Text(
                        text = norm.fileName + DesktopToolFormatter.lineRangeSuffix(norm.startLine, norm.endLine),
                        color = colors.textSecondary.copy(alpha = textAlpha),
                        fontSize = 12.5.sp,
                        fontWeight = FontWeight.Medium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                if (detail.hasDiffStats == true || (detail.additions ?: 0) > 0 || (detail.deletions ?: 0) > 0) {
                    Row(horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                        Text(
                            text = "+${detail.additions ?: 0}",
                            color = Color(0xFF22C55E),
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium,
                            fontFamily = FontFamily.Monospace
                        )
                        Text(
                            text = "-${detail.deletions ?: 0}",
                            color = Color(0xFFEF4444),
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                }
            } else if (norm.title.isNotEmpty()) {
                Text(
                    text = norm.title,
                    color = colors.textSecondary.copy(alpha = textAlpha),
                    fontSize = 12.5.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
            } else if (!norm.command.isNullOrBlank()) {
                Text(
                    text = norm.command,
                    color = colors.textSecondary.copy(alpha = textAlpha),
                    fontSize = 11.5.sp,
                    fontFamily = FontFamily.Monospace,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
            } else if (prefixLabel.isEmpty()) {
                Text(
                    text = ToolLocalization.localizedName(detail.name.ifBlank { "tool_call" }),
                    color = colors.textSecondary.copy(alpha = textAlpha),
                    fontSize = 12.5.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
            }

            if ((detail.numResults ?: 0) > 0 && norm.kind == "search") {
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(colors.textSecondary.copy(alpha = 0.12f))
                        .padding(horizontal = 6.dp, vertical = 1.5.dp)
                ) {
                    Text(
                        text = DesktopToolFormatter.resultsBadge(detail.numResults ?: 0, currentLang),
                        color = colors.textSecondary,
                        fontSize = 10.5.sp
                    )
                }
            }

            if (isError) {
                Box(
                    modifier = Modifier
                        .size(6.dp)
                        .clip(CircleShape)
                        .background(colors.accentRed)
                )
            }

            if (isExpandable) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                    contentDescription = if (isExpanded) "Collapse" else "Expand",
                    tint = colors.textSecondary.copy(alpha = 0.65f),
                    modifier = Modifier
                        .size(13.dp)
                        .rotate(chevronAngle)
                )
            }
        }

        if (isExpandable) {
            AnimatedVisibility(
                visible = isExpanded,
                enter = expandVertically(
                    animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
                ) + fadeIn(),
                exit = shrinkVertically(
                    animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMediumLow)
                ) + fadeOut()
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 4.dp, bottom = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (hasTerminalCard) {
                        DesktopTerminalCard(
                            command = norm.command.orEmpty(),
                            cwd = detail.cwd,
                            output = detail.output,
                            numLinesAbove = detail.numLinesAbove ?: 0,
                            exitCode = detail.exitCode
                        )
                    }
                    detail.markdown?.trim()?.takeIf { it.isNotEmpty() }?.let { md ->
                        SelectionContainer {
                            Text(
                                text = md,
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                lineHeight = 17.sp,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(start = 8.dp, top = 2.dp, bottom = 2.dp)
                            )
                        }
                    }
                    detail.errorText?.trim()?.takeIf { it.isNotEmpty() }?.let { err ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.accentRed.copy(alpha = 0.08f))
                                .padding(horizontal = 10.dp, vertical = 7.dp),
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            verticalAlignment = Alignment.Top
                        ) {
                            Icon(
                                imageVector = Icons.Default.ErrorOutline,
                                contentDescription = "Error",
                                tint = colors.accentRed,
                                modifier = Modifier
                                    .padding(top = 1.dp)
                                    .size(13.dp)
                            )
                            SelectionContainer(modifier = Modifier.weight(1f)) {
                                Text(
                                    text = err,
                                    color = colors.accentRed,
                                    fontSize = 11.5.sp,
                                    lineHeight = 16.sp
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun DesktopTerminalCard(
    command: String,
    cwd: String?,
    output: String?,
    numLinesAbove: Int,
    exitCode: Int?
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val clipboard = LocalClipboardManager.current
    val currentLang = L10n.currentLang
    var copied by remember { mutableStateOf(false) }

    LaunchedEffect(copied) {
        if (copied) {
            delay(1500L)
            copied = false
        }
    }

    val hasOutput = !output.isNullOrEmpty() || numLinesAbove > 0
    val isNonZeroExit = exitCode != null && exitCode != 0

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surface.copy(alpha = if (colors.isDark) 0.85f else 0.94f))
            .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
    ) {
        if (command.isNotEmpty()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 10.dp, vertical = 7.dp),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text(
                    text = if (!cwd.isNullOrBlank()) "$cwd $" else "$",
                    color = colors.textSecondary,
                    fontSize = 11.sp,
                    fontFamily = FontFamily.Monospace
                )
                SelectionContainer(modifier = Modifier.weight(1f)) {
                    Text(
                        text = command,
                        color = colors.textPrimary.copy(alpha = 0.9f),
                        fontSize = 11.5.sp,
                        lineHeight = 16.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Icon(
                        imageVector = if (copied) Icons.Default.Check else Icons.Default.ContentCopy,
                        contentDescription = "Copy command",
                        tint = if (copied) colors.accentGreen else colors.textSecondary,
                        modifier = Modifier
                            .size(15.dp)
                            .clickable {
                                clipboard.setText(AnnotatedString(command))
                                haptic.light()
                                copied = true
                            }
                    )
                    if (isNonZeroExit) {
                        Icon(
                            imageVector = Icons.Default.ErrorOutline,
                            contentDescription = "Exit error",
                            tint = colors.accentRed,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                }
            }
        }

        if (hasOutput) {
            if (command.isNotEmpty()) {
                HorizontalDivider(thickness = 0.6.dp, color = colors.border.copy(alpha = 0.6f))
            }
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 200.dp)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 10.dp, vertical = 7.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                if (numLinesAbove > 0) {
                    Text(
                        text = DesktopToolFormatter.hiddenLinesText(numLinesAbove, currentLang),
                        color = colors.textSecondary,
                        fontSize = 10.5.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }
                if (!output.isNullOrEmpty()) {
                    SelectionContainer {
                        Text(
                            text = output,
                            color = colors.textPrimary.copy(alpha = 0.85f),
                            fontSize = 11.sp,
                            lineHeight = 15.sp,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                }
            }
        }
    }
}

@Composable
fun ToolStepCollapseCard(
    toolCalls: List<ToolCallItem>,
    thinkingText: String? = null,
    thinkingDuration: String? = null,
    thinkingBlocks: List<ThinkingBlockItem>? = null,
    toolDetails: List<ToolDetailItem>? = null,
    toolNames: List<String> = emptyList(),
    toolCount: Int = 0,
    onFileClick: ((uri: String, title: String) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val hasAny = toolCalls.isNotEmpty() ||
        !toolDetails.isNullOrEmpty() ||
        toolNames.isNotEmpty() ||
        toolCount > 0 ||
        !thinkingText.isNullOrBlank() ||
        !thinkingBlocks.isNullOrEmpty()
    if (!hasAny) return

    var isExpanded by remember { mutableStateOf(false) }
    val colors = AntigravityTheme.colors
    val currentLang = L10n.currentLang
    val haptic = rememberHaptic()

    val summaryText = remember(
        toolCount, toolNames, toolCalls, toolDetails, thinkingText, thinkingDuration, thinkingBlocks, currentLang
    ) {
        DesktopToolFormatter.completedSummary(
            toolCount = toolCount,
            toolNames = toolNames,
            toolCalls = toolCalls,
            toolDetails = toolDetails,
            thinkingText = thinkingText,
            thinkingDuration = thinkingDuration,
            thinkingBlocks = thinkingBlocks,
            lang = currentLang
        )
    }

    val chevronAngle by animateFloatAsState(
        targetValue = if (isExpanded) 90f else 0f,
        animationSpec = spring(dampingRatio = 0.85f, stiffness = Spring.StiffnessMedium),
        label = "completedToolChevron"
    )

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(8.dp))
                .clickable {
                    haptic.light()
                    isExpanded = !isExpanded
                }
                .padding(horizontal = 4.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = summaryText,
                color = colors.textSecondary,
                fontSize = 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )
            Icon(
                imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                contentDescription = if (isExpanded) "Collapse" else "Expand",
                tint = colors.textSecondary.copy(alpha = 0.65f),
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
