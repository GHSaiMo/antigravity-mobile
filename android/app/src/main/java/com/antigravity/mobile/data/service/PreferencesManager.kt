package com.antigravity.mobile.data.service

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class PreferencesManager(context: Context) {
    private val appContext = context.applicationContext
    private val prefs: SharedPreferences = try {
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
        Log.w("PreferencesManager", "Failed to init EncryptedSharedPreferences, fallback to standard: ${e.message}")
        context.getSharedPreferences("agy_standard_prefs", Context.MODE_PRIVATE)
    }

    private val _themeModeFlow = MutableStateFlow(themeMode)
    val themeModeFlow: StateFlow<String> = _themeModeFlow.asStateFlow()

    var gatewayBaseUrl: String?
        get() = prefs.getString(KEY_GATEWAY_URL, null)
        set(value) = prefs.edit().putString(KEY_GATEWAY_URL, value?.trimEnd('/')).apply()

    var deviceToken: String?
        get() = prefs.getString(KEY_DEVICE_TOKEN, null)
        set(value) = prefs.edit().putString(KEY_DEVICE_TOKEN, value).apply()

    var deviceId: String?
        get() = prefs.getString(KEY_DEVICE_ID, null)
        set(value) = prefs.edit().putString(KEY_DEVICE_ID, value).apply()

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
        get() = prefs.getString(KEY_CUSTOM_URL, null)
        set(value) = prefs.edit().putString(KEY_CUSTOM_URL, value?.trimEnd('/')).apply()

    var cachedProjectsJson: String?
        get() = prefs.getString(KEY_CACHED_PROJECTS, null)
        set(value) = prefs.edit().putString(KEY_CACHED_PROJECTS, value).apply()

    fun isPaired(): Boolean {
        return !gatewayBaseUrl.isNullOrBlank() && !deviceToken.isNullOrBlank()
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
    }

    fun clearDraftText(cascadeId: String) {
        prefs.edit().remove(KEY_DRAFT_TEXT_PREFIX + cascadeId).apply()
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

    fun clear() {
        prefs.edit().clear().apply()
        _themeModeFlow.value = "system"
    }

    companion object {
        private const val KEY_GATEWAY_URL = "gateway_base_url"
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
    }
}
