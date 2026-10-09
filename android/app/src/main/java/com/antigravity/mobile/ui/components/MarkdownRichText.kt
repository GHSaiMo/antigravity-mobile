package com.antigravity.mobile.ui.components

import android.content.Intent
import android.net.Uri
import java.net.URLDecoder
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.TextLayoutResult
import com.antigravity.mobile.data.service.FileIconResolver
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import com.antigravity.mobile.data.service.MathSymbolProcessor
import com.antigravity.mobile.ui.theme.AppColors

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun ParagraphBlockView(
    text: String,
    colors: AppColors,
    onPlanClick: ((String, String) -> Unit)?
) {
    val cleanText = replaceHtmlBreaks(text).trim()
    val segments = remember(cleanText) { parsePlanSegments(cleanText) }

    if (segments.size <= 1 && segments.firstOrNull() !is PlanSegment.PlanButton) {
        RichTextRenderer(
            text = cleanText,
            colors = colors,
            baseFontSize = 15.sp,
            onPlanClick = onPlanClick
        )
    } else {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            segments.forEach { segment ->
                when (segment) {
                    is PlanSegment.Text -> {
                        val trimmed = segment.content.trim()
                        if (trimmed.isNotEmpty()) {
                            RichTextRenderer(
                                text = trimmed,
                                colors = colors,
                                baseFontSize = 15.sp,
                                onPlanClick = onPlanClick
                            )
                        }
                    }
                    is PlanSegment.PlanButton -> {
                        PlanButtonView(
                            title = segment.title,
                            uri = segment.uri,
                            colors = colors,
                            onClick = {
                                onPlanClick?.invoke(segment.uri, segment.title)
                            }
                        )
                    }
                }
            }
        }
    }
}

@Composable
fun FileIconSvgView(
    iconName: String,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    SubcomposeAsyncImage(
        model = ImageRequest.Builder(context)
            .data("file:///android_asset/file_icons/$iconName.svg")
            .crossfade(false)
            .build(),
        contentDescription = null,
        modifier = modifier,
        contentScale = ContentScale.Fit
    )
}

internal data class RichTextRenderData(
    val annotatedString: AnnotatedString,
    val inlineContent: Map<String, InlineTextContent>
)

/**
 * Parses and renders inline markdown formatting:
 * - Bold: `**text**` or `__text__`
 * - Italic: `*text*` or `_text_`
 * - Inline code: `` `code` `` in Desktop Amber (#E5C07B)
 * - Links: `[text](url)` with inline file icon SVG, compact monospace font, and script filtering
 * - LaTeX Arrows: `\to` -> `→`, etc.
 */
@Composable
internal fun RichTextRenderer(
    text: String,
    colors: AppColors,
    baseFontSize: TextUnit = 15.sp,
    baseFontWeight: FontWeight = FontWeight.Normal,
    onPlanClick: ((String, String) -> Unit)? = null
) {
    val context = LocalContext.current
    val renderData = remember(text, colors, baseFontSize, baseFontWeight) {
        buildRichTextRenderData(
            rawText = text,
            colors = colors,
            baseFontSize = baseFontSize,
            baseFontWeight = baseFontWeight
        )
    }

    var layoutResult by remember { mutableStateOf<TextLayoutResult?>(null) }

    Text(
        text = renderData.annotatedString,
        modifier = Modifier.registerTextHit(layoutResult).pointerInput(renderData.annotatedString) {
            detectTapGestures { pos ->
                layoutResult?.let { layout ->
                    val offset = layout.getOffsetForPosition(pos)
                    renderData.annotatedString.getStringAnnotations(tag = "URL", start = offset, end = offset)
                        .firstOrNull()?.let { annotation ->
                            val url = annotation.item
                            val titleAnnotation = renderData.annotatedString.getStringAnnotations(tag = "URL_TITLE", start = offset, end = offset).firstOrNull()
                            val rawTitle = titleAnnotation?.item?.ifBlank { url.substringAfterLast('/') } ?: url.substringAfterLast('/')
                            val decodedTitle = try {
                                URLDecoder.decode(rawTitle, "UTF-8")
                            } catch (_: Exception) {
                                rawTitle
                            }

                            // Script and code files do not need to be accessible (safety filter)
                            if (FileIconResolver.isScriptFile(url) || FileIconResolver.isScriptFile(decodedTitle)) {
                                return@let
                            }

                            if (onPlanClick != null && FileIconResolver.isAccessibleDocument(url)) {
                                onPlanClick(url, decodedTitle)
                            } else if (url.startsWith("http://") || url.startsWith("https://")) {
                                try {
                                    val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url)).apply {
                                        flags = Intent.FLAG_ACTIVITY_NEW_TASK
                                    }
                                    context.startActivity(intent)
                                } catch (_: Exception) {
                                    onPlanClick?.invoke(url, decodedTitle)
                                }
                            } else if (onPlanClick != null && !FileIconResolver.isScriptFile(url)) {
                                onPlanClick(url, decodedTitle)
                            }
                        }
                }
            }
        },
        style = TextStyle(
            color = colors.textPrimary,
            fontSize = baseFontSize,
            fontWeight = baseFontWeight,
            lineHeight = (baseFontSize.value * 1.45f).sp
        ),
        inlineContent = renderData.inlineContent,
        onTextLayout = { layoutResult = it }
    )
}

internal val BACKTICK_BOLD_REGEX = Regex("""`+(\*{2,3})\s*([^\*`\n]+?)\s*\1`+""")

internal fun unwrapBacktickBold(text: String): String {
    if (!text.contains('`') || !text.contains("**")) return text
    return BACKTICK_BOLD_REGEX.replace(text) { match ->
        val stars = match.groups[1]?.value.orEmpty()
        val inner = match.groups[2]?.value.orEmpty()
        "$stars$inner$stars"
    }
}

internal val BOLD_PAIR_REGEX = Regex("""(?<!\*)(\*{2,3})((?:[^\*]|\*(?!\*))+?)\1(?!\*)""")

internal fun normalizeBoldSpaces(text: String): String {
    if (!text.contains("**")) return text
    return BOLD_PAIR_REGEX.replace(text) { match ->
        val stars = match.groups[1]?.value.orEmpty()
        val inner = match.groups[2]?.value.orEmpty()
        val trimmed = inner.trim(' ', '\t')
        if (trimmed.isEmpty()) {
            match.value
        } else {
            val leading = inner.takeWhile { it == ' ' || it == '\t' }
            val trailing = inner.takeLastWhile { it == ' ' || it == '\t' }
            "$leading$stars$trimmed$stars$trailing"
        }
    }
}

internal val INLINE_TOKEN_REGEX = Regex(
    """(?<!\!)\[([^\]]+)\]\(([^)]+)\)|`([^`]+)`|\*\*\*([^\n]+?)\*\*\*|___([^\n]+?)___|\*\*([^\n]+?)\*\*|__([^\n]+?)__|(?<!\*)\*([^*\n]+?)\*(?!\*)|(?<!_)_([^_\n]+?)_(?!_)|~~([^\n]+?)~~"""
)

internal fun buildRichTextRenderData(
    rawText: String,
    colors: AppColors,
    baseFontSize: TextUnit,
    baseFontWeight: FontWeight
): RichTextRenderData {
    var processed = MathSymbolProcessor.process(rawText)
    processed = replaceHtmlBreaks(processed)
    processed = unwrapBacktickBold(processed)
    processed = normalizeBoldSpaces(processed)

    // Auto-link bare implementation_plan.md, walkthrough.md, task.md if not already in markdown link and outside code spans
    if (processed.contains("implementation_plan.md") || processed.contains("walkthrough.md") || processed.contains("task.md")) {
        val segments = splitCodeSpans(processed)
        val sb = StringBuilder()
        for ((content, isCode) in segments) {
            if (isCode) {
                sb.append(content)
            } else {
                var seg = content
                if (seg.contains("implementation_plan.md") && !seg.contains("[implementation_plan.md]") && !seg.contains("](implementation_plan.md)")) {
                    seg = seg.replace("implementation_plan.md", "[implementation_plan.md](implementation_plan.md)")
                }
                if (seg.contains("walkthrough.md") && !seg.contains("[walkthrough.md]") && !seg.contains("](walkthrough.md)")) {
                    seg = seg.replace("walkthrough.md", "[walkthrough.md](walkthrough.md)")
                }
                if (seg.contains("task.md") && !seg.contains("[task.md]") && !seg.contains("](task.md)")) {
                    seg = seg.replace("task.md", "[task.md](task.md)")
                }
                sb.append(seg)
            }
        }
        processed = sb.toString()
    }

    // Auto-link MEDIA: paths into clickable image links
    if (processed.contains("MEDIA:")) {
        val mediaPattern = Regex("""(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s\)\<\>\"\'\`]+)""", RegexOption.IGNORE_CASE)
        processed = mediaPattern.replace(processed) { match ->
            val rawPath = match.groups[1]?.value.orEmpty()
            val clean = rawPath.trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')
            val fn = clean.substringAfterLast('/')
            val linkTarget = if (clean.startsWith("file://") || clean.startsWith("http://") || clean.startsWith("https://")) clean else "file://$clean"
            "\n[点击放大查看图片 ($fn)]($linkTarget)"
        }
    }

    val inlineContentMap = mutableMapOf<String, InlineTextContent>()
    val codeColor = Color(0xFFE5C07B) // Desktop Amber
    var iconIndex = 0

    val annotatedString = buildAnnotatedString {
        fun appendStyledText(
            plain: String,
            isBold: Boolean,
            isItalic: Boolean,
            isStrike: Boolean
        ) {
            if (plain.isEmpty()) return
            val start = length
            append(plain)
            if (isBold || isItalic || isStrike) {
                addStyle(
                    SpanStyle(
                        fontWeight = if (isBold) FontWeight.Bold else baseFontWeight,
                        fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                        textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None
                    ),
                    start,
                    length
                )
            }
        }

        fun appendInline(
            text: String,
            isBold: Boolean = false,
            isItalic: Boolean = false,
            isStrike: Boolean = false,
            depth: Int = 0
        ) {
            if (text.isEmpty()) return
            if (depth > 5) {
                appendStyledText(text, isBold, isItalic, isStrike)
                return
            }

            var lastIndex = 0
            val matches = INLINE_TOKEN_REGEX.findAll(text)

            for (match in matches) {
                val range = match.range
                if (range.first > lastIndex) {
                    appendStyledText(
                        plain = text.substring(lastIndex, range.first),
                        isBold = isBold,
                        isItalic = isItalic,
                        isStrike = isStrike
                    )
                }

                val linkText = match.groups[1]?.value
                val linkUrl = match.groups[2]?.value
                val inlineCode = match.groups[3]?.value
                val boldItalic1 = match.groups[4]?.value
                val boldItalic2 = match.groups[5]?.value
                val boldText1 = match.groups[6]?.value
                val boldText2 = match.groups[7]?.value
                val italicText1 = match.groups[8]?.value
                val italicText2 = match.groups[9]?.value
                val strikeText = match.groups[10]?.value

                when {
                    linkText != null && linkUrl != null -> {
                        val isInnerBold = (linkText.startsWith("**") && linkText.endsWith("**") && linkText.length >= 4) ||
                                (linkText.startsWith("__") && linkText.endsWith("__") && linkText.length >= 4)
                        val cleanTitle = linkText.trim('`', '\'', '"', '*', '_', ' ').ifEmpty {
                            linkUrl.substringAfterLast('/').ifEmpty { linkText }
                        }
                        val effectiveBold = isBold || isInnerBold
                        val isPlan = linkUrl.contains("implementation_plan") || linkUrl.contains("walkthrough")
                        val isScript = FileIconResolver.isScriptFile(linkUrl) || FileIconResolver.isScriptFile(cleanTitle)

                        // 1. Resolve SVG Icon (1:1 with iOS resolveIcon)
                        val iconName = FileIconResolver.resolveIcon(cleanTitle) ?: FileIconResolver.resolveIcon(linkUrl)
                        if (iconName != null) {
                            val inlineId = "icon_${iconName}_${iconIndex++}"
                            val iconSp = (baseFontSize.value * 0.9f).sp
                            appendInlineContent(id = inlineId, alternateText = " ")
                            append("\u2009") // Thin space (Unicode U+2009) identical to iOS
                            inlineContentMap[inlineId] = InlineTextContent(
                                placeholder = Placeholder(
                                    width = iconSp,
                                    height = iconSp,
                                    placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                                )
                            ) {
                                FileIconSvgView(
                                    iconName = iconName,
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                        }

                        // 2. Append link text with scaled down font, monospaced, Apple Blue, no underline
                        val start = length
                        append(cleanTitle)
                        addStyle(
                            style = SpanStyle(
                                color = colors.accentBlue,
                                fontFamily = FontFamily.Monospace,
                                fontSize = (baseFontSize.value * 0.88f).sp,
                                fontWeight = if (effectiveBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None,
                                background = if (isPlan) colors.accentBlue.copy(alpha = 0.12f) else Color.Transparent
                            ),
                            start = start,
                            end = length
                        )

                        // 3. Script files do NOT need to be accessible, so do not add URL annotation
                        if (!isScript) {
                            addStringAnnotation(
                                tag = "URL",
                                annotation = linkUrl,
                                start = start,
                                end = length
                            )
                            addStringAnnotation(
                                tag = "URL_TITLE",
                                annotation = cleanTitle,
                                start = start,
                                end = length
                            )
                        }
                    }

                    inlineCode != null -> {
                        // Check if bare inline code is a file reference that has a known icon
                        val isFileCode = (inlineCode.contains('.') || inlineCode.contains('/')) &&
                                FileIconResolver.resolveIcon(inlineCode) != null
                        val codeIcon = if (isFileCode) FileIconResolver.resolveIcon(inlineCode) else null

                        if (codeIcon != null) {
                            val inlineId = "icon_${codeIcon}_${iconIndex++}"
                            val iconSp = (baseFontSize.value * 0.9f).sp
                            appendInlineContent(id = inlineId, alternateText = " ")
                            append("\u2009")
                            inlineContentMap[inlineId] = InlineTextContent(
                                placeholder = Placeholder(
                                    width = iconSp,
                                    height = iconSp,
                                    placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                                )
                            ) {
                                FileIconSvgView(
                                    iconName = codeIcon,
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                            val start = length
                            append(inlineCode)
                            addStyle(
                                style = SpanStyle(
                                    color = colors.accentBlue,
                                    fontFamily = FontFamily.Monospace,
                                    fontSize = (baseFontSize.value * 0.88f).sp,
                                    fontWeight = if (isBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                    fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                    textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None
                                ),
                                start = start,
                                end = length
                            )
                        } else {
                            // Standard inline code symbol (e.g. rawTrajectorySignature, readBodyToPool) in Amber
                            val start = length
                            append(inlineCode)
                            addStyle(
                                style = SpanStyle(
                                    color = codeColor,
                                    fontFamily = FontFamily.Monospace,
                                    fontWeight = if (isBold || baseFontWeight == FontWeight.Bold) FontWeight.Bold else FontWeight.Medium,
                                    fontSize = (baseFontSize.value * 0.9f).sp,
                                    fontStyle = if (isItalic) FontStyle.Italic else FontStyle.Normal,
                                    textDecoration = if (isStrike) TextDecoration.LineThrough else TextDecoration.None,
                                    background = colors.surfaceVariant.copy(alpha = 0.45f)
                                ),
                                start = start,
                                end = length
                            )
                        }
                    }

                    (boldItalic1 != null || boldItalic2 != null) -> {
                        val content = boldItalic1 ?: boldItalic2 ?: ""
                        appendInline(
                            text = content,
                            isBold = true,
                            isItalic = true,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    (boldText1 != null || boldText2 != null) -> {
                        val content = boldText1 ?: boldText2 ?: ""
                        appendInline(
                            text = content,
                            isBold = true,
                            isItalic = isItalic,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    (italicText1 != null || italicText2 != null) -> {
                        val content = italicText1 ?: italicText2 ?: ""
                        appendInline(
                            text = content,
                            isBold = isBold,
                            isItalic = true,
                            isStrike = isStrike,
                            depth = depth + 1
                        )
                    }

                    strikeText != null -> {
                        appendInline(
                            text = strikeText,
                            isBold = isBold,
                            isItalic = isItalic,
                            isStrike = true,
                            depth = depth + 1
                        )
                    }

                    else -> {
                        appendStyledText(
                            plain = match.value,
                            isBold = isBold,
                            isItalic = isItalic,
                            isStrike = isStrike
                        )
                    }
                }

                lastIndex = range.last + 1
            }

            if (lastIndex < text.length) {
                appendStyledText(
                    plain = text.substring(lastIndex),
                    isBold = isBold,
                    isItalic = isItalic,
                    isStrike = isStrike
                )
            }
        }

        appendInline(processed)
    }

    return RichTextRenderData(annotatedString, inlineContentMap)
}

/**
 * Preprocesses bare LaTeX arrow commands into native Unicode arrows.
 */
internal fun preprocessArrows(text: String): String {
    if (!text.contains('\\')) return text
    return text
        .replace(Regex("""\\(?:to|rightarrow)\b"""), "→")
        .replace(Regex("""\\(?:gets|leftarrow)\b"""), "←")
        .replace(Regex("""\\(?:implies|Rightarrow)\b"""), "⇒")
        .replace(Regex("""\\(?:iff|Leftrightarrow)\b"""), "⇔")
}

/**
 * Replaces HTML line breaks (<br>, <br/>, <br />, </br>) with newlines.
 */
internal fun replaceHtmlBreaks(text: String): String {
    if (!text.contains("<br", ignoreCase = true)) return text
    return text
        .replace(Regex("""<br\s*/?>""", RegexOption.IGNORE_CASE), "\n")
        .replace("</br>", "\n", ignoreCase = true)
}
