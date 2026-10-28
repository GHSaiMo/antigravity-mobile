package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.model.RevertDiffLine
import com.antigravity.mobile.data.model.RevertPreviewFile
import com.antigravity.mobile.ui.theme.AntigravityTheme

data class DiffSpan(
    val text: String,
    val isChanged: Boolean
)

data class ProcessedDiffLine(
    val id: Int,
    val line: RevertDiffLine,
    val spans: List<DiffSpan>
)

@Composable
fun DiffViewerDialog(
    file: RevertPreviewFile,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val processedLines = remember(file.diffLines) { processDiffLines(file.diffLines) }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = modifier
                .fillMaxWidth(0.95f)
                .fillMaxHeight(0.85f)
                .clip(RoundedCornerShape(16.dp))
                .border(0.5.dp, colors.border, RoundedCornerShape(16.dp)),
            shape = RoundedCornerShape(16.dp),
            color = colors.surface
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Header
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            ActionTypeBadge(actionType = file.actionType)
                            Text(
                                text = file.fileName,
                                fontSize = 15.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        if (file.fileUri.isNotBlank()) {
                            Text(
                                text = file.fileUri,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textMuted,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }

                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        if (file.additions > 0) {
                            Text(
                                text = "+${file.additions}",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace,
                                color = Color(0xFF4CAF50)
                            )
                        }
                        if (file.deletions > 0) {
                            Text(
                                text = "-${file.deletions}",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace,
                                color = Color(0xFFF44336)
                            )
                        }
                        IconButton(
                            onClick = onDismiss,
                            modifier = Modifier.size(32.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Close,
                                contentDescription = "Close",
                                tint = colors.textSecondary,
                                modifier = Modifier.size(18.dp)
                            )
                        }
                    }
                }

                HorizontalDivider(color = colors.border, thickness = 0.5.dp)

                // Content
                if (file.diffLines.isEmpty()) {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(24.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "无行级别代码差异",
                            fontSize = 13.sp,
                            color = colors.textMuted
                        )
                    }
                } else {
                    val horizontalScrollState = rememberScrollState()
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxSize()
                            .horizontalScroll(horizontalScrollState)
                            .padding(vertical = 4.dp)
                    ) {
                        itemsIndexed(processedLines) { _, item ->
                            DiffLineRow(item = item)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ActionTypeBadge(
    actionType: String,
    modifier: Modifier = Modifier
) {
    val (bgColor, textColor, label) = when (actionType.uppercase()) {
        "CREATE" -> Triple(Color(0x264CAF50), Color(0xFF4CAF50), "新增")
        "DELETE" -> Triple(Color(0x26F44336), Color(0xFFF44336), "删除")
        else -> Triple(Color(0x262196F3), Color(0xFF2196F3), "修改")
    }

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(4.dp))
            .background(bgColor)
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = label,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold,
            color = textColor
        )
    }
}

@Composable
private fun DiffLineRow(
    item: ProcessedDiffLine,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val line = item.line
    val isInsert = line.type.equals("INSERT", ignoreCase = true)
    val isDelete = line.type.equals("DELETE", ignoreCase = true)

    val bgColor = when {
        isInsert -> Color(0x264CAF50)
        isDelete -> Color(0x26F44336)
        else -> Color.Transparent
    }

    val textColor = when {
        isInsert -> Color(0xFF4CAF50)
        isDelete -> Color(0xFFF44336)
        else -> colors.textPrimary
    }

    val prefix = when {
        isInsert -> "+"
        isDelete -> "-"
        else -> " "
    }

    val highlightBg = when {
        isInsert -> Color(0x594CAF50)
        isDelete -> Color(0x59F44336)
        else -> Color.Transparent
    }

    val annotatedString = buildAnnotatedString {
        for (span in item.spans) {
            if (span.isChanged) {
                withStyle(SpanStyle(background = highlightBg)) {
                    append(span.text)
                }
            } else {
                append(span.text)
            }
        }
    }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .background(bgColor)
            .padding(horizontal = 12.dp, vertical = 2.dp),
        verticalAlignment = Alignment.Top
    ) {
        Text(
            text = prefix,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            fontWeight = FontWeight.Bold,
            color = textColor,
            modifier = Modifier.width(16.dp)
        )
        Text(
            text = annotatedString,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            color = textColor,
            softWrap = false
        )
    }
}

fun processDiffLines(lines: List<RevertDiffLine>): List<ProcessedDiffLine> {
    val result = mutableListOf<ProcessedDiffLine>()
    var i = 0
    val n = lines.size
    while (i < n) {
        val type = lines[i].type.uppercase()
        if (type == "DELETE") {
            var delEnd = i
            while (delEnd < n && lines[delEnd].type.uppercase() == "DELETE") {
                delEnd++
            }
            var insEnd = delEnd
            while (insEnd < n && lines[insEnd].type.uppercase() == "INSERT") {
                insEnd++
            }

            val delCount = delEnd - i
            val insCount = insEnd - delEnd

            if (insCount > 0) {
                val pairCount = minOf(delCount, insCount)
                for (k in 0 until pairCount) {
                    val (delSpans, _) = computeTokenDiff(lines[i + k].text, lines[delEnd + k].text)
                    result.add(ProcessedDiffLine(i + k, lines[i + k], delSpans))
                }
                for (k in pairCount until delCount) {
                    val text = lines[i + k].text.ifEmpty { " " }
                    result.add(ProcessedDiffLine(i + k, lines[i + k], listOf(DiffSpan(text, false))))
                }
                for (k in 0 until pairCount) {
                    val (_, insSpans) = computeTokenDiff(lines[i + k].text, lines[delEnd + k].text)
                    result.add(ProcessedDiffLine(delEnd + k, lines[delEnd + k], insSpans))
                }
                for (k in pairCount until insCount) {
                    val text = lines[delEnd + k].text.ifEmpty { " " }
                    result.add(ProcessedDiffLine(delEnd + k, lines[delEnd + k], listOf(DiffSpan(text, false))))
                }
                i = insEnd
                continue
            }
        }

        val text = lines[i].text.ifEmpty { " " }
        result.add(ProcessedDiffLine(i, lines[i], listOf(DiffSpan(text, false))))
        i++
    }
    return result
}

fun tokenize(text: String): List<String> {
    if (text.isEmpty()) return listOf(" ")
    val tokens = mutableListOf<String>()
    var current = StringBuilder()
    var currentKind = 0 // 1: word, 2: space, 3: other

    for (ch in text) {
        val kind = when {
            ch.isWhitespace() -> 2
            ch.isLetterOrDigit() || ch == '_' -> 1
            else -> 3
        }
        if (kind == currentKind && kind != 3) {
            current.append(ch)
        } else {
            if (current.isNotEmpty()) {
                tokens.add(current.toString())
            }
            current = StringBuilder().append(ch)
            currentKind = kind
        }
    }
    if (current.isNotEmpty()) {
        tokens.add(current.toString())
    }
    return tokens
}

fun computeTokenDiff(oldText: String, newText: String): Pair<List<DiffSpan>, List<DiffSpan>> {
    val tokensA = tokenize(if (oldText.isEmpty()) " " else oldText)
    val tokensB = tokenize(if (newText.isEmpty()) " " else newText)
    val m = tokensA.size
    val n = tokensB.size

    val dp = Array(m + 1) { IntArray(n + 1) }
    for (i in 1..m) {
        for (j in 1..n) {
            if (tokensA[i - 1] == tokensB[j - 1]) {
                dp[i][j] = dp[i - 1][j - 1] + 1
            } else {
                dp[i][j] = maxOf(dp[i - 1][j], dp[i][j - 1])
            }
        }
    }

    val commonA = mutableSetOf<Int>()
    val commonB = mutableSetOf<Int>()
    var i = m
    var j = n
    while (i > 0 && j > 0) {
        if (tokensA[i - 1] == tokensB[j - 1]) {
            commonA.add(i - 1)
            commonB.add(j - 1)
            i--
            j--
        } else if (dp[i - 1][j] >= dp[i][j - 1]) {
            i--
        } else {
            j--
        }
    }

    val spansA = tokensA.mapIndexed { idx, tok ->
        DiffSpan(tok, isChanged = idx !in commonA)
    }
    val spansB = tokensB.mapIndexed { idx, tok ->
        DiffSpan(tok, isChanged = idx !in commonB)
    }
    return spansA to spansB
}
