package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.AutoFixHigh
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.ChatBubble
import androidx.compose.material.icons.filled.Checklist
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.CompareArrows
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Forum
import androidx.compose.material.icons.filled.Map
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.PushPin
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.QuotaBucketKind
import com.antigravity.mobile.data.model.TokenBookEquivalent
import com.antigravity.mobile.data.model.TokenHistoryCache
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.data.service.ModelPickerQuotaCache
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.loc
import java.util.Calendar
import kotlin.math.roundToInt

private data class LaunchCardItem(
    val id: String,
    val icon: ImageVector,
    val tint: Color,
    val title: String,
    val subtitle: String,
    val prompt: String
)

/**
 * 手机版新建对话窗口 · 方案 A（混合型轻启动台）
 * - 个性化问候头（带账号昵称）
 * - 单行状态微条（实时配额 + 今日用量 + 缓存命中率，点按直达配额页）
 * - 工作区 Git 脉搏栏（分支 + 未提交改动 + 草稿就地切换）
 * - 2×2 情境自适应快捷发起卡（动态感知脏工作区 + 置顶自定义模板 + 高频轻指令）
 * - 底部全球经典书目阶梯折算彩蛋（点按直达 Token 历史）
 * - 键盘弹出时平滑折叠为单行紧凑态；宽屏居中限宽 520dp。
 */
@Composable
fun NewChatLaunchpadView(
    title: String,
    workspaceName: String,
    isPureChat: Boolean,
    isDraftConversation: Boolean,
    gitBranch: String?,
    uncommittedFileCount: Int,
    selectedQuotaBucket: QuotaBucketKind,
    workflowTemplates: List<WorkflowTemplateRecord>,
    isKeyboardVisible: Boolean,
    onFillPrompt: (String) -> Unit,
    onSwitchWorkspace: () -> Unit,
    onOpenQuota: () -> Unit,
    onOpenTokenHistory: () -> Unit,
    onTapDirtyFiles: () -> Unit,
    onPrefetchTokenHistory: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val account by ModelPickerQuotaCache.account.collectAsState()
    val tokenHistory by TokenHistoryCache.history.collectAsState()
    val currentLang by L10n.currentLangFlow.collectAsState()

    LaunchedEffect(Unit) {
        onPrefetchTokenHistory()
    }

    val resolvedQuota = remember(account, selectedQuotaBucket) {
        account?.resolvedQuotaDisplay(selectedQuotaBucket)
    }
    val quotaPercent = resolvedQuota?.second?.remainingPercent?.roundToInt()?.coerceIn(0, 100)
    val quotaColor = when {
        quotaPercent == null -> colors.textSecondary
        quotaPercent >= 50 -> Color(0xFF22C55E)
        quotaPercent >= 20 -> Color(0xFFF59E0B)
        else -> Color(0xFFEF4444)
    }
    val quotaLabelShort = when (resolvedQuota?.first) {
        QuotaBucketKind.GEMINI_5H -> "Gemini 5h"
        QuotaBucketKind.GEMINI_WEEKLY -> "Gemini"
        QuotaBucketKind.CLAUDE_5H -> "Claude 5h"
        QuotaBucketKind.CLAUDE_WEEKLY -> "Claude"
        else -> ""
    }

    val todayTokensText = remember(tokenHistory) {
        val today = tokenHistory?.summary?.todayTokens ?: 0L
        if (today > 0L) formatTokensCompact(today) else null
    }
    val cacheHitPercent = remember(tokenHistory) {
        tokenHistory?.let { TokenBookEquivalent.cacheHitRatePercent(it) }
    }
    val bookEquivalent = remember(tokenHistory, currentLang) {
        tokenHistory?.let { TokenBookEquivalent.resolve(it) }
    }
    val hasStatusStripData = quotaPercent != null || todayTokensText != null || cacheHitPercent != null

    val greetingText = remember(account?.name, isKeyboardVisible, currentLang) {
        val hour = Calendar.getInstance().get(Calendar.HOUR_OF_DAY)
        val period = when (hour) {
            in 5..11 -> "上午好".loc
            in 12..17 -> "下午好".loc
            else -> "晚上好".loc
        }
        val rawName = account?.name?.trim().orEmpty()
        val shortName = rawName.split(" ").firstOrNull { it.isNotBlank() }.orEmpty()
        if (isKeyboardVisible) {
            if (shortName.isEmpty()) period else "$period, $shortName"
        } else {
            when (L10n.currentLang) {
                L10n.AppLang.EN -> if (shortName.isEmpty()) "$period, what's next?" else "$period, what's next, $shortName?"
                L10n.AppLang.JA -> if (shortName.isEmpty()) "${period}、次は何をしますか？" else "${period}、${shortName}さん。次は何をしますか？"
                L10n.AppLang.ZH -> if (shortName.isEmpty()) "${period}，接下来做什么？" else "${period}，接下来做什么，${shortName}？"
            }
        }
    }

    val workspaceDisplayName = remember(workspaceName, title) {
        workspaceName.trim().takeIf { it.isNotEmpty() && it != "Chat" } ?: title
    }

    val launchCards = remember(isPureChat, uncommittedFileCount, workflowTemplates, currentLang) {
        val result = mutableListOf<LaunchCardItem>()
        if (!isPureChat) {
            if (uncommittedFileCount > 0) {
                result.add(
                    LaunchCardItem(
                        id = "dirty_review",
                        icon = Icons.Default.Search,
                        tint = Color(0xFFF59E0B),
                        title = when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Review $uncommittedFileCount changes"
                            L10n.AppLang.JA -> "変更 $uncommittedFileCount 件をレビュー"
                            L10n.AppLang.ZH -> "审查当前 $uncommittedFileCount 个改动"
                        },
                        subtitle = "检查 git diff 风险与优化".loc,
                        prompt = "请先检查当前未提交的改动，指出潜在风险并给出优化建议".loc
                    )
                )
            } else {
                result.add(
                    LaunchCardItem(
                        id = "arch_overview",
                        icon = Icons.Default.Map,
                        tint = colors.accentIndigo,
                        title = "梳理项目结构".loc,
                        subtitle = "快速定位核心模块与入口".loc,
                        prompt = "请先梳理这个工作区的核心模块与关键入口文件".loc
                    )
                )
            }
        }

        val defaultIds = setOf("code_review", "test_and_fix", "research", "custom")
        val maxPinned = if (isPureChat) 1 else 2
        workflowTemplates
            .filter { it.inQuickMenu && it.id !in defaultIds }
            .take(maxPinned)
            .forEach { tpl ->
                result.add(
                    LaunchCardItem(
                        id = "pinned_${tpl.id}",
                        icon = Icons.Default.PushPin,
                        tint = Color(0xFF9333EA),
                        title = tpl.title,
                        subtitle = tpl.subtitle.ifBlank { "置顶模板".loc },
                        prompt = tpl.templatePrompt
                    )
                )
            }

        val fallback = if (isPureChat) {
            listOf(
                LaunchCardItem(
                    id = "chat_grill",
                    icon = Icons.Default.Forum,
                    tint = colors.accentIndigo,
                    title = "/grill-me 探讨".loc,
                    subtitle = "逐项拆解模糊想法".loc,
                    prompt = "/grill-me "
                ),
                LaunchCardItem(
                    id = "chat_search",
                    icon = Icons.Default.Public,
                    tint = colors.accentBlue,
                    title = "联网核查资讯".loc,
                    subtitle = "交叉验证最新进展".loc,
                    prompt = "请帮我联网核查以下消息的最新进展与可靠来源：".loc
                ),
                LaunchCardItem(
                    id = "chat_draft",
                    icon = Icons.Default.Edit,
                    tint = Color(0xFF14B8A6),
                    title = "起草与润色".loc,
                    subtitle = "文章 / 公告 / 提纲".loc,
                    prompt = "帮我起草一份清晰自然的说明：".loc
                ),
                LaunchCardItem(
                    id = "chat_compare",
                    icon = Icons.Default.CompareArrows,
                    tint = Color(0xFFF59E0B),
                    title = "多维方案对比".loc,
                    subtitle = "权衡利弊与取舍".loc,
                    prompt = "请从利弊、成本与风险角度对比以下方案：".loc
                )
            )
        } else {
            listOf(
                LaunchCardItem(
                    id = "ws_plan",
                    icon = Icons.Default.Checklist,
                    tint = colors.accentBlue,
                    title = "/plan 方案规划".loc,
                    subtitle = "先对齐步骤再动手".loc,
                    prompt = "/plan "
                ),
                LaunchCardItem(
                    id = "ws_grill",
                    icon = Icons.Default.Forum,
                    tint = colors.accentIndigo,
                    title = "/grill-me 探讨".loc,
                    subtitle = "逐项拆解需求决策".loc,
                    prompt = "/grill-me "
                ),
                LaunchCardItem(
                    id = "ws_bug",
                    icon = Icons.Default.BugReport,
                    tint = Color(0xFFEF4444),
                    title = "定位排查 Bug".loc,
                    subtitle = "结合上下文与日志定位".loc,
                    prompt = "请帮我排查并定位以下问题的根因：".loc
                ),
                LaunchCardItem(
                    id = "ws_refactor",
                    icon = Icons.Default.AutoFixHigh,
                    tint = Color(0xFF14B8A6),
                    title = "优化与重构".loc,
                    subtitle = "提升可读性与稳定性".loc,
                    prompt = "请分析当前模块可优化的设计点并给出轻量重构方案：".loc
                )
            )
        }
        for (item in fallback) {
            if (result.size >= 4) break
            result.add(item)
        }
        result.take(4)
    }

    Box(
        modifier = modifier.fillMaxSize(),
        contentAlignment = if (isKeyboardVisible) Alignment.Center else Alignment.Center
    ) {
        if (isKeyboardVisible) {
            // 键盘弹出时的单行紧凑折叠态
            Column(
                modifier = Modifier
                    .widthIn(max = 520.dp)
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    LaunchBrandBadge(size = 26.dp, iconSize = 14.dp)
                    Text(
                        text = greetingText,
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    if (hasStatusStripData) {
                        Row(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(colors.surface)
                                .border(0.7.dp, colors.border, CircleShape)
                                .clickable(onClick = onOpenQuota)
                                .padding(horizontal = 9.dp, vertical = 5.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            if (quotaPercent != null) {
                                Icon(
                                    imageVector = Icons.Default.Bolt,
                                    contentDescription = null,
                                    tint = quotaColor,
                                    modifier = Modifier.size(12.dp)
                                )
                                Text(
                                    text = "$quotaPercent%",
                                    color = quotaColor,
                                    fontSize = 11.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            if (todayTokensText != null) {
                                if (quotaPercent != null) {
                                    Text("·", color = colors.textMuted, fontSize = 11.sp)
                                }
                                Text(
                                    text = todayTokensText,
                                    color = colors.textSecondary,
                                    fontSize = 11.5.sp,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                        }
                    }
                }

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    launchCards.forEach { card ->
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(11.dp))
                                .background(colors.surface)
                                .border(0.7.dp, colors.border, RoundedCornerShape(11.dp))
                                .clickable { onFillPrompt(card.prompt) }
                                .padding(horizontal = 11.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Icon(
                                imageVector = card.icon,
                                contentDescription = null,
                                tint = card.tint,
                                modifier = Modifier.size(13.dp)
                            )
                            Text(
                                text = card.title,
                                color = colors.textPrimary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.Medium,
                                maxLines = 1
                            )
                        }
                    }
                }
            }
        } else {
            // 键盘收起时的完整混合轻启动台
            Column(
                modifier = Modifier
                    .widthIn(max = 520.dp)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 20.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                // 1. 个性化问候头
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    modifier = Modifier.padding(bottom = 2.dp)
                ) {
                    LaunchBrandBadge(size = 32.dp, iconSize = 17.dp)
                    Text(
                        text = greetingText,
                        color = colors.textPrimary,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                // 2. 单行状态微条
                if (hasStatusStripData) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(13.dp))
                            .background(colors.surface)
                            .border(0.7.dp, colors.border, RoundedCornerShape(13.dp))
                            .clickable(onClick = onOpenQuota)
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        if (quotaPercent != null) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.Bolt,
                                    contentDescription = null,
                                    tint = quotaColor,
                                    modifier = Modifier.size(13.dp)
                                )
                                if (quotaLabelShort.isNotEmpty()) {
                                    Text(
                                        text = quotaLabelShort,
                                        color = colors.textPrimary,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        maxLines = 1
                                    )
                                }
                                Text(
                                    text = "$quotaPercent%",
                                    color = quotaColor,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.Bold,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            Box(
                                modifier = Modifier
                                    .width(34.dp)
                                    .height(5.dp)
                                    .clip(CircleShape)
                                    .background(colors.textPrimary.copy(alpha = 0.08f))
                            ) {
                                Box(
                                    modifier = Modifier
                                        .fillMaxHeight()
                                        .fillMaxWidth((quotaPercent / 100f).coerceIn(0.06f, 1f))
                                        .clip(CircleShape)
                                        .background(quotaColor)
                                )
                            }
                        }

                        val rightSummary = buildList {
                            if (todayTokensText != null) add("${"今日".loc} $todayTokensText")
                            if (cacheHitPercent != null) add("${"缓存".loc} $cacheHitPercent%")
                        }.joinToString(" · ")

                        if (rightSummary.isNotEmpty()) {
                            Text(
                                text = rightSummary,
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                        } else {
                            Spacer(modifier = Modifier.weight(1f))
                        }

                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }

                // 3. 工作区 Git 脉搏栏 + 就地切换
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(13.dp))
                        .background(colors.surface)
                        .border(0.7.dp, colors.border, RoundedCornerShape(13.dp))
                        .padding(horizontal = 12.dp, vertical = 10.dp),
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
                            text = "纯对话模式 · 不关联代码工作区".loc,
                            color = colors.textSecondary,
                            fontSize = 12.5.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f)
                        )
                        if (isDraftConversation) {
                            Text(
                                text = "选工作区".loc,
                                color = colors.accentIndigo,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold,
                                modifier = Modifier
                                    .clickable(onClick = onSwitchWorkspace)
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
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
                            fontSize = 13.sp,
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
                                text = when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "$uncommittedFileCount dirty"
                                    L10n.AppLang.JA -> "$uncommittedFileCount 変更"
                                    L10n.AppLang.ZH -> "$uncommittedFileCount 改动"
                                },
                                color = Color(0xFFF59E0B),
                                fontSize = 11.sp,
                                fontWeight = FontWeight.SemiBold,
                                maxLines = 1,
                                modifier = Modifier
                                    .clip(RoundedCornerShape(5.dp))
                                    .background(Color(0xFFF59E0B).copy(alpha = 0.14f))
                                    .clickable(onClick = onTapDirtyFiles)
                                    .padding(horizontal = 6.dp, vertical = 2.dp)
                            )
                        } else if (!gitBranch.isNullOrBlank()) {
                            Text(
                                text = "干净".loc,
                                color = colors.textSecondary,
                                fontSize = 11.sp,
                                maxLines = 1
                            )
                        }
                        Spacer(modifier = Modifier.weight(1f))
                        if (isDraftConversation) {
                            Text(
                                text = "切换".loc,
                                color = colors.accentIndigo,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold,
                                modifier = Modifier
                                    .clickable(onClick = onSwitchWorkspace)
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
                            )
                        }
                    }
                }

                // 4. 2×2 快捷发起卡
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "快捷发起".loc,
                        color = colors.textSecondary,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.padding(start = 2.dp)
                    )
                    launchCards.chunked(2).forEach { rowCards ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(9.dp)
                        ) {
                            rowCards.forEach { card ->
                                Column(
                                    modifier = Modifier
                                        .weight(1f)
                                        .clip(RoundedCornerShape(13.dp))
                                        .background(colors.surface)
                                        .border(0.7.dp, colors.border, RoundedCornerShape(13.dp))
                                        .clickable { onFillPrompt(card.prompt) }
                                        .padding(horizontal = 12.dp, vertical = 11.dp),
                                    verticalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Icon(
                                            imageVector = card.icon,
                                            contentDescription = null,
                                            tint = card.tint,
                                            modifier = Modifier.size(14.dp)
                                        )
                                        Text(
                                            text = card.title,
                                            color = colors.textPrimary,
                                            fontSize = 13.sp,
                                            fontWeight = FontWeight.SemiBold,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
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

                // 5. 底部全球经典书目阶梯折算彩蛋
                if (bookEquivalent != null) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .clickable(onClick = onOpenTokenHistory)
                            .padding(horizontal = 8.dp, vertical = 8.dp),
                        horizontalArrangement = Arrangement.Center,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(
                            imageVector = Icons.Default.AutoAwesome,
                            contentDescription = null,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(13.dp)
                        )
                        Spacer(modifier = Modifier.width(5.dp))
                        Text(
                            text = bookEquivalent.sentence,
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Spacer(modifier = Modifier.width(2.dp))
                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun LaunchBrandBadge(size: Dp, iconSize: Dp) {
    val colors = AntigravityTheme.colors
    Box(
        modifier = Modifier
            .size(size)
            .clip(RoundedCornerShape(size * 0.3f))
            .background(colors.accentIndigo.copy(alpha = 0.14f)),
        contentAlignment = Alignment.Center
    ) {
        Icon(
            imageVector = Icons.Default.AutoAwesome,
            contentDescription = null,
            tint = colors.accentIndigo,
            modifier = Modifier.size(iconSize)
        )
    }
}
