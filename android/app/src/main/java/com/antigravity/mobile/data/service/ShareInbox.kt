package com.antigravity.mobile.data.service

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.core.content.FileProvider
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.io.File
import java.util.UUID

/** A file received from another app (WeChat, file manager, ...) that is waiting for a destination. */
data class SharedFile(
    val id: String,
    val name: String,
    val size: Long,
    val path: String,
    val isImage: Boolean,
    /** Non-null when the file cannot be attached; it is shown in the sheet but never delivered. */
    val rejectReason: String? = null
) {
    val isDeliverable: Boolean get() = rejectReason == null
}

/**
 * Staging area for files shared into the app from outside.
 *
 * The incoming URI grant is only valid while the receiving activity is alive, so files are copied
 * into app cache right away. Once the user picks a destination they are handed to the chat's
 * normal attach pipeline (validation, draft copy, background upload).
 */
object ShareInbox {
    private const val DIR = "pending_share"
    private const val MAX_STAGED = 20
    private const val STALE_MS = 24L * 60 * 60 * 1000

    private val _files = MutableStateFlow<List<SharedFile>>(emptyList())
    val files: StateFlow<List<SharedFile>> = _files.asStateFlow()

    /** Whether the destination sheet is open (it can be dismissed while files stay staged). */
    private val _sheetVisible = MutableStateFlow(false)
    val sheetVisible: StateFlow<Boolean> = _sheetVisible.asStateFlow()

    fun showSheet() { if (_files.value.isNotEmpty()) _sheetVisible.value = true }
    fun hideSheet() { _sheetVisible.value = false }

    private fun root(context: Context) = File(context.cacheDir, DIR).apply { mkdirs() }

    /** Drops staging leftovers from a previous process and anything older than a day. */
    fun cleanupStale(context: Context) {
        val now = System.currentTimeMillis()
        val keep = _files.value.map { File(it.path).parentFile?.name }.toSet()
        root(context).listFiles()?.forEach { dir ->
            if (dir.name !in keep && (now - dir.lastModified() > STALE_MS || _files.value.isEmpty())) {
                dir.deleteRecursively()
            }
        }
    }

    /** Copies [uris] into staging (blocking IO; call off the main thread). Returns how many were staged. */
    fun stage(context: Context, uris: List<Uri>): Int {
        var staged = 0
        for (uri in uris.distinct()) {
            if (_files.value.size >= MAX_STAGED) break
            val (name, declared) = queryNameAndSize(context, uri)
            val mime = context.contentResolver.getType(uri).orEmpty()
            val isImage = mime.startsWith("image/")
            val safeName = name.replace(Regex("""[\\/:*?"<>|\u0000-\u001f]"""), "_").take(100).ifBlank { "file" }
            // Cheap rejection before copying; the chat pipeline validates again against the draft.
            val reason = if (isImage) null
            else com.antigravity.mobile.data.service.AttachmentRules.validate(safeName, if (declared >= 0) declared else 1, emptyList())
            val dir = File(root(context), UUID.randomUUID().toString()).apply { mkdirs() }
            if (reason != null) dir.delete()
            val target = File(dir, safeName)
            val copied = if (reason != null) declared.coerceAtLeast(0) else try {
                context.contentResolver.openInputStream(uri)?.use { input ->
                    target.outputStream().use { out -> input.copyTo(out) }
                } ?: -1L
            } catch (_: Exception) {
                -1L
            }
            val finalReason = when {
                reason != null -> reason
                copied < 0 -> "无法读取文件：$safeName"
                else -> if (isImage) null else AttachmentRules.validate(safeName, copied, emptyList())
            }
            if (finalReason != null) target.delete()
            _files.update { it + SharedFile(UUID.randomUUID().toString(), safeName, copied.coerceAtLeast(0), target.absolutePath, isImage, finalReason) }
            staged++
        }
        if (staged > 0) _sheetVisible.value = true
        return staged
    }

    fun remove(context: Context, id: String) {
        _files.value.firstOrNull { it.id == id }?.let { File(it.path).parentFile?.deleteRecursively() }
        _files.update { list -> list.filterNot { it.id == id } }
        if (_files.value.isEmpty()) _sheetVisible.value = false
    }

    /**
     * Forgets the staged files after they were handed to a chat. The staged copies stay on disk
     * until [cleanupStale] runs: the chat pipeline copies them on a background thread.
     */
    fun release() {
        _files.value = emptyList()
        _sheetVisible.value = false
    }

    fun clear(context: Context) {
        _files.value = emptyList()
        _sheetVisible.value = false
        root(context).deleteRecursively()
    }

    /** content:// URIs of the deliverable files, for the chat attach pipeline. */
    fun deliverableUris(context: Context): List<Uri> =
        _files.value.filter { it.isDeliverable }.map {
            FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", File(it.path))
        }

    private fun queryNameAndSize(context: Context, uri: Uri): Pair<String, Long> {
        var name: String? = null
        var size = -1L
        try {
            context.contentResolver.query(
                uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null
            )?.use { c ->
                if (c.moveToFirst()) {
                    val ni = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    val si = c.getColumnIndex(OpenableColumns.SIZE)
                    if (ni >= 0 && !c.isNull(ni)) name = c.getString(ni)
                    if (si >= 0 && !c.isNull(si)) size = c.getLong(si)
                }
            }
        } catch (_: Exception) {}
        val resolved = name?.takeIf { it.isNotBlank() } ?: uri.lastPathSegment?.substringAfterLast('/') ?: "file"
        return resolved to size
    }
}
