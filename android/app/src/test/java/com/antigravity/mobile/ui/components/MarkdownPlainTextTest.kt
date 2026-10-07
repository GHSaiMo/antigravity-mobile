package com.antigravity.mobile.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownPlainTextTest {

    @Test
    fun testRestoreFormattedSelection_fullMessage() {
        val markdown = """
            已完成修改、提交并成功覆盖更新 v1.0.6 Release：

            📦 变更与提交明细
            Commit: 2778b24 fix(android): 修复移动屏幕文字选框消失与返回时复制按钮悬浮残留 (v1.0.6)
            • 分支推送: main -> main
            • Tag 覆盖: v1.0.6（已强制推送到远程仓库，覆盖旧有发布点）
            • Release 构建: GitHub Actions 工作流 Release Multigravity 已基于最新的 v1.0.6 tag 自动触发运行，完成后将自动刷新 GitHub Release 资产（含全新编译且保持固定签名的 Android APK）。
        """.trimIndent()

        val fullPlainText = MarkdownPlainText.convert(markdown)

        // Simulated Compose SelectionManager output when bullets are in DisableSelection:
        // blocks are merged without newlines between selectable Text components.
        val rawSelectedText = "已完成修改、提交并成功覆盖更新 v1.0.6 Release：📦 变更与提交明细Commit: 2778b24 fix(android): 修复移动屏幕文字选框消失与返回时复制按钮悬浮残留 (v1.0.6)分支推送: main -> mainTag 覆盖: v1.0.6（已强制推送到远程仓库，覆盖旧有发布点）Release 构建: GitHub Actions 工作流 Release Multigravity 已基于最新的 v1.0.6 tag 自动触发运行，完成后将自动刷新 GitHub Release 资产（含全新编译且保持固定签名的 Android APK）。"

        val restored = MarkdownPlainText.restoreFormattedSelection(rawSelectedText, fullPlainText)

        // 1. Should preserve double-newlines between paragraphs
        assertTrue("Should contain paragraph breaks", restored.contains("Release：\n\n📦 变更与提交明细"))

        // 2. Should restore bullet list items with proper bullets and newlines
        assertTrue("Should contain bullet for branch push", restored.contains("• 分支推送: main -> main"))
        assertTrue("Should contain bullet for tag coverage", restored.contains("• Tag 覆盖: v1.0.6"))
        assertTrue("Should contain bullet for release build", restored.contains("• Release 构建: GitHub Actions"))

        // 3. Should NOT end with a trailing bullet
        assertFalse("Must not end with trailing bullet", restored.trim().endsWith("•"))

        // 4. Should NOT end with an extra trailing newline
        assertFalse("Must not end with trailing newline", restored.endsWith("\n"))
    }

    @Test
    fun testRestoreFormattedSelection_shortWord() {
        val markdown = "Commit: 2778b24 fix(android)"
        val fullPlainText = MarkdownPlainText.convert(markdown)
        val shortWord = "2778b24"

        val restored = MarkdownPlainText.restoreFormattedSelection(shortWord, fullPlainText)
        assertEquals("2778b24", restored)
    }

    @Test
    fun testRestoreFormattedSelection_partialList() {
        val markdown = """
            任务清单：
            • 任务一完成
            • 任务二进行中
            • 任务三待办
        """.trimIndent()

        val fullPlainText = MarkdownPlainText.convert(markdown)

        // User selected only task 2 and task 3
        val rawSelection = "任务二进行中任务三待办"
        val restored = MarkdownPlainText.restoreFormattedSelection(rawSelection, fullPlainText)

        assertTrue(restored.contains("• 任务二进行中\n• 任务三待办"))
        assertFalse(restored.endsWith("•"))
    }
}
