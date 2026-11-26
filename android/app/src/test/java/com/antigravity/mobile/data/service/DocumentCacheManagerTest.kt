package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.components.resolveExportFileName
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import java.io.File
import org.junit.Test

class DocumentCacheManagerTest {

    @Test
    fun parenthesizedLinkTitleDoesNotCorruptHtmlExtension() {
        val uri = "file:///Users/hal9000/.gemini/antigravity/brain/88bda5f3-3db3-4565-a88a-01870a1e76a4/relay_pool_dashboard.html"
        val linkLabel = "打开 Multigravity 全球中继节点总体面板 (relay_pool_dashboard.html)"

        val cachedName = DocumentCacheManager.buildCacheFileName(uri, linkLabel, "88bda5f3-3db3-4565-a88a-01870a1e76a4")
        assertTrue("Expected .html extension, got: $cachedName", cachedName.endsWith(".html"))
        assertFalse("Must not end with .html)", cachedName.endsWith(".html)"))
        assertEquals("html", File(cachedName).extension.lowercase())
        assertTrue("Expected real base name from URI, got: $cachedName", cachedName.startsWith("relay_pool_dashboard_"))
    }

    @Test
    fun fullWidthParenthesizedOrDottedTitlesPreserveRealDocumentExtension() {
        val pdfUri = "file:///tmp/macro_report.pdf"
        val pdfName = DocumentCacheManager.buildCacheFileName(pdfUri, "2026.11.26 宏观报告（macro_report.pdf）")
        assertEquals("pdf", File(pdfName).extension.lowercase())

        val htmlUri = "https://gw.example.com/api/v1/files/raw?uri=%2FUsers%2Ftest%2Fdashboard.html&cascade_id=abc"
        val htmlName = DocumentCacheManager.buildCacheFileName(htmlUri, "v1.7 实时看板", "abc")
        assertEquals("html", File(htmlName).extension.lowercase())
        assertTrue(htmlName.startsWith("dashboard_"))
    }

    @Test
    fun fallsBackToFileNameExtensionWhenUriHasNoExtension() {
        val uri = "https://example.com/download?id=42"
        val name = DocumentCacheManager.buildCacheFileName(uri, "quarterly_report.pdf")
        assertEquals("pdf", File(name).extension.lowercase())
        assertTrue(name.startsWith("quarterly_report_"))
    }

    @Test
    fun fileIconResolverUnwrapsGatewayRawUriAndValidatesExtensions() {
        val rawUrl = "https://gw.example.com/api/v1/files/raw?uri=%2FUsers%2Ftest%2Frelay_pool_dashboard.html&cascade_id=abc"
        assertEquals("relay_pool_dashboard.html", FileIconResolver.extractFileNameFromUri(rawUrl))
        assertEquals("html", FileIconResolver.extractSafeExtension(rawUrl))
        assertTrue(FileIconResolver.isPreviewableDocument(rawUrl))
        assertTrue(FileIconResolver.isAccessibleDocument(rawUrl))

        assertEquals("", FileIconResolver.extractSafeExtension("打开面板 (relay_pool_dashboard.html)"))
        assertEquals("", FileIconResolver.extractSafeExtension("2026.11.26 报告"))
        assertEquals("html", FileIconResolver.extractSafeExtension("file:///tmp/c++.html#top"))
        assertEquals("c++.html", FileIconResolver.extractFileNameFromUri("file:///tmp/c++.html#top"))
    }

    @Test
    fun resolveExportFileNamePreservesRealExtensionForDescriptiveTitles() {
        val cached = File("/tmp/relay_pool_dashboard_b3eb8699.html")
        val exported = resolveExportFileName("打开 Multigravity 全球中继节点总体面板 (relay_pool_dashboard.html)", cached)
        assertTrue("Expected exported filename to end with .html, got: $exported", exported.endsWith(".html"))

        val plain = resolveExportFileName("relay_pool_dashboard.html", cached)
        assertEquals("relay_pool_dashboard.html", plain)
    }
}
