package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.PairingInfo
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DualDomainSupportTest {

    @Test
    fun cloudflareOptimizedDns_matchesOnlyApexAndThreeLabelProxiedHosts() {
        // Proxied Cloudflare Orange-Cloud hosts (apex & 3-label) on both domains
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("multigravity.app"))
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("dispatcher.multigravity.app"))
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("relay.multigravity.app"))
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("jiuge.space"))
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("dispatcher.jiuge.space"))
        assertTrue(CloudflareOptimizedDns.isCloudflareTunnelHost("r-a1b2c3d4.jiuge.space"))

        // Regional 4-label Gray-Cloud VPS hosts MUST bypass Cloudflare Anycast and use direct DNS
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("r-a1b2c3d4.jp1.multigravity.app"))
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("ctl.hk1.multigravity.app"))
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("r-a1b2c3d4.jp1.jiuge.space"))
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("ctl.hk1.jiuge.space"))

        // Unrelated domains must not match
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("evilmultigravity.app"))
        assertFalse(CloudflareOptimizedDns.isCloudflareTunnelHost("example.com"))
    }

    @Test
    fun relayPoolResolver_acceptsBothOfficialDomainsAndExtractsSid() {
        assertTrue(RelayPoolResolver.isOfficialOrPoolHost("https://r-a1b2c3d4.jp1.multigravity.app"))
        assertTrue(RelayPoolResolver.isOfficialOrPoolHost("r-a1b2c3d4.hk1.multigravity.app"))
        assertTrue(RelayPoolResolver.isOfficialOrPoolHost("multigravity.app"))
        assertTrue(RelayPoolResolver.isOfficialOrPoolHost("https://r-a1b2c3d4.jp1.jiuge.space"))
        assertTrue(RelayPoolResolver.isOfficialOrPoolHost("jiuge.space"))
        assertFalse(RelayPoolResolver.isOfficialOrPoolHost("https://r-a1b2c3d4.byo-relay.example.com"))

        assertEquals("a1b2c3d4", RelayPoolResolver.extractSid("https://r-a1b2c3d4.jp1.multigravity.app"))
        assertEquals("a1b2c3d4", RelayPoolResolver.extractSid("https://r-a1b2c3d4.jp1.jiuge.space"))
    }

    @Test
    fun connectionManager_trustsAndRecognizesBothOfficialRelayDomains() {
        assertTrue(ConnectionManager.isRelayHost("r-a1b2c3d4.jp1.multigravity.app"))
        assertTrue(ConnectionManager.isRelayHost("r-a1b2c3d4.jp1.jiuge.space"))
        assertFalse(ConnectionManager.isRelayHost("evil.example.com"))

        val info = PairingInfo(
            host = "r-a1b2c3d4.jp1.jiuge.space",
            port = 443,
            code = "0123456789abcdef",
            ssl = true
        )
        assertTrue(
            ConnectionManager.isTrustedEndpoint(
                "https://r-a1b2c3d4.hk1.multigravity.app",
                info,
                "https://r-a1b2c3d4.jp1.jiuge.space"
            )
        )
        assertFalse(
            ConnectionManager.isTrustedEndpoint(
                "http://r-a1b2c3d4.hk1.multigravity.app",
                info,
                "https://r-a1b2c3d4.jp1.jiuge.space"
            )
        )
        assertTrue(
            ConnectionManager.isValidAdvertisedCloudUrl(
                "https://r-a1b2c3d4.hk1.multigravity.app",
                prefs = null,
                trustedByE2ee = false
            )
        )
        assertFalse(
            ConnectionManager.isValidAdvertisedCloudUrl(
                "https://attacker.example.com",
                prefs = null,
                trustedByE2ee = false
            )
        )
    }

    @Test
    fun relayPoolResolver_classifiesRelayPairingFailuresAccurately() {
        assertEquals(true to true, RelayPoolResolver.classifyRelayPairingFailure(503, """{"error":"gateway_offline"}"""))
        assertEquals(true to false, RelayPoolResolver.classifyRelayPairingFailure(503, """{"error":"relay_capacity_full"}"""))
        assertEquals(false to false, RelayPoolResolver.classifyRelayPairingFailure(503, """{"error":"upstream_not_connected"}"""))
        assertEquals(true to false, RelayPoolResolver.classifyRelayPairingFailure(502, """{"error":"gateway_unreachable"}"""))
        assertEquals(true to false, RelayPoolResolver.classifyRelayPairingFailure(429, """{"error":"tunnel_busy"}"""))
        assertEquals(false to false, RelayPoolResolver.classifyRelayPairingFailure(429, """{"error":"too many requests"}"""))
        assertEquals(false to false, RelayPoolResolver.classifyRelayPairingFailure(401, """{"error":"invalid_pairing_code"}"""))
        assertEquals(false to false, RelayPoolResolver.classifyRelayPairingFailure(426, """{"error":"e2ee_required"}"""))
    }

    @Test
    fun relayPoolResolver_discoverPeerRelayForPairing_ignoresByoRelayDomains() = runBlocking {
        var probeCalled = false
        val result = RelayPoolResolver.discoverPeerRelayForPairing(
            failedUrl = "https://r-a1b2c3d4.byo-relay.example.com",
            httpClient = OkHttpClient(),
            probeEndpoint = {
                probeCalled = true
                EndpointHealthStatus(it, isReachable = true, latencyMs = 10L)
            }
        )
        assertNull(result)
        assertFalse(probeCalled)
    }
}
