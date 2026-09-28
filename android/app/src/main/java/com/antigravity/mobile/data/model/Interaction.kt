package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class InteractionOption(
    val id: String = "",
    val text: String = "",
    val scope: Int? = null,
    val isDeny: Boolean = false,
    @SerialName("is_default") val isDefault: Boolean = false
)

@Serializable
data class InteractionQuestion(
    val question: String = "",
    val isMultiSelect: Boolean = false,
    val options: List<InteractionOption> = emptyList(),
    val defaultOptionId: String? = null,
    val hasWriteIn: Boolean = false,
    val writeInLabel: String? = null,
    val writeInPlaceholder: String? = null
)

@Serializable
data class QuestionResponse(
    val questionIndex: Int = 0,
    val selectedOptionIds: List<String> = emptyList(),
    val writeInResponse: String? = null,
    val skipped: Boolean = false
)

@Serializable
data class PendingInteraction(
    val type: String = "", // "permission", "ask_question", "file_permission", "run_command"
    val trajectoryId: String = "",
    val stepIndex: Int = 0,
    val title: String = "",
    val target: String? = null,
    val action: String? = null,
    val description: String? = null,
    val options: List<InteractionOption> = emptyList(),
    val isMultiSelect: Boolean = false,
    val defaultOptionId: String? = null,
    val hasWriteIn: Boolean = false,
    val writeInLabel: String? = null,
    val writeInPlaceholder: String? = null,
    val questions: List<InteractionQuestion>? = null,
    // Backward compatibility aliases
    val prompt: String? = null,
    val command: String? = null,
    val reason: String? = null
)

@Serializable
data class InteractionSubmitRequest(
    val cascadeId: String,
    val trajectoryId: String = "",
    val stepIndex: Int = 0,
    val type: String = "",
    val optionId: String? = null,
    val scope: Int = 1,
    val allow: Boolean = true,
    val writeInResponse: String = "",
    val skipped: Boolean = false,
    val target: String? = null,
    val questionResponses: List<QuestionResponse>? = null
)

@Deprecated("Use InteractionSubmitRequest instead")
@Serializable
data class InteractionRespondRequest(
    @SerialName("cascade_id") val cascadeId: String,
    @SerialName("step_index") val stepIndex: Int = 0,
    @SerialName("response_type") val responseType: String = "",
    @SerialName("selected_option_id") val selectedOptionId: String? = null,
    @SerialName("confirmed") val confirmed: Boolean? = null,
    @SerialName("custom_text") val customText: String? = null
)
