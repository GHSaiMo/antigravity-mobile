package com.antigravity.mobile.data.service

import android.util.Log
import okhttp3.Dns
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.util.Collections
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Cloudflare Anycast 智能动态多线路优选 DNS 解析器 (方案 A 深度落地)
 *
 * 核心原理：
 * 1. Cloudflare Anycast 本身并不“默认”指向亚太。若由国内运营商默认 DNS 解析，电信/联通骨干网
 *    往往会将流量默认路由至高延迟的美西（圣何塞/洛杉矶，200~300ms）。
 * 2. 只有特定 IP 段在电信、联通、移动的 BGP 路由中会被就近调度至亚太边缘机房（如东京 NRT、香港 HKG）。
 * 3. 本解析器内置覆盖电信、联通、移动全运营商的高概率亚太直连 IP 候选池，并在后台启动高并发
 *    TCP 握手“赛马（Race）测速”，毫秒级动态锁定当前网络（Wi-Fi / 5G / 各地运营商）下真实延迟最低的节点（实测 60~70ms）。
 * 4. 解析结果优先返回延迟最低的 Top 节点，并在末尾保留系统原生 DNS 作为终极安全兜底。
 */
object CloudflareOptimizedDns : Dns {
    private const val TAG = "CFOptimizedDns"

    // 覆盖国内电信、联通、移动全运营商的核心 Anycast 候选节点池
    private val CANDIDATE_EDGE_IPS = listOf(
        // 电信 / 多线 优质亚太直连段 (108.162.*, 172.64.*)
        "172.64.153.208",  // 实测极速 ~67ms (Tokyo / Narita)
        "108.162.192.1",   // 实测极速 ~70ms (Asia Edge)
        "172.64.32.1",     // 实测极速 ~74ms
        "172.64.0.1",
        // 移动 / 亚太 CMI 优质直连段 (162.159.*, 141.101.*)
        "162.159.0.1",     // 实测极速 ~67ms
        "162.159.131.182", // Hong Kong Anycast
        "162.159.153.1",
        "162.159.192.1",
        "141.101.90.1",
        "141.101.64.1",
        // 联通 / 东京 / 香港经典优质段 (104.16.*, 104.18.*, 104.20.*, 104.26.*)
        "104.20.23.208",   // Tokyo / Narita
        "104.18.0.1",
        "104.18.2.161",    // Hong Kong Anycast
        "104.16.160.1",    // Asia Anycast
        "104.26.0.1",
        // 全球泛播与高可用兜底候选段 (188.114.*, 172.67.*, 104.19.*, 198.41.*)
        "188.114.97.1",
        "172.67.75.1",
        "104.19.16.1",
        "198.41.214.1"
    )

    // 动态排序后的 IP 列表（延迟最低排在最前）
    private val activeIps = CopyOnWriteArrayList<String>(CANDIDATE_EDGE_IPS)
    private val isProbing = AtomicBoolean(false)
    private val probeScheduler = Executors.newSingleThreadScheduledExecutor { r ->
        Thread(r, "cf-dns-scheduler").apply { isDaemon = true }
    }

    private var lastProbeTime = 0L

    init {
        // App 启动时异步触发一次全网并发赛马测速
        triggerProbeIfNeeded(force = true)
    }

    override fun lookup(hostname: String): List<InetAddress> {
        val lowerHost = hostname.lowercase()
        val isCloudflareTunnel = lowerHost.endsWith(".jiuge.space") || lowerHost == "jiuge.space"

        if (!isCloudflareTunnel) {
            return Dns.SYSTEM.lookup(hostname)
        }

        // 异步检查并触发轻量级探测以动态维持最优列表
        triggerProbeIfNeeded(force = false)

        val results = ArrayList<InetAddress>()

        // 1. 优先注入动态赛马优选出来的极速 Anycast IP
        for (ip in activeIps) {
            try {
                results.add(InetAddress.getByName(ip))
            } catch (e: Exception) {
                Log.w(TAG, "Failed to parse optimized IP $ip: ${e.message}")
            }
        }

        // 2. 将系统 DNS 解析结果追加在末尾作为终极兜底
        try {
            val systemIps = Dns.SYSTEM.lookup(hostname)
            for (sysIp in systemIps) {
                if (!results.contains(sysIp)) {
                    results.add(sysIp)
                }
            }
        } catch (e: Exception) {
            Log.w(TAG, "System DNS lookup failed for $hostname: ${e.message}")
        }

        return if (results.isNotEmpty()) results else Dns.SYSTEM.lookup(hostname)
    }

    fun triggerProbeIfNeeded(force: Boolean = false) {
        val now = System.currentTimeMillis()
        if (!force && now - lastProbeTime < 180_000L) { // 3分钟内不重复探测
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

    /**
     * 高并发多线程 TCP 443 握手赛马测速，秒级锁定当前网络真实延迟最低的边缘节点
     */
    private fun probeFastestIpsConcurrently() {
        val pool = Executors.newFixedThreadPool(8)
        val scoredList = Collections.synchronizedList(mutableListOf<Pair<String, Long>>())
        val latch = CountDownLatch(CANDIDATE_EDGE_IPS.size)

        for (ip in CANDIDATE_EDGE_IPS) {
            pool.submit {
                try {
                    val rtt = tcpPing(ip, 443, 800)
                    if (rtt in 0..1500) {
                        scoredList.add(ip to rtt)
                    }
                } finally {
                    latch.countDown()
                }
            }
        }

        try {
            latch.await(1000, TimeUnit.MILLISECONDS)
        } catch (_: Exception) {}
        pool.shutdownNow()

        if (scoredList.isNotEmpty()) {
            val sorted = ArrayList(scoredList)
            sorted.sortBy { it.second }
            val sortedIps = sorted.map { it.first }
            Log.d(TAG, "Parallel race completed. Winner: ${sortedIps.first()} (RTT: ${sorted.first().second}ms)")
            activeIps.clear()
            activeIps.addAll(sortedIps)
            // 补充未在本次响应中的其余候选 IP 位于末尾
            for (ip in CANDIDATE_EDGE_IPS) {
                if (!activeIps.contains(ip)) {
                    activeIps.add(ip)
                }
            }
        }
    }

    private fun tcpPing(ip: String, port: Int, timeoutMs: Int): Long {
        val start = System.nanoTime()
        return try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress(ip, port), timeoutMs)
            }
            TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start)
        } catch (_: Exception) {
            -1L
        }
    }
}
