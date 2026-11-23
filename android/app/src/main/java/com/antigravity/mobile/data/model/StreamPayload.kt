package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import android.util.Base64
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ToolCallItem(
    val id: String = "",
    val type: String = "",
    val name: String = "",
    val input: String? = null,
    val output: String? = null,
    val status: String? = null
)

@Serializable
data class ArtifactItem(
    val uri: String = "",
    val title: String = "",
    val summary: String? = null,
    val requestFeedback: Boolean? = null,
    val userFacing: Boolean? = null
)

@Serializable
data class WorkflowPhaseItem(
    val name: String = "",
    val logs: List<String> = emptyList(),
    val subagents: List<SubagentItem> = emptyList(),
    val status: String = "done", // "running", "done", "failed"
    val duration: String? = null
)

@Serializable
data class WorkflowItem(
    val description: String? = null,
    val scriptPath: String? = null,
    val script: String? = null,
    val phases: List<WorkflowPhaseItem> = emptyList(),
    val logs: List<String> = emptyList(),
    val output: String? = null,
    val resultJson: String? = null,
    val status: String = "done", // "running", "done", "error"
    val duration: String? = null,
    val error: String? = null
)

@androidx.compose.runtime.Immutable
@Serializable
data class StepPlanItemEntry(
    val key: String = "",
    val title: String = "",
    val status: String = "pending" // "pending" | "in_progress" | "done" | "skipped" | "failed"
)

@androidx.compose.runtime.Immutable
@Serializable
data class StepPlanItem(
    val anchorStepIndex: Int = 0,
    val items: List<StepPlanItemEntry> = emptyList(),
    val doneCount: Int = 0,
    val totalCount: Int = 0,
    val skippedCount: Int = 0,
    val failedCount: Int = 0
) {
    val effectiveDoneCount: Int
        get() = if (doneCount > 0 || items.isEmpty()) doneCount else items.count { it.status == "done" }
    val effectiveSkippedCount: Int
        get() = if (skippedCount > 0 || items.isEmpty()) skippedCount else items.count { it.status == "skipped" }
    val effectiveFailedCount: Int
        get() = if (failedCount > 0 || items.isEmpty()) failedCount else items.count { it.status == "failed" }
    val effectiveTotalCount: Int
        get() = if (totalCount > 0) totalCount else maxOf(0, items.size - effectiveSkippedCount)
}

@androidx.compose.runtime.Immutable
@Serializable
data class GatewayMessageItem(
    val id: String = "",
    val type: String = "user", // "user", "agent", "tools", "error", "subagent", "workflow", "step_plan"
    val role: String = "", // "user", "assistant", "system"
    val text: String = "",
    val content: String = "",
    val toolCount: Int? = null,
    val toolNames: List<String>? = null,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    val artifacts: List<ArtifactItem>? = null,
    val timestamp: String? = null,
    /** 生成这条回复的模型 id 与展示名（仅 Agent 消息；未知时为空）。 */
    val model: String? = null,
    val modelName: String? = null,
    val status: String? = null,
    val stepIndex: Int? = null,
    val attemptCount: Int? = null,
    val maxAttempts: Int? = null,
    @SerialName("tool_calls") val toolCalls: List<ToolCallItem>? = null,
    @SerialName("reasoning_content") val reasoningContent: String? = null,
    /** type == "subagent" 时的子代理卡片数据（状态由网关实时补全）。 */
    val subagent: SubagentItem? = null,
    /** type == "workflow" 时的动态多智能体工作流数据 (Antigravity 2.26+)。 */
    val workflow: WorkflowItem? = null,
    /** type == "step_plan" 时的实时步骤清单卡片数据 (update_steps)。 */
    val stepPlan: StepPlanItem? = null,
    /** 工具组实时焦点摘要 (update_focus)。 */
    val focusText: String? = null,
    /** 工具组起始时间戳 (RFC3339，用于实时计时器)。 */
    val startedAt: String? = null,
    /** type == "user" 时引用的斜杠命令 / 技能列表。 */
    val slashCommands: List<SlashCommandMention>? = null,
    /** 客户端发送时生成的幂等标识（Antigravity 2.30+）。 */
    val clientMessageId: String? = null,
    val errorKind: String? = null,
    val isOverflow: Boolean = false,
    val thinking: String? = null,
    val thinkingDuration: String? = null,
    val toolDetails: List<ToolDetailItem>? = null,
    @kotlinx.serialization.Transient val imageDataList: List<ByteArray> = emptyList()
) {
    val effectiveRole: String
        get() = when {
            type.isNotBlank() -> if (type == "agent") "assistant" else type
            role.isNotBlank() -> role
            else -> "user"
        }

    val effectiveText: String
        get() = when {
            text.isNotBlank() -> text
            content.isNotBlank() -> content
            else -> ""
        }

    val isUser: Boolean
        get() = effectiveRole.equals("user", ignoreCase = true)

    val isTools: Boolean
        get() = type.equals("tools", ignoreCase = true) || !toolNames.isNullOrEmpty() || (toolCount != null && toolCount > 0)

    val isError: Boolean
        get() = status.equals("error", ignoreCase = true) || type.equals("error", ignoreCase = true)

    val isSubagent: Boolean
        get() = type.equals("subagent", ignoreCase = true) && subagent != null

    val isWorkflow: Boolean
        get() = type.equals("workflow", ignoreCase = true) && workflow != null

    val isStepPlan: Boolean
        get() = type.equals("step_plan", ignoreCase = true) && stepPlan != null

    val isAgent: Boolean
        get() = !isUser && !isTools && !isError && !isSubagent && !isWorkflow && !isStepPlan

    val effectiveImageDataList: List<ByteArray>
        get() {
            if (imageDataList.isNotEmpty()) return imageDataList
            if (!media.isNullOrEmpty()) {
                return media.mapNotNull { b64 ->
                    try {
                        Base64.decode(b64, Base64.DEFAULT)
                    } catch (_: Exception) {
                        null
                    }
                }
            }
            return emptyList()
        }
}

@Serializable
data class QueuedMessageItem(
    val id: String = "",
    val text: String = "",
    val createdAt: String? = null,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    /** 发送时附带的客户端消息 id（网关写进 tags，读队列时还原）；不是经网关发的消息为空。 */
    val clientMessageId: String? = null
) {
    val hasAttachments: Boolean
        get() = !media.isNullOrEmpty() || !imageUrls.isNullOrEmpty()
}

@Serializable
data class RunningTaskItem(
    val id: String = "",
    val stepIndex: Int = 0,
    val toolName: String? = null,
    val commandLine: String = "",
    val toolSummary: String? = null,
    val toolAction: String? = null,
    val logUri: String? = null,
    val startedAt: String? = null,
    val type: String = "",
    val command: String? = null,
    val status: String? = null,
    /** 常驻守护任务（IsDaemon=true，如 dev server / 定时任务）：跨回合保持 RUNNING，不应让会话显示为执行中。旧版网关不下发时默认 false。 */
    val isDaemon: Boolean = false
) {
    /** 是否阻塞当前回合（非常驻守护任务）。 */
    val blocksTurn: Boolean
        get() = !isDaemon

    val displayTitle: String
        get() = toolSummary?.takeIf { it.isNotBlank() }
            ?: toolAction?.takeIf { it.isNotBlank() }
            ?: toolName?.takeIf { it.isNotBlank() }?.let { com.antigravity.mobile.ui.util.ToolLocalization.localizedName(it) }
            ?: type.takeIf { it.isNotBlank() }?.let { com.antigravity.mobile.ui.util.ToolLocalization.localizedName(it) }
            ?: "运行终端命令".loc

    val displayCommand: String
        get() = commandLine.takeIf { it.isNotBlank() }
            ?: command?.takeIf { it.isNotBlank() }
            ?: ""
}

/** 阻塞当前回合的后台任务（排除常驻守护任务）。 */
val List<RunningTaskItem>.blockingTasks: List<RunningTaskItem>
    get() = filter { it.blocksTurn }

/** 是否存在阻塞当前回合的后台任务；只有常驻守护任务时返回 false。 */
val List<RunningTaskItem>.hasBlockingTasks: Boolean
    get() = any { it.blocksTurn }

/** 父会话通过 invoke_subagent 派发的一个子代理（网关 stream 的 `subagents`）。 */
@Serializable
data class SubagentItem(
    val conversationId: String = "",
    val typeName: String? = null,
    val role: String? = null,
    val prompt: String? = null,
    val modelTier: String? = null,
    val runtimeModel: String? = null,
    val stepIndex: Int? = null,
    /** "running" | "done" | "gone"；网关尚未补全时为 null。 */
    val status: String? = null,
    val stepCount: Int? = null,
    val title: String? = null
) {
    val isRunning: Boolean get() = status == "running"
    val isGone: Boolean get() = status == "gone"

    /** 列表里展示的名字：优先角色，其次类型，最后用会话 ID 前 8 位兜底。 */
    val displayName: String
        get() = role?.trim()?.takeIf { it.isNotEmpty() }
            ?: typeName?.trim()?.takeIf { it.isNotEmpty() }
            ?: conversationId.take(8)

    val statusText: String
        get() = when (status) {
            "running" -> "运行中".loc
            "gone" -> "已清理".loc
            "done" -> "已结束".loc
            else -> ""
        }

    /** 规范化请求档位（去掉 MODEL_TIER_ 前缀），便于详情页与实际落盘模型对照。 */
    val requestedTierDisplay: String?
        get() {
            val raw = modelTier?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            val cleaned = raw
                .replace(Regex("^MODEL_TIER_", RegexOption.IGNORE_CASE), "")
                .replace('_', ' ')
                .trim()
            return cleaned.takeIf { it.isNotEmpty() }?.lowercase()
        }

    /** 优先展示真实落盘模型（紧凑格式，如 `Hydrogen · Orch` / `Neon · High`），未落盘时回退请求档位。 */
    val modelBadgeText: String?
        get() {
            val rt = runtimeModel?.trim()?.takeIf { it.isNotEmpty() }
            if (rt != null) {
                val compact = ModelDefaults.formatFullNameToCompact(rt)
                    .replace("· Orchestrator", "· Orch")
                return compact.ifEmpty { rt }
            }
            val tier = requestedTierDisplay ?: return null
            return tier.split(' ').filter { it.isNotEmpty() }
                .joinToString(" ") { w -> w.replaceFirstChar { it.uppercase() } }
        }
}

@Serializable
data class TurnOutlineItem(
    val turnIndex: Int = 1,
    val firstMessageId: String = "",
    val messageOffset: Int = 0,
    val messageCount: Int = 0,
    val startStepIndex: Int? = null,
    val endStepIndex: Int? = null,
    val promptSummary: String = "",
    val totalTools: Int = 0,
    val artifacts: List<ArtifactItem>? = null,
    val hasError: Boolean = false,
    val modelName: String? = null,
    val timestamp: String? = null,
    val isVoiceTurn: Boolean = false
)

@Serializable
data class StreamUpdatePayload(
    val type: String = "update", // "init", "update", "noop", "error"
    val rev: Long = 0L,
    val cascadeId: String = "",
    val title: String? = null,
    val status: String = "",
    val hasError: Boolean = false,
    val errorMessage: String? = null,
    val duration: String? = null,
    val totalSteps: Int = 0,
    val totalTools: Int = 0,
    val totalMessages: Int = 0,
    val hasMore: Boolean = false,
    val nextOffset: Int = 0,
    val workspaceUri: String? = null,
    val messages: List<GatewayMessageItem>? = null,
    val turnOutline: List<TurnOutlineItem>? = null,
    val queuedMessages: List<QueuedMessageItem>? = null,
    val runningTasks: List<RunningTaskItem>? = null,
    val subagents: List<SubagentItem>? = null,
    /** 非空说明该会话本身是子代理。 */
    val parentConversationId: String? = null,
    val subagentRole: String? = null,
    val isFullSnapshot: Boolean = false,
    val cascadeConfigRaw: String? = null,
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val activeModel: String? = null,
    /** 完整展示名，如 "Gemini 3.8 Flash (High)"。 */
    val activeModelName: String? = null,
    val modelDisplayName: String? = null,
    /** 桌面端 Antigravity 客户端当前选中的模型。 */
    val desktopActiveModel: String? = null,
    val desktopActiveModelName: String? = null,
    /** 会话发起时间（ISO-8601）。 */
    val startedAt: String? = null,
    /** delta=1 增量帧：messages 仅含新增/变化项，messageIds 为当前窗口完整有序 ID（见 StreamDeltaReassembler）。 */
    val delta: Boolean = false,
    val messageIds: List<String>? = null,
    val swarm: SwarmBrief? = null,
    val contextMeter: ContextMeterItem? = null,
    val suggestedResponses: List<String>? = null,
    val activeFocus: String? = null,
    val activeFocusStartedAt: String? = null,
    val activePlan: StepPlanItem? = null
)

@Serializable
data class RevertDiffLine(
    val text: String = "",
    val type: String = "UNCHANGED" // "INSERT", "DELETE", "UNCHANGED"
)

@Serializable
data class RevertPreviewFile(
    val fileUri: String = "",
    val fileName: String = "",
    val actionType: String = "MODIFY", // "MODIFY", "CREATE", "DELETE"
    val additions: Int = 0,
    val deletions: Int = 0,
    val diffLines: List<RevertDiffLine> = emptyList()
)

@Serializable
data class RevertPreviewResponse(
    val cascadeId: String = "",
    val stepIndex: Int = 0,
    val targetStepIndex: Int = 0,
    val files: List<RevertPreviewFile> = emptyList(),
    val hasCodeChanges: Boolean = false
)

@Serializable
data class RevertPreviewRequest(
    val cascadeId: String = "",
    val stepIndex: Int = 0
)

@Serializable
data class RevertExecuteRequest(
    val cascadeId: String = "",
    val stepIndex: Int = 0,
    val conversationOnly: Boolean = false
)

