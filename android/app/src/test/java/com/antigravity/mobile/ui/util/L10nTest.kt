
package com.antigravity.mobile.ui.util

import java.util.Locale
import org.junit.Test
import org.junit.Assert.*

class L10nTest {

    @Test
    fun testLanguageDetectionAndSwitch() {
        // 1. Follow system English
        L10n.setLanguagePreference("system")
        Locale.setDefault(Locale.US)
        assertTrue("US locale should be detected as English", L10n.isEnglish)
        assertEquals("Settings", "设置".loc)
        assertEquals("Device Pairing", "设备配对".loc)
        assertEquals("Conversations", "会话列表".loc)
        assertEquals("Send", "发送".loc)
        assertEquals("Stop", "停止".loc)
        assertEquals("Run Terminal Command", "运行终端命令".loc)

        // 2. Follow system Chinese
        Locale.setDefault(Locale.SIMPLIFIED_CHINESE)
        assertFalse("Chinese locale should not be detected as English", L10n.isEnglish)
        assertEquals("设置", "设置".loc)
        assertEquals("设备配对", "设备配对".loc)
        assertEquals("会话列表", "会话列表".loc)
        assertEquals("发送", "发送".loc)
        assertEquals("停止", "停止".loc)

        // 3. Force English even in Chinese locale
        L10n.setLanguagePreference("en")
        assertTrue("Forced English should be English", L10n.isEnglish)
        assertEquals("Settings", "设置".loc)

        // 4. Force Chinese even in English locale
        Locale.setDefault(Locale.US)
        L10n.setLanguagePreference("zh")
        assertFalse("Forced Chinese should not be English", L10n.isEnglish)
        assertEquals("设置", "设置".loc)

        // Reset to system
        L10n.setLanguagePreference("system")
    }

    @Test
    fun testPatternRules() {
        L10n.setLanguagePreference("en")
        assertEquals("5s ago", "5 秒前".loc)
        assertEquals("10m ago", "10 分钟前".loc)
        assertEquals("2h ago", "2 小时前".loc)
        assertEquals("3d ago", "3 天前".loc)
        assertEquals("12 files", "12 个文件".loc)
        assertEquals("4 workspaces", "4 个工作区".loc)
        assertEquals("3 sub-agents running", "3 个子代理运行中".loc)
        assertEquals("5 background tasks running (2 daemons)", "5 个后台任务运行中（2 个常驻）".loc)
        assertEquals("2 tasks running", "2 个任务正在执行".loc)
        assertEquals("7 tool calls", "7 次工具调用".loc)
        assertEquals("3 steps", "3 步骤".loc)
        assertEquals("4 turns", "4 轮".loc)
        assertEquals("3 matches", "3 处".loc)
        assertEquals("2/5 active", "2/5 活跃".loc)
        assertEquals("#42 Board", "#42 看板".loc)
        assertEquals("Report · Uploading", "Report · 上传中".loc)
        assertEquals("Live · 15 messages", "Live · 共 15 条消息".loc)
        assertEquals("[2 images included]", "[包含 2 张图片]".loc)
        assertEquals("retried 3 times", "已重试 3 次".loc)
        assertEquals("Attempt 1/3", "尝试 1/3".loc)
        assertEquals("Step 5", "第 5 步".loc)
        assertEquals("Turn 2", "第 2 轮".loc)
        assertEquals("Running step 4...", "正在执行第 4 步...".loc)
        assertEquals("Running background tasks (3)...", "正在执行后台任务 (3)...".loc)
        assertEquals("Executing: Run Terminal Command", "正在执行: Run Terminal Command".loc)

        L10n.setLanguagePreference("system")
    }
}
