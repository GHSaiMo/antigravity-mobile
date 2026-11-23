package com.antigravity.mobile.ui.components

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class GenerativeUIHtmlHelperTest {

    @Test
    fun standaloneGenerativeUiDashboardInjectsAllSemanticTokensAndRootClass() {
        val input = """
            <!DOCTYPE html>
            <html lang="zh-CN">
            <head>
              <meta charset="UTF-8" />
              <meta name="viewport" content="width=device-width, initial-scale=1.0" />
              <script src="https://www.gstatic.com/antigravity/web/dev/tailwindcss.min.js"></script>
              <style>.tab-btn { color: var(--primary); }</style>
            </head>
            <body class="bg-[var(--background)] text-[var(--foreground)]">
              <svg><rect fill="var(--background)" stroke="var(--chart-1)" /></svg>
            </body>
            </html>
        """.trimIndent()

        val prepared = GenerativeUIHtmlHelper.prepareStandaloneHtml(input, isDark = true)
        assertTrue(prepared.isGenerativeUi)
        assertTrue(prepared.html.contains("""<html class="dark" lang="zh-CN">"""))
        // Must NOT corrupt descendant class attributes
        assertTrue(prepared.html.contains("""<body class="bg-[var(--background)] text-[var(--foreground)]">"""))
        assertFalse(prepared.html.contains("""class="dark bg-"""))
        // Charset must remain before injected style
        val charsetIdx = prepared.html.indexOf("""<meta charset="UTF-8" />""")
        val styleIdx = prepared.html.indexOf("""<style id="antigravity-theme-vars">""")
        val pageStyleIdx = prepared.html.indexOf("""<style>.tab-btn""")
        assertTrue(charsetIdx in 0 until styleIdx)
        assertTrue(styleIdx < pageStyleIdx)

        // All official Generative UI tokens present
        val requiredTokens = listOf(
            "--background:", "--content:", "--card:", "--sidebar:",
            "--card-border:", "--border:",
            "--foreground:", "--muted-foreground:", "--placeholder:", "--code-foreground:",
            "--primary:", "--primary-foreground:", "--secondary:", "--secondary-foreground:",
            "--accent:", "--link:", "--success:", "--warning:", "--error:",
            "--chart-1:", "--chart-2:", "--chart-3:", "--chart-4:", "--chart-5:", "--chart-6:",
            "color-scheme: dark;"
        )
        for (token in requiredTokens) {
            assertTrue("Missing token $token", prepared.html.contains(token))
        }
    }

    @Test
    fun plainLegacyHtmlDoesNotForceDarkContainerTheme() {
        val input = """
            <!DOCTYPE html>
            <html>
            <head><title>Legacy Report</title><style>body { color: #222; }</style></head>
            <body><h1>Hello</h1></body>
            </html>
        """.trimIndent()

        val prepared = GenerativeUIHtmlHelper.prepareStandaloneHtml(input, isDark = true)
        assertFalse(prepared.isGenerativeUi)
        assertFalse(prepared.html.contains("color-scheme: dark"))
        assertFalse(prepared.html.contains("""<html class="dark""""))
        // Still gets viewport meta and safe light :root variable fallback
        assertTrue(prepared.html.contains("""name="viewport""""))
        assertTrue(prepared.html.contains("--chart-1:"))
    }

    @Test
    fun marpSlidesAreNeverTreatedAsGenerativeUi() {
        val input = """
            <!DOCTYPE html>
            <html><head></head><body><svg data-marpit-svg="" style="color: var(--foreground)"></svg></body></html>
        """.trimIndent()
        assertFalse(GenerativeUIHtmlHelper.usesAntigravityTheme(input))
    }

    @Test
    fun applyHtmlRootThemeClassMergesExistingHtmlClassesCleanly() {
        val input = """<html lang="en" class="light antialiased"><body class="card"></body></html>"""
        val updated = GenerativeUIHtmlHelper.applyHtmlRootThemeClass(input, isDark = true)
        assertTrue(updated.contains("""<html lang="en" class="dark antialiased">"""))
        assertTrue(updated.contains("""<body class="card">"""))
    }

    @Test
    fun inlineEmbedStripsConflictingViewportAndDoesNotForceMaxWidthImportantOnSvg() {
        val input = """
            <!DOCTYPE html>
            <html lang="zh-CN">
            <head>
              <meta charset="UTF-8">
              <meta name="viewport" content="width=1200">
            </head>
            <body class="bg-transparent text-[var(--foreground)]"><div>hi</div></body>
            </html>
        """.trimIndent()

        val built = GenerativeUIHtmlHelper.prepareInlineEmbedHtml(
            html = input,
            isDark = false,
            viewportMeta = """<meta name="viewport" content="width=device-width, initial-scale=1.0">""",
            reporterScript = "<script>/*ok*/</script>"
        )
        assertTrue(built.contains("""<html class="light" lang="zh-CN">"""))
        assertFalse(built.contains("width=1200"))
        assertFalse(built.contains("max-width: 100% !important"))
        assertFalse(built.contains("background: transparent !important"))
        assertTrue(built.contains("""<body class="bg-transparent text-[var(--foreground)]">"""))
    }
}
