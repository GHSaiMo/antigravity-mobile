package com.antigravity.mobile.ui.util

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabColorSchemeParams
import androidx.browser.customtabs.CustomTabsIntent
import androidx.core.graphics.toColorInt

object InAppBrowserUtils {
    /**
     * 以沉浸式 Chrome Custom Tabs 打开外部 Web 链接
     * 保留浏览器登录态与 Cookie，提供返回按钮与便捷工具栏，无需跳出 App
     */
    fun openUrl(context: Context, url: String, toolbarColorHex: String = "#5856D6") {
        val clean = url.trim()
        if (clean.isBlank()) return

        try {
            val uri = Uri.parse(clean)
            val colorParams = CustomTabColorSchemeParams.Builder()
                .setToolbarColor(toolbarColorHex.toColorInt())
                .build()

            val customTabsIntent = CustomTabsIntent.Builder()
                .setDefaultColorSchemeParams(colorParams)
                .setShowTitle(true)
                .setUrlBarHidingEnabled(true)
                .setShareState(CustomTabsIntent.SHARE_STATE_ON)
                .build()

            customTabsIntent.launchUrl(context, uri)
        } catch (_: Exception) {
            // 回退到系统通用 ACTION_VIEW 打开
            try {
                val fallback = Intent(Intent.ACTION_VIEW, Uri.parse(clean)).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK
                }
                context.startActivity(fallback)
            } catch (_: Exception) {}
        }
    }
}
