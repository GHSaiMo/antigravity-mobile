package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChatBubbleOutline
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.SharedFile
import com.antigravity.mobile.ui.theme.AntigravityTheme

/**
 * "投递到…" sheet shown when files are shared into the app from outside.
 * Files are never sent automatically: choosing a destination only drops them into that
 * conversation's input bar as attachments, so the user can add an instruction first.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ShareTargetSheet(
    files: List<SharedFile>,
    projects: List<ProjectItem>,
    conversations: List<ConversationItem>,
    currentConversationId: String?,
    onRefreshProjects: () -> Unit,
    onRemoveFile: (String) -> Unit,
    onSelectProject: (ProjectItem) -> Unit,
    onSelectConversation: (ConversationItem) -> Unit,
    onSelectCurrent: () -> Unit,
    onDiscard: () -> Unit,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var tab by remember { mutableIntStateOf(if (currentConversationId != null) 1 else 0) }
    var query by remember { mutableStateOf("") }
    val deliverable = files.count { it.isDeliverable }

    LaunchedEffect(Unit) { onRefreshProjects() }

    val currentItem = conversations.firstOrNull { it.id == currentConversationId }
    val sortedConversations = remember(conversations, query) {
        val q = query.trim().lowercase()
        conversations
            .filter { it.id != currentConversationId }
            .filter { q.isEmpty() || it.title.lowercase().contains(q) || it.workspaceName.lowercase().contains(q) }
            .sortedWith(compareByDescending<ConversationItem> { it.status.isRunning }.thenByDescending { it.lastModifiedEpochMs })
            .take(50)
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = colors.surface,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.fillMaxWidth().fillMaxHeight(0.88f)) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    "投递 $deliverable 个文件到…",
                    fontSize = 18.sp, fontWeight = FontWeight.Bold, color = colors.textPrimary,
                    modifier = Modifier.weight(1f)
                )
                TextButton(onClick = onDiscard) { Text("全部丢弃", color = colors.accentRed, fontSize = 13.sp) }
            }

            // Incoming files; unsupported ones stay visible with the reason.
            LazyColumn(
                modifier = Modifier.fillMaxWidth().heightIn(max = 168.dp),
                contentPadding = PaddingValues(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                items(files, key = { it.id }) { f ->
                    Row(
                        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant).padding(horizontal = 10.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        FileTypeBadge(f.name, 32.dp)
                        Column(modifier = Modifier.weight(1f)) {
                            Text(f.name, fontSize = 13.sp, color = colors.textPrimary, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            Text(
                                f.rejectReason ?: formatFileSize(f.size),
                                fontSize = 11.sp,
                                color = if (f.rejectReason != null) colors.accentRed else colors.textSecondary,
                                maxLines = 2, overflow = TextOverflow.Ellipsis
                            )
                        }
                        IconButton(onClick = { onRemoveFile(f.id) }, modifier = Modifier.size(28.dp)) {
                            Icon(Icons.Default.Close, contentDescription = "移除", tint = colors.textMuted, modifier = Modifier.size(16.dp))
                        }
                    }
                }
            }

            Spacer(Modifier.height(8.dp))
            TabRow(
                selectedTabIndex = tab,
                containerColor = Color.Transparent,
                contentColor = colors.accentIndigo
            ) {
                Tab(selected = tab == 0, onClick = { tab = 0 }, text = { Text("新对话") })
                Tab(selected = tab == 1, onClick = { tab = 1 }, text = { Text("现有对话") })
            }

            if (deliverable == 0) {
                Text(
                    "没有可投递的文件", color = colors.textSecondary, fontSize = 13.sp,
                    textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(24.dp)
                )
            } else if (tab == 0) {
                LazyColumn(
                    modifier = Modifier.fillMaxWidth().weight(1f),
                    contentPadding = PaddingValues(16.dp, 10.dp, 16.dp, 24.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    item(key = "chat_pure") {
                        TargetRow(Icons.Default.ChatBubbleOutline, "新对话", "不关联任何工作区", colors.accentIndigo) {
                            onSelectProject(ProjectItem.PURE_CHAT)
                        }
                    }
                    items(projects.filter { !it.isPureChat }, key = { it.id }) { p ->
                        TargetRow(Icons.Default.Folder, p.displayName, p.path.ifBlank { p.uri }, colors.accentBlue) {
                            onSelectProject(p)
                        }
                    }
                }
            } else {
                OutlinedTextField(
                    value = query, onValueChange = { query = it }, singleLine = true,
                    placeholder = { Text("搜索会话或工作区", fontSize = 13.sp) },
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)
                )
                LazyColumn(
                    modifier = Modifier.fillMaxWidth().weight(1f),
                    contentPadding = PaddingValues(16.dp, 2.dp, 16.dp, 24.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    if (currentConversationId != null && query.isBlank()) {
                        item(key = "current") {
                            TargetRow(
                                Icons.Default.ChatBubbleOutline,
                                "当前对话",
                                currentItem?.displayTitle ?: "正在查看的会话",
                                colors.accentGreen,
                                onSelectCurrent
                            )
                        }
                    }
                    items(sortedConversations, key = { it.id }) { c ->
                        TargetRow(
                            Icons.Default.ChatBubbleOutline,
                            c.displayTitle,
                            listOfNotNull(
                                c.workspaceName.takeIf { it.isNotBlank() },
                                if (c.status.isRunning) "运行中" else c.relativeTimeString.takeIf { it.isNotBlank() }
                            ).joinToString(" · "),
                            colors.accentIndigo
                        ) { onSelectConversation(c) }
                    }
                }
            }
        }
    }
}

@Composable
private fun TargetRow(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    title: String,
    subtitle: String,
    tint: Color,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(14.dp))
            .background(colors.surfaceVariant).clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        Box(
            modifier = Modifier.size(38.dp).clip(CircleShape).background(tint.copy(alpha = 0.14f)),
            contentAlignment = Alignment.Center
        ) { Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(19.dp)) }
        Column(modifier = Modifier.weight(1f)) {
            Text(title, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = colors.textPrimary, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (subtitle.isNotBlank()) {
                Text(subtitle, fontSize = 12.sp, color = colors.textSecondary, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}
