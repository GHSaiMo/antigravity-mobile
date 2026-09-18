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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
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
    val title: String? = null
)

data class ImageViewerData(
    val items: List<ImageViewerItem> = emptyList(),
    val initialIndex: Int = 0
) {
    constructor(bitmap: Bitmap? = null, url: String? = null, title: String? = null) : this(
        items = listOf(ImageViewerItem(bitmap = bitmap, url = url, title = title)),
        initialIndex = 0
    )

    val bitmap: Bitmap? get() = items.getOrNull(initialIndex)?.bitmap
    val url: String? get() = items.getOrNull(initialIndex)?.url
    val title: String? get() = items.getOrNull(initialIndex)?.title
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ImageViewerSheet(
    data: ImageViewerData,
    onDismiss: () -> Unit
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()

    val effectiveItems = remember(data) {
        if (data.items.isNotEmpty()) data.items
        else listOf(ImageViewerItem(bitmap = data.bitmap, url = data.url, title = data.title))
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
                    } else if (!item?.url.isNullOrBlank()) {
                        SubcomposeAsyncImage(
                            model = ImageRequest.Builder(context)
                                .data(item.url)
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

            // Top Action Bar (Close, Page Indicator, Save, Share)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .statusBarsPadding()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                // Close button
                Box(
                    modifier = Modifier
                        .size(38.dp)
                        .clip(CircleShape)
                        .background(Color.Black.copy(alpha = 0.5f))
                        .clickable { onDismiss() },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = "Close",
                        tint = Color.White,
                        modifier = Modifier.size(20.dp)
                    )
                }

                if (effectiveItems.size > 1) {
                    Text(
                        text = "${pagerState.currentPage + 1} / ${effectiveItems.size}",
                        color = Color.White,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(Color.Black.copy(alpha = 0.5f))
                            .padding(horizontal = 14.dp, vertical = 6.dp)
                    )
                }

                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    val currentItem = effectiveItems.getOrNull(pagerState.currentPage)
                        ?: ImageViewerItem(data.bitmap, data.url, data.title)

                    // Save Button
                    Box(
                        modifier = Modifier
                            .size(38.dp)
                            .clip(CircleShape)
                            .background(Color.Black.copy(alpha = 0.5f))
                            .clickable {
                                coroutineScope.launch {
                                    saveImageToGallery(context, currentItem)
                                }
                            },
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.FileDownload,
                            contentDescription = "Save",
                            tint = Color.White,
                            modifier = Modifier.size(20.dp)
                        )
                    }

                    // Share Button
                    Box(
                        modifier = Modifier
                            .size(38.dp)
                            .clip(CircleShape)
                            .background(Color.Black.copy(alpha = 0.5f))
                            .clickable {
                                coroutineScope.launch {
                                    shareImage(context, currentItem)
                                }
                            },
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Share,
                            contentDescription = "Share",
                            tint = Color.White,
                            modifier = Modifier.size(19.dp)
                        )
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

private suspend fun resolveBitmap(context: Context, item: ImageViewerItem): Bitmap? {
    if (item.bitmap != null) return item.bitmap
    if (item.url.isNullOrBlank()) return null
    return withContext(Dispatchers.IO) {
        try {
            val loader = context.imageLoader
            val request = ImageRequest.Builder(context)
                .data(item.url)
                .allowHardware(false)
                .build()
            val result = loader.execute(request)
            if (result is SuccessResult) {
                (result.drawable as? android.graphics.drawable.BitmapDrawable)?.bitmap
            } else null
        } catch (e: Exception) {
            null
        }
    }
}

private suspend fun resolveBitmap(context: Context, data: ImageViewerData): Bitmap? {
    val item = data.items.getOrNull(data.initialIndex) ?: ImageViewerItem(data.bitmap, data.url, data.title)
    return resolveBitmap(context, item)
}

private suspend fun saveImageToGallery(context: Context, item: ImageViewerItem) {
    val bitmap = resolveBitmap(context, item)
    if (bitmap == null) {
        withContext(Dispatchers.Main) {
            Toast.makeText(context, "图片未就绪，无法保存", Toast.LENGTH_SHORT).show()
        }
        return
    }

    withContext(Dispatchers.IO) {
        try {
            val filename = "Antigravity_${System.currentTimeMillis()}.png"
            val fos: OutputStream?
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                val resolver = context.contentResolver
                val contentValues = ContentValues().apply {
                    put(MediaStore.MediaColumns.DISPLAY_NAME, filename)
                    put(MediaStore.MediaColumns.MIME_TYPE, "image/png")
                    put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_PICTURES + "/Antigravity")
                }
                val imageUri = resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, contentValues)
                fos = imageUri?.let { resolver.openOutputStream(it) }
            } else {
                val imagesDir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_PICTURES).toString()
                val image = java.io.File(imagesDir, filename)
                fos = java.io.FileOutputStream(image)
            }

            fos?.use {
                bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
            }

            withContext(Dispatchers.Main) {
                Toast.makeText(context, "已保存到相册", Toast.LENGTH_SHORT).show()
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) {
                Toast.makeText(context, "保存失败: ${e.localizedMessage}", Toast.LENGTH_SHORT).show()
            }
        }
    }
}

private suspend fun saveImageToGallery(context: Context, data: ImageViewerData) {
    val item = data.items.getOrNull(data.initialIndex) ?: ImageViewerItem(data.bitmap, data.url, data.title)
    saveImageToGallery(context, item)
}

private suspend fun shareImage(context: Context, item: ImageViewerItem) {
    val bitmap = resolveBitmap(context, item)
    if (bitmap == null) {
        if (!item.url.isNullOrBlank()) {
            val shareIntent = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_TEXT, item.url)
            }
            context.startActivity(Intent.createChooser(shareIntent, "分享图片链接"))
        }
        return
    }

    withContext(Dispatchers.IO) {
        try {
            val cachePath = java.io.File(context.cacheDir, "images")
            cachePath.mkdirs()
            val file = java.io.File(cachePath, "shared_image_${System.currentTimeMillis()}.png")
            val stream = java.io.FileOutputStream(file)
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, stream)
            stream.close()

            val contentUri: Uri = androidx.core.content.FileProvider.getUriForFile(
                context,
                "${context.packageName}.fileprovider",
                file
            )

            val shareIntent = Intent(Intent.ACTION_SEND).apply {
                type = "image/png"
                putExtra(Intent.EXTRA_STREAM, contentUri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }

            withContext(Dispatchers.Main) {
                context.startActivity(Intent.createChooser(shareIntent, "分享图片"))
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) {
                Toast.makeText(context, "分享失败: ${e.localizedMessage}", Toast.LENGTH_SHORT).show()
            }
        }
    }
}

private suspend fun shareImage(context: Context, data: ImageViewerData) {
    val item = data.items.getOrNull(data.initialIndex) ?: ImageViewerItem(data.bitmap, data.url, data.title)
    shareImage(context, item)
}
