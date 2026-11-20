package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.Manifest
import android.content.ContentUris
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.MediaStore
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AttachFile
import androidx.compose.material.icons.filled.CameraAlt
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.DocumentScanner
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.PhotoLibrary
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val ALBUM_ID_RECENTS = "__recents__"
private const val ALBUM_ID_FAVORITES = "__favorites__"

private data class PhotoAlbumItem(
    val id: String,
    val title: String,
    val count: Int,
    val coverUri: Uri?,
    val isRecents: Boolean
)

private data class PhotoLibrarySnapshot(
    val albums: List<PhotoAlbumItem>,
    val photosByAlbum: Map<String, List<Uri>>
)

// Ask for photo access automatically only once per process; after that the grid shows an explicit tile.
private var photoAccessAskedThisProcess = false

private fun photoPermissions(): Array<String> = when {
    Build.VERSION.SDK_INT >= 34 -> arrayOf(
        Manifest.permission.READ_MEDIA_IMAGES,
        Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED
    )
    Build.VERSION.SDK_INT >= 33 -> arrayOf(Manifest.permission.READ_MEDIA_IMAGES)
    else -> arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE)
}

/** True when the app may read at least some photos (full access, or Android 14 partial access). */
private fun hasPhotoAccess(context: Context): Boolean {
    fun granted(p: String) = ContextCompat.checkSelfPermission(context, p) == PackageManager.PERMISSION_GRANTED
    return when {
        Build.VERSION.SDK_INT >= 34 ->
            granted(Manifest.permission.READ_MEDIA_IMAGES) || granted(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)
        Build.VERSION.SDK_INT >= 33 -> granted(Manifest.permission.READ_MEDIA_IMAGES)
        else -> granted(Manifest.permission.READ_EXTERNAL_STORAGE)
    }
}

/** True on Android 14+ when the user granted only selected-photos access rather than full access. */
private fun hasPartialPhotoAccessOnly(context: Context): Boolean {
    if (Build.VERSION.SDK_INT < 34) return false
    val full = ContextCompat.checkSelfPermission(context, Manifest.permission.READ_MEDIA_IMAGES) == PackageManager.PERMISSION_GRANTED
    val partial = ContextCompat.checkSelfPermission(context, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED) == PackageManager.PERMISSION_GRANTED
    return !full && partial
}

private fun localizeBucketName(raw: String?): String {
    val trimmed = raw?.trim().orEmpty()
    if (trimmed.isEmpty()) return "其他相册".loc
    return when {
        trimmed.equals("Camera", ignoreCase = true) -> "相机".loc
        trimmed.equals("Screenshots", ignoreCase = true) -> "截屏".loc
        else -> trimmed
    }
}

/**
 * Loads all image URIs and groups them into Recents, Favorites (Android 11+), and MediaStore albums
 * ordered by most recent photo descending. Image-only (MediaStore.Images.Media excludes videos).
 */
private suspend fun loadPhotoLibrary(context: Context): PhotoLibrarySnapshot = withContext(Dispatchers.IO) {
    val allUris = mutableListOf<Uri>()
    val favoriteUris = mutableListOf<Uri>()
    val bucketNames = LinkedHashMap<String, String>()
    val bucketUris = LinkedHashMap<String, MutableList<Uri>>()

    try {
        val supportsFavorite = Build.VERSION.SDK_INT >= Build.VERSION_CODES.R
        val projection = if (supportsFavorite) {
            arrayOf(
                MediaStore.Images.Media._ID,
                MediaStore.Images.Media.BUCKET_ID,
                MediaStore.Images.Media.BUCKET_DISPLAY_NAME,
                MediaStore.Images.Media.IS_FAVORITE
            )
        } else {
            arrayOf(
                MediaStore.Images.Media._ID,
                MediaStore.Images.Media.BUCKET_ID,
                MediaStore.Images.Media.BUCKET_DISPLAY_NAME
            )
        }

        val args = Bundle().apply {
            putStringArray(
                android.content.ContentResolver.QUERY_ARG_SORT_COLUMNS,
                arrayOf(MediaStore.Images.Media.DATE_ADDED, MediaStore.Images.Media._ID)
            )
            putInt(
                android.content.ContentResolver.QUERY_ARG_SORT_DIRECTION,
                android.content.ContentResolver.QUERY_SORT_DIRECTION_DESCENDING
            )
        }

        context.contentResolver.query(
            MediaStore.Images.Media.EXTERNAL_CONTENT_URI,
            projection,
            args,
            null
        )?.use { c ->
            val idCol = c.getColumnIndexOrThrow(MediaStore.Images.Media._ID)
            val bucketIdCol = c.getColumnIndex(MediaStore.Images.Media.BUCKET_ID)
            val bucketNameCol = c.getColumnIndex(MediaStore.Images.Media.BUCKET_DISPLAY_NAME)
            val favoriteCol = if (supportsFavorite) c.getColumnIndex(MediaStore.Images.Media.IS_FAVORITE) else -1

            while (c.moveToNext()) {
                val uri = ContentUris.withAppendedId(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, c.getLong(idCol))
                allUris.add(uri)

                if (favoriteCol >= 0 && c.getInt(favoriteCol) == 1) {
                    favoriteUris.add(uri)
                }

                if (bucketIdCol >= 0) {
                    val bucketId = c.getString(bucketIdCol)?.takeIf { it.isNotBlank() } ?: continue
                    val rawName = if (bucketNameCol >= 0) c.getString(bucketNameCol) else null
                    val list = bucketUris.getOrPut(bucketId) {
                        bucketNames[bucketId] = localizeBucketName(rawName)
                        mutableListOf()
                    }
                    list.add(uri)
                }
            }
        }
    } catch (_: Exception) {
        // Permission revoked mid-flight or provider error: show an empty grid.
    }

    val albums = mutableListOf<PhotoAlbumItem>()
    val photosByAlbum = HashMap<String, List<Uri>>()

    albums.add(
        PhotoAlbumItem(
            id = ALBUM_ID_RECENTS,
            title = "最近项目".loc,
            count = allUris.size,
            coverUri = allUris.firstOrNull(),
            isRecents = true
        )
    )
    photosByAlbum[ALBUM_ID_RECENTS] = allUris

    if (favoriteUris.isNotEmpty()) {
        albums.add(
            PhotoAlbumItem(
                id = ALBUM_ID_FAVORITES,
                title = "个人收藏".loc,
                count = favoriteUris.size,
                coverUri = favoriteUris.firstOrNull(),
                isRecents = false
            )
        )
        photosByAlbum[ALBUM_ID_FAVORITES] = favoriteUris
    }

    for ((bucketId, uris) in bucketUris) {
        if (uris.isEmpty()) continue
        val albumKey = "bucket:$bucketId"
        albums.add(
            PhotoAlbumItem(
                id = albumKey,
                title = bucketNames[bucketId] ?: "其他相册".loc,
                count = uris.size,
                coverUri = uris.firstOrNull(),
                isRecents = false
            )
        )
        photosByAlbum[albumKey] = uris
    }

    PhotoLibrarySnapshot(albums = albums, photosByAlbum = photosByAlbum)
}

/**
 * Full-height pull-up sheet behind the chat "+" button: quick action tiles in Recents,
 * WeChat-style album switcher in the title bar, and lazy image-only photo grid with
 * cross-album multi-selection.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun AttachmentPickerSheet(
    remainingImageSlots: Int,
    onDismiss: () -> Unit,
    onPhotosPicked: (List<Uri>) -> Unit,
    isDocumentScannerSupported: Boolean = false,
    onScanDocument: () -> Unit = {},
    onRecordAudio: () -> Unit = {},
    onOpenCamera: () -> Unit,
    onPickFiles: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val coroutineScope = rememberCoroutineScope()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val gridState = rememberLazyGridState()

    var hasAccess by remember { mutableStateOf(hasPhotoAccess(context)) }
    var isPartialOnly by remember { mutableStateOf(hasPartialPhotoAccessOnly(context)) }
    var libraryReloadTick by remember { mutableStateOf(0) }
    var library by remember { mutableStateOf(PhotoLibrarySnapshot(emptyList(), emptyMap())) }
    var selectedAlbumId by remember { mutableStateOf(ALBUM_ID_RECENTS) }
    var showAlbumDrawer by remember { mutableStateOf(false) }
    val selected = remember { mutableStateListOf<Uri>() }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions()
    ) {
        hasAccess = hasPhotoAccess(context)
        isPartialOnly = hasPartialPhotoAccessOnly(context)
        libraryReloadTick++
    }

    LaunchedEffect(Unit) {
        if (!hasAccess && !photoAccessAskedThisProcess) {
            photoAccessAskedThisProcess = true
            permissionLauncher.launch(photoPermissions())
        }
    }
    LaunchedEffect(hasAccess, libraryReloadTick) {
        if (hasAccess) {
            val loaded = loadPhotoLibrary(context)
            library = loaded
            if (loaded.albums.none { it.id == selectedAlbumId }) {
                selectedAlbumId = loaded.albums.firstOrNull()?.id ?: ALBUM_ID_RECENTS
            }
        } else {
            library = PhotoLibrarySnapshot(emptyList(), emptyMap())
            selectedAlbumId = ALBUM_ID_RECENTS
            showAlbumDrawer = false
        }
    }

    val currentAlbum = library.albums.firstOrNull { it.id == selectedAlbumId } ?: library.albums.firstOrNull()
    val isShowingRecents = currentAlbum?.isRecents ?: true
    val currentAlbumTitle = currentAlbum?.title ?: "最近项目".loc
    val currentPhotos = library.photosByAlbum[currentAlbum?.id ?: ALBUM_ID_RECENTS].orEmpty()
    val canSwitchAlbum = hasAccess && library.albums.size > 1
    val chevronRotation by animateFloatAsState(
        targetValue = if (showAlbumDrawer) 180f else 0f,
        animationSpec = spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = Spring.StiffnessMedium),
        label = "albumChevronRotation"
    )

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = Modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.surface, onDismiss = onDismiss) {
            BackHandler {
                if (showAlbumDrawer) {
                    showAlbumDrawer = false
                } else {
                    onDismiss()
                }
            }

            // Grab handle + centered title (WeChat-style album dropdown trigger)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 10.dp, bottom = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .size(width = 38.dp, height = 5.dp)
                        .clip(CircleShape)
                        .background(colors.textMuted.copy(alpha = 0.35f))
                )
            }

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 10.dp),
                contentAlignment = Alignment.Center
            ) {
                if (canSwitchAlbum) {
                    Row(
                        modifier = Modifier
                            .clip(CircleShape)
                            .clickable(
                                interactionSource = remember { MutableInteractionSource() },
                                indication = null
                            ) {
                                haptic.light()
                                showAlbumDrawer = !showAlbumDrawer
                            }
                            .padding(horizontal = 12.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = currentAlbumTitle,
                            fontSize = 18.sp,
                            fontWeight = FontWeight.Bold,
                            color = colors.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Icon(
                            imageVector = Icons.Default.KeyboardArrowDown,
                            contentDescription = "切换相册".loc,
                            tint = colors.textSecondary,
                            modifier = Modifier
                                .size(20.dp)
                                .graphicsLayer { rotationZ = chevronRotation }
                        )
                    }
                } else {
                    Text(
                        text = currentAlbumTitle,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.padding(vertical = 4.dp)
                    )
                }
            }

            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

            // Photo grid + WeChat-style slide-down album drawer overlay
            BoxWithConstraints(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .clipToBounds()
            ) {
                val drawerMaxHeight = minOf(maxHeight * 0.66f, 520.dp)

                Column(modifier = Modifier.fillMaxSize()) {
                    if (isPartialOnly) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 16.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = "已允许访问部分照片".loc,
                                fontSize = 13.sp,
                                color = colors.textSecondary
                            )
                            Text(
                                text = "管理".loc,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.accentIndigo,
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .clickable {
                                        haptic.light()
                                        permissionLauncher.launch(photoPermissions())
                                    }
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
                            )
                        }
                    }

                    if (hasAccess && currentPhotos.isEmpty() && !isShowingRecents) {
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .fillMaxWidth(),
                            contentAlignment = Alignment.Center
                        ) {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(10.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.PhotoLibrary,
                                    contentDescription = null,
                                    tint = colors.textMuted,
                                    modifier = Modifier.size(34.dp)
                                )
                                Text(
                                    text = "该相册暂无照片".loc,
                                    color = colors.textSecondary,
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.Medium
                                )
                            }
                        }
                    } else {
                        LazyVerticalGrid(
                            state = gridState,
                            columns = GridCells.Fixed(4),
                            modifier = Modifier
                                .weight(1f)
                                .fillMaxWidth(),
                            contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            if (isShowingRecents) {
                                item(key = "files") {
                                    TileFrame(
                                        modifier = Modifier
                                            .background(colors.surfaceVariant)
                                            .clickable {
                                                haptic.light()
                                                onPickFiles()
                                            }
                                    ) {
                                        Column(
                                            modifier = Modifier.fillMaxSize(),
                                            horizontalAlignment = Alignment.CenterHorizontally,
                                            verticalArrangement = Arrangement.Center
                                        ) {
                                            Icon(
                                                imageVector = Icons.Default.AttachFile,
                                                contentDescription = "文件".loc,
                                                tint = colors.textPrimary,
                                                modifier = Modifier.size(28.dp)
                                            )
                                            Spacer(Modifier.height(4.dp))
                                            Text("文件".loc, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                                        }
                                    }
                                }
                                item(key = "record") {
                                    TileFrame(
                                        modifier = Modifier
                                            .background(colors.surfaceVariant)
                                            .clickable {
                                                haptic.light()
                                                onRecordAudio()
                                            }
                                    ) {
                                        Column(
                                            modifier = Modifier.fillMaxSize(),
                                            horizontalAlignment = Alignment.CenterHorizontally,
                                            verticalArrangement = Arrangement.Center
                                        ) {
                                            Icon(
                                                imageVector = Icons.Default.Mic,
                                                contentDescription = "录音".loc,
                                                tint = colors.textPrimary,
                                                modifier = Modifier.size(28.dp)
                                            )
                                            Spacer(Modifier.height(4.dp))
                                            Text("录音".loc, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                                        }
                                    }
                                }
                                if (isDocumentScannerSupported) {
                                    item(key = "document_scanner") {
                                        TileFrame(
                                            modifier = Modifier
                                                .background(colors.surfaceVariant)
                                                .clickable {
                                                    haptic.light()
                                                    onScanDocument()
                                                }
                                        ) {
                                            Column(
                                                modifier = Modifier.fillMaxSize(),
                                                horizontalAlignment = Alignment.CenterHorizontally,
                                                verticalArrangement = Arrangement.Center
                                            ) {
                                                Icon(
                                                    imageVector = Icons.Default.DocumentScanner,
                                                    contentDescription = "文稿".loc,
                                                    tint = colors.textPrimary,
                                                    modifier = Modifier.size(28.dp)
                                                )
                                                Spacer(Modifier.height(4.dp))
                                                Text("文稿".loc, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                                            }
                                        }
                                    }
                                }
                                item(key = "camera") {
                                    TileFrame(
                                        modifier = Modifier
                                            .background(colors.surfaceVariant)
                                            .clickable {
                                                haptic.light()
                                                onOpenCamera()
                                            }
                                    ) {
                                        Column(
                                            modifier = Modifier.fillMaxSize(),
                                            horizontalAlignment = Alignment.CenterHorizontally,
                                            verticalArrangement = Arrangement.Center
                                        ) {
                                            Icon(
                                                imageVector = Icons.Default.CameraAlt,
                                                contentDescription = "相机".loc,
                                                tint = colors.textPrimary,
                                                modifier = Modifier.size(28.dp)
                                            )
                                            Spacer(Modifier.height(4.dp))
                                            Text("相机".loc, color = colors.textPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                                        }
                                    }
                                }
                            }
                            if (!hasAccess) {
                                item(key = "grant") {
                                    TileFrame(
                                        modifier = Modifier
                                            .background(colors.surfaceVariant)
                                            .clickable { permissionLauncher.launch(photoPermissions()) }
                                    ) {
                                        Column(
                                            modifier = Modifier
                                                .fillMaxSize()
                                                .padding(6.dp),
                                            horizontalAlignment = Alignment.CenterHorizontally,
                                            verticalArrangement = Arrangement.Center
                                        ) {
                                            Icon(
                                                imageVector = Icons.Default.PhotoLibrary,
                                                contentDescription = null,
                                                tint = colors.textSecondary,
                                                modifier = Modifier.size(24.dp)
                                            )
                                            Spacer(Modifier.height(4.dp))
                                            Text(
                                                "允许访问照片".loc,
                                                color = colors.textSecondary,
                                                fontSize = 11.sp,
                                                textAlign = TextAlign.Center
                                            )
                                        }
                                    }
                                }
                            }
                            items(currentPhotos, key = { "${currentAlbum?.id ?: ALBUM_ID_RECENTS}_$it" }) { uri ->
                                val order = selected.indexOf(uri)
                                val isSelected = order >= 0
                                val thumbnailModel = remember(uri) {
                                    ImageRequest.Builder(context)
                                        .data(uri)
                                        .size(240)
                                        .build()
                                }
                                TileFrame(
                                    modifier = Modifier.clickable {
                                        haptic.light()
                                        if (isSelected) {
                                            selected.remove(uri)
                                        } else if (selected.size < remainingImageSlots) {
                                            selected.add(uri)
                                        }
                                    }
                                ) {
                                    AsyncImage(
                                        model = thumbnailModel,
                                        contentDescription = null,
                                        contentScale = ContentScale.Crop,
                                        modifier = Modifier
                                            .fillMaxSize()
                                            .background(colors.surfaceVariant)
                                    )
                                    if (isSelected) {
                                        Box(
                                            modifier = Modifier
                                                .fillMaxSize()
                                                .background(Color.Black.copy(alpha = 0.25f))
                                        )
                                    }
                                    Box(
                                        modifier = Modifier
                                            .align(Alignment.TopEnd)
                                            .padding(6.dp)
                                            .size(22.dp)
                                            .clip(CircleShape)
                                            .background(if (isSelected) colors.accentIndigo else Color.Black.copy(alpha = 0.18f))
                                            .border(1.5.dp, Color.White, CircleShape),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        if (isSelected) {
                                            Text("${order + 1}", color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.Bold)
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                // Dimming scrim behind the slide-down album drawer
                androidx.compose.animation.AnimatedVisibility(
                    visible = showAlbumDrawer,
                    enter = fadeIn(),
                    exit = fadeOut(),
                    modifier = Modifier.fillMaxSize()
                ) {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .background(Color.Black.copy(alpha = 0.32f))
                            .clickable(
                                interactionSource = remember { MutableInteractionSource() },
                                indication = null
                            ) {
                                showAlbumDrawer = false
                            }
                    )
                }

                // WeChat-style slide-down album list
                androidx.compose.animation.AnimatedVisibility(
                    visible = showAlbumDrawer,
                    enter = slideInVertically(
                        initialOffsetY = { -it },
                        animationSpec = spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = Spring.StiffnessMediumLow)
                    ) + fadeIn(),
                    exit = slideOutVertically(
                        targetOffsetY = { -it },
                        animationSpec = spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = Spring.StiffnessMedium)
                    ) + fadeOut(),
                    modifier = Modifier.align(Alignment.TopCenter)
                ) {
                    val drawerShape = RoundedCornerShape(bottomStart = 18.dp, bottomEnd = 18.dp)
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(max = drawerMaxHeight)
                            .shadow(12.dp, drawerShape)
                            .clip(drawerShape)
                            .background(colors.surface),
                        contentPadding = PaddingValues(vertical = 6.dp)
                    ) {
                        itemsIndexed(library.albums, key = { _, album -> album.id }) { idx, album ->
                            val isSelectedAlbum = album.id == (currentAlbum?.id ?: ALBUM_ID_RECENTS)
                            val coverModel = remember(album.coverUri) {
                                album.coverUri?.let {
                                    ImageRequest.Builder(context)
                                        .data(it)
                                        .size(160)
                                        .build()
                                }
                            }
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable {
                                        haptic.light()
                                        selectedAlbumId = album.id
                                        showAlbumDrawer = false
                                        coroutineScope.launch {
                                            gridState.scrollToItem(0)
                                        }
                                    }
                                    .padding(horizontal = 16.dp, vertical = 9.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                Box(
                                    modifier = Modifier
                                        .size(52.dp)
                                        .clip(RoundedCornerShape(8.dp))
                                        .background(colors.surfaceVariant),
                                    contentAlignment = Alignment.Center
                                ) {
                                    if (coverModel != null) {
                                        AsyncImage(
                                            model = coverModel,
                                            contentDescription = null,
                                            contentScale = ContentScale.Crop,
                                            modifier = Modifier.fillMaxSize()
                                        )
                                    } else {
                                        Icon(
                                            imageVector = Icons.Default.PhotoLibrary,
                                            contentDescription = null,
                                            tint = colors.textMuted,
                                            modifier = Modifier.size(20.dp)
                                        )
                                    }
                                }

                                Column(
                                    modifier = Modifier.weight(1f),
                                    verticalArrangement = Arrangement.spacedBy(2.dp)
                                ) {
                                    Text(
                                        text = album.title,
                                        color = colors.textPrimary,
                                        fontSize = 15.5.sp,
                                        fontWeight = FontWeight.Medium,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = "${album.count}",
                                        color = colors.textSecondary,
                                        fontSize = 13.sp
                                    )
                                }

                                if (isSelectedAlbum) {
                                    Icon(
                                        imageVector = Icons.Default.Check,
                                        contentDescription = null,
                                        tint = colors.accentIndigo,
                                        modifier = Modifier.size(18.dp)
                                    )
                                }
                            }

                            if (idx < library.albums.lastIndex) {
                                HorizontalDivider(
                                    thickness = 0.5.dp,
                                    color = colors.border,
                                    modifier = Modifier.padding(start = 80.dp)
                                )
                            }
                        }
                    }
                }
            }

            // Confirm button, shown only once photos are selected and album drawer is closed.
            if (selected.isNotEmpty() && !showAlbumDrawer) {
                HorizontalDivider(thickness = 0.5.dp, color = colors.border)
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surface)
                        .navigationBarsPadding()
                        .padding(horizontal = 16.dp, vertical = 10.dp)
                        .height(44.dp)
                        .clip(RoundedCornerShape(22.dp))
                        .background(colors.accentIndigo)
                        .clickable {
                            haptic.medium()
                            onPhotosPicked(selected.toList())
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        "添加 (${selected.size})".loc,
                        color = Color.White,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }
            }
        }
    }
}

@Composable
private fun TileFrame(
    modifier: Modifier = Modifier,
    content: @Composable androidx.compose.foundation.layout.BoxScope.() -> Unit
) {
    Box(
        modifier = modifier
            .aspectRatio(1f)
            .clip(RoundedCornerShape(10.dp)),
        content = content
    )
}
