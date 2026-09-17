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
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun MessageBubble(
    message: GatewayMessageItem,
    modifier: Modifier = Modifier,
    onPlanClick: ((uri: String, title: String) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors

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
            // Attached user images (e.g. screenshots)
            if (message.imageDataList.isNotEmpty()) {
                Row(
                    modifier = Modifier.padding(bottom = 6.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    message.imageDataList.forEach { bytes ->
                        val bitmap = remember(bytes) {
                            try {
                                BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            } catch (e: Exception) {
                                null
                            }
                        }
                        if (bitmap != null) {
                            Image(
                                bitmap = bitmap.asImageBitmap(),
                                contentDescription = "Attached image",
                                contentScale = ContentScale.Crop,
                                modifier = Modifier
                                    .size(72.dp)
                                    .clip(RoundedCornerShape(12.dp))
                                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                            )
                        }
                    }
                }
            }

            if (displayText.isNotBlank()) {
                // User Bubble: Apple Indigo, white text, 18.dp continuous corner radius
                Box(
                    modifier = Modifier
                        .widthIn(max = 320.dp)
                        .clip(RoundedCornerShape(18.dp))
                        .background(colors.userBubbleBg)
                        .padding(horizontal = 14.dp, vertical = 10.dp)
                ) {
                    Text(
                        text = displayText,
                        color = colors.userBubbleText,
                        fontSize = 15.5.sp,
                        lineHeight = 21.sp
                    )
                }
            }
        } else {
            if (displayText.isNotBlank()) {
                // Agent Bubble: Card background, textPrimary, 18.dp radius
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(18.dp))
                        .background(colors.agentBubbleBg)
                        .border(0.5.dp, colors.border, RoundedCornerShape(18.dp))
                        .padding(horizontal = 14.dp, vertical = 12.dp)
                ) {
                    MarkdownContentView(
                        content = displayText,
                        onPlanClick = onPlanClick
                    )
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
    onPlanClick: ((uri: String, title: String) -> Unit)? = null
) {
    MarkdownContentView(
        content = text,
        modifier = modifier,
        onPlanClick = onPlanClick
    )
}

