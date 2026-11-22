package com.antigravity.mobile.data.service

import android.util.Log
import okhttp3.ConnectionPool
import okhttp3.Dns
import okhttp3.Interceptor
import okhttp3.Response
import java.io.ByteArrayOutputStream
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.util.Collections
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

/**
 * Cloudflare Anycast 智能动态多线路优选 DNS 解析器 (三层防 1034 自愈版)
 *
 * 核心防护：
 * 1. 仅保留实测 HTTP 200 可用的亚太/全球 Anycast 边缘节点（已彻底移除 108.162.192.1 / 172.64.32.1 /
 *    162.159.0.1 / 198.41.214.1 等触发 Cloudflare Error 1034 的受限边缘 IP）。
 * 2. 后台并发赛马测速升级为 TLS + HTTPS 7 层验活（请求 Cloudflare 边缘原生 /cdn-cgi/trace，零 Worker 消耗），
 *    仅返回 HTTP 200 的节点才允许进入 verifiedIps；返回 403/1034 的节点自动拉黑 24 小时。
 * 3. 在首个节点通过 HTTPS 验活前（或全部候选节点受限时），优先返回系统原生 DNS，杜绝冷启动误连毒丸 IP。
 */
object CloudflareOptimizedDns : Dns {
    private const val TAG = "CFOptimizedDns"
    private const val PROBE_HOST = "dispatcher.jiuge.space"
    private const val PROBE_REQUEST =
        "GET /cdn-cgi/trace HTTP/1.1\r\n" +
            "Host: $PROBE_HOST\r\n" +
            "User-Agent: Multigravity-Probe/1.0\r\n" +
            "Connection: close\r\n\r\n"
    private const val PROBE_TIMEOUT_MS = 1800
    private const val PROBE_BATCH_AWAIT_MS = 2200L
    private const val BAN_TTL_MS = 24L * 60L * 60L * 1000L
    private const val BYPASS_GRACE_MS = 30_000L

    // 经无代理直连实测 HTTP 200 的健康 Cloudflare Anycast 边缘节点池
    private val CANDIDATE_EDGE_IPS = listOf(
        // 电信 / 多线 优质亚太直连段
        "172.64.153.208",  // Tokyo / Narita
        "104.20.23.208",   // Tokyo / Narita
        // 移动 / 亚太 CMI 优质直连段
        "162.159.131.182", // Hong Kong Anycast
        "162.159.153.1",
        "162.159.192.1",
        // 联通 / 东京 / 香港经典优质段
        "104.18.0.1",
        "104.18.2.161",    // Hong Kong Anycast
        "104.16.160.1",    // Asia Anycast
        "104.26.0.1",
        // 全球泛播高可用兜底候选段
        "188.114.97.1",
        "172.67.75.1",
        "104.19.16.1"
    )

    private const val MAX_CONCURRENT_PROBE_IPS = 6
    private val verifiedIps = CopyOnWriteArrayList<String>()
    private val bannedIps = ConcurrentHashMap<String, Long>()
    private val bypassUntilMs = AtomicLong(0L)
    private val isProbing = AtomicBoolean(false)
    @Volatile private var probeCursor = 0
    private val probeScheduler = Executors.newSingleThreadScheduledExecutor { r ->
        Thread(r, "cf-dns-scheduler").apply { isDaemon = true }
    }
    private val probeWorkerPool = Executors.newFixedThreadPool(MAX_CONCURRENT_PROBE_IPS) { r ->
        Thread(r, "cf-dns-probe").apply { isDaemon = true }
    }

    @Volatile private var lastProbeTime = 0L

    init {
        // App 启动时异步触发一次 HTTPS 7 层赛马测速
        triggerProbeIfNeeded(force = true)
    }

    fun isCloudflareTunnelHost(hostname: String): Boolean {
        val lowerHost = hostname.trim().lowercase()
        // Only 1st-level *.jiuge.space (3 labels) and jiuge.space are Cloudflare Orange-Cloud proxied;
        // 2nd-level regional VPS subdomains (e.g. r-xxxx.jp1.jiuge.space, >=4 labels) use direct DNS.
        return lowerHost == "jiuge.space" ||
            (lowerHost.endsWith(".jiuge.space") && lowerHost.split('.').size == 3)
    }

    fun isError1034Body(body: String): Boolean {
        if (body.isBlank()) return false
        val lower = body.lowercase()
        return lower.contains("error code: 1034") ||
            (lower.contains("1034") && lower.contains("edge ip"))
    }

    override fun lookup(hostname: String): List<InetAddress> {
        if (!isCloudflareTunnelHost(hostname)) {
            return Dns.SYSTEM.lookup(hostname)
        }

        // 异步检查并触发轻量级 HTTPS 探测以动态维持最优列表
        triggerProbeIfNeeded(force = false)

        val now = System.currentTimeMillis()
        val systemIps = try {
            Dns.SYSTEM.lookup(hostname)
        } catch (e: Exception) {
            Log.w(TAG, "System DNS lookup failed for $hostname: ${e.message}")
            emptyList()
        }

        val results = ArrayList<InetAddress>()
        val useOptimized = now >= bypassUntilMs.get()
        if (useOptimized) {
            for (ip in verifiedIps) {
                if (isBanned(ip, now)) continue
                try {
                    results.add(InetAddress.getByName(ip))
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to parse verified IP $ip: ${e.message}")
                }
            }
        }

        for (sysIp in systemIps) {
            if (!results.contains(sysIp)) {
                results.add(sysIp)
            }
        }

        if (results.isNotEmpty()) {
            return results
        }
        return Dns.SYSTEM.lookup(hostname)
    }

    /**
     * 运行时检测到 Cloudflare HTTP 403 / error code: 1034 时调用：
     * 立即拉黑当前首选优选 IP，短期切换回系统原生 DNS，并在后台重新验活健康节点。
     */
    fun reportEdgeRestricted(ip: String? = null) {
        val now = System.currentTimeMillis()
        val target = ip?.takeIf { it.isNotBlank() } ?: verifiedIps.firstOrNull()
        if (target != null) {
            banIp(target, "runtime HTTP 403 error code 1034")
        } else {
            verifiedIps.clear()
        }
        if (verifiedIps.none { !isBanned(it, now) }) {
            bypassUntilMs.set(now + BYPASS_GRACE_MS)
        }
        triggerProbeIfNeeded(force = true)
    }

    fun triggerProbeIfNeeded(force: Boolean = false) {
        val now = System.currentTimeMillis()
        val hasVerified = verifiedIps.any { !isBanned(it, now) }
        val minIntervalMs = if (hasVerified) 300_000L else 60_000L
        if (!force && now - lastProbeTime < minIntervalMs) {
            return
        }
        if (isProbing.compareAndSet(false, true)) {
            lastProbeTime = now
            probeScheduler.submit {
                try {
                    probeFastestIpsConcurrently()
                } finally {
                    isProbing.set(false)
                }
            }
        }
    }

    private fun isBanned(ip: String, now: Long): Boolean {
        val expiresAt = bannedIps[ip] ?: return false
        if (now >= expiresAt) {
            bannedIps.remove(ip, expiresAt)
            return false
        }
        return true
    }

    private fun banIp(ip: String, reason: String) {
        val expiresAt = System.currentTimeMillis() + BAN_TTL_MS
        bannedIps[ip] = expiresAt
        verifiedIps.remove(ip)
        Log.w(TAG, "Banned Cloudflare edge IP $ip for 24h ($reason)")
    }

    private fun selectProbeBatch(): List<String> {
        val now = System.currentTimeMillis()
        val selected = LinkedHashSet<String>(MAX_CONCURRENT_PROBE_IPS)
        // Always re-verify current top 2 winners first so a newly restricted IP is evicted promptly.
        for (ip in verifiedIps.take(2)) {
            if (!isBanned(ip, now)) {
                selected.add(ip)
            }
        }
        val poolSize = CANDIDATE_EDGE_IPS.size
        var scanned = 0
        var idx = probeCursor % poolSize
        while (selected.size < MAX_CONCURRENT_PROBE_IPS && scanned < poolSize) {
            val candidate = CANDIDATE_EDGE_IPS[idx]
            if (!isBanned(candidate, now)) {
                selected.add(candidate)
            }
            idx = (idx + 1) % poolSize
            scanned++
        }
        probeCursor = idx
        return selected.toList()
    }

    private enum class ProbeStatus { OK, RESTRICTED, FAILED }
    private data class ProbeOutcome(val status: ProbeStatus, val rttMs: Long = -1L)

    /**
     * 有界 6 路并发 HTTPS 7 层验活测速（TLS + SNI + GET /cdn-cgi/trace -> HTTP 200）
     */
    private fun probeFastestIpsConcurrently() {
        val batch = selectProbeBatch()
        if (batch.isEmpty()) return

        val scoredList = Collections.synchronizedList(mutableListOf<Pair<String, Long>>())
        val failedInBatch = Collections.synchronizedSet(mutableSetOf<String>())
        val latch = CountDownLatch(batch.size)

        for (ip in batch) {
            probeWorkerPool.submit {
                try {
                    val outcome = httpsTracePing(ip, PROBE_TIMEOUT_MS)
                    when (outcome.status) {
                        ProbeStatus.OK -> {
                            if (outcome.rttMs in 0..PROBE_TIMEOUT_MS) {
                                scoredList.add(ip to outcome.rttMs)
                            } else {
                                failedInBatch.add(ip)
                            }
                        }
                        ProbeStatus.RESTRICTED -> {
                            banIp(ip, "HTTP 403 / 1034 during /cdn-cgi/trace probe")
                            failedInBatch.add(ip)
                        }
                        ProbeStatus.FAILED -> {
                            failedInBatch.add(ip)
                        }
                    }
                } finally {
                    latch.countDown()
                }
            }
        }

        try {
            latch.await(PROBE_BATCH_AWAIT_MS, TimeUnit.MILLISECONDS)
        } catch (_: Exception) {}

        val now = System.currentTimeMillis()
        val previousVerified = ArrayList(verifiedIps)
        val merged = LinkedHashSet<String>()

        if (scoredList.isNotEmpty()) {
            val sorted = ArrayList(scoredList)
            sorted.sortBy { it.second }
            val sortedIps = sorted.map { it.first }
            Log.d(TAG, "HTTPS trace race completed (${batch.size} candidates, ${sortedIps.size} healthy). Winner: ${sortedIps.first()} (RTT: ${sorted.first().second}ms)")
            merged.addAll(sortedIps)
        }
        for (ip in previousVerified) {
            if (!failedInBatch.contains(ip) && !isBanned(ip, now)) {
                merged.add(ip)
            }
        }
        verifiedIps.clear()
        verifiedIps.addAll(merged)
        if (verifiedIps.isNotEmpty()) {
            bypassUntilMs.set(0L)
        }
    }

    private fun httpsTracePing(ip: String, timeoutMs: Int): ProbeOutcome {
        val start = System.nanoTime()
        var rawSocket: Socket? = null
        var sslSocket: SSLSocket? = null
        return try {
            rawSocket = Socket()
            rawSocket.connect(InetSocketAddress(ip, 443), timeoutMs)
            rawSocket.soTimeout = timeoutMs

            val factory = SSLSocketFactory.getDefault() as SSLSocketFactory
            sslSocket = factory.createSocket(rawSocket, PROBE_HOST, 443, true) as SSLSocket
            sslSocket.soTimeout = timeoutMs
            val params = sslSocket.sslParameters
            params.serverNames = listOf(SNIHostName(PROBE_HOST))
            params.endpointIdentificationAlgorithm = "HTTPS"
            sslSocket.sslParameters = params

            sslSocket.startHandshake()

            val out = sslSocket.outputStream
            out.write(PROBE_REQUEST.toByteArray(Charsets.US_ASCII))
            out.flush()

            val input = sslSocket.inputStream
            val buf = ByteArray(512)
            val acc = ByteArrayOutputStream()
            while (acc.size() < 512) {
                val n = input.read(buf, 0, minOf(buf.size, 512 - acc.size()))
                if (n <= 0) break
                acc.write(buf, 0, n)
                val partial = acc.toString(Charsets.UTF_8.name())
                if (partial.contains("\r\n\r\n") || (partial.contains("\r\n") && acc.size() >= 128)) {
                    break
                }
            }

            val respText = acc.toString(Charsets.UTF_8.name())
            val statusLine = respText.lineSequence().firstOrNull()?.trim().orEmpty()
            val statusCode = statusLine.split(' ').getOrNull(1)?.toIntOrNull() ?: -1
            val elapsedMs = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start)

            when {
                statusCode == 200 -> ProbeOutcome(ProbeStatus.OK, elapsedMs)
                statusCode == 403 || isError1034Body(respText) -> ProbeOutcome(ProbeStatus.RESTRICTED)
                else -> ProbeOutcome(ProbeStatus.FAILED)
            }
        } catch (_: Exception) {
            ProbeOutcome(ProbeStatus.FAILED)
        } finally {
            try {
                sslSocket?.close()
            } catch (_: Exception) {}
            try {
                rawSocket?.close()
            } catch (_: Exception) {}
        }
    }
}

/**
 * Cloudflare Error 1034 (HTTP 403 Edge IP Restricted) 运行时自愈拦截器：
 * 命中时立即拉黑当前优选 IP、清空连接池复用连接，并自动通过系统原生 DNS 重试一次。
 */
class CloudflareEdgeFallbackInterceptor(
    private val connectionPool: ConnectionPool
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val response = chain.proceed(request)
        if (response.code != 403 || !CloudflareOptimizedDns.isCloudflareTunnelHost(request.url.host)) {
            return response
        }

        val snippet = try {
            response.peekBody(512).string()
        } catch (_: Exception) {
            ""
        }
        if (!CloudflareOptimizedDns.isError1034Body(snippet)) {
            return response
        }

        Log.w(
            "CFOptimizedDns",
            "Detected Cloudflare Error 1034 on ${request.url.host}; banning active edge IP and retrying via System DNS"
        )
        CloudflareOptimizedDns.reportEdgeRestricted()
        connectionPool.evictAll()
        if (request.body?.isOneShot() == true) {
            return response
        }
        response.close()
        return chain.proceed(request)
    }
}
