package com.antigravity.mobile.ui.components

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.FileProvider
import com.antigravity.mobile.data.model.MarkdownFileViewerData
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.net.URLDecoder

/**
 * Android Markdown Document & Artifact Viewer Sheet (aligned with iOS QuickLookPreviewSheet).
 * Features:
 * - Top-centered iOS-style drag handle indicator (36x5dp capsule).
 * - Top navigation bar: Top-left Apple native Save button, Center Title, Top-right Apple native Share button.
 * - Sheet container with 16dp rounded top corners.
 * - Markdown preview with interactive proceed bar for planning mode artifacts.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MarkdownViewerSheet(
    data: MarkdownFileViewerData,
    onProceed: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((String) -> Unit)? = null
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    val rawTitle = data.title.ifBlank { data.filename }
    val displayTitle = remember(rawTitle) {
        try { URLDecoder.decode(rawTitle, "UTF-8") } catch (_: Exception) { rawTitle }
    }
    val rawFilename = data.filename
    val displayFilename = remember(rawFilename) {
        try { URLDecoder.decode(rawFilename, "UTF-8") } catch (_: Exception) { rawFilename }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
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
        windowInsets = WindowInsets(0, 0, 0, 0),
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight(0.94f)
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .navigationBarsPadding()
                .padding(horizontal = 20.dp)
                .padding(bottom = 24.dp)
        ) {
            // Top Navigation Bar (Apple Native Component Layout)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                // Top-left: Apple native Save button
                ApplePreviewCircleButton(
                    icon = Icons.Default.FileDownload,
                    contentDescription = "保存Markdown",
                    colors = colors,
                    onClick = {
                        haptic.light()
                        coroutineScope.launch {
                            saveMarkdownToDownloads(context, displayFilename.ifBlank { "$displayTitle.md" }, data.content)
                        }
                    }
                )

                // Center: Title & Filename
                Column(
                    modifier = Modifier
                        .weight(1f)
                        .padding(horizontal = 12.dp),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Text(
                        text = displayTitle,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        textAlign = TextAlign.Center
                    )
                    if (displayFilename.isNotBlank() && displayFilename != displayTitle) {
                        Text(
                            text = displayFilename,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            textAlign = TextAlign.Center
                        )
                    }
                }

                // Top-right: Apple native Share button
                ApplePreviewCircleButton(
                    icon = Icons.Default.Share,
                    contentDescription = "分享Markdown",
                    colors = colors,
                    onClick = {
                        haptic.medium()
                        shareMarkdown(context, displayFilename.ifBlank { "$displayTitle.md" }, data.content)
                    }
                )
            }

            HorizontalDivider(color = colors.separator.copy(alpha = 0.5f), thickness = 0.5.dp, modifier = Modifier.padding(vertical = 10.dp))

            // Body
            if (data.isLoading) {
                Box(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxWidth(),
                    contentAlignment = Alignment.Center
                ) {
                    CircularProgressIndicator(color = colors.accentIndigo)
                }
            } else {
                Column(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxWidth()
                        .verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    // Companion Summary card if present
                    data.summary?.takeIf { it.isNotBlank() }?.let { summary ->
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.accentIndigo.copy(alpha = 0.10f))
                                .padding(14.dp)
                        ) {
                            Text(
                                text = "📋 实施方案概要",
                                color = colors.accentIndigo,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.Bold
                            )
                            Text(
                                text = summary,
                                color = colors.textPrimary,
                                fontSize = 13.sp,
                                lineHeight = 18.sp,
                                modifier = Modifier.padding(top = 4.dp)
                            )
                        }
                    }

                    // Markdown Document Content
                    MarkdownContentView(
                        content = data.content,
                        urlResolver = urlResolver,
                        onImageClick = onImageClick
                    )
                }
            }

            // Bottom Fixed Proceed Bar (when planning mode artifact needs approval)
            if (data.canProceed) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 10.dp)
                ) {
                    HorizontalDivider(
                        color = colors.separator.copy(alpha = 0.5f),
                        thickness = 0.5.dp,
                        modifier = Modifier.padding(bottom = 10.dp)
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Column(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(2.dp)
                        ) {
                            Text(
                                text = "方案查阅完毕",
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium
                            )
                            Text(
                                text = "点击立即进入自动化执行",
                                color = colors.textPrimary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }

                        Button(
                            onClick = onProceed,
                            shape = RoundedCornerShape(10.dp),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = colors.accentBlue,
                                contentColor = Color.White
                            ),
                            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 9.dp),
                            modifier = Modifier.height(38.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.PlayArrow,
                                contentDescription = "Proceed",
                                modifier = Modifier.size(14.dp)
                            )
                            Spacer(modifier = Modifier.width(6.dp))
                            Text(
                                text = "确认执行 (Proceed)",
                                fontSize = 13.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                        }
                    }
                }
            }
        }
    }
}

private suspend fun saveMarkdownToDownloads(context: Context, filename: String, content: String) {
    withContext(Dispatchers.IO) {
        try {
            val safeName = if (filename.endsWith(".md", ignoreCase = true)) filename else "$filename.md"
            val resolver = context.contentResolver
            val contentValues = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, safeName)
                put(MediaStore.MediaColumns.MIME_TYPE, "text/markdown")
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/Antigravity")
                    put(MediaStore.MediaColumns.IS_PENDING, 1)
                }
            }
            val collectionUri = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                MediaStore.Downloads.EXTERNAL_CONTENT_URI
            } else {
                MediaStore.Files.getContentUri("external")
            }
            val itemUri = resolver.insert(collectionUri, contentValues)
            if (itemUri != null) {
                resolver.openOutputStream(itemUri)?.use { out ->
                    out.write(content.toByteArray(Charsets.UTF_8))
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    contentValues.clear()
                    contentValues.put(MediaStore.MediaColumns.IS_PENDING, 0)
                    resolver.update(itemUri, contentValues, null, null)
                }
                withContext(Dispatchers.Main) {
                    Toast.makeText(context, "已保存到下载目录", Toast.LENGTH_SHORT).show()
                }
            } else {
                val targetDir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                val destFile = File(targetDir, safeName)
                destFile.writeText(content, Charsets.UTF_8)
                withContext(Dispatchers.Main) {
                    Toast.makeText(context, "已保存到下载目录", Toast.LENGTH_SHORT).show()
                }
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) {
                Toast.makeText(context, "保存失败: ${e.localizedMessage ?: e.message}", Toast.LENGTH_SHORT).show()
            }
        }
    }
}

private fun shareMarkdown(context: Context, filename: String, content: String) {
    try {
        val safeName = if (filename.endsWith(".md", ignoreCase = true)) filename else "$filename.md"
        val cacheFile = File(context.cacheDir, safeName)
        cacheFile.writeText(content, Charsets.UTF_8)

        val uri = FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            cacheFile
        )
        val shareIntent = Intent(Intent.ACTION_SEND).apply {
            type = "text/markdown"
            putExtra(Intent.EXTRA_STREAM, uri)
            putExtra(Intent.EXTRA_TEXT, content)
            putExtra(Intent.EXTRA_SUBJECT, safeName)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        context.startActivity(Intent.createChooser(shareIntent, "分享Markdown文档"))
    } catch (e: Exception) {
        Toast.makeText(context, "分享失败: ${e.message}", Toast.LENGTH_SHORT).show()
    }
}
