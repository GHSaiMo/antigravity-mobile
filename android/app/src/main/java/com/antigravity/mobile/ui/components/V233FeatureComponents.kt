package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
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
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.automirrored.outlined.LiveHelp
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.BtwSideQuestionEntry
import com.antigravity.mobile.data.model.BtwStatusResponse
import com.antigravity.mobile.data.model.GitPullRequestItem
import com.antigravity.mobile.data.model.ProjectTrustStatusResponse
import com.antigravity.mobile.data.model.SideQuestionCardItem
import com.antigravity.mobile.data.model.SideQuestionOrigin
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.fetchGitHubPullRequests
import com.antigravity.mobile.data.service.listBtwSideQuestions
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

data class ParsedGitHubPRRef(
    val url: String,
    val owner: String,
    val repo: String,
    val number: Int
)

object GitHubPRLinkExtractor {
    private val prRegex = Regex("""https?://[A-Za-z0-9.-]+/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/(\d+)(?:[^\s)\]`"'>]*)?""")

    fun extract(text: String): List<ParsedGitHubPRRef> {
        if (!text.contains("/pull/")) return emptyList()
        val seen = mutableSetOf<String>()
        val out = mutableListOf<ParsedGitHubPRRef>()
        for (m in prRegex.findAll(text)) {
            val rawUrl = m.value.trimEnd('.', ',', ';', ':', '!', '?', ')', '`', '"', '\'', '<', '>')
            val owner = m.groupValues.getOrNull(1).orEmpty()
            val repo = m.groupValues.getOrNull(2).orEmpty()
            val num = m.groupValues.getOrNull(3)?.toIntOrNull() ?: 0
            if (owner.isNotEmpty() && repo.isNotEmpty() && num > 0) {
                val key = "$owner/$repo#$num".lowercase()
                if (seen.add(key)) {
                    out.add(ParsedGitHubPRRef(url = rawUrl, owner = owner, repo = repo, number = num))
                }
            }
        }
        return out
    }

    fun parseUrl(url: String): ParsedGitHubPRRef? = extract(url).firstOrNull()
}

object GitHubPRStatusCache {
    private data class Entry(val item: GitPullRequestItem, val fetchedAtMs: Long)
    private val cache = mutableMapOf<String, Entry>()
    private const val TTL_MS = 30_000L

    fun getCached(url: String): GitPullRequestItem? {
        val e = synchronized(cache) { cache[url.lowercase()] } ?: return null
        return if (System.currentTimeMillis() - e.fetchedAtMs < TTL_MS) e.item else null
    }

    suspend fun resolve(urls: List<String>): Map<String, GitPullRequestItem> {
        val res = mutableMapOf<String, GitPullRequestItem>()
        val missing = mutableListOf<String>()
        for (u in urls) {
            val cached = getCached(u)
            if (cached != null) res[u] = cached else missing.add(u)
        }
        if (missing.isNotEmpty()) {
            val client = ApiClient.defaultInstance
            if (client != null) {
                client.fetchGitHubPullRequests(missing).onSuccess { list ->
                    val now = System.currentTimeMillis()
                    synchronized(cache) {
                        for (item in list) {
                            cache[item.url.lowercase()] = Entry(item, now)
                            for (u in missing) {
                                if (u.equals(item.url, ignoreCase = true) || u.lowercase().contains("/pull/${item.number}")) {
                                    cache[u.lowercase()] = Entry(item, now)
                                    res[u] = item
                                }
                            }
                        }
                    }
                }
            }
        }
        return res
    }
}

@Composable
fun GitHubPRPillBadge(
    item: GitPullRequestItem,
    compact: Boolean = true,
    modifier: Modifier = Modifier
) {
    val (label, color) = when (item.state.uppercase()) {
        "MERGED" -> "MERGED" to Color(0xFF8957E5)
        "DRAFT" -> "DRAFT" to Color(0xFF6B7280)
        "CLOSED" -> "CLOSED" to Color(0xFFEF4444)
        else -> "OPEN" to Color(0xFF22C55E)
    }

    Row(
        modifier = modifier
            .clip(RoundedCornerShape(999.dp))
            .background(color.copy(alpha = 0.14f))
            .border(0.7.dp, color.copy(alpha = 0.38f), RoundedCornerShape(999.dp))
            .padding(horizontal = if (compact) 6.dp else 9.dp, vertical = if (compact) 2.dp else 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Icon(
            imageVector = Icons.Default.CallMerge,
            contentDescription = label,
            tint = color,
            modifier = Modifier.size(if (compact) 10.dp else 12.dp)
        )
        Text(
            text = "#${item.number} · $label",
            color = color,
            fontSize = if (compact) 10.sp else 11.5.sp,
            fontWeight = FontWeight.Bold,
            fontFamily = FontFamily.Monospace
        )
    }
}

@Composable
fun InlineGitHubPRPillsSection(
    content: String,
    onOpenPR: (String) -> Unit
) {
    val refs = remember(content) { GitHubPRLinkExtractor.extract(content).take(4) }
    if (refs.isEmpty()) return

    val colors = AntigravityTheme.colors
    var resolved by remember(refs) { mutableStateOf<Map<String, GitPullRequestItem>>(emptyMap()) }

    LaunchedEffect(refs) {
        val map = GitHubPRStatusCache.resolve(refs.map { it.url })
        if (map.isNotEmpty()) resolved = map
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        refs.forEach { ref ->
            val item = resolved[ref.url] ?: GitPullRequestItem(
                owner = ref.owner,
                repo = ref.repo,
                number = ref.number,
                title = "${ref.owner}/${ref.repo} #${ref.number}",
                url = ref.url,
                state = "OPEN"
            )
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surfaceVariant)
                    .border(0.7.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(10.dp))
                    .clickable { onOpenPR(ref.url) }
                    .padding(horizontal = 8.dp, vertical = 5.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                GitHubPRPillBadge(item = item, compact = true)
                val subtitle = resolved[ref.url]?.title?.takeIf { it.isNotBlank() } ?: "${ref.owner}/${ref.repo}"
                Text(
                    text = subtitle,
                    color = colors.textPrimary,
                    fontSize = 11.5.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.widthIn(max = 170.dp)
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GitHubPRDetailBottomSheet(
    prUrl: String,
    onDismiss: () -> Unit,
    onOpenInBrowser: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val ref = remember(prUrl) { GitHubPRLinkExtractor.parseUrl(prUrl) }
    var item by remember(prUrl) { mutableStateOf<GitPullRequestItem?>(GitHubPRStatusCache.getCached(prUrl)) }
    var isLoading by remember(prUrl) { mutableStateOf(item == null) }

    LaunchedEffect(prUrl) {
        isLoading = true
        val map = GitHubPRStatusCache.resolve(listOf(prUrl))
        item = map[prUrl] ?: map.values.firstOrNull() ?: item
        isLoading = false
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = colors.surface
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 28.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                item?.let { GitHubPRPillBadge(item = it, compact = false) }
                if (ref != null) {
                    Text(
                        text = "${ref.owner}/${ref.repo} #${ref.number}",
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier.weight(1f)
                    )
                }
                if (isLoading) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                }
            }

            Text(
                text = item?.title?.takeIf { it.isNotBlank() } ?: prUrl,
                color = colors.textPrimary,
                fontSize = 17.sp,
                fontWeight = FontWeight.Bold
            )

            item?.headRefName?.takeIf { it.isNotBlank() }?.let { head ->
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = head,
                        color = colors.textPrimary,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(colors.surfaceVariant)
                            .padding(horizontal = 7.dp, vertical = 3.dp)
                    )
                    Text("→", color = colors.textSecondary, fontSize = 12.sp)
                    Text(
                        text = item?.baseRefName?.ifBlank { "main" } ?: "main",
                        color = colors.textPrimary,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(colors.surfaceVariant)
                            .padding(horizontal = 7.dp, vertical = 3.dp)
                    )
                }
            }

            item?.authorLogin?.takeIf { it.isNotBlank() }?.let { author ->
                Text(text = "作者 @%s".loc(author), color = colors.textSecondary, fontSize = 13.sp)
            }

            item?.body?.takeIf { it.isNotBlank() }?.let { body ->
                HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                Text(text = body, color = colors.textPrimary, fontSize = 13.5.sp, lineHeight = 19.sp)
            }

            Button(
                onClick = {
                    val target = item?.url?.takeIf { it.isNotBlank() } ?: prUrl
                    onDismiss()
                    onOpenInBrowser(target)
                },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
            ) {
                Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("在浏览器中打开 Pull Request".loc, fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

@Composable
fun WorkspaceTrustBanner(
    status: ProjectTrustStatusResponse,
    isUpdating: Boolean,
    onTrust: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 6.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(Color(0xFFF59E0B).copy(alpha = 0.13f))
            .border(0.8.dp, Color(0xFFF59E0B).copy(alpha = 0.35f), RoundedCornerShape(12.dp))
            .padding(horizontal = 12.dp, vertical = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Icon(
            imageVector = Icons.Default.Security,
            contentDescription = null,
            tint = Color(0xFFF59E0B),
            modifier = Modifier.size(18.dp)
        )
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text("当前工作区尚未受信任".loc, color = colors.textPrimary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
            Text("部分文件与工具操作会受限，可一键授予信任。".loc, color = colors.textSecondary, fontSize = 11.sp)
        }
        Button(
            onClick = onTrust,
            enabled = !isUpdating,
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Color(0xFFD97706))
        ) {
            if (isUpdating) {
                CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = Color.White)
            } else {
                Text("信任".loc, fontSize = 12.sp, color = Color.White, fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

@Composable
fun SideQuestionInlineCardsSection(
    items: List<SideQuestionCardItem>,
    onOpen: (cascadeId: String, title: String) -> Unit,
    modifier: Modifier = Modifier
) {
    if (items.isEmpty()) return
    val colors = AntigravityTheme.colors
    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(start = 18.dp, top = 4.dp, bottom = 2.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        items.forEach { item ->
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.accentIndigo.copy(alpha = 0.07f))
                    .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.22f), RoundedCornerShape(12.dp))
                    .clickable { onOpen(item.cascadeId, item.displayTitle) }
                    .padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(
                    modifier = Modifier
                        .width(2.5.dp)
                        .height(32.dp)
                        .clip(RoundedCornerShape(2.dp))
                        .background(colors.accentIndigo.copy(alpha = 0.5f))
                )
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp)
                    ) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Outlined.LiveHelp,
                            contentDescription = null,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(12.dp)
                        )
                        Text(
                            text = "旁路快问".loc,
                            color = colors.accentIndigo,
                            fontSize = 10.5.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                        Text("·", color = colors.textSecondary, fontSize = 10.5.sp)
                        Text(
                            text = item.displayTitle,
                            color = colors.textPrimary,
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f)
                        )
                        when {
                            item.isRunning -> CircularProgressIndicator(
                                modifier = Modifier.size(11.dp),
                                strokeWidth = 1.6.dp,
                                color = colors.accentIndigo
                            )
                            item.isError -> Box(
                                modifier = Modifier
                                    .size(6.dp)
                                    .clip(CircleShape)
                                    .background(colors.accentRed)
                            )
                            else -> Box(
                                modifier = Modifier
                                    .size(6.dp)
                                    .clip(CircleShape)
                                    .background(colors.accentIndigo.copy(alpha = 0.65f))
                            )
                        }
                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textSecondary.copy(alpha = 0.75f),
                            modifier = Modifier.size(14.dp)
                        )
                    }
                    val preview = item.answerPreview?.trim().orEmpty()
                    if (preview.isNotEmpty()) {
                        val styledPreview = remember(preview, colors) {
                            buildCompactMarkdownPreview(preview, colors, 12.sp)
                        }
                        Text(
                            text = styledPreview,
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis
                        )
                    } else if (item.isRunning) {
                        Text(
                            text = "正在独立生成回复...".loc,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp
                        )
                    }
                }
            }
        }
    }
}

@Composable
fun BranchOriginBanner(
    origin: SideQuestionOrigin,
    isExpanded: Boolean,
    onToggleExpand: () -> Unit,
    onReturnToParent: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val title = origin.parentTitle?.trim().orEmpty()
    val turn = origin.forkTurnNumber
    val originLabel = when {
        title.isNotEmpty() && turn != null && turn > 0 -> "承接自「%1\$s」第 %2\$d 轮".loc(title, turn)
        title.isNotEmpty() -> "承接自「%s」".loc(title)
        turn != null && turn > 0 -> "承接自主会话第 %d 轮".loc(turn)
        else -> "承接自主会话上下文".loc
    }
    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(colors.accentIndigo.copy(alpha = 0.07f))
            .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.24f), RoundedCornerShape(12.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Icon(
                imageVector = Icons.AutoMirrored.Outlined.LiveHelp,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(14.dp)
            )
            Text(
                text = originLabel,
                color = colors.textPrimary,
                fontSize = 12.5.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = if (isExpanded) "已展开共享前文".loc else "已共享前文上下文（默认折叠）".loc,
                color = colors.textSecondary,
                fontSize = 11.5.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
            Row(
                modifier = Modifier
                    .clip(RoundedCornerShape(999.dp))
                    .background(colors.accentIndigo.copy(alpha = 0.12f))
                    .clickable(onClick = onToggleExpand)
                    .padding(horizontal = 9.dp, vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(3.dp)
            ) {
                Icon(
                    imageVector = if (isExpanded) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                    contentDescription = null,
                    tint = colors.accentIndigo,
                    modifier = Modifier.size(13.dp)
                )
                Text(
                    text = if (isExpanded) "收起前文".loc else "展开前文".loc,
                    color = colors.accentIndigo,
                    fontSize = 11.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    softWrap = false
                )
            }
            if (origin.parentCascadeId.isNotBlank()) {
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(999.dp))
                        .background(colors.surfaceVariant)
                        .clickable(onClick = onReturnToParent)
                        .padding(horizontal = 9.dp, vertical = 4.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(3.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.SubdirectoryArrowLeft,
                        contentDescription = null,
                        tint = colors.textPrimary,
                        modifier = Modifier.size(12.dp)
                    )
                    Text(
                        text = "回主会话".loc,
                        color = colors.textPrimary,
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        softWrap = false
                    )
                }
            }
        }
    }
}

@Composable
fun BranchForkDivider(
    onCollapse: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        HorizontalDivider(modifier = Modifier.weight(1f), color = colors.accentIndigo.copy(alpha = 0.25f))
        Row(
            modifier = Modifier
                .clip(RoundedCornerShape(999.dp))
                .background(colors.accentIndigo.copy(alpha = 0.10f))
                .clickable(onClick = onCollapse)
                .padding(horizontal = 10.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Icon(
                imageVector = Icons.AutoMirrored.Outlined.LiveHelp,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(12.dp)
            )
            Text(
                text = "以上为共享前文 · 以下为旁路分支".loc,
                color = colors.accentIndigo,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                softWrap = false
            )
        }
        HorizontalDivider(modifier = Modifier.weight(1f), color = colors.accentIndigo.copy(alpha = 0.25f))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BtwSideQuestionBottomSheet(
    parentCascadeId: String,
    parentTitle: String,
    anchorStepIndex: Int?,
    anchorPreview: String?,
    existingBranches: List<SideQuestionCardItem>,
    onOpenBranch: (cascadeId: String, title: String) -> Unit,
    onStartBranch: suspend (question: String, stepIndex: Int?) -> Boolean,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    var questionText by remember { mutableStateOf("") }
    var useAnchorStep by remember(anchorStepIndex) { mutableStateOf(anchorStepIndex != null) }
    var isStarting by remember { mutableStateOf(false) }
    var loadedBranches by remember { mutableStateOf<List<SideQuestionCardItem>>(emptyList()) }
    val branches = if (loadedBranches.isNotEmpty()) loadedBranches else existingBranches

    LaunchedEffect(parentCascadeId) {
        if (parentCascadeId.isNotBlank()) {
            ApiClient.defaultInstance?.listBtwSideQuestions(parentCascadeId)?.onSuccess { list ->
                loadedBranches = list.map { entry ->
                    SideQuestionCardItem(
                        cascadeId = entry.cascadeId,
                        parentCascadeId = entry.parentCascadeId ?: parentCascadeId,
                        title = entry.title,
                        question = entry.question,
                        answerPreview = entry.answerPreview,
                        status = entry.status,
                        forkStepIndex = entry.forkStepIndex,
                        anchorMessageId = null,
                        turnCount = entry.turnCount,
                        createdAt = entry.createdAt
                    )
                }
            }
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = colors.surface
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 16.dp)
                .padding(bottom = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text("/btw 旁路快问".loc, color = colors.textPrimary, fontSize = 16.sp, fontWeight = FontWeight.Bold)
                TextButton(onClick = onDismiss) { Text("关闭".loc) }
            }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 320.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (anchorStepIndex != null && useAnchorStep) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.accentIndigo.copy(alpha = 0.08f))
                            .border(0.8.dp, colors.accentIndigo.copy(alpha = 0.24f), RoundedCornerShape(12.dp))
                            .padding(11.dp),
                        verticalArrangement = Arrangement.spacedBy(5.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Row(
                                modifier = Modifier.weight(1f).padding(end = 8.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(5.dp)
                            ) {
                                Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(13.dp))
                                Text(
                                    "承接至所选消息的上下文".loc,
                                    color = colors.accentIndigo,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                            Text(
                                text = "改用最新上下文".loc,
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                maxLines = 1,
                                softWrap = false,
                                modifier = Modifier.clickable { useAnchorStep = false }
                            )
                        }
                        val preview = anchorPreview?.trim().orEmpty()
                        if (preview.isNotEmpty()) {
                            val styledAnchor = remember(preview, colors) {
                                buildCompactMarkdownPreview(preview, colors, 12.sp)
                            }
                            Text(
                                text = styledAnchor,
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis
                            )
                        } else {
                            Text(
                                text = "将从 Step #%d 处分叉，不携带后续消息。".loc(anchorStepIndex),
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                } else {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.surfaceVariant)
                            .padding(11.dp),
                        verticalAlignment = Alignment.Top,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(15.dp))
                        Text(
                            text = "基于「%s」当前上下文创建独立旁路分支，不打断主对话流，支持多轮追问。".loc(parentTitle),
                            color = colors.textSecondary,
                            fontSize = 12.5.sp
                        )
                    }
                }

                if (branches.isNotEmpty()) {
                    Column(verticalArrangement = Arrangement.spacedBy(7.dp)) {
                        Text(
                            text = "已有旁路分支 (%d)".loc(branches.size),
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                        branches.forEach { branch ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(12.dp))
                                    .background(colors.surfaceVariant)
                                    .clickable {
                                        onDismiss()
                                        onOpenBranch(branch.cascadeId, branch.displayTitle)
                                    }
                                    .padding(horizontal = 12.dp, vertical = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(14.dp))
                                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                    Text(
                                        text = branch.displayTitle,
                                        color = colors.textPrimary,
                                        fontSize = 13.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    branch.answerPreview?.trim()?.takeIf { it.isNotEmpty() }?.let { ans ->
                                        val styledAns = remember(ans, colors) {
                                            buildCompactMarkdownPreview(ans, colors, 11.5.sp)
                                        }
                                        Text(
                                            text = styledAns,
                                            color = colors.textSecondary,
                                            fontSize = 11.5.sp,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                    }
                                }
                                if (branch.isRunning) {
                                    CircularProgressIndicator(modifier = Modifier.size(12.dp), strokeWidth = 1.6.dp, color = colors.accentIndigo)
                                }
                                Icon(Icons.Default.ChevronRight, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(16.dp))
                            }
                        }
                    }
                }
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                OutlinedTextField(
                    value = questionText,
                    onValueChange = { questionText = it },
                    placeholder = { Text("输入旁路问题（/btw）...".loc, fontSize = 14.sp) },
                    modifier = Modifier.weight(1f),
                    maxLines = 3
                )
                val canSend = !isStarting && questionText.isNotBlank()
                IconButton(
                    enabled = canSend,
                    onClick = {
                        val q = questionText.trim()
                        if (q.isEmpty() || isStarting) return@IconButton
                        isStarting = true
                        scope.launch {
                            val ok = onStartBranch(q, if (useAnchorStep) anchorStepIndex else null)
                            isStarting = false
                            if (ok) onDismiss()
                        }
                    },
                    modifier = Modifier
                        .size(44.dp)
                        .clip(CircleShape)
                        .background(if (canSend) colors.accentIndigo else colors.surfaceVariant)
                ) {
                    if (isStarting) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = Color.White)
                    } else {
                        Icon(
                            Icons.AutoMirrored.Filled.Send,
                            contentDescription = "发送".loc,
                            tint = if (canSend) Color.White else colors.textSecondary.copy(alpha = 0.5f),
                            modifier = Modifier.size(18.dp)
                        )
                    }
                }
            }
        }
    }
}
