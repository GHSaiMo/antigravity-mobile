package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.ChatBubbleOutline
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.WorkOutline
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
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.ui.theme.AntigravityTheme

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NewConversationSheet(
    projects: List<ProjectItem>,
    isLoading: Boolean,
    onSelectProject: (ProjectItem, String, String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    var selectedModel by remember { mutableStateOf("gemini-3.8-flash-high") }
    var initialPrompt by remember { mutableStateOf("") }
    val colors = AntigravityTheme.colors

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            decorFitsSystemWindows = false
        )
    ) {
        Scaffold(
            topBar = {
                TopAppBar(
                    title = {
                        Text(
                            text = "新建会话",
                            color = colors.textPrimary,
                            fontSize = 18.sp,
                            fontWeight = FontWeight.Bold
                        )
                    },
                    navigationIcon = {
                        TextButton(onClick = onDismiss) {
                            Text(
                                text = "取消",
                                color = colors.accentIndigo,
                                fontSize = 16.sp
                            )
                        }
                    },
                    actions = {
                        // Model Toggle (Gemini / Claude)
                        Row(
                            modifier = Modifier
                                .padding(end = 12.dp)
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.surfaceVariant)
                                .padding(2.dp)
                        ) {
                            val isGemini = selectedModel.contains("gemini", ignoreCase = true)
                            Text(
                                text = "Gemini",
                                color = if (isGemini) colors.textPrimary else colors.textSecondary,
                                fontSize = 12.sp,
                                fontWeight = if (isGemini) FontWeight.Bold else FontWeight.Normal,
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .background(if (isGemini) colors.surface else colors.surfaceVariant)
                                    .clickable { selectedModel = "gemini-3.8-flash-high" }
                                    .padding(horizontal = 10.dp, vertical = 4.dp)
                            )
                            Text(
                                text = "Claude",
                                color = if (!isGemini) colors.textPrimary else colors.textSecondary,
                                fontSize = 12.sp,
                                fontWeight = if (!isGemini) FontWeight.Bold else FontWeight.Normal,
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .background(if (!isGemini) colors.surface else colors.surfaceVariant)
                                    .clickable { selectedModel = "claude-opus-4-6-thinking" }
                                    .padding(horizontal = 10.dp, vertical = 4.dp)
                            )
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = colors.background
                    )
                )
            },
            containerColor = colors.background,
            modifier = modifier.fillMaxSize()
        ) { innerPadding ->
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(innerPadding)
                    .padding(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                // Optional Initial Prompt
                OutlinedTextField(
                    value = initialPrompt,
                    onValueChange = { initialPrompt = it },
                    label = { Text("可选：输入初始任务指令...", color = colors.textSecondary) },
                    placeholder = { Text("例如：帮我重构登录模块...", color = colors.textMuted) },
                    maxLines = 3,
                    modifier = Modifier.fillMaxWidth(),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = colors.surface,
                        unfocusedContainerColor = colors.surface,
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border,
                        focusedTextColor = colors.textPrimary,
                        unfocusedTextColor = colors.textPrimary
                    ),
                    shape = RoundedCornerShape(12.dp)
                )

                // Subheader
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = "选择模式或工作区发起新会话",
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium
                    )
                    if (projects.isNotEmpty()) {
                        Text(
                            text = "${projects.size} 个工作区",
                            color = colors.textSecondary,
                            fontSize = 12.sp
                        )
                    }
                }

                // Full-height scrollable list
                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                    contentPadding = PaddingValues(bottom = 24.dp)
                ) {
                    // Chat card (pure conversation)
                    item {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(14.dp))
                                .background(colors.surface)
                                .border(1.dp, colors.accentIndigo.copy(alpha = 0.25f), RoundedCornerShape(14.dp))
                                .clickable { onSelectProject(ProjectItem.PURE_CHAT, initialPrompt, selectedModel) }
                                .padding(14.dp),
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

                            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Text(
                                        text = "Chat",
                                        color = colors.textPrimary,
                                        fontSize = 15.5.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                    Text(
                                        text = "新对话",
                                        color = colors.accentIndigo,
                                        fontSize = 10.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(10.dp))
                                            .background(colors.accentIndigo.copy(alpha = 0.12f))
                                            .padding(horizontal = 6.dp, vertical = 2.dp)
                                    )
                                }
                                Text(
                                    text = "新对话 · 不关联任何工作区",
                                    color = colors.textSecondary,
                                    fontSize = 11.5.sp,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }

                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                                contentDescription = null,
                                tint = colors.textMuted,
                                modifier = Modifier.size(13.dp)
                            )
                        }
                    }

                    if (isLoading && projects.isEmpty()) {
                        item {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 40.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                CircularProgressIndicator(
                                    color = colors.accentIndigo,
                                    modifier = Modifier.size(28.dp)
                                )
                                Text(
                                    text = "正在拉取工作区列表...",
                                    color = colors.textSecondary,
                                    fontSize = 13.sp
                                )
                            }
                        }
                    } else {
                        items(projects, key = { it.id }) { project ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(14.dp))
                                    .background(colors.surface)
                                    .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                                    .clickable { onSelectProject(project, initialPrompt, selectedModel) }
                                    .padding(14.dp),
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

                                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Text(
                                            text = project.name,
                                            color = colors.textPrimary,
                                            fontSize = 15.5.sp,
                                            fontWeight = FontWeight.SemiBold,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                        if (project.sessionCount > 0) {
                                            Text(
                                                text = "${project.sessionCount} 会话",
                                                color = colors.textSecondary,
                                                fontSize = 10.5.sp,
                                                fontWeight = FontWeight.SemiBold,
                                                modifier = Modifier
                                                    .clip(RoundedCornerShape(10.dp))
                                                    .background(colors.surfaceVariant)
                                                    .padding(horizontal = 6.dp, vertical = 2.dp)
                                            )
                                        }
                                    }
                                    Text(
                                        text = project.path.ifBlank { project.uri },
                                        color = colors.textSecondary,
                                        fontSize = 11.sp,
                                        fontFamily = FontFamily.Monospace,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }

                                Icon(
                                    imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                                    contentDescription = null,
                                    tint = colors.textMuted,
                                    modifier = Modifier.size(13.dp)
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
