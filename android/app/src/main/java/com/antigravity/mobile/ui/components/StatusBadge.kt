package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
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
            status.value
        )
        status.isError -> Triple(
            colors.accentRed.copy(alpha = 0.15f),
            colors.accentRed,
            "ERROR"
        )
        status.isRunning -> Triple(
            colors.accentGreen.copy(alpha = 0.15f),
            colors.accentGreen,
            status.value
        )
        else -> return
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
    Box(
        modifier = modifier
            .size(16.dp),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .size(14.dp)
                .clip(CircleShape)
                .background(colors.accentBlue.copy(alpha = 0.15f))
        )
        Box(
            modifier = Modifier
                .size(6.dp)
                .clip(CircleShape)
                .background(colors.accentBlue)
        )
    }
}
