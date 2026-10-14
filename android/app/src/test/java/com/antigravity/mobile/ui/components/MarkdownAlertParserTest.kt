package com.antigravity.mobile.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownAlertParserTest {

    @Test
    fun testParseNoteWithListAndParagraphs() {
        val markdown = """
            已为你完整回滚本次所有配置修改，网络环境已恢复原状：

            > [!NOTE]
            > 关于依然报 ERROR 的排查参考:
            > 1. 账号进入了冷却风控期: 频率锁定。
            > 2. 账号注册地与 IP 国家不匹配。
            > 
            > 建议先将账号放置静默 24 小时以上。

            这是后续正文段落。
        """.trimIndent()

        val blocks = MarkdownParser.parse(markdown)
        assertEquals(3, blocks.size)

        // 1. First block is paragraph
        assertTrue(blocks[0] is MarkdownBlock.Paragraph)
        assertEquals("已为你完整回滚本次所有配置修改，网络环境已恢复原状：", (blocks[0] as MarkdownBlock.Paragraph).text)

        // 2. Second block is Alert Note
        assertTrue(blocks[1] is MarkdownBlock.Alert)
        val alert = blocks[1] as MarkdownBlock.Alert
        assertEquals(MarkdownAlertType.NOTE, alert.type)
        val expectedContent = """
            关于依然报 ERROR 的排查参考:
            1. 账号进入了冷却风控期: 频率锁定。
            2. 账号注册地与 IP 国家不匹配。

            建议先将账号放置静默 24 小时以上。
        """.trimIndent()
        assertEquals(expectedContent, alert.content)

        // 3. Third block is paragraph
        assertTrue(blocks[2] is MarkdownBlock.Paragraph)
        assertEquals("这是后续正文段落。", (blocks[2] as MarkdownBlock.Paragraph).text)
    }

    @Test
    fun testParseOtherAlertTypes() {
        val markdown = """
            > [!TIP]
            > 这是提示内容

            > [!IMPORTANT]
            > 这是重要说明

            > [!WARNING]
            > 这是警告内容

            > [!CAUTION]
            > 这是危险提醒
        """.trimIndent()

        val blocks = MarkdownParser.parse(markdown)
        assertEquals(4, blocks.size)

        assertEquals(MarkdownAlertType.TIP, (blocks[0] as MarkdownBlock.Alert).type)
        assertEquals("这是提示内容", (blocks[0] as MarkdownBlock.Alert).content)

        assertEquals(MarkdownAlertType.IMPORTANT, (blocks[1] as MarkdownBlock.Alert).type)
        assertEquals("这是重要说明", (blocks[1] as MarkdownBlock.Alert).content)

        assertEquals(MarkdownAlertType.WARNING, (blocks[2] as MarkdownBlock.Alert).type)
        assertEquals("这是警告内容", (blocks[2] as MarkdownBlock.Alert).content)

        assertEquals(MarkdownAlertType.CAUTION, (blocks[3] as MarkdownBlock.Alert).type)
        assertEquals("这是危险提醒", (blocks[3] as MarkdownBlock.Alert).content)
    }

    @Test
    fun testParseRegularBlockquote() {
        val markdown = """
            > 这是一段普通引用
            > 包含第二行文本
        """.trimIndent()

        val blocks = MarkdownParser.parse(markdown)
        assertEquals(1, blocks.size)
        assertTrue(blocks[0] is MarkdownBlock.Blockquote)
        val bq = blocks[0] as MarkdownBlock.Blockquote
        assertEquals("这是一段普通引用\n包含第二行文本", bq.content)
    }

    @Test
    fun testMarkdownPlainTextWithAlert() {
        val markdown = """
            > [!NOTE]
            > 关于依然报 ERROR 的排查参考:
            > 1. 账号进入了冷却风控期。
        """.trimIndent()

        val plain = MarkdownPlainText.convert(markdown)
        assertTrue(plain.contains("[Note]"))
        assertTrue(plain.contains("关于依然报 ERROR 的排查参考:"))
        assertTrue(plain.contains("1. 账号进入了冷却风控期。"))
    }
}
