package com.antigravity.mobile.data.service

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.LocalDraftSession
import com.antigravity.mobile.data.model.ProjectItem
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

class PreferencesManager(context: Context) {
    private val appContext = context.applicationContext
    private val securePrefs: SharedPreferences? = try {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            "agy_secure_prefs",
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
        )
    } catch (e: Exception) {
        Log.e("PreferencesManager", "Failed to init EncryptedSharedPreferences: ${e.message}")
        null
    }

    private val prefs: SharedPreferences =
        context.getSharedPreferences("agy_standard_prefs", Context.MODE_PRIVATE)

    init {
        // H-2: If legacy plaintext preferences held credentials, migrate to securePrefs and purge from plaintext
        val legacyToken = prefs.getString(KEY_DEVICE_TOKEN, null)
        if (!legacyToken.isNullOrBlank()) {
            securePrefs?.edit()?.putString(KEY_DEVICE_TOKEN, legacyToken)?.apply()
            prefs.edit().remove(KEY_DEVICE_TOKEN).apply()
        }
        val legacyId = prefs.getString(KEY_DEVICE_ID, null)
        if (!legacyId.isNullOrBlank()) {
            securePrefs?.edit()?.putString(KEY_DEVICE_ID, legacyId)?.apply()
            prefs.edit().remove(KEY_DEVICE_ID).apply()
        }

        // Purge legacy customServerUrl if it was erroneously populated with LAN or cloud/gateway URLs
        val legacyCustom = prefs.getString(KEY_CUSTOM_URL, null)?.trim()?.trimEnd('/')
        if (!legacyCustom.isNullOrBlank()) {
            val lan = prefs.getString(KEY_LAN_URL, null)?.trim()?.trimEnd('/')
            val cloud = prefs.getString(KEY_PRIMARY_CLOUD_URL, null)?.trim()?.trimEnd('/')
            val gateway = prefs.getString(KEY_GATEWAY_URL, null)?.trim()?.trimEnd('/')
            if (legacyCustom.equals(lan, ignoreCase = true) ||
                (!cloud.isNullOrBlank() && legacyCustom.equals(cloud, ignoreCase = true)) ||
                (!gateway.isNullOrBlank() && legacyCustom.equals(gateway, ignoreCase = true) && !ConnectionManager.isLanHost(ConnectionManager.extractHost(gateway)))) {
                prefs.edit().remove(KEY_CUSTOM_URL).apply()
            }
        }
    }

    private val _themeModeFlow = MutableStateFlow(themeMode)
    val themeModeFlow: StateFlow<String> = _themeModeFlow.asStateFlow()

    var gatewayBaseUrl: String?
        get() = prefs.getString(KEY_GATEWAY_URL, null)
        set(value) = prefs.edit().putString(KEY_GATEWAY_URL, value?.trimEnd('/')).apply()

    var deviceToken: String?
        get() = securePrefs?.getString(KEY_DEVICE_TOKEN, null)
        set(value) {
            val sp = securePrefs
            if (sp != null) {
                if (value != null) {
                    sp.edit().putString(KEY_DEVICE_TOKEN, value).apply()
                } else {
                    sp.edit().remove(KEY_DEVICE_TOKEN).apply()
                }
            } else {
                Log.e("PreferencesManager", "Cannot store deviceToken: EncryptedSharedPreferences unavailable. Refusing plain storage.")
            }
        }

    var deviceId: String?
        get() = securePrefs?.getString(KEY_DEVICE_ID, null)
        set(value) {
            val sp = securePrefs
            if (sp != null) {
                if (value != null) {
                    sp.edit().putString(KEY_DEVICE_ID, value).apply()
                } else {
                    sp.edit().remove(KEY_DEVICE_ID).apply()
                }
            } else {
                Log.e("PreferencesManager", "Cannot store deviceId: EncryptedSharedPreferences unavailable.")
            }
        }

    var themeMode: String
        get() = prefs.getString(KEY_THEME_MODE, "system") ?: "system"
        set(value) {
            prefs.edit().putString(KEY_THEME_MODE, value).apply()
            _themeModeFlow.value = value
        }

    var autoApprovePermissions: Boolean
        get() = prefs.getBoolean(KEY_AUTO_APPROVE, false)
        set(value) = prefs.edit().putBoolean(KEY_AUTO_APPROVE, value).apply()

    var enableLiveNotifications: Boolean
        get() = prefs.getBoolean(KEY_LIVE_NOTIFICATIONS, true)
        set(value) = prefs.edit().putBoolean(KEY_LIVE_NOTIFICATIONS, value).apply()

    var primaryCloudUrl: String?
        get() {
            val saved = prefs.getString(KEY_PRIMARY_CLOUD_URL, null)
            if (!saved.isNullOrBlank()) return saved
            val active = gatewayBaseUrl
            if (!active.isNullOrBlank() && !ConnectionManager.isLanHost(ConnectionManager.extractHost(active))) {
                return active
            }
            return null
        }
        set(value) = prefs.edit().putString(KEY_PRIMARY_CLOUD_URL, value?.trimEnd('/')).apply()

    var lanServerUrl: String?
        get() = prefs.getString(KEY_LAN_URL, null)
        set(value) = prefs.edit().putString(KEY_LAN_URL, value?.trimEnd('/')).apply()

    var ipv6ServerUrl: String?
        get() = prefs.getString(KEY_IPV6_URL, null)
        set(value) = prefs.edit().putString(KEY_IPV6_URL, value?.trimEnd('/')).apply()

    var relayServerUrl: String?
        get() = prefs.getString(KEY_RELAY_URL, null)
        set(value) = prefs.edit().putString(KEY_RELAY_URL, value?.trimEnd('/')).apply()

    var customServerUrl: String?
        get() {
            val saved = prefs.getString(KEY_CUSTOM_URL, null)?.trim()?.trimEnd('/')
            if (saved.isNullOrBlank()) return null
            val lan = prefs.getString(KEY_LAN_URL, null)?.trim()?.trimEnd('/')
            val cloud = prefs.getString(KEY_PRIMARY_CLOUD_URL, null)?.trim()?.trimEnd('/')
            val gateway = prefs.getString(KEY_GATEWAY_URL, null)?.trim()?.trimEnd('/')
            if (saved.equals(lan, ignoreCase = true) ||
                (!cloud.isNullOrBlank() && saved.equals(cloud, ignoreCase = true)) ||
                (!gateway.isNullOrBlank() && saved.equals(gateway, ignoreCase = true) && !ConnectionManager.isLanHost(ConnectionManager.extractHost(gateway)))) {
                prefs.edit().remove(KEY_CUSTOM_URL).apply()
                return null
            }
            return saved
        }
        set(value) {
            val clean = value?.trim()?.trimEnd('/')
            val lan = prefs.getString(KEY_LAN_URL, null)?.trim()?.trimEnd('/')
            val cloud = prefs.getString(KEY_PRIMARY_CLOUD_URL, null)?.trim()?.trimEnd('/')
            if (clean.isNullOrBlank() || clean.equals(lan, ignoreCase = true) || clean.equals(cloud, ignoreCase = true)) {
                prefs.edit().remove(KEY_CUSTOM_URL).apply()
            } else {
                prefs.edit().putString(KEY_CUSTOM_URL, clean).apply()
            }
        }

    var cachedProjectsJson: String?
        get() = prefs.getString(KEY_CACHED_PROJECTS, null)
        set(value) = prefs.edit().putString(KEY_CACHED_PROJECTS, value).apply()

    var cachedConversationsJson: String?
        get() = prefs.getString(KEY_CACHED_CONVERSATIONS, null)
        set(value) = prefs.edit().putString(KEY_CACHED_CONVERSATIONS, value).apply()

    var cachedDraftSessionsJson: String?
        get() = prefs.getString(KEY_DRAFT_SESSIONS, null)
        set(value) = prefs.edit().putString(KEY_DRAFT_SESSIONS, value).apply()

    fun updateEndpoints(
        lan: String? = null,
        ipv6: String? = null,
        relay: String? = null,
        custom: String? = null,
        active: String? = null,
        primaryCloud: String? = null
    ) {
        val editor = prefs.edit()
        if (!lan.isNullOrBlank()) {
            editor.putString(KEY_LAN_URL, lan.trimEnd('/'))
        }
        if (!ipv6.isNullOrBlank()) {
            editor.putString(KEY_IPV6_URL, ipv6.trimEnd('/'))
        }
        if (!relay.isNullOrBlank()) {
            editor.putString(KEY_RELAY_URL, relay.trimEnd('/'))
            editor.putString(KEY_PRIMARY_CLOUD_URL, relay.trimEnd('/'))
        }
        if (!primaryCloud.isNullOrBlank()) {
            editor.putString(KEY_PRIMARY_CLOUD_URL, primaryCloud.trimEnd('/'))
        }
        if (!custom.isNullOrBlank()) {
            val cleanCustom = custom.trimEnd('/')
            val cleanLan = lan?.trimEnd('/') ?: prefs.getString(KEY_LAN_URL, null)?.trimEnd('/')
            val cleanCloud = primaryCloud?.trimEnd('/') ?: prefs.getString(KEY_PRIMARY_CLOUD_URL, null)?.trimEnd('/')
            if (!cleanCustom.equals(cleanLan, ignoreCase = true) && !cleanCustom.equals(cleanCloud, ignoreCase = true)) {
                editor.putString(KEY_CUSTOM_URL, cleanCustom)
            }
        }
        if (!active.isNullOrBlank()) {
            val clean = active.trimEnd('/')
            editor.putString(KEY_GATEWAY_URL, clean)
            if (!ConnectionManager.isLanHost(ConnectionManager.extractHost(clean))) {
                editor.putString(KEY_PRIMARY_CLOUD_URL, clean)
            }
        }
        editor.apply()
    }

    val candidateEndpoints: List<String>
        get() = listOfNotNull(
            lanServerUrl?.takeIf { it.isNotBlank() },
            customServerUrl?.takeIf { it.isNotBlank() },
            primaryCloudUrl?.takeIf { it.isNotBlank() },
            gatewayBaseUrl?.takeIf { it.isNotBlank() }
        ).distinct()

    fun getEffectiveGatewayUrl(isCellular: Boolean): String? {
        val active = gatewayBaseUrl?.trim()?.trimEnd('/')
        val lan = lanServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
        val custom = customServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
        val cloud = primaryCloudUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }

        if (isCellular) {
            // When on cellular, never use private LAN IP to avoid connection timeouts
            if (!active.isNullOrBlank() && !ConnectionManager.isLanHost(ConnectionManager.extractHost(active))) {
                return active
            }
            if (custom != null && !ConnectionManager.isLanHost(ConnectionManager.extractHost(custom))) {
                return custom
            }
            if (cloud != null) {
                return cloud
            }
            return active
        } else {
            if (!active.isNullOrBlank()) {
                return active
            }
            if (lan != null) {
                return lan
            }
            if (custom != null) {
                return custom
            }
            if (cloud != null) {
                return cloud
            }
        }
        return active ?: cloud
    }

    fun isPaired(): Boolean {
        return (!gatewayBaseUrl.isNullOrBlank() || !primaryCloudUrl.isNullOrBlank()) && !deviceToken.isNullOrBlank()
    }

    fun getLastViewTime(cascadeId: String): Long {
        return prefs.getLong(KEY_LAST_VIEW_PREFIX + cascadeId, 0L)
    }

    fun setLastViewTime(cascadeId: String, time: Long = System.currentTimeMillis()) {
        prefs.edit().putLong(KEY_LAST_VIEW_PREFIX + cascadeId, time).apply()
    }

    // MARK: - Draft Persistence
    fun getDraftText(cascadeId: String): String {
        return prefs.getString(KEY_DRAFT_TEXT_PREFIX + cascadeId, "") ?: ""
    }

    fun setDraftText(cascadeId: String, text: String) {
        if (text.isBlank()) {
            prefs.edit().remove(KEY_DRAFT_TEXT_PREFIX + cascadeId).apply()
        } else {
            prefs.edit().putString(KEY_DRAFT_TEXT_PREFIX + cascadeId, text).apply()
        }
        if (cascadeId.startsWith("local_draft_")) {
            val sessions = getLocalDraftSessions()
            val target = sessions.find { it.id == cascadeId }
            if (target != null) {
                target.draftText = text
                target.updatedAtEpochMs = System.currentTimeMillis()
                saveLocalDraftSession(target)
            }
        }
    }

    fun clearDraftText(cascadeId: String) {
        prefs.edit().remove(KEY_DRAFT_TEXT_PREFIX + cascadeId).apply()
    }

    fun hasDraft(cascadeId: String): Boolean {
        if (cascadeId.isBlank()) return false
        return getDraftText(cascadeId).isNotBlank() || hasDraftImages(cascadeId)
    }

    fun saveDraftImages(cascadeId: String, images: List<ByteArray>) {
        try {
            val safeKey = cascadeId.replace('/', '_').replace(':', '_')
            val dir = java.io.File(appContext.cacheDir, "draft_images/$safeKey").apply { mkdirs() }
            dir.listFiles()?.forEach { it.delete() }
            images.forEachIndexed { index, bytes ->
                val file = java.io.File(dir, "draft_${index}.png")
                file.writeBytes(bytes)
            }
        } catch (_: Exception) {}
    }

    fun loadDraftImages(cascadeId: String): List<ByteArray> {
        return try {
            val safeKey = cascadeId.replace('/', '_').replace(':', '_')
            val dir = java.io.File(appContext.cacheDir, "draft_images/$safeKey")
            if (!dir.exists()) return emptyList()
            val files = dir.listFiles()?.sortedBy { it.name } ?: return emptyList()
            files.mapNotNull {
                try { it.readBytes() } catch (_: Exception) { null }
            }
        } catch (_: Exception) {
            emptyList()
        }
    }

    fun clearDraftImages(cascadeId: String) {
        try {
            val safeKey = cascadeId.replace('/', '_').replace(':', '_')
            val dir = java.io.File(appContext.cacheDir, "draft_images/$safeKey")
            dir.deleteRecursively()
        } catch (_: Exception) {}
    }

    fun hasDraftImages(cascadeId: String): Boolean {
        return try {
            val safeKey = cascadeId.replace('/', '_').replace(':', '_')
            val dir = java.io.File(appContext.cacheDir, "draft_images/$safeKey")
            dir.exists() && (dir.listFiles()?.isNotEmpty() == true)
        } catch (_: Exception) {
            false
        }
    }

    private val draftJson = Json { ignoreUnknownKeys = true }

    fun getLocalDraftSessions(): List<LocalDraftSession> {
        val jsonStr = cachedDraftSessionsJson ?: return emptyList()
        return try {
            draftJson.decodeFromString<List<LocalDraftSession>>(jsonStr)
        } catch (e: Exception) {
            emptyList()
        }
    }

    fun saveLocalDraftSessions(sessions: List<LocalDraftSession>) {
        try {
            cachedDraftSessionsJson = draftJson.encodeToString(sessions)
        } catch (e: Exception) {
            Log.w("PreferencesManager", "Failed to save draft sessions: ${e.message}")
        }
    }

    fun createLocalDraftSession(project: ProjectItem): LocalDraftSession {
        val session = LocalDraftSession(
            id = "local_draft_${java.util.UUID.randomUUID()}",
            project = project
        )
        val current = getLocalDraftSessions().filter { it.id != session.id }
        saveLocalDraftSessions(listOf(session) + current)
        return session
    }

    fun getLocalDraftSession(id: String): LocalDraftSession? {
        val session = getLocalDraftSessions().find { it.id == id } ?: return null
        val text = getDraftText(id).ifBlank { session.draftText }
        return session.copy(draftText = text)
    }

    fun saveLocalDraftSession(session: LocalDraftSession) {
        val currentList = getLocalDraftSessions().filter { it.id != session.id }
        saveLocalDraftSessions(listOf(session) + currentList)
    }

    fun deleteLocalDraftSession(id: String) {
        val currentList = getLocalDraftSessions().filter { it.id != id }
        saveLocalDraftSessions(currentList)
        clearDraftText(id)
        clearDraftImages(id)
    }

    fun loadLocalDraftConversations(): List<ConversationItem> {
        val sessions = getLocalDraftSessions()
        return sessions.mapNotNull { session ->
            val text = getDraftText(session.id).ifBlank { session.draftText }
            val hasImages = hasDraftImages(session.id)
            if (text.isBlank() && !hasImages) return@mapNotNull null
            session.copy(draftText = text).toConversationItem(hasImages = hasImages)
        }.sortedByDescending { it.lastModifiedEpochMs }
    }

    // MARK: - Deletion Tombstone Persistence (TTL 10 min, aligned with iOS CacheManager)
    private val deletedTombstoneTTL = 600_000L

    fun recordDeletedConversation(cascadeId: String) {
        if (cascadeId.isBlank()) return
        val now = System.currentTimeMillis()
        val raw = prefs.getString(KEY_DELETED_CONVERSATIONS, null)
        val map = try {
            if (!raw.isNullOrBlank()) draftJson.decodeFromString<Map<String, Long>>(raw).toMutableMap() else mutableMapOf()
        } catch (_: Exception) {
            mutableMapOf()
        }
        map[cascadeId] = now
        val cleaned = map.filter { now - it.value < deletedTombstoneTTL }
        prefs.edit().putString(KEY_DELETED_CONVERSATIONS, draftJson.encodeToString(cleaned)).apply()
    }

    fun isDeletedConversation(cascadeId: String): Boolean {
        if (cascadeId.isBlank()) return false
        val now = System.currentTimeMillis()
        val raw = prefs.getString(KEY_DELETED_CONVERSATIONS, null) ?: return false
        return try {
            val map = draftJson.decodeFromString<Map<String, Long>>(raw)
            val ts = map[cascadeId] ?: return false
            now - ts < deletedTombstoneTTL
        } catch (_: Exception) {
            false
        }
    }

    fun purgeExpiredTombstones() {
        val now = System.currentTimeMillis()
        val raw = prefs.getString(KEY_DELETED_CONVERSATIONS, null) ?: return
        try {
            val map = draftJson.decodeFromString<Map<String, Long>>(raw)
            val cleaned = map.filter { now - it.value < deletedTombstoneTTL }
            if (cleaned.size != map.size) {
                prefs.edit().putString(KEY_DELETED_CONVERSATIONS, draftJson.encodeToString(cleaned)).apply()
            }
        } catch (_: Exception) {}
    }

    fun clear() {
        securePrefs?.edit()?.clear()?.apply()
        prefs.edit().clear().apply()
        _themeModeFlow.value = "system"
    }

    companion object {
        private const val KEY_GATEWAY_URL = "gateway_base_url"
        private const val KEY_PRIMARY_CLOUD_URL = "primary_cloud_url"
        private const val KEY_DEVICE_TOKEN = "device_token"
        private const val KEY_DEVICE_ID = "device_id"
        private const val KEY_THEME_MODE = "theme_mode"
        private const val KEY_AUTO_APPROVE = "auto_approve_permissions"
        private const val KEY_LIVE_NOTIFICATIONS = "live_notifications"
        private const val KEY_LAN_URL = "lan_server_url"
        private const val KEY_IPV6_URL = "ipv6_server_url"
        private const val KEY_RELAY_URL = "relay_server_url"
        private const val KEY_CUSTOM_URL = "custom_server_url"
        private const val KEY_LAST_VIEW_PREFIX = "ag_last_view_"
        private const val KEY_DRAFT_TEXT_PREFIX = "ag_draft_text_"
        private const val KEY_CACHED_PROJECTS = "cached_projects_json"
        private const val KEY_CACHED_CONVERSATIONS = "cached_conversations_json"
        private const val KEY_DRAFT_SESSIONS = "cached_draft_sessions_json"
        private const val KEY_DELETED_CONVERSATIONS = "ag_deleted_conversations"
    }
}
