package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
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
    val shape = RoundedCornerShape(14.dp)
    Column(
        modifier = modifier
            .clip(shape)
            .background(colors.textMuted.copy(alpha = 0.10f))
            .border(0.75.dp, Color(0xFFF59E0B).copy(alpha = 0.4f), shape)
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Text(
            text = "当前无法连接语音",
            color = colors.textPrimary,
            fontSize = 14.sp,
            fontWeight = FontWeight.SemiBold
        )
        Text(
            text = "请先在电脑上依次检查：",
            color = colors.textMuted,
            fontSize = 12.sp
        )
        listOf(
            "1. 重新登录：退出 Antigravity 登录，完全关闭后重新打开并登录，授权页勾选全部权限",
            "2. 开启代理 TUN 模式：确保 *.googleapis.com 长连接（HTTP/2、gRPC）正常转发",
            "3. 检查电脑麦克风：系统麦克风权限已开启，默认录音设备正常且未被占用"
        ).forEach {
            Text(text = it, color = colors.textPrimary, fontSize = 12.sp)
        }
        Text(
            text = "如果都不行，是否让智能体自动排查？",
            color = colors.textPrimary,
            fontSize = 12.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(top = 2.dp)
        )
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp, Alignment.End),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "我再试试",
                color = colors.textPrimary,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier
                    .clip(RoundedCornerShape(50))
                    .background(colors.textMuted.copy(alpha = 0.15f))
                    .clickable(onClick = onRetry)
                    .padding(horizontal = 14.dp, vertical = 6.dp)
            )
            Text(
                text = "一键排查",
                color = Color.White,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier
                    .clip(RoundedCornerShape(50))
                    .background(colors.accentIndigo)
                    .clickable(onClick = onTroubleshoot)
                    .padding(horizontal = 14.dp, vertical = 6.dp)
            )
        }
    }
}
