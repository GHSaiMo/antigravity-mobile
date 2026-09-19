package com.antigravity.mobile.ui.screen

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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import com.antigravity.mobile.R
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
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val clipboardManager = LocalClipboardManager.current
    val haptic = rememberHaptic()

    var showManualInput by remember { mutableStateOf(false) }
    var host by remember { mutableStateOf("") }
    var port by remember { mutableStateOf("58900") }
    var code by remember { mutableStateOf("") }
    var ssl by remember { mutableStateOf(false) }

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
            val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
            ModalBottomSheet(
                onDismissRequest = { showManualInput = false },
                sheetState = sheetState,
                containerColor = colors.surface,
                dragHandle = null,
                shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp)
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .navigationBarsPadding()
                        .imePadding()
                        .verticalScroll(rememberScrollState())
                        .padding(bottom = 24.dp)
                ) {
                    // Grab handle
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
                        text = "手动输入配对信息",
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
                        // Quick clipboard paste if available
                        val clipboardText = clipboardManager.getText()?.text
                        if (!clipboardText.isNullOrBlank() && clipboardText.startsWith("agy://pair", ignoreCase = true)) {
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(10.dp))
                                    .background(colors.accentIndigo.copy(alpha = 0.1f))
                                    .clickable {
                                        showManualInput = false
                                        viewModel.pairWithUri(clipboardText)
                                    }
                                    .padding(horizontal = 14.dp, vertical = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.ContentPaste,
                                    contentDescription = null,
                                    tint = colors.accentIndigo,
                                    modifier = Modifier.size(16.dp)
                                )
                                Text(
                                    text = "检测到配对链接，点击立即配对",
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.Medium,
                                    color = colors.accentIndigo
                                )
                            }
                        }

                        OutlinedTextField(
                            value = host,
                            onValueChange = { host = it },
                            label = { Text("网关 IP 或域名 (Host)", color = colors.textSecondary) },
                            placeholder = { Text("例如 192.168.1.100 或 [2408:...]", color = colors.textMuted) },
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
                            label = { Text("端口号 (Port)", color = colors.textSecondary) },
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
                            onValueChange = { code = it },
                            label = { Text("配对码 (Code)", color = colors.textSecondary) },
                            placeholder = { Text("6位一次性配对码", color = colors.textMuted) },
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
                            Text("开启 HTTPS / SSL", color = colors.textPrimary, fontSize = 14.sp)
                            Switch(
                                checked = ssl,
                                onCheckedChange = { ssl = it }
                            )
                        }

                        Button(
                            onClick = {
                                val p = port.toIntOrNull() ?: 58900
                                showManualInput = false
                                viewModel.pairWithHostAndCode(host, p, code, ssl)
                            },
                            enabled = host.isNotBlank() && code.isNotBlank() && uiState !is PairingUiState.Pairing,
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(48.dp),
                            shape = RoundedCornerShape(12.dp),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = colors.accentIndigo,
                                contentColor = Color.White
                            )
                        ) {
                            Text("连接并配对", fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                        }
                    }
                }
            }
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
                            text = "正在与 Mac 网关配对...",
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
                title = { Text("配对失败", fontWeight = FontWeight.Bold, color = colors.textPrimary) },
                text = { Text(errMsg, color = colors.textSecondary, fontSize = 14.sp) },
                confirmButton = {
                    TextButton(onClick = { viewModel.resetState() }) {
                        Text("确定", color = colors.accentIndigo, fontWeight = FontWeight.SemiBold)
                    }
                },
                containerColor = colors.surface,
                shape = RoundedCornerShape(14.dp)
            )
        }
    }
}
