package com.antigravity.mobile.ui.viewmodel

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.AttachmentFile
import com.antigravity.mobile.data.model.UploadState
import com.antigravity.mobile.data.service.AttachmentRules
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File
import java.util.UUID
import kotlin.coroutines.coroutineContext

/**
 * Non-image attachments (documents, archives, source files).
 *
 * Picking a file copies it into app-private storage, then uploads it to the gateway right away
 * so that "send" only has to reference the returned attachment id. State is persisted per draft
 * (see PreferencesManager.saveDraftFiles) so that it survives leaving the screen.
 */

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

private fun safeLocalName(name: String): String =
    name.replace(Regex("""[\\/:*?"<>|\u0000-\u001f]"""), "_").take(100)

/** Adds picked/shared URIs; images are routed to the image path, everything else to uploads. */
internal fun ChatViewModel.addAttachmentsFromUris(context: Context, uris: List<Uri>) {
    val (imageUris, fileUris) = uris.partition {
        context.contentResolver.getType(it)?.startsWith("image/") == true
    }
    if (imageUris.isNotEmpty()) addImagesFromUris(context, imageUris)
    if (fileUris.isNotEmpty()) addFilesFromUris(context, fileUris)
}

internal fun ChatViewModel.addFilesFromUris(context: Context, uris: List<Uri>) {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank() || uris.isEmpty()) return
    viewModelScope.launch(Dispatchers.IO) {
        val dir = prefs?.draftFilesDir(cid) ?: File(context.cacheDir, "draft_files").apply { mkdirs() }
        val accepted = mutableListOf<AttachmentFile>()
        val rejected = mutableListOf<String>()

        for (uri in uris) {
            val (name, declaredSize) = queryNameAndSize(context, uri)
            val existing = currentDraftFiles(cid) + accepted
            // Cheap pre-check on the declared size/type, before copying anything.
            AttachmentRules.validate(name, if (declaredSize >= 0) declaredSize else 1, existing)?.let {
                rejected.add(it); continue
            }
            val target = File(dir, "${UUID.randomUUID()}-${safeLocalName(name)}")
            val copied = try {
                context.contentResolver.openInputStream(uri)?.use { input ->
                    target.outputStream().use { out -> input.copyTo(out) }
                } ?: -1L
            } catch (_: Exception) {
                -1L
            }
            if (copied < 0) {
                target.delete(); rejected.add("无法读取文件：$name"); continue
            }
            // Re-validate with the real size (the provider's declared size may be missing).
            AttachmentRules.validate(name, copied, existing)?.let {
                target.delete(); rejected.add(it); continue
            }
            accepted.add(AttachmentFile(name = name, size = copied, localPath = target.absolutePath))
        }

        if (accepted.isNotEmpty()) {
            updateDraftFiles(cid, persist = true) { it + accepted }
            accepted.forEach { startUpload(cid, it.id) }
        }
        if (rejected.isNotEmpty()) {
            _uiState.update { it.copy(attachmentNotice = rejected.distinct().joinToString("\n")) }
        }
    }
}

internal fun ChatViewModel.currentDraftFiles(cid: String): List<AttachmentFile> =
    if (_uiState.value.cascadeId == cid) _uiState.value.selectedFiles else prefs?.loadDraftFiles(cid).orEmpty()

/** Applies [transform] to a draft's file list, in the UI state when it is the open draft. */
internal fun ChatViewModel.updateDraftFiles(
    cid: String,
    persist: Boolean,
    transform: (List<AttachmentFile>) -> List<AttachmentFile>
) {
    if (_uiState.value.cascadeId == cid) {
        _uiState.update { it.copy(selectedFiles = transform(it.selectedFiles)) }
        if (persist) prefs?.saveDraftFiles(cid, _uiState.value.selectedFiles)
    } else if (persist) {
        prefs?.saveDraftFiles(cid, transform(prefs.loadDraftFiles(cid)))
    }
}

private fun List<AttachmentFile>.mapById(id: String, f: (AttachmentFile) -> AttachmentFile) =
    map { if (it.id == id) f(it) else it }

internal fun ChatViewModel.startUpload(cid: String, fileId: String) {
    val file = currentDraftFiles(cid).firstOrNull { it.id == fileId } ?: return
    fileUploadJobs[fileId]?.cancel()
    fileUploadJobs[fileId] = viewModelScope.launch(Dispatchers.IO) {
        updateDraftFiles(cid, persist = true) {
            it.mapById(fileId) { f -> f.copy(state = UploadState.UPLOADING, progress = 0f, error = null) }
        }
        var lastPercent = -1
        val result = apiClient.uploadAttachment(File(file.localPath), file.name) { p ->
            val percent = (p * 100).toInt()
            if (percent != lastPercent) {
                lastPercent = percent
                updateDraftFiles(cid, persist = false) { it.mapById(fileId) { f -> f.copy(progress = p) } }
            }
        }
        coroutineContext.ensureActive()
        result.onSuccess { up ->
            updateDraftFiles(cid, persist = true) {
                it.mapById(fileId) { f ->
                    f.copy(state = UploadState.DONE, progress = 1f, attachmentId = up.id, line = up.line, error = null)
                }
            }
        }.onFailure { e ->
            updateDraftFiles(cid, persist = true) {
                it.mapById(fileId) { f ->
                    f.copy(state = UploadState.FAILED, error = e.message ?: "上传失败")
                }
            }
        }
        fileUploadJobs.remove(fileId)
    }
}

internal fun ChatViewModel.retryFileUpload(fileId: String) {
    val cid = _uiState.value.cascadeId
    startUpload(cid, fileId)
}

internal fun ChatViewModel.removeFile(fileId: String) {
    val cid = _uiState.value.cascadeId
    fileUploadJobs.remove(fileId)?.cancel()
    val file = _uiState.value.selectedFiles.firstOrNull { it.id == fileId }
    updateDraftFiles(cid, persist = true) { list -> list.filterNot { it.id == fileId } }
    file?.let { runCatching { File(it.localPath).delete() } }
}

internal fun ChatViewModel.consumeAttachmentNotice() {
    _uiState.update { it.copy(attachmentNotice = null) }
}

/** Reloads a draft's files from storage and resumes any upload that is not already running. */
internal fun ChatViewModel.restoreDraftFiles(cascadeId: String) {
    val saved = prefs?.loadDraftFiles(cascadeId).orEmpty()
    if (saved.isEmpty()) return
    val inMemory = _uiState.value.selectedFiles.associateBy { it.id }
    val merged = saved.map { f ->
        val running = fileUploadJobs[f.id]?.isActive == true
        when {
            running -> inMemory[f.id] ?: f
            f.state == UploadState.DONE -> f
            else -> f.copy(state = UploadState.PENDING, progress = 0f, error = null)
        }
    }
    _uiState.update { it.copy(selectedFiles = merged) }
    merged.filter { it.state == UploadState.PENDING && fileUploadJobs[it.id]?.isActive != true }
        .forEach { startUpload(cascadeId, it.id) }
}
