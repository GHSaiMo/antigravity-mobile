package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
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
import com.antigravity.mobile.data.model.RevertDiffLine
import com.antigravity.mobile.data.model.RevertPreviewFile
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun DiffViewerDialog(
    file: RevertPreviewFile,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = modifier
                .fillMaxWidth(0.95f)
                .fillMaxHeight(0.85f)
                .clip(RoundedCornerShape(16.dp))
                .border(0.5.dp, colors.border, RoundedCornerShape(16.dp)),
            shape = RoundedCornerShape(16.dp),
            color = colors.surface
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Header
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            ActionTypeBadge(actionType = file.actionType)
                            Text(
                                text = file.fileName,
                                fontSize = 15.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        if (file.fileUri.isNotBlank()) {
                            Text(
                                text = file.fileUri,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textMuted,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }

                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        if (file.additions > 0) {
                            Text(
                                text = "+${file.additions}",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace,
                                color = Color(0xFF4CAF50)
                            )
                        }
                        if (file.deletions > 0) {
                            Text(
                                text = "-${file.deletions}",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace,
                                color = Color(0xFFF44336)
                            )
                        }
                        IconButton(
                            onClick = onDismiss,
                            modifier = Modifier.size(32.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Close,
                                contentDescription = "Close",
                                tint = colors.textSecondary,
                                modifier = Modifier.size(18.dp)
                            )
                        }
                    }
                }

                HorizontalDivider(color = colors.border, thickness = 0.5.dp)

                // Content
                if (file.diffLines.isEmpty()) {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(24.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "无行级别代码差异",
                            fontSize = 13.sp,
                            color = colors.textMuted
                        )
                    }
                } else {
                    val horizontalScrollState = rememberScrollState()
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxSize()
                            .horizontalScroll(horizontalScrollState)
                            .padding(vertical = 4.dp)
                    ) {
                        itemsIndexed(file.diffLines) { _, line ->
                            DiffLineRow(line = line)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ActionTypeBadge(
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
            .clip(RoundedCornerShape(4.dp))
            .background(bgColor)
            .padding(horizontal = 6.dp, vertical = 2.dp)
    ) {
        Text(
            text = label,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold,
            color = textColor
        )
    }
}

@Composable
private fun DiffLineRow(
    line: RevertDiffLine,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val isInsert = line.type.equals("INSERT", ignoreCase = true)
    val isDelete = line.type.equals("DELETE", ignoreCase = true)

    val bgColor = when {
        isInsert -> Color(0x264CAF50)
        isDelete -> Color(0x26F44336)
        else -> Color.Transparent
    }

    val textColor = when {
        isInsert -> Color(0xFF4CAF50)
        isDelete -> Color(0xFFF44336)
        else -> colors.textPrimary
    }

    val prefix = when {
        isInsert -> "+"
        isDelete -> "-"
        else -> " "
    }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .background(bgColor)
            .padding(horizontal = 12.dp, vertical = 2.dp),
        verticalAlignment = Alignment.Top
    ) {
        Text(
            text = prefix,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            fontWeight = FontWeight.Bold,
            color = textColor,
            modifier = Modifier.width(16.dp)
        )
        Text(
            text = if (line.text.isEmpty()) " " else line.text,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            color = textColor,
            softWrap = false
        )
    }
}
