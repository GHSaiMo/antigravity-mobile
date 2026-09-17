package com.antigravity.mobile.ui.components

import androidx.compose.animation.core.*
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun StatusBadge(
    status: ConversationStatus,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val (bg, fg, label) = when {
        status.needsAction -> Triple(
            colors.accentBlue.copy(alpha = 0.15f),
            colors.accentBlue,
            status.raw
        )
        status.isError -> Triple(
            colors.accentRed.copy(alpha = 0.15f),
            colors.accentRed,
            "ERROR"
        )
        status.isRunning -> Triple(
            colors.accentGreen.copy(alpha = 0.15f),
            colors.accentGreen,
            "RUNNING"
        )
        else -> Triple(
            colors.surfaceVariant,
            colors.textMuted,
            status.raw
        )
    }

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(bg)
            .padding(horizontal = 7.dp, vertical = 3.dp),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            color = fg,
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
