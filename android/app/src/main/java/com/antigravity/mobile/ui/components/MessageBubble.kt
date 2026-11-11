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
import androidx.compose.material.icons.filled.UnfoldLess
import androidx.compose.material.icons.filled.UnfoldMore
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
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
import com.antigravity.mobile.ui.screen.AntigravityVoiceIcon
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
import com.antigravity.mobile.data.service.FileIconResolver

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

    // 动态多智能体工作流卡片 (Antigravity 2.26+)
    message.workflow?.takeIf { message.isWorkflow }?.let { wf ->
        WorkflowStageCard(workflow = wf, modifier = modifier)
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
        // Render folded tools capsule (simplified in Knowledge Mode)
        if (!message.toolCalls.isNullOrEmpty()) {
            if (LocalKnowledgeModeEnabled.current) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth(0.95f)
                        .padding(bottom = 4.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.AutoAwesome,
                        contentDescription = null,
                        tint = colors.textSecondary,
                        modifier = Modifier.size(12.dp)
                    )
                    Text(
                        text = "已完成 ${message.toolCalls.size} 项检索与命令操作",
                        color = colors.textSecondary,
                        fontSize = 11.5.sp
                    )
                }
            } else {
                ToolStepCollapseCard(
                    toolCalls = message.toolCalls,
                    modifier = Modifier
                        .fillMaxWidth(0.95f)
                        .padding(bottom = 6.dp)
                )
            }
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
                val validImageUrls = message.imageUrls?.filter { FileIconResolver.isImage(it) }
                if (!validImageUrls.isNullOrEmpty()) {
                    val list = validImageUrls.mapIndexed { idx, rawUrl ->
                        val resolvedUrl = urlResolver?.invoke(rawUrl) ?: rawUrl
                        val bytes = message.effectiveImageDataList.getOrNull(idx)
                        ImageViewerItem(url = resolvedUrl, bytes = bytes)
                    }.toMutableList()
                    if (message.effectiveImageDataList.size > validImageUrls.size) {
                        for (i in validImageUrls.size until message.effectiveImageDataList.size) {
                            val bytes = message.effectiveImageDataList[i]
                            val bmp = try {
                                BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            } catch (_: Exception) { null }
                            if (bmp != null) {
                                list.add(ImageViewerItem(bytes = bytes, bitmap = bmp))
                            }
                        }
                    }
                    list
                } else {
                    message.effectiveImageDataList.mapNotNull { bytes ->
                        try {
                            val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            if (bmp != null) ImageViewerItem(bytes = bytes, bitmap = bmp) else null
                        } catch (_: Exception) { null }
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
            val isVoiceMessage = remember(userBodyText, attachedFiles) {
                VoiceTranscriptCleaner.isVoiceMessage(userBodyText, attachedFiles)
            }

            if (attachedFiles.isNotEmpty() && !isVoiceMessage) {
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

            if (isVoiceMessage) {
                var showContextMenu by remember { mutableStateOf(false) }
                var isVoiceExpanded by remember { mutableStateOf(false) }
                val cleanedVoiceText = remember(userBodyText) { VoiceTranscriptCleaner.clean(userBodyText) }

                Box(
                    modifier = Modifier
                        .shadow(
                            elevation = 1.5.dp,
                            shape = RoundedCornerShape(18.dp),
                            ambientColor = Color.Black.copy(alpha = 0.04f),
                            spotColor = Color.Black.copy(alpha = 0.10f)
                        )
                        .clip(RoundedCornerShape(18.dp))
                        .background(colors.userBubbleBg)
                        .combinedClickable(
                            onClick = {
                                if (cleanedVoiceText.isNotBlank()) {
                                    isVoiceExpanded = !isVoiceExpanded
                                }
                            },
                            onLongClick = { showContextMenu = true }
                        )
                        .padding(horizontal = 14.dp, vertical = 10.dp)
                ) {
                    if (!isVoiceExpanded) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            AntigravityVoiceIcon(
                                modifier = Modifier.size(18.dp),
                                tint = colors.userBubbleText
                            )
                            Text(
                                text = "语音交互",
                                color = colors.userBubbleText,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }
                    } else {
                        Column(
                            modifier = Modifier.widthIn(max = 320.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                AntigravityVoiceIcon(
                                    modifier = Modifier.size(16.dp),
                                    tint = colors.userBubbleText.copy(alpha = 0.85f)
                                )
                                Text(
                                    text = "语音交互",
                                    color = colors.userBubbleText.copy(alpha = 0.85f),
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                            Text(
                                text = cleanedVoiceText,
                                color = colors.userBubbleText,
                                fontSize = 14.5.sp,
                                lineHeight = 20.sp
                            )
                        }
                    }

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false }
                    ) {
                        if (cleanedVoiceText.isNotBlank()) {
                            DropdownMenuItem(
                                text = { Text(if (isVoiceExpanded) "折叠语音" else "展开详情") },
                                leadingIcon = {
                                    Icon(
                                        if (isVoiceExpanded) Icons.Default.UnfoldLess else Icons.Default.UnfoldMore,
                                        contentDescription = null
                                    )
                                },
                                onClick = {
                                    showContextMenu = false
                                    isVoiceExpanded = !isVoiceExpanded
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("复制语音内容") },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制语音内容") },
                                onClick = {
                                    showContextMenu = false
                                    copyAll(cleanedVoiceText)
                                }
                            )
                        }
                        if (onUndoClick != null) {
                            DropdownMenuItem(
                                text = { Text("撤回至此步骤", color = Color(0xFFDC2626)) },
                                leadingIcon = { Icon(Icons.Default.Undo, contentDescription = "撤回至此步骤", tint = Color(0xFFDC2626)) },
                                onClick = {
                                    showContextMenu = false
                                    onUndoClick(message)
                                }
                            )
                        }
                    }
                }
            } else if (userBodyText.isNotBlank()) {
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
                    val (userAnnotatedText, inlineContentMap) = remember(userBodyText, message.slashCommands) {
                        SlashCommandTextRenderer.buildInlineText(
                            text = userBodyText,
                            mentions = message.slashCommands.orEmpty(),
                            iconColor = colors.userBubbleText.copy(alpha = 0.85f)
                        )
                    }
                    Text(
                        text = userAnnotatedText,
                        inlineContent = inlineContentMap,
                        color = colors.userBubbleText,
                        fontSize = 15.5.sp,
                        lineHeight = 21.sp
                    )

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false }
                    ) {
                        DropdownMenuItem(
                            text = { Text("复制全文") },
                            leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制全文") },
                            onClick = {
                                showContextMenu = false
                                copyAll(userBodyText)
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
                        DropdownMenuItem(
                            text = { Text("分享为长图") },
                            leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享为长图") },
                            onClick = {
                                showContextMenu = false
                                shareCardRequest = ShareCardRequest(userBodyText, true, allUserImages)
                            }
                        )
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
                        BlankLongPressHost(onBlankLongPress = { showAgentMenu = true }) {
                            MarkdownContentView(
                                content = displayText,
                                onPlanClick = onPlanClick,
                                urlResolver = urlResolver,
                                onImageClick = { url -> onImageClick?.invoke(url, null) }
                            )
                        }
                    }

                    DropdownMenu(
                        expanded = showAgentMenu,
                        onDismissRequest = { showAgentMenu = false }
                    ) {
                        if (displayText.isNotBlank()) {
                            DropdownMenuItem(
                                text = { Text("复制全文") },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制全文") },
                                onClick = {
                                    showAgentMenu = false
                                    val plain = MarkdownPlainText.convert(displayText)
                                    copyAll(plain.ifEmpty { displayText })
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
                            DropdownMenuItem(
                                text = { Text("分享为长图") },
                                leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享为长图") },
                                onClick = {
                                    showAgentMenu = false
                                    shareCardRequest = ShareCardRequest(displayText, false, emptyList())
                                }
                            )
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
    val isVideo = com.antigravity.mobile.data.service.FileIconResolver.isVideo(artifact.title) ||
        com.antigravity.mobile.data.service.FileIconResolver.isVideo(artifact.uri)

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
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = if (isVideo) "🎬" else "📄",
                fontSize = 15.sp
            )
            Text(
                text = artifact.title.ifBlank { if (isVideo) "MP4 视频" else "文档详情" },
                color = colors.textPrimary,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
            if (isVideo) {
                Surface(
                    shape = RoundedCornerShape(4.dp),
                    color = colors.accentIndigo.copy(alpha = 0.15f)
                ) {
                    Text(
                        text = "MP4 视频",
                        color = colors.accentIndigo,
                        fontSize = 10.5.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                    )
                }
            }
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

/**
 * 清洗语音交互中的协议标签（<talker_agent_context>、<voice_instructions> 等）、时间戳与说明性文本，
 * 仅提取纯净的用户与智能体多轮语音对话。
 */
object VoiceTranscriptCleaner {
    private val instructionsBlockRegex = Regex(
        """<(?:voice_instructions|instructions)>[\s\S]*?(?:</(?:voice_instructions|instructions)>|(?=<talker_agent_context>)|$)""",
        RegexOption.IGNORE_CASE
    )
    private val contextBlockRegex = Regex(
        """<talker_agent_context>([\s\S]*?)(?:</talker_agent_context>|$)""",
        RegexOption.IGNORE_CASE
    )
    private val tagRegex = Regex(
        """</?(?:voice_instructions|instructions|talker_agent_context)>""",
        RegexOption.IGNORE_CASE
    )
    private val timestampRegex = Regex(
        """^\[\d{4}[-/]\d{2}[-/]\d{2}[ T][^\]]+\]\s*|^\[\d{2}:\d{2}(?::\d{2})?\]\s*"""
    )
    private val roleRegex = Regex(
        """^(user|human|me|用户|我|talker|agent|assistant|ai|智能体|助手|语音)\s*[:：]\s*""",
        RegexOption.IGNORE_CASE
    )

    fun isCJK(codePoint: Int): Boolean {
        return (codePoint in 0x4E00..0x9FFF) ||
               (codePoint in 0x3400..0x4DBF) ||
               (codePoint in 0x3040..0x309F) ||
               (codePoint in 0x30A0..0x30FF) ||
               (codePoint in 0xAC00..0xD7AF) ||
               (codePoint in 0x3000..0x303F) ||
               (codePoint in 0xFF01..0xFF60)
    }

    /**
     * 规范化中日韩（CJK）语音识别转文字的字间空格。
     * 语音听写流式吐字时经常在汉字/假名间携带临时空格（如 "我 想 问 一 下" -> "我想问一下"），
     * 本方法剔除 CJK 字符以及全角标点之间的冗余空格，同时完整保留英文单词及数字间的正常空格。
     */
    fun normalizeCJKSpacing(text: String): String {
        if (text.isEmpty()) return text
        val sb = StringBuilder(text.length)
        var i = 0
        val n = text.length
        while (i < n) {
            val c = text[i]
            if (c.isWhitespace()) {
                val prevCp = if (sb.isNotEmpty()) sb.codePointBefore(sb.length) else -1
                var j = i + 1
                while (j < n && text[j].isWhitespace()) {
                    j++
                }
                val nextCp = if (j < n) text.codePointAt(j) else -1
                if (prevCp != -1 && isCJK(prevCp) && nextCp != -1 && isCJK(nextCp)) {
                    i = j
                    continue
                } else {
                    sb.append(' ')
                    i = j
                    continue
                }
            } else {
                sb.append(c)
                i++
            }
        }
        return sb.toString().trim()
    }

    fun isVoiceMessage(body: String, files: List<AttachmentRules.ParsedFile> = emptyList()): Boolean {
        val trimmed = body.trim()
        return trimmed.contains("<talker_agent_context>") ||
               trimmed.contains("</talker_agent_context>") ||
               trimmed.contains("<voice_instructions>") ||
               trimmed.contains("</voice_instructions>") ||
               (trimmed.contains("talker:") && (trimmed.contains("user:") || trimmed.contains("[202"))) ||
               (files.isNotEmpty() && files.all { it.ext.lowercase() in listOf("wav", "m4a", "aac", "mp3", "caf", "opus") } && trimmed.isBlank())
    }

    fun clean(raw: String): String {
        if (raw.isBlank()) return ""

        val contextMatch = contextBlockRegex.find(raw)
        val textToProcess = if (contextMatch != null && contextMatch.groupValues[1].isNotBlank()) {
            contextMatch.groupValues[1]
        } else {
            val withoutInstructions = instructionsBlockRegex.replace(raw, "")
            tagRegex.replace(withoutInstructions, "")
        }

        val cleaned = mutableListOf<String>()
        var currentRole: String? = null
        val currentLines = mutableListOf<String>()
        var quoteOpen = false

        fun finishCurrent() {
            if (currentRole != null && currentLines.isNotEmpty()) {
                var text = currentLines.joinToString("\n").trim()
                if (text.startsWith("\"") && text.endsWith("\"") && text.length >= 2) {
                    text = text.substring(1, text.length - 1).trim()
                } else if (text.startsWith("\"")) {
                    text = text.substring(1).trim()
                } else if (text.endsWith("\"")) {
                    text = text.substring(0, text.length - 1).trim()
                }
                if (text.isNotEmpty()) {
                    val normalizedText = normalizeCJKSpacing(text)
                    val formatted = "$currentRole$normalizedText"
                    if (cleaned.isEmpty() || cleaned.last() != formatted) {
                        cleaned.add(formatted)
                    }
                }
            }
            currentRole = null
            currentLines.clear()
            quoteOpen = false
        }

        for (rawLine in textToProcess.lines()) {
            val line = rawLine.trim()
            if (line.isEmpty()) continue

            val content = timestampRegex.replace(line, "").trim()
            val roleMatch = roleRegex.find(content)

            if (roleMatch != null) {
                finishCurrent()
                val matched = roleMatch.groupValues[1].lowercase()
                currentRole = if (matched in listOf("user", "human", "me", "用户", "我")) {
                    "用户: "
                } else {
                    "智能体: "
                }
                val rem = content.substring(roleMatch.range.last + 1).trim()
                val quoteCount = rem.count { it == '"' }
                quoteOpen = (quoteCount % 2 == 1)
                currentLines.add(rem)
            } else if (quoteOpen && currentRole != null) {
                val quoteCount = content.count { it == '"' }
                if (quoteCount % 2 == 1) {
                    quoteOpen = false
                }
                currentLines.add(content)
            } else if (contextMatch != null && currentRole != null && !quoteOpen) {
                currentLines.add(content)
            }
        }

        finishCurrent()
        return cleaned.joinToString("\n\n")
    }
}

