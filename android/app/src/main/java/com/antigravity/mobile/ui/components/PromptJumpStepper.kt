package com.antigravity.mobile.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.KeyboardDoubleArrowDown
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.unit.dp
import com.antigravity.mobile.ui.theme.AntigravityTheme

/**
 * 垂直纵向双钮跳跃导航条 (Prompt Jump Stepper)
 * 44×44dp 质感双圆钮堆叠，专用于在长篇会话中快速切轮跳转与直达最新。
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun PromptJumpStepper(
    currentTurn: Int,
    totalTurns: Int,
    isAtTop: Boolean,
    isAtLastTurn: Boolean,
    onJumpPrevious: () -> Unit,
    onJumpNextOrBottom: () -> Unit,
    onForceJumpBottom: () -> Unit,
    modifier: Modifier = Modifier
) {
    val haptic = LocalHapticFeedback.current
    val colors = AntigravityTheme.colors

    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        // 上方圆钮：上一轮提问 (▲)
        Box(
            modifier = Modifier
                .size(44.dp)
                .shadow(elevation = 4.dp, shape = CircleShape)
                .clip(CircleShape)
                .background(colors.surfaceVariant.copy(alpha = 0.88f))
                .border(width = 0.5.dp, color = colors.border.copy(alpha = 0.18f), shape = CircleShape)
                .combinedClickable(
                    enabled = !isAtTop,
                    interactionSource = remember { MutableInteractionSource() },
                    indication = null,
                    onClick = {
                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                        onJumpPrevious()
                    }
                ),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.KeyboardArrowUp,
                contentDescription = "上一轮提问",
                modifier = Modifier
                    .size(20.dp)
                    .alpha(if (isAtTop) 0.25f else 1.0f),
                tint = colors.textPrimary
            )
        }

        // 下方圆钮：自适应 下一轮提问 / ⏬ 直达最新
        Box(
            modifier = Modifier
                .size(44.dp)
                .shadow(elevation = 4.dp, shape = CircleShape)
                .clip(CircleShape)
                .background(colors.surfaceVariant.copy(alpha = 0.88f))
                .border(width = 0.5.dp, color = colors.border.copy(alpha = 0.18f), shape = CircleShape)
                .combinedClickable(
                    interactionSource = remember { MutableInteractionSource() },
                    indication = null,
                    onClick = {
                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                        onJumpNextOrBottom()
                    },
                    onLongClick = {
                        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                        onForceJumpBottom()
                    }
                ),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = if (isAtLastTurn) Icons.Default.KeyboardDoubleArrowDown else Icons.Default.KeyboardArrowDown,
                contentDescription = if (isAtLastTurn) "直达最新消息" else "下一轮提问",
                modifier = Modifier.size(if (isAtLastTurn) 19.dp else 20.dp),
                tint = colors.textPrimary
            )
        }
    }
}
