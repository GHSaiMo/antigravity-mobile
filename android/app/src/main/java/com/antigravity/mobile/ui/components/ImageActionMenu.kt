package com.antigravity.mobile.ui.components

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.platform.LocalContext
import com.antigravity.mobile.ui.util.ShareImageUtils
import kotlinx.coroutines.launch

/**
 * 聊天流内图片的统一长按菜单：查看大图 / 保存到相册 / 分享图片 / 拷贝图片。
 * 与 iOS `ImageContextMenu` 对齐。
 */
@Composable
internal fun ImageActionMenu(
    expanded: Boolean,
    onDismiss: () -> Unit,
    item: ImageViewerItem,
    onOpen: () -> Unit
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    DropdownMenu(expanded = expanded, onDismissRequest = onDismiss) {
        DropdownMenuItem(
            text = { Text("查看大图") },
            leadingIcon = { Icon(Icons.Default.Fullscreen, contentDescription = "查看大图") },
            onClick = {
                onDismiss()
                onOpen()
            }
        )
        DropdownMenuItem(
            text = { Text("保存到相册") },
            leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "保存到相册") },
            onClick = {
                onDismiss()
                scope.launch { ShareImageUtils.saveBitmapToGallery(context, ShareImageUtils.resolveBitmap(context, item)) }
            }
        )
        DropdownMenuItem(
            text = { Text("分享图片") },
            leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享图片") },
            onClick = {
                onDismiss()
                scope.launch {
                    ShareImageUtils.shareBitmap(context, ShareImageUtils.resolveBitmap(context, item), fallbackUrl = item.url)
                }
            }
        )
        DropdownMenuItem(
            text = { Text("拷贝图片") },
            leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "拷贝图片") },
            onClick = {
                onDismiss()
                scope.launch { ShareImageUtils.copyBitmapToClipboard(context, ShareImageUtils.resolveBitmap(context, item)) }
            }
        )
    }
}
