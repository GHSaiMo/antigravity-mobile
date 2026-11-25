package com.antigravity.mobile.ui.screen

import com.antigravity.mobile.ui.util.loc
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowCircleDown
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.Keyboard
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import com.antigravity.mobile.R
import com.antigravity.mobile.data.model.PairingInfo
import com.antigravity.mobile.ui.components.EasterEggDialog
import com.antigravity.mobile.ui.components.OnboardingGuideView
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import com.antigravity.mobile.ui.viewmodel.PairingUiState
import com.antigravity.mobile.ui.viewmodel.PairingViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PairingScreen(
    viewModel: PairingViewModel,
    onLaunchScanner: () -> Unit,
    onPairedSuccess: () -> Unit,
    openManualInputSignal: Int = 0,
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()

    var showManualInput by remember { mutableStateOf(false) }

    var easterEggTapCount by remember { mutableIntStateOf(0) }
    var lastEasterEggTapTime by remember { mutableLongStateOf(0L) }
    var showEasterEgg by remember { mutableStateOf(false) }

    val handleEasterEggTap: () -> Unit = {
        if (!showEasterEgg) {
            val now = System.currentTimeMillis()
            if (now - lastEasterEggTapTime > 2000L) {
                easterEggTapCount = 1
            } else {
                easterEggTapCount++
            }
            lastEasterEggTapTime = now
            haptic.light()

            if (easterEggTapCount >= 10) {
                easterEggTapCount = 0
                haptic.success()
                showEasterEgg = true
            }
        }
    }

    LaunchedEffect(uiState) {
        if (uiState is PairingUiState.Success) {
            onPairedSuccess()
        }
    }

    LaunchedEffect(openManualInputSignal) {
        if (openManualInputSignal > 0) {
            showManualInput = true
        }
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(colors.background)
    ) {
        OnboardingGuideView(
            onScanTapped = onLaunchScanner,
            onManualInputTapped = { showManualInput = true },
            onEasterEggTap = handleEasterEggTap,
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .navigationBarsPadding()
        )

        if (showEasterEgg) {
            EasterEggDialog(
                onDismiss = { showEasterEgg = false }
            )
        }

        // Manual Input Bottom Sheet
        if (showManualInput) {
            ManualPairingBottomSheet(
                isPairing = uiState is PairingUiState.Pairing,
                onDismiss = { showManualInput = false },
                onPairInfo = { info ->
                    showManualInput = false
                    viewModel.pairWithInfo(info)
                },
                onPairHostAndCode = { h, p, c, s ->
                    showManualInput = false
                    viewModel.pairWithHostAndCode(h, p, c, s)
                }
            )
        }

        // Pairing Loading Overlay (matching iOS ultraThinMaterial progress overlay)
        if (uiState is PairingUiState.Pairing) {
            Dialog(onDismissRequest = {}) {
                Box(
                    modifier = Modifier
                        .size(160.dp)
                        .clip(RoundedCornerShape(16.dp))
                        .background(Color.Black.copy(alpha = 0.8f)),
                    contentAlignment = Alignment.Center
                ) {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        CircularProgressIndicator(
                            color = Color.White,
                            strokeWidth = 3.dp,
                            modifier = Modifier.size(36.dp)
                        )
                        Text(
                            text = "正在与电脑网关配对...".loc,
                            fontSize = 13.sp,
                            fontWeight = FontWeight.Medium,
                            color = Color.White,
                            textAlign = TextAlign.Center,
                            modifier = Modifier.padding(horizontal = 12.dp)
                        )
                    }
                }
            }
        }

        // Pairing Error Dialog (matching iOS alert)
        if (uiState is PairingUiState.Error) {
            val errMsg = (uiState as PairingUiState.Error).message
            AlertDialog(
                onDismissRequest = { viewModel.resetState() },
                title = { Text("配对失败".loc, fontWeight = FontWeight.Bold, color = colors.textPrimary) },
                text = { Text(errMsg, color = colors.textSecondary, fontSize = 14.sp) },
                confirmButton = {
                    TextButton(onClick = { viewModel.resetState() }) {
                        Text("确定".loc, color = colors.accentIndigo, fontWeight = FontWeight.SemiBold)
                    }
                },
                containerColor = colors.surface,
                shape = RoundedCornerShape(14.dp)
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ManualPairingBottomSheet(
    onDismiss: () -> Unit,
    onPairInfo: (PairingInfo) -> Unit,
    onPairHostAndCode: (host: String, port: Int, code: String, ssl: Boolean) -> Unit,
    isPairing: Boolean = false
) {
    val colors = AntigravityTheme.colors
    val clipboardManager = LocalClipboardManager.current
    val haptic = rememberHaptic()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    var host by remember { mutableStateOf("") }
    var port by remember { mutableStateOf("58900") }
    var code by remember { mutableStateOf("") }
    var ssl by remember { mutableStateOf(false) }
    var pasteError by remember { mutableStateOf<String?>(null) }

    // Preserve full parsed PairingInfo (including E2EE `pk` and LAN/relay endpoints) when populated from a URI.
    var parsedInfo by remember { mutableStateOf<PairingInfo?>(null) }
    var parsedHostSnapshot by remember { mutableStateOf("") }
    var parsedPortSnapshot by remember { mutableStateOf("") }
    var parsedCodeSnapshot by remember { mutableStateOf("") }
    var parsedSslSnapshot by remember { mutableStateOf(false) }

    fun tryPopulateFromUri(raw: String): Boolean {
        if (PairingInfo.extractPairingUri(raw) == null) return false
        val info = PairingInfo.parseFromUri(raw) ?: return false
        haptic.light()
        val portStr = info.port.toString()
        parsedInfo = info
        parsedHostSnapshot = info.host
        parsedPortSnapshot = portStr
        parsedCodeSnapshot = info.code
        parsedSslSnapshot = info.ssl
        host = info.host
        port = portStr
        code = info.code
        ssl = info.ssl
        pasteError = null
        return true
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = colors.surface,
        dragHandle = null,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) }
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(bottom = 24.dp)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 10.dp, bottom = 16.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            Text(
                text = "手动输入配对信息".loc,
                fontSize = 18.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp)
            )

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 20.dp, vertical = 16.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                // Always-available quick paste row (reads clipboard on demand when tapped)
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.accentIndigo.copy(alpha = 0.08f))
                            .border(
                                width = 1.dp,
                                color = colors.accentIndigo.copy(alpha = 0.28f),
                                shape = RoundedCornerShape(12.dp)
                            )
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.ContentPaste,
                            contentDescription = null,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(18.dp)
                        )
                        Column(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(2.dp)
                        ) {
                            Text(
                                text = "粘贴配对链接".loc,
                                fontSize = 13.5.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary
                            )
                            Text(
                                text = "一键导入终端输出的 agy://pair 链接".loc,
                                fontSize = 11.5.sp,
                                color = colors.textSecondary,
                                lineHeight = 15.sp
                            )
                        }
                        Button(
                            onClick = {
                                val raw = clipboardManager.getText()?.text.orEmpty()
                                val parsed = PairingInfo.parseFromUri(raw)
                                if (parsed != null) {
                                    haptic.medium()
                                    pasteError = null
                                    onPairInfo(parsed)
                                } else {
                                    haptic.light()
                                    pasteError = "剪贴板中未找到有效的配对链接（需包含 agy://pair）".loc
                                }
                            },
                            enabled = !isPairing,
                            shape = RoundedCornerShape(999.dp),
                            contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = colors.accentIndigo,
                                contentColor = Color.White
                            ),
                            modifier = Modifier.height(34.dp)
                        ) {
                            Text(
                                text = "粘贴".loc,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }
                    }

                    pasteError?.let { err ->
                        Text(
                            text = err,
                            fontSize = 12.sp,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.padding(horizontal = 4.dp)
                        )
                    }
                }

                OutlinedTextField(
                    value = host,
                    onValueChange = { newVal ->
                        if (!tryPopulateFromUri(newVal)) {
                            host = newVal
                        }
                    },
                    label = { Text("网关 IP 或域名 (Host)".loc, color = colors.textSecondary) },
                    placeholder = { Text("例如 192.168.1.100 或 [2408:...]".loc, color = colors.textMuted) },
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = colors.surfaceVariant,
                        unfocusedContainerColor = colors.surfaceVariant,
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border,
                        focusedTextColor = colors.textPrimary,
                        unfocusedTextColor = colors.textPrimary
                    ),
                    modifier = Modifier.fillMaxWidth()
                )

                OutlinedTextField(
                    value = port,
                    onValueChange = { port = it },
                    label = { Text("端口号 (Port)".loc, color = colors.textSecondary) },
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = colors.surfaceVariant,
                        unfocusedContainerColor = colors.surfaceVariant,
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border,
                        focusedTextColor = colors.textPrimary,
                        unfocusedTextColor = colors.textPrimary
                    ),
                    modifier = Modifier.fillMaxWidth()
                )

                OutlinedTextField(
                    value = code,
                    onValueChange = { newVal ->
                        if (!tryPopulateFromUri(newVal)) {
                            code = newVal
                        }
                    },
                    label = { Text("配对码 (Code)".loc, color = colors.textSecondary) },
                    placeholder = { Text("终端输出的配对码 (5分钟有效)".loc, color = colors.textMuted) },
                    singleLine = true,
                    shape = RoundedCornerShape(12.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = colors.surfaceVariant,
                        unfocusedContainerColor = colors.surfaceVariant,
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border,
                        focusedTextColor = colors.textPrimary,
                        unfocusedTextColor = colors.textPrimary
                    ),
                    modifier = Modifier.fillMaxWidth()
                )

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        "开启 HTTPS / SSL".loc,
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        modifier = Modifier.weight(1f).padding(end = 8.dp)
                    )
                    Switch(
                        checked = ssl,
                        onCheckedChange = { ssl = it }
                    )
                }

                Button(
                    onClick = {
                        val trimmedHost = host.trim()
                        val trimmedCode = code.trim()
                        val parsedPort = port.trim().toIntOrNull() ?: 58900
                        val fullInfo = parsedInfo
                        if (fullInfo != null &&
                            trimmedHost == parsedHostSnapshot &&
                            parsedPort.toString() == parsedPortSnapshot &&
                            trimmedCode == parsedCodeSnapshot &&
                            ssl == parsedSslSnapshot
                        ) {
                            onPairInfo(fullInfo)
                        } else {
                            onPairHostAndCode(trimmedHost, parsedPort, trimmedCode, ssl)
                        }
                    },
                    enabled = host.isNotBlank() && code.isNotBlank() && !isPairing,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(48.dp),
                    shape = RoundedCornerShape(12.dp),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.accentIndigo,
                        contentColor = Color.White
                    )
                ) {
                    Text("连接并配对".loc, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}
