package com.antigravity.mobile.ui.components

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.DarkMode
import androidx.compose.material.icons.filled.LightMode
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.IconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.layer.drawLayer
import androidx.compose.ui.graphics.layer.GraphicsLayer
import androidx.compose.ui.graphics.rememberGraphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.DarkAppColors
import com.antigravity.mobile.ui.theme.LightAppColors
import com.antigravity.mobile.ui.theme.LocalAppColors
import com.antigravity.mobile.ui.util.ShareImageUtils
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.math.min
import kotlin.math.roundToInt

/** 分享长图卡片头部需要的会话上下文。 */
data class ShareCardContext(
    val sessionTitle: String = "",
    /** 完整模型展示名，如 "Gemini 3.8 Flash (High)" / "Claude Opus 4.6 (Thinking)"。 */
    val modelName: String? = null,
    val modelIsClaude: Boolean = false,
    /** 会话发起时间（ISO-8601）。分享卡片显示它，而不是分享当下的时间。 */
    val startedAtIso: String? = null,
    /** 紧邻该气泡之前的用户提问消息（含图片/附件，用于「包含上一条提问」开关）。 */
    val previousMessage: com.antigravity.mobile.data.model.GatewayMessageItem? = null
) {
    companion object {
        /**
         * 长图里的模型名要和用户当时实际选用的模型一致：
         * - Agent 回复：用生成这条回复的模型；
         * - 用户提问：用紧随其后那条回复的模型；
         * - 都没有（旧数据、未知枚举）：回退到会话当前模型。
         */
        fun resolveModel(
            target: com.antigravity.mobile.data.model.GatewayMessageItem,
            messages: List<com.antigravity.mobile.data.model.GatewayMessageItem>,
            activeModel: String,
            activeModelName: String?
        ): Pair<String, Boolean> {
            fun nameOf(m: com.antigravity.mobile.data.model.GatewayMessageItem): String? =
                m.modelName?.takeIf { it.isNotBlank() } ?: m.model?.takeIf { it.isNotBlank() }?.let { modelBadge(it).first }

            val source: com.antigravity.mobile.data.model.GatewayMessageItem? = when {
                target.isUser -> {
                    val idx = messages.indexOfFirst { it === target || (target.id.isNotBlank() && it.id == target.id) }
                    if (idx < 0) null else messages.drop(idx + 1).takeWhile { !it.isUser }.firstOrNull { nameOf(it) != null }
                }
                nameOf(target) != null -> target
                else -> null
            }
            if (source != null) {
                val id = source.model ?: activeModel
                return nameOf(source)!! to com.antigravity.mobile.data.model.ModelDefaults.isClaude(id)
            }
            val fallback = activeModelName?.takeIf { it.isNotBlank() } ?: modelBadge(activeModel).first
            return fallback to com.antigravity.mobile.data.model.ModelDefaults.isClaude(activeModel)
        }

        /** ISO-8601 → 本地时区的 `yyyy-MM-dd HH:mm`；解析失败返回 null（界面直接不显示，不拿分享时间冒充）。 */
        fun formatStartedAt(iso: String?, zone: java.time.ZoneId = java.time.ZoneId.systemDefault()): String? {
            val raw = iso?.trim().orEmpty()
            if (raw.isEmpty()) return null
            val instant = runCatching { java.time.Instant.parse(raw) }.getOrNull()
                ?: runCatching { java.time.OffsetDateTime.parse(raw).toInstant() }.getOrNull()
                ?: return null
            return java.time.format.DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm").withZone(zone).format(instant)
        }

        /** 把 `gemini-3.8-flash-high` / `claude-opus-4-6-thinking` 这类内部模型 ID 转成展示名。 */
        fun modelBadge(raw: String): Pair<String, Boolean> {
            val isClaude = raw.contains("claude", ignoreCase = true) || raw.contains("M26")
            val effort = setOf("high", "medium", "low")
            val tokens = raw.split("-").filter { it.isNotBlank() }.toMutableList()
            if (tokens.firstOrNull()?.equals("claude", ignoreCase = true) == true) tokens.removeAt(0)
            tokens.removeAll { it.lowercase() in effort }
            val parts = mutableListOf<String>()
            for (token in tokens) {
                val isDigits = token.all { it.isDigit() }
                val last = parts.lastOrNull()
                if (isDigits && last != null && last.all { it.isDigit() || it == '.' }) {
                    parts[parts.lastIndex] = "$last.$token"
                } else if (token.first().isLetter()) {
                    parts.add(token.replaceFirstChar { it.uppercase() })
                } else {
                    parts.add(token)
                }
            }
            var name = parts.joinToString(" ")
            if (!isClaude && !name.startsWith("Gemini", ignoreCase = true)) name = "Gemini $name"
            return (name.ifBlank { if (isClaude) "Claude" else "Gemini" }) to isClaude
        }
    }
}

enum class ShareCardTheme(val isDark: Boolean) {
    DARK(true),
    LIGHT(false);

    val accent: Color
        get() = if (isDark) Color(0xFF8C80FF) else Color(0xFF5856D6)

    val background: Brush
        get() = if (isDark) {
            Brush.linearGradient(listOf(Color(0xFF0F121C), Color(0xFF1F1738)))
        } else {
            Brush.linearGradient(listOf(Color(0xFFFCFCFA), Color(0xFFF5F5F2)))
        }

    val questionBackground: Color
        get() = if (isDark) Color.White.copy(alpha = 0.09f) else Color.Black.copy(alpha = 0.05f)
}

private val CARD_WIDTH = 390.dp
private const val MAX_PIXEL_HEIGHT = 8192

/**
 * 分享长图预览 Sheet：卡片以真实 Composable 渲染（所见即所得），
 * 点击保存/分享/拷贝时通过 GraphicsLayer 将卡片原尺寸录制为位图。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MessageShareCardSheet(
    content: String,
    isUserMessage: Boolean,
    images: List<ImageViewerItem>,
    shareContext: ShareCardContext,
    urlResolver: ((String) -> String)?,
    onDismiss: () -> Unit
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val systemDark = androidx.compose.foundation.isSystemInDarkTheme()
    var theme by remember { mutableStateOf(if (systemDark) ShareCardTheme.DARK else ShareCardTheme.LIGHT) }
    var includeQuestion by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    val layer = rememberGraphicsLayer()
    val canIncludeQuestion = !isUserMessage && (shareContext.previousMessage?.hasQuestionContent() ?: false)

    /** 将卡片录制为位图；超过像素上限时等比缩小，避免 OOM / 纹理超限。 */
    suspend fun capture(): Bitmap? {
        return try {
            val raw = layer.toImageBitmap().asAndroidBitmap()
            if (raw.height > MAX_PIXEL_HEIGHT) {
                val ratio = MAX_PIXEL_HEIGHT.toFloat() / raw.height
                Bitmap.createScaledBitmap(raw, (raw.width * ratio).roundToInt().coerceAtLeast(1), MAX_PIXEL_HEIGHT, true)
            } else raw
        } catch (e: Throwable) {
            null
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        sheetMaxWidth = androidx.compose.ui.unit.Dp.Unspecified,
        shape = RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight(0.92f)
                .navigationBarsPadding()
        ) {
            // Top bar: 保存（左） / 标题 / 分享（右）
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.saveBitmapToGallery(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy
                ) {
                    Icon(Icons.Default.FileDownload, contentDescription = "保存到相册")
                }
                Text(
                    text = "分享长图",
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center
                )
                IconButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.shareBitmap(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy
                ) {
                    Icon(Icons.Default.Share, contentDescription = "分享")
                }
            }

            // Live preview of the card (what you see is what gets exported)
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 4.dp)
            ) {
                ScaledToFitWidth(designWidth = CARD_WIDTH) {
                    Box(
                        modifier = Modifier.drawWithContent {
                            layer.record { this@drawWithContent.drawContent() }
                            drawLayer(layer)
                        }
                    ) {
                        MessageShareCard(
                            content = content,
                            isUserMessage = isUserMessage,
                            question = if (includeQuestion) shareContext.previousMessage else null,
                            images = images,
                            theme = theme,
                            shareContext = shareContext,
                            urlResolver = urlResolver
                        )
                    }
                }
            }

            // 底部：左 = 浅/深色切换，右 = 是否包含上一条提问
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                ShareOptionPill(
                    icon = if (theme == ShareCardTheme.DARK) Icons.Default.DarkMode else Icons.Default.LightMode,
                    label = if (theme == ShareCardTheme.DARK) "深色" else "浅色",
                    selected = false,
                    onClick = {
                        theme = if (theme == ShareCardTheme.DARK) ShareCardTheme.LIGHT else ShareCardTheme.DARK
                    }
                )
                Spacer(Modifier.weight(1f))
                if (canIncludeQuestion) {
                    ShareOptionPill(
                        icon = if (includeQuestion) Icons.Default.CheckCircle else Icons.Default.RadioButtonUnchecked,
                        label = "包含上一条提问",
                        selected = includeQuestion,
                        onClick = { includeQuestion = !includeQuestion }
                    )
                }
            }
        }
    }
}

@Composable
private fun ShareOptionPill(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    selected: Boolean,
    onClick: () -> Unit
) {
    val container = if (selected) MaterialTheme.colorScheme.primary.copy(alpha = 0.16f)
    else MaterialTheme.colorScheme.surfaceVariant
    Row(
        modifier = Modifier
            .height(44.dp)
            .clip(CircleShape)
            .background(container)
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
        Text(label, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
    }
}

/** 以 [designWidth] 排版内容，宽度不足时等比缩小并同步缩小占位高度。 */
@Composable
private fun ScaledToFitWidth(designWidth: androidx.compose.ui.unit.Dp, content: @Composable () -> Unit) {
    Box(
        modifier = Modifier.layout { measurable, constraints ->
            val designPx = designWidth.roundToPx()
            val scale = min(1f, constraints.maxWidth.toFloat() / designPx)
            val placeable = measurable.measure(
                Constraints(minWidth = designPx, maxWidth = designPx, minHeight = 0, maxHeight = Constraints.Infinity)
            )
            layout((designPx * scale).roundToInt(), (placeable.height * scale).roundToInt()) {
                placeable.placeWithLayer(0, 0) {
                    scaleX = scale
                    scaleY = scale
                    transformOrigin = TransformOrigin(0f, 0f)
                }
            }
        }
    ) {
        content()
    }
}

@Composable
private fun MessageShareCard(
    content: String,
    isUserMessage: Boolean,
    question: com.antigravity.mobile.data.model.GatewayMessageItem?,
    images: List<ImageViewerItem>,
    theme: ShareCardTheme,
    shareContext: ShareCardContext,
    urlResolver: ((String) -> String)?
) {
    val dateText = remember(shareContext.startedAtIso) { ShareCardContext.formatStartedAt(shareContext.startedAtIso) }
    CompositionLocalProvider(
        LocalAppColors provides (if (theme.isDark) DarkAppColors else LightAppColors),
        LocalShareExport provides true
    ) {
        val colors = AntigravityTheme.colors
        Column(
            modifier = Modifier
                .width(CARD_WIDTH)
                .background(theme.background)
                .padding(start = 20.dp, end = 20.dp, bottom = 20.dp, top = 56.dp), // 预留顶部：避开刘海/状态栏遮挡
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // Header: 会话标题 / 时间 + 模型胶囊
            Text(
                text = shareContext.sessionTitle.ifBlank { "Multigravity 会话" },
                color = colors.textPrimary,
                fontSize = 24.sp,
                lineHeight = 30.sp,
                fontWeight = FontWeight.Bold
            )
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                if (!shareContext.modelName.isNullOrBlank()) {
                    val tint = if (shareContext.modelIsClaude) colors.accentOrange else colors.accentBlue
                    Text(
                        shareContext.modelName,
                        color = tint,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(tint.copy(alpha = 0.14f))
                            .border(1.dp, tint.copy(alpha = 0.35f), CircleShape)
                            .padding(horizontal = 10.dp, vertical = 4.dp)
                    )
                }
                if (dateText != null) {
                    Text(dateText, color = colors.textSecondary, fontSize = 13.sp, fontFamily = FontFamily.Monospace)
                }
            }
            HorizontalDivider(color = colors.textMuted.copy(alpha = 0.4f), thickness = 0.5.dp)

            if (question != null) {
                QuestionBubble(question, urlResolver)
            }

            val appContext = LocalContext.current
            images.forEach { item ->
                AsyncImage(
                    model = ImageRequest.Builder(appContext)
                        .data(item.bitmap ?: item.bytes ?: item.url)
                        .crossfade(false)
                        .build(),
                    contentDescription = null,
                    contentScale = ContentScale.Fit,
                    alignment = Alignment.CenterStart,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 360.dp)
                        .clip(RoundedCornerShape(12.dp))
                )
            }

            if (isUserMessage) {
                Text(content, color = colors.textPrimary, fontSize = 15.5.sp, lineHeight = 22.sp)
            } else {
                MarkdownContentView(content = content, urlResolver = urlResolver)
            }

            HorizontalDivider(color = colors.textMuted.copy(alpha = 0.4f), thickness = 0.5.dp)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Image(
                    painter = androidx.compose.ui.res.painterResource(com.antigravity.mobile.R.drawable.share_app_logo),
                    contentDescription = null,
                    modifier = Modifier
                        .size(32.dp)
                        .clip(RoundedCornerShape(9.dp))
                        .border(0.5.dp, Color.Black.copy(alpha = 0.08f), RoundedCornerShape(9.dp))
                )
                Spacer(Modifier.width(8.dp))
                Text("Multigravity", color = colors.textPrimary, fontSize = 18.sp, fontWeight = FontWeight.Bold)
                Spacer(Modifier.weight(1f))
                val qr = remember { buildQrBitmap(LANDING_URL) }
                if (qr != null) {
                    Image(
                        bitmap = qr.asImageBitmap(),
                        contentDescription = LANDING_URL,
                        filterQuality = androidx.compose.ui.graphics.FilterQuality.None,
                        modifier = Modifier
                            .size(70.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(Color.White)
                            .padding(5.dp)
                    )
                }
            }
        }
    }
}

private const val LANDING_URL = "https://mgy.jiuge.space"

/** 用 zxing 本地生成落地页二维码（黑白位图）。 */
private fun buildQrBitmap(text: String, size: Int = 320): Bitmap? = try {
    val matrix = com.google.zxing.qrcode.QRCodeWriter().encode(
        text,
        com.google.zxing.BarcodeFormat.QR_CODE,
        size,
        size,
        mapOf(
            com.google.zxing.EncodeHintType.MARGIN to 0,
            com.google.zxing.EncodeHintType.ERROR_CORRECTION to com.google.zxing.qrcode.decoder.ErrorCorrectionLevel.M
        )
    )
    val pixels = IntArray(size * size) { i ->
        if (matrix[i % size, i / size]) android.graphics.Color.BLACK else android.graphics.Color.WHITE
    }
    Bitmap.createBitmap(pixels, size, size, Bitmap.Config.ARGB_8888)
} catch (e: Exception) {
    null
}

private fun com.antigravity.mobile.data.model.GatewayMessageItem.hasQuestionContent(): Boolean {
    val (body, files) = com.antigravity.mobile.data.service.AttachmentRules.parseBlock(effectiveText)
    return body.isNotBlank() || files.isNotEmpty() || !imageUrls.isNullOrEmpty() || effectiveImageDataList.isNotEmpty()
}

/** 复刻会话内用户气泡的排布：右对齐，图片 → 文件卡片 → 靛蓝文字气泡。 */
@Composable
private fun QuestionBubble(
    message: com.antigravity.mobile.data.model.GatewayMessageItem,
    urlResolver: ((String) -> String)?
) {
    val colors = AntigravityTheme.colors
    val appContext = LocalContext.current
    val (bodyText, files) = remember(message) {
        com.antigravity.mobile.data.service.AttachmentRules.parseBlock(message.effectiveText)
    }
    val imageItems = remember(message, urlResolver) {
        if (!message.imageUrls.isNullOrEmpty()) {
            message.imageUrls.mapIndexed { idx, raw ->
                ImageViewerItem(url = urlResolver?.invoke(raw) ?: raw, bytes = message.effectiveImageDataList.getOrNull(idx))
            }
        } else {
            message.effectiveImageDataList.map { ImageViewerItem(bytes = it) }
        }
    }

    Column(
        modifier = Modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.End,
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        if (imageItems.size == 1) {
            val item = imageItems.first()
            AsyncImage(
                model = ImageRequest.Builder(appContext).data(item.bytes ?: item.url).crossfade(false).build(),
                contentDescription = null,
                contentScale = ContentScale.Fit,
                alignment = Alignment.CenterEnd,
                modifier = Modifier
                    .widthIn(max = 240.dp)
                    .heightIn(max = 220.dp)
                    .clip(RoundedCornerShape(14.dp))
                    .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
            )
        } else if (imageItems.size > 1) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                imageItems.forEach { item ->
                    AsyncImage(
                        model = ImageRequest.Builder(appContext).data(item.bytes ?: item.url).crossfade(false).build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        modifier = Modifier
                            .size(72.dp)
                            .clip(RoundedCornerShape(12.dp))
                            .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    )
                }
            }
        }

        files.forEach { f ->
            Row(
                modifier = Modifier
                    .widthIn(max = 280.dp)
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                    .padding(horizontal = 10.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                FileTypeBadge(f.name, 36.dp)
                Column {
                    Text(
                        text = f.name,
                        color = colors.textPrimary,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(text = "${f.ext.uppercase()} · ${f.sizeLabel}", color = colors.textMuted, fontSize = 11.sp)
                }
            }
        }

        if (bodyText.isNotBlank()) {
            Text(
                text = bodyText,
                color = colors.userBubbleText,
                fontSize = 15.5.sp,
                lineHeight = 21.sp,
                modifier = Modifier
                    .widthIn(max = 320.dp)
                    .clip(RoundedCornerShape(18.dp))
                    .background(colors.userBubbleBg)
                    .padding(horizontal = 14.dp, vertical = 10.dp)
            )
        }
    }
}
