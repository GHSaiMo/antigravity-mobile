package com.antigravity.mobile.data.service

import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.net.URI
import java.util.concurrent.ConcurrentHashMap

/**
 * Stateless mobile auto-discovery for multi-VPS relay pools.
 * Queries `GET https://dispatcher.multigravity.app/api/relay/pool` (falling back to
 * `https://dispatcher.jiuge.space/api/relay/pool`, cached 15m with 5m force-refresh cooldown),
 * expands all `host_template` entries with the machine's deterministic `sid`, concurrently probes
 * `/healthz`, and re-locks onto the active VPS node when a paired gateway fails over or migrates.
 */
object RelayPoolResolver {
    private const val POOL_URL = "https://dispatcher.multigravity.app/api/relay/pool"
    private const val LEGACY_POOL_URL = "https://dispatcher.jiuge.space/api/relay/pool"
    private const val CACHE_TTL_MS = 15 * 60_000L
    private const val FORCE_REFRESH_COOLDOWN_MS = 5 * 60_000L

    data class PoolNode(
        val id: String,
        val region: String,
        val priority: Int,
        val hostTemplate: String
    )

    @Volatile private var cachedNodes: List<PoolNode> = emptyList()
    @Volatile private var cachedAtMs: Long = 0L
    private val trustedSuffixes = ConcurrentHashMap.newKeySet<String>()

    fun extractSid(cloudUrl: String?): String? {
        val raw = cloudUrl?.trim()?.takeIf { it.isNotEmpty() } ?: return null
        val host = try {
            URI(if (!raw.contains("://")) "https://$raw" else raw).host?.lowercase().orEmpty()
        } catch (_: Exception) {
            return null
        }
        val firstLabel = host.substringBefore('.')
        val candidate = if (firstLabel.startsWith("r-")) firstLabel.removePrefix("r-") else firstLabel
        if (candidate.length !in 6..16) return null
        if (!candidate.all { it in '0'..'9' || it in 'a'..'f' }) return null
        return candidate
    }

    fun isPoolDomainHost(host: String): Boolean {
        val clean = ConnectionManager.extractHost(host)
        if (clean.isEmpty()) return false
        return trustedSuffixes.any { clean.endsWith(it) }
    }

    /**
     * Returns true if [hostOrUrl] belongs to the official managed relay pool (`.multigravity.app`,
     * `.jiuge.space`, or a suffix published by the official pool manifest). Custom self-hosted
     * BYO-Relay domains return false so clients never contact the official dispatcher.
     */
    fun isOfficialOrPoolHost(hostOrUrl: String): Boolean {
        val clean = ConnectionManager.extractHost(hostOrUrl)
        if (clean.isEmpty()) return false
        if (clean.endsWith(".multigravity.app") || clean == "multigravity.app" ||
            clean.endsWith(".jiuge.space") || clean == "jiuge.space"
        ) {
            return true
        }
        return isPoolDomainHost(clean)
    }

    fun fetchPoolNodes(httpClient: OkHttpClient, forceRefresh: Boolean = false): List<PoolNode> {
        val now = System.currentTimeMillis()
        val ttl = if (forceRefresh) FORCE_REFRESH_COOLDOWN_MS else CACHE_TTL_MS
        if (cachedNodes.isNotEmpty() && (now - cachedAtMs) < ttl) {
            return cachedNodes
        }
        for (endpoint in listOf(POOL_URL, LEGACY_POOL_URL)) {
            try {
                val req = Request.Builder().url(endpoint).get().build()
                httpClient.newCall(req).execute().use { resp ->
                    if (!resp.isSuccessful) return@use
                    val body = resp.body?.string().orEmpty()
                    val json = JSONObject(body)
                    val arr = json.optJSONArray("nodes") ?: return@use
                    val list = mutableListOf<PoolNode>()
                    for (i in 0 until arr.length()) {
                        val obj = arr.optJSONObject(i) ?: continue
                        val id = obj.optString("id").trim()
                        val tmpl = obj.optString("host_template").trim().lowercase()
                        if (id.isEmpty() || tmpl.isEmpty() || !tmpl.contains("{sid}")) continue
                        val dotIdx = tmpl.indexOf('.')
                        if (dotIdx > 0 && dotIdx < tmpl.length - 2) {
                            trustedSuffixes.add(tmpl.substring(dotIdx))
                        }
                        list.add(
                            PoolNode(
                                id = id,
                                region = obj.optString("region", "global"),
                                priority = obj.optInt("priority", 100),
                                hostTemplate = tmpl
                            )
                        )
                    }
                    if (list.isNotEmpty()) {
                        cachedNodes = list
                        cachedAtMs = now
                        return cachedNodes
                    }
                }
            } catch (_: Exception) {
                // Fall through to next endpoint
            }
        }
        return cachedNodes
    }

    suspend fun resolveReachableCloudUrl(
        currentCloudUrl: String?,
        prefs: PreferencesManager,
        connectionManager: ConnectionManager,
        httpClient: OkHttpClient
    ): String? {
        if (!E2eeCrypto.isReady(prefs)) return null
        if (currentCloudUrl == null || !isOfficialOrPoolHost(currentCloudUrl)) return null
        val sid = extractSid(currentCloudUrl) ?: return null
        val currentClean = currentCloudUrl?.trim()?.trimEnd('/')?.lowercase()

        val seen = linkedSetOf<String>()
        if (currentClean != null) seen.add(currentClean)

        val initialNodes = fetchPoolNodes(httpClient, forceRefresh = false)
        val initialCandidates = expandCandidates(initialNodes, sid, seen)
        val initialBest = probeFastestReachable(initialCandidates) { connectionManager.testSingleEndpoint(it) }
        if (initialBest != null) {
            val bestHost = ConnectionManager.extractHost(initialBest.urlString)
            if (bestHost.isNotEmpty()) {
                E2eeCrypto.markHostRequiresE2ee(bestHost)
            }
            return initialBest.urlString
        }

        // All cached peers failed: allow one cooldown-gated refresh in case a new VPS joined the pool.
        val refreshedNodes = fetchPoolNodes(httpClient, forceRefresh = true)
        val extraCandidates = expandCandidates(refreshedNodes, sid, seen)
        val extraBest = probeFastestReachable(extraCandidates) { connectionManager.testSingleEndpoint(it) }
        if (extraBest != null) {
            val bestHost = ConnectionManager.extractHost(extraBest.urlString)
            if (bestHost.isNotEmpty()) {
                E2eeCrypto.markHostRequiresE2ee(bestHost)
            }
            return extraBest.urlString
        }

        // Secondary fallback: if the dispatcher or every peer relay node is temporarily unreachable,
        // try the DualTrack legacy Cloudflare tunnel domain (`https://<sid>.jiuge.space`) so the
        // client stays online and can re-learn the active relay node via X-Antigravity-Cloud-URL.
        val currentHost = currentCloudUrl?.let { ConnectionManager.extractHost(it) }.orEmpty()
        if (currentHost.endsWith(".jiuge.space") || currentHost.endsWith(".multigravity.app")) {
            val legacyCfUrl = "https://$sid.jiuge.space"
            if (legacyCfUrl != currentClean) {
                val cfStatus = connectionManager.testSingleEndpoint(legacyCfUrl)
                if (cfStatus.isReachable) {
                    return cfStatus.urlString
                }
            }
        }
        return null
    }

    /**
     * Classifies outer HTTP status/body returned before a pairing request reaches the gateway
     * (e.g. mgy-relay `503 gateway_offline`, `502 gateway_unreachable`, `429 tunnel_busy`).
     * Returns `(isRelayLayer, isGatewayOffline)`.
     */
    fun classifyRelayPairingFailure(outerCode: Int, rawBody: String): Pair<Boolean, Boolean> {
        val snippet = rawBody.take(320).lowercase()
        return when (outerCode) {
            503 -> {
                if (snippet.contains("upstream_not_connected") ||
                    snippet.contains("antigravity upstream not connected") ||
                    snippet.contains("language_server")
                ) {
                    false to false
                } else {
                    true to snippet.contains("gateway_offline")
                }
            }
            502, 520, 521, 522, 523, 524 -> true to false
            429 -> snippet.contains("tunnel_busy") to false
            else -> false to false
        }
    }

    /**
     * Discovers an active peer VPS relay node during initial pairing (before E2EE device keys
     * are persisted) when the QR code's relay host returns HTTP 502/503 (`gateway_offline`)
     * or fails transport.
     *
     * Security:
     * - Strictly restricted to official managed relay pool hosts ([isOfficialOrPoolHost]);
     *   custom self-hosted BYO-Relay domains return null without contacting the dispatcher.
     */
    suspend fun discoverPeerRelayForPairing(
        failedUrl: String,
        httpClient: OkHttpClient,
        probeEndpoint: suspend (String) -> EndpointHealthStatus
    ): String? {
        if (!isOfficialOrPoolHost(failedUrl)) return null
        val sid = extractSid(failedUrl) ?: return null
        val failedClean = failedUrl.trim().trimEnd('/').lowercase()

        val seen = linkedSetOf(failedClean)
        val initialNodes = fetchPoolNodes(httpClient, forceRefresh = false)
        val initialCandidates = expandCandidates(initialNodes, sid, seen)
        val initialBest = probeFastestReachable(initialCandidates, probeEndpoint)
        if (initialBest != null) {
            val bestHost = ConnectionManager.extractHost(initialBest.urlString)
            if (bestHost.isNotEmpty()) {
                E2eeCrypto.markHostRequiresE2ee(bestHost)
            }
            return initialBest.urlString
        }

        // All cached peers failed: allow one cooldown-gated refresh in case a new VPS joined the pool.
        val refreshedNodes = fetchPoolNodes(httpClient, forceRefresh = true)
        val extraCandidates = expandCandidates(refreshedNodes, sid, seen)
        val extraBest = probeFastestReachable(extraCandidates, probeEndpoint)
        if (extraBest != null) {
            val bestHost = ConnectionManager.extractHost(extraBest.urlString)
            if (bestHost.isNotEmpty()) {
                E2eeCrypto.markHostRequiresE2ee(bestHost)
            }
            return extraBest.urlString
        }

        val failedHost = ConnectionManager.extractHost(failedUrl)
        if (failedHost.endsWith(".jiuge.space") || failedHost.endsWith(".multigravity.app")) {
            val legacyCfUrl = "https://$sid.jiuge.space"
            if (legacyCfUrl != failedClean) {
                val cfStatus = probeEndpoint(legacyCfUrl)
                if (cfStatus.isReachable) {
                    return cfStatus.urlString
                }
            }
        }
        return null
    }

    private fun expandCandidates(
        nodes: List<PoolNode>,
        sid: String,
        seen: MutableSet<String>
    ): List<String> {
        val result = mutableListOf<String>()
        for (node in nodes) {
            val host = node.hostTemplate.replace("{sid}", sid).trim().lowercase()
            if (host.isEmpty() || !host.contains('.')) continue
            val urlStr = "https://$host"
            if (seen.add(urlStr)) {
                result.add(urlStr)
            }
        }
        return result
    }

    private suspend fun probeFastestReachable(
        candidates: List<String>,
        probeEndpoint: suspend (String) -> EndpointHealthStatus
    ): EndpointHealthStatus? {
        if (candidates.isEmpty()) return null
        val statuses = coroutineScope {
            candidates.map { cand ->
                async { probeEndpoint(cand) }
            }.awaitAll()
        }
        return statuses.filter { it.isReachable }.minByOrNull { it.latencyMs }
    }
}
