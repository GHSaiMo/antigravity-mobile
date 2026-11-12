package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.SwarmBrief
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun SwarmBanner(
    brief: SwarmBrief,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val isDark = AntigravityTheme.colors.isDark

    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(SwarmColors.cardBg(isDark))
            .border(0.8.dp, SwarmColors.accent(isDark).copy(alpha = 0.35f), RoundedCornerShape(12.dp))
            .clickable { onClick() }
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        SwarmPixelBee(
            name = "coordinator",
            pose = if (brief.workingCount > 0) SwarmBeePose.WORKING else SwarmBeePose.LANDED,
            size = 28.dp
        )

        Spacer(modifier = Modifier.width(10.dp))

        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(2.dp)
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text(
                    text = brief.title.ifBlank { "蜂群协同 (Swarm)" },
                    color = SwarmColors.ink(isDark),
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )

                if (brief.phase.isNotBlank()) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(SwarmColors.accent(isDark).copy(alpha = 0.12f))
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    ) {
                        Text(
                            text = brief.phase,
                            color = SwarmColors.accent(isDark),
                            fontSize = 10.sp,
                            fontWeight = FontWeight.Bold
                        )
                    }
                }
            }

            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text(
                    text = "${brief.workingCount}/${brief.totalMembers} 活跃",
                    color = SwarmColors.muted(isDark),
                    fontSize = 11.sp
                )
                Text(
                    text = "·",
                    color = SwarmColors.faint(isDark),
                    fontSize = 11.sp
                )
                Text(
                    text = "#${brief.latestSeq} 看板",
                    color = SwarmColors.muted(isDark),
                    fontSize = 11.sp
                )
                if (brief.headline.isNotBlank()) {
                    Text(
                        text = "· ${brief.headline}",
                        color = SwarmColors.muted(isDark),
                        fontSize = 11.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }

        Spacer(modifier = Modifier.width(8.dp))

        if (brief.attentionCount > 0) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .clip(CircleShape)
                    .background(SwarmColors.warnAmber(isDark).copy(alpha = 0.12f))
                    .padding(horizontal = 7.dp, vertical = 3.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Warning,
                    contentDescription = null,
                    tint = SwarmColors.warnAmber(isDark),
                    modifier = Modifier.size(11.dp)
                )
                Spacer(modifier = Modifier.width(3.dp))
                Text(
                    text = "${brief.attentionCount} 待关注",
                    color = SwarmColors.warnAmber(isDark),
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium
                )
            }
            Spacer(modifier = Modifier.width(6.dp))
        }

        Icon(
            imageVector = Icons.Default.ChevronRight,
            contentDescription = null,
            tint = SwarmColors.muted(isDark),
            modifier = Modifier.size(16.dp)
        )
    }
}
