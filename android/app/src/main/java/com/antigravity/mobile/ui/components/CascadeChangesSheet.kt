package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Difference
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.clickable
import com.antigravity.mobile.data.model.RevertPreviewFile
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChangesSheetState

/** 「本会话改动」：Agent 在当前会话里累计改动了哪些文件，点开可看正向 diff；同时内嵌工作区代码/文件检索。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CascadeChangesSheet(
    state: ChangesSheetState,
    cascadeId: String = "",
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var viewingDiffFile by remember { mutableStateOf<RevertPreviewFile?>(null) }
    var activeSegment by remember { mutableIntStateOf(0) } // 0 = 本会话改动, 1 = 搜索代码, 2 = 搜索文件

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
                // Floating grab handle hinting pull-down dismissal (matching iOS Capsule 38x5 / NewConversationSheet)
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

                // Header title: Centered, no icon, aligned with NewConversationSheet
                Text(
                    text = "本会话改动".loc,
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 12.dp)
                )

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                if (cascadeId.isNotBlank()) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 20.dp, vertical = 10.dp)
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant)
                            .padding(3.dp),
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        listOf("本会话改动".loc, "搜索代码".loc, "搜索文件".loc).forEachIndexed { idx, label ->
                            val selected = activeSegment == idx
                            Box(
                                modifier = Modifier
                                    .weight(1f)
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(if (selected) colors.surface else Color.Transparent)
                                    .clickable { activeSegment = idx }
                                    .padding(vertical = 7.dp),
                                contentAlignment = Alignment.Center
                            ) {
                                Text(
                                    text = label,
                                    fontSize = 12.5.sp,
                                    fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
                                    color = if (selected) colors.accentIndigo else colors.textSecondary,
                                    maxLines = 1
                                )
                            }
                        }
                    }
                }

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(horizontal = 20.dp)
                        .padding(top = 6.dp, bottom = 24.dp)
                ) {
                    if (cascadeId.isNotBlank() && activeSegment == 1) {
                        WorkspaceSearchTab(cascadeId = cascadeId, mode = WorkspaceSearchMode.CODE)
                    } else if (cascadeId.isNotBlank() && activeSegment == 2) {
                        WorkspaceSearchTab(cascadeId = cascadeId, mode = WorkspaceSearchMode.FILES)
                    } else {
                    val data = state.data
                    when {
                        state.isLoading -> {
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(vertical = 28.dp),
                                horizontalArrangement = Arrangement.Center,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                                Spacer(Modifier.width(10.dp))
                                Text("正在汇总代码改动...".loc, fontSize = 13.sp, color = colors.textMuted)
                            }
                        }
                        state.error != null -> {
                            ChangesNotice(state.error, isError = true)
                        }
                        data == null || !data.hasChanges -> {
                            ChangesNotice("本会话没有产生文件改动".loc, isError = false)
                        }
                        else -> {
                            Row(
                                modifier = Modifier.padding(bottom = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Text("${data.files.size} 个文件".loc, fontSize = 13.sp, color = colors.textSecondary)
                                Text("+${data.additions}", fontSize = 13.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, color = Color(0xFF4CAF50))
                                Text("-${data.deletions}", fontSize = 13.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, color = Color(0xFFF44336))
                            }
                            LazyColumn(
                                modifier = Modifier.fillMaxWidth().weight(1f),
                                verticalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                items(data.files, key = { it.fileUri.ifBlank { it.fileName } }) { file ->
                                    FileChangeItem(file = file, onViewDiff = { viewingDiffFile = file })
                                }
                            }
                        }
                    }
                    }
                }
            }
        }
    }

    viewingDiffFile?.let { file ->
        DiffViewerDialog(file = file, onDismiss = { viewingDiffFile = null })
    }
}

@Composable
private fun ChangesNotice(text: String, isError: Boolean) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 12.dp)
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant)
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
