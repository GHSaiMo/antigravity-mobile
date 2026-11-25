package com.antigravity.mobile.data.model

import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.E2eeCrypto
import com.antigravity.mobile.ui.util.loc
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong

/**
 * Thread-safe monotonic epoch and active-profile runtime snapshot readable from any thread
 * (including OkHttp interceptors and background workers) without UI-thread or lock contention.
 */
object GatewayRuntimeState {
    private val epochCounter = AtomicLong(1L)

    @Volatile
    var activeProfileId: String? = null
        private set

    @Volatile
    var activeStorageNamespace: String = ""
        private set

    private val profilesMap = ConcurrentHashMap<String, GatewayProfile>()

    val epoch: Long
        get() = epochCounter.get()

    fun bumpEpoch(): Long {
        var next = epochCounter.incrementAndGet()
        if (next == 0L) {
            next = epochCounter.incrementAndGet()
        }
        return next
    }

    fun sync(activeId: String?, profiles: List<GatewayProfile>) {
        val nextMap = HashMap<String, GatewayProfile>(profiles.size)
        for (p in profiles) {
            nextMap[p.id] = p
        }
        profilesMap.clear()
        profilesMap.putAll(nextMap)
        activeProfileId = activeId
        activeStorageNamespace = activeId?.let { nextMap[it]?.storageNamespace }.orEmpty()
    }

    fun profile(id: String): GatewayProfile? = profilesMap[id]

    fun allProfilesSnapshot(): List<GatewayProfile> = profilesMap.values.toList()
}

@Serializable
data class GatewayProfile(
    val id: String,
    val gatewayId: String? = null,
    /**
     * Empty string `""` denotes the legacy default storage namespace (zero-copy upgrade for existing installs).
     * Additional gateways use `id` as their isolated cache/SharedPreferences namespace.
     */
    val storageNamespace: String = "",
    val displayName: String = "",
    val isCustomName: Boolean = false,
    val platform: String? = null,
    val gatewayBaseUrl: String? = null,
    val primaryCloudUrl: String? = null,
    val lanServerUrl: String? = null,
    val relayServerUrl: String? = null,
    val customServerUrl: String? = null,
    val e2eeRequiredHosts: List<String> = emptyList(),
    val createdAtMs: Long = System.currentTimeMillis(),
    val lastUsedAtMs: Long = System.currentTimeMillis()
) {
    val defaultPlatformLabel: String
        get() = when (platform?.lowercase()) {
            "windows", "win" -> "Windows 网关".loc
            "darwin", "macos", "mac" -> "Mac 网关".loc
            "linux" -> "Linux 网关".loc
            else -> "电脑网关".loc
        }

    val resolvedDisplayName: String
        get() = displayName.trim().ifEmpty { defaultPlatformLabel }

    fun candidateUrls(isCellular: Boolean): List<String> {
        val ordered = if (isCellular) {
            listOf(gatewayBaseUrl, customServerUrl, primaryCloudUrl, relayServerUrl)
        } else {
            listOf(lanServerUrl, gatewayBaseUrl, customServerUrl, primaryCloudUrl, relayServerUrl)
        }
        val out = mutableListOf<String>()
        val seen = HashSet<String>()
        for (raw in ordered) {
            val clean = raw?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() } ?: continue
            val host = ConnectionManager.extractHost(clean)
            if (isCellular && ConnectionManager.isLanHost(host)) continue
            val key = clean.lowercase()
            if (seen.add(key)) {
                out.add(clean)
            }
        }
        return out
    }
}

/**
 * Self-contained, side-effect-free credentials and endpoints for sending requests to any paired gateway
 * (active or inactive) without reading or mutating global `PreferencesManager` state.
 */
data class GatewayRequestContext(
    val profileId: String,
    val gatewayId: String?,
    val displayName: String,
    val deviceId: String?,
    val deviceToken: String?,
    val e2eeKey: ByteArray?,
    val e2eeRequiredHosts: Set<String>,
    val lanUrl: String?,
    val customUrl: String?,
    val cloudUrl: String?,
    val activeUrl: String?
) {
    val isE2eeReady: Boolean
        get() = !deviceId.isNullOrBlank() && e2eeKey != null && e2eeKey.size == 32

    fun shouldEncrypt(scheme: String, host: String, path: String): Boolean {
        if (!scheme.equals("https", ignoreCase = true) && !scheme.equals("wss", ignoreCase = true)) {
            return false
        }
        if (path == "/healthz" || path.startsWith("/api/v1/e2ee/")) {
            return false
        }
        if (!isE2eeReady) return false
        val clean = ConnectionManager.extractHost(host)
        if (clean.isEmpty() || ConnectionManager.isLanHost(clean) || ConnectionManager.isTailscaleHost(clean)) {
            return false
        }
        return clean.startsWith("r-") || e2eeRequiredHosts.contains(clean)
    }

    fun candidateUrls(isCellular: Boolean): List<String> {
        val ordered = if (isCellular) {
            listOf(activeUrl, customUrl, cloudUrl)
        } else {
            listOf(lanUrl, activeUrl, customUrl, cloudUrl)
        }
        val out = mutableListOf<String>()
        val seen = HashSet<String>()
        for (raw in ordered) {
            val clean = raw?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() } ?: continue
            val host = ConnectionManager.extractHost(clean)
            if (isCellular && ConnectionManager.isLanHost(host)) continue
            val key = clean.lowercase()
            if (seen.add(key)) {
                out.add(clean)
            }
        }
        return out
    }
}

data class GatewaySummarySnapshot(
    val profileId: String,
    val runningCount: Int = 0,
    val actionCount: Int = 0,
    val unreadCount: Int = 0,
    val totalActive: Int = 0,
    val isReachable: Boolean = true,
    val isRefreshing: Boolean = false,
    val gatewayName: String? = null,
    val updatedAtMs: Long = 0L
)

object GatewayProfileUtils {
    /**
     * Derives the exact same `"gw_" + hex(sha256(pubBytes)[0..<8])` identifier as Go
     * `e2ee.GatewayIDFromPubKeyB64` and iOS `GatewayProfileStore.deriveGatewayId`.
     */
    fun deriveGatewayId(pubKeyB64: String?): String? {
        val clean = pubKeyB64?.trim()?.takeIf { it.isNotEmpty() } ?: return null
        val raw = E2eeCrypto.decodeBase64Flexible(clean) ?: return null
        if (raw.size != 65) return null
        val digest = MessageDigest.getInstance("SHA-256").digest(raw)
        val hex = digest.take(8).joinToString("") { "%02x".format(it) }
        return "gw_$hex"
    }
}
