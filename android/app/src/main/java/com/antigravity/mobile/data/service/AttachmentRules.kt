package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.AttachmentFile
import java.util.Locale

/**
 * Client-side mirror of the gateway's attachment rules (internal/proxy/attachments.go), so that
 * obviously unsupported files are rejected before any bytes are uploaded. The gateway remains the
 * source of truth.
 */
object AttachmentRules {
    const val MAX_FILE_BYTES: Long = 50L * 1024 * 1024
    const val MAX_FILES_PER_MESSAGE = 5
    const val MAX_TOTAL_BYTES: Long = 100L * 1024 * 1024
    const val BLOCK_HEADER = "📎 附件（已上传到本机，可直接读取）："

    private val allowedExtensions: Set<String> = setOf(
        // documents
        "doc", "docx", "dot", "dotx", "rtf", "odt", "pages", "pdf", "md", "markdown", "txt", "log", "epub",
        // spreadsheets
        "xls", "xlsx", "xlsm", "csv", "tsv", "ods", "numbers",
        // presentations
        "ppt", "pptx", "pps", "ppsx", "odp", "key",
        // audio
        "mp3", "m4a", "wav", "aac", "flac", "ogg", "oga", "opus", "aif", "aiff", "wma", "amr", "caf",
        // archives
        "zip", "tar", "gz", "tgz", "7z",
        // source & config
        "py", "js", "mjs", "cjs", "ts", "tsx", "jsx", "go", "rs", "java", "kt", "kts", "swift", "m", "mm",
        "c", "cc", "cpp", "cxx", "h", "hpp", "cs", "rb", "php", "lua", "dart", "scala", "r", "pl",
        "sh", "bash", "zsh", "bat", "ps1", "sql", "vue", "svelte", "gradle", "proto",
        "json", "jsonl", "yaml", "yml", "toml", "ini", "cfg", "conf", "xml", "html", "htm", "css", "scss",
        "less", "ipynb", "dockerfile", "makefile"
    )

    fun isAllowedName(name: String): Boolean {
        val lower = name.lowercase(Locale.ROOT)
        val ext = lower.substringAfterLast('.', missingDelimiterValue = lower)
        return ext in allowedExtensions
    }

    /** Returns a user-facing rejection reason, or null when the file may be attached. */
    fun validate(name: String, size: Long, existing: List<AttachmentFile>): String? {
        if (!isAllowedName(name)) {
            val ext = name.substringAfterLast('.', "").ifBlank { "无扩展名" }
            return "不支持的文件类型（$ext）：$name"
        }
        if (size <= 0) return "文件为空：$name"
        if (size > MAX_FILE_BYTES) return "文件超过 ${MAX_FILE_BYTES / 1024 / 1024}MB：$name"
        if (existing.size >= MAX_FILES_PER_MESSAGE) return "一条消息最多附带 $MAX_FILES_PER_MESSAGE 个文件"
        if (existing.sumOf { it.size } + size > MAX_TOTAL_BYTES) {
            return "附件总大小超过 ${MAX_TOTAL_BYTES / 1024 / 1024}MB"
        }
        return null
    }

    /** Same text the gateway appends to the user's message, used for optimistic bubbles. */
    fun formatBlock(files: List<AttachmentFile>): String {
        val lines = files.mapNotNull { it.line?.takeIf { l -> l.isNotBlank() } }
        if (lines.isEmpty()) return ""
        return BLOCK_HEADER + "\n" + lines.joinToString("\n")
    }

    fun appendBlock(text: String, files: List<AttachmentFile>): String {
        val block = formatBlock(files)
        if (block.isEmpty()) return text
        return if (text.isBlank()) block else text.trimEnd('\n') + "\n\n" + block
    }

    data class ParsedFile(val path: String, val name: String, val ext: String, val sizeLabel: String)

    private val lineRegex = Regex("""^- (.+) \(([^,()]*), ([^()]+)\)$""")

    /**
     * Splits a user message into its body text and the attachment lines the gateway appended.
     * Returns the original text with no files when no attachment block is present.
     */
    fun parseBlock(text: String): Pair<String, List<ParsedFile>> {
        val idx = text.lastIndexOf(BLOCK_HEADER)
        if (idx < 0) return text to emptyList()
        val tail = text.substring(idx + BLOCK_HEADER.length).trim('\n', '\r')
        val files = tail.lines().mapNotNull { raw ->
            val m = lineRegex.matchEntire(raw.trim()) ?: return@mapNotNull null
            val path = m.groupValues[1]
            ParsedFile(
                path = path,
                name = path.substringAfterLast('/').let { n ->
                    // strip "<id16>-" prefix the gateway adds to avoid collisions
                    if (n.length > 17 && n[16] == '-' && n.take(16).all { it in '0'..'9' || it in 'a'..'f' }) n.substring(17) else n
                },
                ext = m.groupValues[2],
                sizeLabel = m.groupValues[3]
            )
        }
        if (files.isEmpty()) return text to emptyList()
        return text.substring(0, idx).trimEnd() to files
    }
}
