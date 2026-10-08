package com.antigravity.mobile.ui.components

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.widget.Toast
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.*
import com.antigravity.mobile.ui.util.rememberHaptic
import com.antigravity.mobile.ui.util.ShareImageUtils
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import coil.compose.AsyncImagePainter
import coil.compose.SubcomposeAsyncImage
import coil.compose.SubcomposeAsyncImageContent
import coil.imageLoader
import coil.request.ImageRequest
import coil.request.SuccessResult
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.OutputStream
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.calculateCentroidSize
import androidx.compose.foundation.gestures.calculatePan
import androidx.compose.foundation.gestures.calculateZoom
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.ui.input.pointer.PointerInputScope
import androidx.compose.ui.text.font.FontWeight
import kotlin.math.abs
import kotlin.math.roundToInt

data class ImageViewerItem(
    val bitmap: Bitmap? = null,
    val url: String? = null,
    val title: String? = null,
    val bytes: ByteArray? = null
)

data class ImageViewerData(
    val items: List<ImageViewerItem> = emptyList(),
    val initialIndex: Int = 0
) {
    constructor(bitmap: Bitmap? = null, url: String? = null, title: String? = null, bytes: ByteArray? = null) : this(
        items = listOf(ImageViewerItem(bitmap = bitmap, url = url, title = title, bytes = bytes)),
        initialIndex = 0
    )

    val bitmap: Bitmap? get() = items.getOrNull(initialIndex)?.bitmap
    val url: String? get() = items.getOrNull(initialIndex)?.url
    val title: String? get() = items.getOrNull(initialIndex)?.title
    val bytes: ByteArray? get() = items.getOrNull(initialIndex)?.bytes
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalMaterial3Api::class)
@Composable
fun ImageViewerSheet(
    data: ImageViewerData,
    onDismiss: () -> Unit
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    var showSaveSheet by remember { mutableStateOf(false) }

    val effectiveItems = remember(data) {
        if (data.items.isNotEmpty()) data.items
        else listOf(ImageViewerItem(bitmap = data.bitmap, url = data.url, title = data.title, bytes = data.bytes))
    }

    val pagerState = rememberPagerState(
        initialPage = data.initialIndex.coerceIn(0, maxOf(0, effectiveItems.size - 1)),
        pageCount = { effectiveItems.size }
    )

    val scaleAnim = remember { Animatable(1f) }
    val offsetXAnim = remember { Animatable(0f) }
    val offsetYAnim = remember { Animatable(0f) }
    val dismissOffsetY = remember { Animatable(0f) }

    // Reset zoom and pan when switching pages
    LaunchedEffect(pagerState.currentPage) {
        scaleAnim.snapTo(1f)
        offsetXAnim.snapTo(0f)
        offsetYAnim.snapTo(0f)
    }

    val bgAlpha = remember(dismissOffsetY.value) {
        val dragDist = kotlin.math.abs(dismissOffsetY.value)
        (1f - (dragDist / 400f)).coerceIn(0.2f, 1f)
    }

    val springSpec = remember {
        spring<Float>(dampingRatio = 0.82f, stiffness = Spring.StiffnessMediumLow)
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            usePlatformDefaultWidth = false,
            decorFitsSystemWindows = false
        )
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Color.Black.copy(alpha = bgAlpha))
                .draggable(
                    state = rememberDraggableState { delta ->
                        coroutineScope.launch {
                            dismissOffsetY.snapTo(dismissOffsetY.value + delta)
                        }
                    },
                    orientation = Orientation.Vertical,
                    enabled = (scaleAnim.value <= 1.01f),
                    onDragStopped = { velocity ->
                        if (kotlin.math.abs(dismissOffsetY.value) > 180f || kotlin.math.abs(velocity) > 800f) {
                            onDismiss()
                        } else {
                            coroutineScope.launch {
                                dismissOffsetY.animateTo(0f, springSpec)
                            }
                        }
                    }
                )
        ) {
            // Dismiss drag and bounce-back watcher
            LaunchedEffect(scaleAnim.value) {
                if (scaleAnim.value < 1.0f && !scaleAnim.isRunning) {
                    scaleAnim.animateTo(1.0f, springSpec)
                    offsetXAnim.animateTo(0f, springSpec)
                    offsetYAnim.animateTo(0f, springSpec)
                }
            }

            // Paged image display container
            HorizontalPager(
                state = pagerState,
                modifier = Modifier
                    .fillMaxSize()
                    .offset { IntOffset(0, dismissOffsetY.value.toInt()) },
                userScrollEnabled = (scaleAnim.value <= 1.01f)
            ) { page ->
                val item = effectiveItems.getOrNull(page)
                val isCurrent = (page == pagerState.currentPage)

                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .pointerInput(page) {
                            detectTapGestures(
                                onTap = {
                                    if (scaleAnim.value <= 1.05f) {
                                        onDismiss()
                                    }
                                },
                                onDoubleTap = {
                                    coroutineScope.launch {
                                        val targetScale = if (scaleAnim.value > 1.05f) 1f else 2.5f
                                        launch { scaleAnim.animateTo(targetScale, springSpec) }
                                        launch { offsetXAnim.animateTo(0f, springSpec) }
                                        launch { offsetYAnim.animateTo(0f, springSpec) }
                                        launch { dismissOffsetY.animateTo(0f, springSpec) }
                                    }
                                },
                                onLongPress = {
                                    haptic.longPress()
                                    showSaveSheet = true
                                }
                            )
                        }
                        .then(
                            if (isCurrent) {
                                Modifier.pointerInput(page) {
                                    detectZoomAndPan(
                                        currentScale = { scaleAnim.value },
                                        onZoom = { zoom ->
                                            coroutineScope.launch {
                                                val newScale = (scaleAnim.value * zoom).coerceIn(0.7f, 6.0f)
                                                scaleAnim.snapTo(newScale)
                                            }
                                        },
                                        onPan = { pan ->
                                            coroutineScope.launch {
                                                if (scaleAnim.value > 1.01f) {
                                                    val maxOffsetX = 600f * (scaleAnim.value - 1f)
                                                    val maxOffsetY = 800f * (scaleAnim.value - 1f)
                                                    val newX = (offsetXAnim.value + pan.x).coerceIn(-maxOffsetX, maxOffsetX)
                                                    val newY = (offsetYAnim.value + pan.y).coerceIn(-maxOffsetY, maxOffsetY)
                                                    offsetXAnim.snapTo(newX)
                                                    offsetYAnim.snapTo(newY)
                                                }
                                            }
                                        }
                                    )
                                }
                            } else Modifier
                        )
                        .graphicsLayer {
                            if (isCurrent) {
                                scaleX = scaleAnim.value
                                scaleY = scaleAnim.value
                                translationX = offsetXAnim.value
                                translationY = offsetYAnim.value
                            }
                        },
                    contentAlignment = Alignment.Center
                ) {
                    if (item?.bitmap != null) {
                        Image(
                            bitmap = item.bitmap.asImageBitmap(),
                            contentDescription = "Full image preview",
                            contentScale = ContentScale.Fit,
                            modifier = Modifier.fillMaxSize()
                        )
                    } else if (item?.bytes != null || !item?.url.isNullOrBlank()) {
                        val imgUrl = item?.url
                        val stableKey = if (!imgUrl.isNullOrBlank()) {
                            if (imgUrl.contains("/api/v1/files/raw")) {
                                imgUrl.substringAfter("/api/v1/files/raw")
                            } else {
                                imgUrl
                            }
                        } else null
                        SubcomposeAsyncImage(
                            model = ImageRequest.Builder(context)
                                .data(item?.bytes ?: item?.url)
                                .apply {
                                    if (stableKey != null) {
                                        diskCacheKey(stableKey)
                                        memoryCacheKey(stableKey)
                                    }
                                }
                                .crossfade(true)
                                .build(),
                            contentDescription = "Full image preview",
                            contentScale = ContentScale.Fit,
                            modifier = Modifier.fillMaxSize()
                        ) {
                            val state = painter.state
                            if (state is AsyncImagePainter.State.Loading) {
                                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp)
                                }
                            } else if (state is AsyncImagePainter.State.Error) {
                                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                    Text("图片加载失败", color = Color.White.copy(alpha = 0.7f), fontSize = 14.sp)
                                }
                            } else {
                                SubcomposeAsyncImageContent()
                            }
                        }
                    }
                }
            }

            // Persistent quick actions (save / share) at the top-right corner
            Row(
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .statusBarsPadding()
                    .padding(top = 8.dp, end = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                ViewerIconButton(Icons.Default.FileDownload, "保存到相册") {
                    val current = effectiveItems.getOrNull(pagerState.currentPage)
                        ?: ImageViewerItem(data.bitmap, data.url, data.title, bytes = data.bytes)
                    coroutineScope.launch { saveImageToGallery(context, current) }
                }
                ViewerIconButton(Icons.Default.Share, "分享图片") {
                    val current = effectiveItems.getOrNull(pagerState.currentPage)
                        ?: ImageViewerItem(data.bitmap, data.url, data.title, bytes = data.bytes)
                    coroutineScope.launch { shareImage(context, current) }
                }
            }

            // Native ModalBottomSheet for Save to Album on long press
            if (showSaveSheet) {
                ModalBottomSheet(
                    onDismissRequest = { showSaveSheet = false },
                    sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
                    sheetMaxWidth = Dp.Unspecified,
                    containerColor = Color(0xFF1E1E1E),
                    dragHandle = {
                        Box(
                            modifier = Modifier
                                .padding(top = 10.dp, bottom = 12.dp)
                                .width(36.dp)
                                .height(4.dp)
                                .clip(CircleShape)
                                .background(Color.White.copy(alpha = 0.35f))
                        )
                    },
                    shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
                    contentWindowInsets = { WindowInsets(0, 0, 0, 0) }
                ) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .navigationBarsPadding()
                            .padding(bottom = 16.dp)
                    ) {
                        val sheetItem = effectiveItems.getOrNull(pagerState.currentPage)
                            ?: ImageViewerItem(data.bitmap, data.url, data.title, bytes = data.bytes)
                        ViewerActionRow(Icons.Default.FileDownload, "保存到相册") {
                            showSaveSheet = false
                            coroutineScope.launch { saveImageToGallery(context, sheetItem) }
                        }
                        ViewerActionRow(Icons.Default.Share, "分享图片") {
                            showSaveSheet = false
                            coroutineScope.launch { shareImage(context, sheetItem) }
                        }
                        ViewerActionRow(Icons.Default.ContentCopy, "拷贝图片") {
                            showSaveSheet = false
                            coroutineScope.launch { copyImage(context, sheetItem) }
                        }
                    }
                }
            }
        }
    }
}

private suspend fun PointerInputScope.detectZoomAndPan(
    currentScale: () -> Float,
    onZoom: (zoom: Float) -> Unit,
    onPan: (pan: Offset) -> Unit
) {
    awaitEachGesture {
        var zoom = 1f
        var pan = Offset.Zero
        var pastTouchSlop = false
        val touchSlop = viewConfiguration.touchSlop

        awaitFirstDown(requireUnconsumed = false)
        do {
            val event = awaitPointerEvent()
            val canceled = event.changes.any { it.isConsumed }
            if (!canceled) {
                val pointerCount = event.changes.size
                val isZoomed = currentScale() > 1.01f

                if (pointerCount >= 2 || isZoomed) {
                    val zoomChange = event.calculateZoom()
                    val panChange = event.calculatePan()

                    if (!pastTouchSlop) {
                        zoom *= zoomChange
                        pan += panChange

                        val centroidSize = event.calculateCentroidSize(useCurrent = false)
                        val zoomMotion = abs(1 - zoom) * centroidSize
                        val panMotion = pan.getDistance()

                        if (zoomMotion > touchSlop || panMotion > touchSlop) {
                            pastTouchSlop = true
                        }
                    }

                    if (pastTouchSlop) {
                        if (zoomChange != 1f) {
                            onZoom(zoomChange)
                        }
                        if (panChange != Offset.Zero) {
                            onPan(panChange)
                        }
                        event.changes.forEach { it.consume() }
                    }
                }
            }
        } while (!canceled && event.changes.any { it.pressed })
    }
}

private suspend fun resolveBitmap(context: Context, item: ImageViewerItem): Bitmap? =
    ShareImageUtils.resolveBitmap(context, item)

private suspend fun saveImageToGallery(context: Context, item: ImageViewerItem) {
    ShareImageUtils.saveBitmapToGallery(context, resolveBitmap(context, item))
}

private suspend fun shareImage(context: Context, item: ImageViewerItem) {
    ShareImageUtils.shareBitmap(context, resolveBitmap(context, item), fallbackUrl = item.url)
}

private suspend fun copyImage(context: Context, item: ImageViewerItem) {
    ShareImageUtils.copyBitmapToClipboard(context, resolveBitmap(context, item))
}

@Composable
private fun ViewerIconButton(icon: androidx.compose.ui.graphics.vector.ImageVector, description: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        shape = CircleShape,
        color = Color.Black.copy(alpha = 0.45f),
        modifier = Modifier.size(40.dp)
    ) {
        Box(contentAlignment = Alignment.Center) {
            Icon(
                imageVector = icon,
                contentDescription = description,
                tint = Color.White,
                modifier = Modifier.size(22.dp)
            )
        }
    }
}

@Composable
private fun ViewerActionRow(icon: androidx.compose.ui.graphics.vector.ImageVector, label: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        color = Color.Transparent,
        modifier = Modifier.fillMaxWidth()
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Icon(imageVector = icon, contentDescription = label, tint = Color.White, modifier = Modifier.size(24.dp))
            Text(text = label, color = Color.White, fontSize = 16.sp, fontWeight = FontWeight.Medium)
        }
    }
}
