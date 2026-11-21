package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.ModelDefaults
import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.LocalDraftSession
import com.antigravity.mobile.data.model.McpServerState
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.model.SkillInfo
import com.antigravity.mobile.data.model.QuotaBucketKind
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

    private val _themeModeFlow = MutableStateFlow(themeMode)
    val themeModeFlow: StateFlow<String> = _themeModeFlow.asStateFlow()

    private val _appLanguageFlow = MutableStateFlow(appLanguage)
    val appLanguageFlow: StateFlow<String> = _appLanguageFlow.asStateFlow()

    private val _gatewayPlatformFlow = MutableStateFlow(gatewayPlatform)
    val gatewayPlatformFlow: StateFlow<String?> = _gatewayPlatformFlow.asStateFlow()

    private val _enableVoiceModeFlow = MutableStateFlow(prefs.getBoolean(KEY_ENABLE_VOICE_MODE, false))
    val enableVoiceModeFlow: StateFlow<Boolean> = _enableVoiceModeFlow.asStateFlow()

    var weeklyPaceCursorEnabled: Boolean
        get() = prefs.getBoolean(KEY_WEEKLY_PACE_CURSOR, true)
        set(value) {
            prefs.edit().putBoolean(KEY_WEEKLY_PACE_CURSOR, value).apply()
            _weeklyPaceCursorFlow.value = value
        }

    private val _weeklyPaceCursorFlow = MutableStateFlow(weeklyPaceCursorEnabled)
    val weeklyPaceCursorFlow: StateFlow<Boolean> = _weeklyPaceCursorFlow.asStateFlow()

    var selectedQuotaBucket: QuotaBucketKind
        get() = QuotaBucketKind.fromKey(prefs.getString(KEY_SELECTED_QUOTA_BUCKET, QuotaBucketKind.GEMINI_5H.key))
        set(value) {
            prefs.edit().putString(KEY_SELECTED_QUOTA_BUCKET, value.key).apply()
            _selectedQuotaBucketFlow.value = value
        }

    private val _selectedQuotaBucketFlow = MutableStateFlow(selectedQuotaBucket)
    val selectedQuotaBucketFlow: StateFlow<QuotaBucketKind> = _selectedQuotaBucketFlow.asStateFlow()

    var cockpitEmailMasked: Boolean
        get() = prefs.getBoolean(KEY_COCKPIT_EMAIL_MASKED, false)
        set(value) {
            prefs.edit().putBoolean(KEY_COCKPIT_EMAIL_MASKED, value).apply()
            _cockpitEmailMaskedFlow.value = value
        }

    private val _cockpitEmailMaskedFlow = MutableStateFlow(cockpitEmailMasked)
    val cockpitEmailMaskedFlow: StateFlow<Boolean> = _cockpitEmailMaskedFlow.asStateFlow()

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
            if (legacyCustom.equals(lan, ignoreCase = true) ||
                (!cloud.isNullOrBlank() && legacyCustom.equals(cloud, ignoreCase = true)) ||
                legacyCustom.endsWith(".mgy.jiuge.space", ignoreCase = true)) {
                prefs.edit().remove(KEY_CUSTOM_URL).apply()
            }
        }
        _gatewayPlatformFlow.value = gatewayPlatform
        // Language always follows system locale; clear any legacy manual override.
        if (prefs.contains(KEY_APP_LANGUAGE)) {
            prefs.edit().remove(KEY_APP_LANGUAGE).apply()
        }
        _appLanguageFlow.value = "system"
        com.antigravity.mobile.ui.util.L10n.setLanguagePreference("system")
    }

    var gatewayPlatform: String?
        get() {
            val saved = prefs.getString(KEY_GATEWAY_PLATFORM, null)
            if (!saved.isNullOrBlank()) return saved
            // Heuristic fallback: check cached projects or conversations for Windows/Mac path clues
            val projects = cachedProjectsJson
            if (!projects.isNullOrBlank()) {
                if (projects.contains(":\\\\") || projects.contains(":/") || projects.contains(":\\")) {
                    return "windows"
                } else if (projects.contains("/Users/")) {
                    return "darwin"
                } else if (projects.contains("/home/")) {
                    return "linux"
                }
            }
            return null
        }
        set(value) {
            val clean = value?.trim()?.lowercase()
            if (clean.isNullOrBlank()) {
                prefs.edit().remove(KEY_GATEWAY_PLATFORM).apply()
            } else {
                val normalized = when (clean) {
                    "darwin", "macos", "mac" -> "darwin"
                    "windows", "win" -> "windows"
                    "linux" -> "linux"
                    else -> clean
                }
                prefs.edit().putString(KEY_GATEWAY_PLATFORM, normalized).apply()
            }
            _gatewayPlatformFlow.value = gatewayPlatform
        }

    val gatewayPlatformDisplayName: String
        get() = when (gatewayPlatform?.lowercase()) {
            "windows", "win" -> "Windows 网关"
            "darwin", "macos", "mac" -> "Mac 网关"
            "linux" -> "Linux 网关"
            else -> "电脑网关"
        }

    var gatewayBaseUrl: String?
        get() = prefs.getString(KEY_GATEWAY_URL, null)
        set(value) = prefs.edit().putString(KEY_GATEWAY_URL, value?.trimEnd('/')).apply()

    var deviceToken: String?
        get() {
            if (memDeviceTokenLoaded) {
                return memDeviceToken
            }
            val loaded = securePrefs?.getString(KEY_DEVICE_TOKEN, null)
            memDeviceToken = loaded
            memDeviceTokenLoaded = true
            return loaded
        }
        set(value) {
            val sp = securePrefs
            if (sp != null) {
                if (value != null) {
                    sp.edit().putString(KEY_DEVICE_TOKEN, value).apply()
                } else {
                    sp.edit().remove(KEY_DEVICE_TOKEN).apply()
                }
                memDeviceToken = value
                memDeviceTokenLoaded = true
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

    var appLanguage: String
        get() = "system"
        set(_) {
            prefs.edit().remove(KEY_APP_LANGUAGE).apply()
            _appLanguageFlow.value = "system"
            com.antigravity.mobile.ui.util.L10n.setLanguagePreference("system")
        }

    var themeMode: String
        get() {
            val raw = prefs.getString(KEY_THEME_MODE, "system") ?: "system"
            return if (raw == "nord") "dark" else raw
        }
        set(value) {
            val normalized = if (value == "nord") "dark" else value
            prefs.edit().putString(KEY_THEME_MODE, normalized).apply()
            _themeModeFlow.value = normalized
        }

    var shareCardSkinId: String
        get() = prefs.getString(KEY_SHARE_CARD_SKIN_ID, "auto") ?: "auto"
        set(value) {
            prefs.edit().putString(KEY_SHARE_CARD_SKIN_ID, value).apply()
        }

    fun resolvedShareCardSkin(isSystemDark: Boolean): com.antigravity.mobile.ui.theme.ThemePreset {
        val saved = shareCardSkinId
        if (saved != "auto") {
            com.antigravity.mobile.ui.theme.ThemePresetCatalog.curatedSkins
                .firstOrNull { it.id == saved }
                ?.let { return it }
        }
        val followDark = when (themeMode.lowercase()) {
            "dark" -> true
            "light" -> false
            else -> isSystemDark
        }
        return if (followDark) {
            com.antigravity.mobile.ui.theme.ThemePresetCatalog.defaultDark
        } else {
            com.antigravity.mobile.ui.theme.ThemePresetCatalog.defaultLight
        }
    }

    var fontSizeScale: String
        get() = prefs.getString(KEY_FONT_SIZE_SCALE, "standard") ?: "standard"
        set(value) {
            prefs.edit().putString(KEY_FONT_SIZE_SCALE, value).apply()
            _fontSizeScaleFlow.value = value
        }

    private val _fontSizeScaleFlow = MutableStateFlow(fontSizeScale)
    val fontSizeScaleFlow: StateFlow<String> = _fontSizeScaleFlow.asStateFlow()

    var isKnowledgeModeEnabled: Boolean
        get() = prefs.getBoolean(KEY_KNOWLEDGE_MODE, false)
        set(value) {
            prefs.edit().putBoolean(KEY_KNOWLEDGE_MODE, value).apply()
            _knowledgeModeFlow.value = value
        }

    private val _knowledgeModeFlow = MutableStateFlow(isKnowledgeModeEnabled)
    val knowledgeModeFlow: StateFlow<Boolean> = _knowledgeModeFlow.asStateFlow()

    var compactCollapseTools: Boolean
        get() = prefs.getBoolean(KEY_COMPACT_COLLAPSE_TOOLS, true)
        set(value) {
            prefs.edit().putBoolean(KEY_COMPACT_COLLAPSE_TOOLS, value).apply()
            _compactCollapseToolsFlow.value = value
        }

    private val _compactCollapseToolsFlow = MutableStateFlow(compactCollapseTools)
    val compactCollapseToolsFlow: StateFlow<Boolean> = _compactCollapseToolsFlow.asStateFlow()

    var compactCollapseTasks: Boolean
        get() = prefs.getBoolean(KEY_COMPACT_COLLAPSE_TASKS, true)
        set(value) {
            prefs.edit().putBoolean(KEY_COMPACT_COLLAPSE_TASKS, value).apply()
            _compactCollapseTasksFlow.value = value
        }

    private val _compactCollapseTasksFlow = MutableStateFlow(compactCollapseTasks)
    val compactCollapseTasksFlow: StateFlow<Boolean> = _compactCollapseTasksFlow.asStateFlow()

    var compactHideHealthyContext: Boolean
        get() = prefs.getBoolean(KEY_COMPACT_HIDE_HEALTHY_CONTEXT, false)
        set(value) {
            prefs.edit().putBoolean(KEY_COMPACT_HIDE_HEALTHY_CONTEXT, value).apply()
            _compactHideHealthyContextFlow.value = value
        }

    private val _compactHideHealthyContextFlow = MutableStateFlow(compactHideHealthyContext)
    val compactHideHealthyContextFlow: StateFlow<Boolean> = _compactHideHealthyContextFlow.asStateFlow()

    private fun legacyFavorite1Seed(): String {
        val g = prefs.getString(KEY_DEFAULT_GEMINI_MODEL, null)?.takeIf { it.isNotBlank() } ?: ModelDefaults.FALLBACK_GEMINI
        val c = prefs.getString(KEY_DEFAULT_CLAUDE_MODEL, null)?.takeIf { it.isNotBlank() } ?: ModelDefaults.FALLBACK_CLAUDE
        val o = prefs.getString(KEY_DEFAULT_ORCHESTRATOR_MODEL, null)?.takeIf { it.isNotBlank() } ?: ModelDefaults.FALLBACK_ORCHESTRATOR
        return when (prefs.getString(KEY_DEFAULT_NEW_SESSION_PROVIDER, "gemini")) {
            "claude" -> c
            "orchestrator" -> o
            else -> g
        }
    }

    private fun legacyFavorite2Seed(): String {
        val g = prefs.getString(KEY_DEFAULT_GEMINI_MODEL, null)?.takeIf { it.isNotBlank() } ?: ModelDefaults.FALLBACK_GEMINI
        val c = prefs.getString(KEY_DEFAULT_CLAUDE_MODEL, null)?.takeIf { it.isNotBlank() } ?: ModelDefaults.FALLBACK_CLAUDE
        return if (legacyFavorite1Seed().equals(c, ignoreCase = true)) g else c
    }

    /** 「常用模型 1」（全模型全思考强度可选；进入新会话首选模型）。 */
    var favoriteModel1: String
        get() = prefs.getString(KEY_FAVORITE_MODEL_1, null)?.takeIf { it.isNotBlank() } ?: legacyFavorite1Seed()
        set(value) {
            val trimmed = value.trim()
            prefs.edit().putString(KEY_FAVORITE_MODEL_1, trimmed).apply()
            _favoriteModel1Flow.value = trimmed.ifBlank { ModelDefaults.FALLBACK_GEMINI }
        }

    private val _favoriteModel1Flow = MutableStateFlow(favoriteModel1)
    val favoriteModel1Flow: StateFlow<String> = _favoriteModel1Flow.asStateFlow()

    /** 「常用模型 2」（全模型全思考强度可选；单击胶囊与常用模型 1 快速切换）。 */
    var favoriteModel2: String
        get() = prefs.getString(KEY_FAVORITE_MODEL_2, null)?.takeIf { it.isNotBlank() } ?: legacyFavorite2Seed()
        set(value) {
            val trimmed = value.trim()
            prefs.edit().putString(KEY_FAVORITE_MODEL_2, trimmed).apply()
            _favoriteModel2Flow.value = trimmed.ifBlank { ModelDefaults.FALLBACK_CLAUDE }
        }

    private val _favoriteModel2Flow = MutableStateFlow(favoriteModel2)
    val favoriteModel2Flow: StateFlow<String> = _favoriteModel2Flow.asStateFlow()

    val defaultNewSessionModel: String
        get() = favoriteModel1

    val cachedModelCatalog: ModelsResponse?
        get() {
            val raw = prefs.getString(KEY_CACHED_MODEL_CATALOG, null) ?: return null
            return runCatching { draftJson.decodeFromString<ModelsResponse>(raw) }.getOrNull()
        }

    private val _modelCatalogFlow = MutableStateFlow(cachedModelCatalog)
    val modelCatalogFlow: StateFlow<ModelsResponse?> = _modelCatalogFlow.asStateFlow()

    fun updateModelCatalog(catalog: ModelsResponse) {
        _modelCatalogFlow.value = catalog
        runCatching { draftJson.encodeToString(catalog) }.getOrNull()?.let { encoded ->
            prefs.edit().putString(KEY_CACHED_MODEL_CATALOG, encoded).apply()
        }
        val fav1 = ModelDefaults.validatedFavorite(
            saved = favoriteModel1,
            fallback = ModelDefaults.FALLBACK_GEMINI,
            avoid = null,
            catalog = catalog
        )
        if (fav1 != favoriteModel1) favoriteModel1 = fav1
        val fav2 = ModelDefaults.validatedFavorite(
            saved = favoriteModel2,
            fallback = ModelDefaults.FALLBACK_CLAUDE,
            avoid = fav1,
            catalog = catalog
        )
        if (fav2 != favoriteModel2) favoriteModel2 = fav2
    }

    fun updateDesktopActiveModel(model: String?, name: String? = null) {
        val cleanModel = model?.takeIf { it.isNotBlank() } ?: return
        val cleanName = name?.takeIf { it.isNotBlank() }
        val current = _modelCatalogFlow.value ?: cachedModelCatalog
        if (current != null &&
            current.desktopActiveModel == cleanModel &&
            (cleanName == null || current.desktopActiveModelName == cleanName)
        ) {
            return
        }
        val updated = (current ?: ModelsResponse()).copy(
            desktopActiveModel = cleanModel,
            desktopActiveModelName = cleanName ?: current?.desktopActiveModelName
        )
        _modelCatalogFlow.value = updated
        runCatching { draftJson.encodeToString(updated) }.getOrNull()?.let { encoded ->
            prefs.edit().putString(KEY_CACHED_MODEL_CATALOG, encoded).apply()
        }
    }

    var enableLiveNotifications: Boolean
        get() = prefs.getBoolean(KEY_LIVE_NOTIFICATIONS, true)
        set(value) = prefs.edit().putBoolean(KEY_LIVE_NOTIFICATIONS, value).apply()

    var enableVoiceMode: Boolean
        get() = prefs.getBoolean(KEY_ENABLE_VOICE_MODE, false)
        set(value) {
            prefs.edit().putBoolean(KEY_ENABLE_VOICE_MODE, value).apply()
            _enableVoiceModeFlow.value = value
        }

    var fcmPushToken: String?
        get() = prefs.getString(KEY_FCM_PUSH_TOKEN, null)
        set(value) = prefs.edit().putString(KEY_FCM_PUSH_TOKEN, value?.trim()).apply()

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
        set(value) = prefs.edit().putString(KEY_LAN_URL, normalizeUrl(value)).apply()

    var relayServerUrl: String?
        get() = prefs.getString(KEY_RELAY_URL, null)
        set(value) = prefs.edit().putString(KEY_RELAY_URL, value?.trimEnd('/')).apply()

    var customServerUrl: String?
        get() {
            val saved = prefs.getString(KEY_CUSTOM_URL, null)?.trim()?.trimEnd('/')
            if (saved.isNullOrBlank()) return null
            val lan = prefs.getString(KEY_LAN_URL, null)?.trim()?.trimEnd('/')
            val cloud = prefs.getString(KEY_PRIMARY_CLOUD_URL, null)?.trim()?.trimEnd('/')
            if (saved.equals(lan, ignoreCase = true) ||
                (!cloud.isNullOrBlank() && saved.equals(cloud, ignoreCase = true)) ||
                saved.endsWith(".mgy.jiuge.space", ignoreCase = true)) {
                prefs.edit().remove(KEY_CUSTOM_URL).apply()
                return null
            }
            return saved
        }
        set(value) {
            val clean = normalizeUrl(value)
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

    var cachedMcpServersJson: String?
        get() = prefs.getString(KEY_CACHED_MCP_SERVERS, null)
        set(value) = prefs.edit().putString(KEY_CACHED_MCP_SERVERS, value).apply()

    var cachedSkillsJson: String?
        get() = prefs.getString(KEY_CACHED_SKILLS, null)
        set(value) = prefs.edit().putString(KEY_CACHED_SKILLS, value).apply()

    fun loadCachedMcpServers(): List<McpServerState> {
        memMcpServers?.let { if (it.isNotEmpty()) return it }
        val raw = cachedMcpServersJson
        if (!raw.isNullOrBlank()) {
            try {
                val list = draftJson.decodeFromString<List<McpServerState>>(raw)
                if (list.isNotEmpty()) {
                    memMcpServers = list
                }
                return list
            } catch (e: Exception) {
                Log.w("PreferencesManager", "Failed to decode cached MCP servers: ${e.message}")
            }
        }
        return emptyList()
    }

    fun saveCachedMcpServers(list: List<McpServerState>) {
        memMcpServers = list
        try {
            cachedMcpServersJson = draftJson.encodeToString(list)
        } catch (e: Exception) {
            Log.w("PreferencesManager", "Failed to encode cached MCP servers: ${e.message}")
        }
    }

    fun loadCachedSkills(): List<SkillInfo> {
        memSkills?.let { if (it.isNotEmpty()) return it }
        val raw = cachedSkillsJson
        if (!raw.isNullOrBlank()) {
            try {
                val list = draftJson.decodeFromString<List<SkillInfo>>(raw)
                if (list.isNotEmpty()) {
                    memSkills = list
                }
                return list
            } catch (e: Exception) {
                Log.w("PreferencesManager", "Failed to decode cached skills: ${e.message}")
            }
        }
        return emptyList()
    }

    fun saveCachedSkills(list: List<SkillInfo>) {
        memSkills = list
        try {
            cachedSkillsJson = draftJson.encodeToString(list)
        } catch (e: Exception) {
            Log.w("PreferencesManager", "Failed to encode cached skills: ${e.message}")
        }
    }

    fun updateEndpoints(
        lan: String? = null,
        relay: String? = null,
        custom: String? = null,
        active: String? = null,
        primaryCloud: String? = null
    ) {
        val editor = prefs.edit()
        if (!lan.isNullOrBlank()) {
            editor.putString(KEY_LAN_URL, lan.trimEnd('/'))
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

    fun getEffectiveGatewayUrl(isCellular: Boolean, connectionManager: ConnectionManager? = null): String? {
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
            return if (!active.isNullOrBlank() && !ConnectionManager.isLanHost(ConnectionManager.extractHost(active))) active else cloud
        } else {
            // When on Wi-Fi:
            // Priority 1: Dynamic active/elected gateway (from ConnectionManager)
            if (!active.isNullOrBlank()) {
                val isLan = ConnectionManager.isLanHost(ConnectionManager.extractHost(active))
                val isKnownUnhealthy = connectionManager?.isEndpointHealthy(active) == false
                if (!isLan || !isKnownUnhealthy) {
                    return active
                }
            }
            // Priority 2: LAN (local direct connection for lowest latency, if not known unreachable)
            if (lan != null && connectionManager?.isEndpointHealthy(lan) != false) {
                return lan
            }
            // Priority 3: Custom
            if (custom != null && connectionManager?.isEndpointHealthy(custom) != false) {
                return custom
            }
            // Priority 4: Primary Cloud domain (Cloudflare HTTPS - ultimate fallback for external Wi-Fi)
            if (cloud != null) {
                return cloud
            }
            // Priority 5: Fallback to custom/lan/active
            if (custom != null) {
                return custom
            }
            if (lan != null) {
                return lan
            }
        }
        return active ?: cloud ?: lan
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
        return getDraftText(cascadeId).isNotBlank() || hasDraftImages(cascadeId) || hasDraftFiles(cascadeId)
    }

    // MARK: - Draft file attachments (documents / archives / source files)
    private fun draftFilesKey(cascadeId: String) = "draft_files_" + cascadeId.replace('/', '_').replace(':', '_')

    /** App-private directory holding local copies of files attached to a draft. */
    fun draftFilesDir(cascadeId: String): java.io.File {
        val safeKey = cascadeId.replace('/', '_').replace(':', '_')
        return java.io.File(appContext.filesDir, "draft_files/$safeKey").apply { mkdirs() }
    }

    fun saveDraftFiles(cascadeId: String, files: List<com.antigravity.mobile.data.model.AttachmentFile>) {
        if (cascadeId.isBlank()) return
        try {
            if (files.isEmpty()) {
                prefs.edit().remove(draftFilesKey(cascadeId)).apply()
            } else {
                prefs.edit().putString(
                    draftFilesKey(cascadeId),
                    JsonConfig.instance.encodeToString(files)
                ).apply()
            }
        } catch (_: Exception) {}
    }

    fun loadDraftFiles(cascadeId: String): List<com.antigravity.mobile.data.model.AttachmentFile> {
        if (cascadeId.isBlank()) return emptyList()
        return try {
            val raw = prefs.getString(draftFilesKey(cascadeId), null) ?: return emptyList()
            JsonConfig.instance.decodeFromString<List<com.antigravity.mobile.data.model.AttachmentFile>>(raw)
                .filter { java.io.File(it.localPath).exists() }
        } catch (_: Exception) {
            emptyList()
        }
    }

    fun hasDraftFiles(cascadeId: String): Boolean = loadDraftFiles(cascadeId).isNotEmpty()

    /** Removes the persisted metadata and the local copies of a draft's files. */
    fun clearDraftFiles(cascadeId: String) {
        if (cascadeId.isBlank()) return
        try {
            prefs.edit().remove(draftFilesKey(cascadeId)).apply()
            val safeKey = cascadeId.replace('/', '_').replace(':', '_')
            java.io.File(appContext.filesDir, "draft_files/$safeKey").deleteRecursively()
        } catch (_: Exception) {}
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

    private val draftJson = JsonConfig.instance

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
            session.copy(draftText = text).toConversationItem(hasImages = hasImages, isPinned = isPinned(session.id))
        }.sortedByDescending { it.lastModifiedEpochMs }
    }

    // MARK: - Deletion Tombstone Persistence (TTL 10 min, aligned with iOS CacheManager)
    private val deletedTombstoneTTL = 600_000L

    private fun loadDeletedConversationsMap(): Map<String, Long> {
        memDeletedConversations?.let { return it }
        val raw = prefs.getString(KEY_DELETED_CONVERSATIONS, null)
        val parsed: Map<String, Long> = if (raw.isNullOrBlank()) {
            emptyMap()
        } else {
            try {
                draftJson.decodeFromString<Map<String, Long>>(raw)
            } catch (_: Exception) {
                emptyMap()
            }
        }
        memDeletedConversations = parsed
        return parsed
    }

    fun recordDeletedConversation(cascadeId: String) {
        if (cascadeId.isBlank()) return
        val now = System.currentTimeMillis()
        val map = loadDeletedConversationsMap().toMutableMap()
        map[cascadeId] = now
        val cleaned = map.filter { now - it.value < deletedTombstoneTTL }
        memDeletedConversations = cleaned
        prefs.edit().putString(KEY_DELETED_CONVERSATIONS, draftJson.encodeToString(cleaned)).apply()

        val pinned = getPinnedConversationIds()
        if (pinned.contains(cascadeId)) {
            val updated = pinned.toMutableSet()
            updated.remove(cascadeId)
            prefs.edit().putStringSet(KEY_PINNED_CONVERSATIONS, updated).apply()
        }
    }

    fun getWorkspaceNote(key: String): String? {
        if (key.isBlank()) return null
        return prefs.getString("$KEY_WS_NOTE_PREFIX$key", null)?.takeIf { it.isNotBlank() }
    }

    fun setWorkspaceNote(key: String, note: String?) {
        if (key.isBlank()) return
        val trimmed = note?.trim().orEmpty()
        if (trimmed.isEmpty()) prefs.edit().remove("$KEY_WS_NOTE_PREFIX$key").apply()
        else prefs.edit().putString("$KEY_WS_NOTE_PREFIX$key", trimmed).apply()
    }

    fun getPinnedConversationIds(): Set<String> {
        return prefs.getStringSet(KEY_PINNED_CONVERSATIONS, emptySet()) ?: emptySet()
    }

    fun isPinned(cascadeId: String): Boolean {
        if (cascadeId.isBlank()) return false
        return getPinnedConversationIds().contains(cascadeId)
    }

    fun setPinned(cascadeId: String, pinned: Boolean) {
        if (cascadeId.isBlank()) return
        val current = getPinnedConversationIds().toMutableSet()
        val changed = if (pinned) current.add(cascadeId) else current.remove(cascadeId)
        if (changed) {
            prefs.edit().putStringSet(KEY_PINNED_CONVERSATIONS, current).apply()
        }
    }

    fun togglePin(cascadeId: String): Boolean {
        if (cascadeId.isBlank()) return false
        val current = getPinnedConversationIds().toMutableSet()
        val willPin = !current.contains(cascadeId)
        if (willPin) {
            current.add(cascadeId)
        } else {
            current.remove(cascadeId)
        }
        prefs.edit().putStringSet(KEY_PINNED_CONVERSATIONS, current).apply()
        return willPin
    }

    fun isDeletedConversation(cascadeId: String): Boolean {
        if (cascadeId.isBlank()) return false
        val now = System.currentTimeMillis()
        val map = loadDeletedConversationsMap()
        val ts = map[cascadeId] ?: return false
        return now - ts < deletedTombstoneTTL
    }

    fun purgeExpiredTombstones() {
        val now = System.currentTimeMillis()
        val map = loadDeletedConversationsMap()
        if (map.isEmpty()) return
        val cleaned = map.filter { now - it.value < deletedTombstoneTTL }
        if (cleaned.size != map.size) {
            memDeletedConversations = cleaned
            prefs.edit().putString(KEY_DELETED_CONVERSATIONS, draftJson.encodeToString(cleaned)).apply()
        }
    }

    fun clear() {
        securePrefs?.edit()?.clear()?.apply()
        prefs.edit().clear().apply()
        memDeviceToken = null
        memDeviceTokenLoaded = false
        memDeletedConversations = null
        memMcpServers = null
        memSkills = null
        _themeModeFlow.value = "system"
        _gatewayPlatformFlow.value = null
        _modelCatalogFlow.value = null
        _favoriteModel1Flow.value = ModelDefaults.FALLBACK_GEMINI
        _favoriteModel2Flow.value = ModelDefaults.FALLBACK_CLAUDE
    }

    companion object {
        fun normalizeUrl(raw: String?): String? {
            if (raw.isNullOrBlank()) return null
            var clean = raw.trim().trimEnd('/')
            if (clean.isBlank()) return null
            if (!clean.startsWith("http://", ignoreCase = true) && !clean.startsWith("https://", ignoreCase = true)) {
                clean = "http://$clean"
            }
            return clean
        }

        fun bodyScaleFactor(scale: String): Float = when (scale) {
            "compact" -> 0.88f
            "comfortable" -> 1.15f
            else -> 1.0f
        }

        fun codeScaleFactor(scale: String): Float = when (scale) {
            "compact" -> 0.90f
            "comfortable" -> 1.12f
            else -> 1.0f
        }

        @Volatile
        private var memDeviceTokenLoaded: Boolean = false
        @Volatile
        private var memDeviceToken: String? = null
        @Volatile
        private var memDeletedConversations: Map<String, Long>? = null
        @Volatile
        private var memMcpServers: List<McpServerState>? = null
        @Volatile
        private var memSkills: List<SkillInfo>? = null

        private const val KEY_GATEWAY_PLATFORM = "gateway_platform"
        private const val KEY_GATEWAY_URL = "gateway_base_url"
        private const val KEY_PRIMARY_CLOUD_URL = "primary_cloud_url"
        private const val KEY_DEVICE_TOKEN = "device_token"
        private const val KEY_DEVICE_ID = "device_id"
        private const val KEY_THEME_MODE = "theme_mode"
        private const val KEY_APP_LANGUAGE = "app_language"
        private const val KEY_SHARE_CARD_SKIN_ID = "share_card_skin_id"
        private const val KEY_FONT_SIZE_SCALE = "font_size_scale"
        private const val KEY_KNOWLEDGE_MODE = "knowledge_mode_enabled"
        private const val KEY_COMPACT_COLLAPSE_TOOLS = "compact_collapse_tools"
        private const val KEY_COMPACT_COLLAPSE_TASKS = "compact_collapse_tasks"
        private const val KEY_COMPACT_HIDE_HEALTHY_CONTEXT = "compact_hide_healthy_context"
        private const val KEY_FAVORITE_MODEL_1 = "favorite_model_1"
        private const val KEY_FAVORITE_MODEL_2 = "favorite_model_2"
        private const val KEY_DEFAULT_GEMINI_MODEL = "default_gemini_model"
        private const val KEY_DEFAULT_CLAUDE_MODEL = "default_claude_model"
        private const val KEY_DEFAULT_ORCHESTRATOR_MODEL = "default_orchestrator_model"
        private const val KEY_DEFAULT_NEW_SESSION_PROVIDER = "default_new_session_provider"
        private const val KEY_LIVE_NOTIFICATIONS = "live_notifications"
        private const val KEY_ENABLE_VOICE_MODE = "enable_voice_mode"
        private const val KEY_WEEKLY_PACE_CURSOR = "weekly_pace_cursor"
        private const val KEY_SELECTED_QUOTA_BUCKET = "selected_quota_bucket"
        private const val KEY_COCKPIT_EMAIL_MASKED = "cockpit_email_masked"
        private const val KEY_FCM_PUSH_TOKEN = "fcm_push_token"
        private const val KEY_LAN_URL = "lan_server_url"
        private const val KEY_RELAY_URL = "relay_server_url"
        private const val KEY_CUSTOM_URL = "custom_server_url"
        private const val KEY_LAST_VIEW_PREFIX = "ag_last_view_"
        private const val KEY_DRAFT_TEXT_PREFIX = "ag_draft_text_"
        private const val KEY_CACHED_PROJECTS = "cached_projects_json"
        private const val KEY_CACHED_CONVERSATIONS = "cached_conversations_json"
        private const val KEY_DRAFT_SESSIONS = "cached_draft_sessions_json"
        private const val KEY_CACHED_MCP_SERVERS = "cached_mcp_servers_json"
        private const val KEY_CACHED_SKILLS = "cached_skills_json"
        private const val KEY_CACHED_MODEL_CATALOG = "cached_model_catalog_json"
        private const val KEY_DELETED_CONVERSATIONS = "ag_deleted_conversations"
        private const val KEY_PINNED_CONVERSATIONS = "ag_pinned_conversations"
        private const val KEY_WS_NOTE_PREFIX = "p3_ws_note_"
    }
}
