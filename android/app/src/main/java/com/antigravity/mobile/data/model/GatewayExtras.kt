package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable

/** 会话内容搜索（language_server SearchConversations）。偏移量按 Unicode 码点计，不是 UTF-16 单元。 */
@Serializable
data class SearchMatchRange(
    val startOffset: Int = 0,
    val endOffsetExclusive: Int = 0
)

@Serializable
data class ConversationSearchResult(
    val cascadeId: String = "",
    val title: String = "",
    val workspaceName: String? = null,
    val lastModifiedTime: String? = null,
    val snippet: String = "",
    val snippetMatchRanges: List<SearchMatchRange> = emptyList(),
    val matchSource: String = "",
    val matchedStepIndex: Int? = null
)

@Serializable
data class ConversationSearchResponse(
    val results: List<ConversationSearchResult> = emptyList()
)

