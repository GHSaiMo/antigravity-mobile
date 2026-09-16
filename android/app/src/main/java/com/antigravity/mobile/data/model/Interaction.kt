package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class InteractionOption(
    val id: String,
    val text: String,
    @SerialName("is_default") val isDefault: Boolean = false
)

@Serializable
data class PendingInteraction(
    val type: String, // "command_execution_approval", "confirm_action", "multiple_choice", etc.
    @SerialName("step_index") val stepIndex: Int,
    @SerialName("default_option_id") val defaultOptionId: String? = null,
    val prompt: String? = null,
    val command: String? = null,
    val reason: String? = null,
    val options: List<InteractionOption>? = null
)

@Serializable
data class InteractionRespondRequest(
    @SerialName("cascade_id") val cascadeId: String,
    @SerialName("step_index") val stepIndex: Int,
    @SerialName("response_type") val responseType: String,
    @SerialName("selected_option_id") val selectedOptionId: String? = null,
    @SerialName("confirmed") val confirmed: Boolean? = null,
    @SerialName("custom_text") val customText: String? = null
)
