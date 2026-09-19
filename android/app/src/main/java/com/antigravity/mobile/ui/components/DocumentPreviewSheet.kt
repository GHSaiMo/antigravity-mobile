package com.antigravity.mobile.ui.components

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.pdf.PdfRenderer
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.os.ParcelFileDescriptor
import android.provider.MediaStore
import android.util.Base64
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import java.io.File
import java.net.URLDecoder
import java.util.zip.ZipFile
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.FileProvider
import coil.compose.SubcomposeAsyncImage
import coil.decode.SvgDecoder
import coil.request.ImageRequest
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Android Document & Code Native Preview Sheet (1:1 aligned with iOS QuickLookPreviewSheet).
 * Features:
 * - Top-centered iOS-style drag handle indicator (36x5dp capsule).
 * - Top navigation bar: Top-left Apple native Save button, Center Document Title, Top-right Apple native Share button.
 * - Sheet container with 16dp rounded top corners.
 * - Supports SVG vector graphics (via Coil SVG), PPTX (multi-slide XML structure parser + theme),
 *   PDF (PdfRenderer), HTML/Marp (WebView), code & text, and Office docs (DOCX, XLSX).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DocumentPreviewSheet(
    file: File,
    title: String,
    onDismiss: VoidHandler,
    colors: AppColors = AntigravityTheme.colors
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    val ext = file.extension.lowercase()
    val decodedTitle = remember(title, file) {
        val raw = title.ifEmpty { file.name }
        try {
            URLDecoder.decode(raw, "UTF-8")
        } catch (_: Exception) {
            raw
        }
    }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    ModalBottomSheet(
        onDismissRequest = { onDismiss() },
        sheetState = sheetState,
        containerColor = colors.background,
        shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
        dragHandle = {
            Box(
                modifier = Modifier
                    .padding(top = 10.dp, bottom = 6.dp)
                    .width(36.dp)
                    .height(5.dp)
                    .clip(CircleShape)
                    .background(colors.textMuted.copy(alpha = 0.35f))
            )
        },
        modifier = Modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        Column(modifier = Modifier.fillMaxSize()) {
            // Top Navigation Bar (Apple Native Component Layout)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                // Top-left: Apple native Save button
                ApplePreviewCircleButton(
                    icon = Icons.Default.FileDownload,
                    contentDescription = "保存文件",
                    colors = colors,
                    onClick = {
                        haptic.light()
                        coroutineScope.launch {
                            saveFileToDownloads(context, file, decodedTitle)
                        }
                    }
                )

                // Center: Title
                Text(
                    text = decodedTitle,
                    color = colors.textPrimary,
                    fontSize = 16.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier
                        .weight(1f)
                        .padding(horizontal = 12.dp),
                    textAlign = TextAlign.Center
                )

                // Top-right: Apple native Share button
                ApplePreviewCircleButton(
                    icon = Icons.Default.Share,
                    contentDescription = "分享文件",
                    colors = colors,
                    onClick = {
                        haptic.medium()
                        shareDocument(context, file, decodedTitle)
                    }
                )
            }

            HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.8.dp)

            // Body content based on file extension
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
            ) {
                when (ext) {
                    "svg" -> {
                        SvgDocumentViewer(file = file, colors = colors)
                    }
                    "png", "jpg", "jpeg", "webp", "gif" -> {
                        ImageDocumentViewer(file = file, colors = colors)
                    }
                    "html", "htm" -> {
                        HtmlDocumentViewer(file = file)
                    }
                    "pptx", "ppt" -> {
                        PptxDocumentViewer(file = file, colors = colors)
                    }
                    "pdf" -> {
                        PdfDocumentViewer(file = file, colors = colors)
                    }
                    "txt", "json", "csv", "log", "xml", "yaml", "yml", "sh", "py", "js", "ts", "kt", "swift", "md" -> {
                        TextDocumentViewer(file = file, colors = colors)
                    }
                    else -> {
                        OfficeDocumentFallback(file = file, colors = colors)
                    }
                }
            }
        }
    }
}

typealias VoidHandler = () -> Unit

/**
 * Apple Native Component Style Circular Button (translucent frosted circle with subtle hairline border).
 */
@Composable
fun ApplePreviewCircleButton(
    icon: ImageVector,
    contentDescription: String,
    colors: AppColors,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    Surface(
        onClick = onClick,
        shape = CircleShape,
        color = colors.surfaceVariant.copy(alpha = 0.65f),
        border = androidx.compose.foundation.BorderStroke(0.5.dp, colors.border.copy(alpha = 0.35f)),
        modifier = modifier.size(36.dp)
    ) {
        Box(
            modifier = Modifier.fillMaxSize(),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = icon,
                contentDescription = contentDescription,
                tint = colors.textPrimary,
                modifier = Modifier.size(19.dp)
            )
        }
    }
}

/**
 * SVG Document Viewer using Coil with SvgDecoder.
 */
@Composable
private fun SvgDocumentViewer(file: File, colors: AppColors) {
    val context = LocalContext.current
    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        contentAlignment = Alignment.Center
    ) {
        SubcomposeAsyncImage(
            model = ImageRequest.Builder(context)
                .data(file)
                .decoderFactory(SvgDecoder.Factory())
                .crossfade(true)
                .build(),
            contentDescription = "SVG Preview",
            modifier = Modifier.fillMaxSize(),
            contentScale = ContentScale.Fit,
            loading = {
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = colors.accentIndigo, modifier = Modifier.size(32.dp))
                }
            },
            error = {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.BrokenImage,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(48.dp)
                    )
                    Spacer(modifier = Modifier.height(12.dp))
                    Text("SVG 矢量图加载失败", color = colors.textSecondary, fontSize = 14.sp)
                }
            }
        )
    }
}

/**
 * Image Viewer for standard image formats.
 */
@Composable
private fun ImageDocumentViewer(file: File, colors: AppColors) {
    val context = LocalContext.current
    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        contentAlignment = Alignment.Center
    ) {
        SubcomposeAsyncImage(
            model = ImageRequest.Builder(context)
                .data(file)
                .crossfade(true)
                .build(),
            contentDescription = "Image Preview",
            modifier = Modifier.fillMaxSize(),
            contentScale = ContentScale.Fit,
            loading = {
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = colors.accentIndigo, modifier = Modifier.size(32.dp))
                }
            },
            error = {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.BrokenImage,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(48.dp)
                    )
                    Spacer(modifier = Modifier.height(12.dp))
                    Text("图片加载失败", color = colors.textSecondary, fontSize = 14.sp)
                }
            }
        )
    }
}

@Composable
private fun HtmlDocumentViewer(file: File) {
    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { ctx ->
            WebView(ctx).apply {
                setBackgroundColor(android.graphics.Color.WHITE)
                webViewClient = WebViewClient()
                webChromeClient = WebChromeClient()
                settings.apply {
                    @Suppress("SetJavaScriptEnabled")
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    databaseEnabled = true
                    allowFileAccess = true
                    allowContentAccess = true
                    allowFileAccessFromFileURLs = true
                    allowUniversalAccessFromFileURLs = true
                    builtInZoomControls = true
                    displayZoomControls = false
                    useWideViewPort = true
                    loadWithOverviewMode = true
                }
                try {
                    var htmlContent = file.readText(Charsets.UTF_8).trim()
                    if (!htmlContent.startsWith("<") && (htmlContent.startsWith("PCFE") || htmlContent.startsWith("PD!"))) {
                        try {
                            val decoded = String(Base64.decode(htmlContent, Base64.DEFAULT), Charsets.UTF_8)
                            if (decoded.contains("<html", ignoreCase = true) || decoded.contains("<!DOCTYPE", ignoreCase = true)) {
                                htmlContent = decoded
                            }
                        } catch (_: Exception) {}
                    }

                    val isMarp = htmlContent.contains("data-marpit-svg", ignoreCase = true) ||
                            htmlContent.contains("bespoke-marp", ignoreCase = true) ||
                            htmlContent.contains("marpit", ignoreCase = true)

                    if (isMarp) {
                        setBackgroundColor(android.graphics.Color.parseColor("#F4F5F7"))
                        val mobileSlideStyle = """
                            <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=3.0, user-scalable=yes">
                            <style id="agy-mobile-slide-adapt">
                            @media screen {
                                html, body {
                                    overflow-y: auto !important;
                                    overflow-x: hidden !important;
                                    height: auto !important;
                                    min-height: 100% !important;
                                    background-color: #f4f5f7 !important;
                                    margin: 0 !important;
                                    padding: 0 !important;
                                    -webkit-overflow-scrolling: touch !important;
                                }
                                div#\:\${'$'}p, .bespoke-marp-parent {
                                    display: flex !important;
                                    flex-direction: column !important;
                                    align-items: center !important;
                                    padding: 16px 12px !important;
                                    gap: 16px !important;
                                    position: static !important;
                                    inset: auto !important;
                                    height: auto !important;
                                    overflow: visible !important;
                                }
                                svg[data-marpit-svg], svg.bespoke-marp-slide {
                                    display: block !important;
                                    width: 100% !important;
                                    max-width: 680px !important;
                                    height: auto !important;
                                    opacity: 1 !important;
                                    visibility: visible !important;
                                    content-visibility: visible !important;
                                    position: static !important;
                                    box-shadow: 0 4px 14px rgba(0,0,0,0.08) !important;
                                    border-radius: 10px !important;
                                    background: #ffffff !important;
                                    margin: 0 auto !important;
                                    transform: none !important;
                                    filter: none !important;
                                }
                                svg.bespoke-marp-slide * {
                                    visibility: visible !important;
                                }
                                [data-bespoke-marp-fragment] {
                                    visibility: visible !important;
                                    opacity: 1 !important;
                                }
                                .bespoke-marp-osc, .bespoke-progress-parent, .bespoke-marp-overview-header {
                                    display: none !important;
                                }
                            }
                            </style>
                            <script id="agy-mobile-guard">
                            try {
                                const noop = () => {};
                                window.history.replaceState = noop;
                                window.history.pushState = noop;
                            } catch (_) {}
                            const activateAll = () => {
                                try {
                                    document.querySelectorAll("svg[data-marpit-svg], svg.bespoke-marp-slide").forEach(s => {
                                        s.classList.add("bespoke-marp-active");
                                        s.removeAttribute("aria-hidden");
                                    });
                                    document.querySelectorAll("[data-bespoke-marp-fragment]").forEach(f => {
                                        f.setAttribute("data-bespoke-marp-fragment", "active");
                                    });
                                } catch (_) {}
                            };
                            activateAll();
                            window.addEventListener("DOMContentLoaded", activateAll);
                            window.addEventListener("load", () => {
                                activateAll();
                                setTimeout(activateAll, 100);
                                setTimeout(activateAll, 500);
                            });
                            </script>
                        """.trimIndent()

                        htmlContent = if (htmlContent.contains("</head>", ignoreCase = true)) {
                            htmlContent.replaceFirst("</head>", "$mobileSlideStyle</head>", ignoreCase = true)
                        } else {
                            "$mobileSlideStyle$htmlContent"
                        }
                    } else {
                        setBackgroundColor(android.graphics.Color.WHITE)
                        if (!htmlContent.contains("viewport", ignoreCase = true)) {
                            val viewportMeta = """<meta name="viewport" content="width=device-width, initial-scale=1.0">"""
                            htmlContent = if (htmlContent.contains("<head>", ignoreCase = true)) {
                                htmlContent.replaceFirst("<head>", "<head>$viewportMeta", ignoreCase = true)
                            } else {
                                "$viewportMeta$htmlContent"
                            }
                        }
                    }

                    val baseUrl = file.parentFile?.let { Uri.fromFile(it).toString() + "/" }
                        ?: Uri.fromFile(file).toString()
                    loadDataWithBaseURL(baseUrl, htmlContent, "text/html", "utf-8", null)
                } catch (_: Exception) {
                    try {
                        loadUrl(Uri.fromFile(file).toString())
                    } catch (_: Exception) {
                        loadUrl("file://${file.absolutePath}")
                    }
                }
            }
        }
    )
}

private data class PptxSlide(
    val slideNumber: Int,
    val bitmap: Bitmap? = null,
    val categoryBadge: String? = null,
    val title: String? = null,
    val subtitle: String? = null,
    val items: List<String> = emptyList(),
    val isDarkTheme: Boolean = true
)

/**
 * PPTX Document Viewer.
 * Solves the previous preview bug where a blank white template dummy thumbnail in docProps/thumbnail.jpeg
 * caused all slide XML parsing to be skipped.
 * Now parses all ppt/slides/slide{N}.xml numerically, extracts structured shapes, categories, titles,
 * subtitles, bullet items, and slide theme, and renders each as a 16:9 presentation slide card.
 */
@Composable
private fun PptxDocumentViewer(file: File, colors: AppColors) {
    val context = LocalContext.current
    val haptic = rememberHaptic()
    var slides by remember { mutableStateOf<List<PptxSlide>>(emptyList()) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(file) {
        withContext(Dispatchers.IO) {
            try {
                val parsedSlides = mutableListOf<PptxSlide>()
                val zip = ZipFile(file)
                val entries = zip.entries().asSequence().toList()

                // Check for slide-rendered image exports (e.g. ppt/media/slide_1.png)
                val slideImageRegex = Regex("""ppt/media/slide_?(\d+)\.(png|jpe?g|webp)""", RegexOption.IGNORE_CASE)
                val slideImageEntries = mutableMapOf<Int, java.util.zip.ZipEntry>()
                for (entry in entries) {
                    val match = slideImageRegex.find(entry.name)
                    if (match != null) {
                        val num = match.groupValues[1].toIntOrNull() ?: 1
                        if (!slideImageEntries.containsKey(num)) {
                            slideImageEntries[num] = entry
                        }
                    }
                }

                if (slideImageEntries.isNotEmpty()) {
                    // Render pure image slides if present
                    val sortedKeys = slideImageEntries.keys.sorted()
                    for (num in sortedKeys) {
                        val entry = slideImageEntries[num]!!
                        val bytes = zip.getInputStream(entry).use { it.readBytes() }
                        val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                        if (bmp != null) {
                            parsedSlides.add(PptxSlide(slideNumber = num, bitmap = bmp))
                        }
                    }
                } else {
                    // Parse structured XML slides from ppt/slides/slide{N}.xml (numerical order)
                    val slideXmlRegex = Regex("""ppt/slides/slide(\d+)\.xml""", RegexOption.IGNORE_CASE)
                    val xmlEntries = entries.filter { slideXmlRegex.matches(it.name) }
                        .sortedBy { slideXmlRegex.find(it.name)?.groupValues?.get(1)?.toIntOrNull() ?: 999 }

                    val shapeRegex = Regex("""<p:sp\b.*?</p:sp>""", RegexOption.DOTALL)
                    val textRegex = Regex("""<a:t\b[^>]*>(.*?)</a:t>""", RegexOption.DOTALL)
                    val darkHexRegex = Regex("""srgbClr val="(0B0F19|111827|1E293B|0A0F1D|000000|0F172A)""", RegexOption.IGNORE_CASE)

                    for (xmlEntry in xmlEntries) {
                        val slideNum = slideXmlRegex.find(xmlEntry.name)?.groupValues?.get(1)?.toIntOrNull()
                            ?: (parsedSlides.size + 1)
                        val xmlContent = zip.getInputStream(xmlEntry).use { it.bufferedReader(Charsets.UTF_8).readText() }

                        val isDark = darkHexRegex.containsMatchIn(xmlContent) || xmlContent.contains("0B0F19", ignoreCase = true)
                        val shapes = shapeRegex.findAll(xmlContent).map { it.value }.toList()
                        val texts = mutableListOf<String>()

                        for (shape in shapes) {
                            val matchedTexts = textRegex.findAll(shape).map { m ->
                                unescapeXml(m.groupValues[1].trim())
                            }.filter { it.isNotBlank() }.toList()

                            if (matchedTexts.isNotEmpty()) {
                                val joined = matchedTexts.joinToString(" ")
                                if (joined.isNotBlank()) {
                                    texts.add(joined)
                                }
                            }
                        }

                        var categoryBadge: String? = null
                        var slideTitle: String? = null
                        var subtitle: String? = null
                        var rem = texts.toList()

                        if (rem.isNotEmpty()) {
                            val first = rem.first()
                            // Determine if first shape is category/theme badge
                            if (first.length <= 32 && (first.all { it.isUpperCase() || it.isDigit() || it.isWhitespace() || it == '-' || it == '_' || it == '/' || it == ':' }
                                    || listOf("DEMO", "SYSTEM", "EVOLUTION", "CHALLENGES", "ARCHITECTURE", "OVERVIEW", "KEYNOTE").any { first.contains(it, ignoreCase = true) })) {
                                categoryBadge = first
                                rem = rem.drop(1)
                            }
                        }

                        if (rem.isNotEmpty()) {
                            slideTitle = rem.first()
                            rem = rem.drop(1)
                        }

                        if (rem.isNotEmpty()) {
                            val candidateSub = rem.first()
                            if (candidateSub.length <= 120 && !candidateSub.startsWith("01") && !candidateSub.startsWith("第一代") && !candidateSub.startsWith("•")) {
                                subtitle = candidateSub
                                rem = rem.drop(1)
                            }
                        }

                        // Filter out bottom footer boilerplate
                        val items = rem.filter { t ->
                            !listOf("PPT Master", "Tech Presentation", "Release: v", "Page 0", "• Page").any { t.contains(it, ignoreCase = true) }
                        }

                        parsedSlides.add(
                            PptxSlide(
                                slideNumber = slideNum,
                                categoryBadge = categoryBadge,
                                title = slideTitle ?: "幻灯片 $slideNum",
                                subtitle = subtitle,
                                items = items,
                                isDarkTheme = isDark
                            )
                        )
                    }
                }

                zip.close()

                withContext(Dispatchers.Main) {
                    slides = parsedSlides
                    isLoading = false
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) {
                    errorMessage = e.localizedMessage ?: "无法解析幻灯片"
                    isLoading = false
                }
            }
        }
    }

    if (isLoading) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(color = colors.accentIndigo, modifier = Modifier.size(32.dp))
        }
    } else if (errorMessage != null || slides.isEmpty()) {
        OfficeDocumentFallback(file = file, colors = colors, customMessage = errorMessage)
    } else {
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            itemsIndexed(slides) { _, slide ->
                Column(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    if (slide.bitmap != null) {
                        Image(
                            bitmap = slide.bitmap.asImageBitmap(),
                            contentDescription = "幻灯片 ${slide.slideNumber}",
                            contentScale = ContentScale.FillWidth,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                        )
                    } else {
                        // Presentation Slide Widescreen Card
                        val cardBg = if (slide.isDarkTheme) Color(0xFF0F172A) else colors.surfaceVariant.copy(alpha = 0.5f)
                        val cardBorder = if (slide.isDarkTheme) Color(0xFF334155) else colors.border
                        val titleColor = if (slide.isDarkTheme) Color(0xFFF8FAFC) else colors.textPrimary
                        val subColor = if (slide.isDarkTheme) Color(0xFF94A3B8) else colors.textSecondary
                        val textColor = if (slide.isDarkTheme) Color(0xFFE2E8F0) else colors.textPrimary

                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(14.dp))
                                .background(cardBg)
                                .border(0.8.dp, cardBorder, RoundedCornerShape(14.dp))
                                .padding(16.dp)
                        ) {
                            // Slide Top Bar
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                if (!slide.categoryBadge.isNullOrBlank()) {
                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = colors.accentIndigo.copy(alpha = 0.2f)
                                    ) {
                                        Text(
                                            text = slide.categoryBadge,
                                            color = colors.accentIndigo,
                                            fontSize = 10.sp,
                                            fontWeight = FontWeight.Bold,
                                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                        )
                                    }
                                } else {
                                    Spacer(modifier = Modifier.width(1.dp))
                                }

                                Text(
                                    text = String.format("%02d / %02d", slide.slideNumber, slides.size),
                                    color = colors.textMuted,
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    fontWeight = FontWeight.Medium
                                )
                            }

                            Spacer(modifier = Modifier.height(10.dp))

                            // Slide Title
                            Text(
                                text = slide.title ?: "幻灯片 ${slide.slideNumber}",
                                color = titleColor,
                                fontSize = 16.5.sp,
                                fontWeight = FontWeight.Bold,
                                lineHeight = 22.sp
                            )

                            // Slide Subtitle
                            if (!slide.subtitle.isNullOrBlank()) {
                                Spacer(modifier = Modifier.height(4.dp))
                                Text(
                                    text = slide.subtitle,
                                    color = subColor,
                                    fontSize = 12.5.sp,
                                    lineHeight = 17.sp
                                )
                            }

                            // Slide Items
                            if (slide.items.isNotEmpty()) {
                                Spacer(modifier = Modifier.height(12.dp))
                                HorizontalDivider(color = cardBorder.copy(alpha = 0.6f), thickness = 0.5.dp)
                                Spacer(modifier = Modifier.height(10.dp))

                                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                    slide.items.take(10).forEach { item ->
                                        Row(
                                            modifier = Modifier.fillMaxWidth(),
                                            verticalAlignment = Alignment.Top
                                        ) {
                                            Box(
                                                modifier = Modifier
                                                    .padding(top = 6.dp, end = 8.dp)
                                                    .size(5.dp)
                                                    .clip(CircleShape)
                                                    .background(colors.accentIndigo)
                                            )
                                            Text(
                                                text = item,
                                                color = textColor,
                                                fontSize = 12.sp,
                                                lineHeight = 16.5.sp
                                            )
                                        }
                                    }
                                    if (slide.items.size > 10) {
                                        Text(
                                            text = "• 还有 ${slide.items.size - 10} 项演示内容...",
                                            color = colors.textMuted,
                                            fontSize = 11.sp,
                                            modifier = Modifier.padding(top = 2.dp)
                                        )
                                    }
                                }
                            }
                        }
                    }

                    Text(
                        text = "第 ${slide.slideNumber} / ${slides.size} 页",
                        color = colors.textMuted,
                        fontSize = 11.5.sp,
                        modifier = Modifier.padding(top = 6.dp)
                    )
                }
            }

            // Bottom Action: Open in External App
            item {
                Spacer(modifier = Modifier.height(8.dp))
                Button(
                    onClick = {
                        haptic.medium()
                        openInExternalApp(context, file)
                    },
                    shape = RoundedCornerShape(10.dp),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.surfaceVariant,
                        contentColor = colors.textPrimary
                    ),
                    border = androidx.compose.foundation.BorderStroke(0.5.dp, colors.border),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Icon(
                        imageVector = Icons.Default.OpenInNew,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp)
                    )
                    Spacer(modifier = Modifier.width(8.dp))
                    Text(
                        text = "在第三方应用中打开 (WPS / Office 完整放映)",
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.Medium
                    )
                }
                Spacer(modifier = Modifier.height(16.dp))
            }
        }
    }
}

private fun unescapeXml(text: String): String {
    return text.replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&apos;", "'")
        .replace("&#39;", "'")
}

@Composable
private fun PdfDocumentViewer(file: File, colors: AppColors) {
    var pages by remember { mutableStateOf<List<Bitmap>>(emptyList()) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(file) {
        withContext(Dispatchers.IO) {
            try {
                val fileDescriptor = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
                val renderer = PdfRenderer(fileDescriptor)
                val bitmaps = mutableListOf<Bitmap>()
                val count = renderer.pageCount

                for (i in 0 until count) {
                    val page = renderer.openPage(i)
                    val width = page.width * 2
                    val height = page.height * 2
                    val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
                    bitmap.eraseColor(android.graphics.Color.WHITE)
                    page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                    page.close()
                    bitmaps.add(bitmap)
                }
                renderer.close()
                fileDescriptor.close()
                withContext(Dispatchers.Main) {
                    pages = bitmaps
                    isLoading = false
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) {
                    errorMessage = e.localizedMessage ?: "无法解析 PDF"
                    isLoading = false
                }
            }
        }
    }

    if (isLoading) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(color = colors.accentIndigo, modifier = Modifier.size(32.dp))
        }
    } else if (errorMessage != null) {
        OfficeDocumentFallback(file = file, colors = colors, customMessage = errorMessage)
    } else {
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            itemsIndexed(pages) { index, bitmap ->
                Column(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Image(
                        bitmap = bitmap.asImageBitmap(),
                        contentDescription = "Page ${index + 1}",
                        contentScale = ContentScale.FillWidth,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .border(0.5.dp, colors.border, RoundedCornerShape(8.dp))
                    )
                    Text(
                        text = "第 ${index + 1} / ${pages.size} 页",
                        color = colors.textMuted,
                        fontSize = 11.sp,
                        modifier = Modifier.padding(top = 4.dp)
                    )
                }
            }
        }
    }
}

@Composable
private fun TextDocumentViewer(file: File, colors: AppColors) {
    val content = remember(file) {
        try {
            file.readText()
        } catch (e: Exception) {
            "无法读取文件内容: ${e.message}"
        }
    }

    SelectionContainer {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .horizontalScroll(rememberScrollState())
                .padding(16.dp)
        ) {
            Text(
                text = content,
                color = colors.textPrimary,
                fontSize = 12.5.sp,
                fontFamily = FontFamily.Monospace,
                lineHeight = 18.sp
            )
        }
    }
}

@Composable
private fun OfficeDocumentFallback(
    file: File,
    colors: AppColors,
    customMessage: String? = null
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    val sizeStr = remember(file) {
        val bytes = file.length()
        if (bytes < 1024) "$bytes B"
        else if (bytes < 1024 * 1024) "${bytes / 1024} KB"
        else "%.1f MB".format(bytes.toFloat() / (1024 * 1024))
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Box(
            modifier = Modifier
                .size(72.dp)
                .clip(CircleShape)
                .background(colors.accentIndigo.copy(alpha = 0.12f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.Description,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(36.dp)
            )
        }

        Spacer(modifier = Modifier.height(16.dp))

        Text(
            text = file.name,
            color = colors.textPrimary,
            fontSize = 17.sp,
            fontWeight = FontWeight.Bold,
            textAlign = TextAlign.Center
        )

        Text(
            text = "文件大小: $sizeStr",
            color = colors.textMuted,
            fontSize = 13.sp,
            modifier = Modifier.padding(top = 4.dp)
        )

        if (customMessage != null) {
            Text(
                text = customMessage,
                color = Color(0xFFFF9800),
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 8.dp)
            )
        }

        Spacer(modifier = Modifier.height(28.dp))

        Button(
            onClick = {
                haptic.medium()
                openInExternalApp(context, file)
            },
            colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo),
            shape = RoundedCornerShape(10.dp),
            modifier = Modifier.fillMaxWidth(0.75f)
        ) {
            Icon(
                imageVector = Icons.Default.OpenInNew,
                contentDescription = null,
                modifier = Modifier.size(16.dp)
            )
            Spacer(modifier = Modifier.width(8.dp))
            Text(text = "在第三方应用中打开", fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
        }

        Spacer(modifier = Modifier.height(12.dp))

        OutlinedButton(
            onClick = {
                haptic.light()
                coroutineScope.launch {
                    saveFileToDownloads(context, file, file.name)
                }
            },
            shape = RoundedCornerShape(10.dp),
            modifier = Modifier.fillMaxWidth(0.75f)
        ) {
            Icon(
                imageVector = Icons.Default.FileDownload,
                contentDescription = null,
                modifier = Modifier.size(16.dp)
            )
            Spacer(modifier = Modifier.width(8.dp))
            Text(text = "保存到下载目录", fontSize = 14.sp)
        }
    }
}

private fun openInExternalApp(context: Context, file: File) {
    try {
        val uri = FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            file
        )
        val mimeType = getMimeType(file)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, mimeType)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(Intent.createChooser(intent, "打开文件"))
    } catch (e: Exception) {
        Toast.makeText(context, "未找到可打开此文件的应用: ${e.message}", Toast.LENGTH_SHORT).show()
    }
}

private fun shareDocument(context: Context, file: File, title: String) {
    try {
        val uri = FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            file
        )
        val mimeType = getMimeType(file)
        val shareIntent = Intent(Intent.ACTION_SEND).apply {
            type = mimeType
            putExtra(Intent.EXTRA_STREAM, uri)
            putExtra(Intent.EXTRA_SUBJECT, title)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        context.startActivity(Intent.createChooser(shareIntent, "分享文件"))
    } catch (e: Exception) {
        Toast.makeText(context, "分享失败: ${e.message}", Toast.LENGTH_SHORT).show()
    }
}

suspend fun saveFileToDownloads(context: Context, file: File, displayName: String) {
    withContext(Dispatchers.IO) {
        try {
            val mimeType = getMimeType(file)
            val isImage = mimeType.startsWith("image/")
            val resolver = context.contentResolver
            val contentValues = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, displayName)
                put(MediaStore.MediaColumns.MIME_TYPE, mimeType)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    val relativePath = if (isImage) {
                        Environment.DIRECTORY_PICTURES + "/Antigravity"
                    } else {
                        Environment.DIRECTORY_DOWNLOADS + "/Antigravity"
                    }
                    put(MediaStore.MediaColumns.RELATIVE_PATH, relativePath)
                    put(MediaStore.MediaColumns.IS_PENDING, 1)
                }
            }

            val collectionUri = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                if (isImage) MediaStore.Images.Media.EXTERNAL_CONTENT_URI else MediaStore.Downloads.EXTERNAL_CONTENT_URI
            } else {
                if (isImage) MediaStore.Images.Media.EXTERNAL_CONTENT_URI else MediaStore.Files.getContentUri("external")
            }

            val itemUri = resolver.insert(collectionUri, contentValues)
            if (itemUri != null) {
                resolver.openOutputStream(itemUri)?.use { out ->
                    file.inputStream().use { input ->
                        input.copyTo(out)
                    }
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    contentValues.clear()
                    contentValues.put(MediaStore.MediaColumns.IS_PENDING, 0)
                    resolver.update(itemUri, contentValues, null, null)
                }
                withContext(Dispatchers.Main) {
                    val destDesc = if (isImage) "相册" else "下载目录"
                    Toast.makeText(context, "已保存到$destDesc", Toast.LENGTH_SHORT).show()
                }
            } else {
                val targetDir = Environment.getExternalStoragePublicDirectory(
                    if (isImage) Environment.DIRECTORY_PICTURES else Environment.DIRECTORY_DOWNLOADS
                )
                val destFile = File(targetDir, displayName)
                file.copyTo(destFile, overwrite = true)
                withContext(Dispatchers.Main) {
                    val destDesc = if (isImage) "相册" else "下载目录"
                    Toast.makeText(context, "已保存到$destDesc", Toast.LENGTH_SHORT).show()
                }
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) {
                Toast.makeText(context, "保存失败: ${e.localizedMessage ?: e.message}", Toast.LENGTH_SHORT).show()
            }
        }
    }
}

private fun getMimeType(file: File): String {
    return when (file.extension.lowercase()) {
        "pdf" -> "application/pdf"
        "pptx" -> "application/vnd.openxmlformats-officedocument.presentationml.presentation"
        "ppt" -> "application/vnd.ms-powerpoint"
        "docx" -> "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
        "doc" -> "application/msword"
        "xlsx" -> "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        "xls" -> "application/vnd.ms-excel"
        "html", "htm" -> "text/html"
        "svg" -> "image/svg+xml"
        "png" -> "image/png"
        "jpg", "jpeg" -> "image/jpeg"
        "webp" -> "image/webp"
        "gif" -> "image/gif"
        "txt", "log", "csv" -> "text/plain"
        "json" -> "application/json"
        "md" -> "text/markdown"
        "zip" -> "application/zip"
        else -> "*/*"
    }
}
