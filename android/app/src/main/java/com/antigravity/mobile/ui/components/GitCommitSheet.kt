package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.GitFileStatus
import com.antigravity.mobile.data.model.GitStatusResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.GitSheetState

/** 默认提交信息：只列文件名，让用户在此基础上改写；真正有语义的信息交给用户或 Agent。 */
internal fun suggestCommitMessage(files: List<GitFileStatus>): String {
    if (files.isEmpty()) return ""
    val names = files.map { it.path.substringAfterLast('/') }
    return if (names.size <= 3) "update ${names.joinToString(", ")}" else "update ${names.size} files"
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GitCommitSheet(
    state: GitSheetState,
    onRefresh: () -> Unit,
    onCommit: (message: String, paths: List<String>, push: Boolean) -> Unit,
    onRetryPush: () -> Unit,
    onDelegateToAgent: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val status = state.status

    // 以文件路径为键保存勾选；状态刷新后保留仍存在的勾选，新增文件默认勾选。
    var unchecked by remember { mutableStateOf(setOf<String>()) }
    var message by remember { mutableStateOf("") }
    var messageEdited by remember { mutableStateOf(false) }

    val selectedPaths = status?.files?.map { it.path }?.filter { it !in unchecked }.orEmpty()
    LaunchedEffect(status?.files, unchecked) {
        if (!messageEdited && status != null) {
            message = suggestCommitMessage(status.files.filter { it.path !in unchecked })
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = colors.surface,
        contentColor = colors.textPrimary,
        dragHandle = { BottomSheetDefaults.DragHandle() },
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 20.dp)
                .padding(bottom = 24.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text("提交改动", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = colors.textPrimary)
                IconButton(onClick = onRefresh, enabled = !state.isLoading && !state.isWorking, modifier = Modifier.size(32.dp)) {
                    Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                }
            }

            val result = state.result
            when {
                state.isLoading && status == null -> LoadingRow("正在读取 Git 状态...")
                state.error != null && status == null -> {
                    NoticeRow(state.error, isError = true)
                    Spacer(Modifier.height(12.dp))
                    TextButton(onClick = onDelegateToAgent) { Text("改为让 Agent 提交", color = colors.accentIndigo) }
                }
                result != null && result.committed -> CommitDone(result.commitId, result.pushed, result.pushError, state, onRetryPush, onDismiss)
                status != null -> {
                    BranchHeader(status)
                    Spacer(Modifier.height(10.dp))
                    if (status.clean) {
                        NoticeRow("工作区没有可提交的改动", isError = false)
                        if (status.ahead > 0 && status.upstream != null) {
                            Spacer(Modifier.height(12.dp))
                            Button(
                                onClick = onRetryPush,
                                enabled = !state.isWorking,
                                modifier = Modifier.fillMaxWidth(),
                                shape = RoundedCornerShape(12.dp)
                            ) { Text(if (state.isWorking) "推送中..." else "推送 ${status.ahead} 个未推送的提交") }
                        }
                        state.actionError?.let { Spacer(Modifier.height(8.dp)); NoticeRow(it, isError = true) }
                    } else {
                        LazyColumn(
                            modifier = Modifier.fillMaxWidth().heightIn(max = 260.dp),
                            verticalArrangement = Arrangement.spacedBy(2.dp)
                        ) {
                            items(status.files, key = { it.path }) { f ->
                                GitFileRow(
                                    file = f,
                                    checked = f.path !in unchecked,
                                    onToggle = { unchecked = if (f.path in unchecked) unchecked - f.path else unchecked + f.path }
                                )
                            }
                        }
                        Spacer(Modifier.height(12.dp))
                        OutlinedTextField(
                            value = message,
                            onValueChange = { message = it; messageEdited = true },
                            label = { Text("提交信息") },
                            minLines = 2,
                            maxLines = 5,
                            modifier = Modifier.fillMaxWidth(),
                            shape = RoundedCornerShape(12.dp)
                        )
                        state.actionError?.let { Spacer(Modifier.height(8.dp)); NoticeRow(it, isError = true) }
                        Spacer(Modifier.height(12.dp))
                        val canCommit = !state.isWorking && message.isNotBlank() && selectedPaths.isNotEmpty()
                        // 全选时传空列表 = 提交全部（含之后新出现的），部分选择时才传明确路径
                        val pathsArg = if (selectedPaths.size == status.files.size) emptyList() else selectedPaths
                        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.fillMaxWidth()) {
                            OutlinedButton(
                                onClick = { onCommit(message, pathsArg, false) },
                                enabled = canCommit,
                                modifier = Modifier.weight(1f),
                                shape = RoundedCornerShape(12.dp)
                            ) { Text("提交") }
                            Button(
                                onClick = { onCommit(message, pathsArg, true) },
                                enabled = canCommit,
                                modifier = Modifier.weight(1f),
                                shape = RoundedCornerShape(12.dp)
                            ) { Text(if (state.isWorking) "处理中..." else "提交并推送") }
                        }
                        TextButton(onClick = onDelegateToAgent, enabled = !state.isWorking, modifier = Modifier.align(Alignment.CenterHorizontally)) {
                            Text("让 Agent 写提交信息并提交", fontSize = 12.5.sp, color = colors.accentIndigo)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun BranchHeader(status: GitStatusResponse) {
    val colors = AntigravityTheme.colors
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(status.repoName, fontSize = 13.sp, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
        Box(
            modifier = Modifier
                .clip(RoundedCornerShape(6.dp))
                .background(colors.accentIndigo.copy(alpha = 0.14f))
                .padding(horizontal = 8.dp, vertical = 2.dp)
        ) {
            Text(status.branch, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = colors.accentIndigo, maxLines = 1)
        }
        if (status.ahead > 0) Text("↑${status.ahead}", fontSize = 12.sp, color = colors.textSecondary, fontFamily = FontFamily.Monospace)
        if (status.behind > 0) Text("↓${status.behind}", fontSize = 12.sp, color = colors.accentOrange, fontFamily = FontFamily.Monospace)
        if (status.upstream == null && !status.detached) Text("未设置上游", fontSize = 11.sp, color = colors.textMuted)
    }
}

@Composable
private fun GitFileRow(file: GitFileStatus, checked: Boolean, onToggle: () -> Unit) {
    val colors = AntigravityTheme.colors
    val (label, tint) = when (file.status) {
        "ADDED" -> "A" to Color(0xFF4CAF50)
        "UNTRACKED" -> "U" to Color(0xFF4CAF50)
        "DELETED" -> "D" to Color(0xFFF44336)
        "RENAMED" -> "R" to Color(0xFF2196F3)
        "CONFLICT" -> "!" to Color(0xFFF44336)
        else -> "M" to Color(0xFFFF9800)
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable(onClick = onToggle)
            .padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Checkbox(checked = checked, onCheckedChange = { onToggle() })
        Text(label, fontSize = 12.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, color = tint, modifier = Modifier.width(18.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(file.path.substringAfterLast('/'), fontSize = 13.5.sp, color = colors.textPrimary, maxLines = 1, overflow = TextOverflow.Ellipsis)
            val dir = file.path.substringBeforeLast('/', "")
            if (dir.isNotEmpty()) Text(dir, fontSize = 11.sp, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis, fontFamily = FontFamily.Monospace)
        }
    }
}

@Composable
private fun ColumnScope.CommitDone(
    commitId: String?,
    pushed: Boolean,
    pushError: String?,
    state: GitSheetState,
    onRetryPush: () -> Unit,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    NoticeRow(
        buildString {
            append("已提交")
            if (!commitId.isNullOrBlank()) append(" $commitId")
            if (pushed) append("，已推送")
        },
        isError = false
    )
    val err = pushError ?: state.actionError
    if (err != null) {
        Spacer(Modifier.height(8.dp))
        NoticeRow("推送失败：$err", isError = true)
        Spacer(Modifier.height(8.dp))
        OutlinedButton(onClick = onRetryPush, enabled = !state.isWorking, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth()) {
            Text(if (state.isWorking) "推送中..." else "重试推送")
        }
    }
    Spacer(Modifier.height(8.dp))
    TextButton(onClick = onDismiss, modifier = Modifier.align(Alignment.CenterHorizontally)) {
        Text("完成", color = colors.accentIndigo)
    }
}

@Composable
private fun LoadingRow(text: String) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 24.dp),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically
    ) {
        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
        Spacer(Modifier.width(10.dp))
        Text(text, fontSize = 13.sp, color = colors.textMuted)
    }
}

@Composable
private fun NoticeRow(text: String, isError: Boolean) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant)
            .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Icon(
            imageVector = if (isError) Icons.Default.ErrorOutline else Icons.Default.CheckCircle,
            contentDescription = null,
            tint = if (isError) colors.accentOrange else Color(0xFF4CAF50),
            modifier = Modifier.size(18.dp)
        )
        Text(text, fontSize = 13.sp, color = colors.textSecondary)
    }
}
