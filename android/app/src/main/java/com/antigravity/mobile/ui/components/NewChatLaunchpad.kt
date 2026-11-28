package com.antigravity.mobile.ui.components

import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.ChatBubble
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Explore
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material.icons.filled.PushPin
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.LaunchpadShortcutItem
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch
import java.util.concurrent.ConcurrentHashMap
import kotlin.random.Random

/** 本地轻量斜杠调用频次记录（作为网关全端历史统计的即时叠加与离线兜底）。 */
object SlashUsageLocalStore {
    private const val PREF_NAME = "mgy_local_slash_usage_v1"

    fun counts(context: Context): Map<String, Int> {
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        val out = HashMap<String, Int>()
        for ((k, v) in prefs.all) {
            if (v is Int && v > 0) out[k] = v
        }
        return out
    }

    fun record(context: Context, name: String) {
        val clean = name.trim().trimStart('/').lowercase()
        if (clean.isEmpty()) return
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        val next = prefs.getInt(clean, 0) + 1
        prefs.edit().putInt(clean, next).apply()
    }
}

/** 启动台专属 24×24 矢量 SVG 图标目录（与 iOS / Web 端共享同一套路径与非洋红克制色系）。 */
internal object LaunchpadSvgCatalog {
    private val cache = ConcurrentHashMap<String, Path>()

    val paths: Map<String, String> = mapOf(
        // 9 大系统核心斜杠命令
        "slash_plan" to "M 8 4.5 L 16 4.5 C 17.1 4.5 18 5.4 18 6.5 L 18 19 C 18 20.1 17.1 21 16 21 L 8 21 C 6.9 21 6 20.1 6 19 L 6 6.5 C 6 5.4 6.9 4.5 8 4.5 Z M 9.5 3 L 14.5 3 L 14.5 6 L 9.5 6 Z M 9 11 L 10.6 12.6 L 13.5 9.7 M 9 16 L 15 16",
        "slash_grill" to "M 4 6.5 C 4 5.1 5.1 4 6.5 4 L 14.5 4 C 15.9 4 17 5.1 17 6.5 L 17 11.5 C 17 12.9 15.9 14 14.5 14 L 9 14 L 5.5 16.8 L 5.5 14 C 4.6 13.5 4 12.6 4 11.5 Z M 17 9.5 L 18 9.5 C 19.1 9.5 20 10.4 20 11.5 L 20 16 C 20 17.1 19.1 18 18 18 L 17.5 18 L 17.5 20.5 L 14.3 18 L 11 18",
        "slash_boost" to "M 13.2 2.5 L 5.2 13 L 11.6 13 L 10.4 21.5 L 18.8 11 L 12.4 11 Z",
        "slash_goal" to "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 12 7.5 C 14.5 7.5 16.5 9.5 16.5 12 C 16.5 14.5 14.5 16.5 12 16.5 C 9.5 16.5 7.5 14.5 7.5 12 C 7.5 9.5 9.5 7.5 12 7.5 Z M 12 10.8 C 12.7 10.8 13.2 11.3 13.2 12 C 13.2 12.7 12.7 13.2 12 13.2 C 11.3 13.2 10.8 12.7 10.8 12 C 10.8 11.3 11.3 10.8 12 10.8 Z",
        "slash_teamwork" to "M 12 3.5 C 13.6 3.5 14.8 4.7 14.8 6.3 C 14.8 7.9 13.6 9.1 12 9.1 C 10.4 9.1 9.2 7.9 9.2 6.3 C 9.2 4.7 10.4 3.5 12 3.5 Z M 5.8 15 C 7.2 15 8.3 16.1 8.3 17.5 C 8.3 18.9 7.2 20 5.8 20 C 4.4 20 3.3 18.9 3.3 17.5 C 3.3 16.1 4.4 15 5.8 15 Z M 18.2 15 C 19.6 15 20.7 16.1 20.7 17.5 C 20.7 18.9 19.6 20 18.2 20 C 16.8 20 15.7 18.9 15.7 17.5 C 15.7 16.1 16.8 15 18.2 15 Z M 10.4 8.8 L 7.2 15.2 M 13.6 8.8 L 16.8 15.2 M 8.3 17.5 L 15.7 17.5",
        "slash_swarm" to "M 12 3 L 16.2 5.4 L 16.2 10.2 L 12 12.6 L 7.8 10.2 L 7.8 5.4 Z M 7.8 10.2 L 12 12.6 L 12 17.4 L 7.8 19.8 L 3.6 17.4 L 3.6 12.6 Z M 16.2 10.2 L 20.4 12.6 L 20.4 17.4 L 16.2 19.8 L 12 17.4 L 12 12.6 Z",
        "slash_workflow" to "M 4 5.5 C 4 4.7 4.7 4 5.5 4 L 9.5 4 C 10.3 4 11 4.7 11 5.5 L 11 8.5 C 11 9.3 10.3 10 9.5 10 L 5.5 10 C 4.7 10 4 9.3 4 8.5 Z M 13 15.5 C 13 14.7 13.7 14 14.5 14 L 18.5 14 C 19.3 14 20 14.7 20 15.5 L 20 18.5 C 20 19.3 19.3 20 18.5 20 L 14.5 20 C 13.7 20 13 19.3 13 18.5 Z M 11 7 L 15.5 7 C 16.3 7 16.5 7.5 16.5 8.5 L 16.5 14 M 7.5 10 L 7.5 15.5 C 7.5 16.5 8.2 17 9 17 L 13 17",
        "slash_learn" to "M 9 17.5 L 15 17.5 M 9.8 20.5 L 14.2 20.5 M 9.5 14.5 C 7.2 13.2 6 11 6 8.8 C 6 5.5 8.7 3 12 3 C 15.3 3 18 5.5 18 8.8 C 18 11 16.8 13.2 14.5 14.5 L 14.5 15.5 L 9.5 15.5 Z",
        "slash_automation" to "M 12 4 C 16.4 4 20 7.6 20 12 C 20 16.4 16.4 20 12 20 C 7.6 20 4 16.4 4 12 C 4 9.4 5.2 7.1 7.2 5.6 M 4.2 5.2 L 7.6 5.2 L 7.6 8.6 M 12 7.5 L 12 12 L 15.2 13.8",
        // 技能专属图标：模块化拼图徽章（替代旧 </> 代码括号）
        "slash_skill" to "M 6.2 7.2 L 9.2 7.2 C 7.6 3.4 14.0 3.4 12.4 7.2 L 15.4 7.2 C 16.2 7.2 16.8 7.8 16.8 8.6 L 16.8 11.6 C 20.6 10.0 20.6 16.4 16.8 14.8 L 16.8 17.8 C 16.8 18.6 16.2 19.2 15.4 19.2 L 6.2 19.2 C 5.4 19.2 4.8 18.6 4.8 17.8 L 4.8 8.6 C 4.8 7.8 5.4 7.2 6.2 7.2 Z",
        "custom_pin" to "M 9 4 L 15 4 L 14.2 10 L 17.2 12.8 L 17.2 14.5 L 6.8 14.5 L 6.8 12.8 L 9.8 10 Z M 12 14.5 L 12 20.5",
        "shuffle_refresh" to "M 4 7.5 L 8.2 7.5 C 10.2 7.5 11.4 9.2 12.4 11.2 L 13.2 12.8 C 14.2 14.8 15.4 16.5 17.4 16.5 L 20 16.5 M 17.5 14 L 20 16.5 L 17.5 19 M 4 16.5 L 8.2 16.5 C 9.6 16.5 10.6 15.6 11.4 14.3 M 13.8 9.7 C 14.6 8.4 15.6 7.5 17.4 7.5 L 20 7.5 M 17.5 5 L 20 7.5 L 17.5 10",

        // 8 项纯对话场景指令
        "prompt_fact_check" to "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 3.8 12 L 20.2 12 M 12 3.5 C 14.4 6 15.6 8.8 15.6 12 C 15.6 15.2 14.4 18 12 20.5 C 9.6 18 8.4 15.2 8.4 12 C 8.4 8.8 9.6 6 12 3.5 Z",
        "prompt_draft" to "M 15.8 4.2 L 19.8 8.2 L 9.2 18.8 L 4.5 19.5 L 5.2 14.8 Z M 13.4 6.6 L 17.4 10.6 M 13.5 19.5 L 20 19.5",
        "prompt_compare" to "M 12 4 L 12 19.5 M 8 19.5 L 16 19.5 M 5 7.5 L 19 7.5 M 6.2 7.5 L 3.8 13.2 C 4.5 14.4 7.9 14.4 8.6 13.2 Z M 17.8 7.5 L 15.4 13.2 C 16.1 14.4 19.5 14.4 20.2 13.2 Z",
        "prompt_digest" to "M 5.5 3.5 L 14.5 3.5 L 18.5 7.5 L 18.5 20.5 L 5.5 20.5 Z M 14.5 3.5 L 14.5 7.5 L 18.5 7.5 M 8.5 11.5 L 15.5 11.5 M 8.5 15 L 15.5 15 M 8.5 18 L 12.2 18",
        "prompt_research" to "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 15.6 8.4 L 13.6 13.6 L 8.4 15.6 L 10.4 10.4 Z",
        "prompt_brainstorm" to "M 4.5 6.5 L 6.3 8.3 L 9.5 5.1 M 12 7 L 19.5 7 M 4.5 12 L 6.3 13.8 L 9.5 10.6 M 12 12.5 L 19.5 12.5 M 4.5 17.5 L 6.3 19.3 L 9.5 16.1 M 12 18 L 17 18",
        "prompt_translate" to "M 4 6.5 L 11.5 6.5 M 7.8 4.2 L 7.8 6.5 M 9.8 6.5 C 9.2 9.8 7.2 12.4 4.5 14 M 6.2 9.2 C 7.5 11 9.3 12.6 11.2 13.6 M 13.2 19.5 L 16.4 11 L 19.6 19.5 M 14.3 16.8 L 18.5 16.8",
        "prompt_notes" to "M 4.5 5.5 L 19.5 5.5 L 19.5 9.5 L 4.5 9.5 Z M 6.5 9.5 L 6.5 18.5 C 6.5 19.3 7.2 20 8 20 L 16 20 C 16.8 20 17.5 19.3 17.5 18.5 L 17.5 9.5 M 9.5 13.5 L 14.5 13.5 M 9.5 16.5 L 13 16.5",

        // 8 项代码工作区场景指令
        "prompt_git_diff" to "M 10.5 4.2 C 14 4.2 16.8 7 16.8 10.5 C 16.8 14 14 16.8 10.5 16.8 C 7 16.8 4.2 14 4.2 10.5 C 4.2 7 7 4.2 10.5 4.2 Z M 15.2 15.2 L 20 20 M 8.3 10.5 L 12.7 10.5 M 10.5 8.3 L 10.5 12.7",
        "prompt_arch" to "M 12 3.8 L 20 8 L 12 12.2 L 4 8 Z M 4 12.2 L 12 16.4 L 20 12.2 M 4 16.2 L 12 20.4 L 20 16.2",
        "prompt_bug" to "M 9 7.5 L 9 6.5 C 9 4.8 10.3 3.5 12 3.5 C 13.7 3.5 15 4.8 15 6.5 L 15 7.5 M 8.5 7.5 L 15.5 7.5 C 16.9 7.5 18 8.6 18 10 L 18 14 C 18 17.3 15.3 20 12 20 C 8.7 20 6 17.3 6 14 L 6 10 C 6 8.6 7.1 7.5 8.5 7.5 Z M 12 10.8 L 12 20 M 6 10.5 L 3 9 M 6 14 L 2.8 14 M 18 10.5 L 21 9 M 18 14 L 21.2 14",
        "prompt_refactor" to "M 4.5 19.5 L 15.5 8.5 M 14.2 6.8 L 17.2 9.8 M 12 3.2 L 12 5.5 M 18.5 12 L 20.8 12 M 17.5 4.5 L 19 3 M 18.5 6.5 L 19.8 5.2 M 7 5 L 7.7 7.1 L 9.8 7.8 L 7.7 8.5 L 7 10.6 L 6.3 8.5 L 4.2 7.8 L 6.3 7.1 Z",
        "prompt_tests" to "M 9.2 3.5 L 14.8 3.5 M 10.2 3.5 L 10.2 9.2 L 5.4 17.8 C 4.8 18.9 5.6 20.2 6.8 20.2 L 17.2 20.2 C 18.4 20.2 19.2 18.9 18.6 17.8 L 13.8 9.2 L 13.8 3.5 M 7.6 15.2 L 16.4 15.2",
        "prompt_git_log" to "M 7 4 C 8.4 4 9.5 5.1 9.5 6.5 C 9.5 7.9 8.4 9 7 9 C 5.6 9 4.5 7.9 4.5 6.5 C 4.5 5.1 5.6 4 7 4 Z M 7 15 C 8.4 15 9.5 16.1 9.5 17.5 C 9.5 18.9 8.4 20 7 20 C 5.6 20 4.5 18.9 4.5 17.5 C 4.5 16.1 5.6 15 7 15 Z M 17.5 7 C 18.9 7 20 8.1 20 9.5 C 20 10.9 18.9 12 17.5 12 C 16.1 12 15 10.9 15 9.5 C 15 8.1 16.1 7 17.5 7 Z M 7 9 L 7 15 M 17.5 12 C 17.5 15.2 14.2 17.5 9.5 17.5",
        "prompt_security" to "M 12 3 L 19 6 L 19 11.8 C 19 16.4 15.9 19.7 12 21 C 8.1 19.7 5 16.4 5 11.8 L 5 6 Z M 9.2 12.2 L 11.2 14.2 L 15.2 10",
        "prompt_changelog" to "M 4.5 12.4 L 12.4 4.5 L 19.5 4.5 L 19.5 11.6 L 11.6 19.5 Z M 15.2 8.2 C 15.8 8.2 16.3 8.7 16.3 9.3 C 16.3 9.9 15.8 10.4 15.2 10.4 C 14.6 10.4 14.1 9.9 14.1 9.3 C 14.1 8.7 14.6 8.2 15.2 8.2 Z"
    )

    fun getPath(key: String): Path {
        if (key.startsWith("wf:")) {
            return WorkflowStyleCatalog.getPath(key.removePrefix("wf:"))
        }
        val resolved = if (paths.containsKey(key)) key else "slash_skill"
        return cache.computeIfAbsent(resolved) { k ->
            val d = paths[k] ?: paths["slash_skill"]!!
            PathParser().parsePathString(d).toPath()
        }
    }
}

@Composable
internal fun LaunchpadSvgIcon(
    svgKey: String,
    tint: Color,
    size: Dp = 14.dp,
    modifier: Modifier = Modifier
) {
    val path = remember(svgKey) { LaunchpadSvgCatalog.getPath(svgKey) }
    Canvas(modifier = modifier.size(size)) {
        val s = this.size.minDimension / 24f
        scale(scaleX = s, scaleY = s, pivot = Offset.Zero) {
            drawPath(
                path = path,
                color = tint,
                style = Stroke(
                    width = 1.95f,
                    cap = StrokeCap.Round,
                    join = StrokeJoin.Round
                )
            )
        }
    }
}

private sealed class LaunchCardAction {
    data class FillPrompt(val prompt: String) : LaunchCardAction()
    data class SelectSlash(val option: SlashCommandOption) : LaunchCardAction()
}

private data class LaunchCardItem(
    val id: String,
    val svgKey: String,
    val tint: Color,
    val title: String,
    val subtitle: String,
    val action: LaunchCardAction,
    val shortcutItem: LaunchpadShortcutItem? = null
)

internal data class LaunchpadCoreSlashSpec(
    val name: String,
    val svgKey: String,
    val tintName: String,
    val tint: Color,
    val titleZh: String,
    val titleEn: String,
    val titleJa: String,
    val subZh: String,
    val subEn: String,
    val subJa: String,
    val fallbackIcon: String
)

internal data class LaunchpadPromptSpec(
    val id: String,
    val svgKey: String,
    val tintName: String,
    val tint: Color,
    val titleZh: String,
    val titleEn: String,
    val titleJa: String,
    val subZh: String,
    val subEn: String,
    val subJa: String,
    val promptZh: String,
    val promptEn: String,
    val promptJa: String,
    val requiresDirtyFiles: Boolean = false
)

// 严禁洋红：统一 Indigo / Blue / Teal / Cyan / Amber / Emerald / Coral
internal val LP_INDIGO = Color(0xFF6366F1)
internal val LP_BLUE = Color(0xFF3B82F6)
internal val LP_TEAL = Color(0xFF14B8A6)
internal val LP_CYAN = Color(0xFF06B6D4)
internal val LP_AMBER = Color(0xFFF59E0B)
internal val LP_EMERALD = Color(0xFF22C55E)
internal val LP_CORAL = Color(0xFFEF4444)

internal val LAUNCHPAD_MID_CHAT_EXCLUDED = setOf("compact", "btw", "schedule", "browser")

internal val LAUNCHPAD_CORE_SLASH_POOL = listOf(
    LaunchpadCoreSlashSpec("plan", "slash_plan", "blue", LP_BLUE, "/plan 方案规划", "/plan Architect", "/plan 実装計画", "先出步骤与风险，确认再动手", "Align steps & risks before editing", "手順とリスクを固めてから着手", "ballot"),
    LaunchpadCoreSlashSpec("grill-me", "slash_grill", "indigo", LP_INDIGO, "/grill-me 追问对齐", "/grill-me Clarify", "/grill-me 深掘り", "逐轮反向提问，拆清模糊想法", "Interactive Q&A to sharpen ideas", "逆質問で曖昧な要件を明確化", "forum"),
    LaunchpadCoreSlashSpec("boost", "slash_boost", "amber", LP_AMBER, "/boost 深度攻坚", "/boost Deep Think", "/boost 徹底推論", "多视角深度推理与严格验证", "Deep multi-angle reasoning & checks", "多角的な深い推論と厳格検証", "bolt"),
    LaunchpadCoreSlashSpec("goal", "slash_goal", "emerald", LP_EMERALD, "/goal 自律闭环", "/goal Autonomous", "/goal 自律達成", "持续迭代自检，达标才收工", "Iterate & self-verify until done", "目標達成まで自律検証を継続", "timer"),
    LaunchpadCoreSlashSpec("teamwork", "slash_teamwork", "cyan", LP_CYAN, "/teamwork 团队协作", "/teamwork Agents", "/teamwork 連携", "多角色子 Agent 分工推进大任务", "Coordinate specialist subagents", "複数エージェントで分担推進", "graph_2"),
    LaunchpadCoreSlashSpec("swarm", "slash_swarm", "amber", LP_AMBER, "/swarm 蜂群并发", "/swarm Parallel", "/swarm 並列群", "共享消息板多 Agent 协同攻坚", "Shared board for parallel workers", "共有ボードで並列エージェント実行", "swarm_bee"),
    LaunchpadCoreSlashSpec("workflow", "slash_workflow", "teal", LP_TEAL, "/workflow 编排流程", "/workflow Pipeline", "/workflow 編成", "并行或流水线批量执行多阶段任务", "Orchestrate staged agent pipelines", "多段階ワークフローを一括編成", "graph_2"),
    LaunchpadCoreSlashSpec("learn", "slash_learn", "amber", LP_AMBER, "/learn 沉淀经验", "/learn Persist", "/learn 知見定着", "将踩坑与规范固化，后续自动生效", "Save lessons & rules for future chats", "学びやルールを今後の会話に定着", "lightbulb_2"),
    LaunchpadCoreSlashSpec("automation", "slash_automation", "blue", LP_BLUE, "/automation 定时任务", "/automation Cron", "/automation 定期", "创建周期巡检或定时提醒任务", "Create recurring background schedules", "定期巡回やリマインドを自動化", "schedule")
)

internal val LAUNCHPAD_PURE_CHAT_PROMPT_POOL = listOf(
    LaunchpadPromptSpec("pure_fact_check", "prompt_fact_check", "blue", LP_BLUE, "联网核查资讯", "Web Fact-Check", "最新情報を検証", "交叉验证最新进展与来源", "Verify latest updates & sources", "信頼できる情報源で事実確認", "请帮我联网核查以下消息的最新进展，并附上权威来源交叉印证：", "Please verify the latest updates and cross-check reliable sources for: ", "以下の件の最新状況と信頼できる情報源を突き合わせて確認してください："),
    LaunchpadPromptSpec("pure_draft", "prompt_draft", "teal", LP_TEAL, "起草与润色", "Draft & Polish", "下書き・推敲", "文章 / 公告 / 邮件 / 提纲", "Articles, notes, emails & outlines", "記事・お知らせ・メール・構成案", "帮我起草并润色一段清晰自然的内容，主题是：", "Help me draft and polish a clear, natural piece about: ", "次のテーマについて自然で分かりやすい文章を作成・推敲してください："),
    LaunchpadPromptSpec("pure_compare", "prompt_compare", "amber", LP_AMBER, "多维方案对比", "Compare Options", "多角的に比較", "权衡利弊、成本与风险", "Weigh pros, cons, cost & risk", "利点・コスト・リスクを比較整理", "请从利弊、成本与风险角度对比以下方案，并给出建议：", "Please compare these options across pros, cons, cost, and risk, then recommend one: ", "以下の案をメリット・コスト・リスクの観点で比較し、推奨案を示してください："),
    LaunchpadPromptSpec("pure_digest", "prompt_digest", "indigo", LP_INDIGO, "长文精读提炼", "Distill Long Text", "長文の要点抽出", "抓核心结论与关键数据", "Extract key takeaways & figures", "核心結論と重要データを整理", "请帮我精读下面内容，提炼核心结论、关键数据与潜在漏洞：", "Please read the following closely and extract key conclusions, data, and blind spots: ", "以下の内容を精読し、核心結論・主要データ・見落としを抽出してください："),
    LaunchpadPromptSpec("pure_research", "prompt_research", "cyan", LP_CYAN, "深度专题调研", "Topic Deep Dive", "テーマ徹底調査", "梳理来龙去脉与现状趋势", "Map background, status & trends", "背景・現状・今後の動向を整理", "请帮我做一次系统的专题调研，梳理背景、现状与关键趋势：", "Please run a structured deep dive covering background, current state, and key trends on: ", "次のテーマについて背景・現状・主要トレンドを体系的に調査してください："),
    LaunchpadPromptSpec("pure_brainstorm", "prompt_brainstorm", "emerald", LP_EMERALD, "拆解模糊想法", "Shape Rough Ideas", "アイデア具体化", "把灵感变成可执行清单", "Turn sparks into action checklists", "着想を実行可能な手順へ分解", "我有一个初步想法，请帮我拆解关键问题并整理成行动清单：", "I have a rough idea—help me break down the key questions into an actionable checklist: ", "まだ曖昧なアイデアがあります。論点を分解し実行リストに整理してください："),
    LaunchpadPromptSpec("pure_translate", "prompt_translate", "blue", LP_BLUE, "地道翻译润色", "Natural Translation", "自然な翻訳推敲", "保留原意与语境的自然表达", "Idiomatic phrasing with exact nuance", "ニュアンスを保った自然な表現", "请将下面内容地道地翻译并润色，保持自然专业语气：", "Please translate and polish the following naturally while preserving nuance and tone: ", "以下の内容を元の意図と専門的なトーンを保って自然に翻訳・推敲してください："),
    LaunchpadPromptSpec("pure_notes", "prompt_notes", "teal", LP_TEAL, "零散记录整理", "Organize Notes", "メモを構造化", "转成结构化纪要与待办", "Structure minutes, decisions & TODOs", "議事録・決定事項・TODO化", "请把以下零散记录整理成结构化纪要，列出结论与下一步待办：", "Please turn these rough notes into structured minutes with decisions and next actions: ", "以下の断片メモを構造化し、結論と次のアクション一覧にまとめてください：")
)

internal val LAUNCHPAD_WORKSPACE_PROMPT_POOL = listOf(
    LaunchpadPromptSpec("ws_git_diff", "prompt_git_diff", "amber", LP_AMBER, "审查未提交改动", "Review Dirty Diff", "未コミット変更確認", "检查 git diff 风险与优化", "Inspect git diff risks & fixes", "git diff のリスクと改善確認", "请先检查当前未提交的改动，指出潜在风险并给出优化建议", "Please review the current uncommitted changes, highlight potential risks, and suggest improvements", "現在の未コミット変更を確認し、潜在リスクと改善案を教えてください", requiresDirtyFiles = true),
    LaunchpadPromptSpec("ws_arch", "prompt_arch", "indigo", LP_INDIGO, "梳理项目架构", "Map Architecture", "構成を把握", "快速定位核心模块与入口", "Find core modules & entry points", "主要モジュールと入口を整理", "请先梳理这个工作区的整体架构、核心模块分工与关键入口文件", "Please map out the overall architecture, core modules, and key entry points of this workspace", "このワークスペースの全体構造、主要モジュール、エントリポイントを整理してください"),
    LaunchpadPromptSpec("ws_bug", "prompt_bug", "coral", LP_CORAL, "定位排查 Bug", "Debug Root Cause", "バグ原因調査", "结合代码与日志追查根因", "Trace root cause from code & logs", "コードとログから根本原因特定", "请帮我排查并定位以下问题的根因，先分析再给修复方案：", "Please investigate and find the root cause of this issue before proposing a fix: ", "以下の問題の根本原因を分析し、修正案を提示してください："),
    LaunchpadPromptSpec("ws_refactor", "prompt_refactor", "teal", LP_TEAL, "代码优化重构", "Refactor & Polish", "最適化・改善", "提升可读性、性能与稳定性", "Improve clarity, speed & stability", "可読性・性能・安定性を改善", "请分析当前模块可优化的设计点，给出轻量低侵入的重构方案：", "Please identify improvements in the current module and propose a low-impact refactor: ", "現在のモジュールの改善点を分析し、低侵襲なリファクタ案を提案してください："),
    LaunchpadPromptSpec("ws_tests", "prompt_tests", "emerald", LP_EMERALD, "补全单元测试", "Add Unit Tests", "単体テスト追加", "覆盖核心路径与边界条件", "Cover core flows & edge cases", "主要経路と境界ケースを網羅", "请为以下模块补充单元测试，重点覆盖异常分支与边界情况：", "Please add high-value unit tests focusing on edge cases and error paths for: ", "以下のモジュールに異常系・境界条件を重視した単体テストを追加してください："),
    LaunchpadPromptSpec("ws_git_log", "prompt_git_log", "cyan", LP_CYAN, "速览近期提交", "Summarize Commits", "直近コミット要約", "总结最近变更与影响范围", "Recap recent git log & blast radius", "最近の変更点と影響範囲を把握", "请结合最近的 git log 梳理近期核心改动，并评估影响范围：", "Please summarize recent key commits from git log and assess their impact area: ", "最近の git log から主な変更点を要約し、影響範囲を評価してください："),
    LaunchpadPromptSpec("ws_security", "prompt_security", "amber", LP_AMBER, "安全隐患体检", "Security Checkup", "セキュリティ点検", "排查权限、输入与异常兜底", "Audit auth, inputs & error handling", "権限・入力検証・例外処理を確認", "请对当前工作区关键链路做一次轻量安全与健壮性检查：", "Please perform a lightweight security and resilience audit on the critical paths here: ", "このワークスペースの重要経路について軽量なセキュリティ・堅牢性チェックを行ってください："),
    LaunchpadPromptSpec("ws_changelog", "prompt_changelog", "blue", LP_BLUE, "生成变更说明", "Draft Changelog", "変更履歴を作成", "整理发版日志或 PR 描述", "Write release notes or PR summary", "リリースノートや PR 概要を整形", "请基于当前分支近期改动，整理一份简洁清晰的变更说明：", "Please draft clear, concise release notes or a PR summary from recent branch changes: ", "現在のブランチの直近変更から簡潔な変更履歴・PR説明を作成してください：")
)

internal val LAUNCHPAD_ALL_PROMPT_POOL: List<LaunchpadPromptSpec> =
    LAUNCHPAD_PURE_CHAT_PROMPT_POOL + LAUNCHPAD_WORKSPACE_PROMPT_POOL

internal fun launchpadTintByName(name: String): Color = when (name.lowercase()) {
    "indigo" -> LP_INDIGO
    "blue" -> LP_BLUE
    "teal" -> LP_TEAL
    "cyan" -> LP_CYAN
    "amber", "orange" -> LP_AMBER
    "emerald", "green" -> LP_EMERALD
    "coral", "red" -> LP_CORAL
    else -> LP_INDIGO
}

internal fun launchpadSafeTemplateTint(colorKey: String): Color = when (colorKey.lowercase()) {
    "indigo", "purple" -> LP_INDIGO
    "green", "emerald" -> LP_EMERALD
    "blue" -> LP_BLUE
    "teal" -> Color(0xFF30B0C7)
    "cyan" -> LP_CYAN
    "red", "coral" -> LP_CORAL
    "orange", "amber" -> LP_AMBER
    else -> LP_INDIGO
}

internal fun launchpadSafeTintName(colorKey: String): String = when (colorKey.lowercase()) {
    "indigo", "purple" -> "indigo"
    "green", "emerald" -> "green"
    "blue" -> "blue"
    "teal" -> "teal"
    "cyan" -> "cyan"
    "red", "coral" -> "red"
    "orange", "amber" -> "orange"
    else -> "indigo"
}

private fun t3(lang: L10n.AppLang, zh: String, en: String, ja: String): String = when (lang) {
    L10n.AppLang.EN -> en
    L10n.AppLang.JA -> ja
    L10n.AppLang.ZH -> zh
}

private fun <T> deterministicShuffle(items: List<T>, seed: Int): List<T> {
    if (items.size <= 1) return items
    val list = items.toMutableList()
    var state = (if (seed == 0) 1L else seed.toLong()) * 6364136223846793005L + 1442695040888963407L
    for (i in list.size - 1 downTo 1) {
        state = state * 6364136223846793005L + 1442695040888963407L
        val j = (((state ushr 33) % (i + 1).toLong()).toInt()).coerceIn(0, i)
        if (i != j) {
            val tmp = list[i]
            list[i] = list[j]
            list[j] = tmp
        }
    }
    return list
}

/**
 * 双态新建对话启动台：
 * - 状态一（默认 · 速发聚焦态，键盘展开）：单行工作区/Git 条 + 2×2 用户自定义快捷起手网格（无问候语、无 7 日 Token 彩蛋）
 * - 状态二（全屏 · 探索与配置态，收起键盘）：单行工作区/Git 条 + 自定义 2×2 配置入口 + 2×4 学习区（长按一键固定到 2×2）
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun NewChatLaunchpadView(
    cascadeId: String,
    title: String,
    workspaceName: String,
    isPureChat: Boolean,
    isDraftConversation: Boolean,
    gitBranch: String?,
    uncommittedFileCount: Int,
    workflowTemplates: List<WorkflowTemplateRecord>,
    slashCommands: List<SlashCommandOption>,
    isImeVisible: Boolean,
    isInputFocused: Boolean,
    isAwaitingInitialAutoFocus: Boolean,
    onFillPrompt: (String) -> Unit,
    onSelectSlashCommand: (SlashCommandOption) -> Unit,
    onSwitchWorkspace: () -> Unit,
    onTapDirtyFiles: () -> Unit,
    onDismissKeyboard: () -> Unit,
    onOpenWorkflowTemplates: () -> Unit,
    onPrefetchSlashCommands: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    val config = LocalConfiguration.current
    val currentLangCode by L10n.currentLangFlow.collectAsState()
    val currentLang = remember(currentLangCode) { L10n.currentLang }
    val shortcutState by LaunchpadShortcutsStore.state.collectAsState()

    var shuffleSeed by remember(cascadeId) { mutableIntStateOf(Random.nextInt(1, 999_999)) }
    var wideScreenExploreMode by remember(cascadeId) { mutableStateOf(false) }
    var showConfiguratorSheet by remember { mutableStateOf(false) }
    var pendingPinItem by remember { mutableStateOf<LaunchpadShortcutItem?>(null) }

    LaunchedEffect(cascadeId) {
        LaunchpadShortcutsStore.ensureLoaded(context)
        onPrefetchSlashCommands()
        LaunchpadShortcutsStore.refresh(context)
    }

    val isWideHardwareMode = config.screenWidthDp >= 700 && !isImeVisible
    val isShowingFocusState = if (isWideHardwareMode) {
        !wideScreenExploreMode
    } else {
        isImeVisible || isInputFocused || isAwaitingInitialAutoFocus
    }

    val workspaceDisplayName = remember(workspaceName, title) {
        workspaceName.trim().takeIf { it.isNotEmpty() && it != "Chat" } ?: title
    }

    val gatewayByName = remember(slashCommands) { slashCommands.associateBy { it.name.lowercase() } }
    val coreByName = remember { LAUNCHPAD_CORE_SLASH_POOL.associateBy { it.name } }

    fun makeCoreCard(spec: LaunchpadCoreSlashSpec, prefix: String): LaunchCardItem {
        val localizedTitle = t3(currentLang, spec.titleZh, spec.titleEn, spec.titleJa)
        val localizedSub = t3(currentLang, spec.subZh, spec.subEn, spec.subJa)
        val opt = gatewayByName[spec.name] ?: SlashCommandOption(
            name = spec.name,
            title = localizedTitle,
            description = localizedSub,
            icon = spec.fallbackIcon,
            kind = "system"
        )
        return LaunchCardItem(
            id = "${prefix}_${spec.name}",
            svgKey = spec.svgKey,
            tint = spec.tint,
            title = localizedTitle,
            subtitle = localizedSub,
            action = LaunchCardAction.SelectSlash(opt),
            shortcutItem = LaunchpadShortcutItem("slash", spec.name, localizedTitle, localizedSub, spec.svgKey, spec.tintName)
        )
    }

    fun makeDynamicSlashCard(opt: SlashCommandOption, prefix: String): LaunchCardItem {
        val lower = opt.name.lowercase()
        coreByName[lower]?.let { spec ->
            val localizedTitle = t3(currentLang, spec.titleZh, spec.titleEn, spec.titleJa)
            val localizedSub = t3(currentLang, spec.subZh, spec.subEn, spec.subJa)
            return LaunchCardItem(
                id = "${prefix}_${opt.name}",
                svgKey = spec.svgKey,
                tint = spec.tint,
                title = localizedTitle,
                subtitle = localizedSub,
                action = LaunchCardAction.SelectSlash(opt),
                shortcutItem = LaunchpadShortcutItem("slash", spec.name, localizedTitle, localizedSub, spec.svgKey, spec.tintName)
            )
        }
        val sub = opt.description.trim()
        val kind = if (opt.kind.equals("system", ignoreCase = true)) "slash" else "skill"
        return LaunchCardItem(
            id = "${prefix}_${opt.name}",
            svgKey = "slash_skill",
            tint = LP_TEAL,
            title = "/${opt.name}",
            subtitle = sub.ifEmpty { t3(currentLang, "高频常用技能指令", "Frequently used skill", "よく使うスキルコマンド") },
            action = LaunchCardAction.SelectSlash(opt),
            shortcutItem = LaunchpadShortcutItem(kind, opt.name, "/${opt.name}", sub, "slash_skill", "teal")
        )
    }

    fun makePromptCard(spec: LaunchpadPromptSpec, prefix: String): LaunchCardItem {
        val localizedTitle = if (spec.id == "ws_git_diff" && uncommittedFileCount > 0) {
            t3(currentLang, "审查 $uncommittedFileCount 个改动", "Review $uncommittedFileCount changes", "変更 $uncommittedFileCount 件を確認")
        } else {
            t3(currentLang, spec.titleZh, spec.titleEn, spec.titleJa)
        }
        val localizedSub = t3(currentLang, spec.subZh, spec.subEn, spec.subJa)
        return LaunchCardItem(
            id = "${prefix}_${spec.id}",
            svgKey = spec.svgKey,
            tint = spec.tint,
            title = localizedTitle,
            subtitle = localizedSub,
            action = LaunchCardAction.FillPrompt(t3(currentLang, spec.promptZh, spec.promptEn, spec.promptJa)),
            shortcutItem = LaunchpadShortcutItem(
                "prompt", spec.id,
                t3(currentLang, spec.titleZh, spec.titleEn, spec.titleJa),
                localizedSub, spec.svgKey, spec.tintName
            )
        )
    }

    // 状态一：2×2 快捷起手（未自定义时不套用用户 Workflow：上排 2 随机核心斜杠 + 下排 2 按模式随机模板）
    val quickFocusCards = remember(
        shortcutState,
        isPureChat,
        uncommittedFileCount,
        workflowTemplates,
        slashCommands,
        shuffleSeed,
        currentLang
    ) {
        val availableCore = if (slashCommands.isNotEmpty()) {
            val hit = LAUNCHPAD_CORE_SLASH_POOL.filter { gatewayByName.containsKey(it.name) }
            if (hit.size >= 2) hit else LAUNCHPAD_CORE_SLASH_POOL
        } else {
            LAUNCHPAD_CORE_SLASH_POOL
        }

        val cards = mutableListOf<LaunchCardItem>()
        val usedSlashNames = mutableSetOf<String>()
        val usedPromptIds = mutableSetOf<String>()

        if (shortcutState.customized) {
            for (raw in shortcutState.items.take(4)) {
                val item = raw.normalized()
                when (item.kind) {
                    "workflow" -> {
                        val tpl = workflowTemplates.firstOrNull { it.id == item.ref } ?: continue
                        val wfIconKey = "wf:${WorkflowStyleCatalog.normalizeIconKey(tpl.icon)}"
                        cards.add(
                            LaunchCardItem(
                                id = "pin_wf_${tpl.id}",
                                svgKey = wfIconKey,
                                tint = launchpadSafeTemplateTint(tpl.color),
                                title = tpl.title,
                                subtitle = tpl.subtitle.ifBlank { t3(currentLang, "自定义快捷模板", "Custom quick template", "カスタムショートカット") },
                                action = LaunchCardAction.FillPrompt(tpl.templatePrompt),
                                shortcutItem = item
                            )
                        )
                    }
                    "slash", "skill" -> {
                        val lower = item.ref.lowercase()
                        usedSlashNames.add(lower)
                        val core = coreByName[lower]
                        val live = gatewayByName[lower]
                        when {
                            core != null -> cards.add(makeCoreCard(core, "pin_slash"))
                            live != null -> cards.add(makeDynamicSlashCard(live, "pin_slash"))
                            else -> {
                                val fallbackOpt = SlashCommandOption(
                                    name = lower,
                                    title = item.title ?: "/$lower",
                                    description = item.subtitle.orEmpty(),
                                    icon = "terminal",
                                    kind = if (item.kind == "slash") "system" else "skill"
                                )
                                cards.add(
                                    LaunchCardItem(
                                        id = "pin_slash_$lower",
                                        svgKey = item.svgKey ?: "slash_skill",
                                        tint = launchpadTintByName(item.tint ?: "teal"),
                                        title = item.title ?: "/$lower",
                                        subtitle = item.subtitle.orEmpty().ifBlank { t3(currentLang, "自定义斜杠指令", "Custom slash", "カスタムコマンド") },
                                        action = LaunchCardAction.SelectSlash(fallbackOpt),
                                        shortcutItem = item
                                    )
                                )
                            }
                        }
                    }
                    "prompt" -> {
                        val spec = LAUNCHPAD_ALL_PROMPT_POOL.firstOrNull { it.id == item.ref } ?: continue
                        usedPromptIds.add(spec.id)
                        cards.add(makePromptCard(spec, "pin_prompt"))
                    }
                }
            }
        }

        // 上排补齐（最多补至 2 格）：基于纯对话或代码工作区的随机提示词模板
        if (cards.size < 2) {
            val modePool = if (isPureChat) {
                LAUNCHPAD_PURE_CHAT_PROMPT_POOL
            } else {
                LAUNCHPAD_WORKSPACE_PROMPT_POOL.filter { !it.requiresDirtyFiles || uncommittedFileCount > 0 }
            }
            val filtered = modePool.filter { it.id !in usedPromptIds }
            for (spec in deterministicShuffle(filtered, shuffleSeed xor 0x7E4B21)) {
                if (cards.size >= 2) break
                if (usedPromptIds.add(spec.id)) {
                    cards.add(makePromptCard(spec, "auto_prompt"))
                }
            }
        }

        // 下排补齐（补满至 4 格）：随机系统核心斜杠（用户自选 2 项时，下排正好为 2 个随机斜杠）
        if (cards.size < 4) {
            val slashPoolRaw = availableCore.filter { it.name !in usedSlashNames }
            val slashPool = if (slashPoolRaw.size >= 2) slashPoolRaw else LAUNCHPAD_CORE_SLASH_POOL.filter { it.name !in usedSlashNames }
            for (spec in deterministicShuffle(slashPool, shuffleSeed xor 0x13579B)) {
                if (cards.size >= 4) break
                if (usedSlashNames.add(spec.name)) {
                    cards.add(makeCoreCard(spec, "auto_slash"))
                }
            }
        }
        cards.take(4)
    }

    // 状态二上半 2×2（4 张随机核心斜杠）
    val exploreSlashCards = remember(slashCommands, shuffleSeed, currentLang) {
        val availableCore = if (slashCommands.isNotEmpty()) {
            val hit = LAUNCHPAD_CORE_SLASH_POOL.filter { gatewayByName.containsKey(it.name) }
            if (hit.size >= 4) hit else LAUNCHPAD_CORE_SLASH_POOL
        } else {
            LAUNCHPAD_CORE_SLASH_POOL
        }
        deterministicShuffle(availableCore, shuffleSeed xor 0x5A3C91)
            .take(4)
            .map { makeCoreCard(it, "exp_slash") }
    }

    // 状态二下半 2×2（4 张按模式切换的提示词池）
    val explorePromptCards = remember(isPureChat, uncommittedFileCount, shuffleSeed, currentLang) {
        val pool = if (isPureChat) {
            LAUNCHPAD_PURE_CHAT_PROMPT_POOL
        } else {
            LAUNCHPAD_WORKSPACE_PROMPT_POOL.filter { !it.requiresDirtyFiles || uncommittedFileCount > 0 }
        }
        deterministicShuffle(pool, shuffleSeed xor 0x2F8B4D)
            .take(4)
            .map { makePromptCard(it, "exp_prompt") }
    }

    fun triggerCard(card: LaunchCardItem) {
        haptic.light()
        when (val act = card.action) {
            is LaunchCardAction.FillPrompt -> onFillPrompt(act.prompt)
            is LaunchCardAction.SelectSlash -> {
                SlashUsageLocalStore.record(context, act.option.name)
                onSelectSlashCommand(act.option)
            }
        }
    }

    fun handleLongPressPin(item: LaunchpadShortcutItem) {
        scope.launch {
            if (LaunchpadShortcutsStore.isPinned(item.kind, item.ref)) {
                haptic.light()
                LaunchpadShortcutsStore.unpin(context, item.kind, item.ref)
                Toast.makeText(context, t3(currentLang, "已从 2×2 移除", "Removed from 2×2", "2×2 から削除しました"), Toast.LENGTH_SHORT).show()
                return@launch
            }
            when (LaunchpadShortcutsStore.pin(context, item)) {
                LaunchpadPinResult.ADDED -> {
                    haptic.medium()
                    val count = LaunchpadShortcutsStore.state.value.items.size
                    Toast.makeText(context, t3(currentLang, "已固定到新建页 2×2 ($count/4)", "Pinned to 2×2 ($count/4)", "2×2 に固定しました ($count/4)"), Toast.LENGTH_SHORT).show()
                }
                LaunchpadPinResult.ALREADY_PINNED -> Unit
                LaunchpadPinResult.FULL -> {
                    haptic.medium()
                    pendingPinItem = item
                    showConfiguratorSheet = true
                }
            }
        }
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null,
                onClick = {
                    if (!isWideHardwareMode && (isImeVisible || isInputFocused)) {
                        onDismissKeyboard()
                    }
                }
            ),
        contentAlignment = Alignment.TopCenter
    ) {
        Column(
            modifier = Modifier
                .widthIn(max = 540.dp)
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 18.dp, vertical = 16.dp)
        ) {
            // 顶部工作区选择栏：固定锚定偏上方位置与恒定高度，不随键盘折叠/展开移动或产生动效
            LaunchpadWorkspacePulseBar(
                isPureChat = isPureChat,
                isDraftConversation = isDraftConversation,
                workspaceDisplayName = workspaceDisplayName,
                gitBranch = gitBranch,
                uncommittedFileCount = uncommittedFileCount,
                currentLang = currentLang,
                onSwitchWorkspace = onSwitchWorkspace,
                onTapDirtyFiles = onTapDirtyFiles
            )

            Spacer(modifier = Modifier.height(22.dp))

            // 下方双态内容区：直接切换，无闪烁动效
            if (isShowingFocusState) {
                    // 状态一 · 速发聚焦态（舒展防误触间距）
                    Column(
                        modifier = Modifier.fillMaxWidth(),
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                text = t3(currentLang, "快捷起手", "Quick Start", "クイック起動"),
                                color = colors.textSecondary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.padding(start = 2.dp)
                            )
                            Spacer(modifier = Modifier.weight(1f))
                            if (isWideHardwareMode) {
                                WideModeTogglePill(
                                    isExplore = false,
                                    currentLang = currentLang,
                                    onToggle = { wideScreenExploreMode = true }
                                )
                            } else {
                                Row(
                                    modifier = Modifier
                                        .clip(CircleShape)
                                        .background(colors.surface.copy(alpha = 0.72f))
                                        .clickable {
                                            haptic.light()
                                            onDismissKeyboard()
                                        }
                                        .padding(horizontal = 10.dp, vertical = 6.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(3.dp)
                                ) {
                                    Text(
                                        text = t3(currentLang, "探索与配置", "Explore & Edit", "探索と設定"),
                                        color = colors.textSecondary,
                                        fontSize = 12.5.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                    Icon(
                                        imageVector = Icons.Default.ChevronRight,
                                        contentDescription = null,
                                        tint = colors.textSecondary,
                                        modifier = Modifier.size(14.dp)
                                    )
                                }
                            }
                        }

                        LaunchpadTwoColumnGrid(
                            cards = quickFocusCards,
                            allowLongPressPin = false,
                            pinnedIds = emptySet(),
                            onTap = ::triggerCard,
                            onLongPressPin = ::handleLongPressPin
                        )
                    }
                } else {
                    // 状态二 · 全屏探索与配置态
                    val pinnedIds = remember(shortcutState) { shortcutState.items.map { it.stableId() }.toSet() }
                    Column(
                        modifier = Modifier.fillMaxWidth(),
                        verticalArrangement = Arrangement.spacedBy(18.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            Row(
                                modifier = Modifier
                                    .weight(1f)
                                    .clip(RoundedCornerShape(14.dp))
                                    .background(colors.surface)
                                    .border(0.9.dp, colors.accentIndigo.copy(alpha = 0.22f), RoundedCornerShape(14.dp))
                                    .clickable {
                                        haptic.light()
                                        pendingPinItem = null
                                        showConfiguratorSheet = true
                                    }
                                    .padding(horizontal = 13.dp, vertical = 11.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(9.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(26.dp)
                                        .clip(RoundedCornerShape(7.dp))
                                        .background(colors.accentIndigo.copy(alpha = 0.14f)),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.GridView,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(14.dp)
                                    )
                                }
                                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.5.dp)) {
                                    Text(
                                        text = t3(currentLang, "自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"),
                                        color = colors.textPrimary,
                                        fontSize = 13.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 1
                                    )
                                    Text(
                                        text = if (shortcutState.customized) {
                                            t3(
                                                currentLang,
                                                "已固定 ${shortcutState.items.size}/4 项 · 长按下方卡片亦可一键添加",
                                                "${shortcutState.items.size}/4 pinned · Long-press cards below to pin",
                                                "${shortcutState.items.size}/4 固定済み · カード長押しで追加"
                                            )
                                        } else {
                                            t3(
                                                currentLang,
                                                "当前为随机推荐 · 长按下方卡片可一键固定",
                                                "Random discovery · Long-press any card below to pin",
                                                "ランダム表示中 · カード長押しで固定可能"
                                            )
                                        },
                                        color = colors.textSecondary,
                                        fontSize = 11.sp,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                                Icon(
                                    imageVector = Icons.Default.ChevronRight,
                                    contentDescription = null,
                                    tint = colors.textSecondary.copy(alpha = 0.7f),
                                    modifier = Modifier.size(16.dp)
                                )
                            }

                            if (isWideHardwareMode) {
                                WideModeTogglePill(
                                    isExplore = true,
                                    currentLang = currentLang,
                                    onToggle = { wideScreenExploreMode = false }
                                )
                            }
                        }

                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = t3(currentLang, "探索斜杠指令", "Discover Slash Commands", "スラッシュコマンド探索"),
                                    color = colors.textSecondary,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.Medium,
                                    modifier = Modifier.padding(start = 2.dp)
                                )
                                Spacer(modifier = Modifier.weight(1f))
                                Row(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(8.dp))
                                        .clickable {
                                            haptic.light()
                                            shuffleSeed += 1
                                        }
                                        .padding(horizontal = 8.dp, vertical = 4.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    LaunchpadSvgIcon(svgKey = "shuffle_refresh", tint = colors.textSecondary, size = 12.5.dp)
                                    Text(
                                        text = t3(currentLang, "换一批", "Shuffle", "入れ替え"),
                                        color = colors.textSecondary,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                }
                            }

                            LaunchpadTwoColumnGrid(
                                cards = exploreSlashCards,
                                allowLongPressPin = true,
                                pinnedIds = pinnedIds,
                                onTap = ::triggerCard,
                                onLongPressPin = ::handleLongPressPin
                            )
                        }

                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Text(
                                text = if (isPureChat) {
                                    t3(currentLang, "纯对话任务表达", "Chat Prompt Ideas", "チャット向けプロンプト例")
                                } else {
                                    t3(currentLang, "代码工作区任务表达", "Workspace Prompt Ideas", "開発ワークスペース向け例")
                                },
                                color = colors.textSecondary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.padding(start = 2.dp)
                            )

                            LaunchpadTwoColumnGrid(
                                cards = explorePromptCards,
                                allowLongPressPin = true,
                                pinnedIds = pinnedIds,
                                onTap = ::triggerCard,
                                onLongPressPin = ::handleLongPressPin
                            )
                        }
                    }
                }
        }
    }

    if (showConfiguratorSheet) {
        LaunchpadShortcutConfiguratorSheet(
            workflowTemplates = workflowTemplates,
            slashCommands = slashCommands,
            pendingPinItem = pendingPinItem,
            onDismiss = {
                showConfiguratorSheet = false
                pendingPinItem = null
            },
            onOpenWorkflowManager = onOpenWorkflowTemplates
        )
    }
}

@Composable
private fun WideModeTogglePill(
    isExplore: Boolean,
    currentLang: L10n.AppLang,
    onToggle: () -> Unit
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .clip(CircleShape)
            .background(colors.accentIndigo.copy(alpha = 0.13f))
            .clickable(onClick = onToggle)
            .padding(horizontal = 10.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Icon(
            imageVector = if (isExplore) Icons.Default.Bolt else Icons.Default.Explore,
            contentDescription = null,
            tint = colors.accentIndigo,
            modifier = Modifier.size(13.dp)
        )
        Text(
            text = if (isExplore) t3(currentLang, "快捷 2×2", "Quick 2×2", "2×2") else t3(currentLang, "探索与配置", "Explore", "探索"),
            color = colors.accentIndigo,
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold
        )
    }
}

@Composable
private fun LaunchpadWorkspacePulseBar(
    isPureChat: Boolean,
    isDraftConversation: Boolean,
    workspaceDisplayName: String,
    gitBranch: String?,
    uncommittedFileCount: Int,
    currentLang: L10n.AppLang,
    onSwitchWorkspace: () -> Unit,
    onTapDirtyFiles: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val shape = RoundedCornerShape(14.dp)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(46.dp)
            .clip(shape)
            .background(colors.surface)
            .border(0.7.dp, colors.border, shape)
            .padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        if (isPureChat) {
            Icon(
                imageVector = Icons.Default.ChatBubble,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(15.dp)
            )
            Text(
                text = t3(currentLang, "纯对话模式 · 不关联代码工作区", "Pure chat · No workspace linked", "チャット専用 · ワークスペース未連携"),
                color = colors.textSecondary,
                fontSize = 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
            if (isDraftConversation) {
                Text(
                    text = t3(currentLang, "选工作区", "Workspace", "選択"),
                    color = colors.accentIndigo,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clickable(onClick = onSwitchWorkspace)
                        .padding(horizontal = 6.dp, vertical = 4.dp)
                )
            }
        } else {
            Icon(
                imageVector = Icons.Default.Folder,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(15.dp)
            )
            Text(
                text = workspaceDisplayName,
                color = colors.textPrimary,
                fontSize = 13.5.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )
            if (!gitBranch.isNullOrBlank()) {
                Text(
                    text = gitBranch,
                    color = colors.textSecondary,
                    fontSize = 11.sp,
                    fontFamily = FontFamily.Monospace,
                    maxLines = 1,
                    modifier = Modifier
                        .clip(RoundedCornerShape(5.dp))
                        .background(colors.textPrimary.copy(alpha = 0.06f))
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                )
            }
            if (uncommittedFileCount > 0) {
                Text(
                    text = t3(currentLang, "$uncommittedFileCount 改动", "$uncommittedFileCount dirty", "$uncommittedFileCount 変更"),
                    color = LP_AMBER,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clip(RoundedCornerShape(5.dp))
                        .background(LP_AMBER.copy(alpha = 0.14f))
                        .clickable(onClick = onTapDirtyFiles)
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                )
            } else if (!gitBranch.isNullOrBlank()) {
                Text(
                    text = t3(currentLang, "干净", "Clean", "クリーン"),
                    color = colors.textSecondary,
                    fontSize = 11.sp
                )
            }
            Spacer(modifier = Modifier.weight(1f))
            if (isDraftConversation) {
                Text(
                    text = t3(currentLang, "切换", "Switch", "切替"),
                    color = colors.accentIndigo,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clickable(onClick = onSwitchWorkspace)
                        .padding(horizontal = 6.dp, vertical = 4.dp)
                )
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun LaunchpadTwoColumnGrid(
    cards: List<LaunchCardItem>,
    allowLongPressPin: Boolean,
    pinnedIds: Set<String>,
    onTap: (LaunchCardItem) -> Unit,
    onLongPressPin: (LaunchpadShortcutItem) -> Unit
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        cards.chunked(2).forEach { rowCards ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                rowCards.forEach { card ->
                    val isPinned = allowLongPressPin && card.shortcutItem?.stableId()?.let { it in pinnedIds } == true
                    Column(
                        modifier = Modifier
                            .weight(1f)
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(
                                width = 0.75.dp,
                                color = if (isPinned) colors.accentIndigo.copy(alpha = 0.35f) else colors.border,
                                shape = RoundedCornerShape(14.dp)
                            )
                            .combinedClickable(
                                onClick = { onTap(card) },
                                onLongClick = if (allowLongPressPin && card.shortcutItem != null) {
                                    { onLongPressPin(card.shortcutItem) }
                                } else null
                            )
                            .padding(horizontal = 13.dp, vertical = 13.5.dp),
                        verticalArrangement = Arrangement.spacedBy(6.5.dp)
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(7.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(22.dp)
                                    .clip(RoundedCornerShape(6.5.dp))
                                    .background(card.tint.copy(alpha = 0.13f)),
                                contentAlignment = Alignment.Center
                            ) {
                                LaunchpadSvgIcon(svgKey = card.svgKey, tint = card.tint, size = 13.5.dp)
                            }
                            Text(
                                text = card.title,
                                color = colors.textPrimary,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.SemiBold,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                            if (isPinned) {
                                Icon(
                                    imageVector = Icons.Default.PushPin,
                                    contentDescription = null,
                                    tint = colors.accentIndigo.copy(alpha = 0.85f),
                                    modifier = Modifier.size(11.dp)
                                )
                            }
                        }
                        Text(
                            text = card.subtitle,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
                if (rowCards.size == 1) {
                    Spacer(modifier = Modifier.weight(1f))
                }
            }
        }
    }
}
