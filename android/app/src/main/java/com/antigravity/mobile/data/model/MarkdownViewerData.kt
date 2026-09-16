package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class FileContentResponse(
    val uri: String = "",
    val filename: String = "",
    val content: String = "",
    val summary: String? = null,
    @SerialName("request_feedback") val requestFeedback: Boolean? = null,
    @SerialName("user_facing") val userFacing: Boolean? = null
)

data class MarkdownFileViewerData(
    val uri: String,
    val title: String,
    val filename: String,
    val content: String,
    val summary: String? = null,
    val canProceed: Boolean = false,
    val isLoading: Boolean = false,
    val errorMessage: String? = null
)
