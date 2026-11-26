package com.antigravity.mobile.data.service

import android.content.Context
import android.util.Log
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.data.model.GatewayProfile
import com.antigravity.mobile.data.model.GatewayRequestContext
import com.antigravity.mobile.data.model.GatewayRuntimeState
import com.antigravity.mobile.data.model.GatewaySummarySnapshot
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.encodeToString
import java.util.UUID

/**
 * Manages decoupled multi-gateway profiles, zero-copy legacy upgrade, per-profile credentials
 * & isolated cache namespaces, and concurrent inactive-host status peeks.
 */
class GatewayProfileStore private constructor(
    private val appContext: Context,
    private val prefs: PreferencesManager
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val json = JsonConfig.instance
    private val lock = Any()

    @Volatile private var cacheManagerRef: CacheManager? = null
    @Volatile private var documentCacheManagerRef: DocumentCacheManager? = null
    @Volatile private var connectionManagerRef: ConnectionManager? = null
    @Volatile private var apiClientRef: ApiClient? = null
    @Volatile private var wsClientRef: StreamWebSocketClient? = null

    private val _profilesFlow = MutableStateFlow<List<GatewayProfile>>(emptyList())
    val profilesFlow: StateFlow<List<GatewayProfile>> = _profilesFlow.asStateFlow()

    private val _activeProfileIdFlow = MutableStateFlow<String?>(null)
    val activeProfileIdFlow: StateFlow<String?> = _activeProfileIdFlow.asStateFlow()

    private val _summariesFlow = MutableStateFlow<Map<String, GatewaySummarySnapshot>>(emptyMap())
    val summariesFlow: StateFlow<Map<String, GatewaySummarySnapshot>> = _summariesFlow.asStateFlow()

    @Volatile private var lastInactiveRefreshMs: Long = 0L
    @Volatile private var inFlightSummaryJob: Job? = null

    val profiles: List<GatewayProfile>
        get() = _profilesFlow.value

    val activeProfileId: String?
        get() = _activeProfileIdFlow.value

    val activeProfile: GatewayProfile?
        get() {
            val aid = _activeProfileIdFlow.value
            val list = _profilesFlow.value
            return if (aid != null) list.firstOrNull { it.id == aid } ?: list.firstOrNull() else list.firstOrNull()
        }

    val hasMultipleGateways: Boolean
        get() = _profilesFlow.value.size > 1

    /** True when at least one non-active paired gateway currently reports pending approvals. */
    val hasInactivePendingAction: Boolean
        get() {
            val list = _profilesFlow.value
            if (list.size <= 1) return false
            val currentActiveId = activeProfile?.id
            return _summariesFlow.value.any { (profileId, snap) ->
                profileId != currentActiveId && snap.actionCount > 0
            }
        }

    /** 1-based `(index, total)` position of the active gateway in stable order. */
    val activeProfileDisplayPosition: Pair<Int, Int>?
        get() {
            val list = _profilesFlow.value
            if (list.isEmpty()) return null
            val aid = activeProfile?.id
            val idx = list.indexOfFirst { it.id == aid }.takeIf { it >= 0 } ?: 0
            return (idx + 1) to list.size
        }

    /** Returns the next (`step > 0`) or previous (`step < 0`) gateway in stable cyclic order. */
    fun adjacentProfile(step: Int): GatewayProfile? {
        val list = _profilesFlow.value
        val count = list.size
        if (count <= 1 || step == 0) return null
        val aid = activeProfile?.id
        val currentIdx = list.indexOfFirst { it.id == aid }.takeIf { it >= 0 } ?: 0
        val nextIdx = ((currentIdx + step) % count + count) % count
        return list[nextIdx]
    }

    init {
        loadOrMigrateFromDisk()
        prefs.onActiveProfileMutated = {
            updateActiveProfileFromPrefs(prefs)
        }
    }

    fun attachRuntimeDependencies(
        cacheManager: CacheManager? = null,
        documentCacheManager: DocumentCacheManager? = null,
        connectionManager: ConnectionManager? = null,
        apiClient: ApiClient? = null,
        wsClient: StreamWebSocketClient? = null
    ) {
        if (cacheManager != null) cacheManagerRef = cacheManager
        if (documentCacheManager != null) documentCacheManagerRef = documentCacheManager
        if (connectionManager != null) connectionManagerRef = connectionManager
        if (apiClient != null) apiClientRef = apiClient
        if (wsClient != null) wsClientRef = wsClient

        // Ensure storage namespaces align with the active profile on startup.
        activeProfile?.let { active ->
            cacheManagerRef?.switchStorageNamespace(active.storageNamespace)
            documentCacheManagerRef?.switchStorageNamespace(active.storageNamespace)
            prefs.switchStorageNamespace(active.storageNamespace)
        }
    }

    internal fun runtimeCacheManager(): CacheManager? = cacheManagerRef
    internal fun runtimeDocumentCacheManager(): DocumentCacheManager? = documentCacheManagerRef
    internal fun runtimeConnectionManager(): ConnectionManager? = connectionManagerRef
    internal fun runtimeApiClient(): ApiClient? = apiClientRef
    internal fun runtimeWsClient(): StreamWebSocketClient? = wsClientRef
    internal fun runtimePrefs(): PreferencesManager = prefs

    private fun loadOrMigrateFromDisk() {
        synchronized(lock) {
            val rawJson = prefs.loadGatewayProfilesJson()
            if (!rawJson.isNullOrBlank()) {
                val decoded = runCatching { json.decodeFromString<List<GatewayProfile>>(rawJson) }.getOrNull()
                if (!decoded.isNullOrEmpty()) {
                    val savedActive = prefs.savedActiveProfileId
                    val resolvedActive = if (savedActive != null && decoded.any { it.id == savedActive }) {
                        savedActive
                    } else {
                        decoded.first().id
                    }
                    _profilesFlow.value = decoded
                    _activeProfileIdFlow.value = resolvedActive
                    GatewayRuntimeState.sync(resolvedActive, decoded)
                    return
                }
            }

            // Seamless zero-copy upgrade for existing single-gateway installs.
            val legacyToken = prefs.readLegacyDeviceToken()
            if (!legacyToken.isNullOrBlank()) {
                val legacyId = "legacy_primary"
                val legacyDevId = prefs.readLegacyDeviceId()
                val legacyE2ee = prefs.readLegacyE2eeKeyB64()
                val plat = prefs.gatewayPlatform
                val defaultName = when (plat?.lowercase()) {
                    "windows", "win" -> "Windows 网关".loc
                    "darwin", "macos", "mac" -> "Mac 网关".loc
                    "linux" -> "Linux 网关".loc
                    else -> "电脑网关".loc
                }
                val migrated = GatewayProfile(
                    id = legacyId,
                    gatewayId = null,
                    storageNamespace = "",
                    displayName = defaultName,
                    isCustomName = false,
                    platform = plat,
                    gatewayBaseUrl = prefs.gatewayBaseUrl,
                    primaryCloudUrl = prefs.primaryCloudUrl,
                    lanServerUrl = prefs.lanServerUrl,
                    relayServerUrl = prefs.relayServerUrl,
                    customServerUrl = prefs.customServerUrl,
                    e2eeRequiredHosts = emptyList()
                )
                prefs.writeProfileCredentials(
                    profileId = legacyId,
                    deviceToken = legacyToken,
                    deviceId = legacyDevId,
                    e2eeKeyB64 = legacyE2ee
                )
                val list = listOf(migrated)
                _profilesFlow.value = list
                _activeProfileIdFlow.value = legacyId
                persistLocked(list, legacyId)
            } else {
                _profilesFlow.value = emptyList()
                _activeProfileIdFlow.value = null
                GatewayRuntimeState.sync(null, emptyList())
            }
        }
    }

    private fun persistLocked(list: List<GatewayProfile>, activeId: String?) {
        runCatching {
            prefs.saveGatewayProfilesJson(json.encodeToString(list))
        }
        prefs.savedActiveProfileId = activeId
        GatewayRuntimeState.sync(activeId, list)
    }

    fun profile(id: String): GatewayProfile? = _profilesFlow.value.firstOrNull { it.id == id }

    fun profileMatchingGatewayId(rawGatewayId: String?): GatewayProfile? {
        val clean = rawGatewayId?.trim()?.takeIf { it.isNotEmpty() } ?: return null
        return _profilesFlow.value.firstOrNull { it.gatewayId == clean || it.id == clean }
    }

    fun profileIdContainingCachedCascade(cascadeId: String): String? {
        val clean = cascadeId.trim()
        if (clean.isEmpty()) return null
        val cm = cacheManagerRef ?: CacheManager(appContext)
        for (p in _profilesFlow.value) {
            if (prefs.containsCachedCascade(clean, p.storageNamespace) ||
                cm.containsCachedSession(clean, p.storageNamespace)
            ) {
                return p.id
            }
        }
        return null
    }

    fun requestContext(profileId: String): GatewayRequestContext? {
        val p = profile(profileId) ?: return null
        val token = prefs.readProfileDeviceToken(p.id)
            ?: if (p.id == activeProfileId) prefs.deviceToken else null
        val devId = prefs.readProfileDeviceId(p.id)
            ?: if (p.id == activeProfileId) prefs.deviceId else null
        val e2eeB64 = prefs.readProfileE2eeKeyB64(p.id)
            ?: if (p.id == activeProfileId) prefs.e2eeKeyB64 else null
        val e2eeKey = e2eeB64?.let { E2eeCrypto.decodeBase64Flexible(it) }?.takeIf { it.size == 32 }

        val hosts = HashSet<String>()
        for (h in p.e2eeRequiredHosts) {
            val clean = ConnectionManager.extractHost(h)
            if (clean.isNotEmpty()) hosts.add(clean)
        }
        p.relayServerUrl?.let { r ->
            val h = ConnectionManager.extractHost(r)
            if (h.isNotEmpty()) hosts.add(h)
        }
        p.primaryCloudUrl?.let { c ->
            val h = ConnectionManager.extractHost(c)
            if (h.startsWith("r-")) hosts.add(h)
        }

        return GatewayRequestContext(
            profileId = p.id,
            gatewayId = p.gatewayId,
            displayName = p.resolvedDisplayName,
            deviceId = devId,
            deviceToken = token,
            e2eeKey = e2eeKey,
            e2eeRequiredHosts = hosts,
            lanUrl = p.lanServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() },
            customUrl = p.customServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() },
            cloudUrl = (p.primaryCloudUrl ?: p.relayServerUrl)?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() },
            activeUrl = p.gatewayBaseUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
        )
    }

    fun allRequestContexts(): List<GatewayRequestContext> =
        _profilesFlow.value.mapNotNull { requestContext(it.id) }

    fun upsertPairedProfile(
        gatewayId: String?,
        gatewayName: String?,
        platform: String?,
        deviceId: String,
        deviceToken: String,
        e2eeKeyB64: String?,
        lanUrl: String?,
        relayUrl: String?,
        cloudUrl: String?,
        activeUrl: String,
        e2eeRequiredHosts: List<String>
    ): Pair<GatewayProfile, Boolean> {
        synchronized(lock) {
            val validGwId = gatewayId?.trim()?.takeIf { it.isNotEmpty() }
            val cleanGwName = gatewayName?.trim()?.takeIf { it.isNotEmpty() }
            val cleanCloud = cloudUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
            val cleanLan = lanUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
            val cleanRelay = relayUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
            val cleanActive = activeUrl.trim().trimEnd('/')
            val current = _profilesFlow.value.toMutableList()

            val existingIdx = current.indexOfFirst { existing ->
                if (validGwId != null && (existing.gatewayId == validGwId || existing.id == validGwId)) {
                    true
                } else if (cleanCloud != null && existing.primaryCloudUrl?.trimEnd('/')?.equals(cleanCloud, ignoreCase = true) == true) {
                    true
                } else if (cleanLan != null &&
                    existing.lanServerUrl?.trimEnd('/')?.equals(cleanLan, ignoreCase = true) == true &&
                    (validGwId == null || existing.gatewayId == null)
                ) {
                    true
                } else {
                    false
                }
            }

            val now = System.currentTimeMillis()
            if (existingIdx >= 0) {
                val existing = current[existingIdx]
                val mergedHosts = (existing.e2eeRequiredHosts + e2eeRequiredHosts)
                    .map { ConnectionManager.extractHost(it) }
                    .filter { it.isNotEmpty() }
                    .distinct()
                    .sorted()
                val nextName = when {
                    existing.isCustomName && existing.displayName.isNotBlank() -> existing.displayName
                    cleanGwName != null -> cleanGwName
                    existing.displayName.isNotBlank() -> existing.displayName
                    else -> existing.defaultPlatformLabel
                }
                val updated = existing.copy(
                    gatewayId = validGwId ?: existing.gatewayId,
                    platform = platform?.takeIf { it.isNotBlank() } ?: existing.platform,
                    displayName = nextName,
                    lanServerUrl = cleanLan ?: existing.lanServerUrl,
                    relayServerUrl = cleanRelay ?: existing.relayServerUrl,
                    primaryCloudUrl = if (cleanCloud != null && !ConnectionManager.isLanHost(ConnectionManager.extractHost(cleanCloud))) cleanCloud else existing.primaryCloudUrl,
                    gatewayBaseUrl = cleanActive,
                    e2eeRequiredHosts = mergedHosts,
                    lastUsedAtMs = now
                )
                current[existingIdx] = updated
                prefs.writeProfileCredentials(updated.id, deviceToken, deviceId, e2eeKeyB64)
                _profilesFlow.value = current
                persistLocked(current, _activeProfileIdFlow.value)
                return updated to false
            }

            val newId = validGwId ?: "gw_${UUID.randomUUID().toString().replace("-", "").take(16).lowercase()}"
            val useLegacyNs = current.isEmpty()
            val resolvedName = cleanGwName ?: when (platform?.lowercase()) {
                "windows", "win" -> "Windows 网关".loc
                "darwin", "macos", "mac" -> "Mac 网关".loc
                "linux" -> "Linux 网关".loc
                else -> "电脑网关".loc
            }
            val created = GatewayProfile(
                id = newId,
                gatewayId = validGwId,
                storageNamespace = if (useLegacyNs) "" else newId,
                displayName = resolvedName,
                isCustomName = false,
                platform = platform?.takeIf { it.isNotBlank() },
                gatewayBaseUrl = cleanActive,
                primaryCloudUrl = if (cleanCloud != null && !ConnectionManager.isLanHost(ConnectionManager.extractHost(cleanCloud))) cleanCloud else null,
                lanServerUrl = cleanLan,
                relayServerUrl = cleanRelay,
                customServerUrl = null,
                e2eeRequiredHosts = e2eeRequiredHosts.map { ConnectionManager.extractHost(it) }.filter { it.isNotEmpty() }.distinct().sorted(),
                createdAtMs = now,
                lastUsedAtMs = now
            )
            prefs.writeProfileCredentials(created.id, deviceToken, deviceId, e2eeKeyB64)
            current.add(created)
            _profilesFlow.value = current
            persistLocked(current, _activeProfileIdFlow.value)
            return created to true
        }
    }

    fun setActiveProfileId(id: String?) {
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            if (id != null) {
                val idx = current.indexOfFirst { it.id == id }
                if (idx >= 0) {
                    current[idx] = current[idx].copy(lastUsedAtMs = System.currentTimeMillis())
                }
            }
            _profilesFlow.value = current
            _activeProfileIdFlow.value = id
            persistLocked(current, id)
        }
    }

    fun updateActiveProfileFromPrefs(sourcePrefs: PreferencesManager) {
        synchronized(lock) {
            val aid = _activeProfileIdFlow.value ?: return
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == aid }
            if (idx < 0) return
            val existing = current[idx]
            val updated = existing.copy(
                gatewayBaseUrl = sourcePrefs.gatewayBaseUrl,
                primaryCloudUrl = sourcePrefs.primaryCloudUrl,
                lanServerUrl = sourcePrefs.lanServerUrl,
                relayServerUrl = sourcePrefs.relayServerUrl,
                customServerUrl = sourcePrefs.customServerUrl,
                platform = sourcePrefs.gatewayPlatform ?: existing.platform,
                displayName = if (!existing.isCustomName && existing.displayName.isBlank()) existing.defaultPlatformLabel else existing.displayName
            )
            if (updated != existing) {
                current[idx] = updated
                _profilesFlow.value = current
                persistLocked(current, aid)
            }
        }
    }

    fun backfillGatewayMetadata(profileId: String, gatewayId: String?, gatewayName: String?, platform: String?) {
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == profileId }
            if (idx < 0) return
            val existing = current[idx]
            val cleanGw = gatewayId?.trim()?.takeIf { it.isNotEmpty() }
            val cleanName = gatewayName?.trim()?.takeIf { it.isNotEmpty() }
            val cleanPlat = platform?.trim()?.lowercase()?.takeIf { it.isNotEmpty() }
            val updated = existing.copy(
                gatewayId = cleanGw ?: existing.gatewayId,
                platform = cleanPlat ?: existing.platform,
                displayName = if (!existing.isCustomName && cleanName != null) cleanName else existing.displayName
            )
            if (updated != existing) {
                current[idx] = updated
                _profilesFlow.value = current
                persistLocked(current, _activeProfileIdFlow.value)
            }
        }
    }

    fun updateCloudUrl(profileId: String, cloudUrl: String) {
        val clean = cloudUrl.trim().trimEnd('/')
        if (clean.isEmpty() || ConnectionManager.isLanHost(ConnectionManager.extractHost(clean))) return
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == profileId }
            if (idx < 0) return
            if (current[idx].primaryCloudUrl != clean) {
                current[idx] = current[idx].copy(primaryCloudUrl = clean)
                _profilesFlow.value = current
                persistLocked(current, _activeProfileIdFlow.value)
            }
        }
    }

    fun markActiveE2eeHost(host: String) {
        val clean = ConnectionManager.extractHost(host)
        val aid = _activeProfileIdFlow.value ?: return
        if (clean.isEmpty()) return
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == aid }
            if (idx < 0) return
            if (!current[idx].e2eeRequiredHosts.contains(clean)) {
                current[idx] = current[idx].copy(e2eeRequiredHosts = (current[idx].e2eeRequiredHosts + clean).sorted())
                _profilesFlow.value = current
                persistLocked(current, aid)
            }
        }
    }

    fun renameProfile(id: String, newName: String) {
        val clean = newName.trim().take(24)
        if (clean.isEmpty()) return
        val ctx: GatewayRequestContext?
        val showLabel: Boolean
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == id }
            if (idx < 0) return
            current[idx] = current[idx].copy(displayName = clean, isCustomName = true)
            _profilesFlow.value = current
            persistLocked(current, _activeProfileIdFlow.value)
            showLabel = current.size > 1
        }
        ctx = requestContext(id)
        val fcmToken = prefs.fcmPushToken
        val client = apiClientRef
        if (ctx != null && client != null && !fcmToken.isNullOrBlank()) {
            scope.launch {
                client.registerPushTokenForContext(
                    fcmToken = fcmToken,
                    context = ctx,
                    showGatewayLabel = showLabel,
                    gatewayLabel = clean
                )
            }
        }
    }

    /**
     * Removes a gateway profile, snapshots its credentials before deletion so the remote gateway can be
     * cleanly unregistered & unpaired, clears its isolated caches, and returns `(removed, nextActive)`.
     */
    fun removeProfile(id: String): Pair<GatewayProfile?, GatewayProfile?> {
        val ctxBeforeRemove = requestContext(id)
        val removed: GatewayProfile
        val nextActive: GatewayProfile?
        val remainingSingle: GatewayProfile?
        synchronized(lock) {
            val current = _profilesFlow.value.toMutableList()
            val idx = current.indexOfFirst { it.id == id }
            if (idx < 0) return null to activeProfile
            removed = current.removeAt(idx)
            prefs.clearProfileCredentials(removed.id)
            if (removed.storageNamespace.isEmpty()) {
                prefs.clearLegacyCredentials()
            }
            prefs.clearStorageForNamespace(removed.storageNamespace)
            cacheManagerRef?.clearStorageForNamespace(removed.storageNamespace)
            documentCacheManagerRef?.clearStorageForNamespace(removed.storageNamespace)
            _summariesFlow.value = _summariesFlow.value - removed.id

            val nextActiveId = if (_activeProfileIdFlow.value == removed.id) {
                current.firstOrNull()?.id
            } else {
                _activeProfileIdFlow.value
            }
            _profilesFlow.value = current
            _activeProfileIdFlow.value = nextActiveId
            persistLocked(current, nextActiveId)
            nextActive = nextActiveId?.let { nid -> current.firstOrNull { it.id == nid } }
            remainingSingle = if (current.size == 1) current.first() else null
        }

        val client = apiClientRef
        if (ctxBeforeRemove != null && client != null) {
            scope.launch {
                try {
                    client.unregisterPushTokenForContext(ctxBeforeRemove)
                } catch (_: Exception) {}
                try {
                    client.unpairContext(ctxBeforeRemove)
                } catch (_: Exception) {}
            }
        }

        // If we dropped back down to a single gateway, turn off multi-gateway notification subtitle on it.
        val fcmToken = prefs.fcmPushToken
        if (remainingSingle != null && client != null && !fcmToken.isNullOrBlank()) {
            requestContext(remainingSingle.id)?.let { remCtx ->
                scope.launch {
                    client.registerPushTokenForContext(
                        fcmToken = fcmToken,
                        context = remCtx,
                        showGatewayLabel = false,
                        gatewayLabel = remainingSingle.resolvedDisplayName
                    )
                }
            }
        }
        return removed to nextActive
    }

    fun updateActiveSummary(conversations: List<ConversationItem>) {
        val aid = _activeProfileIdFlow.value ?: return
        var actionCount = 0
        var runningCount = 0
        var unreadCount = 0
        var totalActive = 0
        for (item in conversations) {
            if (item.isSubagent || item.isArchived || item.isDraft) continue
            totalActive++
            when {
                item.status == ConversationStatus.ACTION -> actionCount++
                item.status == ConversationStatus.RUNNING || item.hasRunningTasks || item.hasRunningSubagents || item.isSwarmWorking -> runningCount++
                item.isUnread -> unreadCount++
            }
        }
        val snap = GatewaySummarySnapshot(
            profileId = aid,
            runningCount = runningCount,
            actionCount = actionCount,
            unreadCount = unreadCount,
            totalActive = totalActive,
            isReachable = true,
            isRefreshing = false,
            gatewayName = activeProfile?.resolvedDisplayName,
            updatedAtMs = System.currentTimeMillis()
        )
        _summariesFlow.value = _summariesFlow.value + (aid to snap)
    }

    fun refreshInactiveSummaries(force: Boolean = false, minIntervalMs: Long = 45_000L) {
        val list = _profilesFlow.value
        if (list.size <= 1) return
        val now = System.currentTimeMillis()
        if (!force && (now - lastInactiveRefreshMs) < minIntervalMs) return
        if (!force && inFlightSummaryJob?.isActive == true) return

        val client = apiClientRef ?: return
        inFlightSummaryJob?.cancel()
        lastInactiveRefreshMs = now

        val activeId = _activeProfileIdFlow.value
        val targets = list.filter { it.id != activeId }
        val contexts = targets.mapNotNull { requestContext(it.id) }
        if (contexts.isEmpty()) return

        val marking = _summariesFlow.value.toMutableMap()
        for (ctx in contexts) {
            val prev = marking[ctx.profileId]
            marking[ctx.profileId] = prev?.copy(isRefreshing = true) ?: GatewaySummarySnapshot(
                profileId = ctx.profileId,
                isReachable = true,
                isRefreshing = true,
                gatewayName = ctx.displayName,
                updatedAtMs = 0L
            )
        }
        _summariesFlow.value = marking

        inFlightSummaryJob = scope.launch {
            val results = coroutineScope {
                contexts.map { ctx ->
                    async {
                        client.fetchGatewaySummary(ctx)
                    }
                }.awaitAll()
            }
            val updated = _summariesFlow.value.toMutableMap()
            for ((snap, meta) in results) {
                updated[snap.profileId] = snap
                if (meta != null) {
                    backfillGatewayMetadata(snap.profileId, meta.gatewayId, meta.gatewayName, meta.platform)
                }
            }
            _summariesFlow.value = updated
        }
    }

    companion object {
        @Volatile
        private var instance: GatewayProfileStore? = null

        fun getOrCreate(context: Context, prefs: PreferencesManager): GatewayProfileStore {
            return instance ?: synchronized(this) {
                instance ?: GatewayProfileStore(context.applicationContext, prefs).also { instance = it }
            }
        }

        fun sharedOrNull(): GatewayProfileStore? = instance
    }
}
