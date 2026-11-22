package com.antigravity.mobile.data.service

import android.content.ComponentCallbacks2
import android.content.Context
import android.util.Log
import android.util.LruCache
import com.antigravity.mobile.data.model.CachedChatSession
import com.antigravity.mobile.data.model.ConversationItem
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.io.File
import java.util.concurrent.ConcurrentHashMap

/**
 * CacheManager manages persistent and in-memory caching for chat sessions and conversations,
 * aligning with iOS CacheManager architecture.
 */
class CacheManager(context: Context) {

    private val appContext = context.applicationContext
    private val ioScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    private val json = JsonConfig.instance

    private val cacheDir = File(appContext.filesDir, "AntigravityCache").apply { mkdirs() }
    private val sessionsDir = File(cacheDir, "sessions").apply { mkdirs() }

    private val memSessions = LruCache<String, CachedChatSession>(MAX_MEM_SESSIONS)
    private val sessionTitleCache = ConcurrentHashMap<String, String>()

    /**
     * Latest not-yet-written snapshot per session. The chat stream saves on every frame (several per
     * second while an agent runs); disk writes are coalesced to one per [SESSION_WRITE_INTERVAL_MS].
     */
    private val pendingWrites = ConcurrentHashMap<String, CachedChatSession>()

    /** Serialises session file operations so two writes never interleave on the same .tmp file. */
    private val fileMutex = Mutex()

    private fun extractAndCacheTitle(session: CachedChatSession): String? {
        val cid = session.cascadeId
        if (!isValidCascadeId(cid)) return null
        val explicit = session.title?.trim()?.takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
        if (explicit != null) {
            sessionTitleCache[cid] = explicit
            return explicit
        }
        val firstUserMsg = session.messages.firstOrNull { it.isUser }
        val prompt = firstUserMsg?.effectiveText?.trim()?.lines()?.firstOrNull { it.isNotBlank() }?.trim()
        if (!prompt.isNullOrBlank()) {
            val derived = if (prompt.length > 36) prompt.take(36) + "..." else prompt
            sessionTitleCache[cid] = derived
            return derived
        }
        return null
    }

    /**
     * Persist chat session to memory immediately and to disk at most once per [SESSION_WRITE_INTERVAL_MS].
     */
    fun saveSession(session: CachedChatSession) {
        val cascadeId = session.cascadeId
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || !isValidCascadeId(cascadeId)) return

        memSessions.put(cascadeId, session)
        extractAndCacheTitle(session)

        val alreadyScheduled = pendingWrites.put(cascadeId, session) != null
        if (!alreadyScheduled) {
            ioScope.launch {
                delay(SESSION_WRITE_INTERVAL_MS)
                writePendingSession(cascadeId)
            }
        }
    }

    /** Writes every pending session now (e.g. when the app leaves the foreground). */
    fun flushPendingSessionWrites() {
        val ids = pendingWrites.keys.toList()
        if (ids.isEmpty()) return
        ioScope.launch {
            ids.forEach { writePendingSession(it) }
        }
    }

    private fun trimmedForDiskPersistence(session: CachedChatSession): CachedChatSession {
        if (session.messages.size <= MAX_DISK_CACHED_MESSAGES) return session
        val dropped = session.messages.size - MAX_DISK_CACHED_MESSAGES
        val tail = session.messages.takeLast(MAX_DISK_CACHED_MESSAGES)
        val adjustedOffset = maxOf(0, session.nextOffset) + dropped
        return session.copy(
            hasMore = true,
            nextOffset = adjustedOffset,
            messages = tail
        )
    }

    private suspend fun writePendingSession(cascadeId: String) {
        if (!isValidCascadeId(cascadeId)) {
            pendingWrites.remove(cascadeId)
            return
        }
        fileMutex.withLock {
            val session = pendingWrites.remove(cascadeId) ?: return
            try {
                val diskSession = trimmedForDiskPersistence(session)
                val dataStr = json.encodeToString(diskSession)
                val targetFile = File(sessionsDir, "$cascadeId.json")
                val tempFile = File(sessionsDir, "$cascadeId.json.tmp")
                tempFile.writeText(dataStr, Charsets.UTF_8)
                if (tempFile.exists()) {
                    if (targetFile.exists()) {
                        targetFile.delete()
                    }
                    tempFile.renameTo(targetFile)
                }
                pruneSessionFiles()
            } catch (e: Exception) {
                Log.w(TAG, "Failed to persist session $cascadeId to disk: ${e.message}")
            }
        }
    }

    /**
     * Bound on-disk session cache growth: keep only the most recently written sessions.
     * filesDir is never reclaimed by the OS, so without this the cache grows forever.
     */
    private fun pruneSessionFiles() {
        val files = sessionsDir.listFiles { f -> f.isFile && f.name.endsWith(".json") } ?: return
        if (files.size <= MAX_CACHED_SESSIONS) return
        files.sortedBy { it.lastModified() }
            .take(files.size - MAX_CACHED_SESSIONS)
            .forEach { it.delete() }
    }

    /**
     * Non-blocking memory-only lookup for hot/prewarmed sessions (never touches disk).
     */
    fun getMemoryCachedSession(cascadeId: String): CachedChatSession? {
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || !isValidCascadeId(cascadeId)) return null
        memSessions.get(cascadeId)?.let { return it }
        return pendingWrites[cascadeId]
    }

    /**
     * Load session from memory cache or disk.
     * Returns null if no cache is found.
     */
    fun loadSession(cascadeId: String): CachedChatSession? {
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || !isValidCascadeId(cascadeId)) return null

        // 1. Memory cache hit
        memSessions.get(cascadeId)?.let { return it }
        pendingWrites[cascadeId]?.let { return it }

        // 2. Disk cache lookup
        return try {
            val file = File(sessionsDir, "$cascadeId.json")
            if (file.exists() && file.length() > 0) {
                val content = file.readText(Charsets.UTF_8)
                val session = json.decodeFromString<CachedChatSession>(content)
                memSessions.put(cascadeId, session)
                extractAndCacheTitle(session)
                session
            } else {
                null
            }
        } catch (e: Exception) {
            Log.w(TAG, "Failed to load session $cascadeId from disk: ${e.message}")
            null
        }
    }

    /**
     * Prewarm session caches into memory asynchronously.
     * Aligns with iOS prewarmSessions(for:).
     */
    fun prewarmSessions(cascadeIds: List<String>) {
        if (cascadeIds.isEmpty()) return
        ioScope.launch {
            for (cid in cascadeIds.take(MAX_MEM_SESSIONS)) {
                if (cid.isBlank() || cid.startsWith("local_draft_") || !isValidCascadeId(cid)) continue
                if (memSessions.get(cid) != null) continue

                try {
                    val file = File(sessionsDir, "$cid.json")
                    if (file.exists() && file.length() > 0) {
                        val content = file.readText(Charsets.UTF_8)
                        val session = json.decodeFromString<CachedChatSession>(content)
                        memSessions.put(cid, session)
                        extractAndCacheTitle(session)
                    }
                } catch (e: Exception) {
                    Log.w(TAG, "Prewarm failed for session $cid: ${e.message}")
                }
            }
        }
    }

    /**
     * Delete session from memory and disk.
     */
    fun deleteSession(cascadeId: String) {
        if (cascadeId.isBlank() || !isValidCascadeId(cascadeId)) return
        memSessions.remove(cascadeId)
        sessionTitleCache.remove(cascadeId)
        pendingWrites.remove(cascadeId)

        ioScope.launch {
            fileMutex.withLock {
                try {
                    val file = File(sessionsDir, "$cascadeId.json")
                    if (file.exists()) {
                        file.delete()
                    }
                    val tmpFile = File(sessionsDir, "$cascadeId.json.tmp")
                    if (tmpFile.exists()) {
                        tmpFile.delete()
                    }
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to delete session file $cascadeId: ${e.message}")
                }
            }
        }
    }

    /**
     * Update session title in cached session if present.
     */
    fun updateSessionTitle(cascadeId: String, newTitle: String) {
        val trimmed = newTitle.trim()
        if (cascadeId.isBlank() || trimmed.isBlank() || trimmed == "未命名会话") return

        val existing = memSessions.get(cascadeId) ?: loadSession(cascadeId)
        if (existing != null && existing.title != trimmed) {
            val updated = existing.copy(title = trimmed)
            saveSession(updated)
        }
    }

    /**
     * Heal conversation title from memory-cached title/session, or load on calling IO thread without blocking Main.
     */
    fun healConversationTitleIfNeeded(item: ConversationItem): ConversationItem {
        val t = item.title.trim()
        if (t.isNotEmpty() && t != "未命名会话" && t != "会话详情") return item
        if (!isValidCascadeId(item.id)) return item

        sessionTitleCache[item.id]?.let { cachedTitle ->
            return item.copy(title = cachedTitle)
        }
        memSessions.get(item.id)?.let { memSession ->
            extractAndCacheTitle(memSession)?.let { return item.copy(title = it) }
            return item
        }
        if (android.os.Looper.myLooper() == android.os.Looper.getMainLooper()) {
            // Never perform synchronous JSON disk reads on Main thread; prewarm in background for next refresh.
            prewarmSessions(listOf(item.id))
            return item
        }

        val session = loadSession(item.id) ?: return item
        val derived = extractAndCacheTitle(session)
        return if (!derived.isNullOrBlank()) item.copy(title = derived) else item
    }

    /**
     * Clear all cached sessions from memory and disk.
     */
    fun clearAllSessions() {
        memSessions.evictAll()
        sessionTitleCache.clear()
        pendingWrites.clear()
        ioScope.launch {
            fileMutex.withLock {
                try {
                    sessionsDir.listFiles()?.forEach { it.delete() }
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to clear sessions directory: ${e.message}")
                }
            }
        }
    }

    fun trimMemory(level: Int) {
        @Suppress("DEPRECATION")
        when {
            level >= ComponentCallbacks2.TRIM_MEMORY_COMPLETE ||
                level == ComponentCallbacks2.TRIM_MEMORY_RUNNING_CRITICAL -> memSessions.evictAll()
            level >= ComponentCallbacks2.TRIM_MEMORY_BACKGROUND ||
                level >= ComponentCallbacks2.TRIM_MEMORY_RUNNING_LOW -> memSessions.trimToSize(6)
        }
    }

    companion object {
        private const val TAG = "CacheManager"
        private const val MAX_MEM_SESSIONS = 24
        private const val MAX_CACHED_SESSIONS = 100
        private const val MAX_DISK_CACHED_MESSAGES = 40
        private const val SESSION_WRITE_INTERVAL_MS = 1500L
        private val SAFE_CASCADE_ID_REGEX = Regex("^(?:local_draft_|draft_)?[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")

        fun isValidCascadeId(id: String?): Boolean {
            if (id.isNullOrBlank()) return false
            return SAFE_CASCADE_ID_REGEX.matches(id)
        }
    }
}
