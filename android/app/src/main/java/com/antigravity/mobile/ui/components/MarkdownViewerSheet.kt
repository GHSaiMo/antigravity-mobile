package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.widget.Toast
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.FileProvider
import com.antigravity.mobile.data.model.ArtifactSnapshotItem
import com.antigravity.mobile.data.model.MarkdownFileViewerData
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.fetchArtifactSnapshots
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
    cascadeId: String = "",
    urlResolver: ((String) -> String)? = null,
    onImageClick: ((String) -> Unit)? = null
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    var snapshots by remember(cascadeId, data.uri) { mutableStateOf<List<ArtifactSnapshotItem>>(emptyList()) }
    var activeSnapshot by remember(cascadeId, data.uri) { mutableStateOf<ArtifactSnapshotItem?>(null) }
    var showSnapshotsDialog by remember { mutableStateOf(false) }

    LaunchedEffect(cascadeId, data.uri) {
        if (cascadeId.isNotBlank() && data.uri.isNotBlank()) {
            ApiClient.defaultInstance?.fetchArtifactSnapshots(cascadeId, data.uri)?.onSuccess {
                snapshots = it
            }
        }
    }

    val effectiveContent = activeSnapshot?.content ?: data.content

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
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
            SheetGrabHandle(colors.textMuted.copy(alpha = 0.35f))

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 20.dp)
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
                    contentDescription = "保存Markdown".loc,
                    colors = colors,
                    onClick = {
                        haptic.light()
                        coroutineScope.launch {
                            saveMarkdownToDownloads(context, displayFilename.ifBlank { "$displayTitle.md" }, data.content)
                        }
                    }
                )

                // Center: Title
                Text(
                    text = displayTitle,
                    color = colors.textPrimary,
                    fontSize = 15.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier
                        .weight(1f)
                        .padding(horizontal = 10.dp),
                    textAlign = TextAlign.Center
                )

                // Top-right: Artifact History button (only when >1 snapshots) + Apple native Share button
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (snapshots.size > 1) {
                        ApplePreviewCircleButton(
                            icon = Icons.Default.History,
                            contentDescription = "历史版本快照".loc,
                            colors = colors,
                            onClick = {
                                haptic.light()
                                showSnapshotsDialog = true
                            }
                        )
                    }
                    ApplePreviewCircleButton(
                        icon = Icons.Default.Share,
                        contentDescription = "分享Markdown".loc,
                        colors = colors,
                        onClick = {
                            haptic.medium()
                            shareMarkdown(context, displayFilename.ifBlank { "$displayTitle.md" }, effectiveContent)
                        }
                    )
                }
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

                    // Markdown Document Content
                    if (activeSnapshot != null) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                text = "正在查看历史快照 v%d".loc((activeSnapshot?.index ?: 0) + 1),
                                fontSize = 12.sp,
                                color = colors.accentOrange,
                                fontWeight = FontWeight.Medium
                            )
                            TextButton(
                                onClick = { activeSnapshot = null },
                                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                            ) {
                                Text("返回最新版".loc, fontSize = 12.sp, color = colors.accentIndigo)
                            }
                        }
                    }
                    MarkdownContentView(
                        content = effectiveContent,
                        urlResolver = urlResolver,
                        onImageClick = onImageClick
                    )

                    // Scrolls to the sheet's bottom edge; this keeps the last line clear of the gesture bar.
                    Spacer(modifier = Modifier.navigationBarsPadding().height(24.dp))
                }
            }

            // Bottom Fixed Proceed Bar (when planning mode artifact needs approval)
            if (data.canProceed) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 10.dp)
                        .navigationBarsPadding()
                        .padding(bottom = 16.dp)
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
                                text = "方案查阅完毕".loc,
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium
                            )
                            Text(
                                text = "点击立即进入自动化执行".loc,
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
                                text = "确认执行 (Proceed)".loc,
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

    if (showSnapshotsDialog && snapshots.size > 1) {
        ArtifactSnapshotsDialog(
            snapshots = snapshots,
            onSelectSnapshot = { snap -> activeSnapshot = snap },
            onDismiss = { showSnapshotsDialog = false }
        )
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
                    Toast.makeText(context, "已保存到下载目录".loc, Toast.LENGTH_SHORT).show()
                }
            } else {
                val targetDir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                val destFile = File(targetDir, safeName)
                destFile.writeText(content, Charsets.UTF_8)
                withContext(Dispatchers.Main) {
                    Toast.makeText(context, "已保存到下载目录".loc, Toast.LENGTH_SHORT).show()
                }
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) {
                Toast.makeText(context, "保存失败: %s".loc(e.localizedMessage ?: e.message.orEmpty()), Toast.LENGTH_SHORT).show()
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
        com.antigravity.mobile.ui.util.ShareImageUtils.startShare(context, shareIntent, "分享Markdown文档".loc)
    } catch (e: Exception) {
        Toast.makeText(context, "分享失败: %s".loc(e.message.orEmpty()), Toast.LENGTH_SHORT).show()
    }
}
