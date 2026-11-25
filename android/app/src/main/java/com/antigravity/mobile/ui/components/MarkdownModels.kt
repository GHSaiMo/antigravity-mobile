package com.antigravity.mobile.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.text.*

enum class TableColumnAlignment {
    LEADING, CENTER, TRAILING
}

enum class MarkdownAlertType(val title: String) {
    NOTE("Note"),
    TIP("Tip"),
    IMPORTANT("Important"),
    WARNING("Warning"),
    CAUTION("Caution");

    companion object {
        fun fromTag(tag: String): MarkdownAlertType? {
            return entries.firstOrNull { it.name.equals(tag.trim(), ignoreCase = true) }
        }
    }
}

sealed class MarkdownBlock {
    abstract val id: String

    data class Frontmatter(override val id: String, val rawContent: String, val lineCount: Int) : MarkdownBlock()
    data class Heading(override val id: String, val level: Int, val text: String) : MarkdownBlock()
    data class Divider(override val id: String) : MarkdownBlock()
    data class CodeBlock(override val id: String, val lang: String, val code: String) : MarkdownBlock()
    data class Table(
        override val id: String,
        val headers: List<String>,
        val rows: List<List<String>>,
        val alignments: List<TableColumnAlignment>
    ) : MarkdownBlock()
    data class BulletList(override val id: String, val items: List<String>, val indents: List<Int> = emptyList()) : MarkdownBlock()
    data class OrderedList(override val id: String, val startIndex: Int, val items: List<String>) : MarkdownBlock()
    data class Alert(
        override val id: String,
        val type: MarkdownAlertType,
        val content: String
    ) : MarkdownBlock()
    data class Blockquote(
        override val id: String,
        val content: String
    ) : MarkdownBlock()
    data class Paragraph(override val id: String, val text: String) : MarkdownBlock()
    data class Image(override val id: String, val alt: String, val url: String) : MarkdownBlock()
    data class AgentEmbed(override val id: String, val src: String) : MarkdownBlock()
    data class PluginCard(override val id: String, val pluginId: String) : MarkdownBlock()
    data class Carousel(
        override val id: String,
        val slides: List<MarkdownCarouselSlide>
    ) : MarkdownBlock()
}

data class MarkdownCarouselSlide(
    val id: String,
    val title: String? = null,
    val content: String
)
