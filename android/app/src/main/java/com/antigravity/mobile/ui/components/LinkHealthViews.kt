package com.antigravity.mobile.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.service.LinkHealth
import com.antigravity.mobile.data.service.LinkHealthState
import com.antigravity.mobile.data.service.LinkSnapshot
import com.antigravity.mobile.ui.theme.AntigravityTheme
import kotlinx.coroutines.delay

@Composable
private fun rememberNowMs(): Long {
    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            delay(1_000)
            value = System.currentTimeMillis()
        }
    }
    return now
}

@Composable
private fun linkColor(state: LinkHealthState): Color {
    val colors = AntigravityTheme.colors
    return when (state) {
        LinkHealthState.CONNECTED -> colors.accentGreen
        LinkHealthState.ATTENTION -> colors.accentOrange
        LinkHealthState.DISCONNECTED -> colors.accentRed
        LinkHealthState.UNKNOWN -> colors.textMuted
    }
}

private fun linkDescription(state: LinkHealthState) = when (state) {
    LinkHealthState.UNKNOWN -> "正在连接电脑"
    LinkHealthState.CONNECTED -> "已连接电脑"
    LinkHealthState.ATTENTION -> "连接异常"
    LinkHealthState.DISCONNECTED -> "已断开"
}

/** Small heartbeat dot for the right edge of the home quota bar; pulses on every gateway heartbeat. */
@Composable
fun LinkHealthDot(snapshot: LinkSnapshot, modifier: Modifier = Modifier) {
    val now = rememberNowMs()
    val state = LinkHealth.evaluate(snapshot, now)
    val color = linkColor(state)
    val pulse = remember { Animatable(1f) }
    LaunchedEffect(snapshot.beatCount) {
        if (snapshot.beatCount > 0) {
            pulse.snapTo(0f)
            pulse.animateTo(1f, tween(800))
        }
    }
    val pulsing = state == LinkHealthState.CONNECTED
    Box(
        modifier = modifier
            .size(14.dp)
            .semantics { contentDescription = linkDescription(state) },
        contentAlignment = Alignment.Center
    ) {
        if (pulsing) {
            Box(
                Modifier
                    .size(14.dp)
                    .scale(0.55f + 0.45f * pulse.value)
                    .alpha(1f - pulse.value)
                    .clip(CircleShape)
                    .background(color.copy(alpha = 0.35f))
            )
        }
        Box(
            Modifier
                .size(7.dp)
                .clip(CircleShape)
                .background(color)
        )
    }
}

/** Connection detail card shown at the top of the quota sheet. */
@Composable
fun LinkHealthDetailCard(snapshot: LinkSnapshot, onReconnect: () -> Unit, modifier: Modifier = Modifier) {
    val colors = AntigravityTheme.colors
    val now = rememberNowMs()
    val state = LinkHealth.evaluate(snapshot, now)
    val title = when (state) {
        LinkHealthState.UNKNOWN -> "连接中"
        LinkHealthState.CONNECTED -> "已连接电脑"
        LinkHealthState.ATTENTION -> "连接不稳或电脑端未就绪"
        LinkHealthState.DISCONNECTED -> "未连接"
    }
    val beatAge = snapshot.lastBeatAtMs?.let { ((now - it) / 1000).coerceAtLeast(0) }
    val beatText = when {
        beatAge == null -> "—"
        beatAge < 2 -> "刚刚"
        else -> "${beatAge} 秒前"
    }
    val desktopText = when (snapshot.desktop) {
        "ok" -> "已就绪"
        "down" -> "未就绪(请确认电脑端 Antigravity 已打开)"
        else -> "未知"
    }
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .background(colors.surface)
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Box(Modifier.size(9.dp).clip(CircleShape).background(linkColor(state)))
            Text(
                text = title,
                color = colors.textPrimary,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f)
            )
            OutlinedButton(onClick = onReconnect, contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp)) {
                Text("立即重连", fontSize = 12.sp)
            }
        }
        Text(
            text = "最近心跳 $beatText · 电脑端 $desktopText",
            color = colors.textSecondary,
            fontSize = 12.sp
        )
    }
}
