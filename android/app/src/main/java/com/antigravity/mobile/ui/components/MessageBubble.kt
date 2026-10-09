package com.antigravity.mobile.ui.components

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Undo
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ArtifactItem
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.data.model.SubagentItem
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic

import androidx.compose.foundation.clickable
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.material.icons.filled.ContentCopy
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import android.graphics.Bitmap
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.ui.text.style.TextOverflow
import com.antigravity.mobile.data.service.AttachmentRules

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun MessageBubble(
    message: GatewayMessageItem,
    modifier: Modifier = Modifier,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((url: String?, bitmap: Bitmap?) -> Unit)? = null,
    onImageGroupClick: ((items: List<ImageViewerItem>, initialIndex: Int) -> Unit)? = null,
    onUndoClick: ((GatewayMessageItem) -> Unit)? = null,
    onExportMarkdownClick: (() -> Unit)? = null,
    onSubagentClick: ((SubagentItem) -> Unit)? = null,
    shareContextProvider: ((GatewayMessageItem) -> ShareCardContext)? = null
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val focusManager = LocalFocusManager.current

    // 子代理卡片：内联在「调用子代理」的位置（对齐桌面端）
    message.subagent?.takeIf { message.isSubagent }?.let { sa ->
        SubagentInlineCard(item = sa, onClick = onSubagentClick, modifier = modifier)
        return
    }

    // Standalone tool message
    if (message.isTools) {
        val toolText = message.effectiveText.ifBlank { "已思考并执行工具操作" }
        Box(
            modifier = modifier
                .fillMaxWidth()
                .padding(vertical = 4.dp),
            contentAlignment = Alignment.CenterStart
        ) {
            Row(
                modifier = Modifier
                    .clip(CircleShape)
                    .background(colors.surfaceVariant)
                    .border(0.8.dp, colors.border, CircleShape)
                    .padding(horizontal = 14.dp, vertical = 7.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.AutoAwesome,
                    contentDescription = "Tool",
                    tint = colors.accentOrange,
                    modifier = Modifier.size(13.dp)
                )
                Text(
                    text = toolText,
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }
        return
    }

    // Error card for execution errors
    if (message.isError) {
        val errText = message.effectiveText.ifBlank { "执行遇到错误" }
        val attemptInfo = remember(errText, message.attemptCount, message.maxAttempts) {
            val cur = message.attemptCount
            val max = message.maxAttempts
            if (cur != null && max != null && max > 0) {
                Pair(cur, max)
            } else if (cur != null && cur > 0) {
                Pair(cur, if (cur <= 9) 9 else cur)
            } else {
                val match = Regex("""(?i)\(attempt\s+(\d+)(?:\s*(?:of|/)\s*(\d+))?(?:\s*[·,]\s*[^)]*)?\)""").find(errText)
                if (match != null) {
                    val c = match.groupValues[1].toIntOrNull() ?: 1
                    val m = match.groupValues.getOrNull(2)?.toIntOrNull() ?: if (c <= 9) 9 else c
                    Pair(c, m)
                } else null
            }
        }
        val badgeText = when {
            attemptInfo != null -> "ERROR · 尝试 ${attemptInfo.first}/${attemptInfo.second}"
            else -> "ERROR"
        }
        val titleText = when {
            attemptInfo != null && attemptInfo.first > 1 -> "执行遇到错误 (已重试 ${attemptInfo.first} 次)"
            else -> "执行遇到错误"
        }

        Box(
            modifier = modifier
                .fillMaxWidth()
                .padding(vertical = 4.dp),
            contentAlignment = Alignment.CenterStart
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth(0.95f)
                    .clip(RoundedCornerShape(14.dp))
                    .background(Color(0xFFFEF2F2))
                    .border(1.dp, Color(0xFFEF4444).copy(alpha = 0.25f), RoundedCornerShape(14.dp))
                    .padding(12.dp),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(28.dp)
                        .clip(CircleShape)
                        .background(Color(0xFFEF4444).copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Warning,
                        contentDescription = "Error",
                        tint = Color(0xFFDC2626),
                        modifier = Modifier.size(15.dp)
                    )
                }

                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .clip(CircleShape)
                                .background(Color(0xFFEF4444).copy(alpha = 0.18f))
                                .padding(horizontal = 6.dp, vertical = 2.dp)
                        ) {
                            Text(
                                text = badgeText.uppercase(),
                                color = Color(0xFFDC2626),
                                fontSize = 10.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace
                            )
                        }

                        Text(
                            text = titleText,
                            color = Color(0xFFDC2626),
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                    }

                    Text(
                        text = errText,
                        color = colors.textPrimary.copy(alpha = 0.9f),
                        fontSize = 13.sp,
                        fontFamily = FontFamily.Monospace,
                        lineHeight = 18.sp
                    )
                }
            }
        }
        return
    }

    val isUser = message.isUser
    val clipboard = LocalClipboardManager.current
    val haptic = LocalHapticFeedback.current
    var copiedHintVisible by remember { mutableStateOf(false) }
    fun copyAll(text: String) {
        clipboard.setText(AnnotatedString(text))
        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
        copiedHintVisible = true
    }
    CopiedHint(visible = copiedHintVisible, onHidden = { copiedHintVisible = false })

    // 分享长图：弹窗所需的内容快照（点击菜单时写入）
    var shareCardRequest by remember { mutableStateOf<ShareCardRequest?>(null) }
    shareCardRequest?.let { request ->
        MessageShareCardSheet(
            content = request.content,
            isUserMessage = request.isUser,
            images = request.images,
            shareContext = shareContextProvider?.invoke(message) ?: ShareCardContext(),
            urlResolver = urlResolver,
            onDismiss = { shareCardRequest = null }
        )
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        horizontalAlignment = if (isUser) Alignment.End else Alignment.Start
    ) {
        // Render folded tools capsule if any
        if (!message.toolCalls.isNullOrEmpty()) {
            ToolStepCollapseCard(
                toolCalls = message.toolCalls,
                modifier = Modifier
                    .fillMaxWidth(0.95f)
                    .padding(bottom = 6.dp)
            )
        }

        // Render Reasoning / Thinking section if present
        message.reasoningContent?.takeIf { it.isNotBlank() }?.let { reasoning ->
            Column(
                modifier = Modifier
                    .fillMaxWidth(0.95f)
                    .shadow(
                        elevation = 1.dp,
                        shape = RoundedCornerShape(12.dp),
                        ambientColor = Color.Black.copy(alpha = 0.04f),
                        spotColor = Color.Black.copy(alpha = 0.06f)
                    )
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surfaceVariant.copy(alpha = 0.5f))
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(10.dp)
                    .padding(bottom = 4.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Psychology,
                        contentDescription = "Thinking",
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(14.dp)
                    )
                    Text(
                        text = "思考过程",
                        color = colors.textMuted,
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }
                Text(
                    text = reasoning.take(320) + if (reasoning.length > 320) "..." else "",
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    lineHeight = 16.sp
                )
            }
            Spacer(modifier = Modifier.height(4.dp))
        }

        // Render Message Content
        val displayText = message.effectiveText
        if (isUser) {
            // Consolidated attached user images (aligning with iOS / Web: use imageUrls as primary with bytes as thumbnail, fallback to bytes)
            val allUserImages = remember(message, urlResolver) {
                if (!message.imageUrls.isNullOrEmpty()) {
                    val list = message.imageUrls.mapIndexed { idx, rawUrl ->
                        val resolvedUrl = urlResolver?.invoke(rawUrl) ?: rawUrl
                        val bytes = message.effectiveImageDataList.getOrNull(idx)
                        ImageViewerItem(url = resolvedUrl, bytes = bytes)
                    }.toMutableList()
                    if (message.effectiveImageDataList.size > message.imageUrls.size) {
                        for (i in message.imageUrls.size until message.effectiveImageDataList.size) {
                            list.add(ImageViewerItem(bytes = message.effectiveImageDataList[i]))
                        }
                    }
                    list
                } else {
                    message.effectiveImageDataList.map { bytes ->
                        ImageViewerItem(bytes = bytes)
                    }
                }
            }

            if (allUserImages.isNotEmpty()) {
                Row(
                    modifier = Modifier
                        .padding(bottom = 6.dp)
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    allUserImages.forEachIndexed { index, item ->
                        var showImageMenu by remember { mutableStateOf(false) }
                        val openImage = {
                            if (onImageGroupClick != null) {
                                onImageGroupClick(allUserImages, index)
                            } else {
                                val fallbackBmp = item.bitmap ?: item.bytes?.let { b ->
                                    try { BitmapFactory.decodeByteArray(b, 0, b.size) } catch (_: Exception) { null }
                                }
                                onImageClick?.invoke(item.url, fallbackBmp)
                            }
                        }
                        Box(
                            modifier = Modifier
                                .size(72.dp)
                                .clip(RoundedCornerShape(12.dp))
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .combinedClickable(
                                    onClick = { openImage() },
                                    onLongClick = { showImageMenu = true }
                                )
                        ) {
                            ImageActionMenu(
                                expanded = showImageMenu,
                                onDismiss = { showImageMenu = false },
                                item = item,
                                onOpen = { openImage() }
                            )
                            if (item.bitmap != null) {
                                Image(
                                    bitmap = item.bitmap.asImageBitmap(),
                                    contentDescription = "Attached image",
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                )
                            } else if (item.bytes != null || !item.url.isNullOrBlank()) {
                                val imgUrl = item.url
                                val stableKey = if (!imgUrl.isNullOrBlank()) {
                                    if (imgUrl.contains("/api/v1/files/raw")) {
                                        imgUrl.substringAfter("/api/v1/files/raw")
                                    } else {
                                        imgUrl
                                    }
                                } else null
                                SubcomposeAsyncImage(
                                    model = ImageRequest.Builder(context)
                                        .data(item.bytes ?: item.url)
                                        .apply {
                                            if (stableKey != null) {
                                                diskCacheKey(stableKey)
                                                memoryCacheKey(stableKey)
                                            }
                                        }
                                        .crossfade(true)
                                        .build(),
                                    contentDescription = "Attached image",
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                        }
                    }
                }
            }

            // Files the user attached: the gateway appends an attachment block to the message text.
            val (userBodyText, attachedFiles) = remember(displayText) { AttachmentRules.parseBlock(displayText) }
            if (attachedFiles.isNotEmpty()) {
                Column(
                    modifier = Modifier.padding(bottom = 6.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                    horizontalAlignment = Alignment.End
                ) {
                    attachedFiles.forEach { f ->
                        Row(
                            modifier = Modifier
                                .widthIn(max = 280.dp)
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surface)
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .clickable { onPlanClick?.invoke(f.path, f.name) }
                                .padding(horizontal = 10.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            FileTypeBadge(f.name, 36.dp)
                            Column {
                                Text(
                                    text = f.name,
                                    color = colors.textPrimary,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.Medium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Text(
                                    text = "${f.ext.uppercase()} · ${f.sizeLabel}",
                                    color = colors.textMuted,
                                    fontSize = 11.sp
                                )
                            }
                        }
                    }
                }
            }

            if (userBodyText.isNotBlank()) {
                var showContextMenu by remember { mutableStateOf(false) }

                // User Bubble: Apple Indigo, white text, 18.dp continuous corner radius
                Box(
                    modifier = Modifier
                        .widthIn(max = 320.dp)
                        .shadow(
                            elevation = 1.5.dp,
                            shape = RoundedCornerShape(18.dp),
                            ambientColor = Color.Black.copy(alpha = 0.04f),
                            spotColor = Color.Black.copy(alpha = 0.10f)
                        )
                        .clip(RoundedCornerShape(18.dp))
                        .background(colors.userBubbleBg)
                        .combinedClickable(
                            onClick = { focusManager.clearFocus() },
                            onLongClick = { showContextMenu = true }
                        )
                        .padding(horizontal = 14.dp, vertical = 10.dp)
                ) {
                    Text(
                        text = userBodyText,
                        color = colors.userBubbleText,
                        fontSize = 15.5.sp,
                        lineHeight = 21.sp
                    )

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false }
                    ) {
                        DropdownMenuItem(
                            text = { Text("复制") },
                            leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制") },
                            onClick = {
                                showContextMenu = false
                                copyAll(userBodyText)
                            }
                        )
                        DropdownMenuItem(
                            text = { Text("分享为长图") },
                            leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享为长图") },
                            onClick = {
                                showContextMenu = false
                                shareCardRequest = ShareCardRequest(userBodyText, true, allUserImages)
                            }
                        )
                        if (onExportMarkdownClick != null) {
                            DropdownMenuItem(
                                text = { Text("导出 MD") },
                                leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "导出 MD") },
                                onClick = {
                                    showContextMenu = false
                                    onExportMarkdownClick()
                                }
                            )
                        }
                        // 当前 Antigravity 不支持撤回接口时，调用方传 null，菜单里就不显示这一项
                        if (onUndoClick != null) {
                            DropdownMenuItem(
                                text = { Text("撤回") },
                                leadingIcon = {
                                    Icon(
                                        imageVector = Icons.Default.Undo,
                                        contentDescription = "撤回"
                                    )
                                },
                                onClick = {
                                    showContextMenu = false
                                    onUndoClick(message)
                                }
                            )
                        }
                    }
                }
            }
        } else {
            val hasArtifacts = !message.artifacts.isNullOrEmpty()
            if (displayText.isNotBlank() || hasArtifacts) {
                // Agent Bubble: Card background, textPrimary, 18.dp radius with subtle soft shadow
                // 长按文字仍是选字；长按气泡非文字区域（边距/空白）弹出菜单
                var showAgentMenu by remember { mutableStateOf(false) }
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .shadow(
                            elevation = 1.5.dp,
                            shape = RoundedCornerShape(18.dp),
                            ambientColor = Color.Black.copy(alpha = 0.04f),
                            spotColor = Color.Black.copy(alpha = 0.08f)
                        )
                        .clip(RoundedCornerShape(18.dp))
                        .background(colors.agentBubbleBg)
                        .border(0.5.dp, colors.border, RoundedCornerShape(18.dp))
                        .combinedClickable(
                            onClick = { focusManager.clearFocus() },
                            onLongClick = { showAgentMenu = true }
                        )
                        .padding(horizontal = 14.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    if (displayText.isNotBlank()) {
                        MarkdownContentView(
                            content = displayText,
                            onPlanClick = onPlanClick,
                            urlResolver = urlResolver,
                            onImageClick = { url -> onImageClick?.invoke(url, null) }
                        )
                    }

                    DropdownMenu(
                        expanded = showAgentMenu,
                        onDismissRequest = { showAgentMenu = false }
                    ) {
                        if (displayText.isNotBlank()) {
                            DropdownMenuItem(
                                text = { Text("复制") },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制") },
                                onClick = {
                                    showAgentMenu = false
                                    val plain = MarkdownPlainText.convert(displayText)
                                    copyAll(plain.ifEmpty { displayText })
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("分享为长图") },
                                leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享为长图") },
                                onClick = {
                                    showAgentMenu = false
                                    shareCardRequest = ShareCardRequest(displayText, false, emptyList())
                                }
                            )
                            if (onExportMarkdownClick != null) {
                                DropdownMenuItem(
                                    text = { Text("导出 MD") },
                                    leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "导出 MD") },
                                    onClick = {
                                        showAgentMenu = false
                                        onExportMarkdownClick()
                                    }
                                )
                            }
                        }
                    }

                    if (hasArtifacts) {
                        message.artifacts!!.forEach { artifact ->
                            ArtifactPreviewCard(
                                artifact = artifact,
                                onClick = {
                                    onPlanClick?.invoke(artifact.uri, artifact.title)
                                }
                            )
                        }
                    }
                }
            }
        }
    }
}

/**
 * Interactive card for Markdown documents & artifacts in chat messages.
 */
@Composable
fun ArtifactPreviewCard(
    artifact: ArtifactItem,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.85f))
            .border(0.8.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(12.dp))
            .clickable {
                haptic.medium()
                onClick()
            }
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        // Document icon + title
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = "📄",
                fontSize = 15.sp
            )
            Text(
                text = artifact.title.ifBlank { "文档详情" },
                color = colors.textPrimary,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        // Summary text if present
        if (!artifact.summary.isNullOrBlank()) {
            Text(
                text = artifact.summary,
                color = colors.textSecondary,
                fontSize = 12.5.sp,
                lineHeight = 17.5.sp
            )
        }
    }
}

/**
 * Native Markdown renderer for chat bubbles delegating to full-fidelity MarkdownContentView.
 */
@Composable
fun SimpleMarkdownContent(
    text: String,
    modifier: Modifier = Modifier,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((url: String) -> Unit)? = null
) {
    MarkdownContentView(
        content = text,
        modifier = modifier,
        onPlanClick = onPlanClick,
        urlResolver = urlResolver,
        onImageClick = onImageClick
    )
}


private data class ShareCardRequest(
    val content: String,
    val isUser: Boolean,
    val images: List<ImageViewerItem>
)
