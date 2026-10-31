package com.antigravity.mobile.ui.components

import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.ui.theme.AntigravityTheme

private data class BadgeConfig(
    val bg: Color,
    val fg: Color,
    val label: String,
    val isTerminal: Boolean = false
)

/**
 * 原样复刻电脑端 (status-task-running) 终端执行指令动效图标
 * - 视口: 16x16
 * - 提示符: M3.7 5.2L6.7 8L3.7 10.8 (strokeWidth 1.4, round cap/join)
 * - 光标下划线: M8.3 10.8H12.3
 * - 动画: 1.05s 周期, 0~55% 完全显示, 56~100% 隐藏 (对齐桌面端 @keyframes agentStateCaret)
 */
@Composable
fun TerminalRunningIndicator(
    modifier: Modifier = Modifier,
    size: Dp = 12.dp,
    color: Color = AntigravityTheme.colors.accentGreen
) {
    val infiniteTransition = rememberInfiniteTransition(label = "TerminalCaretBlink")
    val caretAlpha by infiniteTransition.animateFloat(
        initialValue = 1f,
        targetValue = 0f,
        animationSpec = infiniteRepeatable(
            animation = keyframes {
                durationMillis = 1050
                1f at 0
                1f at 577   // 55% 持续完全可见
                0f at 578   // 阶跃清脆跳动熄灭
                0f at 1050
            },
            repeatMode = RepeatMode.Restart
        ),
        label = "CaretAlpha"
    )

    Canvas(modifier = modifier.size(size)) {
        val scale = this.size.width / 16f
        val stroke = Stroke(
            width = 1.4f * scale,
            cap = StrokeCap.Round,
            join = StrokeJoin.Round
        )

        // 1. 终端提示符 >
        val promptPath = Path().apply {
            moveTo(3.7f * scale, 5.2f * scale)
            lineTo(6.7f * scale, 8.0f * scale)
            lineTo(3.7f * scale, 10.8f * scale)
        }
        drawPath(promptPath, color = color, style = stroke)

        // 2. 闪烁下划线光标 _
        if (caretAlpha > 0.05f) {
            drawLine(
                color = color.copy(alpha = caretAlpha),
                start = Offset(8.3f * scale, 10.8f * scale),
                end = Offset(12.3f * scale, 10.8f * scale),
                strokeWidth = stroke.width,
                cap = StrokeCap.Round
            )
        }
    }
}

@Composable
fun StatusBadge(
    status: ConversationStatus,
    hasRunningTasks: Boolean = false,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val isRunningCommand = hasRunningTasks
    val config = when {
        status.needsAction -> BadgeConfig(
            bg = colors.accentBlue.copy(alpha = 0.15f),
            fg = colors.accentBlue,
            label = "Action required"
        )
        status.isError -> BadgeConfig(
            bg = colors.accentRed.copy(alpha = 0.15f),
            fg = colors.accentRed,
            label = "Error"
        )
        isRunningCommand -> BadgeConfig(
            bg = colors.accentGreen.copy(alpha = 0.15f),
            fg = colors.accentGreen,
            label = "Running command",
            isTerminal = true
        )
        status.isRunning -> BadgeConfig(
            bg = colors.accentGreen.copy(alpha = 0.15f),
            fg = colors.accentGreen,
            label = "Working"
        )
        else -> BadgeConfig(
            bg = colors.surfaceVariant,
            fg = colors.textMuted,
            label = status.raw
        )
    }

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(config.bg)
            .padding(horizontal = 7.dp, vertical = 3.dp),
        contentAlignment = Alignment.Center
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            if (config.isTerminal) {
                TerminalRunningIndicator(
                    size = 12.dp,
                    color = config.fg
                )
            }
            Text(
                text = config.label,
                color = config.fg,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold
            )
        }
    }
}

@Composable
fun DraftBadge(modifier: Modifier = Modifier) {
    val colors = AntigravityTheme.colors
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(colors.accentYellow.copy(alpha = 0.15f))
            .padding(horizontal = 7.dp, vertical = 3.dp),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "DRAFT",
            color = colors.accentYellow,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold
        )
    }
}

@Composable
fun UnreadDot(modifier: Modifier = Modifier) {
    val colors = AntigravityTheme.colors
    val infiniteTransition = rememberInfiniteTransition(label = "UnreadPulseTransition")

    val pulseScale by infiniteTransition.animateFloat(
        initialValue = 1.0f,
        targetValue = 1.55f,
        animationSpec = infiniteRepeatable(
            animation = tween(1400, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Restart
        ),
        label = "PulseScale"
    )

    val pulseAlpha by infiniteTransition.animateFloat(
        initialValue = 0.35f,
        targetValue = 0.02f,
        animationSpec = infiniteRepeatable(
            animation = tween(1400, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Restart
        ),
        label = "PulseAlpha"
    )

    Box(
        modifier = modifier.size(16.dp),
        contentAlignment = Alignment.Center
    ) {
        // Pulsing breathing outer aura
        Box(
            modifier = Modifier
                .size(14.dp)
                .scale(pulseScale)
                .clip(CircleShape)
                .background(colors.accentBlue.copy(alpha = pulseAlpha))
        )
        // Static soft base halo
        Box(
            modifier = Modifier
                .size(12.dp)
                .clip(CircleShape)
                .background(colors.accentBlue.copy(alpha = 0.18f))
        )
        // Core crisp dot
        Box(
            modifier = Modifier
                .size(6.dp)
                .clip(CircleShape)
                .background(colors.accentBlue)
        )
    }
}
