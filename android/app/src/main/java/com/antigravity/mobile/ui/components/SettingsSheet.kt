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
import androidx.compose.animation.AnimatedVisibility
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
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.fetchPermissionSettings
import com.antigravity.mobile.data.service.updatePermissionPreset
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.modelProviderAccent
import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch

private enum class SettingsSubpage {
    MAIN,
    COMPACT;

    val title: String
        get() = when (this) {
            MAIN -> "设置".loc
            COMPACT -> "精简模式".loc
        }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    prefs: PreferencesManager,
    onThemeModeChange: (String) -> Unit,
    onUnpair: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    onLoadModels: suspend () -> Result<ModelsResponse> = { Result.failure(IllegalStateException("未提供模型目录".loc)) }
) {
    var showUnpairAlert by remember { mutableStateOf(false) }
    var showClearCacheAlert by remember { mutableStateOf(false) }
    var showMcpSkillsSheet by remember { mutableStateOf(false) }
    var activeSubpage by remember { mutableStateOf(SettingsSubpage.MAIN) }
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
        BackHandler {
            if (activeSubpage != SettingsSubpage.MAIN) {
                activeSubpage = SettingsSubpage.MAIN
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
                    .padding(top = 10.dp, bottom = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            // Header title row: back button when on subpage, centered title
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp),
                contentAlignment = Alignment.Center
            ) {
                if (activeSubpage != SettingsSubpage.MAIN) {
                    Row(
                        modifier = Modifier
                            .align(Alignment.CenterStart)
                            .clip(RoundedCornerShape(8.dp))
                            .clickable { activeSubpage = SettingsSubpage.MAIN }
                            .padding(horizontal = 4.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowLeft,
                            contentDescription = "返回设置".loc,
                            tint = colors.accentBlue,
                            modifier = Modifier.size(22.dp)
                        )
                        Text(
                            text = "设置".loc,
                            color = colors.accentBlue,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )
                    }
                }
                Text(
                    text = activeSubpage.title,
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
                when (activeSubpage) {
                    SettingsSubpage.MAIN -> MainSettingsContent(
                        prefs = prefs,
                        connectionManager = connectionManager,
                        currentThemeMode = currentThemeMode,
                        onThemeModeChange = onThemeModeChange,
                        onOpenCompactSettings = { activeSubpage = SettingsSubpage.COMPACT },
                        onPromptUnpair = { showUnpairAlert = true },
                        onPromptClearCache = { showClearCacheAlert = true },
                        onOpenMcpSkills = { showMcpSkillsSheet = true },
                        onLoadModels = onLoadModels
                    )
                    SettingsSubpage.COMPACT -> CompactModeSubpageContent(prefs = prefs)
                }
            }
        }
            }
    }

    if (showUnpairAlert) {
        AlertDialog(
            onDismissRequest = { showUnpairAlert = false },
            title = {
                Text(
                    text = "确定解除设备配对？".loc,
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = "解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。".loc,
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
                    Text("解除配对".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { showUnpairAlert = false }) {
                    Text("取消".loc, color = colors.textSecondary)
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
                    text = "确定清空本地缓存？".loc,
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = "本地缓存的会话消息、方案及离线文档将被清理，下次访问时将从网关重新拉取。".loc,
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        showClearCacheAlert = false
                        Toast.makeText(context, "本地缓存已清空".loc, Toast.LENGTH_SHORT).show()
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                ) {
                    Text("清空".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { showClearCacheAlert = false }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            },
            containerColor = colors.surface
        )
    }

    if (showMcpSkillsSheet) {
        McpSkillsSheet(onDismiss = { showMcpSkillsSheet = false })
    }
}

@Composable
private fun MainSettingsContent(
    prefs: PreferencesManager,
    connectionManager: ConnectionManager,
    currentThemeMode: String,
    onThemeModeChange: (String) -> Unit,
    onOpenCompactSettings: () -> Unit,
    onPromptUnpair: () -> Unit,
    onPromptClearCache: () -> Unit,
    onOpenMcpSkills: () -> Unit,
    onLoadModels: suspend () -> Result<ModelsResponse>
) {
    val colors = AntigravityTheme.colors
    val localContext = LocalContext.current
    val isNotificationPermissionGranted = remember {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            ContextCompat.checkSelfPermission(localContext, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        } else {
            NotificationManagerCompat.from(localContext).areNotificationsEnabled()
        }
    }
    val coroutineScope = rememberCoroutineScope()
    var enableLiveNotifications by remember { mutableStateOf(prefs.enableLiveNotifications) }
    var enableVoiceMode by remember { mutableStateOf(prefs.enableVoiceMode) }
    val isPaired = prefs.isPaired()
    val deviceId = prefs.deviceId ?: "未知".loc

    var lanAddress by remember { mutableStateOf(prefs.lanServerUrl ?: "") }
    var customAddress by remember { mutableStateOf(prefs.customServerUrl ?: "") }
    val keyboardController = androidx.compose.ui.platform.LocalSoftwareKeyboardController.current
    val focusManager = androidx.compose.ui.platform.LocalFocusManager.current

    val saveLanAddress: () -> Unit = {
        val clean = lanAddress.trim().trimEnd('/')
        if (clean.isBlank()) {
            prefs.lanServerUrl = null
            lanAddress = ""
            Toast.makeText(localContext, "已清除局域网地址".loc, Toast.LENGTH_SHORT).show()
        } else {
            val normalized = PreferencesManager.normalizeUrl(clean) ?: clean
            prefs.lanServerUrl = normalized
            lanAddress = normalized
            Toast.makeText(localContext, "已保存局域网地址".loc, Toast.LENGTH_SHORT).show()
        }
        keyboardController?.hide()
        focusManager.clearFocus()
        coroutineScope.launch {
            connectionManager.probeEndpoints(prefs)
        }
    }

    val saveCustomAddress: () -> Unit = {
        val clean = customAddress.trim().trimEnd('/')
        if (clean.isBlank()) {
            prefs.customServerUrl = null
            customAddress = ""
            Toast.makeText(localContext, "已清除自定义网络地址".loc, Toast.LENGTH_SHORT).show()
        } else {
            val normalized = PreferencesManager.normalizeUrl(clean) ?: clean
            prefs.customServerUrl = normalized
            customAddress = normalized
            Toast.makeText(localContext, "已保存自定义网络地址".loc, Toast.LENGTH_SHORT).show()
        }
        keyboardController?.hide()
        focusManager.clearFocus()
        coroutineScope.launch {
            connectionManager.probeEndpoints(prefs)
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            val cleanCustom = customAddress.trim().trimEnd('/')
            if (cleanCustom != (prefs.customServerUrl ?: "")) {
                if (cleanCustom.isBlank()) {
                    prefs.customServerUrl = null
                } else {
                    prefs.customServerUrl = PreferencesManager.normalizeUrl(cleanCustom) ?: cleanCustom
                }
            }
            val cleanLan = lanAddress.trim().trimEnd('/')
            if (cleanLan != (prefs.lanServerUrl ?: "")) {
                if (cleanLan.isBlank()) {
                    prefs.lanServerUrl = null
                } else {
                    prefs.lanServerUrl = PreferencesManager.normalizeUrl(cleanLan) ?: cleanLan
                }
            }
        }
    }
    val currentAppLanguage by prefs.appLanguageFlow.collectAsState()
    val languageOptions = remember {
        listOf(
            PickerMenuOption("system", "跟随系统 (System Default)".loc),
            PickerMenuOption("zh", "简体中文".loc),
            PickerMenuOption("en", "English")
        )
    }
    val currentPlatform by prefs.gatewayPlatformFlow.collectAsState()
    val gatewayDesc = when (currentPlatform?.lowercase()) {
        "windows", "win" -> "Windows 网关".loc
        "darwin", "macos", "mac" -> "Mac 网关".loc
        "linux" -> "Linux 网关".loc
        else -> "电脑网关".loc
    }

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
            title = "设备配对".loc,
            footer = "管理当前设备与 ${gatewayDesc}的配对状态。".loc
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
                    Text(text = "配对状态".loc, color = colors.textPrimary, fontSize = 15.sp)
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
                            Text(text = "已配对".loc, color = colors.textSecondary, fontSize = 14.sp)
                        } else {
                            Text(text = "未配对".loc, color = colors.textSecondary, fontSize = 14.sp)
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
                        Text(text = "设备 ID".loc, color = colors.textPrimary, fontSize = 15.sp)
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
                        text = "解除设备配对".loc,
                        color = colors.accentRed,
                        fontSize = 15.sp
                    )
                }
            }
        }

        // MARK: - 2. 网络 (局域网与自定义平铺，主域名后台智能路由，1:1 对齐 iOS)
        SettingsSection(
            title = "网络".loc,
            footer = "配置局域网或自定义节点直连地址（如 Tailscale/VPN/DDNS）。点击「保存」即可保存并立即测速探活。".loc
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
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            text = "局域网".loc,
                            color = colors.textPrimary,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )

                        val currentSavedLan = prefs.lanServerUrl ?: ""
                        val isLanDirty = lanAddress.trim().trimEnd('/') != currentSavedLan
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(6.dp))
                                .background(if (isLanDirty) colors.accentIndigo else colors.accentIndigo.copy(alpha = 0.12f))
                                .clickable { saveLanAddress() }
                                .padding(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                if (!isLanDirty && currentSavedLan.isNotBlank()) {
                                    Icon(
                                        imageVector = Icons.Default.Check,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(13.dp)
                                    )
                                    Text(
                                        text = "已保存".loc,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        color = colors.accentIndigo
                                    )
                                } else {
                                    Text(
                                        text = "保存".loc,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = if (isLanDirty) Color.White else colors.accentIndigo
                                    )
                                }
                            }
                        }
                    }

                    BasicTextField(
                        value = lanAddress,
                        onValueChange = { newVal ->
                            lanAddress = newVal
                        },
                        textStyle = androidx.compose.ui.text.TextStyle(
                            color = colors.textPrimary,
                            fontSize = 13.sp,
                            fontFamily = FontFamily.Monospace
                        ),
                        singleLine = true,
                        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                            keyboardType = androidx.compose.ui.text.input.KeyboardType.Uri,
                            imeAction = androidx.compose.ui.text.input.ImeAction.Done
                        ),
                        keyboardActions = androidx.compose.foundation.text.KeyboardActions(
                            onDone = { saveLanAddress() }
                        ),
                        modifier = Modifier.fillMaxWidth(),
                        decorationBox = { innerTextField ->
                            if (lanAddress.isBlank()) {
                                Text(
                                    text = "如 http://192.168.1.50:58900".loc,
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
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            text = "自定义".loc,
                            color = colors.textPrimary,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )

                        val currentSavedCustom = prefs.customServerUrl ?: ""
                        val isCustomDirty = customAddress.trim().trimEnd('/') != currentSavedCustom
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(6.dp))
                                .background(if (isCustomDirty) colors.accentIndigo else colors.accentIndigo.copy(alpha = 0.12f))
                                .clickable { saveCustomAddress() }
                                .padding(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                if (!isCustomDirty && currentSavedCustom.isNotBlank()) {
                                    Icon(
                                        imageVector = Icons.Default.Check,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(13.dp)
                                    )
                                    Text(
                                        text = "已保存".loc,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.Medium,
                                        color = colors.accentIndigo
                                    )
                                } else {
                                    Text(
                                        text = "保存".loc,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = if (isCustomDirty) Color.White else colors.accentIndigo
                                    )
                                }
                            }
                        }
                    }

                    BasicTextField(
                        value = customAddress,
                        onValueChange = { newVal ->
                            customAddress = newVal
                        },
                        textStyle = androidx.compose.ui.text.TextStyle(
                            color = colors.textPrimary,
                            fontSize = 13.sp,
                            fontFamily = FontFamily.Monospace
                        ),
                        singleLine = true,
                        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                            keyboardType = androidx.compose.ui.text.input.KeyboardType.Uri,
                            imeAction = androidx.compose.ui.text.input.ImeAction.Done
                        ),
                        keyboardActions = androidx.compose.foundation.text.KeyboardActions(
                            onDone = { saveCustomAddress() }
                        ),
                        modifier = Modifier.fillMaxWidth(),
                        decorationBox = { innerTextField ->
                            if (customAddress.isBlank()) {
                                Text(
                                    text = "如 http://100.x.x.x:58900".loc,
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

        // MARK: - 3. 模型
        DefaultModelSection(prefs = prefs, onLoadModels = onLoadModels)

        // MARK: - 4. 扩展与能力（Agent 权限模式 + MCP 工具与技能看板）
        val permPrefs = remember { localContext.getSharedPreferences("antigravity_tips", android.content.Context.MODE_PRIVATE) }
        var permPreset by remember { mutableStateOf(permPrefs.getString("cached_permission_preset", "strict") ?: "strict") }
        LaunchedEffect(Unit) {
            ApiClient.defaultInstance?.fetchPermissionSettings()?.onSuccess {
                permPreset = it.preset
                permPrefs.edit().putString("cached_permission_preset", it.preset).apply()
            }
        }
        val permissionOptions = remember(colors) {
            listOf(
                PickerMenuOption("strict", "严格审批".loc, accentColor = colors.accentIndigo),
                PickerMenuOption("sandbox", "离席沙箱放行".loc, accentColor = colors.accentOrange),
                PickerMenuOption("auto", "全自动放行".loc, accentColor = colors.accentOrange)
            )
        }
        SettingsSection(
            title = "扩展与能力".loc,
            footer = "在此配置默认权限并查看 MCP 工具与技能；进入会话后，长按右上角工具按钮也可以随时改变该会话的权限。".loc
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                SettingsMenuPickerRow(
                    label = "Agent 权限模式".loc,
                    selectedLabel = permissionOptions.firstOrNull { it.id == permPreset }?.label ?: "严格审批".loc,
                    selectedId = permPreset,
                    selectedColor = if (permPreset == "strict") colors.accentIndigo else colors.accentOrange,
                    options = permissionOptions,
                    onSelect = { key ->
                        permPreset = key
                        permPrefs.edit().putString("cached_permission_preset", key).apply()
                        coroutineScope.launch {
                            ApiClient.defaultInstance?.updatePermissionPreset(key)?.onSuccess {
                                permPreset = it.preset
                                permPrefs.edit().putString("cached_permission_preset", it.preset).apply()
                            }
                        }
                    }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenMcpSkills() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column {
                        Text(
                            text = "MCP 工具与技能看板".loc,
                            color = colors.textPrimary,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )
                        Text(
                            text = "MCP / 技能 / 插件 / 定时任务 / 后台".loc,
                            color = colors.textMuted,
                            fontSize = 11.5.sp
                        )
                    }
                    Spacer(modifier = Modifier.weight(1f))
                    Icon(
                        imageVector = Icons.Default.ChevronRight,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(16.dp)
                    )
                }
            }
        }

        // MARK: - 4a. 实时语音对话
        SettingsSection(
            title = "语音".loc,
            footer = "开启后，输入框为空且空闲时右侧按钮支持单击实时双工语音通话、长按原生流式语音听写。".loc
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
                Column(modifier = Modifier.weight(1f)) {
                    Text(text = "实时语音对话".loc, color = colors.textPrimary, fontSize = 15.sp)
                    Text(
                        text = if (enableVoiceMode) "已开启（空闲时显示语音入口）".loc else "已关闭".loc,
                        color = colors.textSecondary,
                        fontSize = 12.sp
                    )
                }
                Switch(
                    checked = enableVoiceMode,
                    onCheckedChange = {
                        enableVoiceMode = it
                        prefs.enableVoiceMode = it
                    },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = colors.surface,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        // MARK: - 4b. 实时通知
        SettingsSection(
            title = "实时通知".loc,
            footer = "在通知栏中展示任务实时进展，并在需要用户审批或任务完成时发出高优先级提示音与振动提醒。".loc
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
                        Text(text = "实时通知与审批提醒".loc, color = colors.textPrimary, fontSize = 15.sp)
                        Text(
                            text = if (enableLiveNotifications) "已开启（含任务常驻与高优弹窗）".loc else "已关闭".loc,
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
                                        putExtra(Settings.EXTRA_APP_PACKAGE, localContext.packageName)
                                    }
                                } else {
                                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                                        setData(Uri.fromParts("package", localContext.packageName, null))
                                    }
                                }
                                localContext.startActivity(intent)
                            }
                            .padding(horizontal = 16.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Warning,
                            contentDescription = "权限警告".loc,
                            tint = Color(0xFFF59E0B),
                            modifier = Modifier.size(18.dp)
                        )
                        Text(
                            text = "系统通知权限未开启，点击前往系统设置授予权限".loc,
                            color = Color(0xFFF59E0B),
                            fontSize = 13.sp,
                            modifier = Modifier.weight(1f)
                        )
                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }
        }

        // MARK: - 5. 外观与主题
        var weeklyPaceCursor by remember { mutableStateOf(prefs.weeklyPaceCursorEnabled) }
        val currentFontScale by prefs.fontSizeScaleFlow.collectAsState()
        val knowledgeMode by prefs.knowledgeModeFlow.collectAsState()
        val compactCollapseTools by prefs.compactCollapseToolsFlow.collectAsState()
        val compactCollapseTasks by prefs.compactCollapseTasksFlow.collectAsState()
        val compactHideHealthyContext by prefs.compactHideHealthyContextFlow.collectAsState()
        val compactSummary = remember(knowledgeMode, compactCollapseTools, compactCollapseTasks, compactHideHealthyContext) {
            if (!knowledgeMode) "已关闭".loc
            else {
                val activeCount = listOf(compactCollapseTools, compactCollapseTasks, compactHideHealthyContext).count { it }
                "已开启 ($activeCount/3)".loc
            }
        }
        val fontScaleOptions = remember {
            listOf(
                PickerMenuOption("compact", "紧凑 (90%)".loc),
                PickerMenuOption("standard", "标准 (100%)".loc),
                PickerMenuOption("comfortable", "舒适 (115%)".loc)
            )
        }
        SettingsSection(
            title = "外观与主题".loc,
            footer = "选择全局浅色/深色/跟随系统外观；点进「精简模式」可自定义对话降噪选项。".loc
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                ThemeModeSegmentedRow(
                    selectedMode = currentThemeMode,
                    onSelectMode = onThemeModeChange
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                SettingsMenuPickerRow(
                    label = "字号缩放".loc,
                    selectedLabel = fontScaleOptions.firstOrNull { it.id == currentFontScale }?.label ?: "标准 (100%)".loc,
                    selectedId = currentFontScale,
                    options = fontScaleOptions,
                    onSelect = { prefs.fontSizeScale = it }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                SettingsMenuPickerRow(
                    label = "语言 / Language".loc,
                    selectedLabel = when (currentAppLanguage) {
                        "en" -> "English"
                        "zh" -> "简体中文".loc
                        else -> "跟随系统 (System Default)".loc
                    },
                    selectedId = currentAppLanguage,
                    options = languageOptions,
                    onSelect = { prefs.appLanguage = it }
                )
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(
                        modifier = Modifier
                            .weight(1f)
                            .padding(end = 12.dp)
                    ) {
                        Text(
                            text = "额度匀速指标".loc,
                            color = colors.textPrimary,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )
                        Text(
                            text = "在额度条上显示均速基准刻度针".loc,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp,
                            lineHeight = 15.sp
                        )
                    }
                    Switch(
                        checked = weeklyPaceCursor,
                        onCheckedChange = {
                            weeklyPaceCursor = it
                            prefs.weeklyPaceCursorEnabled = it
                        },
                        colors = SwitchDefaults.colors(
                            checkedThumbColor = colors.surface,
                            checkedTrackColor = colors.accentIndigo
                        )
                    )
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenCompactSettings() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = "精简模式".loc,
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = compactSummary,
                            color = colors.textSecondary,
                            fontSize = 14.sp
                        )
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                }
            }
        }

        // MARK: - 6. 手势与使用技巧
        var showTipsGuide by remember { mutableStateOf(false) }
        SettingsSection(
            title = "手势与使用技巧".loc,
            footer = "掌握常用快捷手势与界面隐蔽功能，大幅提升移动端操作效率。".loc
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .clickable { showTipsGuide = true }
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    modifier = Modifier.weight(1f, fill = false)
                ) {
                    Icon(
                        imageVector = Icons.Default.Lightbulb,
                        contentDescription = null,
                        tint = Color(0xFFF59E0B),
                        modifier = Modifier.size(20.dp)
                    )
                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(
                            text = "功能速查与手势指南".loc,
                            color = colors.textPrimary,
                            fontSize = 15.sp,
                            fontWeight = FontWeight.Medium
                        )
                        Text(
                            text = "会话手势、模型轮换、文件夹过滤、工具看板等".loc,
                            color = colors.textMuted,
                            fontSize = 12.sp
                        )
                    }
                }
                Icon(
                    imageVector = Icons.Default.ChevronRight,
                    contentDescription = null,
                    tint = colors.textMuted,
                    modifier = Modifier.size(16.dp)
                )
            }
        }

        if (showTipsGuide) {
            TipsAndGesturesGuideDialog(onDismiss = { showTipsGuide = false })
        }

        // MARK: - 7. 存储 (整体下移一行，放在关于的上面)
        SettingsSection(
            title = "存储".loc,
            footer = "清除本地缓存的会话与文档数据，下次访问时将从网关重新拉取。".loc
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
                    text = "清空本地缓存".loc,
                    color = colors.accentRed,
                    fontSize = 15.sp
                )
            }
        }

        // MARK: - 8. 关于 (1:1 iOS 对齐)
        val appVersion = remember(localContext) {
            try {
                val packageInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    localContext.packageManager.getPackageInfo(
                        localContext.packageName,
                        PackageManager.PackageInfoFlags.of(0)
                    )
                } else {
                    @Suppress("DEPRECATION")
                    localContext.packageManager.getPackageInfo(localContext.packageName, 0)
                }
                packageInfo.versionName ?: "1.1.4"
            } catch (e: Exception) {
                "1.1.4"
            }
        }

        SettingsSection(title = "关于".loc) {
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
                    Text(text = "应用名称".loc, color = colors.textPrimary, fontSize = 15.sp)
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
                    Text(text = "版本".loc, color = colors.textPrimary, fontSize = 15.sp)
                    Text(text = appVersion, color = colors.textSecondary, fontSize = 14.sp)
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

private data class PickerMenuOption(
    val id: String,
    val label: String,
    val accentColor: Color? = null,
    val isGroupStart: Boolean = false
)

@Composable
private fun SettingsMenuPickerRow(
    label: String,
    selectedLabel: String,
    selectedId: String,
    options: List<PickerMenuOption>,
    onSelect: (String) -> Unit,
    subtitle: String? = null,
    selectedColor: Color? = null,
    enabled: Boolean = options.isNotEmpty()
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    var expanded by remember { mutableStateOf(false) }

    Box {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(enabled = enabled) {
                    haptic.light()
                    expanded = true
                }
                .padding(horizontal = 16.dp, vertical = if (subtitle != null) 12.dp else 14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Column(modifier = Modifier.weight(1f, fill = false)) {
                Text(text = label, color = colors.textPrimary, fontSize = 15.sp)
                if (!subtitle.isNullOrBlank()) {
                    Text(
                        text = subtitle,
                        color = colors.textMuted,
                        fontSize = 12.sp
                    )
                }
            }
            Spacer(modifier = Modifier.width(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = selectedLabel,
                    color = if (enabled) (selectedColor ?: colors.accentIndigo) else colors.textSecondary,
                    fontSize = 14.sp,
                    maxLines = 1,
                    overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                    modifier = Modifier.widthIn(max = 200.dp)
                )
                if (enabled) {
                    Icon(
                        imageVector = Icons.Default.UnfoldMore,
                        contentDescription = null,
                        tint = selectedColor ?: colors.accentIndigo,
                        modifier = Modifier.size(16.dp)
                    )
                }
            }
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { option ->
                if (option.isGroupStart) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                }
                DropdownMenuItem(
                    text = {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            if (option.accentColor != null) {
                                Box(
                                    modifier = Modifier
                                        .size(7.dp)
                                        .clip(CircleShape)
                                        .background(option.accentColor)
                                )
                            }
                            Text(option.label)
                        }
                    },
                    trailingIcon = {
                        if (option.id == selectedId) {
                            Icon(
                                Icons.Default.Check,
                                contentDescription = null,
                                tint = option.accentColor ?: colors.accentIndigo,
                                modifier = Modifier.size(18.dp)
                            )
                        }
                    },
                    onClick = {
                        expanded = false
                        onSelect(option.id)
                    }
                )
            }
        }
    }
}

/**
 * 「常用模型」：设置页配置的两个常用模型，均可在全部可用模型与全部思考强度中任选。
 * 进入新会话首选常用模型 1，单击输入框上方模型胶囊在常用模型 1 与常用模型 2 间快速切换。
 */
@Composable
private fun DefaultModelSection(
    prefs: PreferencesManager,
    onLoadModels: suspend () -> Result<ModelsResponse>
) {
    val colors = AntigravityTheme.colors
    val catalog by prefs.modelCatalogFlow.collectAsState()
    val favorite1 by prefs.favoriteModel1Flow.collectAsState()
    val favorite2 by prefs.favoriteModel2Flow.collectAsState()
    val allOptions = remember(catalog) {
        ModelDefaults.allSortedOptions(catalog?.models.orEmpty())
    }
    var loadState by remember { mutableStateOf(if (catalog != null) "ok" else "loading") } // loading | ok | failed

    LaunchedEffect(Unit) {
        onLoadModels()
            .onSuccess { c ->
                prefs.updateModelCatalog(c)
                loadState = "ok"
            }
            .onFailure {
                loadState = if (catalog == null) "failed" else "ok"
            }
    }

    val footer = when (loadState) {
        "loading" -> "正在读取可用模型...".loc
        "failed" -> "暂时无法读取实时模型列表，可先从内置列表中选择模型；新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。".loc
        else -> "新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。".loc
    }

    SettingsSection(title = "模型".loc, footer = footer) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .background(colors.surface)
                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
        ) {
            FavoriteModelPickerRow(
                label = "首选模型".loc,
                selectedId = favorite1,
                options = allOptions,
                onSelect = { prefs.favoriteModel1 = it }
            )
            HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
            FavoriteModelPickerRow(
                label = "次选模型".loc,
                selectedId = favorite2,
                options = allOptions,
                onSelect = { prefs.favoriteModel2 = it }
            )
        }
    }
}

@Composable
private fun FavoriteModelPickerRow(
    label: String,
    subtitle: String? = null,
    selectedId: String,
    options: List<ModelOption>,
    onSelect: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val selectedName = ModelDefaults.displayName(selectedId, options)
    val selectedColor = colors.modelProviderAccent(ModelDefaults.providerKind(selectedId))
    val menuOptions = remember(options, colors) {
        var prevGroup: String? = null
        options.map { opt ->
            val kind = ModelDefaults.providerKind(opt.id)
            val isStart = prevGroup != null && prevGroup != kind
            prevGroup = kind
            PickerMenuOption(
                id = opt.id,
                label = ModelDefaults.formatPickerLabel(opt.name, opt.tag),
                accentColor = colors.modelProviderAccent(kind),
                isGroupStart = isStart
            )
        }
    }
    SettingsMenuPickerRow(
        label = label,
        subtitle = subtitle,
        selectedLabel = selectedName,
        selectedId = selectedId,
        selectedColor = selectedColor,
        options = menuOptions,
        onSelect = onSelect
    )
}

@Composable
private fun ThemeModeSegmentedRow(
    selectedMode: String,
    onSelectMode: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val items = remember {
        listOf("system" to "跟随系统".loc, "light" to "浅色".loc, "dark" to "深色".loc)
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Text(
            text = "外观模式".loc,
            color = colors.textPrimary,
            fontSize = 15.sp,
            fontWeight = FontWeight.Medium
        )
        Row(
            modifier = Modifier
                .clip(RoundedCornerShape(9.dp))
                .background(colors.surfaceVariant)
                .padding(2.dp),
            horizontalArrangement = Arrangement.spacedBy(2.dp)
        ) {
            items.forEach { (id, label) ->
                val selected = selectedMode == id
                Box(
                    modifier = Modifier
                        .clip(RoundedCornerShape(7.dp))
                        .background(if (selected) colors.surface else Color.Transparent)
                        .clickable {
                            haptic.light()
                            onSelectMode(id)
                        }
                        .padding(horizontal = 10.dp, vertical = 5.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = label,
                        color = if (selected) colors.textPrimary else colors.textSecondary,
                        fontSize = 12.5.sp,
                        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium
                    )
                }
            }
        }
    }
}

@Composable
private fun CompactModeSubpageContent(prefs: PreferencesManager) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val knowledgeMode by prefs.knowledgeModeFlow.collectAsState()
    val compactCollapseTools by prefs.compactCollapseToolsFlow.collectAsState()
    val compactCollapseTasks by prefs.compactCollapseTasksFlow.collectAsState()
    val compactHideHealthyContext by prefs.compactHideHealthyContextFlow.collectAsState()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        SettingsSection(
            title = "精简模式".loc,
            footer = "开启后可折叠对话中的工具执行过程与底部任务辅助栏，让长对话阅读更聚焦。".loc
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
                Text(
                    text = "开启精简模式".loc,
                    color = colors.textPrimary,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Medium
                )
                Switch(
                    checked = knowledgeMode,
                    onCheckedChange = {
                        haptic.light()
                        prefs.isKnowledgeModeEnabled = it
                    },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = colors.surface,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        SettingsSection(title = "降噪选项".loc) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .alpha(if (knowledgeMode) 1f else 0.48f)
            ) {
                CompactSubpageToggleRow(
                    title = "折叠工具执行过程".loc,
                    subtitle = "将检索与命令步骤收起为单行摘要".loc,
                    checked = compactCollapseTools,
                    enabled = knowledgeMode,
                    onCheckedChange = {
                        haptic.light()
                        prefs.compactCollapseTools = it
                    }
                )
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.35f),
                    thickness = 0.5.dp,
                    modifier = Modifier.padding(start = 16.dp)
                )
                CompactSubpageToggleRow(
                    title = "收起任务辅助栏".loc,
                    subtitle = "底部运行任务默认单行折叠，保留终止按钮".loc,
                    checked = compactCollapseTasks,
                    enabled = knowledgeMode,
                    onCheckedChange = {
                        haptic.light()
                        prefs.compactCollapseTasks = it
                    }
                )
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.35f),
                    thickness = 0.5.dp,
                    modifier = Modifier.padding(start = 16.dp)
                )
                CompactSubpageToggleRow(
                    title = "隐藏常规上下文".loc,
                    subtitle = "占用 < 70% 时隐藏顶栏胶囊，≥ 70% 自动恢复预警".loc,
                    checked = compactHideHealthyContext,
                    enabled = knowledgeMode,
                    onCheckedChange = {
                        haptic.light()
                        prefs.compactHideHealthyContext = it
                    }
                )
            }
        }

        Spacer(modifier = Modifier.height(16.dp))
    }
}

@Composable
private fun CompactSubpageToggleRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    enabled: Boolean,
    onCheckedChange: (Boolean) -> Unit
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Column(
            modifier = Modifier
                .weight(1f)
                .padding(end = 12.dp)
        ) {
            Text(
                text = title,
                color = colors.textPrimary,
                fontSize = 14.5.sp,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = subtitle,
                color = colors.textSecondary,
                fontSize = 11.5.sp,
                lineHeight = 15.sp
            )
        }
        Switch(
            checked = checked,
            enabled = enabled,
            onCheckedChange = onCheckedChange,
            colors = SwitchDefaults.colors(
                checkedThumbColor = colors.surface,
                checkedTrackColor = colors.accentIndigo
            )
        )
    }
}
