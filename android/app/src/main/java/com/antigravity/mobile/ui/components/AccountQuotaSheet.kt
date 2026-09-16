package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.ui.theme.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountQuotaSheet(
    quotaData: CockpitQuotaResponse?,
    isRefreshing: Boolean,
    onRefresh: () -> Unit,
    onSwitchAccount: (String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    var isMasked by remember { mutableStateOf(true) }
    val current = quotaData?.currentAccount

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = DarkSurface,
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp)
                .padding(bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            // Header Row
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column {
                    Text(
                        text = "Cockpit 配额监控",
                        color = TextPrimary,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.Bold
                    )
                    current?.let { acc ->
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Text(
                                text = if (isMasked) acc.maskedEmail else acc.email,
                                color = TextSecondary,
                                fontSize = 13.sp
                            )
                            IconButton(
                                onClick = { isMasked = !isMasked },
                                modifier = Modifier.size(24.dp)
                            ) {
                                Icon(
                                    imageVector = if (isMasked) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                    contentDescription = "Toggle Mask",
                                    tint = TextMuted,
                                    modifier = Modifier.size(16.dp)
                                )
                            }
                        }
                    }
                }

                IconButton(
                    onClick = onRefresh,
                    enabled = !isRefreshing
                ) {
                    if (isRefreshing) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            strokeWidth = 2.dp,
                            color = AccentBlue
                        )
                    } else {
                        Icon(
                            imageVector = Icons.Default.Refresh,
                            contentDescription = "Refresh",
                            tint = AccentBlue
                        )
                    }
                }
            }

            // Quota Four Quadrant Cards
            current?.let { acc ->
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        QuotaMetricCard(
                            title = "Claude 5h",
                            bucket = acc.claude5h,
                            modifier = Modifier.weight(1f)
                        )
                        QuotaMetricCard(
                            title = "Claude 周配额",
                            bucket = acc.claudeWeekly,
                            modifier = Modifier.weight(1f)
                        )
                    }

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        QuotaMetricCard(
                            title = "Gemini 5h",
                            bucket = acc.gemini5h,
                            modifier = Modifier.weight(1f)
                        )
                        QuotaMetricCard(
                            title = "Gemini 周配额",
                            bucket = acc.geminiWeekly,
                            modifier = Modifier.weight(1f)
                        )
                    }
                }
            }

            // Account Switch List
            val accounts = quotaData?.accounts ?: emptyList()
            if (accounts.size > 1) {
                Text(
                    text = "多账号切换 (${accounts.size})",
                    color = TextPrimary,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.padding(top = 8.dp)
                )

                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 200.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(accounts, key = { it.id }) { acc ->
                        val isCurr = acc.isCurrent || acc.id == current?.id
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(if (isCurr) DarkSurfaceVariant else DarkBackground)
                                .clickable { if (!isCurr) onSwitchAccount(acc.id) }
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Column {
                                Text(
                                    text = if (isMasked) acc.maskedEmail else acc.displayName,
                                    color = if (isCurr) AccentBlue else TextPrimary,
                                    fontSize = 13.sp,
                                    fontWeight = if (isCurr) FontWeight.Bold else FontWeight.Normal
                                )
                            }
                            if (isCurr) {
                                Icon(
                                    imageVector = Icons.Default.Check,
                                    contentDescription = "Active",
                                    tint = AccentBlue,
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun QuotaMetricCard(
    title: String,
    bucket: CockpitQuotaBucket?,
    modifier: Modifier = Modifier
) {
    val percent = bucket?.remainingPercent?.coerceIn(0.0, 100.0) ?: 0.0
    val color = when {
        percent >= 50 -> AccentGreen
        percent >= 20 -> AccentYellow
        else -> AccentRed
    }

    Column(
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .background(DarkBackground)
            .border(1.dp, DarkBorder, RoundedCornerShape(10.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Text(
            text = title,
            color = TextSecondary,
            fontSize = 12.sp
        )

        Row(
            verticalAlignment = Alignment.Bottom,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "${percent.toInt()}%",
                color = color,
                fontSize = 20.sp,
                fontWeight = FontWeight.Bold
            )
            bucket?.resetCountdownDisplay?.let {
                Text(
                    text = it,
                    color = TextMuted,
                    fontSize = 11.sp,
                    modifier = Modifier.padding(bottom = 2.dp)
                )
            }
        }

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(4.dp)
                .clip(CircleShape)
                .background(DarkBorder)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(fraction = (percent / 100.0).toFloat())
                    .fillMaxHeight()
                    .clip(CircleShape)
                    .background(color)
            )
        }
    }
}
