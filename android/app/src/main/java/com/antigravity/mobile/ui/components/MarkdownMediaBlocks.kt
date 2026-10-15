package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImagePainter
import coil.compose.SubcomposeAsyncImage
import coil.compose.SubcomposeAsyncImageContent
import coil.request.ImageRequest
import com.antigravity.mobile.ui.theme.AppColors
import android.graphics.Bitmap
import android.media.MediaMetadataRetriever
import java.net.URLDecoder
import androidx.compose.foundation.Image
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Videocam
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import com.antigravity.mobile.data.service.FileIconResolver
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Video Preview Card for Markdown content (1:1 aligned with iOS MarkdownVideoPreviewCard).
 * Displays video poster frame extracted via MediaMetadataRetriever (or elegant placeholder),
 * centered frosted play button, title, and duration / MP4 pill.
 */
@Composable
internal fun MarkdownVideoPreviewCard(
    url: String,
    rawUri: String,
    title: String,
    colors: AppColors,
    onClick: () -> Unit
) {
    val displayTitle = remember(title, url, rawUri) {
        if (title.isNotBlank() && title != "视频预览" && title != "文档详情") {
            title
        } else {
            val target = if (url.isNotBlank()) url else rawUri
            val clean = target.substringAfterLast('/').substringBefore('?').substringBefore('#')
            val decoded = try { URLDecoder.decode(clean, "UTF-8") } catch (_: Exception) { clean }
            if (decoded.isBlank() || decoded == "raw") "MP4 视频" else decoded
        }
    }

    var posterBitmap by remember { mutableStateOf<Bitmap?>(null) }
    var durationMs by remember { mutableStateOf<Long?>(null) }
    var isLoadingPoster by remember { mutableStateOf(false) }

    LaunchedEffect(url) {
        isLoadingPoster = true
        withContext(Dispatchers.IO) {
            var retriever: MediaMetadataRetriever? = null
            try {
                retriever = MediaMetadataRetriever()
                when {
                    url.startsWith("http://") || url.startsWith("https://") -> {
                        retriever.setDataSource(url, HashMap<String, String>())
                    }
                    url.startsWith("file://") -> {
                        retriever.setDataSource(url.removePrefix("file://"))
                    }
                    url.startsWith("/") -> {
                        retriever.setDataSource(url)
                    }
                }
                val frame = retriever.getFrameAtTime(1000000, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                    ?: retriever.frameAtTime
                val durStr = retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)
                val dur = durStr?.toLongOrNull()
                withContext(Dispatchers.Main) {
                    posterBitmap = frame
                    durationMs = dur
                }
            } catch (_: Exception) {
                // Fallback: styled placeholder
            } finally {
                try {
                    retriever?.release()
                } catch (_: Exception) {}
                withContext(Dispatchers.Main) {
                    isLoadingPoster = false
                }
            }
        }
    }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(200.dp)
            .clip(RoundedCornerShape(14.dp))
            .background(
                brush = Brush.linearGradient(
                    colors = listOf(Color(0xFF1E2129), Color(0xFF0F1115))
                )
            )
            .border(0.8.dp, colors.border.copy(alpha = 0.5f), RoundedCornerShape(14.dp))
            .clickable { onClick() }
    ) {
        if (posterBitmap != null) {
            Image(
                bitmap = posterBitmap!!.asImageBitmap(),
                contentDescription = displayTitle,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize()
            )
        } else {
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center
            ) {
                if (isLoadingPoster) {
                    CircularProgressIndicator(
                        color = Color.White.copy(alpha = 0.7f),
                        strokeWidth = 2.dp,
                        modifier = Modifier.size(24.dp)
                    )
                } else {
                    Icon(
                        imageVector = Icons.Default.Videocam,
                        contentDescription = null,
                        tint = Color.White.copy(alpha = 0.35f),
                        modifier = Modifier.size(40.dp)
                    )
                }
            }
        }

        // Vignette gradient overlay for text readability
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(
                    brush = Brush.verticalGradient(
                        colors = listOf(
                            Color.Black.copy(alpha = 0.15f),
                            Color.Transparent,
                            Color.Black.copy(alpha = 0.75f)
                        )
                    )
                )
        )

        // Centered frosted-glass Play button
        Surface(
            shape = CircleShape,
            color = Color.Black.copy(alpha = 0.45f),
            border = androidx.compose.foundation.BorderStroke(1.dp, Color.White.copy(alpha = 0.35f)),
            modifier = Modifier
                .size(52.dp)
                .align(Alignment.Center)
        ) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "播放",
                    tint = Color.White,
                    modifier = Modifier.size(28.dp).offset(x = 1.5.dp)
                )
            }
        }

        // Bottom metadata overlay
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .align(Alignment.BottomStart)
                .padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                imageVector = Icons.Default.Videocam,
                contentDescription = null,
                tint = Color.White.copy(alpha = 0.9f),
                modifier = Modifier.size(15.dp)
            )

            Text(
                text = displayTitle,
                color = Color.White,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )

            val badgeText = durationMs?.let { ms ->
                val sec = (ms / 1000).coerceAtLeast(0)
                "%02d:%02d".format(sec / 60, sec % 60)
            } ?: "MP4"

            Surface(
                shape = RoundedCornerShape(12.dp),
                color = Color.Black.copy(alpha = 0.55f),
                border = androidx.compose.foundation.BorderStroke(0.6.dp, Color.White.copy(alpha = 0.25f))
            ) {
                Text(
                    text = badgeText,
                    color = Color.White.copy(alpha = 0.95f),
                    fontSize = 11.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace,
                    modifier = Modifier.padding(horizontal = 7.dp, vertical = 3.dp)
                )
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun MarkdownImageView(
    alt: String,
    url: String,
    colors: AppColors,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((String) -> Unit)? = null
) {
    val context = LocalContext.current
    val resolvedUrl = remember(url) { urlResolver?.invoke(url) ?: url }
    val isVideo = remember(url, resolvedUrl) {
        FileIconResolver.isVideo(url) || FileIconResolver.isVideo(resolvedUrl)
    }

    if (isVideo) {
        MarkdownVideoPreviewCard(
            url = resolvedUrl,
            rawUri = url,
            title = alt,
            colors = colors,
            onClick = { onImageClick?.invoke(resolvedUrl) }
        )
        return
    }

    val stableCacheKey = remember(url, resolvedUrl) {
        if (resolvedUrl.contains("/api/v1/files/raw")) {
            resolvedUrl.substringAfter("/api/v1/files/raw")
        } else {
            url
        }
    }

    var showMenu by remember { mutableStateOf(false) }

    Box(
        modifier = Modifier.fillMaxWidth(),
        contentAlignment = Alignment.CenterStart
    ) {
        ImageActionMenu(
            expanded = showMenu,
            onDismiss = { showMenu = false },
            item = ImageViewerItem(url = resolvedUrl),
            onOpen = { onImageClick?.invoke(resolvedUrl) }
        )
        SubcomposeAsyncImage(
            model = ImageRequest.Builder(context)
                .data(resolvedUrl)
                .diskCacheKey(stableCacheKey)
                .memoryCacheKey(stableCacheKey)
                .crossfade(!LocalShareExport.current)
                .build(),
            contentDescription = alt.ifBlank { "图片" },
            contentScale = ContentScale.Fit,
            modifier = Modifier
                .heightIn(max = 280.dp)
                .clip(RoundedCornerShape(14.dp))
                .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
        ) {
            val state = painter.state
            when (state) {
                is AsyncImagePainter.State.Loading -> {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 120.dp, max = 160.dp),
                        contentAlignment = Alignment.CenterStart
                    ) {
                        CircularProgressIndicator(
                            color = colors.accentIndigo,
                            strokeWidth = 2.dp,
                            modifier = Modifier.size(24.dp)
                        )
                    }
                }
                is AsyncImagePainter.State.Error -> {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Description,
                            contentDescription = "Failed",
                            tint = colors.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                        Text(
                            text = if (alt.isNotBlank()) alt else "图片加载失败",
                            color = colors.textSecondary,
                            fontSize = 12.sp,
                            maxLines = 2
                        )
                    }
                }
                else -> {
                    SubcomposeAsyncImageContent(
                        modifier = Modifier.combinedClickable(
                            onClick = { onImageClick?.invoke(resolvedUrl) },
                            onLongClick = { showMenu = true }
                        )
                    )
                }
            }
        }
    }
}

@Composable
internal fun FrontmatterCard(
    rawContent: String,
    lineCount: Int,
    colors: AppColors
) {
    var expanded by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.4f))
            .border(0.5.dp, colors.border, RoundedCornerShape(8.dp))
            .clickable { expanded = !expanded }
            .padding(horizontal = 12.dp, vertical = 8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Description,
                    contentDescription = null,
                    tint = colors.textMuted,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "YAML 配置 / 元数据 ($lineCount 行)",
                    color = colors.textSecondary,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.Medium
                )
            }
            Icon(
                imageVector = if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(16.dp)
            )
        }

        AnimatedVisibility(
            visible = expanded,
            enter = expandVertically(
                animationSpec = spring(
                    dampingRatio = 0.82f,
                    stiffness = Spring.StiffnessMediumLow
                )
            ) + fadeIn(
                animationSpec = spring(dampingRatio = 0.82f)
            ),
            exit = shrinkVertically(
                animationSpec = spring(
                    dampingRatio = 0.82f,
                    stiffness = Spring.StiffnessMediumLow
                )
            ) + fadeOut(
                animationSpec = spring(dampingRatio = 0.82f)
            )
        ) {
            SelectionContainer {
                Text(
                    text = rawContent,
                    color = colors.textSecondary,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.5.sp,
                    lineHeight = 16.sp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp)
                )
            }
        }
    }
}
