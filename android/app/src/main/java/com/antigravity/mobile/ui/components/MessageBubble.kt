package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.graphics.BitmapFactory
import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.LiveHelp
import androidx.compose.material.icons.filled.Alarm
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.CallSplit
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.SelectAll
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Undo
import androidx.compose.material.icons.filled.UnfoldLess
import androidx.compose.material.icons.filled.SubdirectoryArrowLeft
import androidx.compose.material.icons.filled.UnfoldMore
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.material.icons.filled.ContentCopy
import coil.compose.AsyncImagePainter
import coil.compose.SubcomposeAsyncImage
import coil.compose.SubcomposeAsyncImageContent
import coil.request.ImageRequest
import com.antigravity.mobile.data.service.ApiClient
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
    isActiveToolBatch: Boolean = false,
    isActivePlan: Boolean = false,
    isStreaming: Boolean = false,
    isScheduledTaskPrompt: Boolean = false,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((url: String?, bitmap: Bitmap?) -> Unit)? = null,
    onImageGroupClick: ((items: List<ImageViewerItem>, initialIndex: Int) -> Unit)? = null,
    onUndoClick: ((GatewayMessageItem) -> Unit)? = null,
    onExportMarkdownClick: (() -> Unit)? = null,
    onSubagentClick: ((SubagentItem) -> Unit)? = null,
    onCompactClick: (() -> Unit)? = null,
    onStartNewSessionClick: (() -> Unit)? = null,
    shareContextProvider: ((GatewayMessageItem) -> ShareCardContext)? = null,
    onStopWorkflow: (() -> Unit)? = null,
    onSelectWorkflowSubagent: ((SubagentItem) -> Unit)? = null,
    onForkClick: ((GatewayMessageItem) -> Unit)? = null,
    onNarrateClick: ((String) -> Unit)? = null,
    onAskBtwClick: ((Int?, String) -> Unit)? = null,
    onInsertToParentClick: ((String) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val focusManager = LocalFocusManager.current

    // 子代理卡片：内联在「调用子代理」的位置（对齐桌面端）
    message.subagent?.takeIf { message.isSubagent }?.let { sa ->
        SubagentInlineCard(item = sa, onClick = onSubagentClick, modifier = modifier)
        return
    }

    // 动态多智能体工作流卡片 (Antigravity 2.32+)
    message.workflow?.takeIf { message.isWorkflow }?.let { wf ->
        WorkflowStageCard(
            workflow = wf,
            modifier = modifier,
            onStopWorkflow = onStopWorkflow,
            onSelectSubagent = onSelectWorkflowSubagent
        )
        return
    }

    // 实时步骤清单卡片 (update_steps，对齐桌面端 Plan 卡片与自动折叠胶囊)
    message.stepPlan?.takeIf { message.isStepPlan }?.let { plan ->
        StepPlanCard(
            plan = plan,
            isActivePlan = isActivePlan,
            modifier = modifier
        )
        return
    }

    // Standalone tool or thinking section (Desktop-aligned borderless header + timeline)
    if (message.isTools) {
        val resolvedToolNames = message.toolNames.orEmpty().ifEmpty {
            val count = message.toolCount ?: 0
            if (count > 0 && message.toolCalls.isNullOrEmpty() && message.toolDetails.isNullOrEmpty()) {
                List(count) { "tool_call" }
            } else {
                emptyList()
            }
        }
        if (isActiveToolBatch) {
            val liveFocus = message.focusText?.trim().orEmpty().ifEmpty { "Working" }
            ActiveFocusToolRow(
                focusText = liveFocus,
                startedAt = message.startedAt,
                thinkingText = message.thinking,
                thinkingDuration = message.thinkingDuration,
                thinkingBlocks = message.thinkingBlocks,
                toolDetails = message.toolDetails,
                toolCalls = message.toolCalls.orEmpty(),
                toolNames = resolvedToolNames,
                isExpandable = true,
                onFileClick = onPlanClick,
                modifier = modifier
            )
            return
        }
        ToolStepCollapseCard(
            toolCalls = message.toolCalls.orEmpty(),
            thinkingText = message.thinking,
            thinkingDuration = message.thinkingDuration,
            thinkingBlocks = message.thinkingBlocks,
            toolDetails = message.toolDetails,
            toolNames = resolvedToolNames,
            toolCount = message.toolCount ?: 0,
            onFileClick = onPlanClick,
            modifier = modifier.fillMaxWidth().padding(vertical = 2.dp)
        )
        return
    }

    // Error card for execution errors
    if (message.isError) {
        val errText = message.effectiveText.ifBlank { "执行遇到错误".loc }
        val isOverflowErr = message.isOverflow ||
            message.errorKind == "context_overflow" ||
            errText.contains("context window", ignoreCase = true) ||
            errText.contains("context length", ignoreCase = true) ||
            errText.contains("prompt is too long", ignoreCase = true)
        if (isOverflowErr) {
            RescueBubbleCard(
                errorText = errText,
                onCompactClick = onCompactClick,
                onStartNewSessionClick = onStartNewSessionClick,
                modifier = modifier
            )
            return
        }
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
            attemptInfo != null -> "ERROR · 尝试 %1\$d/%2\$d".loc(attemptInfo.first, attemptInfo.second)
            else -> "ERROR"
        }
        val titleText = when {
            attemptInfo != null && attemptInfo.first > 1 -> "执行遇到错误 (已重试 %d 次)".loc(attemptInfo.first)
            else -> "执行遇到错误".loc
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
                    .background(colors.accentRed.copy(alpha = if (colors.isDark) 0.12f else 0.08f))
                    .border(1.dp, colors.accentRed.copy(alpha = 0.25f), RoundedCornerShape(14.dp))
                    .padding(12.dp),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(28.dp)
                        .clip(CircleShape)
                        .background(colors.accentRed.copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Warning,
                        contentDescription = "Error",
                        tint = colors.accentRed,
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
                                .background(colors.accentRed.copy(alpha = 0.18f))
                                .padding(horizontal = 6.dp, vertical = 2.dp)
                        ) {
                            Text(
                                text = badgeText.uppercase(),
                                color = colors.accentRed,
                                fontSize = 10.sp,
                                fontWeight = FontWeight.Bold,
                                fontFamily = FontFamily.Monospace
                            )
                        }

                        Text(
                            text = titleText,
                            color = colors.accentRed,
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
    val bodyScale = if (LocalShareExport.current) 1.0f else LocalMarkdownBodyScale.current
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
            isScheduledTask = request.isScheduledTask,
            onDismiss = { shareCardRequest = null }
        )
    }

    // 选择文本：自由选择局部文本弹窗（对齐 iOS TextSelectionSheet）
    var textSelectionContent by remember { mutableStateOf<String?>(null) }
    textSelectionContent?.let { content ->
        TextSelectionSheet(
            content = content,
            onDismiss = { textSelectionContent = null }
        )
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        horizontalAlignment = if (isUser) Alignment.End else Alignment.Start
    ) {
        // Render folded tools / thinking capsule (simplified in Knowledge Mode)
        if (!message.toolCalls.isNullOrEmpty() || !message.thinking.isNullOrBlank()) {
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
                        text = if (!message.toolCalls.isNullOrEmpty()) "已完成 %d 项检索与命令操作".loc(message.toolCalls.size) else "已完成深度思考".loc,
                        color = colors.textSecondary,
                        fontSize = 11.5.sp
                    )
                }
            } else {
                ToolStepCollapseCard(
                    toolCalls = message.toolCalls.orEmpty(),
                    thinkingText = message.thinking,
                    thinkingDuration = message.thinkingDuration,
                    thinkingBlocks = message.thinkingBlocks,
                    toolDetails = message.toolDetails,
                    toolNames = message.toolNames.orEmpty(),
                    toolCount = message.toolCount ?: 0,
                    onFileClick = onPlanClick,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 4.dp)
                )
            }
        }

        // Render Reasoning / Thinking section if present
        message.reasoningContent?.takeIf { it.isNotBlank() }?.let { reasoning ->
            CollapsibleThinkingBlock(
                thinkingText = reasoning.trim(),
                initiallyExpanded = false,
                modifier = Modifier
                    .fillMaxWidth(0.95f)
                    .padding(bottom = 6.dp)
            )
        }

        // Render Message Content
        val displayText = message.effectiveText
        if (isUser) {
            // Consolidated attached user images (aligning with iOS / Web: use imageUrls as primary with bytes as thumbnail, fallback to bytes)
            val allUserImages = remember(message.imageUrls, message.media, message.imageDataList, urlResolver) {
                val imageBytesList = message.effectiveImageDataList
                val validImageUrls = message.imageUrls?.filter { FileIconResolver.isImage(it) }
                if (!validImageUrls.isNullOrEmpty()) {
                    val list = validImageUrls.mapIndexed { idx, rawUrl ->
                        val resolvedUrl = urlResolver?.invoke(rawUrl) ?: rawUrl
                        val bytes = imageBytesList.getOrNull(idx)
                        ImageViewerItem(url = resolvedUrl, bytes = bytes)
                    }.toMutableList()
                    if (imageBytesList.size > validImageUrls.size) {
                        for (i in validImageUrls.size until imageBytesList.size) {
                            list.add(ImageViewerItem(bytes = imageBytesList[i]))
                        }
                    }
                    list
                } else {
                    imageBytesList.map { bytes -> ImageViewerItem(bytes = bytes) }
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
                                    try {
                                        val opts = BitmapFactory.Options().apply {
                                            if (b.size > 2 * 1024 * 1024) inSampleSize = 2
                                        }
                                        BitmapFactory.decodeByteArray(b, 0, b.size, opts)
                                    } catch (_: Exception) { null }
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
                            } else if (!item.url.isNullOrBlank()) {
                                val bubbleUrl = remember(item.url) {
                                    ApiClient.withImagePreview(item.url, ApiClient.IMAGE_PREVIEW_THUMB)
                                }
                                val bubbleKey = remember(bubbleUrl) { ApiClient.mediaCacheKey(bubbleUrl) }
                                val fallbackThumb = remember(item.bytes) {
                                    item.bytes?.let { b ->
                                        try {
                                            BitmapFactory.decodeByteArray(b, 0, b.size)
                                        } catch (_: Exception) { null }
                                    }
                                }
                                SubcomposeAsyncImage(
                                    model = ImageRequest.Builder(context)
                                        .data(bubbleUrl)
                                        .diskCacheKey(bubbleKey)
                                        .memoryCacheKey(bubbleKey)
                                        .crossfade(fallbackThumb == null)
                                        .build(),
                                    contentDescription = "Attached image",
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                ) {
                                    when (painter.state) {
                                        is AsyncImagePainter.State.Success -> SubcomposeAsyncImageContent()
                                        else -> {
                                            if (fallbackThumb != null) {
                                                Image(
                                                    bitmap = fallbackThumb.asImageBitmap(),
                                                    contentDescription = "Attached image",
                                                    contentScale = ContentScale.Crop,
                                                    modifier = Modifier.fillMaxSize()
                                                )
                                            }
                                        }
                                    }
                                }
                            } else if (item.bytes != null) {
                                SubcomposeAsyncImage(
                                    model = ImageRequest.Builder(context)
                                        .data(item.bytes)
                                        .size(256)
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
            val isScheduledTaskMessage = !isVoiceMessage && (isScheduledTaskPrompt || message.isScheduledTask) && userBodyText.isNotBlank()

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
                                val typeLabel = if (FileIconResolver.isWeChatChatArchive(f.name)) "微信聊天记录".loc else f.ext.uppercase()
                                Text(
                                    text = "$typeLabel · ${f.sizeLabel}",
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
                                modifier = Modifier.size((18f * bodyScale).dp),
                                tint = colors.userBubbleText
                            )
                            Text(
                                text = "语音交互".loc,
                                color = colors.userBubbleText,
                                fontSize = (14f * bodyScale).sp,
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
                                    modifier = Modifier.size((16f * bodyScale).dp),
                                    tint = colors.userBubbleText.copy(alpha = 0.85f)
                                )
                                Text(
                                    text = "语音交互".loc,
                                    color = colors.userBubbleText.copy(alpha = 0.85f),
                                    fontSize = (12f * bodyScale).sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                            Text(
                                text = cleanedVoiceText,
                                color = colors.userBubbleText,
                                fontSize = (14.5f * bodyScale).sp,
                                lineHeight = (20f * bodyScale).sp
                            )
                        }
                    }

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false },
                        containerColor = colors.surface
                    ) {
                        if (cleanedVoiceText.isNotBlank()) {
                            DropdownMenuItem(
                                text = { Text(if (isVoiceExpanded) "折叠语音".loc else "展开详情".loc) },
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
                                text = { Text("复制语音内容".loc) },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制语音内容".loc) },
                                onClick = {
                                    showContextMenu = false
                                    copyAll(cleanedVoiceText)
                                }
                            )
                        }
                        if (onUndoClick != null) {
                            DropdownMenuItem(
                                text = { Text("撤回至此步骤".loc, color = colors.accentRed) },
                                leadingIcon = { Icon(Icons.Default.Undo, contentDescription = "撤回至此步骤".loc, tint = colors.accentRed) },
                                onClick = {
                                    showContextMenu = false
                                    onUndoClick(message)
                                }
                            )
                        }
                    }
                }
            } else if (isScheduledTaskMessage) {
                var showContextMenu by remember { mutableStateOf(false) }
                var isScheduledExpanded by remember(message.id) { mutableStateOf(false) }
                val tapHaptic = rememberHaptic()
                val (userAnnotatedText, inlineContentMap) = remember(
                    userBodyText,
                    message.slashCommands,
                    colors.userBubbleText,
                    bodyScale
                ) {
                    SlashCommandTextRenderer.buildInlineText(
                        text = userBodyText,
                        mentions = message.slashCommands.orEmpty(),
                        iconColor = colors.userBubbleText.copy(alpha = 0.85f),
                        scale = bodyScale
                    )
                }

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
                        .animateContentSize()
                        .combinedClickable(
                            onClick = {
                                focusManager.clearFocus()
                                tapHaptic.light()
                                isScheduledExpanded = !isScheduledExpanded
                            },
                            onLongClick = { showContextMenu = true }
                        )
                        .padding(horizontal = 14.dp, vertical = 10.dp)
                ) {
                    if (!isScheduledExpanded) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Alarm,
                                contentDescription = null,
                                tint = colors.userBubbleText,
                                modifier = Modifier.size((17f * bodyScale).dp)
                            )
                            Text(
                                text = "定时任务".loc,
                                color = colors.userBubbleText,
                                fontSize = (14f * bodyScale).sp,
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
                                Icon(
                                    imageVector = Icons.Default.Alarm,
                                    contentDescription = null,
                                    tint = colors.userBubbleText.copy(alpha = 0.85f),
                                    modifier = Modifier.size((15f * bodyScale).dp)
                                )
                                Text(
                                    text = "定时任务".loc,
                                    color = colors.userBubbleText.copy(alpha = 0.85f),
                                    fontSize = (12f * bodyScale).sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                            Text(
                                text = userAnnotatedText,
                                inlineContent = inlineContentMap,
                                color = colors.userBubbleText,
                                fontSize = (14.5f * bodyScale).sp,
                                lineHeight = (20f * bodyScale).sp
                            )
                        }
                    }

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false },
                        containerColor = colors.surface
                    ) {
                        DropdownMenuItem(
                            text = { Text(if (isScheduledExpanded) "折叠提示词".loc else "展开提示词".loc) },
                            leadingIcon = {
                                Icon(
                                    if (isScheduledExpanded) Icons.Default.UnfoldLess else Icons.Default.UnfoldMore,
                                    contentDescription = null
                                )
                            },
                            onClick = {
                                showContextMenu = false
                                isScheduledExpanded = !isScheduledExpanded
                            }
                        )
                        DropdownMenuItem(
                            text = { Text("复制全文".loc) },
                            leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制全文".loc) },
                            onClick = {
                                showContextMenu = false
                                copyAll(userBodyText)
                            }
                        )
                        if (onExportMarkdownClick != null) {
                            DropdownMenuItem(
                                text = { Text("导出MD".loc) },
                                leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "导出MD".loc) },
                                onClick = {
                                    showContextMenu = false
                                    onExportMarkdownClick()
                                }
                            )
                        }
                        DropdownMenuItem(
                            text = { Text("分享长图".loc) },
                            leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享长图".loc) },
                            onClick = {
                                showContextMenu = false
                                shareCardRequest = ShareCardRequest(userBodyText, true, allUserImages, isScheduledTask = true)
                            }
                        )
                        if (onAskBtwClick != null && message.stepIndex != null) {
                            HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            DropdownMenuItem(
                                text = { Text("旁路快问".loc) },
                                leadingIcon = { Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = "旁路快问".loc) },
                                onClick = {
                                    showContextMenu = false
                                    onAskBtwClick(message.stepIndex, userBodyText)
                                }
                            )
                        }
                        if (onUndoClick != null) {
                            HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            DropdownMenuItem(
                                text = { Text("撤回".loc) },
                                leadingIcon = {
                                    Icon(
                                        imageVector = Icons.Default.Undo,
                                        contentDescription = "撤回".loc
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
                    val (userAnnotatedText, inlineContentMap) = remember(
                        userBodyText,
                        message.slashCommands,
                        colors.userBubbleText,
                        bodyScale
                    ) {
                        SlashCommandTextRenderer.buildInlineText(
                            text = userBodyText,
                            mentions = message.slashCommands.orEmpty(),
                            iconColor = colors.userBubbleText.copy(alpha = 0.85f),
                            scale = bodyScale
                        )
                    }
                    val hasSkillDefLinks = remember(userAnnotatedText) {
                        userAnnotatedText.getStringAnnotations(
                            tag = SlashCommandTextRenderer.SKILL_DEF_ANNOTATION_TAG,
                            start = 0,
                            end = userAnnotatedText.length
                        ).isNotEmpty()
                    }
                    var userTextLayoutResult by remember { mutableStateOf<TextLayoutResult?>(null) }
                    Text(
                        text = userAnnotatedText,
                        inlineContent = inlineContentMap,
                        color = colors.userBubbleText,
                        fontSize = (15.5f * bodyScale).sp,
                        lineHeight = (21f * bodyScale).sp,
                        onTextLayout = { userTextLayoutResult = it },
                        modifier = if (hasSkillDefLinks && onPlanClick != null) {
                            Modifier.pointerInput(userAnnotatedText, onPlanClick) {
                                detectTapGestures(
                                    onTap = { pos ->
                                        val layout = userTextLayoutResult
                                        val hit = layout?.let {
                                            val offset = it.getOffsetForPosition(pos)
                                            userAnnotatedText.getStringAnnotations(
                                                tag = SlashCommandTextRenderer.SKILL_DEF_ANNOTATION_TAG,
                                                start = offset,
                                                end = offset
                                            ).firstOrNull()
                                        }
                                        if (hit != null) {
                                            val parts = hit.item.split("\n", limit = 2)
                                            val defPath = parts.getOrNull(0).orEmpty()
                                            val skillName = parts.getOrNull(1).orEmpty().ifBlank { "SKILL.md" }
                                            if (defPath.isNotBlank()) {
                                                onPlanClick(defPath, "$skillName · SKILL.md")
                                                return@detectTapGestures
                                            }
                                        }
                                        focusManager.clearFocus()
                                    },
                                    onLongPress = { showContextMenu = true }
                                )
                            }
                        } else {
                            Modifier
                        }
                    )

                    DropdownMenu(
                        expanded = showContextMenu,
                        onDismissRequest = { showContextMenu = false },
                        containerColor = colors.surface
                    ) {
                        DropdownMenuItem(
                            text = { Text("复制全文".loc) },
                            leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制全文".loc) },
                            onClick = {
                                showContextMenu = false
                                copyAll(userBodyText)
                            }
                        )
                        if (onExportMarkdownClick != null) {
                            DropdownMenuItem(
                                text = { Text("导出MD".loc) },
                                leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "导出MD".loc) },
                                onClick = {
                                    showContextMenu = false
                                    onExportMarkdownClick()
                                }
                            )
                        }
                        DropdownMenuItem(
                            text = { Text("分享长图".loc) },
                            leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享长图".loc) },
                            onClick = {
                                showContextMenu = false
                                shareCardRequest = ShareCardRequest(userBodyText, true, allUserImages)
                            }
                        )
                        if (onAskBtwClick != null && message.stepIndex != null) {
                            HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            DropdownMenuItem(
                                text = { Text("旁路快问".loc) },
                                leadingIcon = { Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = "旁路快问".loc) },
                                onClick = {
                                    showContextMenu = false
                                    onAskBtwClick(message.stepIndex, userBodyText)
                                }
                            )
                        }
                        // 当前 Antigravity 不支持撤回接口时，调用方传 null，菜单里就不显示这一项
                        if (onUndoClick != null) {
                            HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            DropdownMenuItem(
                                text = { Text("撤回".loc) },
                                leadingIcon = {
                                    Icon(
                                        imageVector = Icons.Default.Undo,
                                        contentDescription = "撤回".loc
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
                // 非流式状态下：长按文字直接原位选字；长按气泡边缘或文字行尾空白处弹出菜单（菜单内仍保留「选择文本」兜底）
                var showAgentMenu by remember { mutableStateOf(false) }
                val allowDirectSelection = !isStreaming && !LocalShareExport.current
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
                        if (allowDirectSelection) {
                            BlankLongPressHost(onBlankLongPress = { showAgentMenu = true }) {
                                MarkdownContentView(
                                    content = displayText,
                                    onPlanClick = onPlanClick,
                                    urlResolver = urlResolver,
                                    onImageClick = { url -> onImageClick?.invoke(url, null) },
                                    selectable = true,
                                    onLongPress = null
                                )
                            }
                        } else {
                            MarkdownContentView(
                                content = displayText,
                                onPlanClick = onPlanClick,
                                urlResolver = urlResolver,
                                onImageClick = { url -> onImageClick?.invoke(url, null) },
                                selectable = false,
                                onLongPress = { showAgentMenu = true }
                            )
                        }
                        InlineGitHubPRPillsSection(
                            content = displayText,
                            onOpenPR = { prUrl -> onPlanClick?.invoke(prUrl, "Pull Request") }
                        )
                    }

                    DropdownMenu(
                        expanded = showAgentMenu,
                        onDismissRequest = { showAgentMenu = false },
                        containerColor = colors.surface
                    ) {
                        if (displayText.isNotBlank()) {
                            if (onInsertToParentClick != null) {
                                DropdownMenuItem(
                                    text = { Text("插入主对话".loc) },
                                    leadingIcon = { Icon(Icons.Default.SubdirectoryArrowLeft, contentDescription = "插入主对话".loc) },
                                    onClick = {
                                        showAgentMenu = false
                                        val plain = MarkdownPlainText.convert(displayText)
                                        onInsertToParentClick(plain.ifEmpty { displayText })
                                    }
                                )
                                HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            }
                            val hasGroup1 = onNarrateClick != null
                            val hasGroup2 = onForkClick != null || onAskBtwClick != null

                            if (hasGroup1) {
                                DropdownMenuItem(
                                    text = { Text("语音播报".loc) },
                                    leadingIcon = { Icon(Icons.Default.VolumeUp, contentDescription = "语音播报".loc) },
                                    onClick = {
                                        showAgentMenu = false
                                        val plain = MarkdownPlainText.convert(displayText)
                                        onNarrateClick?.invoke(plain.ifEmpty { displayText })
                                    }
                                )
                                HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            }

                            if (hasGroup2) {
                                if (onForkClick != null) {
                                    DropdownMenuItem(
                                        text = { Text("分叉会话".loc) },
                                        leadingIcon = { Icon(Icons.Default.CallSplit, contentDescription = "分叉会话".loc) },
                                        onClick = {
                                            showAgentMenu = false
                                            onForkClick(message)
                                        }
                                    )
                                }
                                if (onAskBtwClick != null) {
                                    DropdownMenuItem(
                                        text = { Text("旁路快问".loc) },
                                        leadingIcon = { Icon(Icons.AutoMirrored.Outlined.LiveHelp, contentDescription = "旁路快问".loc) },
                                        onClick = {
                                            showAgentMenu = false
                                            val plain = MarkdownPlainText.convert(displayText)
                                            onAskBtwClick(message.stepIndex, plain.ifEmpty { displayText })
                                        }
                                    )
                                }
                                HorizontalDivider(color = colors.border.copy(alpha = 0.4f))
                            }

                            DropdownMenuItem(
                                text = { Text("选择文本".loc) },
                                leadingIcon = { Icon(Icons.Default.SelectAll, contentDescription = "选择文本".loc) },
                                onClick = {
                                    showAgentMenu = false
                                    val plain = MarkdownPlainText.convert(displayText)
                                    textSelectionContent = plain.ifEmpty { displayText }
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("复制全文".loc) },
                                leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = "复制全文".loc) },
                                onClick = {
                                    showAgentMenu = false
                                    val plain = MarkdownPlainText.convert(displayText)
                                    copyAll(plain.ifEmpty { displayText })
                                }
                            )
                            if (onExportMarkdownClick != null) {
                                DropdownMenuItem(
                                    text = { Text("导出MD".loc) },
                                    leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = "导出MD".loc) },
                                    onClick = {
                                        showAgentMenu = false
                                        onExportMarkdownClick()
                                    }
                                )
                            }
                            DropdownMenuItem(
                                text = { Text("分享长图".loc) },
                                leadingIcon = { Icon(Icons.Default.Share, contentDescription = "分享长图".loc) },
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

                    if (onInsertToParentClick != null && displayText.isNotBlank()) {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(999.dp))
                                .background(colors.accentIndigo.copy(alpha = 0.10f))
                                .clickable {
                                    val plain = MarkdownPlainText.convert(displayText)
                                    onInsertToParentClick(plain.ifEmpty { displayText })
                                }
                                .padding(horizontal = 10.dp, vertical = 5.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.SubdirectoryArrowLeft,
                                contentDescription = null,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(13.dp)
                            )
                            Text(
                                text = "插入主对话".loc,
                                color = colors.accentIndigo,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.SemiBold
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
                text = artifact.title.ifBlank { if (isVideo) "MP4 视频".loc else "文档详情".loc },
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
                        text = "MP4 视频".loc,
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
    val images: List<ImageViewerItem>,
    val isScheduledTask: Boolean = false
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
                    "用户: ".loc
                } else {
                    "智能体: ".loc
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

