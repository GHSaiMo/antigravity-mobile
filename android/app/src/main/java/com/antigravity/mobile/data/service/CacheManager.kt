package com.antigravity.mobile.data.service

import android.content.Context
import android.util.Log
import com.antigravity.mobile.data.model.CachedChatSession
import com.antigravity.mobile.data.model.ConversationItem
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
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

    private val memSessions = ConcurrentHashMap<String, CachedChatSession>()

    /**
     * Persist chat session to memory and asynchronously write to disk.
     */
    fun saveSession(session: CachedChatSession) {
        val cascadeId = session.cascadeId
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_")) return

        memSessions[cascadeId] = session

        ioScope.launch {
            try {
                val dataStr = json.encodeToString(session)
                val targetFile = File(sessionsDir, "$cascadeId.json")
                val tempFile = File(sessionsDir, "$cascadeId.json.tmp")
                tempFile.writeText(dataStr, Charsets.UTF_8)
                if (tempFile.exists()) {
                    if (targetFile.exists()) {
                        targetFile.delete()
                    }
                    tempFile.renameTo(targetFile)
                }
            } catch (e: Exception) {
                Log.w(TAG, "Failed to persist session $cascadeId to disk: ${e.message}")
            }
        }
    }

    /**
     * Load session from memory cache or disk.
     * Returns null if no cache is found.
     */
    fun loadSession(cascadeId: String): CachedChatSession? {
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_")) return null

        // 1. Memory cache hit
        memSessions[cascadeId]?.let { return it }

        // 2. Disk cache lookup
        return try {
            val file = File(sessionsDir, "$cascadeId.json")
            if (file.exists() && file.length() > 0) {
                val content = file.readText(Charsets.UTF_8)
                val session = json.decodeFromString<CachedChatSession>(content)
                memSessions[cascadeId] = session
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
            for (cid in cascadeIds) {
                if (cid.isBlank() || cid.startsWith("local_draft_")) continue
                if (memSessions.containsKey(cid)) continue

                try {
                    val file = File(sessionsDir, "$cid.json")
                    if (file.exists() && file.length() > 0) {
                        val content = file.readText(Charsets.UTF_8)
                        val session = json.decodeFromString<CachedChatSession>(content)
                        memSessions[cid] = session
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
        if (cascadeId.isBlank()) return
        memSessions.remove(cascadeId)

        ioScope.launch {
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

    /**
     * Update session title in cached session if present.
     */
    fun updateSessionTitle(cascadeId: String, newTitle: String) {
        val trimmed = newTitle.trim()
        if (cascadeId.isBlank() || trimmed.isBlank() || trimmed == "未命名会话") return

        val existing = memSessions[cascadeId] ?: loadSession(cascadeId)
        if (existing != null && existing.title != trimmed) {
            val updated = existing.copy(title = trimmed)
            saveSession(updated)
        }
    }

    /**
     * Heal conversation title from cached session or first user message if title is missing.
     */
    fun healConversationTitleIfNeeded(item: ConversationItem): ConversationItem {
        val t = item.title.trim()
        if (t.isNotEmpty() && t != "未命名会话" && t != "会话详情") return item

        val session = loadSession(item.id) ?: return item
        val sessionTitle = session.title?.trim()
        if (!sessionTitle.isNullOrBlank() && sessionTitle != "未命名会话" && sessionTitle != "会话详情") {
            return item.copy(title = sessionTitle)
        }

        val firstUserMsg = session.messages.firstOrNull { it.isUser }
        val prompt = firstUserMsg?.effectiveText?.trim()?.lines()?.firstOrNull { it.isNotBlank() }?.trim()
        if (!prompt.isNullOrBlank()) {
            val derived = if (prompt.length > 36) prompt.take(36) + "..." else prompt
            return item.copy(title = derived)
        }

        return item
    }

    /**
     * Clear all cached sessions from memory and disk.
     */
    fun clearAllSessions() {
        memSessions.clear()
        ioScope.launch {
            try {
                sessionsDir.listFiles()?.forEach { it.delete() }
            } catch (e: Exception) {
                Log.w(TAG, "Failed to clear sessions directory: ${e.message}")
            }
        }
    }

    companion object {
        private const val TAG = "CacheManager"
    }
}
