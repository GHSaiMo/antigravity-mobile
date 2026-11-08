package com.antigravity.mobile.ui.components

import com.antigravity.mobile.data.service.MathSymbolProcessor
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

    @Test
    fun testMathSymbolProcessor_underbraceLangleAndMathcal() {
        // Case 1: Screenshot 1 — \underbrace with \text{...} subscript and no underscore leak
        val oopFormula = """$$\text{OOP: } \underbrace{\text{subject}}_{\text{主语}}.\underbrace{\text{verb}}_{\text{谓语}}(\underbrace{\text{object}}_{\text{宾语}}) \xleftrightarrow{\text{编译器完全等价改写}} \text{Procedural: } \underbrace{\text{verb}}_{\text{谓语}}(\underbrace{\text{subject}}_{\text{主语 self/this}}, \underbrace{\text{object}}_{\text{宾语}})$$"""
        val processed1 = MathSymbolProcessor.process(oopFormula)
        assertFalse("Should not contain raw \\underbrace", processed1.contains("\\underbrace"))
        assertTrue("Should render subject〔主语〕", processed1.contains("subject〔主语〕.verb〔谓语〕(object〔宾语〕)"))
        assertTrue("Should render xleftrightarrow", processed1.contains("<──(编译器完全等价改写)──>"))
        assertTrue("Should render Procedural underbrace", processed1.contains("verb〔谓语〕(subject〔主语 self/this〕, object〔宾语〕)"))

        // Also test when _{...} was already stripped to {主语} in bare prose
        val bareUnderbrace = """OOP: \underbrace{subject}{主语}.\underbrace{verb}{谓语}(\underbrace{object}{宾语})"""
        val processedBare = MathSymbolProcessor.process(bareUnderbrace)
        assertEquals("OOP: subject〔主语〕.verb〔谓语〕(object〔宾语〕)", processedBare)

        // Case 2: Screenshot 2 — \langle ... \rangle and \mathcal{C}
        val categoryText = """\langle Subject (主语), Predicate (谓语), Object (宾语) \rangle 范畴 \mathcal{C}："""
        val processed2 = MathSymbolProcessor.process(categoryText)
        assertEquals("⟨ Subject (主语), Predicate (谓语), Object (宾语) ⟩ 范畴 𝒞：", processed2)

        // Case 3: Screenshot 3 — \underbrace with \xrightarrow
        val eventSchema = """$$\underbrace{\text{主语 (Subject / Agent)}}_{\text{能量源头 / 起始实体 } u} \xrightarrow{\text{谓语 (Verb / Predicate: 关系边或状态算子 } f\text{)}} \underbrace{\text{宾语 (Object / Patient)}}_{\text{能量落点 / 目标实体 } v}$$"""
        val processed3 = MathSymbolProcessor.process(eventSchema)
        assertFalse("Should not contain raw \\underbrace", processed3.contains("\\underbrace"))
        assertTrue(processed3.contains("主语 (Subject / Agent)〔能量源头 / 起始实体 u〕"))
        assertTrue(processed3.contains("宾语 (Object / Patient)〔能量落点 / 目标实体 v〕"))
    }
}
