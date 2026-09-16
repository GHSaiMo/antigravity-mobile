package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.ui.theme.*

@Composable
fun MessageBubble(
    message: GatewayMessageItem,
    modifier: Modifier = Modifier
) {
    val isUser = message.role.equals("user", ignoreCase = true)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        horizontalAlignment = if (isUser) Alignment.End else Alignment.Start
    ) {
        // Render folded tools if any
        if (!message.toolCalls.isNullOrEmpty()) {
            ToolStepCollapseCard(
                toolCalls = message.toolCalls,
                modifier = Modifier
                    .fillMaxWidth(0.92f)
                    .padding(bottom = 6.dp)
            )
        }

        // Render Reasoning / Thinking section if present
        message.reasoningContent?.takeIf { it.isNotBlank() }?.let { reasoning ->
            Column(
                modifier = Modifier
                    .fillMaxWidth(0.92f)
                    .clip(RoundedCornerShape(8.dp))
                    .background(DarkBackground.copy(alpha = 0.6f))
                    .border(1.dp, DarkBorder, RoundedCornerShape(8.dp))
                    .padding(10.dp)
                    .padding(bottom = 4.dp)
            ) {
                Text(
                    text = "💭 思考过程",
                    color = TextMuted,
                    fontSize = 11.sp
                )
                Text(
                    text = reasoning.take(280) + if (reasoning.length > 280) "..." else "",
                    color = TextSecondary,
                    fontSize = 12.sp,
                    lineHeight = 16.sp
                )
            }
            Spacer(modifier = Modifier.height(4.dp))
        }

        // Render Message Content
        if (message.content.isNotBlank()) {
            Box(
                modifier = Modifier
                    .widthIn(max = 320.dp)
                    .clip(
                        RoundedCornerShape(
                            topStart = 12.dp,
                            topEnd = 12.dp,
                            bottomStart = if (isUser) 12.dp else 2.dp,
                            bottomEnd = if (isUser) 2.dp else 12.dp
                        )
                    )
                    .background(if (isUser) UserBubbleBg else AgentBubbleBg)
                    .border(
                        1.dp,
                        if (isUser) UserBubbleBg else DarkBorder,
                        RoundedCornerShape(12.dp)
                    )
                    .padding(horizontal = 14.dp, vertical = 10.dp)
            ) {
                SimpleMarkdownContent(text = message.content)
            }
        }
    }
}

/**
 * Lightweight native Markdown / Text formatter for Android streaming chat bubbles
 */
@Composable
fun SimpleMarkdownContent(
    text: String,
    modifier: Modifier = Modifier
) {
    val blocks = text.split("```")
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        blocks.forEachIndexed { index, block ->
            if (index % 2 == 1) {
                // Code block
                val lines = block.lines()
                val lang = lines.firstOrNull()?.trim() ?: ""
                val code = if (lines.size > 1) lines.drop(1).joinToString("\n") else block

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(6.dp))
                        .background(CodeBlockBg)
                        .padding(8.dp)
                ) {
                    if (lang.isNotBlank()) {
                        Text(
                            text = lang,
                            color = TextMuted,
                            fontSize = 10.sp,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                    Text(
                        text = code.trimEnd(),
                        color = TextPrimary,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        lineHeight = 16.sp
                    )
                }
            } else {
                // Regular prose
                if (block.isNotBlank()) {
                    Text(
                        text = block.trim(),
                        color = TextPrimary,
                        fontSize = 14.sp,
                        lineHeight = 20.sp
                    )
                }
            }
        }
    }
}
