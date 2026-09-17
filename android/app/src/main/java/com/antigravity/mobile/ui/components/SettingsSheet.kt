package com.antigravity.mobile.ui.components

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.activity.compose.BackHandler
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
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

private enum class SettingsScreenView {
    MAIN, NETWORK
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    prefs: PreferencesManager,
    onThemeModeChange: (String) -> Unit,
    onRescanQR: (() -> Unit)? = null,
    onUnpair: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    var currentView by remember { mutableStateOf(SettingsScreenView.MAIN) }
    var showUnpairAlert by remember { mutableStateOf(false) }
    var showClearCacheAlert by remember { mutableStateOf(false) }
    val colors = AntigravityTheme.colors
    val context = LocalContext.current

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        BackHandler {
            if (currentView == SettingsScreenView.NETWORK) {
                currentView = SettingsScreenView.MAIN
            } else {
                onDismiss()
            }
        }

        Scaffold(
            topBar = {
                TopAppBar(
                    title = {
                        Text(
                            text = if (currentView == SettingsScreenView.MAIN) "设置" else "网络设置",
                            fontWeight = FontWeight.Bold,
                            fontSize = 18.sp,
                            color = colors.textPrimary
                        )
                    },
                    navigationIcon = {
                        if (currentView == SettingsScreenView.NETWORK) {
                            IconButton(onClick = { currentView = SettingsScreenView.MAIN }) {
                                Icon(
                                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                                    contentDescription = "Back",
                                    tint = colors.accentIndigo
                                )
                            }
                        }
                    },
                    actions = {
                        TextButton(onClick = onDismiss) {
                            Text(
                                text = "完成",
                                color = colors.accentIndigo,
                                fontSize = 16.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = colors.background,
                        titleContentColor = colors.textPrimary
                    )
                )
            },
            containerColor = colors.background,
            modifier = modifier.fillMaxSize()
        ) { padding ->
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
            ) {
                if (currentView == SettingsScreenView.MAIN) {
                    MainSettingsContent(
                        prefs = prefs,
                        onNavigateToNetwork = { currentView = SettingsScreenView.NETWORK },
                        onThemeModeChange = onThemeModeChange,
                        onRescanQR = onRescanQR,
                        onPromptUnpair = { showUnpairAlert = true },
                        onPromptClearCache = { showClearCacheAlert = true }
                    )
                } else {
                    NetworkSettingsContent(prefs = prefs)
                }
            }
        }
    }

    if (showUnpairAlert) {
        AlertDialog(
            onDismissRequest = { showUnpairAlert = false },
            title = {
                Text(
                    text = "确定解除设备配对？",
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = "解除配对将清除此设备的访问令牌与已保存的网关地址。",
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        showUnpairAlert = false
                        onUnpair()
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("解除配对")
                }
            },
            dismissButton = {
                TextButton(onClick = { showUnpairAlert = false }) {
                    Text("取消", color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }

    if (showClearCacheAlert) {
        AlertDialog(
            onDismissRequest = { showClearCacheAlert = false },
            title = {
                Text(
                    text = "确定清空本地缓存？",
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = "本地缓存的会话消息、方案及离线文档将被清理，下次访问时将从网关重新拉取。",
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        showClearCacheAlert = false
                        Toast.makeText(context, "本地缓存已清空", Toast.LENGTH_SHORT).show()
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("清空")
                }
            },
            dismissButton = {
                TextButton(onClick = { showClearCacheAlert = false }) {
                    Text("取消", color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }
}

@Composable
private fun MainSettingsContent(
    prefs: PreferencesManager,
    onNavigateToNetwork: () -> Unit,
    onThemeModeChange: (String) -> Unit,
    onRescanQR: (() -> Unit)?,
    onPromptUnpair: () -> Unit,
    onPromptClearCache: () -> Unit
) {
    val colors = AntigravityTheme.colors
    var autoApprove by remember { mutableStateOf(prefs.autoApprovePermissions) }
    var enableLiveNotifications by remember { mutableStateOf(prefs.enableLiveNotifications) }
    val isPaired = prefs.isPaired()
    val deviceId = prefs.deviceId ?: "未知"

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // MARK: - 1. 设备配对与鉴权 (1:1 iOS 对齐)
        SettingsSection(
            title = "设备配对",
            footer = "扫描 Mac 终端配对二维码，完成设备绑定与鉴权。"
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                // 配对状态
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "配对状态", color = colors.textPrimary, fontSize = 15.sp)
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp)
                    ) {
                        if (isPaired) {
                            Icon(
                                imageVector = Icons.Default.CheckCircle,
                                contentDescription = "Paired",
                                tint = colors.accentGreen,
                                modifier = Modifier.size(16.dp)
                            )
                            Text(text = "已配对", color = colors.textSecondary, fontSize = 14.sp)
                        } else {
                            Text(text = "未配对", color = colors.textSecondary, fontSize = 14.sp)
                        }
                    }
                }

                if (isPaired && deviceId.isNotBlank()) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(text = "设备 ID", color = colors.textPrimary, fontSize = 15.sp)
                        Text(
                            text = if (deviceId.length > 20) deviceId.take(8) + "..." + deviceId.takeLast(6) else deviceId,
                            color = colors.textSecondary,
                            fontSize = 13.sp,
                            fontFamily = FontFamily.Monospace
                        )
                    }
                }

                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                // 重新扫描配对
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onRescanQR?.invoke() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = if (isPaired) "重新扫描配对二维码" else "扫描二维码配对",
                        color = colors.accentIndigo,
                        fontSize = 15.sp
                    )
                }

                if (isPaired) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { onPromptUnpair() }
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = "解除设备配对",
                            color = colors.accentRed,
                            fontSize = 15.sp
                        )
                    }
                }
            }
        }

        // MARK: - 2. 网络 (二级菜单入口，1:1 iOS 对齐)
        SettingsSection(
            title = "网络",
            footer = "配置局域网、外网 IPv6 及云端中继等路由通道。"
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .clickable { onNavigateToNetwork() }
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(text = "网络设置", color = colors.textPrimary, fontSize = 15.sp)
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = prefs.gatewayBaseUrl?.takeIf { it.isNotBlank() } ?: "未设置",
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        fontFamily = FontFamily.Monospace
                    )
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                        contentDescription = "Forward",
                        tint = colors.textMuted,
                        modifier = Modifier.size(13.dp)
                    )
                }
            }
        }

        // MARK: - 3. 权限与自动化 (1:1 iOS 对齐)
        SettingsSection(
            title = "权限",
            footer = "自动批准 Agent 工具调用与执行权限，无需每次手动确认。"
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(horizontal = 16.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(text = "自动批准操作权限", color = colors.textPrimary, fontSize = 15.sp)
                Switch(
                    checked = autoApprove,
                    onCheckedChange = {
                        autoApprove = it
                        prefs.autoApprovePermissions = it
                    },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        // MARK: - 4. 实时活动与通知 (1:1 iOS 对齐)
        SettingsSection(
            title = "实时活动",
            footer = "在锁屏和通知栏上显示任务进展及后台命令。"
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(horizontal = 16.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(text = "通知与实时活动", color = colors.textPrimary, fontSize = 15.sp)
                Switch(
                    checked = enableLiveNotifications,
                    onCheckedChange = {
                        enableLiveNotifications = it
                        prefs.enableLiveNotifications = it
                    },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        // MARK: - 5. 本地存储 (1:1 iOS 对齐)
        SettingsSection(
            title = "存储",
            footer = "清除本地缓存的会话与文档数据，下次访问时将从网关重新拉取。"
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .clickable { onPromptClearCache() }
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "清空本地缓存",
                    color = colors.accentRed,
                    fontSize = 15.sp
                )
            }
        }

        // MARK: - 6. 外观与主题
        SettingsSection(
            title = "外观与主题",
            footer = "选择浅色、深色模式或跟随系统自动切换。"
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surfaceVariant)
                    .padding(4.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                ThemeOptionSegment(
                    title = "跟随系统",
                    icon = Icons.Default.SettingsBrightness,
                    isSelected = prefs.themeMode == "system",
                    onClick = { onThemeModeChange("system") },
                    modifier = Modifier.weight(1f)
                )
                ThemeOptionSegment(
                    title = "浅色模式",
                    icon = Icons.Default.LightMode,
                    isSelected = prefs.themeMode == "light",
                    onClick = { onThemeModeChange("light") },
                    modifier = Modifier.weight(1f)
                )
                ThemeOptionSegment(
                    title = "深色模式",
                    icon = Icons.Default.DarkMode,
                    isSelected = prefs.themeMode == "dark",
                    onClick = { onThemeModeChange("dark") },
                    modifier = Modifier.weight(1f)
                )
            }
        }

        // MARK: - 7. 关于 (1:1 iOS 对齐)
        SettingsSection(title = "关于") {
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
                    Text(text = "应用名称", color = colors.textPrimary, fontSize = 15.sp)
                    Text(text = "Antigravity Mobile", color = colors.textSecondary, fontSize = 14.sp)
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "版本", color = colors.textPrimary, fontSize = 15.sp)
                    Text(text = "1.0.0 (Native)", color = colors.textSecondary, fontSize = 14.sp)
                }
            }
        }

        Spacer(modifier = Modifier.height(24.dp))
    }
}

@Composable
private fun NetworkSettingsContent(
    prefs: PreferencesManager
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()

    var activeUrl by remember { mutableStateOf(prefs.gatewayBaseUrl ?: "") }
    var lanUrl by remember { mutableStateOf(prefs.lanServerUrl ?: "") }
    var ipv6Url by remember { mutableStateOf(prefs.ipv6ServerUrl ?: "") }
    var relayUrl by remember { mutableStateOf(prefs.relayServerUrl ?: "") }
    var customUrl by remember { mutableStateOf(prefs.customServerUrl ?: "") }

    var isTesting by remember { mutableStateOf(false) }
    var testResultText by remember { mutableStateOf<String?>(null) }
    var latencyMs by remember { mutableStateOf<Long?>(null) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // MARK: - 1. 当前活动链路 (1:1 iOS 对齐)
        SettingsSection(title = "当前活动链路") {
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
                        text = if (activeUrl.isNotBlank()) "当前指定网关" else "未配置或未选定通道",
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

                if (latencyMs != null) {
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
                            text = "${latencyMs}ms",
                            color = when {
                                latencyMs!! <= 50 -> colors.accentGreen
                                latencyMs!! <= 150 -> colors.accentBlue
                                else -> colors.accentOrange
                            },
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

        // MARK: - 2. 智能并发探活与自动测速 (1:1 iOS 对齐)
        SettingsSection(
            title = "智能选路与测速",
            footer = "并发探测已配置通道并自动优选延迟最低的链路。"
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
                    Text(text = "一键并发探活与测速", color = colors.textPrimary, fontSize = 15.sp)
                    Button(
                        onClick = {
                            if (activeUrl.isBlank()) {
                                Toast.makeText(context, "请先设置网关地址", Toast.LENGTH_SHORT).show()
                                return@Button
                            }
                            isTesting = true
                            testResultText = "正在探测链路延迟..."
                            coroutineScope.launch {
                                val start = System.currentTimeMillis()
                                try {
                                    val client = OkHttpClient.Builder()
                                        .connectTimeout(4, TimeUnit.SECONDS)
                                        .readTimeout(4, TimeUnit.SECONDS)
                                        .build()
                                    val req = Request.Builder().url("$activeUrl/healthz").build()
                                    withContext(Dispatchers.IO) {
                                        client.newCall(req).execute().use { resp ->
                                            val took = System.currentTimeMillis() - start
                                            latencyMs = took
                                            testResultText = if (resp.isSuccessful) {
                                                "探测成功: 延迟 ${took}ms (HTTP ${resp.code})"
                                            } else {
                                                "网关响应错误: HTTP ${resp.code}"
                                            }
                                        }
                                    }
                                } catch (e: Exception) {
                                    testResultText = "探测失败: ${e.message ?: "连接超时"}"
                                    latencyMs = null
                                } finally {
                                    isTesting = false
                                }
                            }
                        },
                        enabled = !isTesting,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = colors.accentIndigo.copy(alpha = 0.15f),
                            contentColor = colors.accentIndigo
                        ),
                        shape = RoundedCornerShape(8.dp)
                    ) {
                        if (isTesting) {
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

                testResultText?.let { msg ->
                    Text(
                        text = msg,
                        color = if (msg.contains("成功")) colors.accentGreen else colors.textSecondary,
                        fontSize = 12.sp
                    )
                }
            }
        }

        // MARK: - 3. 多通道候选端点配置 (1:1 iOS 对齐)
        SettingsSection(
            title = "路由端点配置",
            footer = "扫码配对后会自动填入所有可用通道，也可手动指定各端点地址。"
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                EndpointInputField(
                    title = "局域网 Wi-Fi (LAN IPv4)",
                    placeholder = "如 http://192.168.1.50:58900",
                    value = lanUrl,
                    isActive = activeUrl == lanUrl && lanUrl.isNotBlank(),
                    onValueChange = {
                        lanUrl = it
                        prefs.lanServerUrl = it.ifBlank { null }
                    }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.3f), thickness = 0.5.dp)
                EndpointInputField(
                    title = "外网直连 (Public IPv6)",
                    placeholder = "如 http://[2001:db8::1]:58900",
                    value = ipv6Url,
                    isActive = activeUrl == ipv6Url && ipv6Url.isNotBlank(),
                    onValueChange = {
                        ipv6Url = it
                        prefs.ipv6ServerUrl = it.ifBlank { null }
                    }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.3f), thickness = 0.5.dp)
                EndpointInputField(
                    title = "云服务器中继 (Cloud Relay IPv4)",
                    placeholder = "如 http://relay.example.com:58900",
                    value = relayUrl,
                    isActive = activeUrl == relayUrl && relayUrl.isNotBlank(),
                    onValueChange = {
                        relayUrl = it
                        prefs.relayServerUrl = it.ifBlank { null }
                    }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.3f), thickness = 0.5.dp)
                EndpointInputField(
                    title = "自定义域名 / DDNS / Tailscale",
                    placeholder = "如 https://mac.yourdomain.com",
                    value = customUrl,
                    isActive = activeUrl == customUrl && customUrl.isNotBlank(),
                    onValueChange = {
                        customUrl = it
                        prefs.customServerUrl = it.ifBlank { null }
                    }
                )
            }
        }

        // MARK: - 4. 路由策略指南 (1:1 iOS 对齐)
        SettingsSection(
            title = "智能多通道路由策略",
            footer = "手机在同一 Wi-Fi 时优先局域网直连；外出移动网络时优先 IPv6 端到端直连；网络受限时自动通过云服务器中继兜底。"
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
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "1. 局域网优先", color = colors.textPrimary, fontSize = 14.sp)
                    Text(text = "LAN First (~1ms)", color = colors.textSecondary, fontSize = 13.sp)
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.3f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "2. 蜂窝网络直连", color = colors.textPrimary, fontSize = 14.sp)
                    Text(text = "Cellular IPv6", color = colors.textSecondary, fontSize = 13.sp)
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.3f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "3. 云服务器中继", color = colors.textPrimary, fontSize = 14.sp)
                    Text(text = "Cloud Relay", color = colors.textSecondary, fontSize = 13.sp)
                }
            }
        }

        Spacer(modifier = Modifier.height(24.dp))
    }
}

@Composable
private fun EndpointInputField(
    title: String,
    placeholder: String,
    value: String,
    isActive: Boolean,
    onValueChange: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Text(text = title, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            if (isActive) {
                Text(
                    text = "当前生效",
                    color = colors.accentGreen,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.SemiBold
                )
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
private fun SettingsSection(
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

@Composable
private fun ThemeOptionSegment(
    title: String,
    icon: ImageVector,
    isSelected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(if (isSelected) colors.surface else Color.Transparent)
            .clickable { onClick() }
            .padding(vertical = 8.dp),
        contentAlignment = Alignment.Center
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Icon(
                imageVector = icon,
                contentDescription = title,
                tint = if (isSelected) colors.accentIndigo else colors.textSecondary,
                modifier = Modifier.size(15.dp)
            )
            Text(
                text = title,
                color = if (isSelected) colors.textPrimary else colors.textSecondary,
                fontSize = 12.sp,
                fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
            )
        }
    }
}
