package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.util.UUID

enum class UploadState { PENDING, UPLOADING, DONE, FAILED }

/**
 * A non-image file attached to the draft (document, archive, source file).
 *
 * The file is copied to app-private storage ([localPath]) as soon as it is picked and uploaded to
 * the gateway in the background. Once [state] is [UploadState.DONE], [attachmentId] and [line]
 * come from the gateway and are what the message actually references.
 */
@Serializable
data class AttachmentFile(
    val id: String = UUID.randomUUID().toString(),
    val name: String,
    val size: Long,
    val localPath: String,
    val state: UploadState = UploadState.PENDING,
    val progress: Float = 0f,
    val attachmentId: String? = null,
    val line: String? = null,
    val error: String? = null,
    val estimatedTokens: Long = 0L
) {
    val isUploaded: Boolean get() = state == UploadState.DONE && attachmentId != null

    fun resolvedEstimatedTokens(): Long {
        if (estimatedTokens > 0L) return estimatedTokens
        if (size <= 0L) return 0L
        val ext = name.substringAfterLast('.', "").lowercase()
        return when (ext) {
            "pdf" -> (size / 24L).coerceIn(250L, 500_000L)
            "docx", "doc", "pptx", "ppt", "xlsx", "xls", "xlsm", "odt", "rtf", "epub" -> (size / 10L).coerceIn(150L, 500_000L)
            "png", "jpg", "jpeg", "gif", "webp", "heic" -> (size / 1500L).coerceIn(258L, 2048L)
            "mp3", "m4a", "wav", "aac", "ogg", "amr", "flac" -> (size / 512L).coerceIn(64L, 200_000L)
            "mp4", "mov", "webm", "mkv" -> (size / 1200L).coerceIn(256L, 300_000L)
            "zip", "tar", "gz", "tgz", "7z", "exe", "dmg", "apk", "bin" -> 0L
            else -> (size / 4L).coerceAtLeast(1L)
        }
    }
}

/** Response of POST /api/v1/attachments. */
@Serializable
data class UploadedAttachment(
    val id: String,
    val path: String = "",
    val name: String = "",
    val size: Long = 0,
    val mime: String = "",
    @SerialName("sha256") val sha256: String = "",
    val kind: String = "",
    val line: String = "",
    val estimatedTokens: Long = 0L
)
