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
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme

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
    val connectionManager = remember { ConnectionManager(context) }
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
        BackHandler {
            if (currentView == SettingsScreenView.NETWORK) {
                currentView = SettingsScreenView.MAIN
            } else {
                onDismiss()
            }
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

            // Header title row: centered title, optional back button on the left when in subview, no "完成" button
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp),
                contentAlignment = Alignment.Center
            ) {
                if (currentView == SettingsScreenView.NETWORK) {
                    IconButton(
                        onClick = { currentView = SettingsScreenView.MAIN },
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
                }

                Text(
                    text = if (currentView == SettingsScreenView.MAIN) "设置" else "网络设置",
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
                    NetworkSettingsContent(
                        prefs = prefs,
                        connectionManager = connectionManager
                    )
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
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
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
                    Text(text = "1.0.0", color = colors.textSecondary, fontSize = 14.sp)
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
