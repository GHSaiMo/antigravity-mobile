package com.antigravity.mobile.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme
import java.text.SimpleDateFormat
import java.util.*

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
    var isMasked by remember { mutableStateOf(false) }
    var pendingSwitchAccount by remember { mutableStateOf<CockpitAccountQuota?>(null) }
    val current = quotaData?.currentAccount
    val otherAccounts = remember(quotaData, current) {
        val all = quotaData?.accounts ?: emptyList()
        if (current != null) {
            all.filter { it.id != current.id }
        } else {
            all
        }
    }
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.background,
        dragHandle = {
            Box(
                modifier = Modifier
                    .padding(top = 10.dp, bottom = 8.dp)
                    .width(38.dp)
                    .height(5.dp)
                    .clip(CircleShape)
                    .background(colors.textMuted.copy(alpha = 0.35f))
            )
        },
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight(0.92f)
    ) {
        Column(
            modifier = Modifier.fillMaxSize()
        ) {
            // iOS-Style Top Bar: Mask toggle (Left), Centered Title, Refresh (Right) - No "完成" button needed
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 8.dp, vertical = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                IconButton(
                    onClick = { isMasked = !isMasked },
                    modifier = Modifier.size(40.dp)
                ) {
                    Icon(
                        imageVector = if (isMasked) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                        contentDescription = "Toggle Mask",
                        tint = if (isMasked) colors.accentBlue else colors.textSecondary,
                        modifier = Modifier.size(20.dp)
                    )
                }

                Text(
                    text = "Cockpit Tools",
                    fontWeight = FontWeight.Bold,
                    fontSize = 17.sp,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.weight(1f)
                )

                IconButton(
                    onClick = onRefresh,
                    enabled = !isRefreshing,
                    modifier = Modifier.size(40.dp)
                ) {
                    if (isRefreshing) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(18.dp),
                            strokeWidth = 2.dp,
                            color = colors.accentBlue
                        )
                    } else {
                        Icon(
                            imageVector = Icons.Default.Refresh,
                            contentDescription = "Refresh",
                            tint = colors.accentBlue,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }
            }

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f, fill = false)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 10.dp)
                    .navigationBarsPadding(),
                verticalArrangement = Arrangement.spacedBy(20.dp)
            ) {
                // Section 1: 当前使用账号
                if (current != null) {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = "当前使用账号",
                                color = colors.textSecondary,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            Row(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(colors.accentBlue.copy(alpha = 0.12f))
                                    .padding(horizontal = 8.dp, vertical = 3.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(6.dp)
                                        .clip(CircleShape)
                                        .background(colors.accentBlue)
                                )
                                Text(
                                    text = "使用中",
                                    color = colors.accentBlue,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                        }

                        AccountQuotaCard(
                            account = current,
                            isCurrent = true,
                            isMasked = isMasked,
                            onSwitch = null
                        )
                    }
                }

                // Section 2: 备用账号 (N)
                if (otherAccounts.isNotEmpty()) {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Text(
                            text = "备用账号 (${otherAccounts.size})",
                            color = colors.textSecondary,
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier.padding(horizontal = 4.dp)
                        )

                        otherAccounts.forEach { acc ->
                            AccountQuotaCard(
                                account = acc,
                                isCurrent = false,
                                isMasked = isMasked,
                                onSwitch = { pendingSwitchAccount = acc }
                            )
                        }
                    }
                }

                // Footer: Updated at time
                quotaData?.updatedAt?.takeIf { it > 0 }?.let { timestamp ->
                    val date = Date(timestamp)
                    val timeFmt = SimpleDateFormat("HH:mm:ss", Locale.getDefault())
                    Text(
                        text = "配额数据更新于 ${timeFmt.format(date)}",
                        color = colors.textMuted,
                        fontSize = 11.sp,
                        modifier = Modifier
                            .fillMaxWidth()
                            .wrapContentWidth(Alignment.CenterHorizontally)
                            .padding(top = 4.dp, bottom = 24.dp)
                    )
                }
            }
        }
    }

    // Switch account confirmation dialog
    pendingSwitchAccount?.let { acc ->
        AlertDialog(
            onDismissRequest = { pendingSwitchAccount = null },
            title = {
                Text(
                    text = "确认切换账号",
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = "切换到 ${if (isMasked) maskEmail(acc.email) else acc.email} 将关闭并重启电脑上的 Antigravity。",
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        val targetId = acc.id
                        pendingSwitchAccount = null
                        onSwitchAccount(targetId)
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentBlue),
                    shape = RoundedCornerShape(10.dp)
                ) {
                    Text("切换并重启 Antigravity", fontWeight = FontWeight.SemiBold)
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingSwitchAccount = null }) {
                    Text("取消", color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }
}

@Composable
private fun AccountQuotaCard(
    account: CockpitAccountQuota,
    isCurrent: Boolean,
    isMasked: Boolean,
    onSwitch: (() -> Unit)?
) {
    val colors = AntigravityTheme.colors

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .background(colors.surface)
            .border(
                if (isCurrent) 1.5.dp else 0.5.dp,
                if (isCurrent) colors.accentBlue.copy(alpha = 0.45f) else colors.border,
                RoundedCornerShape(14.dp)
            )
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Header row: Email + iOS-style Switch button for non-current accounts
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            val displayEmail = if (isMasked) maskEmail(account.email) else account.email
            Text(
                text = displayEmail,
                color = colors.textPrimary,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false)
            )

            if (!isCurrent && onSwitch != null) {
                Spacer(modifier = Modifier.width(8.dp))
                IosSwitchButton(onClick = onSwitch)
            }
        }

        HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)

        // 4 Metrics Grid: 1:1 iOS Layout Order!
        // Row 1: Left Claude 5h, Right Gemini 5h
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            QuotaMetricCell(
                title = "Claude 5h",
                bucket = account.claude5h,
                modifier = Modifier.weight(1f)
            )
            QuotaMetricCell(
                title = "Gemini 5h",
                bucket = account.gemini5h,
                modifier = Modifier.weight(1f)
            )
        }

        // Row 2: Left Claude Weekly, Right Gemini Weekly
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            QuotaMetricCell(
                title = "Claude Weekly",
                bucket = account.claudeWeekly,
                modifier = Modifier.weight(1f)
            )
            QuotaMetricCell(
                title = "Gemini Weekly",
                bucket = account.geminiWeekly,
                modifier = Modifier.weight(1f)
            )
        }
    }
}

@Composable
private fun QuotaMetricCell(
    title: String,
    bucket: CockpitQuotaBucket?,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val percent = bucket?.remainingPercent?.coerceIn(0.0, 100.0) ?: 0.0
    val color = when {
        percent >= 50 -> colors.accentGreen
        percent >= 20 -> colors.accentOrange
        else -> colors.accentRed
    }

    Column(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.5f))
            .padding(9.dp),
        verticalArrangement = Arrangement.spacedBy(5.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(
                text = title,
                color = colors.textSecondary,
                fontSize = 11.sp,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = String.format(Locale.US, "%.1f%%", percent),
                color = color,
                fontSize = 12.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace
            )
        }

        // Mini progress bar
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(5.dp)
                .clip(CircleShape)
                .background(colors.border.copy(alpha = 0.5f))
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(fraction = (percent / 100.0).toFloat().coerceIn(0f, 1f))
                    .fillMaxHeight()
                    .clip(CircleShape)
                    .background(color)
            )
        }

        // Countdown & reset description
        val displayText = bucket?.resetCountdownDisplay?.takeIf { it.isNotBlank() } ?: "配额充足"
        Text(
            text = displayText,
            color = if (displayText == "配额充足") colors.textMuted.copy(alpha = 0.7f) else colors.textSecondary,
            fontSize = 9.5.sp,
            maxLines = 1
        )
    }
}

@Composable
private fun IosSwitchButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val interactionSource = remember { MutableInteractionSource() }
    val isPressed by interactionSource.collectIsPressedAsState()
    val animatedAlpha by animateFloatAsState(
        targetValue = if (isPressed) 0.65f else 1f,
        label = "iosSwitchButtonAlpha"
    )

    Box(
        modifier = modifier
            .graphicsLayer { alpha = animatedAlpha }
            .clip(CircleShape)
            .background(colors.accentBlue)
            .clickable(
                interactionSource = interactionSource,
                indication = null, // Authentic iOS touch feedback: smooth opacity fade, no Android ink ripple
                onClick = onClick
            )
            .padding(horizontal = 12.dp, vertical = 4.5.dp),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "切换",
            color = Color.White,
            fontSize = 11.5.sp,
            fontWeight = FontWeight.SemiBold,
            letterSpacing = (-0.2).sp
        )
    }
}

private fun maskEmail(email: String): String {
    val parts = email.split("@")
    if (parts.size != 2) return email
    val local = parts[0]
    val domain = parts[1]

    val maskedLocal = if (local.length <= 2) {
        "${local.first()}*"
    } else {
        "${local.first()}${local.drop(1).dropLast(1).map { '*' }.joinToString("")}${local.last()}"
    }

    val dotIdx = domain.lastIndexOf('.')
    if (dotIdx > 0) {
        val domainName = domain.substring(0, dotIdx)
        val domainExt = domain.substring(dotIdx)
        val maskedDomain = if (domainName.length <= 2) {
            "${domainName.first()}*"
        } else {
            "${domainName.first()}${domainName.drop(1).dropLast(1).map { '*' }.joinToString("")}${domainName.last()}"
        }
        return "$maskedLocal@$maskedDomain$domainExt"
    }
    return "$maskedLocal@$domain"
}

