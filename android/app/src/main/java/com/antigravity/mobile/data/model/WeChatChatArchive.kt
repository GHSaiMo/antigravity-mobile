package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class WeChatZipFileInfo(
    val path: String = "",
    val name: String = "",
    val size: Long = 0L
)

@Serializable
data class WeChatChatMessage(
    val id: Int = 0,
    val sender: String = "",
    @SerialName("sender_initial") val senderInitial: String = "?",
    @SerialName("color_index") val colorIndex: Int = 0,
    val timestamp: String = "",
    @SerialName("display_time") val displayTime: String = "",
    val kind: String = "text",
    val text: String = "",
    @SerialName("media_entry") val mediaEntry: String? = null,
    @SerialName("media_name") val mediaName: String? = null,
    @SerialName("media_size") val mediaSize: Long? = null,
    @SerialName("duration_label") val durationLabel: String? = null
)

@Serializable
data class WeChatChatArchive(
    @SerialName("is_wechat_archive") val isWeChatArchive: Boolean = false,
    val title: String = "群聊的聊天记录",
    @SerialName("date_range") val dateRange: String = "",
    @SerialName("is_multi_day") val isMultiDay: Boolean = false,
    @SerialName("file_name") val fileName: String = "",
    @SerialName("message_count") val messageCount: Int = 0,
    @SerialName("raw_text") val rawText: String? = null,
    val messages: List<WeChatChatMessage> = emptyList(),
    val entries: List<WeChatZipFileInfo> = emptyList()
)

data class WeChatArchiveSessionItem(
    val uri: String,
    val fileName: String
)
