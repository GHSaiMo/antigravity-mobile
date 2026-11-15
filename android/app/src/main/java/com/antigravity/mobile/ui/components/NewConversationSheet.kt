package com.antigravity.mobile.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ChatBubbleOutline
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.filled.WorkOutline
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathFillType
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.launch

private val GitHubOctocatIcon: ImageVector by lazy {
    val pathData = "M12.3364 2.2467C6.81143 2.2467 2.33643 6.7217 2.33643 12.2467C2.33643 16.6717 5.19893 20.4092 9.17393 21.7342C9.67393 21.8217 9.86143 21.5217 9.86143 21.2592C9.86143 21.0217 9.84893 20.2342 9.84893 19.3967C7.33643 19.8592 6.68643 18.7842 6.48643 18.2217C6.37393 17.9342 5.88643 17.0467 5.46143 16.8092C5.11143 16.6217 4.61143 16.1592 5.44893 16.1467C6.23643 16.1342 6.79893 16.8717 6.98643 17.1717C7.88643 18.6842 9.32393 18.2592 9.89893 17.9967C9.98643 17.3467 10.2489 16.9092 10.5364 16.6592C8.31143 16.4092 5.98643 15.5467 5.98643 11.7217C5.98643 10.6342 6.37393 9.7342 7.01143 9.0342C6.91143 8.7842 6.56143 7.7592 7.11143 6.3842C7.11143 6.3842 7.94893 6.1217 9.86143 7.4092C10.6614 7.1842 11.5114 7.0717 12.3614 7.0717C13.2114 7.0717 14.0614 7.1842 14.8614 7.4092C16.7739 6.1092 17.6114 6.3842 17.6114 6.3842C18.1614 7.7592 17.8114 8.7842 17.7114 9.0342C18.3489 9.7342 18.7364 10.6217 18.7364 11.7217C18.7364 15.5592 16.3989 16.4092 14.1739 16.6592C14.5364 16.9717 14.8489 17.5717 14.8489 18.5092C14.8489 19.8467 14.8364 20.9217 14.8364 21.2592C14.8364 21.5217 15.0239 21.8342 15.5239 21.7342C17.5091 21.064 19.2341 19.7882 20.4562 18.0862C21.6783 16.3843 22.3359 14.342 22.3364 12.2467C22.3364 6.7217 17.8614 2.2467 12.3364 2.2467Z"
    ImageVector.Builder(
        name = "GitHubOctocat",
        defaultWidth = 24.dp,
        defaultHeight = 24.dp,
        viewportWidth = 25f,
        viewportHeight = 24f
    ).addPath(
        pathData = PathParser().parsePathString(pathData).toNodes(),
        fill = SolidColor(Color.Black),
        pathFillType = PathFillType.EvenOdd
    ).build()
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun NewConversationSheet(
    projects: List<ProjectItem>,
    isLoading: Boolean,
    errorMessage: String? = null,
    onRefreshProjects: () -> Unit = {},
    onSelectProject: (ProjectItem) -> Unit,
    onEditProjectAlias: (ProjectItem) -> Unit = {},
    onStartWorktreeSession: ((ProjectItem) -> Unit)? = null,
    onCloneGitHubRepo: ((String) -> Unit)? = null,
    onAddLocalWorkspace: (suspend (String) -> String?)? = null,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val coroutineScope = rememberCoroutineScope()
    var longPressedProject by remember { mutableStateOf<ProjectItem?>(null) }
    var showCloneDialog by remember { mutableStateOf(false) }
    var cloneRepoSlug by remember { mutableStateOf("") }
    var showAddWorkspaceDialog by remember { mutableStateOf(false) }
    var localWorkspacePath by remember { mutableStateOf("") }
    var isAddingWorkspace by remember { mutableStateOf(false) }
    var localActionError by remember { mutableStateOf<String?>(null) }
    val displayedError = localActionError ?: errorMessage

    LaunchedEffect(Unit) {
        onRefreshProjects()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.surface, onDismiss = onDismiss) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight()
        ) {
            // Floating grab handle hinting pull-down dismissal (matching iOS Capsule 38x5)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 10.dp, bottom = 18.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            // Header: top-left GitHub Clone button + centered title + top-right Add Local Workspace button
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 12.dp)
            ) {
                if (onCloneGitHubRepo != null) {
                    IconButton(
                        onClick = { showCloneDialog = true },
                        modifier = Modifier.align(Alignment.CenterStart).size(32.dp)
                    ) {
                        Icon(
                            imageVector = GitHubOctocatIcon,
                            contentDescription = "克隆 GitHub 仓库",
                            tint = colors.accentBlue,
                            modifier = Modifier.size(19.dp)
                        )
                    }
                }
                Text(
                    text = "新建会话",
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.align(Alignment.Center)
                )
                if (onAddLocalWorkspace != null) {
                    IconButton(
                        onClick = {
                            if (!isAddingWorkspace) {
                                showAddWorkspaceDialog = true
                            }
                        },
                        enabled = !isAddingWorkspace,
                        modifier = Modifier.align(Alignment.CenterEnd).size(32.dp)
                    ) {
                        if (isAddingWorkspace) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(16.dp),
                                strokeWidth = 2.dp,
                                color = colors.accentBlue
                            )
                        } else {
                            Icon(
                                imageVector = Icons.Default.Add,
                                contentDescription = "添加本地工作区",
                                tint = colors.accentBlue,
                                modifier = Modifier.size(21.dp)
                            )
                        }
                    }
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            // Header description
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 20.dp)
                    .padding(top = 14.dp, bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(
                    text = "选择模式或工作区发起新会话",
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.textSecondary
                )
                if (projects.isNotEmpty()) {
                    Text(
                        text = "${projects.size} 个工作区",
                        fontSize = 12.sp,
                        color = colors.textSecondary
                    )
                }
            }

            // Error banner if any
            if (displayedError != null) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 6.dp)
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.accentOrange.copy(alpha = 0.12f))
                        .padding(horizontal = 14.dp, vertical = 8.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Warning,
                        contentDescription = null,
                        tint = colors.accentOrange,
                        modifier = Modifier.size(14.dp)
                    )
                    Text(
                        text = displayedError,
                        fontSize = 12.5.sp,
                        color = colors.textPrimary,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }

            // Vertical cards list
            LazyColumn(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f),
                verticalArrangement = Arrangement.spacedBy(9.dp),
                contentPadding = PaddingValues(
                    start = 16.dp,
                    end = 16.dp,
                    top = 4.dp,
                    // Bottom inset lives inside the scrolling area so the list reaches the sheet edge
                    // (also in landscape) while the last card still clears the navigation bar.
                    bottom = 24.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                )
            ) {
                // Pure Chat Card
                item(key = "chat_pure") {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surfaceVariant)
                            .border(1.dp, colors.accentIndigo.copy(alpha = 0.2f), RoundedCornerShape(14.dp))
                            .clickable { onSelectProject(ProjectItem.PURE_CHAT) }
                            .padding(horizontal = 14.dp, vertical = 12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(42.dp)
                                .clip(CircleShape)
                                .background(colors.accentIndigo.copy(alpha = 0.14f)),
                            contentAlignment = Alignment.Center
                        ) {
                            Icon(
                                imageVector = Icons.Default.ChatBubbleOutline,
                                contentDescription = "Chat",
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(20.dp)
                            )
                        }

                        Column(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(3.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = "Chat",
                                    modifier = Modifier.weight(1f, fill = false),
                                    fontSize = 15.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textPrimary,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Box(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.accentIndigo.copy(alpha = 0.12f))
                                        .padding(horizontal = 6.dp, vertical = 2.dp)
                                ) {
                                    Text(
                                        text = "新对话",
                                        fontSize = 10.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentIndigo,
                                        maxLines = 1,
                                        softWrap = false
                                    )
                                }
                            }

                            Text(
                                text = "新对话 · 不关联任何工作区",
                                fontSize = 11.5.sp,
                                color = colors.textSecondary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }

                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(12.5.dp)
                        )
                    }
                }

                if (isLoading && projects.isEmpty()) {
                    item(key = "loading_projects") {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 28.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            CircularProgressIndicator(
                                color = colors.accentIndigo,
                                modifier = Modifier.size(28.dp),
                                strokeWidth = 2.5.dp
                            )
                            Text(
                                text = "正在拉取工作区列表...",
                                fontSize = 12.5.sp,
                                color = colors.textSecondary
                            )
                        }
                    }
                } else {
                    items(projects, key = { it.id }) { project ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(14.dp))
                                .background(colors.surfaceVariant)
                                .combinedClickable(
                                    onClick = { onSelectProject(project) },
                                    onLongClick = {
                                        if (onStartWorktreeSession != null) longPressedProject = project
                                        else onEditProjectAlias(project)
                                    }
                                )
                                .padding(horizontal = 14.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(14.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(42.dp)
                                    .clip(CircleShape)
                                    .background(colors.accentOrange.copy(alpha = 0.14f)),
                                contentAlignment = Alignment.Center
                            ) {
                                Icon(
                                    imageVector = if (project.isWorkspace) Icons.Default.WorkOutline else Icons.Default.Folder,
                                    contentDescription = "Workspace",
                                    tint = colors.accentOrange,
                                    modifier = Modifier.size(20.dp)
                                )
                            }

                            Column(
                                modifier = Modifier.weight(1f),
                                verticalArrangement = Arrangement.spacedBy(3.dp)
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Text(
                                        text = project.displayName,
                                        modifier = Modifier.weight(1f, fill = false),
                                        fontSize = 15.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.textPrimary,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )

                                    if (project.sessionCount > 0) {
                                        Box(
                                            modifier = Modifier
                                                .clip(RoundedCornerShape(10.dp))
                                                .background(colors.textSecondary.copy(alpha = 0.12f))
                                                .padding(horizontal = 6.dp, vertical = 2.dp)
                                        ) {
                                            Text(
                                                text = "${project.sessionCount} 会话",
                                                fontSize = 10.5.sp,
                                                fontWeight = FontWeight.SemiBold,
                                                color = colors.textSecondary,
                                                maxLines = 1,
                                                softWrap = false
                                            )
                                        }
                                    }
                                }

                                val displayPath = project.path.ifBlank { project.uri }
                                val subtitleText = if (project.hasCustomAlias) {
                                    "${project.name} · $displayPath"
                                } else {
                                    displayPath
                                }
                                Text(
                                    text = subtitleText,
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }

                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                                contentDescription = null,
                                tint = colors.textMuted,
                                modifier = Modifier.size(12.5.dp)
                            )
                        }
                    }
                }
            }
        }
            }
    }

    longPressedProject?.let { proj ->
        AlertDialog(
            onDismissRequest = { longPressedProject = null },
            title = { Text(proj.displayName) },
            text = { Text("选择工作区操作：") },
            confirmButton = {
                TextButton(onClick = {
                    val target = proj
                    longPressedProject = null
                    onStartWorktreeSession?.invoke(target)
                }) {
                    Text("在独立 Worktree 开启会话")
                }
            },
            dismissButton = {
                TextButton(onClick = {
                    val target = proj
                    longPressedProject = null
                    onEditProjectAlias(target)
                }) {
                    Text("设置工作区备注")
                }
            }
        )
    }

    if (showCloneDialog) {
        AlertDialog(
            onDismissRequest = { showCloneDialog = false },
            title = { Text("克隆 GitHub 仓库") },
            text = {
                OutlinedTextField(
                    value = cloneRepoSlug,
                    onValueChange = { cloneRepoSlug = it },
                    label = { Text("owner/repo 或 HTTPS URL") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val slug = cloneRepoSlug.trim()
                    showCloneDialog = false
                    if (slug.isNotEmpty()) {
                        cloneRepoSlug = ""
                        onCloneGitHubRepo?.invoke(slug)
                    }
                }) { Text("克隆并开启") }
            },
            dismissButton = {
                TextButton(onClick = { showCloneDialog = false }) { Text("取消") }
            }
        )
    }

    if (showAddWorkspaceDialog) {
        AlertDialog(
            onDismissRequest = { showAddWorkspaceDialog = false },
            title = { Text("添加本地工作区") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "输入电脑上的本地目录路径，添加到工作区列表",
                        fontSize = 13.sp,
                        color = colors.textSecondary
                    )
                    OutlinedTextField(
                        value = localWorkspacePath,
                        onValueChange = { localWorkspacePath = it },
                        label = { Text("~/Projects/my-app 或 /Users/...") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    val path = localWorkspacePath.trim()
                    showAddWorkspaceDialog = false
                    if (path.isNotEmpty()) {
                        localWorkspacePath = ""
                        localActionError = null
                        isAddingWorkspace = true
                        coroutineScope.launch {
                            val err = onAddLocalWorkspace?.invoke(path)
                            isAddingWorkspace = false
                            if (!err.isNullOrBlank()) {
                                localActionError = err
                            }
                        }
                    }
                }) { Text("添加") }
            },
            dismissButton = {
                TextButton(onClick = { showAddWorkspaceDialog = false }) { Text("取消") }
            }
        )
    }
}
