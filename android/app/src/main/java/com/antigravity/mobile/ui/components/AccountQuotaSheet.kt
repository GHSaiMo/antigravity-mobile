package com.antigravity.mobile.ui.components

import android.widget.Toast
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
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.layer.drawLayer
import androidx.compose.ui.graphics.rememberGraphicsLayer
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.CockpitQuotaResponse
import com.antigravity.mobile.data.model.DailyTokenRecord
import com.antigravity.mobile.data.model.TokenHistoryResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.ShareImageUtils
import java.text.SimpleDateFormat
import java.util.*
import kotlin.math.roundToInt
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountQuotaSheet(
    quotaData: CockpitQuotaResponse?,
    isRefreshing: Boolean,
    showWeeklyPaceCursor: Boolean = true,
    tokenHistory: TokenHistoryResponse? = null,
    isLoadingTokenHistory: Boolean = false,
    onLoadTokenHistory: (Boolean) -> Unit = {},
    onRefresh: () -> Unit,
    onSwitchAccount: (String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    var selectedTab by remember { mutableIntStateOf(0) }
    var isMasked by remember { mutableStateOf(false) }
    var isSharingTokenHistory by remember { mutableStateOf(false) }
    var selectedHeatmapDate by remember { mutableStateOf<String?>(null) }
    var pendingSwitchAccount by remember { mutableStateOf<CockpitAccountQuota?>(null) }
    var paceDetailTitleAndInfo by remember { mutableStateOf<Pair<String, com.antigravity.mobile.data.model.WeeklyPaceInfo>?>(null) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val density = LocalDensity.current
    val cardGraphicsLayer = rememberGraphicsLayer()
    val scaledCardLayer = rememberGraphicsLayer()

    val current = quotaData?.currentAccount
    val otherAccounts = remember(quotaData, current) {
        val all = quotaData?.accounts ?: emptyList()
        if (current != null) {
            all.filter { it.id != current.id }
        } else {
            all
        }
    }
    val selectedRecord = remember(tokenHistory?.dailyMatrix, selectedHeatmapDate) {
        val matrix = tokenHistory?.dailyMatrix ?: emptyList()
        val activeDate = selectedHeatmapDate
            ?: matrix.lastOrNull { it.total > 0 }?.date
            ?: matrix.lastOrNull()?.date
        matrix.firstOrNull { it.date == activeDate }
    }
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val tightTextStyle = TextStyle(platformStyle = PlatformTextStyle(includeFontPadding = false))

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
        Column(
            modifier = Modifier.fillMaxSize()
        ) {
            // Floating grab handle hinting pull-down dismissal (matching iOS Capsule 38x5)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 8.dp, bottom = 10.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            // Header title row: Mask toggle (Left), Centered Title, Refresh (Right)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 10.dp),
                contentAlignment = Alignment.Center
            ) {
                IconButton(
                    onClick = { isMasked = !isMasked },
                    modifier = Modifier
                        .align(Alignment.CenterStart)
                        .padding(start = 8.dp)
                        .size(34.dp)
                ) {
                    Icon(
                        imageVector = if (isMasked) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                        contentDescription = "Toggle Mask",
                        tint = if (isMasked) colors.accentBlue else colors.textSecondary,
                        modifier = Modifier.size(19.dp)
                    )
                }

                Text(
                    text = "账户与配额",
                    fontWeight = FontWeight.Bold,
                    fontSize = 17.sp,
                    lineHeight = 20.sp,
                    style = tightTextStyle,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .align(Alignment.Center)
                )

                if (selectedTab == 0) {
                    IconButton(
                        onClick = onRefresh,
                        enabled = !isRefreshing,
                        modifier = Modifier
                            .align(Alignment.CenterEnd)
                            .padding(end = 8.dp)
                            .size(34.dp)
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
                                modifier = Modifier.size(19.dp)
                            )
                        }
                    }
                } else {
                    IconButton(
                        onClick = {
                            if (tokenHistory == null || isSharingTokenHistory) return@IconButton
                            scope.launch {
                                isSharingTokenHistory = true
                                try {
                                    if (cardGraphicsLayer.size.width <= 0 || cardGraphicsLayer.size.height <= 0) {
                                        kotlinx.coroutines.delay(120)
                                    }
                                    val size = cardGraphicsLayer.size
                                    val maxPixelHeight = 8192
                                    val bitmap = if (size.height > maxPixelHeight) {
                                        val ratio = maxPixelHeight.toFloat() / size.height
                                        val target = androidx.compose.ui.unit.IntSize(
                                            (size.width * ratio).roundToInt().coerceAtLeast(1),
                                            maxPixelHeight
                                        )
                                        scaledCardLayer.record(density, androidx.compose.ui.unit.LayoutDirection.Ltr, target) {
                                            scale(ratio, pivot = Offset.Zero) {
                                                drawLayer(cardGraphicsLayer)
                                            }
                                        }
                                        scaledCardLayer.toImageBitmap().asAndroidBitmap()
                                    } else {
                                        cardGraphicsLayer.toImageBitmap().asAndroidBitmap()
                                    }
                                    ShareImageUtils.shareBitmap(context, bitmap)
                                } catch (e: Throwable) {
                                    android.util.Log.e("AccountQuotaSheet", "Share token history failed", e)
                                    Toast.makeText(context, "长图生成失败: ${e.localizedMessage}", Toast.LENGTH_SHORT).show()
                                } finally {
                                    isSharingTokenHistory = false
                                }
                            }
                        },
                        enabled = tokenHistory != null && !isLoadingTokenHistory && !isSharingTokenHistory,
                        modifier = Modifier
                            .align(Alignment.CenterEnd)
                            .padding(end = 8.dp)
                            .size(34.dp)
                    ) {
                        if (isLoadingTokenHistory || isSharingTokenHistory) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(18.dp),
                                strokeWidth = 2.dp,
                                color = colors.accentBlue
                            )
                        } else {
                            Icon(
                                imageVector = Icons.Default.Share,
                                contentDescription = "Share",
                                tint = if (tokenHistory == null) colors.textMuted else colors.accentBlue,
                                modifier = Modifier.size(19.dp)
                            )
                        }
                    }
                }
            }

            // Segmented tabs: 实时配额 | Token 历史
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 10.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.textMuted.copy(alpha = 0.12f))
                    .padding(3.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                listOf("实时配额", "Token 历史").forEachIndexed { idx, label ->
                    val active = selectedTab == idx
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .clip(RoundedCornerShape(8.dp))
                            .background(if (active) colors.surface else Color.Transparent)
                            .clickable {
                                selectedTab = idx
                                if (idx == 1 && tokenHistory == null) {
                                    onLoadTokenHistory(false)
                                }
                            }
                            .padding(vertical = 6.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = label,
                            fontSize = 13.sp,
                            fontWeight = if (active) FontWeight.SemiBold else FontWeight.Medium,
                            color = if (active) colors.textPrimary else colors.textSecondary,
                            style = tightTextStyle
                        )
                    }
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f, fill = false)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 12.dp)
                    .navigationBarsPadding(),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                if (selectedTab == 1) {
                    TokenHistoryTabContent(
                        tokenHistory = tokenHistory,
                        isLoading = isLoadingTokenHistory,
                        onRetry = { onLoadTokenHistory(true) },
                        selectedDate = selectedHeatmapDate,
                        onSelectDate = { selectedHeatmapDate = it }
                    )
                } else {
                // Section 1: 当前使用账号
                if (current != null) {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
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
                                lineHeight = 16.sp,
                                fontWeight = FontWeight.SemiBold,
                                style = tightTextStyle
                            )
                            Row(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(colors.accentBlue.copy(alpha = 0.12f))
                                    .padding(horizontal = 8.dp, vertical = 2.5.dp),
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
                                    lineHeight = 13.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    style = tightTextStyle
                                )
                            }
                        }

                        AccountQuotaCard(
                            account = current,
                            isCurrent = true,
                            isMasked = isMasked,
                            showWeeklyPaceCursor = showWeeklyPaceCursor,
                            onWeeklyClick = { title, info -> paceDetailTitleAndInfo = Pair(title, info) },
                            onSwitch = null
                        )
                    }
                }

                // Section 2: Cockpit 未配置时给出安装提示
                if (quotaData?.needsCockpitInstall == true) {
                    val clipboard = androidx.compose.ui.platform.LocalClipboardManager.current
                    Column(verticalArrangement = Arrangement.spacedBy(9.dp)) {
                        Text(
                            text = "备用账号",
                            color = colors.textSecondary,
                            fontSize = 13.sp,
                            lineHeight = 16.sp,
                            fontWeight = FontWeight.SemiBold,
                            style = tightTextStyle,
                            modifier = Modifier.padding(horizontal = 4.dp)
                        )
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(14.dp))
                                .background(colors.textMuted.copy(alpha = 0.12f))
                                .padding(14.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Text(
                                text = "未检测到 Cockpit Tools",
                                color = colors.textSecondary,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            Text(
                                text = "请安装 Cockpit Tools,并在网关里用 mgy cockpit 进行配置,即可享受备用账号额度总览与一键切换等功能。",
                                color = colors.textMuted,
                                fontSize = 13.sp,
                                lineHeight = 18.sp
                            )
                            Text(
                                text = "复制命令 mgy cockpit",
                                color = colors.textSecondary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.clickable {
                                    clipboard.setText(androidx.compose.ui.text.AnnotatedString("mgy cockpit"))
                                }
                            )
                        }
                    }
                } else if (otherAccounts.isNotEmpty()) {
                    Column(verticalArrangement = Arrangement.spacedBy(9.dp)) {
                        Text(
                            text = "备用账号 (${otherAccounts.size})",
                            color = colors.textSecondary,
                            fontSize = 13.sp,
                            lineHeight = 16.sp,
                            fontWeight = FontWeight.SemiBold,
                            style = tightTextStyle,
                            modifier = Modifier.padding(horizontal = 4.dp)
                        )

                        otherAccounts.forEach { acc ->
                            AccountQuotaCard(
                                account = acc,
                                isCurrent = false,
                                isMasked = isMasked,
                                showWeeklyPaceCursor = showWeeklyPaceCursor,
                                onWeeklyClick = { title, info -> paceDetailTitleAndInfo = Pair(title, info) },
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
                        lineHeight = 14.sp,
                        style = tightTextStyle,
                        modifier = Modifier
                            .fillMaxWidth()
                            .wrapContentWidth(Alignment.CenterHorizontally)
                            .padding(top = 2.dp, bottom = 20.dp)
                    )
                }
                }
            }

            // 离屏高清长图海报录制容器（0 尺寸占位，屏幕外布局，完整测量并绘制到 GraphicsLayer）
            if (tokenHistory != null) {
                Box(
                    modifier = Modifier
                        .layout { measurable, _ ->
                            val widthPx = (TOKEN_HISTORY_SHARE_CARD_WIDTH.toPx()).roundToInt()
                            val placeable = measurable.measure(
                                Constraints(
                                    minWidth = widthPx,
                                    maxWidth = widthPx,
                                    minHeight = 0,
                                    maxHeight = Constraints.Infinity
                                )
                            )
                            layout(0, 0) {
                                placeable.place(-10000, -10000)
                            }
                        }
                        .drawWithContent {
                            cardGraphicsLayer.record {
                                this@drawWithContent.drawContent()
                            }
                        }
                ) {
                    TokenHistoryShareCard(
                        tokenHistory = tokenHistory,
                        selectedRecord = selectedRecord
                    )
                }
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

    paceDetailTitleAndInfo?.let { (title, info) ->
        WeeklyPaceDetailDialog(
            title = title,
            info = info,
            onDismiss = { paceDetailTitleAndInfo = null }
        )
    }
}

private val CompactPlatformTextStyle = PlatformTextStyle(includeFontPadding = false)
private val CompactTextStyle = TextStyle(platformStyle = CompactPlatformTextStyle)

@Composable
private fun AccountQuotaCard(
    account: CockpitAccountQuota,
    isCurrent: Boolean,
    isMasked: Boolean,
    showWeeklyPaceCursor: Boolean = true,
    onWeeklyClick: ((String, com.antigravity.mobile.data.model.WeeklyPaceInfo) -> Unit)? = null,
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
            .padding(horizontal = 12.dp, vertical = 11.dp),
        verticalArrangement = Arrangement.spacedBy(9.dp)
    ) {
        // Header row: Email + Tier Badge + iOS-style Switch button for non-current accounts
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                modifier = Modifier.weight(1f),
                verticalAlignment = Alignment.CenterVertically
            ) {
                val displayEmail = if (isMasked) maskEmail(account.email) else account.email
                Text(
                    text = displayEmail,
                    color = colors.textPrimary,
                    fontSize = 15.sp,
                    lineHeight = 18.sp,
                    fontWeight = FontWeight.SemiBold,
                    style = CompactTextStyle,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
                TierBadge(tier = account.subscriptionTier)
            }

            if (!isCurrent && onSwitch != null) {
                Spacer(modifier = Modifier.width(8.dp))
                IosSwitchButton(onClick = onSwitch)
            }
        }

        HorizontalDivider(color = colors.separator.copy(alpha = 0.25f), thickness = 0.5.dp)

        val isFree = account.subscriptionTier?.trim()?.equals("FREE", ignoreCase = true) == true ||
            (account.claude5h == null && account.gemini5h == null)

        // Metrics Grid: Free accounts show 2 weekly quotas; others show 4 metrics (5h + weekly)
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (isFree) {
                // Free 账号仅展示两个 weekly 数值：Row 1: Left Gemini · Weekly, Right Claude · Weekly
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    QuotaMetricCell(
                        title = "Gemini · Weekly",
                        bucket = account.geminiWeekly,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Gemini · Weekly", it) } },
                        modifier = Modifier.weight(1f)
                    )
                    QuotaMetricCell(
                        title = "Claude · Weekly",
                        bucket = account.claudeWeekly,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Claude · Weekly", it) } },
                        modifier = Modifier.weight(1f)
                    )
                }
            } else {
                // Row 1: Gemini (5h in left, Weekly in right)
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    QuotaMetricCell(
                        title = "Gemini · 5h",
                        bucket = account.gemini5h,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Gemini · 5h", it) } },
                        modifier = Modifier.weight(1f)
                    )
                    QuotaMetricCell(
                        title = "Gemini · Weekly",
                        bucket = account.geminiWeekly,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Gemini · Weekly", it) } },
                        modifier = Modifier.weight(1f)
                    )
                }

                // Row 2: Claude (5h in left, Weekly in right)
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    QuotaMetricCell(
                        title = "Claude · 5h",
                        bucket = account.claude5h,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Claude · 5h", it) } },
                        modifier = Modifier.weight(1f)
                    )
                    QuotaMetricCell(
                        title = "Claude · Weekly",
                        bucket = account.claudeWeekly,
                        tier = account.subscriptionTier,
                        showPaceCursor = showWeeklyPaceCursor,
                        onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Claude · Weekly", it) } },
                        modifier = Modifier.weight(1f)
                    )
                }

                val isUltra = account.subscriptionTier?.trim()?.equals("ULTRA", ignoreCase = true) == true
                if (isUltra) {
                    // Ultra 专属通栏：展示 Gemini 4 Carbon Weekly，数据对齐 geminiWeekly
                    val carbonBucket = account.geminiWeekly ?: account.gemini4Argon
                    carbonBucket?.let { carbon ->
                        val carbonPaceInfo = if (showWeeklyPaceCursor) carbon.calculatePaceForTitle("Gemini 4 Carbon Weekly", account.subscriptionTier) else null
                        QuotaMetricCell(
                            title = "Gemini 4 Carbon Weekly",
                            bucket = carbon,
                            isCarbon = true,
                            tier = account.subscriptionTier,
                            showPaceCursor = showWeeklyPaceCursor && carbonPaceInfo != null,
                            onWeeklyClick = onWeeklyClick?.let { cb -> { cb("Gemini 4 Carbon Weekly", it) } },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }
        }
    }
}

private fun carbonAuroraColors(isDark: Boolean): List<Color> = if (isDark) {
    listOf(Color(0xFF4285F4), Color(0xFF9B72CF), Color(0xFFFF7597))
} else {
    listOf(Color(0xFF4285F4), Color(0xFF9B72CF), Color(0xFFF06292))
}

@Composable
private fun TierBadge(tier: String?) {
    val norm = tier?.trim()?.uppercase(Locale.US) ?: return
    val colors = AntigravityTheme.colors
    when (norm) {
        "ULTRA" -> {
            Spacer(modifier = Modifier.width(6.dp))
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(5.dp))
                    .background(Brush.horizontalGradient(colors = carbonAuroraColors(colors.isDark)))
                    .padding(horizontal = 6.dp, vertical = 2.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "ULTRA",
                    color = Color.White,
                    fontSize = 9.5.sp,
                    lineHeight = 11.sp,
                    fontWeight = FontWeight.ExtraBold,
                    letterSpacing = 0.5.sp,
                    style = TextStyle(
                        platformStyle = CompactPlatformTextStyle,
                        shadow = Shadow(
                            color = Color(0x554C1D95),
                            offset = Offset(0f, 0.8f),
                            blurRadius = 1.5f
                        )
                    )
                )
            }
        }
        "PRO" -> {
            Spacer(modifier = Modifier.width(6.dp))
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(5.dp))
                    .background(
                        Brush.linearGradient(
                            colors = listOf(Color(0xFF1D4ED8), Color(0xFF0EA5A5))
                        )
                    )
                    .padding(horizontal = 6.dp, vertical = 2.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "PRO",
                    color = Color.White,
                    fontSize = 9.5.sp,
                    lineHeight = 11.sp,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = 0.4.sp,
                    style = CompactTextStyle
                )
            }
        }
        "FREE" -> {
            Spacer(modifier = Modifier.width(6.dp))
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(5.dp))
                    .background(colors.textSecondary.copy(alpha = 0.16f))
                    .border(0.5.dp, colors.textSecondary.copy(alpha = 0.32f), RoundedCornerShape(5.dp))
                    .padding(horizontal = 6.dp, vertical = 2.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "FREE",
                    color = colors.textSecondary,
                    fontSize = 9.5.sp,
                    lineHeight = 11.sp,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = 0.4.sp,
                    style = CompactTextStyle
                )
            }
        }
    }
}

@Composable
private fun QuotaMetricCell(
    title: String,
    bucket: CockpitQuotaBucket?,
    isArgon: Boolean = false,
    isCarbon: Boolean = false,
    tier: String? = null,
    showPaceCursor: Boolean = true,
    onWeeklyClick: ((com.antigravity.mobile.data.model.WeeklyPaceInfo) -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val percent = bucket?.remainingPercent?.coerceIn(0.0, 100.0) ?: 0.0
    val isPaceSupported = title.contains("Weekly", ignoreCase = true) ||
        title.contains("5h", ignoreCase = true) ||
        isArgon ||
        isCarbon ||
        title.contains("Argon", ignoreCase = true) ||
        title.contains("Carbon", ignoreCase = true)
    val paceInfo = remember(bucket, title, tier) {
        if (isPaceSupported) bucket?.calculatePaceForTitle(title, tier) else null
    }
    val argonAccent = if (colors.isDark) Color(0xFFFF9F0A) else Color(0xFFD97706)
    val carbonAccent = if (colors.isDark) Color(0xFFC58AF9) else Color(0xFF7C3AED)
    val carbonAuroraColors = carbonAuroraColors(colors.isDark)
    val carbonTitleColors = if (colors.isDark) {
        listOf(Color(0xFF8AB4F8), Color(0xFFC58AF9), Color(0xFFFF8BCB))
    } else {
        listOf(Color(0xFF2563EB), Color(0xFF7C3AED), Color(0xFFDB2777))
    }
    val isDanger = percent < 20.0
    val color = when {
        isCarbon -> if (isDanger) colors.accentRed else carbonAccent
        isArgon -> if (isDanger) colors.accentRed else argonAccent
        percent >= 50 -> colors.accentGreen
        percent >= 20 -> colors.accentOrange
        else -> colors.accentRed
    }
    val isSpecial = isArgon || isCarbon
    val cellShape = RoundedCornerShape(if (isSpecial) 10.dp else 8.dp)
    var baseModifier = when {
        isCarbon -> {
            modifier
                .clip(cellShape)
                .background(
                    Brush.linearGradient(
                        colors = listOf(
                            Color(0xFF4285F4).copy(alpha = if (colors.isDark) 0.12f else 0.08f),
                            Color(0xFF9B72CF).copy(alpha = if (colors.isDark) 0.10f else 0.07f),
                            Color(0xFFF06292).copy(alpha = if (colors.isDark) 0.08f else 0.06f)
                        )
                    )
                )
                .border(
                    1.dp,
                    Brush.linearGradient(
                        colors = listOf(
                            Color(0xFF4285F4).copy(alpha = if (colors.isDark) 0.45f else 0.42f),
                            Color(0xFF9B72CF).copy(alpha = if (colors.isDark) 0.42f else 0.38f),
                            Color(0xFFF06292).copy(alpha = if (colors.isDark) 0.38f else 0.36f)
                        )
                    ),
                    cellShape
                )
                .padding(horizontal = 11.dp, vertical = 10.dp)
        }
        isArgon -> {
            modifier
                .clip(cellShape)
                .background(
                    Brush.linearGradient(
                        colors = listOf(
                            Color(0xFFFF9F0A).copy(alpha = if (colors.isDark) 0.13f else 0.11f),
                            Color(0xFFF97316).copy(alpha = if (colors.isDark) 0.06f else 0.05f)
                        )
                    )
                )
                .border(
                    1.dp,
                    Brush.linearGradient(
                        colors = listOf(
                            Color(0xFFFFCA28).copy(alpha = if (colors.isDark) 0.42f else 0.48f),
                            Color(0xFFFF9F0A).copy(alpha = if (colors.isDark) 0.22f else 0.28f)
                        )
                    ),
                    cellShape
                )
                .padding(horizontal = 11.dp, vertical = 10.dp)
        }
        else -> {
            modifier
                .clip(cellShape)
                .background(colors.surfaceVariant.copy(alpha = if (colors.isDark) 0.85f else 0.55f))
                .padding(horizontal = 9.dp, vertical = 9.5.dp)
        }
    }

    if (isPaceSupported && paceInfo != null && onWeeklyClick != null) {
        baseModifier = baseModifier.clickable { onWeeklyClick(paceInfo) }
    }

    Column(
        modifier = baseModifier,
        verticalArrangement = Arrangement.spacedBy(if (isSpecial) 5.5.dp else 5.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(5.dp),
                modifier = Modifier.weight(1f, fill = false)
            ) {
                if (isCarbon) {
                    Text(
                        text = buildAnnotatedString {
                            withStyle(SpanStyle(brush = Brush.horizontalGradient(carbonTitleColors))) {
                                append(title)
                            }
                        },
                        fontSize = 11.5.sp,
                        lineHeight = 13.sp,
                        fontWeight = FontWeight.SemiBold,
                        style = CompactTextStyle,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                } else {
                    Text(
                        text = title,
                        color = if (isArgon) argonAccent else colors.textSecondary,
                        fontSize = if (isSpecial) 11.5.sp else 11.sp,
                        lineHeight = 13.sp,
                        fontWeight = if (isSpecial) FontWeight.SemiBold else FontWeight.Medium,
                        style = CompactTextStyle,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
            Spacer(modifier = Modifier.width(6.dp))
            Text(
                text = String.format(Locale.US, "%.1f%%", percent),
                color = color,
                fontSize = 12.sp,
                lineHeight = 13.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace,
                style = CompactTextStyle
            )
        }

        // Mini progress bar with quota pace cursor
        val hasCursor = isPaceSupported && showPaceCursor && paceInfo != null
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(if (hasCursor) 7.5.dp else 4.5.dp),
            contentAlignment = Alignment.CenterStart
        ) {
            // Track base
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(4.5.dp)
                    .clip(CircleShape)
                    .background(
                        when {
                            isCarbon -> Color(0xFF9B72CF).copy(alpha = if (colors.isDark) 0.18f else 0.14f)
                            isArgon -> Color(0xFFFF9F0A).copy(alpha = if (colors.isDark) 0.16f else 0.18f)
                            else -> colors.border.copy(alpha = 0.5f)
                        }
                    )
            ) {
                val fillModifier = Modifier
                    .fillMaxWidth(fraction = (percent / 100.0).toFloat().coerceIn(0f, 1f))
                    .fillMaxHeight()
                    .clip(CircleShape)
                Box(
                    modifier = when {
                        isCarbon && !isDanger -> {
                            fillModifier.background(Brush.horizontalGradient(colors = carbonAuroraColors))
                        }
                        isArgon && !isDanger -> {
                            fillModifier.background(
                                Brush.horizontalGradient(
                                    colors = listOf(Color(0xFFFF9F0A), Color(0xFFFFD60A))
                                )
                            )
                        }
                        else -> {
                            fillModifier.background(color)
                        }
                    }
                )
            }

            // Vertical Pace Cursor Needle
            if (hasCursor) {
                val expectedFrac = (paceInfo.expectedRemainingPercent / 100.0).toFloat().coerceIn(0f, 1f)
                val needleColor = if (paceInfo.isOverPacing) Color(0xFFFF9F0A) else Color.White.copy(alpha = 0.92f)
                BoxWithConstraints(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(7.5.dp),
                    contentAlignment = Alignment.CenterStart
                ) {
                    val maxW = maxWidth
                    val needleOffset = (maxW * expectedFrac - 0.75.dp).coerceIn(0.dp, (maxW - 1.5.dp).coerceAtLeast(0.dp))
                    Box(
                        modifier = Modifier
                            .padding(start = needleOffset)
                            .size(width = 1.5.dp, height = 7.5.dp)
                            .clip(RoundedCornerShape(0.75.dp))
                            .background(needleColor)
                    )
                }
            }
        }

        // Countdown & reset description
        val displayText = bucket?.resetCountdownDisplay?.takeIf { it.isNotBlank() } ?: "配额充足"
        Text(
            text = displayText,
            color = if (displayText == "配额充足") colors.textMuted.copy(alpha = 0.7f) else colors.textSecondary,
            fontSize = 9.5.sp,
            lineHeight = 11.sp,
            style = CompactTextStyle,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun WeeklyPaceDetailDialog(
    title: String,
    info: com.antigravity.mobile.data.model.WeeklyPaceInfo,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(10.dp)
                        .clip(CircleShape)
                        .background(if (info.isOverPacing) Color(0xFFFF9F0A) else colors.accentGreen)
                )
                Text(
                    text = "$title 消耗节奏分析",
                    fontSize = 17.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary
                )
            }
        },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                // 状态卡片
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(
                            if (info.isOverPacing) Color(0xFFFF9F0A).copy(alpha = 0.12f)
                            else colors.accentGreen.copy(alpha = 0.12f)
                        )
                        .border(
                            1.dp,
                            if (info.isOverPacing) Color(0xFFFF9F0A).copy(alpha = 0.35f)
                            else colors.accentGreen.copy(alpha = 0.35f),
                            RoundedCornerShape(10.dp)
                        )
                        .padding(12.dp)
                ) {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            text = if (info.isOverPacing) "⚠️ 消耗偏快 (超速透支)" else "✅ 节奏健康 (额度充裕)",
                            fontSize = 14.sp,
                            fontWeight = FontWeight.Bold,
                            color = if (info.isOverPacing) Color(0xFFFF9F0A) else colors.accentGreen
                        )
                        Text(
                            text = if (info.isOverPacing) {
                                "当前额度已消耗 ${String.format(Locale.US, "%.1f%%", info.usedPercent)}，快于 ${info.windowLabel}时间匀速基准线 (${String.format(Locale.US, "%.1f%%", info.expectedUsedPercent)})。若保持此速率可能在周期结束前提前耗尽。"
                            } else {
                                "当前额度已消耗 ${String.format(Locale.US, "%.1f%%", info.usedPercent)}，低于或符合 ${info.windowLabel}时间匀速基准线 (${String.format(Locale.US, "%.1f%%", info.expectedUsedPercent)})。剩余额度充足，无需担心断粮。"
                            },
                            fontSize = 12.sp,
                            lineHeight = 16.sp,
                            color = colors.textSecondary
                        )
                    }
                }

                // 数值对比明细表
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.surfaceVariant.copy(alpha = 0.6f))
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    PaceDetailRow("当前实际剩余", String.format(Locale.US, "%.1f%%", info.actualRemainingPercent), colors.textPrimary)
                    PaceDetailRow("当前实际已用", String.format(Locale.US, "%.1f%%", info.usedPercent), colors.textPrimary)
                    HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.4f))
                    PaceDetailRow("${info.windowLabel}匀速应剩 (游标位)", String.format(Locale.US, "%.1f%%", info.expectedRemainingPercent), colors.textSecondary)
                    PaceDetailRow("${info.windowLabel}匀速应耗基准", String.format(Locale.US, "%.1f%%", info.expectedUsedPercent), colors.textSecondary)
                    HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.4f))
                    PaceDetailRow(
                        label = "节奏差值 (实际 - 理论)",
                        value = (if (info.paceDeltaPercent >= 0) "+" else "") + String.format(Locale.US, "%.1f%%", info.paceDeltaPercent),
                        color = if (info.isOverPacing) Color(0xFFFF9F0A) else colors.accentGreen
                    )
                    PaceDetailRow("重置倒计时", info.resetCountdownDisplay, colors.textSecondary)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) {
                Text("知道了", fontWeight = FontWeight.SemiBold, color = colors.accentBlue)
            }
        },
        containerColor = colors.surface
    )
}

@Composable
private fun PaceDetailRow(label: String, value: String, color: Color) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(text = label, fontSize = 12.sp, color = AntigravityTheme.colors.textSecondary)
        Text(text = value, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = color)
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
            .padding(horizontal = 10.dp, vertical = 3.5.dp),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "切换",
            color = Color.White,
            fontSize = 11.sp,
            lineHeight = 13.sp,
            fontWeight = FontWeight.SemiBold,
            letterSpacing = (-0.2).sp,
            style = CompactTextStyle
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

