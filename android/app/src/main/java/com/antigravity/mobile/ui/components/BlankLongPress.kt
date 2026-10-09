package com.antigravity.mobile.ui.components

import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.PointerEventTimeoutCancellationException
import androidx.compose.ui.input.pointer.changedToUp
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalTextToolbar
import androidx.compose.ui.platform.LocalViewConfiguration
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * Compose 的文字选择在 Text 的整个外接矩形内都会响应长按，包括换行后一行末尾的空白。
 * 这里登记每段可选文字的版面，长按时按"字形实际占据的范围"判定落点是否真在文字上；
 * 落在文字之外的空白处则弹出气泡菜单，并撤销随之启动的文字选择。
 */
internal class TextHitRegistry {
    class Entry {
        var coords: LayoutCoordinates? = null
        var layout: TextLayoutResult? = null
    }

    private val entries = LinkedHashSet<Entry>()

    fun register(entry: Entry) { entries.add(entry) }
    fun unregister(entry: Entry) { entries.remove(entry) }

    /** 落点在某段文字的外接矩形内，却不在该行字形范围内时为 true。 */
    fun isBlank(windowPos: Offset, tolerancePx: Float): Boolean {
        for (e in entries) {
            val c = e.coords?.takeIf { it.isAttached } ?: continue
            val l = e.layout ?: continue
            val p = c.windowToLocal(windowPos)
            if (p.x < 0f || p.y < 0f || p.x > c.size.width || p.y > c.size.height) continue
            if (l.lineCount == 0) return true
            if (p.y > l.size.height) return true
            val line = l.getLineForVerticalPosition(p.y)
            return p.x > l.getLineRight(line) + tolerancePx
        }
        return false
    }
}

internal val LocalTextHitRegistry = compositionLocalOf<TextHitRegistry?> { null }

/** 供 RichTextRenderer 等可选文字登记版面。 */
@Composable
internal fun Modifier.registerTextHit(layout: TextLayoutResult?): Modifier {
    val registry = LocalTextHitRegistry.current ?: return this
    val entry = remember { TextHitRegistry.Entry() }
    entry.layout = layout
    DisposableEffect(registry, entry) {
        registry.register(entry)
        onDispose { registry.unregister(entry) }
    }
    return this.onGloballyPositioned { entry.coords = it }
}

@Composable
internal fun BlankLongPressHost(
    onBlankLongPress: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit
) {
    val registry = remember { TextHitRegistry() }
    val focusManager = LocalFocusManager.current
    val textToolbar = LocalTextToolbar.current
    val currentCallback by rememberUpdatedState(onBlankLongPress)
    val tolerancePx = with(LocalDensity.current) { 6.dp.toPx() }
    val scope = rememberCoroutineScope()
    var origin by remember { mutableStateOf(Offset.Zero) }
    val longPressTimeout = LocalViewConfiguration.current.longPressTimeoutMillis

    Box(
        modifier = modifier
            .onGloballyPositioned { origin = it.positionInWindow() }
            .pointerInput(registry) {
                awaitEachGesture {
                    val down = awaitFirstDown(requireUnconsumed = false, pass = PointerEventPass.Initial)
                    val blank = registry.isBlank(origin + down.position, tolerancePx)
                    if (!blank) return@awaitEachGesture
                    var fired = false
                    try {
                        withTimeout(longPressTimeout) {
                            while (true) {
                                val event = awaitPointerEvent(PointerEventPass.Initial)
                                val change = event.changes.firstOrNull { it.id == down.id } ?: return@withTimeout
                                if (change.changedToUp()) return@withTimeout
                                if ((change.position - down.position).getDistance() > viewConfiguration.touchSlop) return@withTimeout
                            }
                        }
                    } catch (_: PointerEventTimeoutCancellationException) {
                        fired = true
                    }
                    if (fired) {
                        currentCallback()
                        // 与文字选择的长按同时触发：稍后撤销刚启动的选择与浮动工具栏
                        scope.launch {
                            delay(80)
                            focusManager.clearFocus()
                            textToolbar.hide()
                        }
                    }
                }
            }
    ) {
        androidx.compose.runtime.CompositionLocalProvider(LocalTextHitRegistry provides registry) {
            content()
        }
    }
}
