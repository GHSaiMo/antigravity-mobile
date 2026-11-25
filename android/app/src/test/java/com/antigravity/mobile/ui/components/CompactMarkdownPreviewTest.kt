package com.antigravity.mobile.ui.components

import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.DarkAppColors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CompactMarkdownPreviewTest {

    @Test
    fun rendersCjkBoldWithoutRawAsterisks() {
        val raw = "一句话定性：**这是一场由南京市秦淮区（白下高新区）政府园区主导的“以赛引才、以赛招商”型早期创业孵化赛**（属于 2026 首届..."
        val annotated = buildCompactMarkdownPreview(raw, DarkAppColors, 12.sp)
        assertFalse(annotated.text.contains("**"))
        assertTrue(annotated.text.contains("这是一场由南京市秦淮区"))
        assertTrue(annotated.spanStyles.any { it.item.fontWeight == FontWeight.Bold })
    }

    @Test
    fun stripsBlockPrefixesAndRepairsTruncatedBold() {
        val normalized = normalizeCompactPreviewMarkdown("### 核心结论\n- **这是一段被截断的加粗…")
        assertEquals("核心结论 • **这是一段被截断的加粗**…", normalized)

        val annotated = buildCompactMarkdownPreview(normalized, DarkAppColors, 12.sp)
        assertEquals("核心结论 • 这是一段被截断的加粗…", annotated.text)
        assertTrue(annotated.spanStyles.any { it.item.fontWeight == FontWeight.Bold })
    }

    @Test
    fun rendersInlineCodeAndNonInteractiveLinksCleanly() {
        val raw = "调用 `compactSidePreview` 并查看 [方案文档](https://example.com/doc)"
        val annotated = buildCompactMarkdownPreview(raw, DarkAppColors, 12.sp)
        assertEquals("调用 compactSidePreview 并查看 方案文档", annotated.text)
        assertTrue(annotated.spanStyles.any { it.item.fontFamily == FontFamily.Monospace })
        assertTrue(annotated.getStringAnnotations(tag = "URL", start = 0, end = annotated.length).isEmpty())
    }

    @Test
    fun repairsBrokenTrailingLink() {
        val normalized = normalizeCompactPreviewMarkdown("参见 [秦淮方案](https://example.com/very/lo…")
        assertEquals("参见 秦淮方案…", normalized)
    }
}
