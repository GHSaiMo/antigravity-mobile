
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
        assertEquals(
            "Used 0.3%, at or below the 5-Hour even-pace baseline (14.3%). Healthy remaining headroom.",
            "当前额度已消耗 0.3%，低于或符合 5-Hour时间匀速基准线 (14.3%)。剩余额度充足，无需担心断粮。".loc
        )
        assertEquals(
            "Used 45.0%, faster than the 7-Day even-pace baseline (20.0%). May run out before reset at current pace.",
            "当前额度已消耗 45.0%，快于 7-Day时间匀速基准线 (20.0%)。若保持此速率可能在周期结束前提前耗尽。".loc
        )
        assertEquals("5-Hour Even-Pace Remaining", "5-Hour匀速应剩 (游标位)".loc)
        assertEquals("5-Hour Even-Pace Used Baseline", "5-Hour匀速应耗基准".loc)

        L10n.setLanguagePreference("ja")
        assertEquals(
            "現在の消費は 0.3% で、5時間の均等基準ライン (14.3%) 以下です。残枠に十分な余裕があります。",
            "当前额度已消耗 0.3%，低于或符合 5時間时间匀速基准线 (14.3%)。剩余额度充足，无需担心断粮。".loc
        )
        assertEquals("5時間均等残枠の目安", "5時間匀速应剩 (游标位)".loc)
        assertEquals("5時間均等消費の基準", "5時間匀速应耗基准".loc)

        L10n.setLanguagePreference("system")
    }
}
