package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.AttachmentFile
import com.antigravity.mobile.data.model.UploadState
import com.antigravity.mobile.data.service.FileIconResolver
import com.antigravity.mobile.ui.theme.AntigravityTheme

internal fun formatFileSize(bytes: Long): String = when {
    bytes >= 1L shl 20 -> String.format(java.util.Locale.US, "%.1f MB", bytes / (1024.0 * 1024.0))
    bytes >= 1L shl 10 -> String.format(java.util.Locale.US, "%d KB", Math.round(bytes / 1024.0))
    else -> "$bytes B"
}

/** Square badge showing the file extension on a tinted background (shared by input chips and bubbles). */
@Composable
internal fun FileTypeBadge(fileName: String, size: androidx.compose.ui.unit.Dp = 36.dp) {
    val info = FileIconResolver.resolve(fileName)
    val ext = fileName.substringAfterLast('.', "").take(4).uppercase().ifBlank { "FILE" }
    Box(
        modifier = Modifier
            .size(size)
            .clip(RoundedCornerShape(8.dp))
            .background(info.color.copy(alpha = 0.16f)),
        contentAlignment = Alignment.Center
    ) {
        Text(ext, color = info.color, fontSize = 10.sp, fontWeight = FontWeight.Bold, maxLines = 1)
    }
}

/** Chip for a file attached to the draft: type badge, name, size / upload state, remove button. */
@Composable
fun AttachmentFileChip(
    file: AttachmentFile,
    onRemove: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Box(modifier = modifier.padding(top = 4.dp, end = 6.dp)) {
        Row(
            modifier = Modifier
                .height(52.dp)
                .widthIn(max = 220.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(colors.surface)
                .border(
                    1.dp,
                    if (file.state == UploadState.FAILED) colors.accentRed else colors.border,
                    RoundedCornerShape(10.dp)
                )
                .then(if (file.state == UploadState.FAILED) Modifier.clickable { onRetry() } else Modifier)
                .padding(horizontal = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Box(contentAlignment = Alignment.Center) {
                FileTypeBadge(file.name, 36.dp)
                if (file.state == UploadState.UPLOADING || file.state == UploadState.PENDING) {
                    Box(
                        modifier = Modifier
                            .size(36.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(Color.Black.copy(alpha = 0.35f)),
                        contentAlignment = Alignment.Center
                    ) {
                        if (file.state == UploadState.UPLOADING && file.progress > 0f) {
                            CircularProgressIndicator(
                                progress = { file.progress },
                                modifier = Modifier.size(22.dp),
                                strokeWidth = 2.5.dp,
                                color = Color.White,
                                trackColor = Color.White.copy(alpha = 0.3f)
                            )
                        } else {
                            CircularProgressIndicator(
                                modifier = Modifier.size(22.dp),
                                strokeWidth = 2.5.dp,
                                color = Color.White
                            )
                        }
                    }
                }
                if (file.state == UploadState.FAILED) {
                    Box(
                        modifier = Modifier
                            .size(36.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(Color.Black.copy(alpha = 0.45f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(Icons.Default.Refresh, contentDescription = "重试上传", tint = Color.White, modifier = Modifier.size(20.dp))
                    }
                }
            }
            Column(modifier = Modifier.padding(end = 10.dp)) {
                Text(
                    text = file.name,
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Text(
                    text = when (file.state) {
                        UploadState.FAILED -> "上传失败，点按重试"
                        UploadState.DONE -> formatFileSize(file.size)
                        else -> "${formatFileSize(file.size)} · 上传中"
                    },
                    color = if (file.state == UploadState.FAILED) colors.accentRed else colors.textMuted,
                    fontSize = 11.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }
        Box(
            modifier = Modifier
                .size(20.dp)
                .align(Alignment.TopEnd)
                .offset(x = 6.dp, y = (-6).dp)
                .clip(CircleShape)
                .background(Color.Black.copy(alpha = 0.65f))
                .clickable { onRemove() },
            contentAlignment = Alignment.Center
        ) {
            Icon(Icons.Default.Close, contentDescription = "移除文件", tint = Color.White, modifier = Modifier.size(12.dp))
        }
    }
}
