package com.antigravity.mobile.ui.components

import android.graphics.Bitmap
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
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
    val modelName: String? = null,
    /** 紧邻该气泡之前的用户提问（用于「包含上一条提问」开关）。 */
    val previousQuestion: String? = null
)

enum class ShareCardTheme(val label: String, val isDark: Boolean) {
    DARK_GLASS("暗黑极客", true),
    CLEAN_LIGHT("极简纸白", false),
    TITANIUM("渐变钛金", true);

    val accent: Color
        get() = when (this) {
            DARK_GLASS -> Color(0xFF8C80FF)
            CLEAN_LIGHT -> Color(0xFF5856D6)
            TITANIUM -> Color(0xFFCCD6EB)
        }

    val background: Brush
        get() = when (this) {
            DARK_GLASS -> Brush.linearGradient(listOf(Color(0xFF0F121C), Color(0xFF1F1738)))
            CLEAN_LIGHT -> Brush.linearGradient(listOf(Color(0xFFFCFCFA), Color(0xFFF5F5F2)))
            TITANIUM -> Brush.linearGradient(listOf(Color(0xFF292E38), Color(0xFF5C6373)))
        }

    val questionBackground: Color
        get() = if (this == CLEAN_LIGHT) Color.Black.copy(alpha = 0.05f) else Color.White.copy(alpha = 0.09f)
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
    var theme by remember { mutableStateOf(if (systemDark) ShareCardTheme.DARK_GLASS else ShareCardTheme.CLEAN_LIGHT) }
    var includeQuestion by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    val layer = rememberGraphicsLayer()
    val canIncludeQuestion = !isUserMessage && !shareContext.previousQuestion.isNullOrBlank()

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
            Text(
                text = "分享长图",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier
                    .align(Alignment.CenterHorizontally)
                    .padding(bottom = 8.dp)
            )

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
                            question = if (includeQuestion) shareContext.previousQuestion else null,
                            images = images,
                            theme = theme,
                            shareContext = shareContext,
                            urlResolver = urlResolver
                        )
                    }
                }
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                ShareCardTheme.entries.forEach { item ->
                    FilterChip(
                        selected = theme == item,
                        onClick = { theme = item },
                        label = { Text(item.label, fontSize = 13.sp) }
                    )
                }
            }

            if (canIncludeQuestion) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 20.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text("包含上一条提问", modifier = Modifier.weight(1f), fontSize = 15.sp)
                    Switch(checked = includeQuestion, onCheckedChange = { includeQuestion = it })
                }
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                OutlinedButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.saveBitmapToGallery(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy,
                    modifier = Modifier.weight(1f),
                    contentPadding = PaddingValues(horizontal = 8.dp)
                ) {
                    Icon(Icons.Default.FileDownload, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("保存相册", fontSize = 13.sp, maxLines = 1)
                }
                OutlinedButton(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.copyBitmapToClipboard(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy,
                    modifier = Modifier.weight(1f),
                    contentPadding = PaddingValues(horizontal = 8.dp)
                ) {
                    Icon(Icons.Default.ContentCopy, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("拷贝", fontSize = 13.sp, maxLines = 1)
                }
                Button(
                    onClick = {
                        scope.launch {
                            busy = true
                            ShareImageUtils.shareBitmap(context, capture())
                            busy = false
                        }
                    },
                    enabled = !busy,
                    modifier = Modifier.weight(1f),
                    contentPadding = PaddingValues(horizontal = 8.dp)
                ) {
                    Icon(Icons.Default.Share, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("系统分享", fontSize = 13.sp, maxLines = 1)
                }
            }
        }
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
    question: String?,
    images: List<ImageViewerItem>,
    theme: ShareCardTheme,
    shareContext: ShareCardContext,
    urlResolver: ((String) -> String)?
) {
    val dateText = remember { SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault()).format(Date()) }
    CompositionLocalProvider(
        LocalAppColors provides (if (theme.isDark) DarkAppColors else LightAppColors),
        LocalShareExport provides true
    ) {
        val colors = AntigravityTheme.colors
        Column(
            modifier = Modifier
                .width(CARD_WIDTH)
                .background(theme.background)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // Header
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(
                    modifier = Modifier
                        .size(26.dp)
                        .clip(RoundedCornerShape(7.dp))
                        .background(theme.accent),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(Icons.Default.AutoAwesome, contentDescription = null, tint = Color.White, modifier = Modifier.size(15.dp))
                }
                Text("Multigravity", color = colors.textPrimary, fontSize = 16.sp, fontWeight = FontWeight.Bold)
                Spacer(Modifier.weight(1f))
                Text(dateText, color = colors.textSecondary, fontSize = 11.sp, fontFamily = FontFamily.Monospace)
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (shareContext.sessionTitle.isNotBlank()) {
                    Text(
                        shareContext.sessionTitle,
                        color = colors.textSecondary,
                        fontSize = 12.5.sp,
                        fontWeight = FontWeight.Medium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false)
                    )
                }
                if (!shareContext.modelName.isNullOrBlank()) {
                    Text(
                        shareContext.modelName,
                        color = theme.accent,
                        fontSize = 10.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(theme.accent.copy(alpha = 0.16f))
                            .padding(horizontal = 7.dp, vertical = 2.dp)
                    )
                }
            }
            HorizontalDivider(color = colors.textMuted.copy(alpha = 0.4f), thickness = 0.5.dp)

            if (!question.isNullOrBlank()) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(theme.questionBackground)
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text("提问", color = theme.accent, fontSize = 11.sp, fontWeight = FontWeight.Bold)
                    Text(question, color = colors.textPrimary, fontSize = 14.5.sp, lineHeight = 20.sp)
                }
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
            Text(
                "Generated by Multigravity · Powering AI Workflows",
                color = colors.textSecondary,
                fontSize = 10.5.sp,
                modifier = Modifier.align(Alignment.CenterHorizontally)
            )
        }
    }
}
