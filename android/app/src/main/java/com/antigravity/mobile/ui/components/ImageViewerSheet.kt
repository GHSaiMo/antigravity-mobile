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
import kotlin.math.abs
import kotlin.math.roundToInt

data class ImageViewerData(
    val bitmap: Bitmap? = null,
    val url: String? = null,
    val title: String? = null
)

@Composable
fun ImageViewerSheet(
    data: ImageViewerData,
    onDismiss: () -> Unit
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()

    val scaleAnim = remember { Animatable(1f) }
    val offsetXAnim = remember { Animatable(0f) }
    val offsetYAnim = remember { Animatable(0f) }
    val dismissOffsetY = remember { Animatable(0f) }

    val bgAlpha = remember(dismissOffsetY.value) {
        val dragDist = kotlin.math.abs(dismissOffsetY.value)
        (1f - (dragDist / 400f)).coerceIn(0.2f, 1f)
    }

    val springSpec = remember {
        spring<Float>(dampingFraction = 0.82f, stiffness = Spring.StiffnessMediumLow)
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
                .pointerInput(Unit) {
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
                .pointerInput(Unit) {
                    detectTransformGestures { _, pan, zoom, _ ->
                        coroutineScope.launch {
                            val newScale = (scaleAnim.value * zoom).coerceIn(0.7f, 6.0f)
                            scaleAnim.snapTo(newScale)

                            if (newScale > 1.01f) {
                                val maxOffsetX = 600f * (newScale - 1f)
                                val maxOffsetY = 800f * (newScale - 1f)
                                val newX = (offsetXAnim.value + pan.x).coerceIn(-maxOffsetX, maxOffsetX)
                                val newY = (offsetYAnim.value + pan.y).coerceIn(-maxOffsetY, maxOffsetY)
                                offsetXAnim.snapTo(newX)
                                offsetYAnim.snapTo(newY)
                            } else {
                                val curY = dismissOffsetY.value + pan.y
                                dismissOffsetY.snapTo(curY)
                                if (kotlin.math.abs(curY) > 220f) {
                                    onDismiss()
                                }
                            }
                        }
                    }
                }
        ) {
            // Dismiss drag and bounce-back watcher
            LaunchedEffect(scaleAnim.value) {
                if (scaleAnim.value < 1.0f && !scaleAnim.isRunning) {
                    scaleAnim.animateTo(1.0f, springSpec)
                    offsetXAnim.animateTo(0f, springSpec)
                    offsetYAnim.animateTo(0f, springSpec)
                }
            }

            // Image display container
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .offset { IntOffset(0, dismissOffsetY.value.toInt()) }
                    .graphicsLayer {
                        scaleX = scaleAnim.value
                        scaleY = scaleAnim.value
                        translationX = offsetXAnim.value
                        translationY = offsetYAnim.value
                    },
                contentAlignment = Alignment.Center
            ) {
                if (data.bitmap != null) {
                    Image(
                        bitmap = data.bitmap.asImageBitmap(),
                        contentDescription = "Full image preview",
                        contentScale = ContentScale.Fit,
                        modifier = Modifier.fillMaxSize()
                    )
                } else if (!data.url.isNullOrBlank()) {
                    SubcomposeAsyncImage(
                        model = ImageRequest.Builder(context)
                            .data(data.url)
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

            // Top Action Bar (Close, Save, Share)
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

                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    // Save Button
                    Box(
                        modifier = Modifier
                            .size(38.dp)
                            .clip(CircleShape)
                            .background(Color.Black.copy(alpha = 0.5f))
                            .clickable {
                                coroutineScope.launch {
                                    saveImageToGallery(context, data)
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
                                    shareImage(context, data)
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

private suspend fun resolveBitmap(context: Context, data: ImageViewerData): Bitmap? {
    if (data.bitmap != null) return data.bitmap
    if (data.url.isNullOrBlank()) return null
    return withContext(Dispatchers.IO) {
        try {
            val loader = context.imageLoader
            val request = ImageRequest.Builder(context)
                .data(data.url)
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

private suspend fun saveImageToGallery(context: Context, data: ImageViewerData) {
    val bitmap = resolveBitmap(context, data)
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

private suspend fun shareImage(context: Context, data: ImageViewerData) {
    val bitmap = resolveBitmap(context, data)
    if (bitmap == null) {
        if (!data.url.isNullOrBlank()) {
            val shareIntent = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_TEXT, data.url)
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
