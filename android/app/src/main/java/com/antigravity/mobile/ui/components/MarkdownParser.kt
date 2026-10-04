package com.antigravity.mobile.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.text.*

internal val ORDERED_LIST_REGEX = Regex("""^(\d{1,9})[.)]\s+(.*)$""")

/**
 * 1:1 Markdown parser replicating iOS MarkdownContentView.swift AST generation.
 */
object MarkdownParser {
    fun parse(rawText: String): List<MarkdownBlock> {
        val blocks = mutableListOf<MarkdownBlock>()
        val lines = rawText.replace("\r\n", "\n").replace('\r', '\n').lines()
        var i = 0
        var blockIdx = 0

        while (i < lines.size) {
            val line = lines[i]
            val trimmed = line.trim()

            if (trimmed.isEmpty()) {
                i++
                continue
            }

            // 0. Frontmatter check at document start
            if (blockIdx == 0 && trimmed == "---") {
                var endIdx = i + 1
                var foundEnd = false
                while (endIdx < lines.size) {
                    val t = lines[endIdx].trim()
                    if (t == "---" || t == "...") {
                        foundEnd = true
                        break
                    }
                    endIdx++
                }
                if (foundEnd && endIdx > i + 1) {
                    val raw = lines.subList(i + 1, endIdx).joinToString("\n")
                    blocks.add(MarkdownBlock.Frontmatter("block-${blockIdx++}", raw, endIdx - i + 1))
                    i = endIdx + 1
                    continue
                }
            }

            // 1. Fenced Code Block
            if (trimmed.startsWith("```")) {
                val lang = trimmed.removePrefix("```").trim()
                val codeLines = mutableListOf<String>()
                i++
                while (i < lines.size) {
                    if (lines[i].trim().startsWith("```")) {
                        i++
                        break
                    }
                    codeLines.add(lines[i])
                    i++
                }
                blocks.add(MarkdownBlock.CodeBlock("block-${blockIdx++}", lang, codeLines.joinToString("\n")))
                continue
            }

            // 2. Divider
            if (trimmed == "---" || trimmed == "***" || trimmed == "___") {
                blocks.add(MarkdownBlock.Divider("block-${blockIdx++}"))
                i++
                continue
            }

            // 3. Headings
            if (trimmed.startsWith("#")) {
                var level = 0
                while (level < trimmed.length && trimmed[level] == '#') {
                    level++
                }
                if (level in 1..6 && trimmed.length > level && trimmed[level] == ' ') {
                    val headingText = trimmed.substring(level + 1).trim()
                    blocks.add(MarkdownBlock.Heading("block-${blockIdx++}", level, headingText))
                    i++
                    continue
                }
            }

            // 4. Tables
            if (trimmed.startsWith("|") && trimmed.endsWith("|") && trimmed.contains("|")) {
                val tableLines = mutableListOf<String>()
                while (i < lines.size) {
                    val tLine = lines[i].trim()
                    if (tLine.startsWith("|") && tLine.endsWith("|")) {
                        tableLines.add(tLine)
                        i++
                    } else {
                        break
                    }
                }

                if (tableLines.size >= 2) {
                    val parseRow: (String) -> List<String> = { rowStr ->
                        val placeholder = "\uE000"
                        val sanitized = rowStr.replace("\\|", placeholder)
                        val parts = sanitized.split("|")
                        if (parts.size >= 2) {
                            parts.subList(1, parts.size - 1).map {
                                it.replace(placeholder, "|").trim()
                            }
                        } else emptyList()
                    }

                    val isSeparatorRow: (List<String>) -> Boolean = { cells ->
                        cells.isNotEmpty() && cells.all { cell ->
                            val t = cell.trim()
                            t.isNotEmpty() && t.all { it == '-' || it == ':' }
                        }
                    }

                    val parseAlignments: (List<String>) -> List<TableColumnAlignment> = { cells ->
                        cells.map { cell ->
                            val t = cell.trim()
                            val left = t.startsWith(":")
                            val right = t.endsWith(":")
                            when {
                                left && right -> TableColumnAlignment.CENTER
                                right -> TableColumnAlignment.TRAILING
                                else -> TableColumnAlignment.LEADING
                            }
                        }
                    }

                    val headers = parseRow(tableLines[0])
                    var alignments = emptyList<TableColumnAlignment>()
                    val rows = mutableListOf<List<String>>()

                    for (rowIdx in 1 until tableLines.size) {
                        val r = parseRow(tableLines[rowIdx])
                        if (isSeparatorRow(r)) {
                            if (alignments.isEmpty()) {
                                alignments = parseAlignments(r)
                            }
                        } else {
                            rows.add(r)
                        }
                    }

                    blocks.add(MarkdownBlock.Table("block-${blockIdx++}", headers, rows, alignments))
                    continue
                }
            }

            // 5. Bullet List
            if (trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("• ")) {
                val items = mutableListOf<String>()
                while (i < lines.size) {
                    val lLine = lines[i].trim()
                    if (lLine.startsWith("- ") || lLine.startsWith("* ") || lLine.startsWith("• ")) {
                        items.add(lLine.drop(2).trim())
                        i++
                    } else {
                        break
                    }
                }
                blocks.add(MarkdownBlock.BulletList("block-${blockIdx++}", items))
                continue
            }

            // 5b. Ordered List
            val orderedMatch = ORDERED_LIST_REGEX.find(trimmed)
            if (orderedMatch != null) {
                val startNum = orderedMatch.groups[1]?.value?.toIntOrNull() ?: 1
                val items = mutableListOf<String>()
                while (i < lines.size) {
                    val lLine = lines[i].trim()
                    val m = ORDERED_LIST_REGEX.find(lLine)
                    if (m != null) {
                        items.add(m.groups[2]?.value?.trim() ?: "")
                        i++
                    } else {
                        break
                    }
                }
                blocks.add(MarkdownBlock.OrderedList("block-${blockIdx++}", startNum, items))
                continue
            }

            // 6. Standalone image line: ![alt](url), [![alt](thumb)](url), MEDIA:url
            val standaloneImg = parseStandaloneImage(trimmed)
            if (standaloneImg != null) {
                blocks.add(MarkdownBlock.Image("block-${blockIdx++}", standaloneImg.first, standaloneImg.second))
                i++
                continue
            }

            // 7. Paragraph
            val paraLines = mutableListOf<String>()
            paraLines.add(line)
            i++
            while (i < lines.size) {
                val nextLine = lines[i]
                val nTrimmed = nextLine.trim()
                if (nTrimmed.isEmpty() ||
                    nTrimmed.startsWith("```") ||
                    nTrimmed.startsWith("#") ||
                    nTrimmed == "---" ||
                    (nTrimmed.startsWith("|") && nTrimmed.endsWith("|")) ||
                    nTrimmed.startsWith("- ") ||
                    nTrimmed.startsWith("* ") ||
                    nTrimmed.startsWith("• ") ||
                    ORDERED_LIST_REGEX.containsMatchIn(nTrimmed) ||
                    parseStandaloneImage(nTrimmed) != null
                ) {
                    break
                }
                paraLines.add(nextLine)
                i++
            }
            val paraText = paraLines.joinToString("\n")
            val imagesInPara = findImages(paraText)
            if (imagesInPara.isEmpty()) {
                blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", paraText))
            } else {
                var curIdx = 0
                for (img in imagesInPara) {
                    if (img.range.first > curIdx) {
                        val textBefore = paraText.substring(curIdx, img.range.first).trim()
                        if (textBefore.isNotEmpty()) {
                            blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", textBefore))
                        }
                    }
                    blocks.add(MarkdownBlock.Image("block-${blockIdx++}", img.alt, img.url))
                    curIdx = img.range.last + 1
                }
                if (curIdx < paraText.length) {
                    val textAfter = paraText.substring(curIdx).trim()
                    if (textAfter.isNotEmpty()) {
                        blocks.add(MarkdownBlock.Paragraph("block-${blockIdx++}", textAfter))
                    }
                }
            }
        }

        return blocks
    }

    private val linkedImageRegex = Regex("""\[!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)""")
    private val markdownImageRegex = Regex("""!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)""")
    private val mediaPrefixRegex = Regex("""(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s)<>"'`]+)""", RegexOption.IGNORE_CASE)

    private fun cleanImageURL(raw: String): String {
        return raw.trim().trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')
    }

    private data class FoundImage(val alt: String, val url: String, val range: IntRange)

    private fun findImages(text: String): List<FoundImage> {
        if (text.isEmpty()) return emptyList()
        val results = mutableListOf<FoundImage>()

        linkedImageRegex.findAll(text).forEach { match ->
            val alt = match.groups[1]?.value.orEmpty()
            val orig = match.groups[3]?.value.orEmpty()
            val cleaned = cleanImageURL(orig)
            if (cleaned.isNotBlank()) {
                results.add(FoundImage(alt.trim(), cleaned, match.range))
            }
        }

        markdownImageRegex.findAll(text).forEach { match ->
            val alt = match.groups[1]?.value.orEmpty()
            val url = match.groups[2]?.value.orEmpty()
            val cleaned = cleanImageURL(url)
            if (cleaned.isNotBlank() && results.none { it.range.contains(match.range.first) }) {
                results.add(FoundImage(alt.trim(), cleaned, match.range))
            }
        }

        mediaPrefixRegex.findAll(text).forEach { match ->
            val url = match.groups[1]?.value.orEmpty()
            val cleaned = cleanImageURL(url)
            if (cleaned.isNotBlank() && results.none { it.range.contains(match.range.first) }) {
                val fileName = cleaned.substringAfterLast('/')
                results.add(FoundImage(fileName, cleaned, match.range))
            }
        }

        return results.sortedBy { it.range.first }
    }

    private fun parseStandaloneImage(trimmed: String): Pair<String, String>? {
        val images = findImages(trimmed)
        if (images.size != 1) return null
        val img = images[0]
        val before = trimmed.substring(0, img.range.first).trim()
        val after = trimmed.substring(img.range.last + 1).trim()
        if (before.isEmpty() && after.isEmpty()) {
            return Pair(img.alt, img.url)
        }
        return null
    }
}
