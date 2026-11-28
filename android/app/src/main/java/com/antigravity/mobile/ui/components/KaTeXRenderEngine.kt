package com.antigravity.mobile.ui.components

import android.annotation.SuppressLint
import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color as AndroidColor
import android.os.Handler
import android.os.Looper
import android.util.LruCache
import android.view.View
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import com.antigravity.mobile.data.service.MathSymbolProcessor
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import org.json.JSONObject
import java.lang.ref.WeakReference
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import kotlin.math.ceil
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

/**
 * Rendered KaTeX formula bitmap and typographic layout metrics for Jetpack Compose.
 */
data class RenderedMathFormula(
    val imageBitmap: ImageBitmap,
    val widthDp: Float,
    val heightDp: Float,
    val baselineFromBottomDp: Float,
    val isMonochrome: Boolean
)

/**
 * Global singleton offscreen KaTeX renderer backed by a single lazily-created software-layered WebView.
 * Produces transparent high-DPI ImageBitmaps cached in an LruCache so LazyColumn scrolling,
 * theme switching, and share-card exports stay 120fps without embedding WebViews per formula.
 */
object KaTeXRenderEngine {
    private data class CacheEntry(val formula: RenderedMathFormula?)

    private val cache = object : LruCache<String, CacheEntry>(400) {}
    private val inFlight = ConcurrentHashMap<String, CompletableDeferred<RenderedMathFormula?>>()
    private val renderMutex = Mutex()
    private val mainHandler = Handler(Looper.getMainLooper())
    private val requestSeq = AtomicLong(1L)

    @Volatile
    private var webView: WebView? = null
    @Volatile
    private var hostContainer: FrameLayout? = null
    @Volatile
    private var hostDecorRef: WeakReference<ViewGroup>? = null
    @Volatile
    private var pageReadyDeferred: CompletableDeferred<Boolean>? = null

    private data class MeasureResult(
        val ok: Boolean,
        val widthCss: Float,
        val heightCss: Float,
        val baselineFromBottomCss: Float
    )

    private val pendingMeasures = ConcurrentHashMap<Long, CompletableDeferred<MeasureResult>>()

    fun cacheKey(latex: String, displayMode: Boolean, fontSizeSp: Float, isDark: Boolean): String {
        val normalized = latex.trim()
        val bucketFont = (fontSizeSp * 2f).roundToInt() / 2f
        val monochrome = !MathSymbolProcessor.hasExplicitColor(normalized)
        val themeTag = if (monochrome) "M" else if (isDark) "D" else "L"
        val modeTag = if (displayMode) "B" else "I"
        return "$modeTag|$themeTag|$bucketFont|$normalized"
    }

    fun cached(latex: String, displayMode: Boolean, fontSizeSp: Float, isDark: Boolean): RenderedMathFormula? {
        val key = cacheKey(latex, displayMode, fontSizeSp, isDark)
        return cache.get(key)?.formula
    }

    suspend fun render(
        context: Context,
        latex: String,
        displayMode: Boolean,
        fontSizeSp: Float,
        density: Float,
        isDark: Boolean
    ): RenderedMathFormula? {
        val normalized = latex.trim()
        if (normalized.isEmpty()) return null

        val key = cacheKey(normalized, displayMode, fontSizeSp, isDark)
        cache.get(key)?.let { return it.formula }

        val newDeferred = CompletableDeferred<RenderedMathFormula?>()
        val existing = inFlight.putIfAbsent(key, newDeferred)
        if (existing != null) {
            return existing.await()
        }

        val result = try {
            renderMutex.withLock {
                cache.get(key)?.let { return@withLock it.formula }
                renderSingleFormula(
                    context = context,
                    latex = normalized,
                    displayMode = displayMode,
                    fontSizeSp = (fontSizeSp * 2f).roundToInt() / 2f,
                    density = density.coerceIn(1f, 4f),
                    isDark = isDark
                ).also { rendered ->
                    cache.put(key, CacheEntry(rendered))
                }
            }
        } catch (_: Exception) {
            null
        } finally {
            inFlight.remove(key)
        }

        newDeferred.complete(result)
        return result
    }

    private fun findActivity(context: Context): Activity? {
        var cur: Context? = context
        while (cur is ContextWrapper) {
            if (cur is Activity) return cur
            cur = cur.baseContext
        }
        return null
    }

    @SuppressLint("SetJavaScriptEnabled")
    private suspend fun ensureWebViewReady(context: Context): WebView? {
        val ready = withContext(Dispatchers.Main) {
            val existing = webView
            if (existing != null) {
                attachToActivityDecorIfNeeded(context, existing)
                return@withContext pageReadyDeferred
            }

            val deferred = CompletableDeferred<Boolean>()
            pageReadyDeferred = deferred

            val appCtx = context.applicationContext
            val wv = WebView(appCtx).apply {
                setBackgroundColor(AndroidColor.TRANSPARENT)
                // Software layer guarantees deterministic synchronous Canvas drawing even offscreen.
                // Keep WebView at alpha = 1.0f inside an offscreen 1x1 container so drawn pixels stay 100% opaque.
                setLayerType(View.LAYER_TYPE_SOFTWARE, null)
                alpha = 1.0f
                isClickable = false
                isFocusable = false
                importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = false
                    allowFileAccess = true
                    @Suppress("DEPRECATION")
                    allowFileAccessFromFileURLs = true
                    @Suppress("DEPRECATION")
                    allowUniversalAccessFromFileURLs = false
                    cacheMode = WebSettings.LOAD_NO_CACHE
                    textZoom = 100
                    useWideViewPort = false
                    loadWithOverviewMode = false
                    builtInZoomControls = false
                    displayZoomControls = false
                }
                addJavascriptInterface(JsBridge(), "KaTeXBridge")
                webViewClient = object : WebViewClient() {
                    override fun onPageFinished(view: WebView?, url: String?) {
                        super.onPageFinished(view, url)
                        deferred.complete(true)
                    }
                }
                val initPxW = 1200
                val initPxH = 600
                layoutParams = FrameLayout.LayoutParams(initPxW, initPxH)
                measure(
                    View.MeasureSpec.makeMeasureSpec(initPxW, View.MeasureSpec.EXACTLY),
                    View.MeasureSpec.makeMeasureSpec(initPxH, View.MeasureSpec.EXACTLY)
                )
                layout(0, 0, initPxW, initPxH)
            }

            val container = FrameLayout(appCtx).apply {
                layoutParams = FrameLayout.LayoutParams(1, 1)
                translationX = -20000f
                clipChildren = true
                clipToPadding = true
                isClickable = false
                isFocusable = false
                importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS
                addView(wv)
            }

            webView = wv
            hostContainer = container
            attachToActivityDecorIfNeeded(context, wv)
            wv.loadDataWithBaseURL(
                "file:///android_asset/katex/",
                RENDERER_HTML,
                "text/html",
                "UTF-8",
                null
            )
            deferred
        } ?: return null

        val ok = withTimeoutOrNull(6000L) { ready.await() } == true
        return if (ok) webView else null
    }

    private fun attachToActivityDecorIfNeeded(context: Context, wv: WebView) {
        val container = hostContainer ?: return
        val activity = findActivity(context) ?: return
        if (activity.isFinishing || activity.isDestroyed) return
        val decor = activity.window?.decorView as? ViewGroup ?: return
        val currentHost = hostDecorRef?.get()
        if (currentHost === decor && container.parent === decor) return
        (container.parent as? ViewGroup)?.removeView(container)
        decor.addView(container, 0)
        hostDecorRef = WeakReference(decor)
    }

    private suspend fun renderSingleFormula(
        context: Context,
        latex: String,
        displayMode: Boolean,
        fontSizeSp: Float,
        density: Float,
        isDark: Boolean
    ): RenderedMathFormula? {
        val wv = ensureWebViewReady(context) ?: return null
        val isMonochrome = !MathSymbolProcessor.hasExplicitColor(latex)
        val colorHex = if (isMonochrome) "#000000" else if (isDark) "#F2F2F7" else "#1C1C1E"
        val reqId = requestSeq.getAndIncrement()
        val measureDeferred = CompletableDeferred<MeasureResult>()
        pendingMeasures[reqId] = measureDeferred

        withContext(Dispatchers.Main) {
            attachToActivityDecorIfNeeded(context, wv)
            val jsLatex = JSONObject.quote(latex)
            val jsColor = JSONObject.quote(colorHex)
            val script = "window.__renderFormula($reqId, $jsLatex, $displayMode, $fontSizeSp, $jsColor);"
            wv.evaluateJavascript(script, null)
        }

        val measure = withTimeoutOrNull(4000L) { measureDeferred.await() }
        pendingMeasures.remove(reqId)
        if (measure == null || !measure.ok || measure.widthCss <= 1f || measure.heightCss <= 1f) {
            return null
        }

        val targetDpW = min(ceil(measure.widthCss.toDouble()).toFloat(), 2200f)
        val targetDpH = min(ceil(measure.heightCss.toDouble()).toFloat(), 1400f)
        val pxW = max(2, min(ceil((targetDpW * density).toDouble()).toInt(), 3600))
        val pxH = max(2, min(ceil((targetDpH * density).toDouble()).toInt(), 2400))

        val bitmapDeferred = CompletableDeferred<Bitmap?>()
        withContext(Dispatchers.Main) {
            val needW = max(pxW + 16, (1000 * density).toInt())
            val needH = max(pxH + 16, (400 * density).toInt())
            if (wv.width < pxW || wv.height < pxH) {
                wv.layoutParams = FrameLayout.LayoutParams(needW, needH)
                wv.measure(
                    View.MeasureSpec.makeMeasureSpec(needW, View.MeasureSpec.EXACTLY),
                    View.MeasureSpec.makeMeasureSpec(needH, View.MeasureSpec.EXACTLY)
                )
                wv.layout(0, 0, needW, needH)
            }
            wv.invalidate()

            val captureAction = Runnable {
                try {
                    val bmp = Bitmap.createBitmap(pxW, pxH, Bitmap.Config.ARGB_8888)
                    val canvas = Canvas(bmp)
                    wv.scrollTo(0, 0)
                    wv.draw(canvas)
                    bitmapDeferred.complete(bmp)
                } catch (_: Throwable) {
                    bitmapDeferred.complete(null)
                }
            }

            try {
                wv.postVisualStateCallback(reqId, object : WebView.VisualStateCallback() {
                    override fun onComplete(requestId: Long) {
                        mainHandler.postDelayed(captureAction, 16L)
                    }
                })
            } catch (_: Throwable) {
                mainHandler.postDelayed(captureAction, 32L)
            }
        }

        val rawBitmap = withTimeoutOrNull(2500L) { bitmapDeferred.await() } ?: return null
        val finalBitmap = normalizeBitmapTransparency(rawBitmap, isMonochrome) ?: return null
        return RenderedMathFormula(
            imageBitmap = finalBitmap.asImageBitmap(),
            widthDp = targetDpW,
            heightDp = targetDpH,
            baselineFromBottomDp = measure.baselineFromBottomCss.coerceIn(0f, targetDpH),
            isMonochrome = isMonochrome
        )
    }

    /**
     * Verifies the captured bitmap has visible pixels; if a device's WebView baked an opaque white
     * background onto a monochrome formula, converts white-matte luminance into crisp transparent alpha.
     */
    private fun normalizeBitmapTransparency(bmp: Bitmap, isMonochrome: Boolean): Bitmap? {
        val w = bmp.width
        val h = bmp.height
        if (w <= 0 || h <= 0) return null
        val pixels = IntArray(w * h)
        bmp.getPixels(pixels, 0, w, 0, 0, w, h)

        val corners = intArrayOf(pixels[0], pixels[w - 1], pixels[(h - 1) * w], pixels[w * h - 1])
        val hasOpaqueWhiteBg = isMonochrome && corners.all { p ->
            val a = (p ushr 24) and 0xFF
            val r = (p ushr 16) and 0xFF
            val g = (p ushr 8) and 0xFF
            val b = p and 0xFF
            a >= 250 && r >= 250 && g >= 250 && b >= 250
        }

        var visibleCount = 0
        if (hasOpaqueWhiteBg) {
            for (i in pixels.indices) {
                val p = pixels[i]
                val r = (p ushr 16) and 0xFF
                val g = (p ushr 8) and 0xFF
                val b = p and 0xFF
                val luma = (r * 77 + g * 150 + b * 29) shr 8
                val alpha = (255 - luma).coerceIn(0, 255)
                if (alpha <= 2) {
                    pixels[i] = 0
                } else {
                    pixels[i] = (alpha shl 24)
                    visibleCount++
                }
            }
            if (visibleCount == 0) return null
            bmp.setPixels(pixels, 0, w, 0, 0, w, h)
            return bmp
        }

        for (p in pixels) {
            if (((p ushr 24) and 0xFF) > 8) {
                visibleCount++
                break
            }
        }
        return if (visibleCount > 0) bmp else null
    }

    private class JsBridge {
        @JavascriptInterface
        fun onRendered(reqId: Long, ok: Boolean, widthCss: Float, heightCss: Float, baselineFromBottomCss: Float) {
            pendingMeasures[reqId]?.complete(
                MeasureResult(
                    ok = ok,
                    widthCss = widthCss,
                    heightCss = heightCss,
                    baselineFromBottomCss = baselineFromBottomCss
                )
            )
        }
    }

    private const val RENDERER_HTML = """
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0" />
  <link rel="stylesheet" href="katex.min.css" />
  <style>
    * { box-sizing: border-box; }
    html, body {
      margin: 0;
      padding: 0;
      background: transparent !important;
      overflow: hidden;
      font-family: "Noto Sans CJK SC", "Noto Sans", Roboto, sans-serif;
    }
    #stage {
      position: fixed;
      top: 0;
      left: 0;
      display: inline-flex;
      align-items: baseline;
      padding: 1px 2px;
      background: transparent;
      color: #000000;
      white-space: nowrap;
    }
    #stage.display-mode {
      padding: 3px 5px;
    }
    #stage .katex-display {
      margin: 0 !important;
      padding: 0 !important;
      display: inline-block !important;
    }
    #stage .katex-display > .katex {
      display: inline-block !important;
      text-align: left !important;
    }
    #baseline-probe {
      display: inline-block;
      width: 0;
      height: 0;
      overflow: hidden;
    }
  </style>
  <script src="katex.min.js"></script>
  <script src="mhchem.min.js"></script>
</head>
<body>
  <div id="stage"><span id="formula"></span><span id="baseline-probe"></span></div>
  <script>
    let __fontsWarmed = false;
    window.__renderFormula = async function(reqId, latex, displayMode, fontSizePx, textColorHex) {
      try {
        if (typeof katex === 'undefined') {
          KaTeXBridge.onRendered(reqId, false, 0, 0, 0);
          return;
        }
        const stage = document.getElementById('stage');
        const formulaEl = document.getElementById('formula');
        const probe = document.getElementById('baseline-probe');
        stage.className = displayMode ? 'display-mode' : '';
        stage.style.fontSize = fontSizePx + 'px';
        stage.style.color = textColorHex || '#000000';
        formulaEl.innerHTML = '';
        katex.render(latex, formulaEl, {
          displayMode: !!displayMode,
          throwOnError: true,
          strict: 'ignore',
          trust: false,
          output: 'html',
          maxSize: 50,
          maxExpand: 300
        });
        if (!__fontsWarmed && document.fonts && document.fonts.ready) {
          try { await document.fonts.ready; } catch (_) {}
          __fontsWarmed = true;
        }
        await new Promise(function(resolve) {
          requestAnimationFrame(resolve);
        });
        const stageRect = stage.getBoundingClientRect();
        const formulaRect = formulaEl.getBoundingClientRect();
        const probeRect = probe.getBoundingClientRect();
        const w = Math.ceil(Math.max(stageRect.width, formulaRect.right + 1));
        const h = Math.ceil(Math.max(stageRect.height, formulaRect.bottom + 1));
        const baselineFromBottom = Math.max(0, h - probeRect.bottom);
        KaTeXBridge.onRendered(reqId, true, w, h, baselineFromBottom);
      } catch (e) {
        KaTeXBridge.onRendered(reqId, false, 0, 0, 0);
      }
    };
  </script>
</body>
</html>
"""
}
