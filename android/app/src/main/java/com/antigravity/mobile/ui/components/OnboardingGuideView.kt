package com.antigravity.mobile.ui.components

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.R
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 1:1 Kotlin port of iOS OnboardingGuideView.swift.
 * First-launch onboarding view guiding user to download Mac gateway, run mgy, and scan QR code.
 */
@Composable
fun OnboardingGuideView(
    onScanTapped: () -> Unit,
    onManualInputTapped: () -> Unit,
    onEasterEggTap: (() -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val clipboardManager = LocalClipboardManager.current
    val coroutineScope = rememberCoroutineScope()

    val installCommand = "curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash"
    val runCommand = "mgy"

    var isInstallCopied by remember { mutableStateOf(false) }

    val copyInstallCommand: () -> Unit = {
        clipboardManager.setText(AnnotatedString(installCommand))
        haptic.success()
        isInstallCopied = true
        coroutineScope.launch {
            delay(2000L)
            isInstallCopied = false
        }
    }

    BoxWithConstraints(
        modifier = modifier.fillMaxSize()
    ) {
        val minHeight = maxHeight
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .heightIn(min = minHeight)
                .padding(horizontal = 20.dp, vertical = 14.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically)
        ) {
            // App Logo & Welcome Header
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
            Image(
                painter = painterResource(id = R.drawable.app_logo),
                contentDescription = "Multigravity Logo",
                modifier = Modifier
                    .size(60.dp)
                    .clip(RoundedCornerShape(14.dp))
                    .border(1.dp, colors.textPrimary.copy(alpha = 0.08f), RoundedCornerShape(14.dp))
                    .shadow(8.dp, RoundedCornerShape(14.dp), ambientColor = Color.Black.copy(alpha = 0.12f))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        enabled = onEasterEggTap != null,
                        onClick = { onEasterEggTap?.invoke() }
                    )
            )

            Text(
                text = "欢迎使用 Multigravity",
                fontSize = 21.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary,
                textAlign = TextAlign.Center
            )

            Text(
                text = "Multigravity 智能体全栈移动伴侣\n随时随地监控思考流、下发指令与决策",
                fontSize = 12.5.sp,
                color = colors.textSecondary,
                textAlign = TextAlign.Center,
                lineHeight = 17.sp,
                modifier = Modifier.padding(horizontal = 12.dp)
            )
        }

        // Step 1: Download & Run Gateway
        Card(
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(14.dp),
            colors = CardDefaults.cardColors(containerColor = colors.surface),
            border = androidx.compose.foundation.BorderStroke(0.6.dp, colors.border.copy(alpha = 0.5f))
        ) {
            Column(
                modifier = Modifier.padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(24.dp)
                            .clip(CircleShape)
                            .background(colors.accentBlue.copy(alpha = 0.15f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "1",
                            fontSize = 13.sp,
                            fontWeight = FontWeight.Bold,
                            color = colors.accentBlue
                        )
                    }
                    Text(
                        text = "在 Mac 上启动网关服务",
                        fontSize = 15.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textPrimary
                    )
                }

                Text(
                    text = "手机端需配合运行在 Mac 上的本地网关协同工作，嗅探后台实例打通直连。",
                    fontSize = 12.sp,
                    color = colors.textSecondary,
                    lineHeight = 17.sp
                )

                // Mac Terminal one-click installation and run script block
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .background(colors.surfaceVariant.copy(alpha = 0.45f))
                        .border(0.6.dp, colors.border.copy(alpha = 0.6f), RoundedCornerShape(10.dp))
                        .padding(10.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(5.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Terminal,
                                contentDescription = null,
                                tint = colors.textSecondary,
                                modifier = Modifier.size(13.dp)
                            )
                            Text(
                                text = "Mac 终端一键安装脚本",
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.textSecondary
                            )
                        }

                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(6.dp))
                                .background(
                                    if (isInstallCopied) colors.accentGreen.copy(alpha = 0.15f)
                                    else colors.accentBlue.copy(alpha = 0.12f)
                                )
                                .clickable(onClick = copyInstallCommand)
                                .padding(horizontal = 8.dp, vertical = 4.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Icon(
                                    imageVector = if (isInstallCopied) Icons.Default.Check else Icons.Default.ContentCopy,
                                    contentDescription = null,
                                    tint = if (isInstallCopied) colors.accentGreen else colors.accentBlue,
                                    modifier = Modifier.size(11.dp)
                                )
                                Text(
                                    text = if (isInstallCopied) "已复制" else "一键复制",
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = if (isInstallCopied) colors.accentGreen else colors.accentBlue
                                )
                            }
                        }
                    }

                    // Horizontal scrollable single-line code block
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(6.dp))
                            .background(colors.surface)
                            .border(0.6.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(6.dp))
                            .clickable(onClick = copyInstallCommand)
                            .padding(horizontal = 8.dp, vertical = 7.dp)
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .horizontalScroll(rememberScrollState()),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                text = installCommand,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary,
                                maxLines = 1,
                                softWrap = false
                            )
                        }
                    }

                    // Clean mgy prompt without redundant copy button
                    Row(
                        modifier = Modifier.padding(horizontal = 2.dp, vertical = 1.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Text(
                            text = "若已安装网关，直接在终端执行",
                            fontSize = 11.5.sp,
                            color = colors.textMuted
                        )
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .background(colors.surface)
                                .border(0.5.dp, colors.border.copy(alpha = 0.6f), RoundedCornerShape(4.dp))
                                .padding(horizontal = 6.dp, vertical = 1.5.dp)
                        ) {
                            Text(
                                text = runCommand,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace,
                                fontWeight = FontWeight.Bold,
                                color = colors.accentIndigo
                            )
                        }
                    }
                }

                // GitHub guide button
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(8.dp))
                        .background(colors.accentBlue.copy(alpha = 0.08f))
                        .clickable {
                            haptic.light()
                            val intent = Intent(
                                Intent.ACTION_VIEW,
                                Uri.parse("https://github.com/GHSaiMo/antigravity-mobile#readme")
                            )
                            context.startActivity(intent)
                        }
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Icon(
                        imageVector = Icons.Default.ArrowCircleDown,
                        contentDescription = null,
                        tint = colors.accentBlue,
                        modifier = Modifier.size(15.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "查看 GitHub 部署指南与下载",
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.accentBlue
                    )
                    Spacer(modifier = Modifier.weight(1f))
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowForwardIos,
                        contentDescription = null,
                        tint = colors.accentBlue.copy(alpha = 0.6f),
                        modifier = Modifier.size(10.dp)
                    )
                }
            }
        }

        // Step 2: Scan QR Code & Pair
        Card(
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(14.dp),
            colors = CardDefaults.cardColors(containerColor = colors.surface),
            border = androidx.compose.foundation.BorderStroke(0.6.dp, colors.border.copy(alpha = 0.5f))
        ) {
            Column(
                modifier = Modifier.padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(24.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.15f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "2",
                            fontSize = 13.sp,
                            fontWeight = FontWeight.Bold,
                            color = colors.accentIndigo
                        )
                    }
                    Text(
                        text = "扫码一键自动配对",
                        fontSize = 15.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textPrimary
                    )
                }

                Text(
                    text = "电脑终端运行网关后会自动生成复合二维码，手机扫码即可直接交换凭证并绑定，零手动配置。",
                    fontSize = 12.sp,
                    color = colors.textSecondary,
                    lineHeight = 17.sp
                )

                Button(
                    onClick = {
                        haptic.medium()
                        onScanTapped()
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(44.dp)
                        .clip(RoundedCornerShape(10.dp))
                        .background(
                            Brush.horizontalGradient(
                                listOf(colors.accentIndigo, Color(0xFF9333EA))
                            )
                        ),
                    colors = ButtonDefaults.buttonColors(containerColor = Color.Transparent)
                ) {
                    Icon(
                        imageVector = Icons.Default.QrCodeScanner,
                        contentDescription = "Scan",
                        tint = Color.White,
                        modifier = Modifier.size(18.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text(
                        text = "扫描电脑端配对二维码",
                        fontSize = 14.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = Color.White
                    )
                }
            }
        }

        // Secondary Action: Manual Input
        Row(
            modifier = Modifier
                .clip(RoundedCornerShape(8.dp))
                .clickable {
                    haptic.light()
                    onManualInputTapped()
                }
                .padding(horizontal = 10.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Keyboard,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(15.dp)
            )
            Text(
                text = "高级选项：手动输入网址或配对码",
                color = colors.textSecondary,
                fontSize = 12.5.sp,
                fontWeight = FontWeight.Medium
            )
        }
    }
}
}
