package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import com.antigravity.mobile.data.service.FileIconResolver
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AppColors

sealed class PlanSegment {
    abstract val id: String
    data class Text(val content: String, override val id: String) : PlanSegment()
    data class PlanButton(val title: String, val uri: String, override val id: String) : PlanSegment()
}

fun splitCodeSpans(text: String): List<Pair<String, Boolean>> {
    if (!text.contains('`')) {
        return listOf(text to false)
    }
    val segments = mutableListOf<Pair<String, Boolean>>()
    val chars = text.toCharArray()
    var i = 0
    var lastIdx = 0
    while (i < chars.size) {
        if (chars[i] == '`') {
            val tickStart = i
            while (i < chars.size && chars[i] == '`') {
                i++
            }
            val tickLen = i - tickStart
            if (tickStart > lastIdx) {
                segments.add(text.substring(lastIdx, tickStart) to false)
            }
            var closeFound = false
            var j = i
            while (j < chars.size) {
                if (chars[j] == '`') {
                    val cStart = j
                    while (j < chars.size && chars[j] == '`') {
                        j++
                    }
                    if (j - cStart == tickLen) {
                        closeFound = true
                        segments.add(text.substring(tickStart, j) to true)
                        i = j
                        lastIdx = j
                        break
                    }
                } else {
                    j++
                }
            }
            if (!closeFound) {
                i = tickStart + 1
            }
        } else {
            i++
        }
    }
    if (lastIdx < chars.size) {
        segments.add(text.substring(lastIdx) to false)
    }
    return segments
}

fun findCodeSpanRanges(text: String): List<IntRange> {
    if (!text.contains('`')) return emptyList()
    val ranges = mutableListOf<IntRange>()
    var loc = 0
    for ((content, isCode) in splitCodeSpans(text)) {
        val len = content.length
        if (isCode && len > 0) {
            ranges.add(loc until (loc + len))
        }
        loc += len
    }
    return ranges
}

internal val PLAN_REGEX = Regex(
    """(?:(?<!\!)\[([^\]]+)\]\(([^)]+)\)|(?<![a-zA-Z0-9_\-./])((?:implementation_plan|walkthrough)\.md)(?![a-zA-Z0-9_\-./]))""",
    RegexOption.IGNORE_CASE
)

fun parsePlanSegments(rawText: String): List<PlanSegment> {
    if (!rawText.contains("implementation_plan", ignoreCase = true) &&
        !rawText.contains("walkthrough", ignoreCase = true)
    ) {
        return listOf(PlanSegment.Text(rawText, "text-0"))
    }

    val allMatches = PLAN_REGEX.findAll(rawText).toList()
    val codeSpanRanges = if (rawText.contains('`')) findCodeSpanRanges(rawText) else emptyList()
    val matches = allMatches.filter { match ->
        if (codeSpanRanges.isNotEmpty()) {
            val r = match.range
            if (codeSpanRanges.any { cs -> r.first < cs.last && r.last >= cs.first }) {
                return@filter false
            }
        }
        val g1 = match.groups[1]?.value?.lowercase()
        val g2 = match.groups[2]?.value?.lowercase()
        val g3 = match.groups[3]?.value
        if (g1 != null && g2 != null) {
            g1.contains("implementation_plan") || g1.contains("walkthrough") ||
            g2.contains("implementation_plan") || g2.contains("walkthrough")
        } else {
            g3 != null
        }
    }

    if (matches.isEmpty()) {
        return listOf(PlanSegment.Text(rawText, "text-0"))
    }

    val segments = mutableListOf<PlanSegment>()
    var lastEnd = 0
    var segIdx = 0

    for ((idx, match) in matches.withIndex()) {
        val range = match.range
        var prefix = ""
        if (range.first > lastEnd) {
            prefix = rawText.substring(lastEnd, range.first)
        }

        val nextIndex = range.last + 1
        val suffixLength = if (idx + 1 < matches.size) {
            matches[idx + 1].range.first - nextIndex
        } else {
            rawText.length - nextIndex
        }
        val suffixPreview = if (suffixLength > 0 && nextIndex + suffixLength <= rawText.length) {
            rawText.substring(nextIndex, nextIndex + suffixLength)
        } else {
            ""
        }

        var strippedLength = 0
        if (prefix.endsWith("**") && suffixPreview.startsWith("**")) {
            prefix = prefix.dropLast(2)
            strippedLength = 2
        } else if (prefix.endsWith("*") && suffixPreview.startsWith("*")) {
            prefix = prefix.dropLast(1)
            strippedLength = 1
        } else if (prefix.endsWith("__") && suffixPreview.startsWith("__")) {
            prefix = prefix.dropLast(2)
            strippedLength = 2
        } else if (prefix.endsWith("`") && suffixPreview.startsWith("`")) {
            prefix = prefix.dropLast(1)
            strippedLength = 1
        }

        val trimmedPrefix = prefix.trim()
        if (trimmedPrefix.isNotEmpty()) {
            segments.add(PlanSegment.Text(prefix, "seg-${segIdx++}"))
        }

        val g1 = match.groups[1]?.value
        val g2 = match.groups[2]?.value
        val g3 = match.groups[3]?.value

        val (title, uri) = when {
            g1 != null && g2 != null -> g1.trim() to g2.trim()
            g3 != null -> g3.trim() to g3.trim()
            else -> "implementation_plan.md" to "implementation_plan.md"
        }

        segments.add(PlanSegment.PlanButton(title, uri, "seg-${segIdx++}"))
        lastEnd = range.last + 1 + strippedLength
    }

    if (lastEnd < rawText.length) {
        val suffix = rawText.substring(lastEnd)
        val punctChars = setOf('。', '.', '，', ',', '！', '!', '？', '?', '；', ';', '：', ':')
        val trimmedSuffix = suffix.trim()
        val hasMeaningfulContent = trimmedSuffix.any { it !in punctChars }
        if (hasMeaningfulContent) {
            segments.add(PlanSegment.Text(suffix, "seg-${segIdx++}"))
        }
    }

    return segments
}

@Composable
fun PlanButtonView(
    title: String,
    uri: String,
    colors: AppColors,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
    val iconName = remember(uri, title) {
        FileIconResolver.resolveIcon(uri) ?: FileIconResolver.resolveIcon(title)
    }

    Row(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(colors.accentBlue.copy(alpha = 0.12f))
            .border(1.dp, colors.accentBlue.copy(alpha = 0.35f), RoundedCornerShape(8.dp))
            .clickable {
                haptic.light()
                onClick()
            }
            .padding(horizontal = 9.dp, vertical = 4.5.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        if (iconName != null) {
            FileIconSvgView(
                iconName = iconName,
                modifier = Modifier.size(14.dp)
            )
        } else {
            Icon(
                imageVector = Icons.Default.Description,
                contentDescription = null,
                tint = colors.accentBlue,
                modifier = Modifier.size(14.dp)
            )
        }

        Text(
            text = title,
            color = colors.accentBlue,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            fontFamily = FontFamily.Monospace,
            maxLines = 1
        )

        Icon(
            imageVector = Icons.Default.OpenInNew,
            contentDescription = null,
            tint = colors.accentBlue.copy(alpha = 0.65f),
            modifier = Modifier.size(11.dp)
        )
    }
}
