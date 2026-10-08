package com.antigravity.mobile.ui.util

import android.content.ClipData
import android.content.ClipboardManager
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.widget.Toast
import androidx.core.content.FileProvider
import coil.imageLoader
import coil.request.ImageRequest
import coil.request.SuccessResult
import com.antigravity.mobile.ui.components.ImageViewerItem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.io.FileOutputStream
import java.io.OutputStream

/**
 * 图片「保存到相册 / 系统分享 / 拷贝」的统一实现，供全屏预览器、聊天流长按菜单与长图卡片共用。
 */
object ShareImageUtils {

    /** 把预览项解析为可操作的 Bitmap（优先内存位图，其次字节/URL 经 Coil 解码）。 */
    suspend fun resolveBitmap(context: Context, item: ImageViewerItem): Bitmap? {
        if (item.bitmap != null) return item.bitmap
        val data: Any = item.bytes ?: item.url?.takeIf { it.isNotBlank() } ?: return null
        return withContext(Dispatchers.IO) {
            try {
                val request = ImageRequest.Builder(context)
                    .data(data)
                    .allowHardware(false)
                    .build()
                val result = context.imageLoader.execute(request)
                if (result is SuccessResult) {
                    (result.drawable as? android.graphics.drawable.BitmapDrawable)?.bitmap
                } else null
            } catch (e: Exception) {
                null
            }
        }
    }

    /** 写入系统相册（Android 10+ 走 MediaStore 分区存储，免权限）。 */
    suspend fun saveBitmapToGallery(context: Context, bitmap: Bitmap?) {
        if (bitmap == null) {
            toast(context, "图片未就绪，无法保存")
            return
        }
        withContext(Dispatchers.IO) {
            try {
                val filename = "Antigravity_${System.currentTimeMillis()}.png"
                val fos: OutputStream?
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    val resolver = context.contentResolver
                    val values = ContentValues().apply {
                        put(MediaStore.MediaColumns.DISPLAY_NAME, filename)
                        put(MediaStore.MediaColumns.MIME_TYPE, "image/png")
                        put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_PICTURES + "/Antigravity")
                    }
                    val uri = resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values)
                    fos = uri?.let { resolver.openOutputStream(it) }
                } else {
                    val dir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_PICTURES).toString()
                    fos = FileOutputStream(File(dir, filename))
                }
                fos?.use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
                toast(context, "已保存到相册")
            } catch (e: Exception) {
                toast(context, "保存失败: ${e.localizedMessage}")
            }
        }
    }

    /** 写入 cache 并通过 FileProvider 生成可授权的 content Uri。 */
    private suspend fun writeToCache(context: Context, bitmap: Bitmap, dirName: String): Uri =
        withContext(Dispatchers.IO) {
            val dir = File(context.cacheDir, dirName).apply { mkdirs() }
            val file = File(dir, "share_${System.currentTimeMillis()}.png")
            FileOutputStream(file).use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
            FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
        }

    /** 调起系统分享面板分享位图。 */
    suspend fun shareBitmap(context: Context, bitmap: Bitmap?, fallbackUrl: String? = null) {
        if (bitmap == null) {
            if (!fallbackUrl.isNullOrBlank()) {
                val intent = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_TEXT, fallbackUrl)
                }
                withContext(Dispatchers.Main) {
                    context.startActivity(Intent.createChooser(intent, "分享图片链接").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                }
            } else {
                toast(context, "图片未就绪，无法分享")
            }
            return
        }
        try {
            val uri = writeToCache(context, bitmap, "images")
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "image/png"
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            withContext(Dispatchers.Main) {
                context.startActivity(Intent.createChooser(intent, "分享图片").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            }
        } catch (e: Exception) {
            toast(context, "分享失败: ${e.localizedMessage}")
        }
    }

    /** 将位图作为图片 Uri 写入系统剪贴板。 */
    suspend fun copyBitmapToClipboard(context: Context, bitmap: Bitmap?) {
        if (bitmap == null) {
            toast(context, "图片未就绪，无法拷贝")
            return
        }
        try {
            val uri = writeToCache(context, bitmap, "images")
            withContext(Dispatchers.Main) {
                val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                clipboard.setPrimaryClip(ClipData.newUri(context.contentResolver, "image", uri))
                Toast.makeText(context, "已拷贝图片", Toast.LENGTH_SHORT).show()
            }
        } catch (e: Exception) {
            toast(context, "拷贝失败: ${e.localizedMessage}")
        }
    }

    private suspend fun toast(context: Context, message: String) {
        withContext(Dispatchers.Main) {
            Toast.makeText(context, message, Toast.LENGTH_SHORT).show()
        }
    }
}
