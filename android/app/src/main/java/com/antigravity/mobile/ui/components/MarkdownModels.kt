package com.antigravity.mobile.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.text.*

enum class TableColumnAlignment {
    LEADING, CENTER, TRAILING
}

sealed class MarkdownBlock {
    data class Frontmatter(val id: String, val rawContent: String, val lineCount: Int) : MarkdownBlock()
    data class Heading(val id: String, val level: Int, val text: String) : MarkdownBlock()
    data class Divider(val id: String) : MarkdownBlock()
    data class CodeBlock(val id: String, val lang: String, val code: String) : MarkdownBlock()
    data class Table(
        val id: String,
        val headers: List<String>,
        val rows: List<List<String>>,
        val alignments: List<TableColumnAlignment>
    ) : MarkdownBlock()
    data class BulletList(val id: String, val items: List<String>) : MarkdownBlock()
    data class OrderedList(val id: String, val startIndex: Int, val items: List<String>) : MarkdownBlock()
    data class Paragraph(val id: String, val text: String) : MarkdownBlock()
    data class Image(val id: String, val alt: String, val url: String) : MarkdownBlock()
}
