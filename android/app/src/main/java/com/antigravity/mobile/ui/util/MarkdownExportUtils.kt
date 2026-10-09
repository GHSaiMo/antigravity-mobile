package com.antigravity.mobile.ui.util

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/** 把会话导出的 Markdown 写入 cache 并调起系统分享（可存到「文件」、发给微信/邮件等）。 */
object MarkdownExportUtils {
    private const val DIR_NAME = "exports"

    /** 文件名：去掉路径与保留字符，限制长度，空标题回退到固定名。 */
    internal fun safeFileName(title: String): String {
        val cleaned = title
            .replace(Regex("[\\\\/:*?\"<>|\\p{Cntrl}]"), " ")
            .replace(Regex("\\s+"), " ")
            .trim()
            .take(60)
            .trim('.', ' ')
        return cleaned.ifEmpty { "conversation" }
    }

    suspend fun shareMarkdown(context: Context, title: String, markdown: String) {
        val uri = withContext(Dispatchers.IO) {
            val dir = File(context.cacheDir, DIR_NAME).apply { mkdirs() }
            // 清掉旧导出，避免 cache 无限增长
            dir.listFiles()?.filter { it.isFile }?.forEach { it.delete() }
            val file = File(dir, "${safeFileName(title)}.md")
            file.writeText(markdown, Charsets.UTF_8)
            FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
        }
        withContext(Dispatchers.Main) {
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "text/markdown"
                putExtra(Intent.EXTRA_STREAM, uri)
                putExtra(Intent.EXTRA_SUBJECT, title)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            context.startActivity(
                Intent.createChooser(intent, "导出 Markdown").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        }
    }
}
