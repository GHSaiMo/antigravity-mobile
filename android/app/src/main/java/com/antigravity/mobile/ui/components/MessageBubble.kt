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
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.Undo
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
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.ui.theme.AntigravityTheme

import androidx.compose.foundation.clickable
import androidx.compose.ui.platform.LocalContext
import android.widget.Toast
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitLongPressOrCancellation
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
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
    onUndoClick: ((GatewayMessageItem) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current

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

    val isUser = message.isUser
    val clipboard = LocalClipboardManager.current
    val haptic = LocalHapticFeedback.current
    fun copyAll(text: String) {
        clipboard.setText(AnnotatedString(text))
        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
        Toast.makeText(context, "已复制", Toast.LENGTH_SHORT).show()
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
                        Box(
                            modifier = Modifier
                                .size(72.dp)
                                .clip(RoundedCornerShape(12.dp))
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .clickable {
                                    if (onImageGroupClick != null) {
                                        onImageGroupClick(allUserImages, index)
                                    } else {
                                        val fallbackBmp = item.bitmap ?: item.bytes?.let { b ->
                                            try { BitmapFactory.decodeByteArray(b, 0, b.size) } catch (_: Exception) { null }
                                        }
                                        onImageClick?.invoke(item.url, fallbackBmp)
                                    }
                                }
                        ) {
                            if (item.bitmap != null) {
                                Image(
                                    bitmap = item.bitmap.asImageBitmap(),
                                    contentDescription = "Attached image",
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                )
                            } else if (item.bytes != null || !item.url.isNullOrBlank()) {
                                SubcomposeAsyncImage(
                                    model = ImageRequest.Builder(context)
                                        .data(item.bytes ?: item.url)
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
                            onClick = {},
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
                            text = { Text("撤回") },
                            leadingIcon = {
                                Icon(
                                    imageVector = Icons.Default.Undo,
                                    contentDescription = "撤回"
                                )
                            },
                            onClick = {
                                showContextMenu = false
                                onUndoClick?.invoke(message)
                            }
                        )
                    }
                }
            }
        } else {
            if (displayText.isNotBlank()) {
                val registry = remember { TextRegionRegistry() }
                var bubbleOrigin by remember { mutableStateOf(Offset.Zero) }
                // Agent Bubble: Card background, textPrimary, 18.dp radius with subtle soft shadow
                Box(
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
                        .onGloballyPositioned { bubbleOrigin = it.boundsInRoot().topLeft }
                        // Long-press on blank area (outside any selectable text block) copies the whole message.
                        .pointerInput(displayText) {
                            awaitEachGesture {
                                val down = awaitFirstDown(requireUnconsumed = false)
                                if (registry.containsRoot(down.position + bubbleOrigin)) return@awaitEachGesture
                                if (awaitLongPressOrCancellation(down.id) != null) {
                                    copyAll(MarkdownPlainText.convert(displayText))
                                }
                            }
                        }
                        .padding(horizontal = 14.dp, vertical = 12.dp)
                ) {
                    CompositionLocalProvider(LocalTextRegions provides registry) {
                        MarkdownContentView(
                            content = displayText,
                            onPlanClick = onPlanClick,
                            urlResolver = urlResolver,
                            onImageClick = { url -> onImageClick?.invoke(url, null) }
                        )
                    }
                }
            }
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

