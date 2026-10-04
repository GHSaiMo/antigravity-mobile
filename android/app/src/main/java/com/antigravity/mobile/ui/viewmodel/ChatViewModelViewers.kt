package com.antigravity.mobile.ui.viewmodel

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.ui.components.ImageViewerItem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.URLDecoder

internal fun ChatViewModel.openMarkdownViewer(uri: String, title: String) {
    val isPlan = uri.contains("implementation_plan", ignoreCase = true) || title.contains("implementation_plan", ignoreCase = true)
    val targetUri = if (isPlan && !_uiState.value.proceedArtifactUri.isNullOrBlank()) {
        _uiState.value.proceedArtifactUri!!
    } else {
        uri
    }

    val rawFilename = targetUri.substringAfterLast('/')
    val decodedFilename = try { URLDecoder.decode(rawFilename, "UTF-8") } catch (_: Exception) { rawFilename }
    val decodedTitle = try { URLDecoder.decode(title, "UTF-8") } catch (_: Exception) { title }
    val initialTitle = when {
        isPlan -> "Implementation Plan"
        decodedTitle.isNotBlank() -> decodedTitle
        else -> decodedFilename
    }

    val isProceedActive = _uiState.value.canProceed && isPlan

    _uiState.value = _uiState.value.copy(
        markdownViewerData = MarkdownFileViewerData(
            uri = targetUri,
            title = initialTitle,
            filename = decodedFilename,
            content = "",
            summary = null,
            canProceed = isProceedActive,
            isLoading = true
        )
    )

    viewModelScope.launch {
        val res = apiClient.fetchFileContent(targetUri, _uiState.value.cascadeId)
        res.onSuccess { resp ->
            val serverFilename = try { URLDecoder.decode(resp.filename, "UTF-8") } catch (_: Exception) { resp.filename }
            val resolvedFilename = serverFilename.ifBlank { decodedFilename }
            val resolvedTitle = if (isPlan) {
                "Implementation Plan"
            } else if (initialTitle.isBlank() || initialTitle == rawFilename || initialTitle.contains("%")) {
                resolvedFilename
            } else {
                initialTitle
            }

            val canProceedFromResp = isProceedActive || (resp.requestFeedback == true && _uiState.value.canProceed)

            _uiState.value = _uiState.value.copy(
                markdownViewerData = _uiState.value.markdownViewerData?.copy(
                    title = resolvedTitle,
                    filename = resolvedFilename,
                    content = resp.content,
                    summary = resp.summary,
                    canProceed = canProceedFromResp,
                    isLoading = false
                )
            )
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                markdownViewerData = _uiState.value.markdownViewerData?.copy(
                    errorMessage = err.message ?: "无法加载文档内容",
                    isLoading = false
                )
            )
        }
    }
}

internal fun ChatViewModel.openAttachmentImageViewer(attachment: AttachmentImage) {
    val selected = _uiState.value.selectedImages
    val index = selected.indexOf(attachment).coerceAtLeast(0)
    val items = if (selected.isNotEmpty()) {
        selected.map { ImageViewerItem(bitmap = it.bitmap, bytes = it.byteArray) }
    } else {
        listOf(ImageViewerItem(bitmap = attachment.bitmap, bytes = attachment.byteArray))
    }
    openImageViewer(items = items, initialIndex = index)

    viewModelScope.launch(Dispatchers.IO) {
        try {
            val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            BitmapFactory.decodeByteArray(attachment.byteArray, 0, attachment.byteArray.size, options)
            val maxDim = maxOf(options.outWidth, options.outHeight)
            var sampleSize = 1
            if (maxDim > 2560) {
                while ((maxDim / (sampleSize * 2)) >= 2560) {
                    sampleSize *= 2
                }
            }
            val decodeOptions = BitmapFactory.Options().apply {
                inSampleSize = sampleSize
                inPreferredConfig = Bitmap.Config.ARGB_8888
            }
            val highRes = BitmapFactory.decodeByteArray(attachment.byteArray, 0, attachment.byteArray.size, decodeOptions)
            if (highRes != null) {
                withContext(Dispatchers.Main) {
                    _uiState.value.imageViewerData?.let { current ->
                        val updatedItems = current.items.toMutableList()
                        if (index in updatedItems.indices) {
                            updatedItems[index] = updatedItems[index].copy(bitmap = highRes)
                            _uiState.value = _uiState.value.copy(
                                imageViewerData = current.copy(items = updatedItems)
                            )
                        }
                    }
                }
            }
        } catch (_: Exception) {
            // Keep existing bitmap preview
        }
    }
}

internal fun ChatViewModel.downloadAndPreviewDocument(uri: String, fileName: String) {
    documentDownloadJob?.cancel()

    val decodedFileName = try { URLDecoder.decode(fileName, "UTF-8") } catch (_: Exception) { fileName }
    val cascadeId = _uiState.value.cascadeId

    // 1. Check local cache first
    val cached = documentCacheManager?.getCachedFile(uri, decodedFileName, cascadeId)
    if (cached != null && cached.exists() && cached.length() > 0) {
        _uiState.value = _uiState.value.copy(
            previewDocumentFile = cached,
            previewDocumentTitle = decodedFileName
        )
        return
    }

    // 2. Prepare target file in cache
    val target = documentCacheManager?.cacheFile(uri, decodedFileName, cascadeId)
        ?: java.io.File.createTempFile("doc_", "_$decodedFileName")

    _uiState.value = _uiState.value.copy(
        isDownloadingDocument = true,
        downloadingDocumentName = decodedFileName,
        downloadProgress = 0f
    )

    documentDownloadJob = viewModelScope.launch {
        val res = apiClient.downloadFile(
            uri = uri,
            targetFile = target,
            cascadeId = cascadeId,
            onProgress = { progress, _, _ ->
                _uiState.value = _uiState.value.copy(downloadProgress = progress)
            }
        )

        _uiState.value = _uiState.value.copy(isDownloadingDocument = false)

        res.onSuccess { downloadedFile ->
            _uiState.value = _uiState.value.copy(
                previewDocumentFile = downloadedFile,
                previewDocumentTitle = decodedFileName
            )
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                errorMessage = "下载文档失败: ${err.message}"
            )
        }
    }
}
