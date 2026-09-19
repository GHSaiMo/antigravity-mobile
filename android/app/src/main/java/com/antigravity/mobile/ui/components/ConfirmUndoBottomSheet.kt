package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Undo
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
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.data.model.RevertPreviewFile
import com.antigravity.mobile.data.model.RevertPreviewResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConfirmUndoBottomSheet(
    message: GatewayMessageItem?,
    preview: RevertPreviewResponse?,
    isLoadingPreview: Boolean,
    isReverting: Boolean,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var viewingDiffFile by remember { mutableStateOf<RevertPreviewFile?>(null) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.surface,
        contentColor = colors.textPrimary,
        dragHandle = { BottomSheetDefaults.DragHandle() },
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp)
                .padding(bottom = 28.dp)
        ) {
            // Sheet Title
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(bottom = 16.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Undo,
                    contentDescription = "Undo",
                    tint = colors.accentOrange,
                    modifier = Modifier.size(24.dp)
                )
                Text(
                    text = "撤回确认",
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary
                )
            }

            // Warning / Explanation Banner
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.accentOrange.copy(alpha = 0.12f))
                    .padding(14.dp)
            ) {
                Text(
                    text = "撤回将删除此消息及后续的所有对话历史与代码修改。该句话将被放回输入框，方便您重新编辑。",
                    fontSize = 13.sp,
                    color = colors.textSecondary,
                    lineHeight = 18.sp
                )
            }

            Spacer(modifier = Modifier.height(14.dp))

            // Message text preview
            if (message != null) {
                val text = message.effectiveText
                Text(
                    text = "撤回的内容：",
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.textMuted,
                    modifier = Modifier.padding(bottom = 6.dp)
                )
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.surfaceVariant)
                        .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                        .padding(12.dp)
                ) {
                    Text(
                        text = if (text.isBlank()) "（图片附件消息）" else text,
                        fontSize = 14.sp,
                        color = colors.textPrimary,
                        maxLines = 4,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                Spacer(modifier = Modifier.height(14.dp))
            }

            // Code changes section
            if (isLoadingPreview) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 16.dp),
                    horizontalArrangement = Arrangement.Center,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
                        color = colors.accentIndigo
                    )
                    Spacer(modifier = Modifier.width(10.dp))
                    Text(
                        text = "正在计算代码变更差异...",
                        fontSize = 13.sp,
                        color = colors.textMuted
                    )
                }
            } else if (preview != null) {
                if (preview.hasCodeChanges) {
                    Text(
                        text = "代码回退变更 (${preview.files.size} 个文件)",
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        modifier = Modifier.padding(bottom = 8.dp)
                    )

                    LazyColumn(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(max = 200.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(preview.files) { file ->
                            FileChangeItem(
                                file = file,
                                onViewDiff = { viewingDiffFile = file }
                            )
                        }
                    }
                } else {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant)
                            .padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.CheckCircle,
                            contentDescription = "No changes",
                            tint = Color(0xFF4CAF50),
                            modifier = Modifier.size(18.dp)
                        )
                        Text(
                            text = "无工作区代码变更（仅回退会话与指令记录）",
                            fontSize = 13.sp,
                            color = colors.textSecondary
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(20.dp))

            // Action Buttons: Cancel and Confirm Undo
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                OutlinedButton(
                    onClick = onDismiss,
                    enabled = !isReverting,
                    modifier = Modifier
                        .weight(1f)
                        .height(46.dp),
                    shape = RoundedCornerShape(12.dp),
                    colors = ButtonDefaults.outlinedButtonColors(contentColor = colors.textPrimary)
                ) {
                    Text(text = "取消", fontSize = 15.sp, fontWeight = FontWeight.Medium)
                }

                Button(
                    onClick = onConfirm,
                    enabled = !isReverting && !isLoadingPreview,
                    modifier = Modifier
                        .weight(1f)
                        .height(46.dp),
                    shape = RoundedCornerShape(12.dp),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = Color(0xFFE53935),
                        contentColor = Color.White
                    )
                ) {
                    if (isReverting) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(16.dp),
                            strokeWidth = 2.dp,
                            color = Color.White
                        )
                        Spacer(modifier = Modifier.width(8.dp))
                        Text(text = "正在撤回...", fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    } else {
                        Text(text = "确认撤回", fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    }
                }
            }
        }
    }

    // Line-level diff dialog
    viewingDiffFile?.let { file ->
        DiffViewerDialog(
            file = file,
            onDismiss = { viewingDiffFile = null }
        )
    }
}

@Composable
private fun FileChangeItem(
    file: RevertPreviewFile,
    onViewDiff: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors

    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant)
            .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
            .padding(10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Row(
            modifier = Modifier.weight(1f),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Description,
                contentDescription = "File",
                tint = colors.accentIndigo,
                modifier = Modifier.size(18.dp)
            )
            Column {
                Text(
                    text = file.fileName,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    ActionMiniBadge(actionType = file.actionType)
                    if (file.additions > 0) {
                        Text(
                            text = "+${file.additions}",
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Bold,
                            fontFamily = FontFamily.Monospace,
                            color = Color(0xFF4CAF50)
                        )
                    }
                    if (file.deletions > 0) {
                        Text(
                            text = "-${file.deletions}",
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Bold,
                            fontFamily = FontFamily.Monospace,
                            color = Color(0xFFF44336)
                        )
                    }
                }
            }
        }

        TextButton(
            onClick = onViewDiff,
            shape = RoundedCornerShape(8.dp),
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
        ) {
            Text(
                text = "查看 Diff",
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                color = colors.accentIndigo
            )
        }
    }
}

@Composable
private fun ActionMiniBadge(
    actionType: String,
    modifier: Modifier = Modifier
) {
    val (bgColor, textColor, label) = when (actionType.uppercase()) {
        "CREATE" -> Triple(Color(0x264CAF50), Color(0xFF4CAF50), "新增")
        "DELETE" -> Triple(Color(0x26F44336), Color(0xFFF44336), "删除")
        else -> Triple(Color(0x262196F3), Color(0xFF2196F3), "修改")
    }

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(3.dp))
            .background(bgColor)
            .padding(horizontal = 4.dp, vertical = 1.dp)
    ) {
        Text(
            text = label,
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            color = textColor
        )
    }
}
