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
 * `https://dispatcher.jiuge.space/api/relay/pool`, cached 60s), expands all
 * `host_template` entries with the machine's deterministic `sid`, concurrently probes `/healthz`,
 * and re-locks onto the active VPS node when a paired gateway fails over or migrates.
 */
object RelayPoolResolver {
    private const val POOL_URL = "https://dispatcher.multigravity.app/api/relay/pool"
    private const val LEGACY_POOL_URL = "https://dispatcher.jiuge.space/api/relay/pool"
    private const val CACHE_TTL_MS = 60_000L

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
        if (!forceRefresh && cachedNodes.isNotEmpty() && (now - cachedAtMs) < CACHE_TTL_MS) {
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

        val nodes = fetchPoolNodes(httpClient, forceRefresh = true)
        val relayCandidates = linkedSetOf<String>()
        for (node in nodes) {
            val host = node.hostTemplate.replace("{sid}", sid).trim().lowercase()
            if (host.isEmpty() || !host.contains('.')) continue
            val urlStr = "https://$host"
            if (urlStr != currentClean) {
                relayCandidates.add(urlStr)
            }
        }

        if (relayCandidates.isNotEmpty()) {
            val statuses = coroutineScope {
                relayCandidates.map { cand ->
                    async { connectionManager.testSingleEndpoint(cand) }
                }.awaitAll()
            }
            val best = statuses.filter { it.isReachable }.minByOrNull { it.latencyMs }
            if (best != null) {
                val bestHost = ConnectionManager.extractHost(best.urlString)
                if (bestHost.isNotEmpty()) {
                    E2eeCrypto.markHostRequiresE2ee(bestHost)
                }
                return best.urlString
            }
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
}
