package com.antigravity.mobile.data.service

import android.content.Context
import java.io.File
import java.security.MessageDigest

/**
 * 1:1 Kotlin port of iOS DocumentCacheManager.swift.
 * Deterministic on-disk caching of previewed documents, PDFs, presentations, and HTML files.
 */
class DocumentCacheManager(private val context: Context) {
    private val cacheDir: File by lazy {
        val dir = File(context.cacheDir, "antigravity_documents")
        if (!dir.exists()) dir.mkdirs()
        dir
    }

    /**
     * Deterministic file location in cache directory.
     */
    fun cacheFile(uri: String, fileName: String, cascadeId: String? = null): File {
        val cleanUri = uri.trim()
        val scopeKey = if (!cascadeId.isNullOrBlank() && !cleanUri.contains(cascadeId)) {
            "${cascadeId}_$cleanUri"
        } else {
            cleanUri
        }
        val hash = sha256Prefix(scopeKey)
        val rawExt = fileName.substringAfterLast('.', "")
        val ext = if (rawExt.isNotEmpty()) rawExt else cleanUri.substringAfterLast('.', "")
        val rawBase = if (fileName.contains('.')) fileName.substringBeforeLast('.') else fileName
        val base = (if (rawBase.isBlank()) "document" else rawBase)
            .replace(Regex("""[/\\:?*\"<>|&]"""), "_")

        val finalName = if (ext.isNotEmpty()) "${base}_$hash.$ext" else "${base}_$hash"
        return File(cacheDir, finalName)
    }

    /**
     * Checks if a valid non-empty cached file exists.
     */
    fun getCachedFile(uri: String, fileName: String, cascadeId: String? = null): File? {
        val file = cacheFile(uri, fileName, cascadeId)
        if (file.exists() && file.length() > 0) {
            return file
        }
        if (cascadeId != null) {
            val unscoped = cacheFile(uri, fileName, null)
            if (unscoped.exists() && unscoped.length() > 0) {
                return unscoped
            }
        }
        return null
    }

    /**
     * Clears all cached documents.
     */
    fun clearCache() {
        cacheDir.deleteRecursively()
        cacheDir.mkdirs()
    }

    private fun sha256Prefix(input: String): String {
        val md = MessageDigest.getInstance("SHA-256")
        val digest = md.digest(input.toByteArray(Charsets.UTF_8))
        return digest.take(4).joinToString("") { "%02x".format(it) }
    }
}
