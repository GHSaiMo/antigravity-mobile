package com.antigravity.mobile.ui.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownExportUtilsTest {
    @Test
    fun stripsPathAndReservedCharacters() {
        val name = MarkdownExportUtils.safeFileName("../../etc/pass:wd*?\"<>|")
        assertFalse(name.contains('/'))
        assertFalse(name.contains(':'))
        assertFalse(name.startsWith("."))
    }

    @Test
    fun keepsChineseTitles() {
        assertEquals("独立站点数据分析", MarkdownExportUtils.safeFileName("独立站点数据分析"))
    }

    @Test
    fun emptyOrDotTitlesFallBack() {
        assertEquals("conversation", MarkdownExportUtils.safeFileName(""))
        assertEquals("conversation", MarkdownExportUtils.safeFileName("   "))
        assertEquals("conversation", MarkdownExportUtils.safeFileName("..."))
    }

    @Test
    fun longTitlesAreTruncated() {
        assertTrue(MarkdownExportUtils.safeFileName("a".repeat(500)).length <= 60)
    }
}
