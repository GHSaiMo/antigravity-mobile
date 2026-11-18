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
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.automirrored.filled.Send
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
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.fetchBtwStatus
import com.antigravity.mobile.data.service.fetchGitHubPullRequests
import com.antigravity.mobile.data.service.listBtwSideQuestions
import com.antigravity.mobile.data.service.startBtwSideQuestion
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
                Text(text = "作者 @$author", color = colors.textSecondary, fontSize = 13.sp)
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
                Text("在浏览器中打开 Pull Request", fontWeight = FontWeight.SemiBold)
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
            Text("当前工作区尚未受信任", color = colors.textPrimary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
            Text("部分文件与工具操作会受限，可一键授予信任。", color = colors.textSecondary, fontSize = 11.sp)
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
                Text("信任", fontSize = 12.sp, color = Color.White, fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BtwSideQuestionBottomSheet(
    parentCascadeId: String,
    activeModel: String,
    initialQuote: String?,
    onInsertIntoMain: (String) -> Unit,
    onOpenStandalone: ((String) -> Unit)? = null,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    var questionText by remember { mutableStateOf("") }
    var quoteText by remember(initialQuote) { mutableStateOf(initialQuote.orEmpty()) }
    var entries by remember { mutableStateOf<List<BtwSideQuestionEntry>>(emptyList()) }
    var activeSideId by remember { mutableStateOf<String?>(null) }
    var activeStatus by remember { mutableStateOf<BtwStatusResponse?>(null) }
    var isStarting by remember { mutableStateOf(false) }
    var errorMsg by remember { mutableStateOf<String?>(null) }
    var showThinking by remember { mutableStateOf(false) }

    LaunchedEffect(parentCascadeId) {
        if (parentCascadeId.isNotBlank()) {
            ApiClient.defaultInstance?.listBtwSideQuestions(parentCascadeId)?.onSuccess { list ->
                entries = list
                if (activeSideId == null && list.isNotEmpty()) {
                    activeSideId = list.last().cascadeId
                }
            }
        }
    }

    LaunchedEffect(activeSideId) {
        val sid = activeSideId ?: return@LaunchedEffect
        val client = ApiClient.defaultInstance ?: return@LaunchedEffect
        while (true) {
            var done = false
            client.fetchBtwStatus(sid).onSuccess { st ->
                activeStatus = st
                if (!st.isRunning) done = true
            }
            if (done) break
            delay(850L)
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
                Text("/btw 旁路快问", color = colors.textPrimary, fontSize = 16.sp, fontWeight = FontWeight.Bold)
                TextButton(onClick = onDismiss) { Text("关闭") }
            }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 180.dp, max = 380.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (quoteText.isNotBlank()) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant)
                            .padding(10.dp),
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("引用片段", color = colors.accentIndigo, fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold)
                            Text("清除", color = colors.textSecondary, fontSize = 11.5.sp, modifier = Modifier.clickable { quoteText = "" })
                        }
                        Text(quoteText, color = colors.textSecondary, fontSize = 12.sp, maxLines = 4, overflow = TextOverflow.Ellipsis)
                    }
                }

                if (entries.isNotEmpty()) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        entries.forEach { entry ->
                            val selected = activeSideId == entry.cascadeId
                            Row(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(if (selected) colors.accentIndigo.copy(alpha = 0.20f) else colors.surfaceVariant)
                                    .clickable { activeSideId = entry.cascadeId }
                                    .padding(horizontal = 10.dp, vertical = 5.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(5.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(6.dp)
                                        .clip(CircleShape)
                                        .background(if (entry.status == "CASCADE_RUN_STATUS_RUNNING") colors.accentGreen else colors.accentIndigo)
                                )
                                Text(
                                    text = entry.title ?: entry.question ?: "Side question",
                                    color = colors.textPrimary,
                                    fontSize = 12.sp,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.widthIn(max = 150.dp)
                                )
                            }
                        }
                    }
                }

                val st = activeStatus
                if (st != null) {
                    st.question?.takeIf { it.isNotBlank() }?.let { q ->
                        Text(
                            text = q,
                            color = colors.textPrimary,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.accentIndigo.copy(alpha = 0.12f))
                                .padding(10.dp)
                        )
                    }

                    st.thinking?.takeIf { it.isNotBlank() }?.let { think ->
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.surfaceVariant)
                                .clickable { showThinking = !showThinking }
                                .padding(10.dp)
                        ) {
                            Text(
                                text = if (showThinking) "▼ 深度思考" else "▶ 深度思考",
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Medium
                            )
                            if (showThinking) {
                                Spacer(Modifier.height(6.dp))
                                Text(think, color = colors.textSecondary, fontSize = 12.sp)
                            }
                        }
                    }

                    st.errorMessage?.takeIf { st.hasError && it.isNotBlank() }?.let { err ->
                        Text(
                            text = err,
                            color = colors.accentRed,
                            fontSize = 12.5.sp,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.accentRed.copy(alpha = 0.12f))
                                .padding(10.dp)
                        )
                    }

                    val ans = st.answer.orEmpty()
                    if (ans.isNotBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surfaceVariant.copy(alpha = 0.7f))
                                .padding(12.dp)
                        ) {
                            MarkdownContentView(content = ans)
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Button(
                                onClick = {
                                    val inserted = st.insertText?.takeIf { it.isNotBlank() } ?: ans
                                    onDismiss()
                                    onInsertIntoMain(inserted)
                                },
                                modifier = Modifier.weight(1f),
                                colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                            ) {
                                Text("插入主对话", fontWeight = FontWeight.SemiBold)
                            }
                            if (onOpenStandalone != null) {
                                OutlinedButton(onClick = {
                                    onDismiss()
                                    onOpenStandalone(st.cascadeId)
                                }) {
                                    Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = "在独立会话打开", modifier = Modifier.size(16.dp))
                                }
                            }
                        }
                    } else if (st.isRunning) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                            Text("旁路快问正在生成回复...", color = colors.textSecondary, fontSize = 13.sp)
                        }
                    }
                } else if (!isStarting && entries.isEmpty()) {
                    Text(
                        text = "随时在不打断主任务流的前提下发起旁路提问，结果可一键引用插入主对话输入框。",
                        color = colors.textSecondary,
                        fontSize = 12.5.sp,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant)
                            .padding(12.dp)
                    )
                }

                errorMsg?.let { Text(it, color = colors.accentRed, fontSize = 12.sp) }
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                OutlinedTextField(
                    value = questionText,
                    onValueChange = { questionText = it },
                    placeholder = { Text("输入旁路快问（/btw）...", fontSize = 14.sp) },
                    modifier = Modifier.weight(1f),
                    maxLines = 3
                )
                IconButton(
                    enabled = !isStarting && (questionText.isNotBlank() || quoteText.isNotBlank()),
                    onClick = {
                        val q = questionText.trim()
                        val qt = quoteText.trim()
                        val client = ApiClient.defaultInstance ?: return@IconButton
                        isStarting = true
                        errorMsg = null
                        scope.launch {
                            client.startBtwSideQuestion(
                                parentCascadeId = parentCascadeId,
                                question = q,
                                quote = qt.ifBlank { null },
                                model = activeModel
                            ).onSuccess { res ->
                                questionText = ""
                                quoteText = ""
                                val entry = BtwSideQuestionEntry(
                                    cascadeId = res.cascadeId,
                                    parentCascadeId = parentCascadeId,
                                    title = res.title,
                                    question = res.question,
                                    quote = res.quote,
                                    status = "CASCADE_RUN_STATUS_RUNNING",
                                    createdAt = res.createdAt
                                )
                                entries = entries + entry
                                activeSideId = res.cascadeId
                            }.onFailure { err ->
                                errorMsg = err.message ?: "启动旁路快问失败"
                            }
                            isStarting = false
                        }
                    },
                    modifier = Modifier
                        .size(44.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo)
                ) {
                    if (isStarting) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = Color.White)
                    } else {
                        Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "发送", tint = Color.White, modifier = Modifier.size(18.dp))
                    }
                }
            }
        }
    }
}
