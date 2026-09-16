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
import com.antigravity.mobile.ui.theme.*

@Composable
fun StatusBadge(
    status: ConversationStatus,
    modifier: Modifier = Modifier
) {
    val (bg, fg, label) = when (status) {
        ConversationStatus.RUNNING -> Triple(
            AccentGreen.copy(alpha = 0.15f),
            AccentGreen,
            "RUNNING"
        )
        ConversationStatus.ACTION -> Triple(
            AccentBlue.copy(alpha = 0.15f),
            AccentBlue,
            "ACTION"
        )
        ConversationStatus.ERROR -> Triple(
            AccentRed.copy(alpha = 0.15f),
            AccentRed,
            "ERROR"
        )
        else -> return
    }

    Row(
        modifier = modifier
            .clip(RoundedCornerShape(4.dp))
            .background(bg)
            .padding(horizontal = 6.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Box(
            modifier = Modifier
                .size(6.dp)
                .clip(CircleShape)
                .background(fg)
        )
        Text(
            text = label,
            color = fg,
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold
        )
    }
}

@Composable
fun UnreadDot(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .size(10.dp)
            .clip(CircleShape)
            .background(AccentBlue.copy(alpha = 0.25f)),
        contentAlignment = Alignment.Center
    ) {
        Box(
            modifier = Modifier
                .size(6.dp)
                .clip(CircleShape)
                .background(AccentBlue)
        )
    }
}
