package com.antigravity.mobile.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SwarmMarkdownTest {

    @Test
    fun parsesBoardMessageHeadingInlineCodeAndNestedBullets() {
        val boardMsg = """
            用户本地试用后反馈了 3 项高价值移动端交互与体验优化（同时配合`seed-curator`将课程扩充至 8 阶段 128 单元），请在`src/**/*`内完成以下升级（严禁触碰`server/`、`data/`、`shared/`，不要`git commit`）：

            ### 1. 根治手账及全站搜索栏图标遮挡文字（Bug 修复）
            - 根因：`src/styles/index.css`中的`.washi-input`（以及`.washi-card`）未包裹在`@layer components { ... }`中，在 Tailwind v4 下覆盖了`pl-10`工具类，导致`<Search>`图标直接压在输入文字和占位符上！
            - 修复：
              - 将`src/styles/index.css`中上述组件类统一移入`@layer components { ... }`；
              - 检查`NotebookPage.tsx`及全站所有带`<Search>`的输入框：为`<Search>`加`pointer-events-none`。
        """.trimIndent()

        val blocks = MarkdownParser.parse(boardMsg)
        assertEquals(3, blocks.size)
        assertTrue(blocks[0] is MarkdownBlock.Paragraph)

        val heading = blocks[1] as MarkdownBlock.Heading
        assertEquals(3, heading.level)
        assertEquals("1. 根治手账及全站搜索栏图标遮挡文字（Bug 修复）", heading.text)

        val list = blocks[2] as MarkdownBlock.BulletList
        assertEquals(4, list.items.size)
        assertEquals(listOf(0, 0, 1, 1), list.indents)
        assertTrue(list.items[0].contains("`<Search>`"))

        val plain = MarkdownPlainText.convert(boardMsg)
        assertFalse(plain.contains("### "))
        assertTrue(plain.contains("• 修复：\n  • 将"))
        assertTrue(plain.contains("<Search>"))
    }

    @Test
    fun collapsedPreviewPreservesShortTextAndClosesOpenCodeFence() {
        assertFalse(needsSwarmBoardClamp("短消息 `ok`"))
        assertTrue(needsSwarmBoardClamp("```kotlin\nval x = 1\n```"))

        val longWithFence = (1..20).joinToString("\n") { i ->
            when (i) {
                1 -> "### 标题"
                2 -> "```ts"
                else -> "const line$i = $i;"
            }
        } + "\n```"

        val preview = buildSwarmCollapsedMarkdownPreview(longWithFence, maxLines = 6, maxChars = 400)
        val fenceCount = preview.lines().count { it.trim().startsWith("```") }
        assertEquals(2, fenceCount)
        assertTrue(preview.trimEnd().endsWith("```"))
    }
}
