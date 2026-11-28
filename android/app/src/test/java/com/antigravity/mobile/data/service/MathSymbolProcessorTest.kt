package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.components.MarkdownBlock
import com.antigravity.mobile.ui.components.MarkdownParser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MathSymbolProcessorTest {

    @Test
    fun `subscript 8 maps to subscript eight instead of superscript`() {
        assertEquals("x₈", MathSymbolProcessor.cleanMathExpression("x_8"))
        assertEquals("a₀₁₈₉", MathSymbolProcessor.cleanMathExpression("a_{0189}"))
        assertEquals("x⁸", MathSymbolProcessor.cleanMathExpression("x^8"))
    }

    @Test
    fun `fraction fallback parenthesizes compound numerator and denominator`() {
        assertEquals("(a+b) / (c+d)", MathSymbolProcessor.cleanMathExpression("\\frac{a+b}{c+d}"))
        assertEquals("1 / 2", MathSymbolProcessor.cleanMathExpression("\\frac{1}{2}"))
        assertEquals("x / (n+1)", MathSymbolProcessor.cleanMathExpression("\\frac{x}{n+1}"))
    }

    @Test
    fun `currency amounts are not mistaken for inline math`() {
        val text = "预算从 $10 涨到 $20，总计 $100 to $200，另有 \\$50 补贴。"
        val segments = MathSymbolProcessor.splitInlineMathPreservingCodeSpans(text)
        assertTrue(segments.none { it is MathSymbolProcessor.InlineMathSegment.Math })
        assertEquals(text, MathSymbolProcessor.process(text))
    }

    @Test
    fun `inline code spans protect dollar signs and latex commands`() {
        val text = "源码 `\$\\frac{a}{b}\$` 与正常公式 \$\\alpha + \\beta\$"
        val segments = MathSymbolProcessor.splitInlineMathPreservingCodeSpans(text)
        val mathSegments = segments.filterIsInstance<MathSymbolProcessor.InlineMathSegment.Math>()
        assertEquals(1, mathSegments.size)
        assertEquals("\\alpha + \\beta", mathSegments[0].latex)
    }

    @Test
    fun `needsRichMathRendering separates simple inline symbols from 2D formulas`() {
        assertFalse(MathSymbolProcessor.needsRichMathRendering("x^2"))
        assertFalse(MathSymbolProcessor.needsRichMathRendering("\\alpha + \\beta"))
        assertFalse(MathSymbolProcessor.needsRichMathRendering("O(n \\log n)"))

        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\frac{a+b}{c+d}"))
        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\sqrt{x^2 + 1}"))
        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\sum_{i=1}^{n} x_i"))
        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\begin{pmatrix} 1 & 2 \\\\ 3 & 4 \\end{pmatrix}"))
        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\theta_{\\text{max}}"))
        assertTrue(MathSymbolProcessor.needsRichMathRendering("\\ce{H2O -> H+ + OH-}"))
    }

    @Test
    fun `markdown parser emits MathBlock for single-line and multi-line display formulas`() {
        val md = """
            说明文字
            $$ E = mc^2 $$

            $$
            \mathcal{L}(\theta) = \frac{1}{N} \sum_{i=1}^{N} (y_i - \hat{y}_i)^2
            $$

            \[
            A = \begin{bmatrix} 1 & 0 \\ 0 & 1 \end{bmatrix}
            \]
        """.trimIndent()

        val blocks = MarkdownParser.parse(md)
        val mathBlocks = blocks.filterIsInstance<MarkdownBlock.MathBlock>()
        assertEquals(3, mathBlocks.size)
        assertEquals("E = mc^2", mathBlocks[0].latex)
        assertTrue(mathBlocks[1].latex.contains("\\mathcal{L}(\\theta)"))
        assertTrue(mathBlocks[2].latex.contains("\\begin{bmatrix}"))
    }
}
