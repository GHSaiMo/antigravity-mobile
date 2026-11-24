package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.MoreHoriz
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.VideocamOff
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.SubcomposeAsyncImage
import com.antigravity.mobile.data.model.WeChatArchiveSessionItem
import com.antigravity.mobile.data.model.WeChatChatArchive
import com.antigravity.mobile.data.model.WeChatChatMessage
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.theme.AppColors
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch
import java.io.File

private val weChatAvatarPalette = listOf(
    Color(0xFF07C160),
    Color(0xFF10AEFF),
    Color(0xFFFA9D3B),
    Color(0xFF6467F0),
    Color(0xFFF4606C),
    Color(0xFF19B5FE),
    Color(0xFFA06CD5),
    Color(0xFF2AC993)
)

@OptIn(ExperimentalMaterial3Api::class, androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
fun WeChatChatArchiveSheet(
    item: WeChatArchiveSessionItem,
    apiClient: ApiClient,
    cascadeId: String,
    onFallbackToDocumentPreview: () -> Unit,
    onDismiss: () -> Unit,
    colors: AppColors = AntigravityTheme.colors
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val haptic = rememberHaptic()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    val isDark = colors.isDark
    val headerBg = if (isDark) Color(0xFF191919) else Color(0xFFEDEDED)
    val listBg = if (isDark) Color(0xFF111111) else Color(0xFFFFFFFF)
    val textPrimary = if (isDark) Color(0xFFE6E6E6) else Color(0xFF191919)
    val textMuted = if (isDark) Color(0xFF8E8E93) else Color(0xFF888888)
    val dividerColor = if (isDark) Color(0xFF262626) else Color(0xFFEDEDED)
    val cardBg = if (isDark) Color(0xFF1E1E1E) else Color(0xFFF3F3F3)
    val mentionBlue = if (isDark) Color(0xFF7D90A9) else Color(0xFF576B95)

    var archive by remember(item.uri) { mutableStateOf<WeChatChatArchive?>(null) }
    var isLoading by remember(item.uri) { mutableStateOf(true) }
    var errorMessage by remember(item.uri) { mutableStateOf<String?>(null) }
    var reloadKey by remember(item.uri) { mutableIntStateOf(0) }

    var menuExpanded by remember { mutableStateOf(false) }
    var isSearching by remember { mutableStateOf(false) }
    var searchQuery by remember { mutableStateOf("") }
    var isDownloadingZip by remember { mutableStateOf(false) }
    var isDownloadingEntry by remember { mutableStateOf(false) }

    var galleryData by remember { mutableStateOf<ImageViewerData?>(null) }
    var entryPreviewFile by remember { mutableStateOf<Pair<File, String>?>(null) }

    LaunchedEffect(item.uri, reloadKey) {
        isLoading = true
        errorMessage = null
        val res = apiClient.fetchWeChatChatArchive(item.uri, cascadeId)
        res.onSuccess { parsed ->
            if (!parsed.isWeChatArchive) {
                onDismiss()
                onFallbackToDocumentPreview()
            } else {
                archive = parsed
                isLoading = false
            }
        }.onFailure { err ->
            errorMessage = "解析聊天记录失败：%s".loc(err.localizedMessage ?: err.message ?: "未知错误".loc)
            isLoading = false
        }
    }

    val filteredMessages = remember(archive, searchQuery) {
        val all = archive?.messages.orEmpty()
        val q = searchQuery.trim().lowercase()
        if (q.isEmpty()) all
        else all.filter {
            it.sender.lowercase().contains(q) ||
                it.text.lowercase().contains(q) ||
                (it.mediaName?.lowercase()?.contains(q) == true)
        }
    }

    val imageEntries = remember(archive, item.uri, cascadeId) {
        archive?.messages.orEmpty().mapNotNull { msg ->
            if (msg.kind == "image" && !msg.mediaEntry.isNullOrBlank()) {
                val url = apiClient.buildZipEntryUrl(item.uri, msg.mediaEntry, cascadeId)
                msg.id to ImageViewerItem(url = url, title = msg.mediaName ?: "图片".loc)
            } else null
        }
    }

    fun copyToClipboard(label: String, text: String, toast: String) {
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText(label, text))
        Toast.makeText(context, toast, Toast.LENGTH_SHORT).show()
    }

    fun openEntryInDocumentPreview(entry: String, fileName: String) {
        if (isDownloadingEntry) return
        val url = apiClient.buildZipEntryUrl(item.uri, entry, cascadeId)
        if (url.isBlank()) return
        scope.launch {
            isDownloadingEntry = true
            val safeName = fileName.replace(Regex("""[\\/:*?"<>|]"""), "_").ifBlank { "entry" }
            val cacheDir = File(context.cacheDir, "wechat_entries").apply { mkdirs() }
            val target = File(cacheDir, "${(item.uri + "|" + entry).hashCode().toUInt()}_$safeName")
            if (target.exists() && target.length() > 0) {
                isDownloadingEntry = false
                entryPreviewFile = target to fileName
                return@launch
            }
            val dl = apiClient.downloadFile(url, target, cascadeId)
            isDownloadingEntry = false
            dl.onSuccess { f ->
                entryPreviewFile = f to fileName
            }.onFailure {
                Toast.makeText(context, "打开媒体失败: %s".loc(it.message.orEmpty()), Toast.LENGTH_SHORT).show()
            }
        }
    }

    fun downloadOriginalZip(shareAfter: Boolean) {
        if (isDownloadingZip) return
        scope.launch {
            isDownloadingZip = true
            val safeName = item.fileName.replace(Regex("""[\\/:*?"<>|]"""), "_").ifBlank { "聊天记录.zip" }
            val cacheDir = File(context.cacheDir, "wechat_zips").apply { mkdirs() }
            val target = File(cacheDir, "${item.uri.hashCode().toUInt()}_$safeName")
            val dl = if (target.exists() && target.length() > 0) {
                Result.success(target)
            } else {
                apiClient.downloadFile(item.uri, target, cascadeId)
            }
            isDownloadingZip = false
            dl.onSuccess { file ->
                if (shareAfter) {
                    shareDocument(context, file, item.fileName)
                } else {
                    saveFileToDownloads(context, file, item.fileName)
                }
            }.onFailure {
                Toast.makeText(context, "获取原始 ZIP 失败: %s".loc(it.message.orEmpty()), Toast.LENGTH_SHORT).show()
            }
        }
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
        modifier = Modifier.fillMaxSize()
    ) {
        TallSheetBody(containerColor = listBg, onDismiss = onDismiss) {
            Column(modifier = Modifier.fillMaxSize().background(listBg)) {
                // Header
                Column(modifier = Modifier.fillMaxWidth().background(headerBg)) {
                    SheetGrabHandle(textMuted.copy(alpha = 0.35f))
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 8.dp, vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        IconButton(onClick = onDismiss) {
                            Icon(
                                imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                                contentDescription = "关闭".loc,
                                tint = textPrimary
                            )
                        }
                        Column(
                            modifier = Modifier.weight(1f).padding(horizontal = 6.dp),
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            Text(
                                text = archive?.title?.ifBlank { "群聊的聊天记录".loc } ?: "群聊的聊天记录".loc,
                                color = textPrimary,
                                fontSize = 16.5.sp,
                                fontWeight = FontWeight.SemiBold,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                            val cleanFallbackName = remember(item.fileName) {
                                val f = item.fileName
                                if (f.length > 17 && f[16] == '-' && f.substring(0, 16).all { it in '0'..'9' || it in 'a'..'f' }) {
                                    f.substring(17)
                                } else f
                            }
                            val sub = archive?.dateRange?.takeIf { it.isNotBlank() } ?: cleanFallbackName
                            Text(
                                text = sub,
                                color = textMuted,
                                fontSize = 11.5.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        Box {
                            IconButton(onClick = { menuExpanded = true }) {
                                Icon(
                                    imageVector = Icons.Default.MoreHoriz,
                                    contentDescription = "更多操作".loc,
                                    tint = textPrimary
                                )
                            }
                            DropdownMenu(
                                expanded = menuExpanded,
                                onDismissRequest = { menuExpanded = false },
                                containerColor = cardBg
                            ) {
                                DropdownMenuItem(
                                    text = { Text(if (isSearching) "关闭搜索".loc else "搜索聊天记录".loc) },
                                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                                    onClick = {
                                        menuExpanded = false
                                        isSearching = !isSearching
                                        if (!isSearching) searchQuery = ""
                                    }
                                )
                                archive?.rawText?.takeIf { it.isNotBlank() }?.let { raw ->
                                    DropdownMenuItem(
                                        text = { Text("复制全部文本".loc) },
                                        leadingIcon = { Icon(Icons.Default.ContentCopy, contentDescription = null) },
                                        onClick = {
                                            menuExpanded = false
                                            copyToClipboard("wechat_chat", raw, "已复制全部聊天记录".loc)
                                        }
                                    )
                                }
                                DropdownMenuItem(
                                    text = { Text("保存原始 ZIP".loc) },
                                    leadingIcon = { Icon(Icons.Default.FileDownload, contentDescription = null) },
                                    onClick = {
                                        menuExpanded = false
                                        downloadOriginalZip(shareAfter = false)
                                    }
                                )
                                DropdownMenuItem(
                                    text = { Text("分享原始 ZIP".loc) },
                                    leadingIcon = { Icon(Icons.Default.Share, contentDescription = null) },
                                    onClick = {
                                        menuExpanded = false
                                        downloadOriginalZip(shareAfter = true)
                                    }
                                )
                            }
                        }
                    }

                    if (isSearching) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 12.dp, vertical = 6.dp)
                                .clip(RoundedCornerShape(8.dp))
                                .background(listBg)
                                .padding(horizontal = 10.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(Icons.Default.Search, contentDescription = null, tint = textMuted, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(8.dp))
                            Box(modifier = Modifier.weight(1f)) {
                                if (searchQuery.isEmpty()) {
                                    Text("搜索群成员或聊天内容".loc, color = textMuted, fontSize = 14.sp)
                                }
                                BasicTextField(
                                    value = searchQuery,
                                    onValueChange = { searchQuery = it },
                                    singleLine = true,
                                    textStyle = TextStyle(color = textPrimary, fontSize = 14.sp),
                                    cursorBrush = SolidColor(Color(0xFF07C160)),
                                    modifier = Modifier.fillMaxWidth()
                                )
                            }
                            if (searchQuery.isNotEmpty()) {
                                Icon(
                                    imageVector = Icons.Default.Close,
                                    contentDescription = "清除".loc,
                                    tint = textMuted,
                                    modifier = Modifier.size(16.dp).clickable { searchQuery = "" }
                                )
                            }
                        }
                    }
                }

                HorizontalDivider(color = dividerColor, thickness = 0.6.dp)

                // Body
                Box(modifier = Modifier.fillMaxWidth().weight(1f)) {
                    when {
                        isLoading -> {
                            Column(
                                modifier = Modifier.fillMaxSize(),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.Center
                            ) {
                                CircularProgressIndicator(color = Color(0xFF07C160), modifier = Modifier.size(30.dp))
                                Spacer(Modifier.height(10.dp))
                                Text("正在解析微信聊天记录...".loc, color = textMuted, fontSize = 13.5.sp)
                            }
                        }
                        errorMessage != null -> {
                            Column(
                                modifier = Modifier.fillMaxSize().padding(28.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.Center
                            ) {
                                Icon(Icons.Default.Warning, contentDescription = null, tint = Color(0xFFFF9500), modifier = Modifier.size(36.dp))
                                Spacer(Modifier.height(10.dp))
                                Text(errorMessage!!, color = textMuted, fontSize = 14.sp, textAlign = TextAlign.Center)
                                Spacer(Modifier.height(16.dp))
                                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                    Button(onClick = { reloadKey++ }) { Text("重试".loc) }
                                    OutlinedButton(onClick = {
                                        onDismiss()
                                        onFallbackToDocumentPreview()
                                    }) { Text("打开压缩包".loc) }
                                }
                            }
                        }
                        filteredMessages.isEmpty() -> {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                Text(
                                    text = if (isSearching) "没有匹配的聊天记录".loc else "聊天记录为空".loc,
                                    color = textMuted,
                                    fontSize = 14.sp
                                )
                            }
                        }
                        else -> {
                            LazyColumn(
                                modifier = Modifier.fillMaxSize(),
                                contentPadding = PaddingValues(
                                    bottom = 24.dp + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
                                )
                            ) {
                                items(filteredMessages, key = { it.id }) { msg ->
                                    WeChatMessageRow(
                                        msg = msg,
                                        item = item,
                                        apiClient = apiClient,
                                        cascadeId = cascadeId,
                                        textPrimary = textPrimary,
                                        textMuted = textMuted,
                                        dividerColor = dividerColor,
                                        cardBg = cardBg,
                                        mentionBlue = mentionBlue,
                                        onImageClick = { msgId ->
                                            haptic.light()
                                            val idx = imageEntries.indexOfFirst { it.first == msgId }.coerceAtLeast(0)
                                            if (imageEntries.isNotEmpty()) {
                                                galleryData = ImageViewerData(
                                                    items = imageEntries.map { it.second },
                                                    initialIndex = idx
                                                )
                                            }
                                        },
                                        onEntryClick = { entry, name ->
                                            haptic.light()
                                            openEntryInDocumentPreview(entry, name)
                                        },
                                        onLongPressCopy = { text ->
                                            haptic.medium()
                                            copyToClipboard("wechat_msg", text, "已复制消息".loc)
                                        }
                                    )
                                }
                            }
                        }
                    }

                    if (isDownloadingEntry || isDownloadingZip) {
                        Box(
                            modifier = Modifier
                                .align(Alignment.Center)
                                .clip(RoundedCornerShape(12.dp))
                                .background(Color.Black.copy(alpha = 0.72f))
                                .padding(horizontal = 20.dp, vertical = 14.dp)
                        ) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                CircularProgressIndicator(color = Color.White, modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
                                Text("正在加载...".loc, color = Color.White, fontSize = 13.sp)
                            }
                        }
                    }
                }
            }
        }
    }

    galleryData?.let { data ->
        ImageViewerSheet(
            data = data,
            onDismiss = { galleryData = null }
        )
    }

    entryPreviewFile?.let { (file, title) ->
        DocumentPreviewSheet(
            file = file,
            title = title,
            onDismiss = { entryPreviewFile = null },
            colors = colors
        )
    }
}

@OptIn(androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
private fun WeChatMessageRow(
    msg: WeChatChatMessage,
    item: WeChatArchiveSessionItem,
    apiClient: ApiClient,
    cascadeId: String,
    textPrimary: Color,
    textMuted: Color,
    dividerColor: Color,
    cardBg: Color,
    mentionBlue: Color,
    onImageClick: (Int) -> Unit,
    onEntryClick: (String, String) -> Unit,
    onLongPressCopy: (String) -> Unit
) {
    val avatarColor = weChatAvatarPalette[kotlin.math.abs(msg.colorIndex) % weChatAvatarPalette.size]
    val copyText = msg.text.ifBlank { msg.mediaName.orEmpty() }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(
                onClick = {},
                onLongClick = { if (copyText.isNotBlank()) onLongPressCopy(copyText) }
            )
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.Top
        ) {
            Box(
                modifier = Modifier
                    .size(38.dp)
                    .clip(RoundedCornerShape(5.dp))
                    .background(avatarColor),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = msg.senderInitial.ifBlank { "?" },
                    color = Color.White,
                    fontSize = 16.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }

            Spacer(Modifier.width(11.dp))

            Column(modifier = Modifier.weight(1f)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = msg.sender,
                        color = textMuted,
                        fontSize = 13.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = msg.displayTime,
                        color = textMuted,
                        fontSize = 11.5.sp,
                        maxLines = 1
                    )
                }

                Spacer(Modifier.height(4.dp))

                when (msg.kind) {
                    "image" -> {
                        val entry = msg.mediaEntry
                        if (!entry.isNullOrBlank()) {
                            val url = apiClient.buildZipEntryUrl(item.uri, entry, cascadeId)
                            SubcomposeAsyncImage(
                                model = url,
                                contentDescription = msg.mediaName ?: "图片".loc,
                                contentScale = ContentScale.Fit,
                                modifier = Modifier
                                    .widthIn(max = 190.dp)
                                    .heightIn(max = 240.dp)
                                    .clip(RoundedCornerShape(4.dp))
                                    .clickable { onImageClick(msg.id) },
                                loading = {
                                    Box(
                                        modifier = Modifier.size(130.dp).background(cardBg),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp)
                                    }
                                },
                                error = {
                                    MissingMediaPlaceholder(Icons.Default.Image, "[图片] 加载失败".loc, cardBg, textMuted)
                                }
                            )
                        } else {
                            MissingMediaPlaceholder(Icons.Default.Image, "[图片] 未随记录导出".loc, cardBg, textMuted)
                        }
                    }

                    "video" -> {
                        val entry = msg.mediaEntry
                        if (!entry.isNullOrBlank()) {
                            val thumbUrl = apiClient.buildZipEntryUrl(item.uri, entry, cascadeId, thumb = true)
                            Box(
                                modifier = Modifier
                                    .width(170.dp)
                                    .height(220.dp)
                                    .clip(RoundedCornerShape(6.dp))
                                    .background(Color(0xFF1C1C1E))
                                    .clickable { onEntryClick(entry, msg.mediaName ?: "video.mp4") },
                                contentAlignment = Alignment.Center
                            ) {
                                SubcomposeAsyncImage(
                                    model = thumbUrl,
                                    contentDescription = msg.mediaName ?: "视频".loc,
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize(),
                                    error = {}
                                )
                                Box(
                                    modifier = Modifier
                                        .size(44.dp)
                                        .clip(CircleShape)
                                        .background(Color.Black.copy(alpha = 0.45f)),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.PlayArrow,
                                        contentDescription = "播放视频".loc,
                                        tint = Color.White,
                                        modifier = Modifier.size(26.dp)
                                    )
                                }
                                if (!msg.durationLabel.isNullOrBlank()) {
                                    Text(
                                        text = msg.durationLabel,
                                        color = Color.White,
                                        fontSize = 11.sp,
                                        modifier = Modifier
                                            .align(Alignment.BottomEnd)
                                            .padding(6.dp)
                                            .clip(RoundedCornerShape(4.dp))
                                            .background(Color.Black.copy(alpha = 0.55f))
                                            .padding(horizontal = 6.dp, vertical = 2.dp)
                                    )
                                }
                            }
                        } else {
                            MissingMediaPlaceholder(Icons.Default.VideocamOff, "[视频] 未随记录导出".loc, cardBg, textMuted)
                        }
                    }

                    "missing_image" -> {
                        MissingMediaPlaceholder(Icons.Default.Image, "[图片] 未随记录导出".loc, cardBg, textMuted)
                    }

                    "missing_video" -> {
                        MissingMediaPlaceholder(Icons.Default.VideocamOff, "[视频] 未随记录导出".loc, cardBg, textMuted)
                    }

                    "sticker" -> {
                        val label = if (msg.text.isBlank()) "[动画表情]".loc else "[动画表情] %s".loc(msg.text)
                        Text(
                            text = label,
                            color = textMuted,
                            fontSize = 14.sp,
                            modifier = Modifier
                                .clip(RoundedCornerShape(6.dp))
                                .background(cardBg)
                                .padding(horizontal = 10.dp, vertical = 6.dp)
                        )
                    }

                    "file" -> {
                        val entry = msg.mediaEntry
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(8.dp))
                                .background(cardBg)
                                .clickable(enabled = !entry.isNullOrBlank()) {
                                    if (!entry.isNullOrBlank()) onEntryClick(entry, msg.mediaName ?: "file")
                                }
                                .padding(10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            FileTypeBadge(msg.mediaName ?: "file", 34.dp)
                            Column {
                                Text(
                                    text = msg.mediaName ?: msg.text,
                                    color = textPrimary,
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.Medium,
                                    maxLines = 2,
                                    overflow = TextOverflow.Ellipsis
                                )
                                msg.mediaSize?.takeIf { it > 0 }?.let { sz ->
                                    Text(text = formatFileSize(sz), color = textMuted, fontSize = 11.5.sp)
                                }
                            }
                        }
                    }

                    else -> {
                        val annotated = remember(msg.text, mentionBlue, textPrimary) {
                            buildAnnotatedString {
                                val mentionRegex = Regex("""@[^\s\n]+""")
                                var last = 0
                                for (m in mentionRegex.findAll(msg.text)) {
                                    if (m.range.first > last) {
                                        append(msg.text.substring(last, m.range.first))
                                    }
                                    withStyle(SpanStyle(color = mentionBlue)) {
                                        append(m.value)
                                    }
                                    last = m.range.last + 1
                                }
                                if (last < msg.text.length) {
                                    append(msg.text.substring(last))
                                }
                            }
                        }
                        Text(
                            text = annotated,
                            color = textPrimary,
                            fontSize = 15.5.sp,
                            lineHeight = 22.sp
                        )
                    }
                }
            }
        }

        HorizontalDivider(
            color = dividerColor,
            thickness = 0.6.dp,
            modifier = Modifier.padding(start = 63.dp)
        )
    }
}

@Composable
private fun MissingMediaPlaceholder(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    cardBg: Color,
    textMuted: Color
) {
    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(6.dp))
            .background(cardBg)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Icon(icon, contentDescription = null, tint = textMuted, modifier = Modifier.size(18.dp))
        Text(text = label, color = textMuted, fontSize = 13.sp)
    }
}
