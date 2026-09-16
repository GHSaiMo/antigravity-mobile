package com.antigravity.mobile.ui.screen

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AntigravityTheme
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
    val uiState by viewModel.uiState.collectAsState()
    val colors = AntigravityTheme.colors

    var host by remember { mutableStateOf("") }
    var port by remember { mutableStateOf("58900") }
    var code by remember { mutableStateOf("") }
    var ssl by remember { mutableStateOf(false) }

    LaunchedEffect(uiState) {
        if (uiState is PairingUiState.Success) {
            onPairedSuccess()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("连接到 Antigravity", fontWeight = FontWeight.Bold, color = colors.textPrimary) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = colors.background,
                    titleContentColor = colors.textPrimary
                )
            )
        },
        containerColor = colors.background,
        modifier = modifier
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = "在 Mac 终端运行 `make pair` 或 `make run`，扫描屏幕显示的二维码，秒级建立点对点安全连接。",
                color = colors.textSecondary,
                fontSize = 14.sp,
                lineHeight = 20.sp
            )

            // QR Scanner Button
            Button(
                onClick = onLaunchScanner,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(52.dp),
                shape = RoundedCornerShape(12.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = colors.accentIndigo,
                    contentColor = Color.White
                )
            ) {
                Icon(
                    imageVector = Icons.Default.QrCodeScanner,
                    contentDescription = "Scan QR",
                    modifier = Modifier.size(20.dp)
                )
                Spacer(modifier = Modifier.width(8.dp))
                Text("扫描终端配对二维码", fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                HorizontalDivider(modifier = Modifier.weight(1f), color = colors.border)
                Text(
                    text = " 或手动填入配对信息 ",
                    color = colors.textMuted,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(horizontal = 8.dp)
                )
                HorizontalDivider(modifier = Modifier.weight(1f), color = colors.border)
            }

            // Manual Form Card
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = colors.surface),
                border = CardDefaults.outlinedCardBorder().copy(brush = androidx.compose.ui.graphics.SolidColor(colors.border)),
                shape = RoundedCornerShape(14.dp)
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    OutlinedTextField(
                        value = host,
                        onValueChange = { host = it },
                        label = { Text("网关 IP 或域名 (Host)", color = colors.textSecondary) },
                        placeholder = { Text("例如 192.168.1.100 或 [2408:...]", color = colors.textMuted) },
                        singleLine = true,
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedContainerColor = colors.surface,
                            unfocusedContainerColor = colors.surface,
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
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedContainerColor = colors.surface,
                            unfocusedContainerColor = colors.surface,
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
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedContainerColor = colors.surface,
                            unfocusedContainerColor = colors.surface,
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
                            viewModel.pairWithHostAndCode(host, p, code, ssl)
                        },
                        enabled = host.isNotBlank() && code.isNotBlank() && uiState !is PairingUiState.Pairing,
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(10.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = colors.accentGreen)
                    ) {
                        Text("连接并配对", fontWeight = FontWeight.SemiBold)
                    }
                }
            }

            // Status indication
            when (val state = uiState) {
                is PairingUiState.Pairing -> {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.padding(top = 8.dp)
                    ) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                        Text(state.message, color = colors.textSecondary, fontSize = 13.sp)
                    }
                }
                is PairingUiState.Error -> {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.accentRed.copy(alpha = 0.15f))
                            .border(0.5.dp, colors.accentRed.copy(alpha = 0.3f), RoundedCornerShape(10.dp))
                        .padding(12.dp)
                    ) {
                        Text(state.message, color = colors.accentRed, fontSize = 13.sp)
                    }
                }
                else -> Unit
            }
        }
    }
}
