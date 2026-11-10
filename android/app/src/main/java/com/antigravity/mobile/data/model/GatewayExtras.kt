package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable

/** 本会话改动（正向累计 diff）。文件结构与撤回预览一致，直接复用 [RevertPreviewFile] 与 Diff 渲染。 */
@Serializable
data class CascadeChangesRequest(
    val cascadeId: String = "",
    val fromStepIndex: Int? = null
)

@Serializable
data class CascadeChangesResponse(
    val cascadeId: String = "",
    val files: List<RevertPreviewFile> = emptyList(),
    val additions: Int = 0,
    val deletions: Int = 0,
    val hasChanges: Boolean = false
)

/** Git 工作区状态与提交。 */
@Serializable
data class GitFileStatus(
    val path: String = "",
    val origPath: String? = null,
    val index: String = "",
    val worktree: String = "",
    val staged: Boolean = false,
    val status: String = "MODIFIED" // MODIFIED / ADDED / DELETED / RENAMED / UNTRACKED / CONFLICT
)

@Serializable
data class GitHubBranchTarget(
    val host: String? = null,
    val owner: String = "",
    val repo: String = "",
    val branch: String? = null
)

@Serializable
data class GitPullRequestItem(
    val number: Int = 0,
    val title: String = "",
    val url: String = "",
    val state: String = "OPEN", // OPEN / DRAFT / MERGED / CLOSED
    val authorLogin: String? = null,
    val headRefName: String? = null,
    val baseRefName: String? = null
)

@Serializable
data class GitStatusResponse(
    val repoName: String = "",
    val branch: String = "",
    val upstream: String? = null,
    val ahead: Int = 0,
    val behind: Int = 0,
    val detached: Boolean = false,
    val files: List<GitFileStatus> = emptyList(),
    val clean: Boolean = false,
    val githubTarget: GitHubBranchTarget? = null,
    val pullRequests: List<GitPullRequestItem> = emptyList()
)

/** 会话章节目录 TOC (Antigravity 2.30.0 GetChapterTitles)。 */
@Serializable
data class CascadeStepRange(
    val startStepIndex: Int = 0,
    val endStepIndexExclusive: Int = 0
)

@Serializable
data class CascadeChapterTitleItem(
    val startStepIndex: Int = 0,
    val endStepIndexExclusive: Int = 0,
    val title: String = "",
    val generatedAt: String? = null
)

@Serializable
data class CascadeChaptersRequest(
    val cascadeId: String = "",
    val ranges: List<CascadeStepRange> = emptyList()
)

@Serializable
data class CascadeChaptersResponse(
    val chapters: List<CascadeChapterTitleItem> = emptyList()
)

@Serializable
data class GitCommitRequest(
    val cascadeId: String = "",
    val message: String = "",
    val paths: List<String> = emptyList(),
    val push: Boolean = false
)

@Serializable
data class GitCommitResponse(
    val committed: Boolean = false,
    val pushed: Boolean = false,
    val commitId: String? = null,
    val output: String? = null,
    val pushError: String? = null
)

@Serializable
data class GitCascadeRequest(val cascadeId: String = "")

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

@Serializable
data class MarkdownExportResponse(val markdown: String = "")

// region 模型目录 / 默认模型

/** 网关 /gateway/models 返回的可选模型。provider 为 "gemini" | "claude" | "other"。 */
@Serializable
data class ModelOption(
    val id: String = "",
    val name: String = "",
    val provider: String = "other",
    val enum: String? = null,
    val supportsImages: Boolean = false,
    val thinking: Boolean = false,
    val tag: String? = null
)

@Serializable
data class ModelsResponse(
    val models: List<ModelOption> = emptyList(),
    /** 网关建议的各厂商默认模型（provider -> model id）。 */
    val defaults: Map<String, String> = emptyMap(),
    /** false 表示网关暂时取不到实时列表，返回的是内置兜底。 */
    val live: Boolean = false
)

/** 默认模型与当前会话模型的解析规则（纯函数，便于单测）。 */
object ModelDefaults {
    const val FALLBACK_GEMINI = "gemini-3.8-flash-high"
    const val FALLBACK_CLAUDE = "claude-opus-4-6-thinking"

    private val CLAUDE_ENUM_TOKENS = listOf("m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405")

    fun isClaude(modelId: String): Boolean =
        modelId.contains("claude", ignoreCase = true) ||
            CLAUDE_ENUM_TOKENS.any { modelId.contains(it, ignoreCase = true) }

    /** 格式化模型选择器选项展示名：过滤掉 "New" 标签及前导的点符号，避免手机端全量标记 New 造成视觉冗余。 */
    fun formatPickerLabel(name: String, tag: String?): String {
        val cleanTag = tag?.trim()
        return if (!cleanTag.isNullOrBlank() && !cleanTag.equals("new", ignoreCase = true)) {
            "$name · $cleanTag"
        } else {
            name
        }
    }

    /** 输入框上方胶囊的文字：只有厂商名，保持简洁。 */
    fun providerLabel(modelId: String): String = when {
        isClaude(modelId) -> "Claude"
        modelId.contains("gpt", ignoreCase = true) -> "GPT"
        else -> "Gemini"
    }

    /** 胶囊点击后要切到的默认模型：Claude → Gemini 默认；其它 → Claude 默认。 */
    fun toggleTarget(current: String, geminiDefault: String, claudeDefault: String): String =
        if (isClaude(current)) geminiDefault else claudeDefault

    /**
     * 网关推送的 activeModel 如何落到界面状态：
     * - 为空：保持当前；
     * - 已是可读模型 id：原样保留，让后续消息继续用该会话实际的模型；
     * - 只拿到未解析的裸枚举（MODEL_...）：退回该厂商的默认模型。
     */
    fun resolveActive(raw: String?, current: String, geminiDefault: String, claudeDefault: String): String {
        val value = raw?.trim().orEmpty()
        if (value.isEmpty()) return current
        if (value.startsWith("MODEL_", ignoreCase = true)) {
            return if (isClaude(value)) claudeDefault else geminiDefault
        }
        return value
    }

    /** 设置里保存的默认模型已不在实时列表时，换成网关建议的默认值。 */
    fun validated(saved: String, provider: String, catalog: ModelsResponse): String {
        if (!catalog.live) return saved
        if (catalog.models.any { it.id == saved && it.provider == provider }) return saved
        return catalog.defaults[provider]
            ?: catalog.models.firstOrNull { it.provider == provider }?.id
            ?: saved
    }
}

// endregion

// region 斜杠命令

/** 网关 /gateway/slash-commands 返回的一项。发送时只需把 [name] 带回去，网关会补全权威定义。 */
@Serializable
data class SlashCommandOption(
    val name: String = "",
    val title: String = "",
    val description: String = "",
    val icon: String? = null,
    /** "system" 系统命令 | "skill" 技能。 */
    val kind: String = "system"
)

@Serializable
data class SlashCommandsResponse(val commands: List<SlashCommandOption> = emptyList())

/** 输入框里 "/xxx" 的筛选规则（纯函数，便于单测）。 */
object SlashCommandFilter {
    /**
     * 检测输入框末尾是否正在键入 "/xxx" 斜杠命令；若命中则返回要筛选的关键字（不含 "/"），否则返回 null。
     * 支持在空框开头输入（"/" -> ""，"/pl" -> "pl"），也支持在已有文字、空格、换行或标点后输入（"思路。/" -> ""，"hello /pl" -> "pl"）。
     * 为避免误触 URL（https://...）、文件路径（/Users/foo）或英文斜杠词组（Go/Rust），"/" 前紧邻 ASCII 字母数字或路径符号时不触发。
     */
    fun queryOf(input: String): String? {
        val slashIdx = trailingSlashIndex(input) ?: return null
        return input.substring(slashIdx + 1)
    }

    /** 选中命令后，仅剥离输入框末尾触发菜单的 "/xxx" 片段（及紧邻其前的水平空格），完整保留此前已输入的正文。 */
    fun stripTrailingQuery(input: String): String {
        val slashIdx = trailingSlashIndex(input) ?: return input
        var end = slashIdx
        while (end > 0 && (input[end - 1] == ' ' || input[end - 1] == '\t')) {
            end--
        }
        return input.substring(0, end)
    }

    private fun trailingSlashIndex(input: String): Int? {
        val slashIdx = input.lastIndexOf('/')
        if (slashIdx < 0) return null
        val rest = input.substring(slashIdx + 1)
        if (rest.any { it.isWhitespace() }) return null
        if (slashIdx > 0) {
            val prevChar = input[slashIdx - 1]
            if (isDisallowedBeforeSlash(prevChar)) return null
        }
        return slashIdx
    }

    private fun isDisallowedBeforeSlash(ch: Char): Boolean {
        if (ch.code < 128 && (ch.isLetterOrDigit())) return true
        return ch == '/' || ch == ':' || ch == '.' || ch == '~' || ch == '_' || ch == '-'
    }

    /**
     * 把 "/plan 帮我规划" 拆成（命令，"帮我规划"）；开头不是已知命令（或命令列表还没加载）时原样返回。
     * 用于队列里的「编辑 / 立即发送」：队列显示的是服务端生成的显示文本，需要还原成「命令 + 文本」。
     */
    fun splitPrefix(text: String, commands: List<SlashCommandOption>): Pair<SlashCommandOption?, String> {
        if (!text.startsWith("/") || commands.isEmpty()) return null to text
        val firstToken = text.substring(1).takeWhile { !it.isWhitespace() }
        val command = commands.firstOrNull { it.name == firstToken } ?: return null to text
        return command to text.substring(1 + firstToken.length).trimStart()
    }

    /** 名称前缀命中优先，其次名称/标题/描述包含；保持原有顺序。 */
    fun filter(all: List<SlashCommandOption>, query: String): List<SlashCommandOption> {
        val q = query.trim().lowercase()
        if (q.isEmpty()) return all
        val prefix = all.filter { it.name.lowercase().startsWith(q) }
        val rest = all.filter {
            it !in prefix && (it.name.lowercase().contains(q) || it.title.lowercase().contains(q) || it.description.lowercase().contains(q))
        }
        return prefix + rest
    }
}

/** 用户消息中引用的斜杠命令 / 技能（由网关从 trajectory.userInput.items 提取）。 */
@Serializable
data class SlashCommandMention(
    val name: String = "",
    val icon: String? = null,
    val kind: String = "skill"
)

/** 桌面端（Antigravity Desktop）斜杠命令与技能图标矢量目录（统一归一化到 0..960 坐标系，仅含 M/L/Q/Z 指令）。 */
object SlashCommandIconCatalog {
    val defaultSystemIcons: Map<String, String> = mapOf(
        "grill-me" to "forum",
        "plan" to "ballot",
        "goal" to "timer",
        "compact" to "collapse_all",
        "schedule" to "schedule",
        "automation" to "schedule",
        "browser" to "language",
        "teamwork" to "graph_2",
        "learn" to "lightbulb_2",
        "boost" to "bolt",
        "btw" to "chat_add_on"
    )

    val svgPaths: Map<String, String> = mapOf(
        "forum" to "M852.31 826.15 L710.77 684.61 L320 684.61 Q290.81 684.61 270.02 663.82 Q249.23 643.03 249.23 613.84 L249.23 589.23 L684.62 589.23 Q714.45 589.23 735.69 567.99 Q756.93 546.75 756.93 516.92 L756.93 240 L781.54 240 Q810.73 240 831.52 260.79 Q852.31 281.58 852.31 310.77 L852.31 826.15 Z M167.69 505.85 L224.31 449.23 L604.62 449.23 Q610.01 449.23 613.47 445.77 Q616.93 442.31 616.93 436.92 L616.93 172.31 Q616.93 166.92 613.47 163.46 Q610.01 160 604.62 160 L180 160 Q174.61 160 171.15 163.46 Q167.69 166.92 167.69 172.31 L167.69 505.85 Z M107.69 650.77 L107.69 172.31 Q107.69 142.48 128.93 121.24 Q150.17 100 180 100 L604.62 100 Q634.45 100 655.69 121.24 Q676.93 142.48 676.93 172.31 L676.93 436.92 Q676.93 466.75 655.69 487.99 Q634.45 509.23 604.62 509.23 L249.23 509.23 L107.69 650.77 Z",
        "code_blocks" to "M384 610.15 L426.15 567.38 L338.77 480 L425.77 393 L383.62 350.23 L253.85 480 L384 610.15 Z M576 610.15 L706.15 480 L576 349.85 L533.85 392.62 L621.23 480 L533.85 567.38 L576 610.15 Z M212.31 820 Q182 820 161 799 Q140 778 140 747.69 L140 212.31 Q140 182 161 161 Q182 140 212.31 140 L747.69 140 Q778 140 799 161 Q820 182 820 212.31 L820 747.69 Q820 778 799 799 Q778 820 747.69 820 L212.31 820 Z M212.31 760 L747.69 760 Q752.31 760 756.15 756.15 Q759.99 752.3 760 747.69 L760 212.31 Q760 207.69 756.15 203.85 Q752.3 200.01 747.69 200 L212.31 200 Q207.69 200 203.85 203.85 Q200.01 207.7 200 212.31 L200 747.69 Q200 752.31 203.85 756.15 Q207.7 759.99 212.31 760 Z",
        "ballot" to "M480 400 L672.31 400 L672.31 340 L480 340 L480 400 Z M480 620 L672.31 620 L672.31 560 L480 560 L480 620 Z M405.73 415.73 Q424.61 396.85 424.61 370 Q424.61 343.15 405.73 324.27 Q386.85 305.39 360 305.39 Q333.15 305.39 314.27 324.27 Q295.39 343.15 295.39 370 Q295.39 396.85 314.27 415.73 Q333.15 434.61 360 434.61 Q386.85 434.61 405.73 415.73 Z M405.73 635.73 Q424.61 616.85 424.61 590 Q424.61 563.15 405.73 544.27 Q386.85 525.39 360 525.39 Q333.15 525.39 314.27 544.27 Q295.39 563.15 295.39 590 Q295.39 616.85 314.27 635.73 Q333.15 654.61 360 654.61 Q386.85 654.61 405.73 635.73 Z M212.31 820 Q182 820 161 799 Q140 778 140 747.69 L140 212.31 Q140 182 161 161 Q182 140 212.31 140 L747.69 140 Q778 140 799 161 Q820 182 820 212.31 L820 747.69 Q820 778 799 799 Q778 820 747.69 820 L212.31 820 Z M212.31 760 L747.69 760 Q752.31 760 756.15 756.15 Q759.99 752.3 760 747.69 L760 212.31 Q760 207.69 756.15 203.85 Q752.3 200.01 747.69 200 L212.31 200 Q207.69 200 203.85 203.85 Q200.01 207.7 200 212.31 L200 747.69 Q200 752.31 203.85 756.15 Q207.7 759.99 212.31 760 Z",
        "timer" to "M367.69 110 L367.69 50 L592.31 50 L592.31 110 L367.69 110 Z M450 552.31 L510 552.31 L510 327.69 L450 327.69 L450 552.3 Z M348 833.23 Q286.15 806.46 239.85 760.15 Q193.55 713.84 166.77 652 Q139.99 590.16 140 520 Q140.01 449.84 166.77 388 Q193.53 326.16 239.85 279.85 Q286.17 233.54 348 206.77 Q409.83 180 480 180 Q540.08 180 595.73 200.38 Q651.38 220.76 699.08 258.76 L748.93 208.91 L791.08 251.06 L741.23 300.91 Q779.23 348.6 799.61 404.26 Q819.99 459.92 820 520 Q820 590.15 793.23 652 Q766.46 713.85 720.15 760.15 Q673.84 806.45 612 833.23 Q550.16 860.01 480 860 Q409.84 859.99 348 833.23 Z M678 718 Q760 636 760 520 Q760 404 678 322 Q596 240 480 240 Q364 240 282 322 Q200 404 200 520 Q200 636 282 718 Q364 800 480 800 Q596 800 678 718 Z",
        "collapse_all" to "M296 866.15 L253.85 824 L480 597.85 L706.15 824 L664 866.15 L480 682.15 L296 866.15 Z M480 362.15 L253.85 136 L296 93.85 L480 277.85 L664 93.85 L706.15 136 L480 362.15 Z",
        "schedule" to "M618.92 661.08 L661.07 618.93 L510 467.84 L510 280 L450 280 L450 492.15 L618.92 661.08 Z M480.07 860 Q401.23 860 331.87 830.08 Q262.51 800.16 211.18 748.87 Q159.85 697.58 129.93 628.24 Q100.01 558.9 100 480.07 Q99.99 401.24 129.92 331.87 Q159.85 262.5 211.13 211.19 Q262.41 159.88 331.76 129.94 Q401.11 100 479.93 100 Q558.75 100 628.13 129.92 Q697.51 159.84 748.81 211.13 Q800.11 262.42 830.06 331.76 Q860.01 401.1 860 479.93 Q859.99 558.76 830.08 628.13 Q800.17 697.5 748.87 748.82 Q697.57 800.14 628.24 830.07 Q558.91 860 480.07 860 Z M480 800 Q613 800 706.5 706.5 Q800 613 800 480 Q800 347 706.5 253.5 Q613 160 480 160 Q347 160 253.5 253.5 Q160 347 160 480 Q160 613 253.5 706.5 Q347 800 480 800 Z",
        "language" to "M332.5 830.04 Q263.15 800.08 211.54 748.46 Q159.93 696.84 129.96 627.5 Q99.99 558.16 100 479.69 Q100.01 401.22 129.96 332.19 Q159.91 263.16 211.54 211.54 Q263.17 159.92 332.5 129.96 Q401.83 100 480.31 100 Q558.79 100 627.81 129.96 Q696.83 159.92 748.46 211.54 Q800.09 263.16 830.04 332.19 Q859.99 401.22 860 479.69 Q860.01 558.16 830.04 627.5 Q800.07 696.84 748.46 748.46 Q696.85 800.08 627.81 830.04 Q558.77 860 480.31 860 Q401.85 860 332.5 830.04 Z M480 799.15 Q510.62 758.53 531.54 717.23 Q552.46 675.93 565.62 626.92 L394.38 626.92 Q408.3 677.46 428.84 718.77 Q449.38 760.08 480 799.15 Z M402.54 788.15 Q379.54 755.15 361.23 713.11 Q342.92 671.07 332.77 626.92 L197.08 626.92 Q228.77 689.23 282.08 731.61 Q335.39 773.99 402.54 788.15 Z M557.46 788.15 Q624.61 774 677.92 731.62 Q731.23 689.24 762.92 626.93 L627.23 626.93 Q615.15 671.47 596.84 713.51 Q578.53 755.55 557.45 788.16 Z M171.92 566.92 L320.62 566.92 Q316.85 544.61 315.16 523.19 Q313.47 501.77 313.46 480 Q313.45 458.23 315.15 436.81 Q316.85 415.39 320.61 393.08 L171.92 393.08 Q166.15 413.46 163.07 435.46 Q159.99 457.46 160 480 Q160.01 502.54 163.08 524.54 Q166.15 546.54 171.93 566.92 Z M380.61 566.92 L579.39 566.92 Q583.16 544.61 584.85 523.57 Q586.54 502.53 586.54 480 Q586.54 457.47 584.85 436.42 Q583.16 415.37 579.39 393.07 L380.61 393.07 Q376.84 415.38 375.15 436.42 Q373.46 457.46 373.46 480 Q373.46 502.54 375.15 523.58 Q376.84 544.62 380.61 566.93 Z M639.38 566.92 L788.08 566.92 Q793.85 546.54 796.93 524.54 Q800.01 502.54 800 480 Q799.99 457.46 796.92 435.46 Q793.85 413.46 788.07 393.08 L639.38 393.08 Q643.15 415.39 644.84 436.81 Q646.53 458.23 646.54 480 Q646.55 501.77 644.85 523.19 Q643.15 544.61 639.39 566.92 Z M627.23 333.08 L762.92 333.08 Q730.85 270 678.5 228.38 Q626.15 186.76 557.46 171.46 Q580.46 206.38 598.38 247.85 Q616.3 289.32 627.23 333.08 Z M394.38 333.08 L565.62 333.08 Q551.7 282.93 530.58 240.66 Q509.46 198.39 480 160.85 Q450.54 198.39 429.42 240.66 Q408.3 282.93 394.38 333.08 Z M197.07 333.08 L332.77 333.08 Q343.69 289.31 361.62 247.85 Q379.55 206.39 402.54 171.46 Q333.46 186.77 281.31 228.58 Q229.16 270.39 197.08 333.08 Z",
        "graph_2" to "M206.97 860 Q162.4 860 131.2 828.7 Q100 797.4 100 752.69 Q100 716.38 121.92 687.88 Q143.84 659.38 177.3 650.07 L177.3 557.68 Q177.3 513.07 208.84 481.53 Q240.38 449.99 285 450 L450 450 L450 309.92 Q416.54 300.61 394.62 272.11 Q372.7 243.61 372.7 207.3 Q372.7 162.59 403.9 131.29 Q435.1 99.99 479.66 100 Q524.22 100.01 555.77 131.3 Q587.32 162.59 587.31 207.31 Q587.31 243.62 565.39 272.12 Q543.47 300.62 510 309.92 L510 450 L675 450 Q719.61 450 751.15 481.54 Q782.69 513.08 782.69 557.69 L782.69 650.08 Q816.15 659.39 838.07 687.89 Q859.99 716.39 860 752.69 Q860 797.4 828.46 828.7 Q796.92 860 752.35 860 Q707.78 860 676.58 828.7 Q645.38 797.4 645.38 752.69 Q645.38 716.38 667.3 687.88 Q689.22 659.38 722.68 650.07 L722.68 557.68 Q722.68 537.41 708.97 523.7 Q695.26 509.99 675 510 L510 510 L510 650.08 Q543.46 659.39 565.38 687.89 Q587.3 716.39 587.3 752.7 Q587.3 797.41 555.76 828.71 Q524.22 860.01 479.66 860 Q435.1 859.99 403.89 828.7 Q372.68 797.41 372.69 752.69 Q372.69 716.38 394.61 687.88 Q416.53 659.38 450 650.08 L450 510 L285 510 Q264.73 510 251.02 523.71 Q237.31 537.42 237.31 557.69 L237.31 650.08 Q270.77 659.39 292.69 687.89 Q314.61 716.39 314.61 752.7 Q314.61 797.41 283.07 828.71 Q251.53 860.01 206.97 860 Z M207.12 800 Q227 800 240.81 786.4 Q254.62 772.8 254.62 752.71 Q254.62 732.62 240.9 719 Q227.18 705.38 206.92 705.38 Q187.23 705.38 173.61 719.19 Q159.99 733 160 752.69 Q160.01 772.38 173.62 786.19 Q187.23 800 207.12 800 Z M479.81 800 Q499.69 800 513.5 786.4 Q527.31 772.8 527.31 752.71 Q527.31 732.62 513.6 719 Q499.89 705.38 479.62 705.38 Q459.93 705.38 446.31 719.19 Q432.69 733 432.69 752.69 Q432.69 772.38 446.31 786.19 Q459.93 800 479.81 800 Z M752.5 800 Q772.38 800 786.19 786.4 Q800 772.8 800 752.71 Q800 732.62 786.29 719 Q772.58 705.38 752.31 705.38 Q732.62 705.38 719 719.19 Q705.38 733 705.38 752.69 Q705.38 772.38 719 786.19 Q732.62 800 752.5 800 Z M479.81 254.62 Q499.69 254.62 513.5 241.02 Q527.31 227.42 527.31 207.33 Q527.31 187.24 513.6 173.62 Q499.89 160 479.62 160 Q459.93 160 446.31 173.81 Q432.69 187.62 432.69 207.31 Q432.69 227 446.31 240.81 Q459.93 254.62 479.81 254.62 Z",
        "lightbulb_2" to "M407.31 712.31 Q377.39 712.31 356.2 691.12 Q335.01 669.93 335 640 L335 576.15 Q280.69 539.84 250.35 482.69 Q220.01 425.54 220 360.38 Q220 251.84 295.73 175.92 Q371.46 100 480 100 Q588.54 100 664.27 175.73 Q740 251.46 740 360 Q740 425.54 709.65 482.38 Q679.3 539.22 625 576.15 L625 640 Q625 669.92 603.81 691.11 Q582.62 712.3 552.7 712.3 L407.31 712.3 Z M407.31 652.31 L552.69 652.31 Q558.08 652.31 561.54 648.85 Q565 645.39 565 640 L565 544.54 L594 524 Q634.62 495.62 657.31 452.31 Q680 409 680 360 Q680 277 621.5 218.5 Q563 160 480 160 Q397 160 338.5 218.5 Q280 277 280 360 Q280 409 302.69 452.31 Q325.38 495.62 366 524 L395 544.54 L395 640 Q395 645.39 398.46 648.85 Q401.92 652.31 407.31 652.31 Z M403.46 860 Q390.69 860 382.08 851.38 Q373.47 842.76 373.46 830 L373.46 800 L586.54 800 L586.54 830 Q586.54 842.77 577.92 851.38 Q569.3 859.99 556.54 860 L403.46 860 Z",
        "bolt" to "M422 728 L629 480 L469 480 L498 253 L313 520 L452 520 L422 728 Z M343.08 857.69 L383.08 580 L198.08 580 L527.69 104.23 L576.92 104.23 L537.31 420 L757.31 420 L392.31 857.69 L343.08 857.69 Z",
        "code" to "M320 706.15 L93.85 480 L320 253.85 L362.77 296.62 L178.77 480.62 L362.15 664 L320 706.15 Z M640 706.15 L597.23 663.38 L781.23 479.38 L597.85 296 L640 253.85 L866.15 480 L640 706.15 Z",
        "chat_add_on" to "M130 775.77 L130 202.31 Q130 172.48 151.24 151.24 Q172.48 130 202.31 130 L677.69 130 Q707.52 130 728.76 151.24 Q750 172.48 750 202.31 L750 401.85 Q742.5 400.62 735 400.31 Q727.5 400 720 400 Q712.5 400 705 400.31 Q697.5 400.62 690 401.85 L690 202.31 Q690 196.92 686.54 193.46 Q683.08 190 677.69 190 L202.31 190 Q196.92 190 193.46 193.46 Q190 196.92 190 202.31 L190 610 L481.85 610 Q480.62 617.5 480.31 625 Q480 632.5 480 640 Q480 647.5 480.31 655 Q480.62 662.5 481.85 670 L235.77 670 L130 775.77 Z M282.31 350 L597.69 350 L597.69 290 L282.31 290 L282.31 350 Z M282.31 510 L477.69 510 L477.69 450 L282.31 450 L282.31 510 Z M690 790 L690 670 L570 670 L570 610 L690 610 L690 490 L750 490 L750 610 L870 610 L870 670 L750 670 L750 790 L690 790 Z"
    )

    fun resolveIcon(name: String, icon: String?, kind: String?): String {
        val trimmed = icon?.trim()
        if (!trimmed.isNullOrEmpty() && svgPaths.containsKey(trimmed)) {
            return trimmed
        }
        val cleanName = name.trim()
        val sys = defaultSystemIcons[cleanName]
        if (sys != null) return sys
        return if (kind == "skill") "code_blocks" else "code"
    }
}

sealed interface SlashTextSegment {
    data class Text(val content: String) : SlashTextSegment
    data class Slash(val mention: SlashCommandMention) : SlashTextSegment
}

object SlashCommandTextTokenizer {
    private val unixRootDirs = setOf(
        "tmp", "var", "usr", "etc", "opt", "bin", "sbin", "dev",
        "home", "root", "proc", "sys", "vol1", "mnt", "media",
        "srv", "run", "users", "library", "system", "applications",
        "volumes", "private", "cores"
    )

    private val tokenRegex = Regex("""(^|\s)/([A-Za-z0-9_-]+)(?=$|\s)""")

    fun tokenize(
        text: String,
        mentions: List<SlashCommandMention> = emptyList(),
        availableCommands: List<SlashCommandOption> = emptyList()
    ): List<SlashTextSegment> {
        if (text.isEmpty() || !text.contains("/")) {
            return if (text.isEmpty()) emptyList() else listOf(SlashTextSegment.Text(text))
        }

        val lookup = mutableMapOf<String, SlashCommandMention>()
        for ((sysName, sysIcon) in SlashCommandIconCatalog.defaultSystemIcons) {
            lookup[sysName] = SlashCommandMention(name = sysName, icon = sysIcon, kind = "system")
        }
        for (cmd in availableCommands) {
            val icon = SlashCommandIconCatalog.resolveIcon(cmd.name, cmd.icon, cmd.kind)
            lookup[cmd.name] = SlashCommandMention(name = cmd.name, icon = icon, kind = cmd.kind)
        }
        for (m in mentions) {
            val icon = SlashCommandIconCatalog.resolveIcon(m.name, m.icon, m.kind)
            lookup[m.name] = SlashCommandMention(name = m.name, icon = icon, kind = m.kind)
        }

        val matches = tokenRegex.findAll(text).toList()
        if (matches.isEmpty()) return listOf(SlashTextSegment.Text(text))

        val segments = mutableListOf<SlashTextSegment>()
        var lastLocation = 0

        for (match in matches) {
            val wsGroup = match.groups[1]!!
            val nameGroup = match.groups[2]!!
            val cmdName = nameGroup.value
            val slashLocation = wsGroup.range.first + wsGroup.value.length

            val resolved: SlashCommandMention? = lookup[cmdName] ?: run {
                if (slashLocation == 0 &&
                    cmdName.lowercase() !in unixRootDirs &&
                    cmdName.firstOrNull()?.isLetter() == true
                ) {
                    SlashCommandMention(name = cmdName, icon = "code_blocks", kind = "skill")
                } else null
            }

            if (resolved == null) continue

            if (slashLocation > lastLocation) {
                val prefix = text.substring(lastLocation, slashLocation)
                if (prefix.isNotEmpty()) {
                    segments.add(SlashTextSegment.Text(prefix))
                }
            }
            segments.add(SlashTextSegment.Slash(resolved))

            var tokenEnd = nameGroup.range.last + 1
            if (tokenEnd < text.length) {
                val after = text.substring(tokenEnd)
                val mTrailing = Regex("""^[ \t]+(?=\r?\n)""").find(after)
                if (mTrailing != null) {
                    tokenEnd += mTrailing.value.length
                }
            }
            lastLocation = tokenEnd
        }

        if (lastLocation == 0) {
            return listOf(SlashTextSegment.Text(text))
        }
        if (lastLocation < text.length) {
            val tail = text.substring(lastLocation)
            if (tail.isNotEmpty()) {
                segments.add(SlashTextSegment.Text(tail))
            }
        }
        return segments
    }
}

// region 全局后台任务中心 (ListBackgroundTasks)

@Serializable
data class GlobalBackgroundTaskItem(
    val conversationId: String = "",
    val conversationTitle: String = "未命名会话",
    val workspaceName: String? = null,
    val taskId: String = "",
    val stepIndex: Int = 0,
    val toolName: String? = null,
    val toolSummary: String? = null,
    val commandLine: String = "",
    val startTime: String? = null,
    val logUri: String? = null,
    val isDaemon: Boolean = false
) {
    val uniqueId: String get() = "$conversationId:$taskId:$stepIndex"

    val displayTitle: String
        get() {
            val s = toolSummary?.trim().orEmpty()
            if (s.isNotEmpty()) return s
            if (toolName == "schedule") return "定时任务"
            return if (isDaemon) "常驻守护进程" else "后台终端命令"
        }

    val elapsedText: String?
        get() {
            val raw = startTime?.trim().orEmpty()
            if (raw.isEmpty()) return null
            val epochMs = runCatching { java.time.Instant.parse(raw).toEpochMilli() }.getOrNull() ?: return null
            val sec = ((System.currentTimeMillis() - epochMs) / 1000L).coerceAtLeast(0L)
            if (sec < 60) return "${sec}s"
            val min = sec / 60
            if (min < 60) return "${min}m ${sec % 60}s"
            val hr = min / 60
            return "${hr}h ${min % 60}m"
        }
}

@Serializable
data class GlobalBackgroundTasksResponse(
    val tasks: List<GlobalBackgroundTaskItem> = emptyList()
)

@Serializable
data class BackgroundTaskLogResponse(
    val taskId: String = "",
    val logUri: String = "",
    val content: String = "",
    val truncated: Boolean = false,
    val sizeBytes: Long = 0L
)

// endregion


// endregion
