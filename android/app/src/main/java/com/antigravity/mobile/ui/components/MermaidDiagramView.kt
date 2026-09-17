package com.antigravity.mobile.ui.components

import android.annotation.SuppressLint
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import android.webkit.WebSettings
import android.webkit.WebView
import android.widget.Toast
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.spring
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import org.json.JSONObject

enum class MermaidViewMode(val label: String) {
    DIAGRAM("图表"),
    CODE("代码")
}

/**
 * 1:1 Jetpack Compose implementation of iOS MermaidDiagramView.swift.
 * Displays interactive, zoomable Mermaid.js architecture and sequence diagrams
 * rendered via local assets and dark-theme SVG generation.
 */
@Composable
fun MermaidDiagramView(
    code: String,
    colors: AppColors = AntigravityTheme.colors,
    modifier: Modifier = Modifier
) {
    var viewMode by remember { mutableStateOf(MermaidViewMode.DIAGRAM) }
    var diagramHeightDp by remember { mutableStateOf(220.dp) }
    var renderError by remember { mutableStateOf<String?>(null) }
    var isFullscreen by remember { mutableStateOf(false) }
    var isCopied by remember { mutableStateOf(false) }

    val clipboardManager = LocalClipboardManager.current
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val isDark = isSystemInDarkTheme()

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.4f))
            .border(0.8.dp, colors.border, RoundedCornerShape(10.dp))
            .animateContentSize(spring(dampingFraction = 0.82f))
    ) {
        // Top Header Bar
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(colors.surfaceVariant.copy(alpha = 0.7f))
                .padding(horizontal = 12.dp, vertical = 7.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            // Label with Schematic Icon
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.AccountTree,
                    contentDescription = null,
                    tint = colors.textMuted,
                    modifier = Modifier.size(13.dp)
                )
                Text(
                    text = "MERMAID",
                    color = colors.textMuted,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Bold,
                    fontFamily = FontFamily.Monospace
                )
            }

            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                // Segmented Switcher: Diagram / Code
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(7.dp))
                        .background(colors.surface.copy(alpha = 0.6f))
                        .padding(2.dp)
                ) {
                    MermaidViewMode.values().forEach { mode ->
                        val isSelected = viewMode == mode
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(5.dp))
                                .background(if (isSelected) colors.surfaceVariant else Color.Transparent)
                                .clickable {
                                    haptic.light()
                                    viewMode = mode
                                }
                                .padding(horizontal = 8.dp, vertical = 3.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            Text(
                                text = mode.label,
                                color = if (isSelected) colors.textPrimary else colors.textMuted,
                                fontSize = 11.sp,
                                fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                            )
                        }
                    }
                }

                // Fullscreen button (only in diagram mode when no error)
                if (viewMode == MermaidViewMode.DIAGRAM && renderError == null) {
                    Box(
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .clickable {
                                haptic.light()
                                isFullscreen = true
                            }
                            .padding(4.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Fullscreen,
                            contentDescription = "全屏查看",
                            tint = colors.textSecondary,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }

                // Copy Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(6.dp))
                        .clickable {
                            haptic.medium()
                            clipboardManager.setText(AnnotatedString(code))
                            isCopied = true
                            Toast.makeText(context, "代码已复制", Toast.LENGTH_SHORT).show()
                            Handler(Looper.getMainLooper()).postDelayed({ isCopied = false }, 1800)
                        }
                        .padding(horizontal = 6.dp, vertical = 3.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Icon(
                        imageVector = if (isCopied) Icons.Default.Check else Icons.Default.ContentCopy,
                        contentDescription = "Copy",
                        tint = if (isCopied) colors.accentIndigo else colors.textSecondary,
                        modifier = Modifier.size(12.dp)
                    )
                    Text(
                        text = if (isCopied) "已复制" else "复制",
                        color = if (isCopied) colors.accentIndigo else colors.textSecondary,
                        fontSize = 11.sp
                    )
                }
            }
        }

        HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.6.dp)

        // Main Content: Diagram or Monospaced Code
        if (viewMode == MermaidViewMode.DIAGRAM) {
            if (renderError != null) {
                // Parse Error Fallback
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Warning,
                                contentDescription = null,
                                tint = Color(0xFFFF9800),
                                modifier = Modifier.size(15.dp)
                            )
                            Text(
                                text = "Mermaid 语法解析失败",
                                color = colors.textPrimary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }
                        Text(
                            text = "查看源码",
                            color = colors.accentIndigo,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.Medium,
                            modifier = Modifier.clickable { viewMode = MermaidViewMode.CODE }
                        )
                    }

                    Text(
                        text = renderError ?: "未知错误",
                        color = colors.textSecondary,
                        fontSize = 11.sp,
                        fontFamily = FontFamily.Monospace,
                        lineHeight = 15.sp
                    )
                }
            } else {
                // Interactive Diagram Container
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(diagramHeightDp),
                    contentAlignment = Alignment.BottomEnd
                ) {
                    val density = LocalDensity.current
                    MermaidWebViewContainer(
                        code = code,
                        isDark = isDark,
                        isFullscreen = false,
                        modifier = Modifier.fillMaxSize(),
                        onHeightMeasured = { hPx ->
                            val dpVal = with(density) { hPx.toDp() }
                            diagramHeightDp = dpVal.coerceIn(120.dp, 520.dp)
                        },
                        onError = { err ->
                            renderError = err
                        }
                    )

                    // Subtle hint pill for pinch zoom
                    Row(
                        modifier = Modifier
                            .padding(8.dp)
                            .clip(CircleShape)
                            .background(colors.surface.copy(alpha = 0.85f))
                            .border(0.5.dp, colors.border.copy(alpha = 0.5f), CircleShape)
                            .padding(horizontal = 8.dp, vertical = 3.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.ZoomIn,
                            contentDescription = null,
                            tint = colors.textMuted,
                            modifier = Modifier.size(11.dp)
                        )
                        Text(
                            text = "双指可缩放",
                            color = colors.textSecondary,
                            fontSize = 10.sp,
                            fontWeight = FontWeight.Medium
                        )
                    }
                }
            }
        } else {
            // Code View
            SelectionContainer {
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState())
                        .padding(12.dp)
                ) {
                    Text(
                        text = code.trimEnd(),
                        color = colors.textPrimary,
                        fontSize = 12.5.sp,
                        fontFamily = FontFamily.Monospace,
                        lineHeight = 18.sp
                    )
                }
            }
        }
    }

    // Fullscreen Dialog Modal
    if (isFullscreen) {
        Dialog(
            onDismissRequest = { isFullscreen = false },
            properties = DialogProperties(usePlatformDefaultWidth = false)
        ) {
            Surface(
                modifier = Modifier.fillMaxSize(),
                color = colors.background
            ) {
                Column(modifier = Modifier.fillMaxSize()) {
                    // Dialog Top Bar
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .statusBarsPadding()
                            .padding(horizontal = 8.dp, vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        IconButton(onClick = { isFullscreen = false }) {
                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                                contentDescription = "Back",
                                tint = colors.textPrimary
                            )
                        }

                        Text(
                            text = "Mermaid 架构图",
                            color = colors.textPrimary,
                            fontSize = 16.sp,
                            fontWeight = FontWeight.SemiBold
                        )

                        TextButton(onClick = {
                            clipboardManager.setText(AnnotatedString(code))
                            Toast.makeText(context, "代码已复制", Toast.LENGTH_SHORT).show()
                        }) {
                            Text(
                                text = "复制源码",
                                color = colors.accentIndigo,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }
                    }

                    HorizontalDivider(color = colors.border.copy(alpha = 0.4f))

                    // Fullscreen Diagram
                    MermaidWebViewContainer(
                        code = code,
                        isDark = isDark,
                        isFullscreen = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f)
                    )
                }
            }
        }
    }
}

/**
 * Native Android WebView wrapper rendering Mermaid.js.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
private fun MermaidWebViewContainer(
    code: String,
    isDark: Boolean = false,
    isFullscreen: Boolean = false,
    modifier: Modifier = Modifier,
    onHeightMeasured: ((Int) -> Unit)? = null,
    onError: ((String) -> Unit)? = null
) {
    val context = LocalContext.current
    val htmlContent = remember(code, isDark, isFullscreen) {
        buildMermaidHtml(code = code, isDark = isDark, isFullscreen = isFullscreen)
    }

    AndroidView(
        modifier = modifier,
        factory = { ctx ->
            WebView(ctx).apply {
                setBackgroundColor(android.graphics.Color.TRANSPARENT)
                isVerticalScrollBarEnabled = false
                isHorizontalScrollBarEnabled = true

                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = true
                    loadWithOverviewMode = true
                    useWideViewPort = true
                    builtInZoomControls = true
                    displayZoomControls = false
                    cacheMode = WebSettings.LOAD_DEFAULT
                }

                addJavascriptInterface(
                    object {
                        @JavascriptInterface
                        fun postSize(height: Int, width: Int) {
                            Handler(Looper.getMainLooper()).post {
                                onHeightMeasured?.invoke(height)
                            }
                        }

                        @JavascriptInterface
                        fun postError(error: String) {
                            Handler(Looper.getMainLooper()).post {
                                onError?.invoke(error)
                            }
                        }
                    },
                    "AndroidBridge"
                )

                loadDataWithBaseURL("file:///android_asset/", htmlContent, "text/html", "UTF-8", null)
            }
        },
        update = { webView ->
            webView.loadDataWithBaseURL("file:///android_asset/", htmlContent, "text/html", "UTF-8", null)
        }
    )
}

/**
 * Builds HTML template bundling Mermaid.js from assets.
 */
private fun buildMermaidHtml(code: String, isDark: Boolean, isFullscreen: Boolean): String {
    val escapedCodeJson = JSONObject.quote(code)

    return """
        <!DOCTYPE html>
        <html>
        <head>
          <meta charset="utf-8">
          <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=5.0, user-scalable=yes">
          <style>
            :root {
              color-scheme: ${if (isDark) "dark" else "light"};
            }
            * {
              box-sizing: border-box;
              -webkit-touch-callout: none;
            }
            html, body {
              margin: 0;
              padding: 0;
              width: 100%;
              min-height: 100%;
              background: transparent;
              font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
              display: flex;
              justify-content: center;
              align-items: center;
            }
            #wrapper {
              width: 100%;
              display: flex;
              justify-content: center;
              align-items: center;
              padding: ${if (isFullscreen) "24px 16px" else "12px 8px"};
              overflow-x: auto;
              -webkit-overflow-scrolling: touch;
            }
            #container {
              display: inline-block;
              max-width: 100%;
              text-align: center;
            }
            svg {
              max-width: 100%;
              height: auto !important;
              display: block;
              margin: 0 auto;
            }
            ${if (isFullscreen) "body.fullscreen svg { max-width: 95vw; }" else ""}
          </style>
          <script src="mermaid.min.js"></script>
        </head>
        <body class="${if (isFullscreen) "fullscreen" else ""}">
          <div id="wrapper">
            <div id="container"></div>
          </div>
          <script>
            (function() {
              const isDark = ${if (isDark) "true" else "false"};
              const rawCode = $escapedCodeJson;

              function reportError(err) {
                if (window.AndroidBridge && window.AndroidBridge.postError) {
                  window.AndroidBridge.postError(String(err));
                }
              }
              function reportSize(h, w) {
                if (window.AndroidBridge && window.AndroidBridge.postSize) {
                  window.AndroidBridge.postSize(Math.round(h), Math.round(w));
                }
              }

              if (typeof mermaid === 'undefined') {
                reportError("未能加载 mermaid.min.js 引擎");
                return;
              }

              try {
                mermaid.initialize({
                  startOnLoad: false,
                  securityLevel: 'strict',
                  theme: isDark ? 'dark' : 'default',
                  themeVariables: isDark ? {
                    darkMode: true,
                    background: 'transparent',
                    primaryColor: '#1e293b',
                    primaryTextColor: '#f8fafc',
                    primaryBorderColor: '#475569',
                    lineColor: '#94a3b8',
                    secondaryColor: '#334155',
                    tertiaryColor: '#0f172a'
                  } : {
                    darkMode: false,
                    background: 'transparent',
                    primaryColor: '#f1f5f9',
                    primaryTextColor: '#0f172a',
                    primaryBorderColor: '#cbd5e1',
                    lineColor: '#64748b',
                    secondaryColor: '#f8fafc',
                    tertiaryColor: '#ffffff'
                  }
                });

                const id = 'mermaid_' + Math.random().toString(36).substring(2, 9);
                mermaid.render(id, rawCode).then(function(result) {
                  const container = document.getElementById('container');
                  container.innerHTML = result.svg;

                  setTimeout(function() {
                    let h = 200, w = 300;
                    const svgEl = container.querySelector('svg');
                    if (svgEl) {
                      const rect = svgEl.getBoundingClientRect();
                      h = Math.ceil(rect.height || svgEl.clientHeight || 200) + 20;
                      w = Math.ceil(rect.width || svgEl.clientWidth || 300);
                    } else {
                      h = Math.ceil(document.body.scrollHeight || 200);
                    }
                    reportSize(h, w);
                  }, 60);
                }).catch(function(err) {
                  const msg = (err && err.message) ? err.message : String(err);
                  reportError(msg);
                });
              } catch (e) {
                reportError(String(e));
              }
            })();
          </script>
        </body>
        </html>
    """.trimIndent()
}
