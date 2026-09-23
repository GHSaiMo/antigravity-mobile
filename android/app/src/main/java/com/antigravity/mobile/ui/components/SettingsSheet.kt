package com.antigravity.mobile.ui.components

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.widget.Toast
import androidx.activity.compose.BackHandler
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    prefs: PreferencesManager,
    onThemeModeChange: (String) -> Unit,
    onUnpair: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    var showUnpairAlert by remember { mutableStateOf(false) }
    var showClearCacheAlert by remember { mutableStateOf(false) }
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val connectionManager = remember { ConnectionManager(context) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val currentThemeMode by prefs.themeModeFlow.collectAsState()
    val isNotificationPermissionGranted = remember {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        } else {
            NotificationManagerCompat.from(context).areNotificationsEnabled()
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.background,
        dragHandle = null,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        windowInsets = WindowInsets(0, 0, 0, 0),
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight(0.94f)
    ) {
        BackHandler {
            onDismiss()
        }

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight()
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

            // Header title row: centered title, no "完成" button
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "设置",
                    fontWeight = FontWeight.Bold,
                    fontSize = 18.sp,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth()
                )
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
            ) {
                MainSettingsContent(
                    prefs = prefs,
                    connectionManager = connectionManager,
                    currentThemeMode = currentThemeMode,
                    onThemeModeChange = onThemeModeChange,
                    onPromptUnpair = { showUnpairAlert = true },
                    onPromptClearCache = { showClearCacheAlert = true }
                )
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
                    text = "解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。",
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        showUnpairAlert = false
                        onDismiss()
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
    connectionManager: ConnectionManager,
    currentThemeMode: String,
    onThemeModeChange: (String) -> Unit,
    onPromptUnpair: () -> Unit,
    onPromptClearCache: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val coroutineScope = rememberCoroutineScope()
    var autoApprove by remember { mutableStateOf(prefs.autoApprovePermissions) }
    var enableLiveNotifications by remember { mutableStateOf(prefs.enableLiveNotifications) }
    val isPaired = prefs.isPaired()
    val deviceId = prefs.deviceId ?: "未知"

    var lanAddress by remember { mutableStateOf(prefs.lanServerUrl ?: "") }
    var customAddress by remember { mutableStateOf(prefs.customServerUrl ?: "") }

    LaunchedEffect(Unit) {
        connectionManager.probeEndpoints(prefs)
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        // MARK: - 1. 设备配对与鉴权 (1:1 iOS 对齐)
        SettingsSection(
            title = "设备配对",
            footer = "管理当前设备与 Mac 网关的配对状态。"
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

        // MARK: - 2. 网络 (局域网与自定义平铺，主域名后台智能路由，1:1 对齐 iOS)
        SettingsSection(
            title = "网络"
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                // 第一行：局域网 (扫码配对默认填写)
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = "局域网",
                        color = colors.textPrimary,
                        fontSize = 15.sp
                    )

                    BasicTextField(
                        value = lanAddress,
                        onValueChange = { newVal ->
                            lanAddress = newVal
                            val clean = newVal.trim().trimEnd('/')
                            prefs.lanServerUrl = if (clean.isBlank()) null else clean
                            coroutineScope.launch {
                                connectionManager.probeEndpoints(prefs)
                            }
                        },
                        textStyle = androidx.compose.ui.text.TextStyle(
                            color = colors.textPrimary,
                            fontSize = 13.sp,
                            fontFamily = FontFamily.Monospace
                        ),
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                        decorationBox = { innerTextField ->
                            if (lanAddress.isBlank()) {
                                Text(
                                    text = "如 http://192.168.1.50:58900",
                                    color = colors.textMuted,
                                    fontSize = 13.sp,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            innerTextField()
                        }
                    )
                }

                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)

                // 第二行：自定义 (留给用户自己配置，如 Tailscale)
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = "自定义",
                        color = colors.textPrimary,
                        fontSize = 15.sp
                    )

                    BasicTextField(
                        value = customAddress,
                        onValueChange = { newVal ->
                            customAddress = newVal
                            val clean = newVal.trim().trimEnd('/')
                            prefs.customServerUrl = if (clean.isBlank()) null else clean
                            coroutineScope.launch {
                                connectionManager.probeEndpoints(prefs)
                            }
                        },
                        textStyle = androidx.compose.ui.text.TextStyle(
                            color = colors.textPrimary,
                            fontSize = 13.sp,
                            fontFamily = FontFamily.Monospace
                        ),
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                        decorationBox = { innerTextField ->
                            if (customAddress.isBlank()) {
                                Text(
                                    text = "如 http://100.x.x.x:58900",
                                    color = colors.textMuted,
                                    fontSize = 13.sp,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            innerTextField()
                        }
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
                        checkedThumbColor = colors.surface,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        // MARK: - 4. 实时通知 (1:1 iOS 对齐)
        SettingsSection(
            title = "实时通知",
            footer = "在通知栏中展示任务实时进展，并在需要用户审批或任务完成时发出高优先级提示音与振动提醒。"
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
                        .padding(horizontal = 16.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(text = "实时通知与审批提醒", color = colors.textPrimary, fontSize = 15.sp)
                        Text(
                            text = if (enableLiveNotifications) "已开启（含任务常驻与高优弹窗）" else "已关闭",
                            color = colors.textSecondary,
                            fontSize = 12.sp
                        )
                    }
                    Switch(
                        checked = enableLiveNotifications,
                        onCheckedChange = {
                            enableLiveNotifications = it
                            prefs.enableLiveNotifications = it
                        },
                        colors = SwitchDefaults.colors(
                            checkedThumbColor = colors.surface,
                            checkedTrackColor = colors.accentIndigo
                        )
                    )
                }

                if (!isNotificationPermissionGranted) {
                    HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable {
                                val intent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                                    Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).apply {
                                        putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName)
                                    }
                                } else {
                                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                                        data = Uri.fromParts("package", context.packageName, null)
                                    }
                                }
                                context.startActivity(intent)
                            }
                            .padding(horizontal = 16.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Warning,
                            contentDescription = "权限警告",
                            tint = Color(0xFFF59E0B),
                            modifier = Modifier.size(18.dp)
                        )
                        Text(
                            text = "系统通知权限未开启，点击前往系统设置授予权限",
                            color = Color(0xFFF59E0B),
                            fontSize = 13.sp,
                            modifier = Modifier.weight(1f)
                        )
                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textTertiary,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }
        }

        // MARK: - 5. 存储
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
                    isSelected = currentThemeMode == "system",
                    onClick = { onThemeModeChange("system") },
                    modifier = Modifier.weight(1f)
                )
                ThemeOptionSegment(
                    title = "浅色模式",
                    icon = Icons.Default.LightMode,
                    isSelected = currentThemeMode == "light",
                    onClick = { onThemeModeChange("light") },
                    modifier = Modifier.weight(1f)
                )
                ThemeOptionSegment(
                    title = "深色模式",
                    icon = Icons.Default.DarkMode,
                    isSelected = currentThemeMode == "dark",
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
                    Text(text = "Multigravity", color = colors.textSecondary, fontSize = 14.sp)
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
                    Text(text = "1.0.2", color = colors.textSecondary, fontSize = 14.sp)
                }
            }
        }

        Spacer(modifier = Modifier.height(24.dp))
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
    val haptic = rememberHaptic()
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(if (isSelected) colors.surface else Color.Transparent)
            .clickable {
                haptic.light()
                onClick()
            }
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
