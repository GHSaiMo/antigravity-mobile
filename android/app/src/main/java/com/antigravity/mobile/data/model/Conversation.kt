package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.text.SimpleDateFormat
import java.time.Instant
import java.time.OffsetDateTime
import java.util.*

@Serializable
data class WorkspaceItem(
    @SerialName("workspaceFolderAbsoluteUri") val workspaceFolderAbsoluteUri: String? = null
)

@Serializable
data class Annotations(
    val title: String? = null,
    val lastUserViewTime: String? = null,
    val markedAsUnread: Boolean? = null,
    val archived: Boolean? = null
)

@Serializable
data class TrajectoryMetadata(
    val workspaceUris: List<String>? = null,
    val projectId: String? = null,
    val createdAt: String? = null,
    val parentConversationId: String? = null,
    val rootConversationId: String? = null,
    val nestingDepth: Int? = null,
    val isBattleModeFork: Boolean? = null
)

@Serializable
data class TrajectorySummary(
    val summary: String? = null,
    val stepCount: Int? = null,
    val lastModifiedTime: String? = null,
    val trajectoryId: String? = null,
    val status: String? = null,
    val workspaces: List<WorkspaceItem>? = null,
    val annotations: Annotations? = null,
    val trajectoryMetadata: TrajectoryMetadata? = null,
    val needsInput: Boolean? = null,
    val hasError: Boolean? = null,
    val errorMessage: String? = null
) {
    val isSubagent: Boolean
        get() {
            val meta = trajectoryMetadata ?: return false
            if (!meta.parentConversationId.isNullOrBlank()) return true
            if (meta.isBattleModeFork == true) return true
            if ((meta.nestingDepth ?: 0) > 0) return true
            return false
        }
}

@Serializable
data class GetAllCascadeTrajectoriesResponse(
    val trajectorySummaries: Map<String, TrajectorySummary>? = null
)

enum class ConversationStatus(val raw: String) {
    RUNNING("RUNNING"),
    ACTION("ACTION"),
    ERROR("ERROR"),
    IDLE("IDLE"),
    UNKNOWN("UNKNOWN");

    val isRunning: Boolean get() = this == RUNNING
    val needsAction: Boolean get() = this == ACTION
    val isError: Boolean get() = this == ERROR
}

@Serializable
data class ConversationItem(
    val id: String,
    val title: String,
    val status: ConversationStatus = ConversationStatus.UNKNOWN,
    val stepCount: Int = 0,
    val workspaceName: String = "Chat",
    val lastModifiedTime: String? = null,
    val isSubagent: Boolean = false,
    val isUnread: Boolean = false,
    val draftProject: ProjectItem? = null
) {
    val displayTitle: String
        get() = title.ifBlank { "未命名会话" }

    val lastModifiedEpochMs: Long
        get() = parseIsoDate(lastModifiedTime)

    val relativeTimeString: String
        get() {
            val timeMs = lastModifiedEpochMs
            if (timeMs <= 0L) return ""
            val diffMs = System.currentTimeMillis() - timeMs
            if (diffMs < 0) return "刚刚"
            val diffSec = diffMs / 1000
            return when {
                diffSec < 60 -> "刚刚"
                diffSec < 3600 -> "${diffSec / 60}分钟前"
                diffSec < 86400 -> "${diffSec / 3600}小时前"
                else -> "${diffSec / 86400}天前"
            }
        }

    val isPureChat: Boolean
        get() = workspaceName == "Chat" || workspaceName.isEmpty() || draftProject?.isPureChat == true

    val isDraft: Boolean
        get() = id.startsWith("local_draft_") || id.startsWith("draft_") || draftProject != null

    companion object {
        fun sanitizeTitle(raw: String): String {
            val stripped = raw.replace(Regex("<[^>]+>"), "").trim()
            val firstLine = stripped.lines().firstOrNull { it.isNotBlank() }?.trim() ?: ""
            if (firstLine.isEmpty()) return "未命名会话"
            return if (firstLine.length > 36) firstLine.take(36) + "..." else firstLine
        }

        fun fromSummary(id: String, summary: TrajectorySummary, localViewTime: Long = 0): ConversationItem {
            val resolvedTitle = when {
                !summary.annotations?.title.isNullOrBlank() && summary.annotations?.title != "未命名会话" ->
                    sanitizeTitle(summary.annotations.title)
                !summary.summary.isNullOrBlank() && summary.summary != "未命名会话" ->
                    sanitizeTitle(summary.summary)
                else -> "未命名会话"
            }

            val status = when {
                summary.needsInput == true -> ConversationStatus.ACTION
                summary.hasError == true || summary.status == "CASCADE_RUN_STATUS_ERROR" -> ConversationStatus.ERROR
                summary.status == "CASCADE_RUN_STATUS_RUNNING" -> ConversationStatus.RUNNING
                summary.status == "CASCADE_RUN_STATUS_IDLE" -> ConversationStatus.IDLE
                else -> ConversationStatus.UNKNOWN
            }

            val stepCount = summary.stepCount ?: 0

            val wsUri = summary.trajectoryMetadata?.workspaceUris?.firstOrNull()
                ?: summary.workspaces?.firstOrNull()?.workspaceFolderAbsoluteUri
                ?: ""

            val workspaceName = if (wsUri.isNotBlank()) {
                wsUri.trimEnd('/').substringAfterLast('/').ifBlank { "Chat" }
            } else {
                "Chat"
            }

            val resolvedTime = summary.lastModifiedTime?.takeIf { it.isNotBlank() }
                ?: summary.annotations?.lastUserViewTime?.takeIf { it.isNotBlank() }
                ?: summary.trajectoryMetadata?.createdAt?.takeIf { it.isNotBlank() }

            // Unread calculation
            var unread = false
            if (status != ConversationStatus.RUNNING && status != ConversationStatus.ACTION && summary.annotations?.archived != true) {
                if (summary.annotations?.markedAsUnread == true) {
                    unread = true
                } else if (!resolvedTime.isNullOrBlank()) {
                    val modDate = parseIsoDate(resolvedTime)
                    val serverViewDate = summary.annotations?.lastUserViewTime?.let { parseIsoDate(it) } ?: 0L
                    val effectiveDate = maxOf(serverViewDate, localViewTime)
                    unread = if (effectiveDate > 0) modDate > effectiveDate else true
                }
            }

            return ConversationItem(
                id = id,
                title = resolvedTitle,
                status = status,
                stepCount = stepCount,
                workspaceName = workspaceName,
                lastModifiedTime = resolvedTime,
                isSubagent = summary.isSubagent,
                isUnread = unread
            )
        }

        fun parseIsoDate(isoString: String?): Long {
            if (isoString.isNullOrBlank()) return 0L
            val trimmed = isoString.trim()
            val normalized = if (trimmed.contains(' ') && !trimmed.contains('T')) {
                trimmed.replace(' ', 'T')
            } else {
                trimmed
            }
            return try {
                Instant.parse(normalized).toEpochMilli()
            } catch (_: Exception) {
                try {
                    OffsetDateTime.parse(normalized).toInstant().toEpochMilli()
                } catch (_: Exception) {
                    try {
                        val clean = normalized.trimEnd('Z').substringBefore('.')
                        val inputFmt = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US).apply {
                            timeZone = TimeZone.getTimeZone("UTC")
                        }
                        inputFmt.parse(clean)?.time ?: 0L
                    } catch (_: Exception) {
                        0L
                    }
                }
            }
        }
    }
}

@Serializable
data class LocalDraftSession(
    val id: String = "local_draft_${UUID.randomUUID()}",
    val project: ProjectItem,
    var draftText: String = "",
    val createdAtEpochMs: Long = System.currentTimeMillis(),
    var updatedAtEpochMs: Long = System.currentTimeMillis()
) {
    fun toConversationItem(hasImages: Boolean = false): ConversationItem {
        val trimmed = draftText.trim()
        val firstLine = trimmed.lines().firstOrNull { it.isNotBlank() }?.trim() ?: ""
        val isPure = project.isPureChat
        val displayTitle = when {
            firstLine.isNotEmpty() -> if (firstLine.length > 36) firstLine.take(36) + "..." else firstLine
            hasImages -> if (isPure) "[图片] 新对话" else "[图片] ${project.name}"
            else -> if (isPure) "新对话" else project.name
        }
        val nowIso = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply {
            timeZone = TimeZone.getTimeZone("UTC")
        }.format(Date(updatedAtEpochMs))

        return ConversationItem(
            id = id,
            title = displayTitle,
            status = ConversationStatus.IDLE,
            stepCount = 0,
            workspaceName = if (isPure) "Chat" else project.name,
            lastModifiedTime = nowIso,
            isSubagent = false,
            isUnread = false,
            draftProject = project
        )
    }
}
