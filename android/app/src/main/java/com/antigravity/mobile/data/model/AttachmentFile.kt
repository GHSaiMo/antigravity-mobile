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
    val error: String? = null
) {
    val isUploaded: Boolean get() = state == UploadState.DONE && attachmentId != null
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
    val line: String = ""
)
