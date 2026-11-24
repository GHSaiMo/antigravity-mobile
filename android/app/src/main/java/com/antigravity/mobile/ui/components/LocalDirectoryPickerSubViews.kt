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
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.CreateNewFolder
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.filled.WorkOutline
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.LocalDirEntry
import com.antigravity.mobile.data.model.LocalDirQuickItem
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.cancelCloneGitHubRepo
import com.antigravity.mobile.data.service.createLocalDir
import com.antigravity.mobile.data.service.createProjectWorkspace
import com.antigravity.mobile.data.service.fetchCloneGitHubRepoStatus
import com.antigravity.mobile.data.service.fetchLocalDirs
import com.antigravity.mobile.data.service.startCloneGitHubRepo
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

internal data class LocalDirBrowserState(
    val currentPath: String = "",
    val currentDisplay: String = "",
    val parentPath: String = "",
    val canGoUp: Boolean = false,
    val recommendedDirs: List<LocalDirQuickItem> = emptyList(),
    val entries: List<LocalDirEntry> = emptyList(),
    val isLoading: Boolean = false,
    val isCreatingDir: Boolean = false,
    val error: String? = null
)

@Composable
internal fun LocalDirectoryBrowserSection(
    state: LocalDirBrowserState,
    sectionTitle: String,
    subtitle: String,
    onLoadPath: (String?) -> Unit,
    onCreateDir: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    var showNewFolderDialog by remember { mutableStateOf(false) }
    var newFolderInput by remember { mutableStateOf("") }

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        // Section header + New Folder button
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = sectionTitle,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.textPrimary
                )
                Text(
                    text = subtitle,
                    fontSize = 11.5.sp,
                    color = colors.textSecondary
                )
            }
            Spacer(modifier = Modifier.width(8.dp))
            Row(
                modifier = Modifier
                    .clip(CircleShape)
                    .background(colors.accentBlue.copy(alpha = 0.12f))
                    .clickable(enabled = !state.isCreatingDir && state.currentPath.isNotBlank()) {
                        newFolderInput = ""
                        showNewFolderDialog = true
                    }
                    .padding(horizontal = 10.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                if (state.isCreatingDir) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(12.dp),
                        strokeWidth = 1.8.dp,
                        color = colors.accentBlue
                    )
                } else {
                    Icon(
                        imageVector = Icons.Default.CreateNewFolder,
                        contentDescription = null,
                        tint = colors.accentBlue,
                        modifier = Modifier.size(14.dp)
                    )
                }
                Text(
                    text = "新建目录".loc,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.accentBlue
                )
            }
        }

        // Quick recommended project root chips
        if (state.recommendedDirs.isNotEmpty()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                state.recommendedDirs.forEach { item ->
                    val isSelected = state.currentPath == item.path
                    Row(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(if (isSelected) colors.accentBlue else colors.surfaceVariant)
                            .clickable { onLoadPath(item.path) }
                            .padding(horizontal = 11.dp, vertical = 7.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp)
                    ) {
                        Icon(
                            imageVector = if (item.projectCount > 0) Icons.Default.Folder else Icons.Default.Home,
                            contentDescription = null,
                            tint = if (isSelected) Color.White else colors.textSecondary,
                            modifier = Modifier.size(13.dp)
                        )
                        Text(
                            text = item.name,
                            fontSize = 12.5.sp,
                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Medium,
                            color = if (isSelected) Color.White else colors.textPrimary,
                            maxLines = 1
                        )
                        if (item.projectCount > 0) {
                            Box(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(
                                        if (isSelected) Color.White.copy(alpha = 0.22f)
                                        else colors.textSecondary.copy(alpha = 0.14f)
                                    )
                                    .padding(horizontal = 5.dp, vertical = 1.dp)
                            ) {
                                Text(
                                    text = "${item.projectCount}",
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = if (isSelected) Color.White else colors.textSecondary
                                )
                            }
                        }
                    }
                }
            }
        }

        // Folder browser card
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(13.dp))
                .background(colors.surfaceVariant)
                .border(0.5.dp, colors.border, RoundedCornerShape(13.dp))
        ) {
            // Current path header bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(colors.surface)
                    .padding(horizontal = 12.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                if (state.canGoUp) {
                    Row(
                        modifier = Modifier
                            .clip(RoundedCornerShape(7.dp))
                            .background(colors.accentBlue.copy(alpha = 0.1f))
                            .clickable { onLoadPath(state.parentPath) }
                            .padding(horizontal = 8.dp, vertical = 5.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "上一级".loc,
                            tint = colors.accentBlue,
                            modifier = Modifier.size(13.dp)
                        )
                        Text(
                            text = "上一级".loc,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium,
                            color = colors.accentBlue
                        )
                    }
                }

                Icon(
                    imageVector = Icons.Default.Folder,
                    contentDescription = null,
                    tint = colors.accentBlue,
                    modifier = Modifier.size(15.dp)
                )

                Text(
                    text = state.currentDisplay.ifBlank { "加载中…".loc },
                    fontSize = 12.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f)
                )

                if (state.isLoading) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = colors.accentBlue
                    )
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            when {
                state.error != null -> {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Warning,
                            contentDescription = null,
                            tint = colors.accentOrange,
                            modifier = Modifier.size(15.dp)
                        )
                        Text(
                            text = state.error,
                            fontSize = 12.sp,
                            color = colors.textPrimary
                        )
                    }
                }
                state.isLoading && state.entries.isEmpty() -> {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 24.dp),
                        horizontalArrangement = Arrangement.Center,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(18.dp),
                            strokeWidth = 2.dp,
                            color = colors.accentBlue
                        )
                        Spacer(modifier = Modifier.width(8.dp))
                        Text(
                            text = "正在读取本机目录…".loc,
                            fontSize = 12.5.sp,
                            color = colors.textSecondary
                        )
                    }
                }
                state.entries.isEmpty() -> {
                    Column(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 22.dp, horizontal = 12.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Folder,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(22.dp)
                        )
                        Text(
                            text = "当前目录下暂无子文件夹".loc,
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.Medium,
                            color = colors.textSecondary
                        )
                        Text(
                            text = "可直接选用此目录，或点击右上角「新建目录」".loc,
                            fontSize = 11.sp,
                            color = colors.textMuted,
                            textAlign = TextAlign.Center
                        )
                    }
                }
                else -> {
                    state.entries.forEachIndexed { idx, entry ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { onLoadPath(entry.path) }
                                .padding(horizontal = 12.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            Icon(
                                imageVector = if (entry.isProject) Icons.Default.WorkOutline else Icons.Default.Folder,
                                contentDescription = null,
                                tint = when {
                                    entry.isProject -> colors.accentBlue
                                    entry.isGitRepo -> colors.accentIndigo
                                    else -> colors.textSecondary
                                },
                                modifier = Modifier.size(17.dp)
                            )

                            Text(
                                text = entry.name,
                                fontSize = 13.5.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.textPrimary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )

                            if (entry.isProject) {
                                Box(
                                    modifier = Modifier
                                        .clip(CircleShape)
                                        .background(colors.accentBlue.copy(alpha = 0.12f))
                                        .padding(horizontal = 6.dp, vertical = 2.dp)
                                ) {
                                    Text(
                                        text = "项目".loc,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentBlue
                                    )
                                }
                            } else if (entry.isGitRepo) {
                                Box(
                                    modifier = Modifier
                                        .clip(CircleShape)
                                        .background(colors.accentIndigo.copy(alpha = 0.12f))
                                        .padding(horizontal = 6.dp, vertical = 2.dp)
                                ) {
                                    Text(
                                        text = "Git",
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentIndigo
                                    )
                                }
                            }

                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                                contentDescription = null,
                                tint = colors.textMuted,
                                modifier = Modifier.size(11.dp)
                            )
                        }
                        if (idx < state.entries.lastIndex) {
                            HorizontalDivider(
                                modifier = Modifier.padding(start = 39.dp),
                                thickness = 0.5.dp,
                                color = colors.border
                            )
                        }
                    }
                }
            }
        }
    }

    if (showNewFolderDialog) {
        AlertDialog(
            onDismissRequest = { showNewFolderDialog = false },
            title = { Text("新建本地目录".loc) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "将在 ${state.currentDisplay.ifBlank { "~" }} 下新建目录，或输入 ~/... 完整路径".loc,
                        fontSize = 12.5.sp,
                        color = colors.textSecondary
                    )
                    OutlinedTextField(
                        value = newFolderInput,
                        onValueChange = { newFolderInput = it },
                        label = { Text("文件夹名称 或 ~/自定义路径".loc) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    val trimmed = newFolderInput.trim()
                    showNewFolderDialog = false
                    if (trimmed.isNotEmpty()) {
                        onCreateDir(trimmed)
                    }
                }) { Text("创建并选中".loc) }
            },
            dismissButton = {
                TextButton(onClick = { showNewFolderDialog = false }) { Text("取消".loc) }
            }
        )
    }
}

private data class ParsedGitHubRepo(val slug: String, val repoName: String)

private val gitHubSegRegex = Regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$")

private fun parseGitHubRepoInput(raw: String): ParsedGitHubRepo? {
    var s = raw.trim()
    if (s.isEmpty() || s.startsWith("-")) return null
    var fromUrl = false
    when {
        s.startsWith("git@github.com:") -> {
            s = s.removePrefix("git@github.com:")
            fromUrl = true
        }
        s.startsWith("https://github.com/") -> {
            s = s.removePrefix("https://github.com/")
            fromUrl = true
        }
        s.startsWith("http://github.com/") -> {
            s = s.removePrefix("http://github.com/")
            fromUrl = true
        }
        s.contains("://") || s.contains("@") -> return null
    }
    val cutIdx = s.indexOfAny(charArrayOf('?', '#'))
    if (cutIdx >= 0) s = s.substring(0, cutIdx)
    s = s.trim('/')
    val parts = s.split('/').filter { it.isNotEmpty() }
    if (parts.size < 2 || (!fromUrl && parts.size != 2)) return null
    val owner = parts[0]
    val repo = parts[1].removeSuffix(".git")
    if (owner == "." || owner == ".." || repo == "." || repo == "..") return null
    if (!gitHubSegRegex.matches(owner) || !gitHubSegRegex.matches(repo)) return null
    return ParsedGitHubRepo("$owner/$repo", repo)
}

private sealed class CloneUiPhase {
    data object Form : CloneUiPhase()
    data class Running(
        val jobId: String,
        val progress: Int,
        val message: String,
        val targetPath: String
    ) : CloneUiPhase()
    data class Completed(
        val project: ProjectItem,
        val repoSlug: String,
        val alreadyExists: Boolean
    ) : CloneUiPhase()
}

@Composable
internal fun CloneGitHubRepoSubView(
    onBack: () -> Unit,
    onProjectReady: (ProjectItem) -> Unit,
    onStartConversation: (ProjectItem) -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()

    var dirState by remember { mutableStateOf(LocalDirBrowserState(isLoading = true)) }
    var repoInput by remember { mutableStateOf("") }
    var customFolderName by remember { mutableStateOf("") }
    var showFolderRename by remember { mutableStateOf(false) }
    var phase by remember { mutableStateOf<CloneUiPhase>(CloneUiPhase.Form) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var conflictAlreadyClonedPath by remember { mutableStateOf<String?>(null) }
    var isSubmitting by remember { mutableStateOf(false) }
    var pollJob by remember { mutableStateOf<Job?>(null) }

    val parsedRepo = remember(repoInput) { parseGitHubRepoInput(repoInput) }
    val effectiveFolderName = customFolderName.trim().ifEmpty { parsedRepo?.repoName.orEmpty() }
    val previewPath = remember(dirState.currentPath, dirState.currentDisplay, effectiveFolderName) {
        if (dirState.currentPath.isBlank()) ""
        else {
            val base = dirState.currentDisplay.ifBlank { dirState.currentPath }
            if (effectiveFolderName.isBlank()) base
            else {
                val sep = if (dirState.currentPath.contains('\\')) "\\" else "/"
                if (base.endsWith(sep)) "$base$effectiveFolderName" else "$base$sep$effectiveFolderName"
            }
        }
    }

    fun loadDirs(path: String?) {
        val client = ApiClient.defaultInstance
        if (client == null) {
            dirState = dirState.copy(isLoading = false, error = "未配置网关地址".loc)
            return
        }
        dirState = dirState.copy(isLoading = true, error = null)
        scope.launch {
            client.fetchLocalDirs(path).fold(
                onSuccess = { resp ->
                    dirState = dirState.copy(
                        currentPath = resp.currentPath,
                        currentDisplay = resp.currentDisplay?.takeIf { it.isNotBlank() } ?: resp.currentPath,
                        parentPath = resp.parentPath.orEmpty(),
                        canGoUp = resp.canGoUp,
                        recommendedDirs = resp.recommendedDirs,
                        entries = resp.entries,
                        isLoading = false,
                        error = null
                    )
                    errorMessage = null
                    conflictAlreadyClonedPath = null
                },
                onFailure = { err ->
                    dirState = dirState.copy(
                        isLoading = false,
                        error = err.message ?: "读取本机目录失败".loc
                    )
                }
            )
        }
    }

    fun createDir(input: String) {
        val client = ApiClient.defaultInstance ?: return
        dirState = dirState.copy(isCreatingDir = true, error = null)
        scope.launch {
            val isFullPath = input.startsWith("/") || input.startsWith("~") || input.contains(":\\") || input.contains(":/")
            client.createLocalDir(
                parentPath = if (isFullPath) null else dirState.currentPath,
                name = if (isFullPath) null else input,
                path = if (isFullPath) input else null
            ).fold(
                onSuccess = { resp ->
                    dirState = dirState.copy(isCreatingDir = false)
                    val created = resp.path?.takeIf { it.isNotBlank() }
                    if (created != null) {
                        loadDirs(created)
                    }
                },
                onFailure = { err ->
                    dirState = dirState.copy(
                        isCreatingDir = false,
                        error = err.message ?: "创建本地目录失败".loc
                    )
                }
            )
        }
    }

    fun finishWithProject(project: ProjectItem, repoSlug: String, alreadyExists: Boolean) {
        pollJob?.cancel()
        pollJob = null
        onProjectReady(project)
        phase = CloneUiPhase.Completed(project, repoSlug, alreadyExists)
    }

    fun startPolling(jobId: String, repoSlug: String, fallbackTarget: String) {
        pollJob?.cancel()
        pollJob = scope.launch {
            var failures = 0
            while (isActive) {
                delay(500)
                val client = ApiClient.defaultInstance ?: break
                val res = client.fetchCloneGitHubRepoStatus(jobId)
                res.fold(
                    onSuccess = { st ->
                        failures = 0
                        val target = st.targetPath?.takeIf { it.isNotBlank() }
                            ?: st.path?.takeIf { it.isNotBlank() }
                            ?: fallbackTarget
                        when (st.status) {
                            "done" -> {
                                val proj = st.project ?: ProjectItem(
                                    name = target.substringAfterLast('/').substringAfterLast('\\').ifBlank { repoSlug.substringAfterLast('/') },
                                    uri = st.folderUri ?: "file://$target",
                                    path = st.path ?: target,
                                    isWorkspace = false
                                )
                                finishWithProject(proj, st.repoSlug ?: repoSlug, st.alreadyExists)
                                return@launch
                            }
                            "failed" -> {
                                phase = CloneUiPhase.Form
                                errorMessage = st.error?.takeIf { it.isNotBlank() } ?: st.message ?: "克隆失败".loc
                                return@launch
                            }
                            "cancelled" -> {
                                phase = CloneUiPhase.Form
                                errorMessage = "已取消克隆".loc
                                return@launch
                            }
                            else -> {
                                phase = CloneUiPhase.Running(
                                    jobId = jobId,
                                    progress = st.progress.coerceIn(0, 100),
                                    message = st.message?.takeIf { it.isNotBlank() } ?: "正在克隆仓库…".loc,
                                    targetPath = target
                                )
                            }
                        }
                    },
                    onFailure = { err ->
                        failures += 1
                        if (failures >= 6) {
                            phase = CloneUiPhase.Form
                            errorMessage = "查询克隆进度失败: ${err.message}".loc
                            return@launch
                        }
                    }
                )
            }
        }
    }

    fun startClone(importExisting: Boolean) {
        val parsed = parsedRepo ?: return
        val client = ApiClient.defaultInstance
        if (client == null) {
            errorMessage = "未配置网关地址".loc
            return
        }
        if (isSubmitting) return
        isSubmitting = true
        errorMessage = null
        conflictAlreadyClonedPath = null

        val folderArg = customFolderName.trim().takeIf { it.isNotEmpty() }
        val parentArg = dirState.currentPath

        scope.launch {
            val res = client.startCloneGitHubRepo(
                repoSlug = repoInput.trim(),
                parentDir = parentArg,
                folderName = folderArg,
                asyncMode = true,
                importExisting = importExisting,
                startSession = false
            )
            isSubmitting = false
            res.fold(
                onSuccess = { resp ->
                    when {
                        resp.code == "already_cloned" -> {
                            conflictAlreadyClonedPath = resp.existingPath ?: resp.targetPath ?: previewPath
                        }
                        resp.code == "target_exists" || resp.code == "path_is_file" -> {
                            showFolderRename = true
                            errorMessage = resp.error ?: "目标目录已存在，请更换位置或修改文件夹名".loc
                        }
                        !resp.ok -> {
                            errorMessage = resp.error ?: "克隆仓库失败".loc
                        }
                        resp.status == "done" && resp.project != null -> {
                            finishWithProject(resp.project, resp.repoSlug ?: parsed.slug, resp.alreadyExists)
                        }
                        !resp.jobId.isNullOrBlank() -> {
                            val target = resp.targetPath ?: previewPath
                            phase = CloneUiPhase.Running(
                                jobId = resp.jobId,
                                progress = resp.progress.coerceIn(5, 100),
                                message = resp.message?.takeIf { it.isNotBlank() } ?: "正在连接 GitHub 并克隆仓库…".loc,
                                targetPath = target
                            )
                            startPolling(resp.jobId, parsed.slug, target)
                        }
                        else -> {
                            errorMessage = resp.error ?: "网关未返回有效的克隆任务 ID".loc
                        }
                    }
                },
                onFailure = { err ->
                    errorMessage = "克隆仓库失败: ${err.message}".loc
                }
            )
        }
    }

    LaunchedEffect(Unit) {
        loadDirs(null)
    }

    DisposableEffect(Unit) {
        onDispose { pollJob?.cancel() }
    }

    val isRunning = phase is CloneUiPhase.Running

    Column(modifier = Modifier.fillMaxWidth().fillMaxHeight()) {
        // Subpage Header
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 12.dp)
        ) {
            if (!isRunning) {
                IconButton(
                    onClick = onBack,
                    modifier = Modifier.align(Alignment.CenterStart).size(32.dp)
                ) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = "返回".loc,
                        tint = colors.accentBlue,
                        modifier = Modifier.size(20.dp)
                    )
                }
            }
            Text(
                text = "从 GitHub 克隆仓库".loc,
                fontSize = 17.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier.align(Alignment.Center)
            )
        }

        HorizontalDivider(thickness = 0.5.dp, color = colors.border)

        when (val p = phase) {
            is CloneUiPhase.Form -> {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    // Section 1: Repo URL
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(
                            text = "GitHub 仓库地址".loc,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.textPrimary
                        )
                        OutlinedTextField(
                            value = repoInput,
                            onValueChange = {
                                repoInput = it
                                errorMessage = null
                                conflictAlreadyClonedPath = null
                            },
                            placeholder = { Text("owner/repo 或 https://github.com/...".loc, fontSize = 13.sp) },
                            singleLine = true,
                            trailingIcon = {
                                if (repoInput.isNotEmpty()) {
                                    IconButton(onClick = {
                                        repoInput = ""
                                        customFolderName = ""
                                        errorMessage = null
                                        conflictAlreadyClonedPath = null
                                    }) {
                                        Icon(Icons.Default.Clear, contentDescription = null, modifier = Modifier.size(18.dp))
                                    }
                                }
                            },
                            shape = RoundedCornerShape(12.dp),
                            modifier = Modifier.fillMaxWidth()
                        )
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            if (parsedRepo != null) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.CheckCircle,
                                        contentDescription = null,
                                        tint = colors.accentGreen,
                                        modifier = Modifier.size(14.dp)
                                    )
                                    Text(
                                        text = parsedRepo.slug,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        color = colors.accentGreen
                                    )
                                }
                                Text(
                                    text = if (showFolderRename) "收起目录名".loc else "自定义目录名".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.Medium,
                                    color = colors.accentBlue,
                                    modifier = Modifier.clickable { showFolderRename = !showFolderRename }
                                )
                            } else if (repoInput.isNotBlank()) {
                                Text(
                                    text = "支持 owner/repo、HTTPS 或 git@github.com 地址".loc,
                                    fontSize = 11.5.sp,
                                    color = colors.accentOrange
                                )
                            } else {
                                Text(
                                    text = "克隆完成后会自动加入 Antigravity 项目列表".loc,
                                    fontSize = 11.5.sp,
                                    color = colors.textSecondary
                                )
                            }
                        }

                        if (showFolderRename) {
                            OutlinedTextField(
                                value = customFolderName,
                                onValueChange = {
                                    customFolderName = it
                                    errorMessage = null
                                    conflictAlreadyClonedPath = null
                                },
                                label = { Text("目标文件夹名（默认 ${parsedRepo?.repoName ?: "仓库名"}）".loc) },
                                singleLine = true,
                                shape = RoundedCornerShape(10.dp),
                                modifier = Modifier.fillMaxWidth()
                            )
                        }
                    }

                    // Section 2: Local Directory Browser
                    LocalDirectoryBrowserSection(
                        state = dirState,
                        sectionTitle = "选择本机存放目录".loc,
                        subtitle = "可从已有项目目录选择，浏览子目录，或新建本地目录".loc,
                        onLoadPath = { loadDirs(it) },
                        onCreateDir = { createDir(it) }
                    )

                    // Section 3: Final path preview
                    if (previewPath.isNotBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.accentBlue.copy(alpha = 0.08f))
                                .padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Text(
                                text = "克隆目标路径".loc,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textSecondary
                            )
                            Text(
                                text = previewPath,
                                fontSize = 12.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary
                            )
                        }
                    }

                    // Conflict already_cloned card
                    if (conflictAlreadyClonedPath != null) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.accentOrange.copy(alpha = 0.12f))
                                .padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                Text(
                                    text = "该目录已存在同一 GitHub 仓库".loc,
                                    fontSize = 13.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textPrimary
                                )
                                Text(
                                    text = conflictAlreadyClonedPath.orEmpty(),
                                    fontSize = 11.5.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                            }
                            Button(
                                onClick = { startClone(importExisting = true) },
                                enabled = !isSubmitting,
                                colors = ButtonDefaults.buttonColors(containerColor = colors.accentOrange),
                                shape = RoundedCornerShape(10.dp),
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text("直接导入到 Antigravity 项目列表".loc, color = Color.White, fontWeight = FontWeight.SemiBold)
                            }
                        }
                    } else if (errorMessage != null) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.accentOrange.copy(alpha = 0.12f))
                                .padding(12.dp),
                            verticalAlignment = Alignment.Top,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Warning,
                                contentDescription = null,
                                tint = colors.accentOrange,
                                modifier = Modifier.size(16.dp)
                            )
                            Text(
                                text = errorMessage.orEmpty(),
                                fontSize = 12.5.sp,
                                color = colors.textPrimary
                            )
                        }
                    }
                }

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                val canSubmit = parsedRepo != null && dirState.currentPath.isNotBlank() && effectiveFolderName.isNotBlank() && !isSubmitting
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(
                            start = 16.dp,
                            end = 16.dp,
                            top = 12.dp,
                            bottom = 14.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                        )
                ) {
                    Button(
                        onClick = { startClone(importExisting = false) },
                        enabled = canSubmit,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = colors.accentBlue,
                            disabledContainerColor = colors.textMuted.copy(alpha = 0.25f)
                        ),
                        shape = RoundedCornerShape(13.dp),
                        modifier = Modifier.fillMaxWidth().height(48.dp)
                    ) {
                        if (isSubmitting) {
                            CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = Color.White)
                            Spacer(modifier = Modifier.width(8.dp))
                            Text("正在启动克隆…".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                        } else {
                            Icon(Icons.Default.Download, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
                            Spacer(modifier = Modifier.width(8.dp))
                            Text("确认并克隆".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                        }
                    }
                }
            }

            is CloneUiPhase.Running -> {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(horizontal = 24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(68.dp)
                            .clip(CircleShape)
                            .background(colors.accentBlue.copy(alpha = 0.12f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Download,
                            contentDescription = null,
                            tint = colors.accentBlue,
                            modifier = Modifier.size(30.dp)
                        )
                    }
                    Spacer(modifier = Modifier.height(18.dp))
                    Text(
                        text = parsedRepo?.slug ?: repoInput,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(6.dp))
                    Text(
                        text = p.message,
                        fontSize = 13.5.sp,
                        color = colors.textSecondary,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(20.dp))
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surfaceVariant)
                            .padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        LinearProgressIndicator(
                            progress = { p.progress / 100f },
                            modifier = Modifier.fillMaxWidth().height(6.dp).clip(CircleShape),
                            color = colors.accentBlue,
                            trackColor = colors.border
                        )
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                text = p.targetPath,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textSecondary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                            Spacer(modifier = Modifier.width(8.dp))
                            Text(
                                text = "${p.progress}%",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold,
                                fontFamily = FontFamily.Monospace,
                                color = colors.accentBlue
                            )
                        }
                    }
                    Spacer(modifier = Modifier.height(22.dp))
                    TextButton(
                        onClick = {
                            pollJob?.cancel()
                            pollJob = null
                            phase = CloneUiPhase.Form
                            errorMessage = "已取消克隆".loc
                            scope.launch {
                                ApiClient.defaultInstance?.cancelCloneGitHubRepo(p.jobId)
                            }
                        },
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(colors.accentRed.copy(alpha = 0.1f))
                            .padding(horizontal = 14.dp)
                    ) {
                        Text("取消克隆".loc, color = colors.accentRed, fontWeight = FontWeight.SemiBold)
                    }
                }
            }

            is CloneUiPhase.Completed -> {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(
                            start = 20.dp,
                            end = 20.dp,
                            top = 24.dp,
                            bottom = 20.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                        ),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Spacer(modifier = Modifier.weight(1f))
                    Box(
                        modifier = Modifier
                            .size(70.dp)
                            .clip(CircleShape)
                            .background(colors.accentGreen.copy(alpha = 0.14f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.CheckCircle,
                            contentDescription = null,
                            tint = colors.accentGreen,
                            modifier = Modifier.size(38.dp)
                        )
                    }
                    Spacer(modifier = Modifier.height(16.dp))
                    Text(
                        text = if (p.alreadyExists) "已导入 Antigravity 项目列表".loc else "克隆完成，已加入项目列表".loc,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(6.dp))
                    Text(
                        text = "现在可以在「新建会话」工作区列表中随时选用该项目".loc,
                        fontSize = 13.sp,
                        color = colors.textSecondary,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(20.dp))
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surfaceVariant)
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(40.dp)
                                    .clip(CircleShape)
                                    .background(colors.accentBlue.copy(alpha = 0.14f)),
                                contentAlignment = Alignment.Center
                            ) {
                                Icon(Icons.Default.Folder, contentDescription = null, tint = colors.accentBlue, modifier = Modifier.size(20.dp))
                            }
                            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Text(p.project.displayName, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, color = colors.textPrimary)
                                if (p.repoSlug.isNotBlank()) {
                                    Text(p.repoSlug, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = colors.textSecondary)
                                }
                            }
                        }
                        HorizontalDivider(thickness = 0.5.dp, color = colors.border)
                        Text(p.project.path, fontSize = 11.5.sp, fontFamily = FontFamily.Monospace, color = colors.textSecondary)
                    }
                    Spacer(modifier = Modifier.weight(1f))
                    Button(
                        onClick = { onStartConversation(p.project) },
                        colors = ButtonDefaults.buttonColors(containerColor = colors.accentBlue),
                        shape = RoundedCornerShape(13.dp),
                        modifier = Modifier.fillMaxWidth().height(48.dp)
                    ) {
                        Text("立即开启新会话".loc, fontSize = 15.5.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                    }
                    Spacer(modifier = Modifier.height(10.dp))
                    OutlinedButton(
                        onClick = onBack,
                        shape = RoundedCornerShape(13.dp),
                        modifier = Modifier.fillMaxWidth().height(46.dp)
                    ) {
                        Text("返回工作区列表".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = colors.textPrimary)
                    }
                }
            }
        }
    }
}

@Composable
internal fun AddLocalWorkspaceSubView(
    onBack: () -> Unit,
    onProjectReady: (ProjectItem) -> Unit,
    onStartConversation: (ProjectItem) -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()

    var dirState by remember { mutableStateOf(LocalDirBrowserState(isLoading = true)) }
    var isSubmitting by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var showManualDialog by remember { mutableStateOf(false) }
    var manualInput by remember { mutableStateOf("") }
    var completedProject by remember { mutableStateOf<ProjectItem?>(null) }
    var completedAlreadyExists by remember { mutableStateOf(false) }

    fun loadDirs(path: String?) {
        val client = ApiClient.defaultInstance
        if (client == null) {
            dirState = dirState.copy(isLoading = false, error = "未配置网关地址".loc)
            return
        }
        dirState = dirState.copy(isLoading = true, error = null)
        scope.launch {
            client.fetchLocalDirs(path).fold(
                onSuccess = { resp ->
                    dirState = dirState.copy(
                        currentPath = resp.currentPath,
                        currentDisplay = resp.currentDisplay?.takeIf { it.isNotBlank() } ?: resp.currentPath,
                        parentPath = resp.parentPath.orEmpty(),
                        canGoUp = resp.canGoUp,
                        recommendedDirs = resp.recommendedDirs,
                        entries = resp.entries,
                        isLoading = false,
                        error = null
                    )
                    errorMessage = null
                },
                onFailure = { err ->
                    dirState = dirState.copy(isLoading = false, error = err.message ?: "读取本机目录失败".loc)
                }
            )
        }
    }

    fun createDir(input: String) {
        val client = ApiClient.defaultInstance ?: return
        dirState = dirState.copy(isCreatingDir = true, error = null)
        scope.launch {
            val isFullPath = input.startsWith("/") || input.startsWith("~") || input.contains(":\\") || input.contains(":/")
            client.createLocalDir(
                parentPath = if (isFullPath) null else dirState.currentPath,
                name = if (isFullPath) null else input,
                path = if (isFullPath) input else null
            ).fold(
                onSuccess = { resp ->
                    dirState = dirState.copy(isCreatingDir = false)
                    resp.path?.takeIf { it.isNotBlank() }?.let { loadDirs(it) }
                },
                onFailure = { err ->
                    dirState = dirState.copy(isCreatingDir = false, error = err.message ?: "创建本地目录失败".loc)
                }
            )
        }
    }

    fun addWorkspace(path: String) {
        val trimmed = path.trim()
        val client = ApiClient.defaultInstance
        if (client == null) {
            errorMessage = "未配置网关地址".loc
            return
        }
        if (trimmed.isEmpty() || isSubmitting) return
        isSubmitting = true
        errorMessage = null
        scope.launch {
            val res = client.createProjectWorkspace(path = trimmed, startSession = false)
            isSubmitting = false
            res.fold(
                onSuccess = { resp ->
                    val proj = resp.project ?: ProjectItem(
                        name = trimmed.substringAfterLast('/').substringAfterLast('\\').ifBlank { trimmed },
                        uri = "file://$trimmed",
                        path = trimmed,
                        isWorkspace = false
                    )
                    onProjectReady(proj)
                    completedAlreadyExists = resp.alreadyExists
                    completedProject = proj
                },
                onFailure = { err ->
                    errorMessage = err.message ?: "添加工作区失败".loc
                }
            )
        }
    }

    LaunchedEffect(Unit) {
        loadDirs(null)
    }

    Column(modifier = Modifier.fillMaxWidth().fillMaxHeight()) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 12.dp)
        ) {
            IconButton(
                onClick = onBack,
                modifier = Modifier.align(Alignment.CenterStart).size(32.dp)
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "返回".loc,
                    tint = colors.accentBlue,
                    modifier = Modifier.size(20.dp)
                )
            }
            Text(
                text = "添加本地工作区".loc,
                fontSize = 17.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier.align(Alignment.Center)
            )
            if (completedProject == null) {
                Text(
                    text = "手输路径".loc,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.accentBlue,
                    modifier = Modifier
                        .align(Alignment.CenterEnd)
                        .clickable {
                            manualInput = dirState.currentPath
                            showManualDialog = true
                        }
                        .padding(horizontal = 4.dp, vertical = 4.dp)
                )
            }
        }

        HorizontalDivider(thickness = 0.5.dp, color = colors.border)

        val doneProj = completedProject
        if (doneProj != null) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
                    .padding(
                        start = 20.dp,
                        end = 20.dp,
                        top = 24.dp,
                        bottom = 20.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                    ),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Spacer(modifier = Modifier.weight(1f))
                Box(
                    modifier = Modifier
                        .size(70.dp)
                        .clip(CircleShape)
                        .background(colors.accentGreen.copy(alpha = 0.14f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(Icons.Default.CheckCircle, contentDescription = null, tint = colors.accentGreen, modifier = Modifier.size(38.dp))
                }
                Spacer(modifier = Modifier.height(16.dp))
                Text(
                    text = if (completedAlreadyExists) "工作区已在项目列表中".loc else "已添加到 Antigravity 项目列表".loc,
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center
                )
                Spacer(modifier = Modifier.height(8.dp))
                Text(
                    text = doneProj.path,
                    fontSize = 12.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary,
                    textAlign = TextAlign.Center
                )
                Spacer(modifier = Modifier.weight(1f))
                Button(
                    onClick = { onStartConversation(doneProj) },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentBlue),
                    shape = RoundedCornerShape(13.dp),
                    modifier = Modifier.fillMaxWidth().height(48.dp)
                ) {
                    Text("立即开启新会话".loc, fontSize = 15.5.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                }
                Spacer(modifier = Modifier.height(10.dp))
                OutlinedButton(
                    onClick = onBack,
                    shape = RoundedCornerShape(13.dp),
                    modifier = Modifier.fillMaxWidth().height(46.dp)
                ) {
                    Text("返回工作区列表".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = colors.textPrimary)
                }
            }
        } else {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                LocalDirectoryBrowserSection(
                    state = dirState,
                    sectionTitle = "浏览或新建本机文件夹".loc,
                    subtitle = "进入目标目录后点击底部按钮，即可将其加入项目列表".loc,
                    onLoadPath = { loadDirs(it) },
                    onCreateDir = { createDir(it) }
                )

                if (dirState.currentPath.isNotBlank()) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.accentBlue.copy(alpha = 0.08f))
                            .padding(12.dp),
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text("当前选定目录".loc, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = colors.textSecondary)
                        Text(dirState.currentPath, fontSize = 12.5.sp, fontFamily = FontFamily.Monospace, color = colors.textPrimary)
                    }
                }

                if (errorMessage != null) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.accentOrange.copy(alpha = 0.12f))
                            .padding(12.dp),
                        verticalAlignment = Alignment.Top,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(Icons.Default.Warning, contentDescription = null, tint = colors.accentOrange, modifier = Modifier.size(16.dp))
                        Text(errorMessage.orEmpty(), fontSize = 12.5.sp, color = colors.textPrimary)
                    }
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(
                        start = 16.dp,
                        end = 16.dp,
                        top = 12.dp,
                        bottom = 14.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                    )
            ) {
                Button(
                    onClick = { addWorkspace(dirState.currentPath) },
                    enabled = dirState.currentPath.isNotBlank() && !isSubmitting,
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.accentBlue,
                        disabledContainerColor = colors.textMuted.copy(alpha = 0.25f)
                    ),
                    shape = RoundedCornerShape(13.dp),
                    modifier = Modifier.fillMaxWidth().height(48.dp)
                ) {
                    if (isSubmitting) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = Color.White)
                        Spacer(modifier = Modifier.width(8.dp))
                        Text("正在添加工作区…".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                    } else {
                        Icon(Icons.Default.Add, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
                        Spacer(modifier = Modifier.width(8.dp))
                        Text("添加当前目录为工作区".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Color.White)
                    }
                }
            }
        }
    }

    if (showManualDialog) {
        AlertDialog(
            onDismissRequest = { showManualDialog = false },
            title = { Text("输入本机目录路径".loc) },
            text = {
                OutlinedTextField(
                    value = manualInput,
                    onValueChange = { manualInput = it },
                    label = { Text("~/Projects/my-app 或 /Users/...".loc) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val p = manualInput.trim()
                    showManualDialog = false
                    if (p.isNotEmpty()) addWorkspace(p)
                }) { Text("直接添加".loc) }
            },
            dismissButton = {
                TextButton(onClick = {
                    val p = manualInput.trim()
                    showManualDialog = false
                    if (p.isNotEmpty()) loadDirs(p)
                }) { Text("跳转浏览".loc) }
            }
        )
    }
}
