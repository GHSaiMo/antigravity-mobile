package com.antigravity.mobile.ui.util

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.antigravity.mobile.data.model.GatewayMessageItem
import kotlinx.coroutines.delay

private const val PACING_TICK_MS = 33L
/** 积压按此帧数追平：越大越平缓；积压很少时每帧 1 个字符（约 30 字/秒）。 */
private const val PACING_CATCH_UP_TICKS = 20

/**
 * 流式输出的“匀速播放”：网关推送的文本增量可能一次来很多，直接渲染会让内容疯狂上涨、
 * 什么都看不清。这里把 Agent 消息的显示进度和网络到达速度解耦，按稳定速度追上目标文本。
 * 首次出现（历史加载、列表项回收后重现）直接显示完整内容，只对之后的增长做节奏控制。
 */
@Composable
fun rememberPacedMessage(message: GatewayMessageItem): GatewayMessageItem {
    if (!message.isAgent) return message
    val full = message.effectiveText
    var shown by remember { mutableIntStateOf(full.length) }

    LaunchedEffect(full.length) {
        if (full.length < shown) shown = full.length
        while (shown < full.length) {
            val backlog = full.length - shown
            val step = maxOf(1, (backlog + PACING_CATCH_UP_TICKS - 1) / PACING_CATCH_UP_TICKS)
            shown = minOf(shown + step, full.length)
            delay(PACING_TICK_MS)
        }
    }

    if (shown >= full.length) return message
    var end = shown
    if (end > 0 && full[end - 1].isHighSurrogate()) end -= 1
    return message.copy(text = full.substring(0, end), content = "")
}
