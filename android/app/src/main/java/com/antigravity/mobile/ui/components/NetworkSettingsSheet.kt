package com.antigravity.mobile.ui.components

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.EndpointHealthStatus
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.HapticUtils
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NetworkSettingsSheet(
    prefs: PreferencesManager,
    connectionManager: ConnectionManager,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.background,
        dragHandle = null,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight(0.94f)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight()
                .imePadding()
        ) {
            // Floating grab handle hinting pull-down dismissal (matching iOS Capsule 38x5)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 10.dp, bottom = 18.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            // Header title row: centered title, back button on left, no "完成" button
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp),
                contentAlignment = Alignment.Center
            ) {
                IconButton(
                    onClick = onDismiss,
                    modifier = Modifier
                        .align(Alignment.CenterStart)
                        .padding(start = 8.dp)
                        .size(36.dp)
                ) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = "Back",
                        tint = colors.accentIndigo
                    )
                }

                Text(
                    text = "网络设置",
                    fontWeight = FontWeight.Bold,
                    fontSize = 18.sp,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .align(Alignment.Center)
                )
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
            ) {
                NetworkSettingsContent(
                    prefs = prefs,
                    connectionManager = connectionManager
                )
            }
        }
    }
}

@Composable
fun NetworkSettingsContent(
    prefs: PreferencesManager,
    connectionManager: ConnectionManager,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()

    var activeUrl by remember { mutableStateOf(prefs.gatewayBaseUrl ?: "") }
    var cloudUrl by remember { mutableStateOf(prefs.primaryCloudUrl ?: "") }
    var customUrl by remember { mutableStateOf(prefs.customServerUrl ?: prefs.lanServerUrl ?: "") }

    LaunchedEffect(activeUrl) {
        if (cloudUrl.isBlank() && activeUrl.isNotBlank() && !ConnectionManager.isLanHost(ConnectionManager.extractHost(activeUrl))) {
            cloudUrl = activeUrl
            prefs.primaryCloudUrl = activeUrl
        }
        if (customUrl.isBlank() && prefs.lanServerUrl?.isNotBlank() == true) {
            customUrl = prefs.lanServerUrl ?: ""
            prefs.customServerUrl = customUrl
        }
    }

    val isProbing by connectionManager.isProbing.collectAsState()
    val endpointStatuses by connectionManager.endpointStatuses.collectAsState()

    var isTesting by remember { mutableStateOf(false) }
    var testResultText by remember { mutableStateOf<String?>(null) }
    var testSuccess by remember { mutableStateOf<Boolean?>(null) }

    val currentStatus = endpointStatuses[activeUrl]
    val activeLatency = currentStatus?.takeIf { it.isReachable }?.latencyMs

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // MARK: - 1. 当前活动链路
        SettingsGroupSection(
            title = "当前活动链路",
            footer = null
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "当前通道", color = colors.textPrimary, fontSize = 15.sp)
                    Text(
                        text = if (activeUrl.isNotBlank()) connectionManager.describeEndpoint(activeUrl) else "未连接",
                        color = colors.textSecondary,
                        fontSize = 13.sp
                    )
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "连接状态", color = colors.textPrimary, fontSize = 15.sp)
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(7.dp)
                                .clip(CircleShape)
                                .background(if (activeUrl.isNotBlank()) colors.accentGreen else colors.textMuted)
                        )
                        Text(
                            text = if (activeUrl.isNotBlank()) "已连接" else "未连接",
                            color = colors.textSecondary,
                            fontSize = 13.sp
                        )
                    }
                }

                if (activeLatency != null) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(text = "网络延迟", color = colors.textPrimary, fontSize = 15.sp)
                        Text(
                            text = "${activeLatency}ms",
                            color = getLatencyColor(activeLatency, colors),
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                }

                if (activeUrl.isNotBlank()) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            text = activeUrl,
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            fontFamily = FontFamily.Monospace,
                            modifier = Modifier.weight(1f)
                        )
                        IconButton(
                            onClick = {
                                val clip = ClipData.newPlainText("Gateway URL", activeUrl)
                                (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(clip)
                                HapticUtils.lightTap(context)
                                Toast.makeText(context, "地址已复制到剪贴板", Toast.LENGTH_SHORT).show()
                            },
                            modifier = Modifier.size(24.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.ContentCopy,
                                contentDescription = "Copy",
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(15.dp)
                            )
                        }
                    }
                }
            }
        }

        // MARK: - 2. 主连接 · 专属公网域名 (Cloudflare HTTPS)
        SettingsGroupSection(
            title = "主连接 · 专属公网域名",
            footer = "默认统一使用专属分配的 HTTPS 域名。无论在外使用蜂窝网络还是 Wi-Fi，无需 VPN 即可安全直连电脑。"
        ) {
            val isCloudActive = ConnectionManager.isSameEndpoint(activeUrl, cloudUrl)
            val cloudStatus = endpointStatuses[cloudUrl] ?: endpointStatuses[cloudUrl.trim().trimEnd('/')]

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "Cloudflare 专属域名", color = colors.textPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        if (isCloudActive) {
                            Text(
                                text = "默认首选 (生效中)",
                                color = colors.accentGreen,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        } else if (cloudUrl.isNotBlank()) {
                            Text(
                                text = "设为主连接",
                                color = colors.accentIndigo,
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier
                                    .clip(RoundedCornerShape(4.dp))
                                    .clickable {
                                        activeUrl = cloudUrl
                                        prefs.gatewayBaseUrl = cloudUrl
                                        HapticUtils.lightTap(context)
                                        Toast.makeText(context, "已切换为主公网域名", Toast.LENGTH_SHORT).show()
                                    }
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
                            )
                        }
                        if (cloudStatus != null) {
                            if (cloudStatus.isReachable) {
                                Text(
                                    text = "${cloudStatus.latencyMs}ms",
                                    color = getLatencyColor(cloudStatus.latencyMs, colors),
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    fontFamily = FontFamily.Monospace
                                )
                            } else {
                                Text(text = "不可达", color = colors.accentRed, fontSize = 11.sp)
                            }
                        }
                    }
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = if (cloudUrl.isNotBlank()) cloudUrl else "未配置专属公网域名 (扫码配对自动下发)",
                        color = if (cloudUrl.isNotBlank()) colors.textSecondary else colors.textMuted,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier.weight(1f)
                    )
                    if (cloudUrl.isNotBlank()) {
                        IconButton(
                            onClick = {
                                val clip = ClipData.newPlainText("Cloudflare URL", cloudUrl)
                                (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(clip)
                                HapticUtils.lightTap(context)
                                Toast.makeText(context, "公网域名已复制", Toast.LENGTH_SHORT).show()
                            },
                            modifier = Modifier.size(24.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.ContentCopy,
                                contentDescription = "Copy",
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(15.dp)
                            )
                        }
                    }
                }
            }
        }

        // MARK: - 3. 备用连接 · 自定义与局域网
        SettingsGroupSection(
            title = "备用连接 · 自定义与局域网",
            footer = "扫码配对默认已填入电脑局域网 Wi-Fi 地址。在同一 Wi-Fi 下可手动切换为此通道，享受 0 延迟响应。"
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                EndpointItemRow(
                    title = "局域网 Wi-Fi / 自定义地址",
                    placeholder = "如 http://192.168.1.50:58900",
                    value = customUrl,
                    isActive = ConnectionManager.isSameEndpoint(activeUrl, customUrl),
                    probeStatus = endpointStatuses[customUrl] ?: endpointStatuses[customUrl.trim().trimEnd('/')],
                    onValueChange = {
                        customUrl = it
                        prefs.customServerUrl = it.ifBlank { null }
                        if (activeUrl.isBlank() && it.isNotBlank()) {
                            activeUrl = it
                            prefs.gatewayBaseUrl = it
                        }
                    },
                    onSelectActive = {
                        activeUrl = customUrl
                        prefs.gatewayBaseUrl = customUrl
                        HapticUtils.lightTap(context)
                        Toast.makeText(context, "已切换为自定义/局域网连接", Toast.LENGTH_SHORT).show()
                    }
                )
            }
        }

        // MARK: - 4. 链路探活与智能测速
        SettingsGroupSection(
            title = "通道测速与链路检查",
            footer = "并发探测专属公网域名与备用地址的健康状态并回显最新往返延迟。"
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "一键并发测速与探活", color = colors.textPrimary, fontSize = 15.sp)
                    Button(
                        onClick = {
                            isTesting = true
                            testResultText = "正在探测各通道延迟..."
                            testSuccess = null
                            HapticUtils.lightTap(context)

                            coroutineScope.launch {
                                val winner = connectionManager.probeEndpoints(prefs)
                                if (winner != null) {
                                    activeUrl = winner
                                    cloudUrl = prefs.primaryCloudUrl ?: ""
                                    customUrl = prefs.customServerUrl ?: prefs.lanServerUrl ?: ""
                                    val statusRes = connectionManager.testGatewayStatus(winner)
                                    statusRes.onSuccess { msg ->
                                        testResultText = msg
                                        testSuccess = true
                                        HapticUtils.heavyClick(context)
                                    }.onFailure { err ->
                                        testResultText = "探测失败: ${err.message}"
                                        testSuccess = false
                                        HapticUtils.heavyClick(context)
                                    }
                                } else {
                                    testResultText = "未找到可用端点，请检查网络"
                                    testSuccess = false
                                    HapticUtils.heavyClick(context)
                                }
                                isTesting = false
                            }
                        },
                        enabled = !isTesting && !isProbing,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = colors.accentIndigo.copy(alpha = 0.15f),
                            contentColor = colors.accentIndigo
                        ),
                        shape = RoundedCornerShape(8.dp)
                    ) {
                        if (isTesting || isProbing) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(14.dp),
                                strokeWidth = 2.dp,
                                color = colors.accentIndigo
                            )
                        } else {
                            Text("开始测速", fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                        }
                    }
                }

                AnimatedVisibility(
                    visible = testResultText != null,
                    enter = fadeIn(),
                    exit = fadeOut()
                ) {
                    testResultText?.let { msg ->
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            if (testSuccess == true) {
                                Icon(
                                    imageVector = Icons.Default.CheckCircle,
                                    contentDescription = "Success",
                                    tint = colors.accentGreen,
                                    modifier = Modifier.size(14.dp)
                                )
                            } else if (testSuccess == false) {
                                Icon(
                                    imageVector = Icons.Default.Warning,
                                    contentDescription = "Failed",
                                    tint = colors.accentRed,
                                    modifier = Modifier.size(14.dp)
                                )
                            }
                            Text(
                                text = msg,
                                color = when (testSuccess) {
                                    true -> colors.accentGreen
                                    false -> colors.accentRed
                                    else -> colors.textSecondary
                                },
                                fontSize = 12.sp
                            )
                        }
                    }
                }
            }
        }

        Spacer(modifier = Modifier.height(24.dp))
    }
}

@Composable
private fun EndpointItemRow(
    title: String,
    placeholder: String,
    value: String,
    isActive: Boolean,
    probeStatus: EndpointHealthStatus?,
    onValueChange: (String) -> Unit,
    onSelectActive: (() -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(text = title, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                if (isActive) {
                    Text(
                        text = "当前生效",
                        color = colors.accentGreen,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                } else if (value.isNotBlank() && onSelectActive != null) {
                    Text(
                        text = "设为生效",
                        color = colors.accentIndigo,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier
                            .clip(RoundedCornerShape(4.dp))
                            .clickable { onSelectActive() }
                            .padding(horizontal = 4.dp, vertical = 2.dp)
                    )
                }
                if (probeStatus != null) {
                    if (probeStatus.isReachable) {
                        Text(
                            text = "${probeStatus.latencyMs}ms",
                            color = getLatencyColor(probeStatus.latencyMs, colors),
                            fontSize = 11.sp,
                            fontWeight = FontWeight.SemiBold,
                            fontFamily = FontFamily.Monospace
                        )
                    } else {
                        Text(
                            text = "不可达",
                            color = colors.accentRed,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Medium
                        )
                    }
                }
            }
        }
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text(placeholder, color = colors.textMuted, fontSize = 12.sp) },
            singleLine = true,
            textStyle = androidx.compose.ui.text.TextStyle(
                fontSize = 13.sp,
                fontFamily = FontFamily.Monospace,
                color = colors.textPrimary
            ),
            shape = RoundedCornerShape(8.dp),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = colors.accentIndigo,
                unfocusedBorderColor = colors.border,
                focusedContainerColor = colors.surfaceVariant.copy(alpha = 0.3f),
                unfocusedContainerColor = colors.surfaceVariant.copy(alpha = 0.3f)
            ),
            trailingIcon = {
                if (value.isNotEmpty()) {
                    IconButton(
                        onClick = { onValueChange("") },
                        modifier = Modifier.size(20.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Cancel,
                            contentDescription = "Clear",
                            tint = colors.textMuted,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                }
            },
            modifier = Modifier.fillMaxWidth()
        )
    }
}

@Composable
private fun SettingsGroupSection(
    title: String,
    footer: String? = null,
    content: @Composable () -> Unit
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            text = title.uppercase(),
            color = colors.textSecondary,
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(horizontal = 4.dp)
        )
        content()
        if (!footer.isNullOrBlank()) {
            Text(
                text = footer,
                color = colors.textMuted,
                fontSize = 12.sp,
                lineHeight = 16.sp,
                modifier = Modifier.padding(horizontal = 4.dp, vertical = 2.dp)
            )
        }
    }
}

private fun getLatencyColor(ms: Long, colors: com.antigravity.mobile.ui.theme.AppColors): Color {
    return when {
        ms <= 30 -> colors.accentGreen
        ms <= 100 -> colors.accentBlue
        ms <= 250 -> colors.accentOrange
        else -> colors.accentRed
    }
}
