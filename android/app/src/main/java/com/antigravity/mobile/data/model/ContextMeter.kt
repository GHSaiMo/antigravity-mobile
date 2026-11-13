package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ContextMeterItem(
    val usedTokens: Long = 0,
    val maxTokens: Long = 200000,
    val percentage: Double = 0.0,
    val inputTokens: Long = 0,
    val outputTokens: Long = 0,
    val cacheReadTokens: Long = 0,
    val thinkingTokens: Long = 0,
    val timeToFirstTokenSec: Double = 0.0,
    val streamingDurationSec: Double = 0.0,
    val tokensPerSecond: Double = 0.0,
    val isOverflow: Boolean = false,
    val errorKind: String? = null
)
