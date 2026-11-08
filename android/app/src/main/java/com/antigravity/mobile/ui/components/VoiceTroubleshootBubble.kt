package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AntigravityTheme

/** 语音连接失败后的提示气泡：给出三条自查建议，并允许一键让智能体排查。 */
@Composable
fun VoiceTroubleshootBubble(
    onRetry: () -> Unit,
    onTroubleshoot: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val shape = RoundedCornerShape(18.dp)
    val amberWarning = Color(0xFFF59E0B)

    val checklistItems = listOf(
        "**重新登录**：退出 Antigravity 登录，完全关闭后重新打开并登录，授权页勾选全部权限",
        "**开启代理 TUN 模式**：确保 `*.googleapis.com` 长连接（HTTP/2、gRPC）正常转发",
        "**检查电脑麦克风**：系统麦克风权限已开启，默认录音设备正常且未被占用"
    )

    Column(
        modifier = modifier
            .shadow(
                elevation = 2.dp,
                shape = shape,
                ambientColor = Color.Black.copy(alpha = 0.05f),
                spotColor = Color.Black.copy(alpha = 0.10f)
            )
            .clip(shape)
            .background(colors.surface)
            .border(1.dp, amberWarning.copy(alpha = 0.45f), shape)
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Warning,
                contentDescription = null,
                tint = amberWarning,
                modifier = Modifier.size(20.dp)
            )
            Text(
                text = "当前无法连接语音",
                color = colors.textPrimary,
                fontSize = 16.5.sp,
                fontWeight = FontWeight.Bold
            )
        }

        Text(
            text = "请先在电脑上依次检查：",
            color = colors.textSecondary,
            fontSize = 15.sp,
            fontWeight = FontWeight.Medium
        )

        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            checklistItems.forEachIndexed { index, itemText ->
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.Top
                ) {
                    Text(
                        text = "${index + 1}.",
                        color = amberWarning,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace,
                        lineHeight = 22.sp,
                        modifier = Modifier.padding(top = 1.dp)
                    )
                    Box(modifier = Modifier.weight(1f)) {
                        RichTextRenderer(
                            text = itemText,
                            colors = colors,
                            baseFontSize = 15.sp,
                            baseFontWeight = FontWeight.Normal
                        )
                    }
                }
            }
        }

        Text(
            text = "如果都不行，是否让智能体自动排查？",
            color = colors.textPrimary,
            fontSize = 15.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(top = 2.dp)
        )

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 2.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "我再试试",
                color = colors.textPrimary,
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier
                    .clip(RoundedCornerShape(50))
                    .background(colors.textMuted.copy(alpha = 0.15f))
                    .clickable(onClick = onRetry)
                    .padding(horizontal = 16.dp, vertical = 8.dp)
            )
            Text(
                text = "一键排查",
                color = Color.White,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier
                    .clip(RoundedCornerShape(50))
                    .background(colors.accentIndigo)
                    .clickable(onClick = onTroubleshoot)
                    .padding(horizontal = 18.dp, vertical = 8.dp)
            )
        }
    }
}
