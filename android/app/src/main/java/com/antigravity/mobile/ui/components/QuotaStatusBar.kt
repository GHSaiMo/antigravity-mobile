package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.ui.theme.AntigravityTheme

private val StatusBarCompactTextStyle = TextStyle(platformStyle = PlatformTextStyle(includeFontPadding = false))

@Composable
fun QuotaStatusBar(
    account: CockpitAccountQuota?,
    onTap: () -> Unit,
    showPaceCursor: Boolean = true,
    linkSnapshot: com.antigravity.mobile.data.service.LinkSnapshot? = null,
    modifier: Modifier = Modifier
) {
    val isFree = account?.subscriptionTier?.trim()?.equals("FREE", ignoreCase = true) == true ||
        (account?.gemini5h == null && account?.claude5h == null)
    val bucket = if (isFree) {
        account?.geminiWeekly ?: account?.claudeWeekly ?: return
    } else {
        account?.gemini5h ?: account?.geminiWeekly ?: return
    }
    val titleText = if (isFree) {
        if (account?.geminiWeekly != null) "Gemini · Weekly" else "Claude · Weekly"
    } else {
        if (account?.gemini5h != null) "Gemini · 5h" else "Gemini · Weekly"
    }
    val percent = bucket.remainingPercent.coerceIn(0.0, 100.0)
    val paceInfo = androidx.compose.runtime.remember(bucket, titleText, account?.subscriptionTier) {
        bucket.calculatePaceForTitle(titleText, account?.subscriptionTier)
    }
    val hasCursor = showPaceCursor && paceInfo != null
    val colors = AntigravityTheme.colors

    val progressColor = when {
        percent >= 50 -> colors.accentGreen
        percent >= 20 -> colors.accentOrange
        else -> colors.accentRed
    }

    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            .clickable { onTap() }
            .padding(horizontal = 12.dp, vertical = 7.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Bolt,
                contentDescription = "Quota",
                tint = progressColor,
                modifier = Modifier.size(14.dp)
            )

            Text(
                text = titleText,
                color = colors.textPrimary,
                fontSize = 12.sp,
                lineHeight = 14.sp,
                fontWeight = FontWeight.Medium,
                style = StatusBarCompactTextStyle,
                maxLines = 1
            )

            // Dynamic width capsule progress bar with pace needle
            Box(
                modifier = Modifier
                    .weight(1f)
                    .height(if (hasCursor) 9.dp else 6.dp),
                contentAlignment = Alignment.CenterStart
            ) {
                // Base track & fill
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(6.dp)
                        .clip(CircleShape)
                        .background(colors.border.copy(alpha = 0.5f))
                ) {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth(fraction = (percent / 100.0).toFloat())
                            .fillMaxHeight()
                            .clip(CircleShape)
                            .background(progressColor)
                    )
                }

                // Vertical Pace Cursor Needle
                if (hasCursor) {
                    val expectedFrac = (paceInfo.expectedRemainingPercent / 100.0).toFloat().coerceIn(0f, 1f)
                    val needleColor = if (paceInfo.isOverPacing) Color(0xFFFF9F0A) else Color.White.copy(alpha = 0.92f)
                    BoxWithConstraints(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(9.dp),
                        contentAlignment = Alignment.CenterStart
                    ) {
                        val maxW = maxWidth
                        val needleOffset = (maxW * expectedFrac - 0.75.dp).coerceIn(0.dp, (maxW - 1.5.dp).coerceAtLeast(0.dp))
                        Box(
                            modifier = Modifier
                                .padding(start = needleOffset)
                                .size(width = 1.5.dp, height = 9.dp)
                                .clip(RoundedCornerShape(0.75.dp))
                                .background(needleColor)
                        )
                    }
                }
            }

            Text(
                text = "${percent.toInt()}%",
                color = progressColor,
                fontSize = 12.sp,
                lineHeight = 14.sp,
                fontWeight = FontWeight.SemiBold,
                fontFamily = FontFamily.Monospace,
                style = StatusBarCompactTextStyle,
                maxLines = 1
            )

            bucket.resetCountdownDisplay?.let { countdown ->
                Text(
                    text = countdown,
                    color = colors.textSecondary,
                    fontSize = 10.sp,
                    lineHeight = 12.sp,
                    style = StatusBarCompactTextStyle,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }

            if (linkSnapshot != null) {
                LinkHealthDot(snapshot = linkSnapshot)
            }
        }
    }
}
