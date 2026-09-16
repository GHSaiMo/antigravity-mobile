package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChatBubbleOutline
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.ui.theme.*

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

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = DarkSurface,
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp)
                .padding(bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // Header
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(
                    text = "新建会话",
                    color = TextPrimary,
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold
                )

                // Model Toggle
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(DarkBackground)
                        .padding(2.dp)
                ) {
                    val isGemini = selectedModel.contains("gemini", ignoreCase = true)
                    Text(
                        text = "Gemini",
                        color = if (isGemini) TextPrimary else TextMuted,
                        fontSize = 12.sp,
                        fontWeight = if (isGemini) FontWeight.Bold else FontWeight.Normal,
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(if (isGemini) AccentBlue else DarkBackground)
                            .clickable { selectedModel = "gemini-3.8-flash-high" }
                            .padding(horizontal = 8.dp, vertical = 4.dp)
                    )
                    Text(
                        text = "Claude",
                        color = if (!isGemini) TextPrimary else TextMuted,
                        fontSize = 12.sp,
                        fontWeight = if (!isGemini) FontWeight.Bold else FontWeight.Normal,
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(if (!isGemini) AccentYellow else DarkBackground)
                            .clickable { selectedModel = "claude-opus-4-6-thinking" }
                            .padding(horizontal = 8.dp, vertical = 4.dp)
                    )
                }
            }

            OutlinedTextField(
                value = initialPrompt,
                onValueChange = { initialPrompt = it },
                label = { Text("可选：输入初始任务指令...") },
                placeholder = { Text("例如：帮我重构登录模块...") },
                maxLines = 3,
                modifier = Modifier.fillMaxWidth(),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedContainerColor = DarkBackground,
                    unfocusedContainerColor = DarkBackground,
                    focusedBorderColor = AccentBlue,
                    unfocusedBorderColor = DarkBorder
                )
            )

            Text(
                text = "选择模式或工作区发起新会话",
                color = TextSecondary,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium
            )

            // Options List
            LazyColumn(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 300.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                // Pure Chat option
                item {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(DarkBackground)
                            .border(1.dp, DarkBorder, RoundedCornerShape(10.dp))
                            .clickable { onSelectProject(ProjectItem.PURE_CHAT, initialPrompt, selectedModel) }
                            .padding(14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.ChatBubbleOutline,
                            contentDescription = "Chat",
                            tint = AccentBlue,
                            modifier = Modifier.size(20.dp)
                        )
                        Column {
                            Text(
                                text = "Chat (通用对话模式)",
                                color = TextPrimary,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            Text(
                                text = "不关联任何本地文件目录，快速对话或解答",
                                color = TextMuted,
                                fontSize = 12.sp
                            )
                        }
                    }
                }

                // Workspace projects
                items(projects, key = { it.id }) { project ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(DarkBackground)
                            .border(1.dp, DarkBorder, RoundedCornerShape(10.dp))
                            .clickable { onSelectProject(project, initialPrompt, selectedModel) }
                            .padding(14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Folder,
                            contentDescription = "Workspace",
                            tint = AccentYellow,
                            modifier = Modifier.size(20.dp)
                        )
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = project.name,
                                color = TextPrimary,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                            Text(
                                text = project.path.ifBlank { project.uri },
                                color = TextMuted,
                                fontSize = 12.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }
            }
        }
    }
}
