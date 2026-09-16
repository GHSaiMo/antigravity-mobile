package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.ui.theme.*

@Composable
fun QuotaStatusBar(
    account: CockpitAccountQuota?,
    onTap: () -> Unit,
    modifier: Modifier = Modifier
) {
    val bucket = account?.gemini5h ?: return
    val percent = bucket.remainingPercent.coerceIn(0.0, 100.0)

    val progressColor = when {
        percent >= 50 -> AccentGreen
        percent >= 20 -> AccentYellow
        else -> AccentRed
    }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(20.dp))
            .background(DarkSurface)
            .clickable { onTap() }
            .padding(horizontal = 12.dp, vertical = 6.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Bolt,
                contentDescription = "Quota",
                tint = progressColor,
                modifier = Modifier.size(16.dp)
            )

            Text(
                text = "Gemini 5h",
                color = TextPrimary,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium
            )

            // Dynamic width capsule progress bar
            Box(
                modifier = Modifier
                    .weight(1f)
                    .height(6.dp)
                    .clip(CircleShape)
                    .background(DarkBorder)
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth(fraction = (percent / 100.0).toFloat())
                        .fillMaxHeight()
                        .clip(CircleShape)
                        .background(progressColor)
                )
            }

            Text(
                text = "${percent.toInt()}%",
                color = progressColor,
                fontSize = 12.sp,
                fontWeight = FontWeight.Bold
            )

            bucket.resetCountdownDisplay?.let { countdown ->
                Text(
                    text = countdown,
                    color = TextMuted,
                    fontSize = 11.sp
                )
            }
        }
    }
}
