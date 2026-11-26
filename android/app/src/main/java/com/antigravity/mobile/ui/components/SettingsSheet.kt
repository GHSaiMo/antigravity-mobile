package com.antigravity.mobile.ui.components

import android.Manifest
import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.widget.Toast
import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
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
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.AppReleaseInfo
import com.antigravity.mobile.data.service.AppUpdateInstaller
import com.antigravity.mobile.data.service.AppUpdateParser
import com.antigravity.mobile.data.service.AppUpdateState
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.fetchDesktopUpdateStatus
import com.antigravity.mobile.data.service.fetchPermissionSettings
import com.antigravity.mobile.data.service.updatePermissionPreset
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.modelProviderAccent
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

private enum class SettingsSubpage {
    MAIN,
    COMPACT,
    QUOTA_DISPLAY,
    NOTIFICATION_ALERTS,
    AI_ASSIST,
    TIPS;

    val title: String
        get() = when (this) {
            MAIN -> "设置".loc
            COMPACT -> "精简模式".loc
            QUOTA_DISPLAY -> "额度显示".loc
            NOTIFICATION_ALERTS -> when (L10n.currentLang) {
                L10n.AppLang.EN -> "Alert Types"
                L10n.AppLang.JA -> "通知項目"
                L10n.AppLang.ZH -> "提醒内容"
            }
            AI_ASSIST -> when (L10n.currentLang) {
                L10n.AppLang.EN -> "AI Assist"
                L10n.AppLang.JA -> "AI アシスト"
                L10n.AppLang.ZH -> "智能辅助"
            }
            TIPS -> "手势与使用技巧".loc
        }
}

private fun resolveAppShareUrl(): String {
    // 统一按当前语言指向官网三语 OG 落地页（direct / play 均合规；待 play 变体正式上架 Google Play 后可在此按渠道覆写）
    return when (L10n.currentLang) {
        L10n.AppLang.ZH -> "https://mgy.jiuge.space/zh.html"
        L10n.AppLang.JA -> "https://mgy.jiuge.space/ja.html"
        L10n.AppLang.EN -> "https://mgy.jiuge.space/"
    }
}

private fun shareMultigravityApp(context: Context) {
    val shareUrl = resolveAppShareUrl()
    val shareMessage = "推荐 Multigravity —— Google Antigravity 原生移动客户端，支持实时审批、任务进度与额度监控".loc
    val shareBody = "$shareMessage\n$shareUrl"
    val sendIntent = Intent(Intent.ACTION_SEND).apply {
        type = "text/plain"
        putExtra(Intent.EXTRA_SUBJECT, "Multigravity")
        putExtra(Intent.EXTRA_TEXT, shareBody)
    }
    try {
        val chooser = Intent.createChooser(sendIntent, "分享此应用".loc).apply {
            if (context !is Activity) {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
        }
        context.startActivity(chooser)
    } catch (_: ActivityNotFoundException) {
        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
        clipboard?.setPrimaryClip(ClipData.newPlainText("Multigravity", shareUrl))
        Toast.makeText(context, "已复制分享链接".loc, Toast.LENGTH_SHORT).show()
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
    var alertPrefs by remember { mutableStateOf(ApiClient.AlertPreferencesPayload()) }
    var alertPrefsError by remember { mutableStateOf<String?>(null) }
    val aiAssistStatus by com.antigravity.mobile.data.service.AIAssistCache.statusFlow.collectAsState()
    var aiAssistSyncError by remember { mutableStateOf<String?>(null) }
    var activeSubpage by remember { mutableStateOf(SettingsSubpage.MAIN) }
    val sheetScope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        if (prefs.isPaired()) {
            ApiClient.defaultInstance?.fetchAlertPreferences()?.onSuccess {
                alertPrefs = it
            }
            com.antigravity.mobile.data.service.AIAssistCache.refresh(ApiClient.defaultInstance)
        }
    }
    var renderedSubpage by remember { mutableStateOf<SettingsSubpage?>(null) }
    val subpageProgress by animateFloatAsState(
        targetValue = if (activeSubpage != SettingsSubpage.MAIN) 1f else 0f,
        animationSpec = tween(durationMillis = 260, easing = FastOutSlowInEasing),
        label = "settings_subpage_slide"
    )
    LaunchedEffect(activeSubpage, subpageProgress) {
        if (activeSubpage == SettingsSubpage.MAIN && subpageProgress == 0f) {
            renderedSubpage = null
        }
    }
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val focusManager = LocalFocusManager.current
    val connectionManager = remember { ConnectionManager(context) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val currentThemeMode by prefs.themeModeFlow.collectAsState()
    val openSubpage: (SettingsSubpage) -> Unit = { target ->
        focusManager.clearFocus()
        renderedSubpage = target
        activeSubpage = target
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
            BackHandler(enabled = activeSubpage != SettingsSubpage.MAIN) {
                activeSubpage = SettingsSubpage.MAIN
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

                // Sliding navigation container: keep MAIN mounted so its scroll position is never reset
                BoxWithConstraints(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .clipToBounds()
                ) {
                    val widthPx = constraints.maxWidth.toFloat()

                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .graphicsLayer {
                                translationX = -widthPx * subpageProgress
                            }
                    ) {
                        SettingsPageHeader(
                            title = SettingsSubpage.MAIN.title,
                            showBack = false,
                            onBack = {}
                        )
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
                                alertPrefs = alertPrefs,
                                aiAssistStatus = aiAssistStatus,
                                onThemeModeChange = onThemeModeChange,
                                onOpenCompactSettings = { openSubpage(SettingsSubpage.COMPACT) },
                                onOpenQuotaDisplay = { openSubpage(SettingsSubpage.QUOTA_DISPLAY) },
                                onOpenAIAssist = {
                                    openSubpage(SettingsSubpage.AI_ASSIST)
                                    if (prefs.isPaired()) {
                                        sheetScope.launch {
                                            aiAssistSyncError = null
                                            com.antigravity.mobile.data.service.AIAssistCache.refresh(ApiClient.defaultInstance)
                                        }
                                    }
                                },
                                onOpenNotificationAlerts = {
                                    openSubpage(SettingsSubpage.NOTIFICATION_ALERTS)
                                    if (prefs.isPaired()) {
                                        sheetScope.launch {
                                            ApiClient.defaultInstance?.fetchAlertPreferences()?.onSuccess {
                                                alertPrefs = it
                                                alertPrefsError = null
                                            }
                                        }
                                    }
                                },
                                onOpenTipsGuide = { openSubpage(SettingsSubpage.TIPS) },
                                onPromptUnpair = { showUnpairAlert = true },
                                onPromptClearCache = { showClearCacheAlert = true },
                                onOpenMcpSkills = { showMcpSkillsSheet = true },
                                onLoadModels = onLoadModels
                            )
                        }
                    }

                    val currentSubpage = renderedSubpage
                    if (currentSubpage != null && currentSubpage != SettingsSubpage.MAIN) {
                        Column(
                            modifier = Modifier
                                .fillMaxSize()
                                .graphicsLayer {
                                    translationX = widthPx * (1f - subpageProgress)
                                }
                                .background(colors.background)
                                .clickable(
                                    interactionSource = remember { MutableInteractionSource() },
                                    indication = null,
                                    onClick = {}
                                )
                        ) {
                            SettingsPageHeader(
                                title = currentSubpage.title,
                                showBack = true,
                                onBack = { activeSubpage = SettingsSubpage.MAIN }
                            )
                            HorizontalDivider(thickness = 0.5.dp, color = colors.border)
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .weight(1f)
                            ) {
                                when (currentSubpage) {
                                    SettingsSubpage.COMPACT -> CompactModeSubpageContent(prefs = prefs)
                                    SettingsSubpage.QUOTA_DISPLAY -> QuotaDisplaySubpageContent(prefs = prefs)
                                    SettingsSubpage.NOTIFICATION_ALERTS -> NotificationAlertsSubpageContent(
                                        alertPrefs = alertPrefs,
                                        syncError = alertPrefsError,
                                        isPaired = prefs.isPaired(),
                                        onToggle = { key, value ->
                                            val prev = alertPrefs
                                            alertPrefs = prev.withUpdated(key, value)
                                            alertPrefsError = null
                                            if (prefs.isPaired()) {
                                                sheetScope.launch {
                                                    val res = ApiClient.defaultInstance?.updateAlertPreference(key, value)
                                                    if (res != null && res.isSuccess) {
                                                        alertPrefs = res.getOrThrow()
                                                    } else {
                                                        alertPrefs = prev
                                                        alertPrefsError = when (L10n.currentLang) {
                                                            L10n.AppLang.EN -> "Sync failed. Check gateway connection."
                                                            L10n.AppLang.JA -> "同期に失敗しました。接続を確認してください。"
                                                            L10n.AppLang.ZH -> "同步失败，请检查网关连接"
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    )
                                    SettingsSubpage.AI_ASSIST -> AIAssistSubpageContent(
                                        status = aiAssistStatus,
                                        syncError = aiAssistSyncError,
                                        isPaired = prefs.isPaired(),
                                        onSyncError = { aiAssistSyncError = it }
                                    )
                                    SettingsSubpage.TIPS -> TipsAndGesturesSubpageContent()
                                    SettingsSubpage.MAIN -> Unit
                                }
                            }
                        }
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
                val gwStore = com.antigravity.mobile.data.service.GatewayProfileStore.sharedOrNull()
                val activeHostName = gwStore?.activeProfile?.resolvedDisplayName
                val msg = if (gwStore?.hasMultipleGateways == true && !activeHostName.isNullOrBlank()) {
                    "解除与「%s」的配对将通知该网关清理此设备绑定，并自动切换至其他已配对主机。".loc(activeHostName)
                } else {
                    "解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。".loc
                }
                Text(
                    text = msg,
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
private fun SettingsPageHeader(
    title: String,
    showBack: Boolean,
    onBack: () -> Unit
) {
    val colors = AntigravityTheme.colors
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = 34.dp)
            .padding(horizontal = 12.dp)
            .padding(bottom = 10.dp),
        contentAlignment = Alignment.Center
    ) {
        if (showBack) {
            Row(
                modifier = Modifier
                    .align(Alignment.CenterStart)
                    .clip(RoundedCornerShape(8.dp))
                    .clickable(onClick = onBack)
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
            text = title,
            fontWeight = FontWeight.Bold,
            fontSize = 18.sp,
            color = colors.textPrimary,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = if (showBack) 96.dp else 0.dp)
        )
    }
}

@Composable
private fun MainSettingsContent(
    prefs: PreferencesManager,
    connectionManager: ConnectionManager,
    currentThemeMode: String,
    alertPrefs: ApiClient.AlertPreferencesPayload,
    aiAssistStatus: com.antigravity.mobile.data.service.AIAssistStatusPayload,
    onThemeModeChange: (String) -> Unit,
    onOpenCompactSettings: () -> Unit,
    onOpenQuotaDisplay: () -> Unit,
    onOpenAIAssist: () -> Unit,
    onOpenNotificationAlerts: () -> Unit,
    onOpenTipsGuide: () -> Unit,
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
    val currentPlatform by prefs.gatewayPlatformFlow.collectAsState()
    val gatewayDesc = when (currentPlatform?.lowercase()) {
        "windows", "win" -> "Windows 网关".loc
        "darwin", "macos", "mac" -> "Mac 网关".loc
        "linux" -> "Linux 网关".loc
        else -> "电脑网关".loc
    }
    val gwStore = remember { com.antigravity.mobile.data.service.GatewayProfileStore.sharedOrNull() }
    val emptyProfiles = remember { kotlinx.coroutines.flow.MutableStateFlow<List<com.antigravity.mobile.data.model.GatewayProfile>>(emptyList()) }
    val emptyActiveId = remember { kotlinx.coroutines.flow.MutableStateFlow<String?>(null) }
    val gwProfiles by (gwStore?.profilesFlow ?: emptyProfiles).collectAsState()
    val gwActiveId by (gwStore?.activeProfileIdFlow ?: emptyActiveId).collectAsState()
    val hasMultipleGateways = isPaired && gwProfiles.size > 1
    val activeHostLabel = gwProfiles.firstOrNull { it.id == gwActiveId }?.resolvedDisplayName
        ?: gwProfiles.firstOrNull()?.resolvedDisplayName
        ?: gatewayDesc

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
            footer = if (hasMultipleGateways) {
                "管理当前设备与「%s」的配对状态。".loc(activeHostLabel)
            } else {
                "管理当前设备与 %s的配对状态。".loc(gatewayDesc)
            }
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
                    Spacer(modifier = Modifier.width(12.dp))
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(5.dp),
                        modifier = Modifier.weight(1f, fill = false)
                    ) {
                        if (isPaired) {
                            Icon(
                                imageVector = Icons.Default.CheckCircle,
                                contentDescription = "Paired",
                                tint = colors.accentGreen,
                                modifier = Modifier.size(16.dp)
                            )
                            Text(
                                text = if (hasMultipleGateways) "已配对 · %s".loc(activeHostLabel) else "已配对".loc,
                                color = colors.textSecondary,
                                fontSize = 14.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
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
        var permMode by remember {
            val saved = permPrefs.getString("cached_permission_preset", "strict") ?: "strict"
            mutableStateOf(
                when (saved.lowercase()) {
                    "sandbox" -> "default"
                    "strict", "default", "auto", "turbo" -> saved.lowercase()
                    else -> "strict"
                }
            )
        }
        var pendingTurboConfirm by remember { mutableStateOf(false) }
        LaunchedEffect(Unit) {
            ApiClient.defaultInstance?.fetchPermissionSettings()?.onSuccess {
                permMode = it.effectiveMode
                permPrefs.edit().putString("cached_permission_preset", it.effectiveMode).apply()
            }
        }
        val currentLang = L10n.currentLang
        val permissionOptions = remember(colors, currentLang) {
            listOf(
                PickerMenuOption("strict", "严格审批".loc, accentColor = colors.accentIndigo),
                PickerMenuOption("default", "默认沙箱".loc, accentColor = colors.accentBlue),
                PickerMenuOption("auto", "智能自动 · 沙箱".loc, accentColor = colors.accentOrange),
                PickerMenuOption("turbo", "全自动 · 无沙箱".loc, accentColor = colors.accentRed)
            )
        }
        fun applyPermissionMode(key: String) {
            permMode = key
            permPrefs.edit().putString("cached_permission_preset", key).apply()
            coroutineScope.launch {
                ApiClient.defaultInstance?.updatePermissionPreset(key)?.onSuccess {
                    permMode = it.effectiveMode
                    permPrefs.edit().putString("cached_permission_preset", it.effectiveMode).apply()
                }
            }
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
                    selectedLabel = permissionOptions.firstOrNull { it.id == permMode }?.label ?: "严格审批".loc,
                    selectedId = permMode,
                    selectedColor = when (permMode) {
                        "strict" -> colors.accentIndigo
                        "default" -> colors.accentBlue
                        "turbo" -> colors.accentRed
                        else -> colors.accentOrange
                    },
                    options = permissionOptions,
                    onSelect = { key ->
                        if (key == "turbo") {
                            pendingTurboConfirm = true
                        } else {
                            applyPermissionMode(key)
                        }
                    }
                )
                if (pendingTurboConfirm) {
                    AlertDialog(
                        onDismissRequest = { pendingTurboConfirm = false },
                        containerColor = colors.surface,
                        title = { Text("开启全自动 · 无沙箱？".loc, fontWeight = FontWeight.Bold) },
                        text = {
                            Text(
                                "开启后 Agent 将自动执行终端命令并访问工作区外文件（同步作用于桌面端全部会话）。建议仅在可信或隔离环境中使用。".loc,
                                fontSize = 13.5.sp,
                                color = colors.textSecondary
                            )
                        },
                        confirmButton = {
                            Button(
                                onClick = {
                                    pendingTurboConfirm = false
                                    applyPermissionMode("turbo")
                                },
                                colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed)
                            ) { Text("确认开启".loc) }
                        },
                        dismissButton = {
                            TextButton(onClick = { pendingTurboConfirm = false }) {
                                Text("取消".loc, color = colors.textSecondary)
                            }
                        }
                    )
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenMcpSkills() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(modifier = Modifier.weight(1f).padding(end = 8.dp)) {
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
                    Icon(
                        imageVector = Icons.Default.ChevronRight,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(16.dp)
                    )
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                val devLangForSummary = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
                val aiSummary = run {
                    val titleOn = devLangForSummary.shouldShowTitleTranslation(aiAssistStatus.prefs) && aiAssistStatus.prefs.titleTranslateEnabled
                    val stepOn = devLangForSummary.shouldShowLiveTranslation(aiAssistStatus.prefs) && aiAssistStatus.prefs.stepPlanTranslateEnabled
                    val focusOn = devLangForSummary.shouldShowLiveTranslation(aiAssistStatus.prefs) && aiAssistStatus.prefs.focusTranslateEnabled
                    val nextOn = aiAssistStatus.prefs.nextPromptEnabled
                    val enabledCount = listOf(titleOn, stepOn, focusOn, nextOn).count { it }
                    when {
                        enabledCount >= 2 -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "$enabledCount Enabled"
                            L10n.AppLang.JA -> "$enabledCount 件有効"
                            L10n.AppLang.ZH -> "已开启 $enabledCount 项"
                        }
                        titleOn -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Title Translation"
                            L10n.AppLang.JA -> "タイトル翻訳"
                            L10n.AppLang.ZH -> "标题翻译"
                        }
                        stepOn -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Step Translation"
                            L10n.AppLang.JA -> "ステップ翻訳"
                            L10n.AppLang.ZH -> "步骤翻译"
                        }
                        focusOn -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Status Translation"
                            L10n.AppLang.JA -> "ステータス翻訳"
                            L10n.AppLang.ZH -> "状态翻译"
                        }
                        nextOn -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Prompt Suggestions"
                            L10n.AppLang.JA -> "プロンプト提案"
                            L10n.AppLang.ZH -> "提示词建议"
                        }
                        else -> "已关闭".loc
                    }
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenAIAssist() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = when (L10n.currentLang) {
                            L10n.AppLang.EN -> "AI Assist"
                            L10n.AppLang.JA -> "AI アシスト"
                            L10n.AppLang.ZH -> "智能辅助"
                        },
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.weight(1f)
                    )
                    Text(
                        text = aiSummary,
                        color = colors.textSecondary,
                        fontSize = 13.5.sp,
                        maxLines = 1,
                        modifier = Modifier.padding(end = 6.dp)
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

                HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                val alertSummary = remember(alertPrefs, L10n.currentLang) {
                    val count = alertPrefs.enabledCount
                    val total = alertPrefs.visibleTotalCount
                    when {
                        count == total -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "All On"
                            L10n.AppLang.JA -> "すべてオン"
                            L10n.AppLang.ZH -> "全部开启"
                        }
                        count == 0 -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "All Off"
                            L10n.AppLang.JA -> "すべてオフ"
                            L10n.AppLang.ZH -> "全部关闭"
                        }
                        else -> when (L10n.currentLang) {
                            L10n.AppLang.EN -> "$count/$total On"
                            L10n.AppLang.JA -> "$count/$total オン"
                            L10n.AppLang.ZH -> "已开 $count/$total"
                        }
                    }
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenNotificationAlerts() }
                        .padding(horizontal = 16.dp, vertical = 13.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Alert Types"
                            L10n.AppLang.JA -> "通知項目"
                            L10n.AppLang.ZH -> "提醒内容"
                        },
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = alertSummary,
                            color = colors.textSecondary,
                            fontSize = 13.sp,
                            maxLines = 1,
                            softWrap = false
                        )
                        Icon(
                            imageVector = Icons.Default.ChevronRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                    }
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
        val weeklyPaceCursor by prefs.weeklyPaceCursorFlow.collectAsState()
        val currentFontScale by prefs.fontSizeScaleFlow.collectAsState()
        val knowledgeMode by prefs.knowledgeModeFlow.collectAsState()
        val compactCollapseTools by prefs.compactCollapseToolsFlow.collectAsState()
        val compactCollapseTasks by prefs.compactCollapseTasksFlow.collectAsState()
        val compactHideHealthyContext by prefs.compactHideHealthyContextFlow.collectAsState()
        val pickerQuotaEnabled by com.antigravity.mobile.data.service.ModelPickerQuotaCache.enabled.collectAsState()
        val quotaDisplaySummary = run {
            when {
                weeklyPaceCursor && pickerQuotaEnabled -> "${"匀速".loc} · ${"余量".loc}"
                weeklyPaceCursor -> "匀速游标".loc
                pickerQuotaEnabled -> "余量面板".loc
                else -> "已关闭".loc
            }
        }
        val compactSummary = remember(knowledgeMode, compactCollapseTools, compactCollapseTasks, compactHideHealthyContext, currentLang) {
            if (!knowledgeMode) "已关闭".loc
            else {
                val activeCount = listOf(compactCollapseTools, compactCollapseTasks, compactHideHealthyContext).count { it }
                "已开启 (%d/3)".loc(activeCount)
            }
        }
        val fontScaleOptions = remember(currentLang) {
            listOf(
                PickerMenuOption("compact", "紧凑 (90%)".loc),
                PickerMenuOption("standard", "标准 (100%)".loc),
                PickerMenuOption("comfortable", "舒适 (115%)".loc)
            )
        }
        SettingsSection(
            title = "外观与主题".loc,
            footer = "选择浅色、深色模式与界面字号缩放；点进「额度显示」与「精简模式」可自定义配额指示和对话降噪选项。".loc
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
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onOpenQuotaDisplay() }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = "额度显示".loc,
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = quotaDisplaySummary,
                            color = colors.textSecondary,
                            fontSize = 14.sp,
                            maxLines = 1,
                            softWrap = false
                        )
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(18.dp)
                        )
                    }
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
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = compactSummary,
                            color = colors.textSecondary,
                            fontSize = 14.sp,
                            maxLines = 1,
                            softWrap = false
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
                    .clickable { onOpenTipsGuide() }
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
                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                    contentDescription = null,
                    tint = colors.textMuted,
                    modifier = Modifier.size(18.dp)
                )
            }
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

        // MARK: - 8. 关于 (1:1 iOS 对齐；直装版轻点版本行可检查更新，Play 版保持纯静态)
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
                packageInfo.versionName ?: "1.2.7"
            } catch (e: Exception) {
                "1.2.7"
            }
        }
        val appUpdateState by AppUpdateInstaller.state.collectAsState()
        val updateHaptic = rememberHaptic()
        var showAppUpdateDialog by remember { mutableStateOf(false) }
        var desktopUpdateStatus by remember { mutableStateOf<com.antigravity.mobile.data.model.DesktopUpdateStatus?>(null) }
        var showDesktopUpdateDialog by remember { mutableStateOf(false) }
        LaunchedEffect(isPaired) {
            if (isPaired) {
                ApiClient.defaultInstance?.fetchDesktopUpdateStatus()?.onSuccess {
                    desktopUpdateStatus = it
                }
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
                val versionRowModifier = if (AppUpdateInstaller.isSupported) {
                    Modifier
                        .fillMaxWidth()
                        .clickable {
                            updateHaptic.light()
                            when (appUpdateState) {
                                is AppUpdateState.Checking -> Unit
                                is AppUpdateState.Available,
                                is AppUpdateState.Downloading,
                                is AppUpdateState.ReadyToInstall,
                                is AppUpdateState.NeedsInstallPermission -> {
                                    showAppUpdateDialog = true
                                }
                                else -> {
                                    AppUpdateInstaller.checkNow(localContext, silent = false) { result ->
                                        when (result) {
                                            is AppUpdateState.Available,
                                            is AppUpdateState.ReadyToInstall,
                                            is AppUpdateState.NeedsInstallPermission -> {
                                                showAppUpdateDialog = true
                                            }
                                            is AppUpdateState.UpToDate -> {
                                                Toast.makeText(
                                                    localContext,
                                                    "已是最新版本".loc,
                                                    Toast.LENGTH_SHORT
                                                ).show()
                                            }
                                            is AppUpdateState.Error -> {
                                                Toast.makeText(
                                                    localContext,
                                                    result.message,
                                                    Toast.LENGTH_SHORT
                                                ).show()
                                            }
                                            else -> Unit
                                        }
                                    }
                                }
                            }
                        }
                        .padding(horizontal = 16.dp, vertical = 14.dp)
                } else {
                    Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp)
                }
                Row(
                    modifier = versionRowModifier,
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "版本".loc, color = colors.textPrimary, fontSize = 15.sp)
                    if (!AppUpdateInstaller.isSupported) {
                        Text(text = appVersion, color = colors.textSecondary, fontSize = 14.sp)
                    } else {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            when (val st = appUpdateState) {
                                is AppUpdateState.Checking -> {
                                    CircularProgressIndicator(
                                        modifier = Modifier.size(13.dp),
                                        strokeWidth = 1.6.dp,
                                        color = colors.accentIndigo
                                    )
                                    Text(
                                        text = "检查中…".loc,
                                        color = colors.textSecondary,
                                        fontSize = 13.sp
                                    )
                                }
                                is AppUpdateState.Available -> {
                                    Text(
                                        text = appVersion,
                                        color = colors.textSecondary,
                                        fontSize = 14.sp
                                    )
                                    Text(
                                        text = "v${st.release.versionName} ${"可更新".loc}",
                                        color = colors.accentIndigo,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(999.dp))
                                            .background(colors.accentIndigo.copy(alpha = 0.12f))
                                            .padding(horizontal = 8.dp, vertical = 2.dp)
                                    )
                                }
                                is AppUpdateState.Downloading -> {
                                    val pct = if (!st.selectingChannel && st.downloadedBytes > 0L && st.totalBytes > 0L) {
                                        ((st.downloadedBytes * 100L) / st.totalBytes).coerceIn(0L, 100L)
                                    } else {
                                        -1L
                                    }
                                    Text(
                                        text = if (pct >= 0L) "${"下载中".loc} $pct%" else "下载中…".loc,
                                        color = colors.accentIndigo,
                                        fontSize = 12.5.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                }
                                is AppUpdateState.ReadyToInstall,
                                is AppUpdateState.NeedsInstallPermission -> {
                                    Text(
                                        text = appVersion,
                                        color = colors.textSecondary,
                                        fontSize = 14.sp
                                    )
                                    Text(
                                        text = "点击安装".loc,
                                        color = colors.accentIndigo,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(999.dp))
                                            .background(colors.accentIndigo.copy(alpha = 0.12f))
                                            .padding(horizontal = 8.dp, vertical = 2.dp)
                                    )
                                }
                                else -> {
                                    Text(
                                        text = appVersion,
                                        color = colors.textSecondary,
                                        fontSize = 14.sp
                                    )
                                }
                            }
                        }
                    }
                }
                if (isPaired) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { showDesktopUpdateDialog = true }
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(text = "Antigravity 桌面端".loc, color = colors.textPrimary, fontSize = 15.sp)
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            val curVer = desktopUpdateStatus?.currentVersion?.takeIf { it.isNotBlank() }
                            if (curVer != null) {
                                Text(text = "v$curVer", color = colors.textSecondary, fontSize = 13.5.sp)
                            }
                            if (desktopUpdateStatus?.updateAvailable == true) {
                                Text(
                                    text = "有更新".loc,
                                    color = colors.accentOrange,
                                    fontSize = 11.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(999.dp))
                                        .background(colors.accentOrange.copy(alpha = 0.14f))
                                        .padding(horizontal = 7.dp, vertical = 2.dp)
                                )
                            }
                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                contentDescription = null,
                                tint = colors.textMuted,
                                modifier = Modifier.size(16.dp)
                            )
                        }
                    }
                }
                HorizontalDivider(color = colors.separator.copy(alpha = 0.4f), thickness = 0.5.dp)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable {
                            updateHaptic.light()
                            shareMultigravityApp(localContext)
                        }
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(text = "分享此应用".loc, color = colors.textPrimary, fontSize = 15.sp)
                    Icon(
                        imageVector = Icons.Default.Share,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(16.dp)
                    )
                }
            }
        }

        if (showDesktopUpdateDialog) {
            DesktopUpdateDialog(onDismiss = { showDesktopUpdateDialog = false })
        }

        if (AppUpdateInstaller.isSupported && showAppUpdateDialog) {
            AppUpdateDialog(
                currentVersion = appVersion,
                state = appUpdateState,
                onDismiss = {
                    if (appUpdateState !is AppUpdateState.Downloading) {
                        AppUpdateInstaller.dismissError()
                    }
                    showAppUpdateDialog = false
                },
                onStartDownload = { release ->
                    AppUpdateInstaller.downloadAndInstall(localContext, release)
                },
                onCancelDownload = {
                    AppUpdateInstaller.cancelDownload()
                },
                onOpenInstallPermission = {
                    AppUpdateInstaller.openInstallPermissionSettings(localContext)
                },
                onTriggerInstall = {
                    AppUpdateInstaller.resumePendingInstall(localContext)
                },
                onOpenInBrowser = { url ->
                    runCatching {
                        localContext.startActivity(
                            Intent(Intent.ACTION_VIEW, Uri.parse(url)).apply {
                                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                            }
                        )
                    }
                }
            )
        }

        Spacer(modifier = Modifier.height(24.dp))
    }
}

@Composable
private fun AppUpdateDialog(
    currentVersion: String,
    state: AppUpdateState,
    onDismiss: () -> Unit,
    onStartDownload: (AppReleaseInfo) -> Unit,
    onCancelDownload: () -> Unit,
    onOpenInstallPermission: () -> Unit,
    onTriggerInstall: () -> Unit,
    onOpenInBrowser: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val release = when (state) {
        is AppUpdateState.Available -> state.release
        is AppUpdateState.Downloading -> state.release
        is AppUpdateState.ReadyToInstall -> state.release
        is AppUpdateState.NeedsInstallPermission -> state.release
        is AppUpdateState.Error -> state.release
        else -> null
    } ?: return

    val langPref by L10n.currentLangFlow.collectAsState()
    val currentLang = remember(langPref) { L10n.currentLang }
    val formattedNotes = remember(release.releaseNotes, currentLang) {
        AppUpdateParser.formatReleaseNotes(
            release.releaseNotes,
            preferChinese = currentLang == L10n.AppLang.ZH,
            preferJapanese = currentLang == L10n.AppLang.JA
        )
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = colors.surface,
        title = {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = "${"发现新版本".loc} v${release.versionName}",
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold,
                    fontSize = 17.sp
                )
                Text(
                    text = "v${AppUpdateParser.normalizeVersion(currentVersion)} → v${release.versionName}",
                    color = colors.textSecondary,
                    fontSize = 12.5.sp,
                    fontFamily = FontFamily.Monospace
                )
            }
        },
        text = {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 280.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (formattedNotes.isNotBlank()) {
                    Text(
                        text = formattedNotes,
                        color = colors.textSecondary,
                        fontSize = 13.sp,
                        lineHeight = 19.sp
                    )
                }

                when (state) {
                    is AppUpdateState.Downloading -> {
                        val progress = if (!state.selectingChannel && state.downloadedBytes > 0L && state.totalBytes > 0L) {
                            (state.downloadedBytes.toFloat() / state.totalBytes.toFloat()).coerceIn(0f, 1f)
                        } else {
                            null
                        }
                        if (progress != null) {
                            LinearProgressIndicator(
                                progress = { progress },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(4.dp)),
                                color = colors.accentIndigo,
                                trackColor = colors.border
                            )
                        } else {
                            LinearProgressIndicator(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(4.dp)),
                                color = colors.accentIndigo,
                                trackColor = colors.border
                            )
                        }
                        val statusText = if (state.selectingChannel && state.downloadedBytes <= 0L) {
                            "正在选择最快通道…".loc
                        } else {
                            val dlMb = String.format("%.1f MB", state.downloadedBytes / (1024.0 * 1024.0))
                            val totalMb = if (state.totalBytes > 0L) {
                                " / " + String.format("%.1f MB", state.totalBytes / (1024.0 * 1024.0))
                            } else {
                                ""
                            }
                            val speedSuffix = when {
                                state.speedBytesPerSec >= 1024L * 1024L ->
                                    " · " + String.format("%.1f MB/s", state.speedBytesPerSec / (1024.0 * 1024.0))
                                state.speedBytesPerSec >= 1024L ->
                                    " · ${state.speedBytesPerSec / 1024L} KB/s"
                                else -> ""
                            }
                            val channelLabel = if (state.usingMirror) "国内镜像".loc else "GitHub"
                            "${"正在下载".loc} ($channelLabel) · $dlMb$totalMb$speedSuffix"
                        }
                        Text(
                            text = statusText,
                            color = colors.textMuted,
                            fontSize = 12.sp
                        )
                    }
                    is AppUpdateState.NeedsInstallPermission -> {
                        Text(
                            text = "安装包已校验完成。请先在系统设置中开启「允许来自此来源的应用」，返回后点击继续安装。".loc,
                            color = colors.accentIndigo,
                            fontSize = 12.5.sp,
                            lineHeight = 18.sp
                        )
                        if (state.signatureDiffers) {
                            Text(
                                text = "提示：新版已启用独立安全签名。若系统提示安装冲突，请先卸载旧版再安装，扫码即可重新配对。".loc,
                                color = colors.textMuted,
                                fontSize = 12.sp,
                                lineHeight = 17.sp
                            )
                        }
                    }
                    is AppUpdateState.ReadyToInstall -> {
                        Text(
                            text = "安装包已下载并完成完整性校验，点击下方按钮拉起系统安装。".loc,
                            color = colors.accentIndigo,
                            fontSize = 12.5.sp,
                            lineHeight = 18.sp
                        )
                        if (state.signatureDiffers) {
                            Text(
                                text = "提示：新版已启用独立安全签名。若系统提示安装冲突，请先卸载旧版再安装，扫码即可重新配对。".loc,
                                color = colors.textMuted,
                                fontSize = 12.sp,
                                lineHeight = 17.sp
                            )
                        }
                    }
                    is AppUpdateState.Error -> {
                        Text(
                            text = state.message,
                            color = colors.accentRed,
                            fontSize = 12.5.sp,
                            lineHeight = 18.sp
                        )
                        if (release.releasePageUrl.isNotBlank()) {
                            Text(
                                text = "在浏览器打开 GitHub Releases ›".loc,
                                color = colors.accentIndigo,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.clickable { onOpenInBrowser(release.releasePageUrl) }
                            )
                        }
                    }
                    else -> Unit
                }
            }
        },
        confirmButton = {
            when (state) {
                is AppUpdateState.Downloading -> {
                    TextButton(onClick = onCancelDownload) {
                        Text(text = "取消下载".loc, color = colors.accentRed)
                    }
                }
                is AppUpdateState.NeedsInstallPermission -> {
                    TextButton(onClick = onOpenInstallPermission) {
                        Text(
                            text = "前往授权".loc,
                            color = colors.accentIndigo,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }
                is AppUpdateState.ReadyToInstall -> {
                    TextButton(onClick = onTriggerInstall) {
                        Text(
                            text = "立即安装".loc,
                            color = colors.accentIndigo,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }
                else -> {
                    TextButton(onClick = { onStartDownload(release) }) {
                        Text(
                            text = "立即更新".loc,
                            color = colors.accentIndigo,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }
            }
        },
        dismissButton = {
            when (state) {
                is AppUpdateState.NeedsInstallPermission -> {
                    TextButton(onClick = onTriggerInstall) {
                        Text(text = "继续安装".loc, color = colors.textSecondary)
                    }
                }
                is AppUpdateState.Downloading -> {
                    TextButton(onClick = onDismiss) {
                        Text(text = "后台下载".loc, color = colors.textSecondary)
                    }
                }
                else -> {
                    TextButton(onClick = onDismiss) {
                        Text(text = "稍后".loc, color = colors.textSecondary)
                    }
                }
            }
        }
    )
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
            Column(modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)) {
                Text(text = label, color = colors.textPrimary, fontSize = 15.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (!subtitle.isNullOrBlank()) {
                    Text(
                        text = subtitle,
                        color = colors.textMuted,
                        fontSize = 12.sp
                    )
                }
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = selectedLabel,
                    color = if (enabled) (selectedColor ?: colors.accentIndigo) else colors.textSecondary,
                    fontSize = 14.sp,
                    maxLines = 1,
                    overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                    modifier = Modifier.widthIn(max = 175.dp)
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
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
            containerColor = colors.surface
        ) {
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
    val currentLang = L10n.currentLang
    val items = remember(currentLang) {
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
            fontWeight = FontWeight.Medium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false).padding(end = 8.dp)
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
                        .padding(horizontal = 8.dp, vertical = 5.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = label,
                        color = if (selected) colors.textPrimary else colors.textSecondary,
                        fontSize = 12.sp,
                        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
                        maxLines = 1,
                        softWrap = false
                    )
                }
            }
        }
    }
}

@Composable
private fun NotificationAlertsSubpageContent(
    alertPrefs: ApiClient.AlertPreferencesPayload,
    syncError: String?,
    isPaired: Boolean,
    onToggle: (String, Boolean) -> Unit
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val stallMins = if (alertPrefs.stallMinutes > 0) alertPrefs.stallMinutes else 8

    fun t(zh: String, en: String, ja: String): String = when (L10n.currentLang) {
        L10n.AppLang.EN -> en
        L10n.AppLang.JA -> ja
        L10n.AppLang.ZH -> zh
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        SettingsSection(
            title = t("任务与会话提醒", "Tasks & Sessions", "タスクとセッション"),
            footer = t(
                "审批请求与 Agent 提问在通知开启时始终提醒，确保离席不漏批。",
                "Approval requests and agent questions always notify while notifications are enabled.",
                "承認リクエストと Agent からの質問は、通知がオンの間は常に通知されます。"
            )
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                CompactSubpageToggleRow(
                    title = t("任务完成与失败", "Task Completed & Failed", "タスク完了・失敗"),
                    subtitle = t(
                        "任务执行完毕或异常终止时提醒",
                        "Notify when a run finishes or stops with an error",
                        "タスク完了またはエラー停止時に通知"
                    ),
                    checked = alertPrefs.taskCompletion,
                    enabled = true,
                    onCheckedChange = {
                        haptic.light()
                        onToggle("task_completion", it)
                    }
                )
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.35f),
                    thickness = 0.5.dp,
                    modifier = Modifier.padding(start = 16.dp)
                )
                CompactSubpageToggleRow(
                    title = t("长任务疑似卡死", "Stalled Long Task", "長時間タスクの停止検知"),
                    subtitle = t(
                        "连续 $stallMins 分钟无新输出或步骤推进时提醒",
                        "Notify after $stallMins min with no new output or step progress",
                        "$stallMins 分間新しい出力や進行がない場合に通知"
                    ),
                    checked = alertPrefs.taskStall,
                    enabled = true,
                    onCheckedChange = {
                        haptic.light()
                        onToggle("task_stall", it)
                    }
                )
                if (alertPrefs.isCacheMissLive) {
                    HorizontalDivider(
                        color = colors.separator.copy(alpha = 0.35f),
                        thickness = 0.5.dp,
                        modifier = Modifier.padding(start = 16.dp)
                    )
                    CompactSubpageToggleRow(
                        title = t("缓存连续未命中", "Repeated Cache Misses", "キャッシュ連続ミス"),
                        subtitle = t(
                            "长任务连续 3 步大上下文未命中缓存时提醒",
                            "Notify after 3 consecutive large-context cache misses",
                            "大コンテキストで3ステップ連続キャッシュ未命中時に通知"
                        ),
                        checked = alertPrefs.cacheMiss,
                        enabled = true,
                        onCheckedChange = {
                            haptic.light()
                            onToggle("cache_miss", it)
                        }
                    )
                }
            }
        }

        SettingsSection(
            title = t("额度与配额提醒", "Quota & Usage Alerts", "クォータと使用量"),
            footer = syncError ?: if (!isPaired) {
                t(
                    "配对网关后修改的开关将实时同步并生效。",
                    "Pair with a gateway to sync alert settings live.",
                    "ペアリング後に設定がゲートウェイへ即時同期されます。"
                )
            } else {
                t(
                    "开关实时同步至本机网关并跨设备生效。",
                    "Toggles sync live to your local gateway across paired devices.",
                    "設定はローカルゲートウェイへ即時同期され、全端末に反映されます。"
                )
            }
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                CompactSubpageToggleRow(
                    title = t("低额度预警", "Low Quota Warning", "残量低下アラート"),
                    subtitle = t(
                        "任一额度池降至 20% 或 5% 以下时提醒",
                        "Notify when any quota bucket drops below 20% or 5%",
                        "いずれかのクォータ残量が 20% または 5% 未満で通知"
                    ),
                    checked = alertPrefs.quotaLow,
                    enabled = true,
                    onCheckedChange = {
                        haptic.light()
                        onToggle("quota_low", it)
                    }
                )
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.35f),
                    thickness = 0.5.dp,
                    modifier = Modifier.padding(start = 16.dp)
                )
                CompactSubpageToggleRow(
                    title = t("额度提前重置提醒", "Bonus Quota Reset", "クォータ早期リセット通知"),
                    subtitle = t(
                        "检测到官方非周期提前回满额度时提醒",
                        "Notify when an unexpected early quota refill is detected",
                        "周期外の公式クォータ回復を検知した際に通知"
                    ),
                    checked = alertPrefs.quotaBonusReset,
                    enabled = true,
                    onCheckedChange = {
                        haptic.light()
                        onToggle("quota_bonus_reset", it)
                    }
                )
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.35f),
                    thickness = 0.5.dp,
                    modifier = Modifier.padding(start = 16.dp)
                )
                CompactSubpageToggleRow(
                    title = t("异常骤降与静默扣额", "Rapid Drop & Idle Drain", "急激な減少・アイドル消費"),
                    subtitle = t(
                        "短时骤降或无活跃会话却发生扣额时提醒",
                        "Notify on sudden quota drops or drain while idle",
                        "短時間の急減やアイドル中の消費を検知した際に通知"
                    ),
                    checked = alertPrefs.quotaAnomaly,
                    enabled = true,
                    onCheckedChange = {
                        haptic.light()
                        onToggle("quota_anomaly", it)
                    }
                )
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
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
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
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.weight(1f).padding(end = 8.dp)
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
private fun QuotaDisplaySubpageContent(prefs: PreferencesManager) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val weeklyPaceCursor by prefs.weeklyPaceCursorFlow.collectAsState()
    val enabled by com.antigravity.mobile.data.service.ModelPickerQuotaCache.enabled.collectAsState()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        SettingsSection(
            title = "配额条".loc,
            footer = "在首页与配额条上显示时间均速基准刻度针。".loc
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
                    text = "额度匀速指标".loc,
                    color = colors.textPrimary,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier.weight(1f).padding(end = 8.dp)
                )
                Switch(
                    checked = weeklyPaceCursor,
                    onCheckedChange = {
                        haptic.light()
                        prefs.weeklyPaceCursorEnabled = it
                    },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = colors.surface,
                        checkedTrackColor = colors.accentIndigo
                    )
                )
            }
        }

        SettingsSection(
            title = "模型选择器".loc,
            footer = "长按模型胶囊上滑时，在右侧伴随面板同屏显示 Gemini 与 Claude 共享池的 5h 与周剩余额度，低于 20% 标红。".loc
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
                    Text(
                        text = "显示余量".loc,
                        color = colors.textPrimary,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.weight(1f).padding(end = 8.dp)
                    )
                    Switch(
                        checked = enabled,
                        onCheckedChange = {
                            haptic.light()
                            prefs.modelPickerQuotaEnabled = it
                        },
                        colors = SwitchDefaults.colors(
                            checkedThumbColor = colors.surface,
                            checkedTrackColor = colors.accentIndigo
                        )
                    )
                }
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

@Composable
private fun AIAssistSubpageContent(
    status: com.antigravity.mobile.data.service.AIAssistStatusPayload,
    syncError: String?,
    isPaired: Boolean,
    onSyncError: (String?) -> Unit
) {
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    val devLang = remember(L10n.getLanguagePreference()) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
    }
    val prefs = status.prefs
    val showTitleSection = devLang.shouldShowTitleTranslation(prefs)
    val showLiveSection = devLang.shouldShowLiveTranslation(prefs)

    var isTranslatingBatch by remember { mutableStateOf(false) }
    var isRestoringTitles by remember { mutableStateOf(false) }
    var batchFeedback by remember { mutableStateOf<String?>(null) }
    var batchFeedbackIsError by remember { mutableStateOf(false) }
    var showRestoreConfirm by remember { mutableStateOf(false) }

    val activeTargetEnglishName = when {
        !devLang.isEnglish -> devLang.englishDisplayName
        prefs.titleTranslateLangName.isNotBlank() -> prefs.titleTranslateLangName
        status.resolvedLangName.isNotBlank() -> status.resolvedLangName
        else -> "Local Language"
    }

    val titleSectionHeader = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "会话标题翻译"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "会話タイトルの翻訳"
        else -> "Conversation Title Translation"
    }
    val titleToggleLabel = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE -> "自动翻译为简体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "自动翻译为繁体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "英語タイトルを日本語に自動翻訳"
        else -> "Auto-translate to $activeTargetEnglishName"
    }
    val titleToggleSubtitle = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "Google 生成英文标题后自动翻译并同步到桌面与手机"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "Google 生成の英語タイトルを自動翻訳しデスクトップと同期"
        else -> "Translate English titles into $activeTargetEnglishName and sync across desktop & mobile"
    }
    val batchButtonLabel = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "立即翻译历史英文标题"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "過去の英語タイトルを一括翻訳"
        else -> "Translate Existing Titles to $activeTargetEnglishName"
    }
    val restoreButtonLabel = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "还原已翻译标题（${status.translatedCount}）"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "翻訳済みタイトルを元に戻す（${status.translatedCount}）"
        else -> "Restore Original Titles (${status.translatedCount})"
    }
    val titleSectionFooter = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "仅翻译未归档活跃会话，不会覆盖手动重命名的标题；翻译结果直接写回 Antigravity，桌面端与手机端同步生效。"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "アーカイブ済みを除く会話のみ対象。手動変更したタイトルは上書きされず、デスクトップと同期されます。"
        else -> "Applies to active non-archived conversations only. Manually renamed titles are preserved."
    }

    val liveSectionHeader = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "进度与状态翻译"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "進捗・ステータスの翻訳"
        else -> "Progress & Status Translation"
    }
    val stepPlanToggleLabel = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "步骤清单实时翻译"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "ステップ一覧を自動翻訳"
        else -> "Translate Step Checklist"
    }
    val stepPlanToggleSubtitle = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE -> "将英文待办步骤翻译为简体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "将英文待办步骤翻译为繁体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "英語のタスクリストを日本語に自動翻訳"
        else -> "Translate English checklist steps into $activeTargetEnglishName"
    }
    val focusToggleLabel = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "实时状态行翻译"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "リアルタイム状況を自動翻訳"
        else -> "Translate Live Status Line"
    }
    val focusToggleSubtitle = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE -> "将执行中的英文状态摘要翻译为简体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "将执行中的英文状态摘要翻译为繁体中文"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "実行中の英語ステータスを日本語に自動翻訳"
        else -> "Translate live working status into $activeTargetEnglishName"
    }
    val liveSectionFooter = when (devLang.kind) {
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "仅在客户端与灵动岛展示译文，不修改桌面端原始记录；相同条目本地缓存复用，随时可独立开关。"
        com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "クライアント表示のみ翻訳され、デスクトップの元記録は変更されません。同じ項目はキャッシュで再利用されます。"
        else -> "Translates display on mobile, Web, and Live Activity only without modifying desktop history. Cached locally for reuse."
    }

    fun applyPatch(optimistic: com.antigravity.mobile.data.service.AIAssistPrefsPayload, patch: kotlinx.serialization.json.JsonObject) {
        val prev = prefs
        com.antigravity.mobile.data.service.AIAssistCache.updatePrefs(optimistic)
        onSyncError(null)
        if (!isPaired) return
        scope.launch {
            val res = ApiClient.defaultInstance?.patchAIAssistPrefs(patch)
            if (res != null && res.isSuccess) {
                com.antigravity.mobile.data.service.AIAssistCache.update(res.getOrThrow())
            } else {
                com.antigravity.mobile.data.service.AIAssistCache.updatePrefs(prev)
                onSyncError(
                    when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Sync failed. Check gateway connection."
                        L10n.AppLang.JA -> "同期に失敗しました。接続を確認してください。"
                        L10n.AppLang.ZH -> "同步失败，请检查网关连接"
                    }
                )
            }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 14.dp)
            .navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        if (showTitleSection) {
            SettingsSection(
                title = titleSectionHeader,
                footer = titleSectionFooter
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                ) {
                    CompactSubpageToggleRow(
                        title = titleToggleLabel,
                        subtitle = titleToggleSubtitle,
                        checked = prefs.titleTranslateEnabled,
                        enabled = isPaired && status.available,
                        onCheckedChange = { enabled ->
                            haptic.light()
                            val next = if (enabled && !devLang.isEnglish) {
                                prefs.copy(
                                    titleTranslateEnabled = true,
                                    titleTranslateLang = devLang.bcp47Tag,
                                    titleTranslateLangName = devLang.englishDisplayName
                                )
                            } else {
                                prefs.copy(titleTranslateEnabled = enabled)
                            }
                            val patch = buildJsonObject {
                                put("titleTranslateEnabled", enabled)
                                if (enabled && !devLang.isEnglish) {
                                    put("titleTranslateLang", devLang.bcp47Tag)
                                    put("titleTranslateLangName", devLang.englishDisplayName)
                                }
                            }
                            applyPatch(next, patch)
                        }
                    )

                    if (!devLang.isEnglish) {
                        HorizontalDivider(color = colors.separator.copy(alpha = 0.35f), thickness = 0.5.dp, modifier = Modifier.padding(start = 16.dp))
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable(enabled = isPaired && status.available && !isTranslatingBatch) {
                                    haptic.light()
                                    isTranslatingBatch = true
                                    batchFeedback = null
                                    batchFeedbackIsError = false
                                    scope.launch {
                                        val res = ApiClient.defaultInstance?.translateExistingTitles(
                                            targetLang = devLang.bcp47Tag,
                                            targetLangName = devLang.englishDisplayName
                                        )
                                        isTranslatingBatch = false
                                        if (res != null && res.isSuccess) {
                                            val r = res.getOrThrow()
                                            com.antigravity.mobile.data.service.AIAssistCache.updateTranslatedCount(r.translatedCount)
                                            batchFeedbackIsError = r.translated == 0 && r.failed > 0
                                            batchFeedback = when {
                                                r.alreadyRunning -> when (devLang.kind) {
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "后台正在翻译中，请稍候"
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "翻訳処理が実行中です"
                                                    else -> "Translation is already running in the background"
                                                }
                                                r.translated == 0 && r.failed == 0 -> when (devLang.kind) {
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "所有活跃会话标题已是目标语言（跳过 ${r.skipped} 条）"
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "翻訳対象の英語タイトルはありませんでした（${r.skipped} 件スキップ）"
                                                    else -> "All active titles are already up to date (${r.skipped} skipped)"
                                                }
                                                else -> when (devLang.kind) {
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE ->
                                                        if (r.failed > 0) "已翻译 ${r.translated} 条，跳过 ${r.skipped} 条，失败 ${r.failed} 条"
                                                        else "已翻译 ${r.translated} 条标题，跳过 ${r.skipped} 条"
                                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE ->
                                                        if (r.failed > 0) "${r.translated} 件翻訳、${r.skipped} 件スキップ、${r.failed} 件失敗"
                                                        else "${r.translated} 件のタイトルを翻訳しました（${r.skipped} 件スキップ）"
                                                    else ->
                                                        if (r.failed > 0) "Translated ${r.translated}, skipped ${r.skipped}, failed ${r.failed}"
                                                        else "Translated ${r.translated} titles (${r.skipped} skipped)"
                                                }
                                            }
                                        } else {
                                            batchFeedbackIsError = true
                                            batchFeedback = when (L10n.currentLang) {
                                                L10n.AppLang.EN -> "Translation failed. Check gateway connection."
                                                L10n.AppLang.JA -> "翻訳に失敗しました。接続を確認してください。"
                                                L10n.AppLang.ZH -> "翻译失败，请确认网关已连接 Antigravity"
                                            }
                                        }
                                    }
                                }
                                .padding(horizontal = 16.dp, vertical = 14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = batchButtonLabel,
                                color = if (isPaired && status.available && !isTranslatingBatch) colors.accentIndigo else colors.textMuted,
                                fontSize = 14.5.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.weight(1f)
                            )
                            if (isTranslatingBatch) {
                                CircularProgressIndicator(
                                    modifier = Modifier.size(16.dp),
                                    strokeWidth = 2.dp,
                                    color = colors.accentIndigo
                                )
                            }
                        }
                    }

                    if (status.translatedCount > 0) {
                        HorizontalDivider(color = colors.separator.copy(alpha = 0.35f), thickness = 0.5.dp, modifier = Modifier.padding(start = 16.dp))
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable(enabled = isPaired && !isRestoringTitles && !isTranslatingBatch) {
                                    haptic.light()
                                    showRestoreConfirm = true
                                }
                                .padding(horizontal = 16.dp, vertical = 14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = restoreButtonLabel,
                                color = colors.accentRed,
                                fontSize = 14.5.sp,
                                fontWeight = FontWeight.Medium,
                                modifier = Modifier.weight(1f)
                            )
                            if (isRestoringTitles) {
                                CircularProgressIndicator(
                                    modifier = Modifier.size(16.dp),
                                    strokeWidth = 2.dp,
                                    color = colors.accentRed
                                )
                            }
                        }
                    }

                    if (!batchFeedback.isNullOrBlank()) {
                        HorizontalDivider(color = colors.separator.copy(alpha = 0.35f), thickness = 0.5.dp, modifier = Modifier.padding(start = 16.dp))
                        Text(
                            text = batchFeedback.orEmpty(),
                            color = if (batchFeedbackIsError) colors.accentRed else colors.textSecondary,
                            fontSize = 12.sp,
                            lineHeight = 16.sp,
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp)
                        )
                    }
                }
            }
        }

        if (showLiveSection) {
            SettingsSection(
                title = liveSectionHeader,
                footer = liveSectionFooter
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                ) {
                    CompactSubpageToggleRow(
                        title = stepPlanToggleLabel,
                        subtitle = stepPlanToggleSubtitle,
                        checked = prefs.stepPlanTranslateEnabled,
                        enabled = isPaired && status.available,
                        onCheckedChange = { enabled ->
                            haptic.light()
                            val next = if (enabled && !devLang.isEnglish) {
                                prefs.copy(
                                    stepPlanTranslateEnabled = true,
                                    titleTranslateLang = devLang.bcp47Tag,
                                    titleTranslateLangName = devLang.englishDisplayName
                                )
                            } else {
                                prefs.copy(stepPlanTranslateEnabled = enabled)
                            }
                            val patch = buildJsonObject {
                                put("stepPlanTranslateEnabled", enabled)
                                if (enabled && !devLang.isEnglish) {
                                    put("titleTranslateLang", devLang.bcp47Tag)
                                    put("titleTranslateLangName", devLang.englishDisplayName)
                                }
                            }
                            applyPatch(next, patch)
                        }
                    )

                    HorizontalDivider(color = colors.separator.copy(alpha = 0.35f), thickness = 0.5.dp, modifier = Modifier.padding(start = 16.dp))

                    CompactSubpageToggleRow(
                        title = focusToggleLabel,
                        subtitle = focusToggleSubtitle,
                        checked = prefs.focusTranslateEnabled,
                        enabled = isPaired && status.available,
                        onCheckedChange = { enabled ->
                            haptic.light()
                            val next = if (enabled && !devLang.isEnglish) {
                                prefs.copy(
                                    focusTranslateEnabled = true,
                                    titleTranslateLang = devLang.bcp47Tag,
                                    titleTranslateLangName = devLang.englishDisplayName
                                )
                            } else {
                                prefs.copy(focusTranslateEnabled = enabled)
                            }
                            val patch = buildJsonObject {
                                put("focusTranslateEnabled", enabled)
                                if (enabled && !devLang.isEnglish) {
                                    put("titleTranslateLang", devLang.bcp47Tag)
                                    put("titleTranslateLangName", devLang.englishDisplayName)
                                }
                            }
                            applyPatch(next, patch)
                        }
                    )
                }
            }
        }

        // Next-Prompt Suggestions section (visible for all languages including English)
        SettingsSection(
            title = when (L10n.currentLang) {
                L10n.AppLang.EN -> "Prompt Suggestions"
                L10n.AppLang.JA -> "プロンプト提案"
                L10n.AppLang.ZH -> "提示词建议"
            },
            footer = when (L10n.currentLang) {
                L10n.AppLang.EN -> "Automatically hidden when approvals, Proceed/Continue buttons, or draft text are present."
                L10n.AppLang.JA -> "承認カード・続行ボタン表示中や下書き入力中は自動で非表示になります。"
                L10n.AppLang.ZH -> "存在审批卡片、Proceed/Continue 按钮或输入框已有草稿时自动隐藏，避免打扰。"
            }
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
            ) {
                CompactSubpageToggleRow(
                    title = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Next-Prompt Suggestions"
                        L10n.AppLang.JA -> "次のプロンプト提案"
                        L10n.AppLang.ZH -> "下一步提示词建议"
                    },
                    subtitle = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Predict 1 concise follow-up suggestion after each turn"
                        L10n.AppLang.JA -> "各ターンの完了後に次の指示を1件軽量予測します"
                        L10n.AppLang.ZH -> "每轮回复完成后轻量预测 1 条建议胶囊"
                    },
                    checked = prefs.nextPromptEnabled,
                    enabled = isPaired && status.available,
                    onCheckedChange = { enabled ->
                        haptic.light()
                        applyPatch(
                            prefs.copy(nextPromptEnabled = enabled),
                            buildJsonObject { put("nextPromptEnabled", enabled) }
                        )
                    }
                )

                if (prefs.nextPromptEnabled) {
                    HorizontalDivider(color = colors.separator.copy(alpha = 0.35f), thickness = 0.5.dp, modifier = Modifier.padding(start = 16.dp))
                    val fillLabel = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Fill Input Box"
                        L10n.AppLang.JA -> "入力欄に挿入"
                        L10n.AppLang.ZH -> "填入输入框"
                    }
                    val sendLabel = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Send Immediately"
                        L10n.AppLang.JA -> "そのまま送信"
                        L10n.AppLang.ZH -> "直接发送"
                    }
                    SettingsMenuPickerRow(
                        label = when (L10n.currentLang) {
                            L10n.AppLang.EN -> "On Suggestion Tap"
                            L10n.AppLang.JA -> "提案タップ時の動作"
                            L10n.AppLang.ZH -> "点击建议胶囊"
                        },
                        selectedLabel = if (prefs.nextPromptTapAction == "send") sendLabel else fillLabel,
                        selectedId = prefs.nextPromptTapAction,
                        options = listOf(
                            PickerMenuOption("fill", fillLabel),
                            PickerMenuOption("send", sendLabel)
                        ),
                        onSelect = { chosen ->
                            applyPatch(
                                prefs.copy(nextPromptTapAction = chosen),
                                buildJsonObject { put("nextPromptTapAction", chosen) }
                            )
                        },
                        enabled = isPaired
                    )
                }
            }
        }

        // Inference Engine Info section
        SettingsSection(
            title = when (L10n.currentLang) {
                L10n.AppLang.EN -> "Inference Engine"
                L10n.AppLang.JA -> "推論エンジン"
                L10n.AppLang.ZH -> "推理引擎"
            },
            footer = syncError ?: when {
                !status.available -> when (L10n.currentLang) {
                    L10n.AppLang.EN -> "This Antigravity version does not support native GetModelResponse."
                    L10n.AppLang.JA -> "現在の Antigravity バージョンでは GetModelResponse に対応していません。"
                    L10n.AppLang.ZH -> "当前 Antigravity 版本不支持原生 GetModelResponse 接口。"
                }
                else -> when (L10n.currentLang) {
                    L10n.AppLang.EN -> "Runs statelessly via local Antigravity native RPC without creating extra sessions. Syncs across devices."
                    L10n.AppLang.JA -> "ローカルの Antigravity ネイティブ API でステートレス実行され、追加会話は作成されません。設定は全端末に同期されます。"
                    L10n.AppLang.ZH -> "通过本机 Antigravity 原生接口无状态执行，不产生额外会话，微量共享当前账号 Gemini 额度，开关跨设备同步至网关。"
                }
            }
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(
                    text = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Lightweight Model"
                        L10n.AppLang.JA -> "軽量モデル"
                        L10n.AppLang.ZH -> "轻量模型"
                    },
                    color = colors.textPrimary,
                    fontSize = 14.5.sp,
                    fontWeight = FontWeight.Medium
                )
                Text(
                    text = status.modelName.ifBlank { "Gemini 3.5 Flash Lite" },
                    color = colors.textSecondary,
                    fontSize = 13.5.sp
                )
            }
        }

        Spacer(modifier = Modifier.height(16.dp))
    }

    if (showRestoreConfirm) {
        AlertDialog(
            onDismissRequest = { showRestoreConfirm = false },
            title = {
                Text(
                    text = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Restore Original Titles"
                        L10n.AppLang.JA -> "元のタイトルを復元"
                        L10n.AppLang.ZH -> "还原原始标题"
                    },
                    color = colors.textPrimary,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Text(
                    text = when (L10n.currentLang) {
                        L10n.AppLang.EN -> "Restore titles previously translated by AI Assist back to their original text (manually renamed titles are preserved)."
                        L10n.AppLang.JA -> "自動・一括翻訳された会話タイトルを元の英語タイトルに戻します（手動変更したタイトルは維持されます）。"
                        L10n.AppLang.ZH -> "将把此前自动或批量翻译的会话标题恢复为原始标题（手动重命名的会话不受影响）。"
                    },
                    color = colors.textSecondary,
                    fontSize = 14.sp
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        showRestoreConfirm = false
                        isRestoringTitles = true
                        batchFeedback = null
                        batchFeedbackIsError = false
                        scope.launch {
                            val res = ApiClient.defaultInstance?.restoreOriginalTitles()
                            isRestoringTitles = false
                            if (res != null && res.isSuccess) {
                                val r = res.getOrThrow()
                                com.antigravity.mobile.data.service.AIAssistCache.updateTranslatedCount(r.translatedCount)
                                batchFeedback = when (devLang.kind) {
                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE,
                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE -> "已还原 ${r.restored} 条原始标题"
                                    com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE -> "${r.restored} 件の元のタイトルを復元しました"
                                    else -> "Restored ${r.restored} original titles"
                                }
                            } else {
                                batchFeedbackIsError = true
                                batchFeedback = when (L10n.currentLang) {
                                    L10n.AppLang.EN -> "Restore failed. Check gateway connection."
                                    L10n.AppLang.JA -> "復元に失敗しました。"
                                    L10n.AppLang.ZH -> "还原失败，请检查网关连接"
                                }
                            }
                        }
                    }
                ) {
                    Text(
                        text = when (L10n.currentLang) {
                            L10n.AppLang.EN -> "Restore"
                            L10n.AppLang.JA -> "復元"
                            L10n.AppLang.ZH -> "还原"
                        },
                        color = colors.accentRed,
                        fontWeight = FontWeight.SemiBold
                    )
                }
            },
            dismissButton = {
                TextButton(onClick = { showRestoreConfirm = false }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            }
        )
    }
}
