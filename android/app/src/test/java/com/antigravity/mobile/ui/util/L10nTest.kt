
package com.antigravity.mobile.ui.util

import java.util.Locale
import org.junit.Test
import org.junit.Assert.*

class L10nTest {

    @Test
    fun testLanguageDetectionAndSwitch() {
        val originalLocale = Locale.getDefault()
        try {
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
        } finally {
            Locale.setDefault(originalLocale)
            L10n.setLanguagePreference("system")
        }
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

    @Test
    fun testFormattedTemplatesAndTerminologyAlignment() {
        try {
            L10n.setLanguagePreference("en")
            assertEquals("System", "跟随系统".loc)
            assertEquals("Swarm", "蜂群协同".loc)
            assertEquals("Unpair", "解除配对".loc)
            assertEquals("Manage Tasks", "管理后台任务".loc)
            assertEquals("Schedule", "定时调度".loc)
            assertEquals("Offline", "未连接".loc)
            assertEquals("Deny", "拒绝".loc)
            assertEquals("Deny", "拒绝执行".loc)
            assertEquals("Approval", "待审批".loc)
            assertEquals("Switch workspace", "切换工作区".loc)
            assertEquals("3 of 5 done", "已完成 %1\$d/%2\$d".loc(3, 5))
            assertEquals("Failed to load messages: HTTP 502", "获取消息失败: HTTP %d".loc(502))
            assertEquals("File exceeds 20MB: demo.zip", "文件超过 %1\$dMB：%2\$s".loc(20, "demo.zip"))
            assertEquals("2 waiting on @verifier", "%1\$d 项等待 @%2\$s".loc(2, "verifier"))
            assertEquals("Post", "发板".loc)
            assertEquals("Recents", "最近项目".loc)
            assertEquals("Scan", "文稿".loc)
            assertEquals("Allow Photos", "允许访问照片".loc)
            assertEquals("1 background task running (1 daemon)", "1 个后台任务运行中（1 个常驻）".loc)
            assertEquals("1 artifact", "1 个产物".loc)
            assertEquals("1 task running", "1 个任务正在执行".loc)
            assertEquals("1 chat", "1 个会话".loc)
            assertEquals("Retired", "已退役".loc)
            assertEquals("Swarm Session", "蜂群会话".loc)
            assertEquals("Waiting for build (3 tasks) to finish", "等待 %1\$s 等 %2\$d 项完成后出列".loc("build", 3))
            assertEquals("[Attachment: a.png (3 files)]", "[附件: %1\$s 等 %2\$d 个文件]".loc("a.png", 3))
            assertFalse("No iOS mention in Android album permission text", "未获得相册权限，请在系统设置中开启".loc.contains("iOS"))
            assertFalse("No iOS mention in Android alert sound text", "跟随 iOS 系统提示音".loc.contains("iOS"))
            assertFalse("No iOS mention in Android photos permission prompt", "请在系统“设置”中允许 Multigravity 访问相册以保存图片。".loc.contains("iOS"))

            L10n.setLanguagePreference("ja")
            assertEquals("システム", "跟随系统".loc)
            assertEquals("スウォーム連携", "蜂群协同".loc)
            assertEquals("解除", "解除配对".loc)
            assertEquals("タスク管理", "管理后台任务".loc)
            assertEquals("スケジュール", "定时调度".loc)
            assertEquals("オフライン", "未连接".loc)
            assertEquals("拒否", "拒绝".loc)
            assertEquals("実行を拒否", "拒绝执行".loc)
            assertEquals("承認待ち", "待审批".loc)
            assertEquals("ワークスペースを切替", "切换工作区".loc)
            assertEquals("3/5 完了", "已完成 %1\$d/%2\$d".loc(3, 5))
            assertEquals("メッセージの取得に失敗しました: HTTP 502", "获取消息失败: HTTP %d".loc(502))
            assertEquals("ファイルが 20MB を超えています：demo.zip", "文件超过 %1\$dMB：%2\$s".loc(20, "demo.zip"))
            assertEquals("2 件が @verifier 待ち", "%1\$d 项等待 @%2\$s".loc(2, "verifier"))
            assertEquals("最近の項目", "最近项目".loc)
            assertEquals("スキャン", "文稿".loc)
            assertEquals("写真を許可", "允许访问照片".loc)
            assertEquals("バックグラウンド", "转后台".loc)
            assertEquals("クイックメニュー", "长按快捷菜单".loc)
            assertEquals("終了", "已退役".loc)
            assertEquals("スウォームセッション", "蜂群会话".loc)
            assertEquals("[添付: a.png など 3 件]", "[附件: %1\$s 等 %2\$d 个文件]".loc("a.png", 3))
            assertFalse("No iOS mention in JA album permission text", "未获得相册权限，请在系统设置中开启".loc.contains("iOS"))
            assertFalse("No iOS mention in JA alert sound text", "跟随 iOS 系统提示音".loc.contains("iOS"))
            assertFalse("No iOS mention in JA photos permission prompt", "请在系统“设置”中允许 Multigravity 访问相册以保存图片。".loc.contains("iOS"))

            L10n.setLanguagePreference("zh")
            assertEquals("获取消息失败: HTTP 502", "获取消息失败: HTTP %d".loc(502))
            assertEquals("文件超过 20MB：demo.zip", "文件超过 %1\$dMB：%2\$s".loc(20, "demo.zip"))
            assertEquals("2 项等待 @verifier", "%1\$d 项等待 @%2\$s".loc(2, "verifier"))
        } finally {
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun testVoiceTroubleshootLocalizationAndPrompt() {
        val networkChecklistKey = "**检查网络与代理**：确保 `*.googleapis.com` 长连接（HTTP/2、gRPC）未被拦截（使用代理时请开启 TUN 模式）"
        val micChecklistKey = "**检查电脑麦克风**：系统麦克风权限已开启，默认录音设备正常且未被占用"
        try {
            L10n.setLanguagePreference("en")
            val enNet = networkChecklistKey.loc
            assertTrue("EN network hint should mention firewall/VPN/proxy", enNet.contains("firewall", ignoreCase = true))
            assertFalse("EN network hint should not mention TUN mode", enNet.contains("TUN"))
            assertFalse("EN mic hint should not hardcode Mac", micChecklistKey.loc.contains("Mac"))

            val enPrompt = VoiceCallManager.troubleshootPrompt(L10n.AppLang.EN)
            assertTrue("EN prompt should be English", enPrompt.contains("[Task]"))
            assertFalse("EN prompt should not mention Clash", enPrompt.contains("Clash"))
            assertFalse("EN prompt should not mention TUN", enPrompt.contains("TUN"))

            L10n.setLanguagePreference("ja")
            val jaNet = networkChecklistKey.loc
            assertTrue("JA network hint should mention firewall", jaNet.contains("ファイアウォール"))
            assertFalse("JA network hint should not mention TUN mode", jaNet.contains("TUN"))

            val jaPrompt = VoiceCallManager.troubleshootPrompt(L10n.AppLang.JA)
            assertTrue("JA prompt should be Japanese", jaPrompt.contains("【タスク】"))
            assertFalse("JA prompt should not mention Clash", jaPrompt.contains("Clash"))
            assertFalse("JA prompt should not mention TUN", jaPrompt.contains("TUN"))

            L10n.setLanguagePreference("zh")
            val zhPrompt = VoiceCallManager.troubleshootPrompt(L10n.AppLang.ZH)
            assertTrue("ZH prompt should be Chinese", zhPrompt.contains("【任务】"))
            assertTrue("ZH prompt should keep conditional proxy TUN guidance", zhPrompt.contains("TUN"))
        } finally {
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun testEnglishFirstFallbackForMissingJapaneseAndOtherLocales() {
        // 1. Simulated missing Japanese key/pattern must fall back to English, never Chinese.
        val missingDictFallback = L10n.translateWithFallback(
            chinese = "设置",
            lang = L10n.AppLang.JA,
            jaDict = emptyMap(),
            jaRules = emptyList()
        )
        assertEquals("Settings", missingDictFallback)

        val missingPatternFallback = L10n.translateWithFallback(
            chinese = "3 分钟前",
            lang = L10n.AppLang.JA,
            jaDict = emptyMap(),
            jaRules = emptyList()
        )
        assertEquals("3m ago", missingPatternFallback)

        // 2. Non-zh/ja system locales (e.g. Korean, French) must resolve to English, never Chinese.
        val prevLocale = java.util.Locale.getDefault()
        try {
            L10n.setLanguagePreference("system")
            java.util.Locale.setDefault(java.util.Locale.KOREA)
            assertEquals(L10n.AppLang.EN, L10n.currentLang)
            assertEquals("Settings", "设置".loc)

            java.util.Locale.setDefault(java.util.Locale.FRANCE)
            assertEquals(L10n.AppLang.EN, L10n.currentLang)

            java.util.Locale.setDefault(java.util.Locale.JAPAN)
            assertEquals(L10n.AppLang.JA, L10n.currentLang)

            java.util.Locale.setDefault(java.util.Locale.SIMPLIFIED_CHINESE)
            assertEquals(L10n.AppLang.ZH, L10n.currentLang)
        } finally {
            java.util.Locale.setDefault(prevLocale)
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun testV128LocalizationAndSideQuestionParity() {
        val prevLocale = Locale.getDefault()
        try {
            L10n.setLanguagePreference("en")
            assertEquals("Offline", "未连接".loc)
            assertEquals("Auto Tightest", "自动最紧".loc)
            assertEquals("Context Breakdown", "上下文占用分布".loc)
            assertEquals("Compacted", "已压缩".loc)
            assertEquals("Viewing snapshot v3", "正在查看历史快照 v%d".loc(3))
            assertEquals("Viewing snapshot v4", "正在查看历史快照 v4".loc)
            assertEquals("Sign in to GitHub", "%s 登录".loc("GitHub"))
            assertEquals(
                "Note: 2 active session(s) are running on the host. Applying the update will restart Antigravity Desktop.",
                "注意：电脑端当前有 %d 个活跃会话运行中，应用更新会重启 Antigravity 桌面客户端。".loc(2)
            )
            assertEquals(
                "Describe what you want to automate for AI to set up, or configure a Cron schedule manually",
                "描述你想自动化的事项让 AI 帮你设计，或手动配置 Cron 计划".loc
            )
            assertEquals("Ready", com.antigravity.mobile.data.model.CockpitQuotaBucket(resetFriendly = "已就绪", resetTime = "2026-11-28T00:00:00Z").resetCountdownDisplay)

            L10n.setLanguagePreference("ja")
            assertEquals("オフライン", "未连接".loc)
            assertEquals("自動最小", "自动最紧".loc)
            assertEquals("コンテキスト内訳", "上下文占用分布".loc)
            assertEquals("圧縮済み", "已压缩".loc)
            assertEquals("スナップショット v3 を表示中", "正在查看历史快照 v%d".loc(3))
            assertEquals("1 件のサイド質問", "1 个旁路".loc)
            assertEquals("3 件のサイド質問", "%d 个旁路".loc(3))
            assertTrue("JA update banner should not leak Chinese", "注意：电脑端当前有 %d 个活跃会话运行中，应用更新会重启 Antigravity 桌面客户端。".loc(1).contains("アクティブ"))
        } finally {
            Locale.setDefault(prevLocale)
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun testDeviceTitleTargetLanguageAcrossLocales() {
        val prevLocale = Locale.getDefault()
        try {
            L10n.setLanguagePreference("system")

            Locale.setDefault(Locale.US)
            val en = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.ENGLISH, en.kind)
            assertFalse("Smart Translation MUST be hidden/off for English", en.shouldShowSmartTranslation)
            assertEquals("en", en.bcp47Tag)

            Locale.setDefault(Locale.SIMPLIFIED_CHINESE)
            val zhCn = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE, zhCn.kind)
            assertTrue(zhCn.shouldShowSmartTranslation)
            assertEquals("zh-Hans", zhCn.bcp47Tag)

            Locale.setDefault(Locale.TAIWAN)
            val zhTw = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.TRADITIONAL_CHINESE, zhTw.kind)
            assertTrue(zhTw.shouldShowSmartTranslation)
            assertEquals("zh-Hant", zhTw.bcp47Tag)

            Locale.setDefault(Locale.JAPAN)
            val ja = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.JAPANESE, ja.kind)
            assertTrue(ja.shouldShowSmartTranslation)
            assertEquals("ja", ja.bcp47Tag)

            Locale.setDefault(Locale.FRANCE)
            val fr = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.OTHER_NON_ENGLISH, fr.kind)
            assertTrue(fr.shouldShowSmartTranslation)
            assertEquals("fr-FR", fr.bcp47Tag)
            assertEquals("French", fr.englishDisplayName)
            assertEquals(L10n.AppLang.EN, L10n.currentLang)

            Locale.setDefault(Locale.KOREA)
            val ko = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.OTHER_NON_ENGLISH, ko.kind)
            assertTrue(ko.shouldShowSmartTranslation)
            assertEquals("ko-KR", ko.bcp47Tag)
            assertEquals("Korean", ko.englishDisplayName)

            // Explicit app preference "zh" must also resolve to Simplified Chinese even on en-US OS
            Locale.setDefault(Locale.US)
            L10n.setLanguagePreference("zh")
            val forcedZh = com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.current()
            assertEquals(com.antigravity.mobile.data.service.DeviceTitleTargetLanguage.Kind.SIMPLIFIED_CHINESE, forcedZh.kind)
            assertEquals("zh-Hans", forcedZh.bcp47Tag)
        } finally {
            Locale.setDefault(prevLocale)
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun testAutomationScheduleSummaryLocaleFormatting() {
        val prevLocale = Locale.getDefault()
        try {
            L10n.setLanguagePreference("system")
            Locale.setDefault(Locale.FRANCE)
            assertEquals(
                "Mon, Wed, Fri 09:05",
                com.antigravity.mobile.ui.components.AutomationCronHelper.humanSummary("5 9 * * 1,3,5")
            )
            Locale.setDefault(Locale.SIMPLIFIED_CHINESE)
            assertEquals(
                "周一、周三、周五 09:05",
                com.antigravity.mobile.ui.components.AutomationCronHelper.humanSummary("5 9 * * 1,3,5")
            )
        } finally {
            Locale.setDefault(prevLocale)
            L10n.setLanguagePreference("system")
        }
    }
}
