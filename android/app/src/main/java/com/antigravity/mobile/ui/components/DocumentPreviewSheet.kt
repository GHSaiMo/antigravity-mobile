package com.antigravity.mobile.ui.components

import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.pdf.PdfRenderer
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.util.Base64
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
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
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.core.content.FileProvider
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * Android Document & Code Native Preview Sheet (1:1 with iOS QuickLookPreviewSheet & HTMLPreviewSheet).
 * Supports:
 * - HTML files via local WebView
 * - PDF documents via native Android PdfRenderer
 * - Code & plain text via scrollable monospaced viewer
 * - Office presentations & spreadsheets (PPTX, DOCX, XLSX) via external app launch and file sharing
 */
@Composable
fun DocumentPreviewSheet(
    file: File,
    title: String,
    onDismiss: VoidHandler,
    colors: AppColors = AntigravityTheme.colors
) {
    val context = LocalContext.current
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

    Dialog(
        onDismissRequest = { onDismiss() },
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = colors.background
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Top Header Bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .statusBarsPadding()
                        .padding(horizontal = 8.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    IconButton(onClick = {
                        haptic.light()
                        onDismiss()
                    }) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = "关闭",
                            tint = colors.textPrimary
                        )
                    }

                    Text(
                        text = decodedTitle,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier
                            .weight(1f)
                            .padding(horizontal = 8.dp),
                        textAlign = TextAlign.Center
                    )

                    // Share Button (matches iOS single action button)
                    IconButton(onClick = {
                        haptic.medium()
                        shareDocument(context, file, decodedTitle)
                    }) {
                        Icon(
                            imageVector = Icons.Default.Share,
                            contentDescription = "分享文件",
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }

                HorizontalDivider(color = colors.border.copy(alpha = 0.4f), thickness = 0.8.dp)

                // Body content based on file extension
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                ) {
                    when (ext) {
                        "html", "htm" -> {
                            HtmlDocumentViewer(file = file)
                        }
                        "pptx", "ppt" -> {
                            PptxDocumentViewer(file = file, colors = colors)
                        }
                        "pdf" -> {
                            PdfDocumentViewer(file = file, colors = colors)
                        }
                        "txt", "json", "csv", "log", "xml", "yaml", "yml", "sh", "py", "js", "ts", "kt", "swift" -> {
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
}

typealias VoidHandler = () -> Unit

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
                    var htmlContent = file.readText(Charsets.UTF_8)
                    val isMarp = htmlContent.contains("data-marpit-svg", ignoreCase = true) ||
                            htmlContent.contains("bespoke-marp", ignoreCase = true)

                    if (isMarp) {
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
                                }
                                div#\:\$p, .bespoke-marp-parent {
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
                                .bespoke-marp-osc, .bespoke-progress-parent, .bespoke-marp-overview-header {
                                    display: none !important;
                                }
                            }
                            </style>
                        """.trimIndent()

                        htmlContent = if (htmlContent.contains("</head>", ignoreCase = true)) {
                            htmlContent.replaceFirst("</head>", "$mobileSlideStyle</head>", ignoreCase = true)
                        } else {
                            "$mobileSlideStyle$htmlContent"
                        }
                    } else {
                        if (!htmlContent.contains("viewport", ignoreCase = true)) {
                            val viewportMeta = """<meta name="viewport" content="width=device-width, initial-scale=1.0">"""
                            htmlContent = if (htmlContent.contains("<head>", ignoreCase = true)) {
                                htmlContent.replaceFirst("<head>", "<head>$viewportMeta", ignoreCase = true)
                            } else {
                                "$viewportMeta$htmlContent"
                            }
                        }
                    }

                    val base64Data = Base64.encodeToString(htmlContent.toByteArray(Charsets.UTF_8), Base64.NO_WRAP)
                    loadDataWithBaseURL("https://localhost/", base64Data, "text/html; charset=utf-8", "base64", null)
                } catch (_: Exception) {
                    loadUrl("file://${file.absolutePath}")
                }
            }
        }
    )
}

private data class PptxSlide(
    val slideNumber: Int,
    val bitmap: Bitmap? = null,
    val title: String? = null,
    val paragraphs: List<String> = emptyList()
)

@Composable
private fun PptxDocumentViewer(file: File, colors: AppColors) {
    var slides by remember { mutableStateOf<List<PptxSlide>>(emptyList()) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(file) {
        withContext(Dispatchers.IO) {
            try {
                val zip = ZipFile(file)
                val entries = zip.entries().toList()
                val parsedSlides = mutableListOf<PptxSlide>()

                // 1. Check for extracted slide images in ppt/media/
                val slideImageRegex = Regex("""ppt/media/Slide-(\d+)-image-\d+\.(png|jpe?g|webp)""", RegexOption.IGNORE_CASE)
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
                    // Check for general images or thumbnails
                    val generalImageRegex = Regex("""ppt/media/image(\d+)\.(png|jpe?g|webp)""", RegexOption.IGNORE_CASE)
                    val genericEntries = entries.filter { generalImageRegex.find(it.name) != null }
                        .sortedBy { generalImageRegex.find(it.name)?.groupValues?.get(1)?.toIntOrNull() ?: 999 }

                    if (genericEntries.isNotEmpty()) {
                        genericEntries.forEachIndexed { idx, entry ->
                            val bytes = zip.getInputStream(entry).use { it.readBytes() }
                            val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            if (bmp != null) {
                                parsedSlides.add(PptxSlide(slideNumber = idx + 1, bitmap = bmp))
                            }
                        }
                    } else {
                        // Check for docProps/thumbnail
                        val thumb = entries.firstOrNull { it.name.contains("thumbnail", ignoreCase = true) }
                        if (thumb != null) {
                            val bytes = zip.getInputStream(thumb).use { it.readBytes() }
                            val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            if (bmp != null) {
                                parsedSlides.add(PptxSlide(slideNumber = 1, bitmap = bmp))
                            }
                        }
                    }
                }

                // 2. If no slide images found, parse text from ppt/slides/slide{N}.xml
                if (parsedSlides.isEmpty()) {
                    val slideXmlRegex = Regex("""ppt/slides/slide(\d+)\.xml""", RegexOption.IGNORE_CASE)
                    val xmlEntries = entries.filter { slideXmlRegex.matches(it.name) }
                        .sortedBy { slideXmlRegex.find(it.name)?.groupValues?.get(1)?.toIntOrNull() ?: 999 }

                    val textRegex = Regex("""<a:t[^>]*>(.*?)</a:t>""")
                    for (xmlEntry in xmlEntries) {
                        val num = slideXmlRegex.find(xmlEntry.name)?.groupValues?.get(1)?.toIntOrNull() ?: (parsedSlides.size + 1)
                        val textContent = zip.getInputStream(xmlEntry).use { it.bufferedReader(Charsets.UTF_8).readText() }
                        val matches = textRegex.findAll(textContent).map { it.groupValues[1].trim() }.filter { it.isNotBlank() }.toList()
                        if (matches.isNotEmpty()) {
                            val title = matches.firstOrNull() ?: "幻灯片 $num"
                            val paragraphs = if (matches.size > 1) matches.subList(1, matches.size) else emptyList()
                            parsedSlides.add(PptxSlide(slideNumber = num, title = title, paragraphs = paragraphs))
                        }
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
            itemsIndexed(slides) { index, slide ->
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
                                .clip(RoundedCornerShape(8.dp))
                                .border(0.5.dp, colors.border, RoundedCornerShape(8.dp))
                        )
                    } else {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surfaceVariant.copy(alpha = 0.5f))
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .padding(16.dp)
                        ) {
                            Text(
                                text = slide.title ?: "幻灯片 ${slide.slideNumber}",
                                color = colors.textPrimary,
                                fontSize = 16.sp,
                                fontWeight = FontWeight.Bold
                            )
                            Spacer(modifier = Modifier.height(8.dp))
                            slide.paragraphs.forEach { p ->
                                Text(
                                    text = "• $p",
                                    color = colors.textSecondary,
                                    fontSize = 13.5.sp,
                                    lineHeight = 18.sp,
                                    modifier = Modifier.padding(vertical = 2.dp)
                                )
                            }
                        }
                    }
                    Text(
                        text = "第 ${slide.slideNumber} / ${slides.size} 页",
                        color = colors.textMuted,
                        fontSize = 11.5.sp,
                        modifier = Modifier.padding(top = 4.dp)
                    )
                }
            }
        }
    }
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
                    // Render page into high-res bitmap
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
                shareDocument(context, file, file.name)
            },
            shape = RoundedCornerShape(10.dp),
            modifier = Modifier.fillMaxWidth(0.75f)
        ) {
            Icon(
                imageVector = Icons.Default.Share,
                contentDescription = null,
                modifier = Modifier.size(16.dp)
            )
            Spacer(modifier = Modifier.width(8.dp))
            Text(text = "分享文件", fontSize = 14.sp)
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
        "txt", "log", "csv" -> "text/plain"
        "json" -> "application/json"
        "zip" -> "application/zip"
        else -> "*/*"
    }
}
