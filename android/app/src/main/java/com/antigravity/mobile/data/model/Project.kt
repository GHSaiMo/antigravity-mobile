package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ProjectItem(
    @SerialName("id") val rawId: String? = null,
    val name: String,
    val uri: String = "",
    val path: String = "",
    val isWorkspace: Boolean = true,
    val sessionCount: Int = 0,
    val lastActive: String? = null
) {
    val id: String
        get() = rawId?.takeIf { it.isNotBlank() } ?: uri

    val isPureChat: Boolean
        get() = rawId == "outside-of-project" || name == "Chat" || (uri.isEmpty() && path.contains("不关联任何工作区"))

    companion object {
        val PURE_CHAT = ProjectItem(
            rawId = "outside-of-project",
            name = "Chat",
            uri = "",
            path = "新对话 · 不关联任何工作区",
            isWorkspace = false
        )
    }
}
