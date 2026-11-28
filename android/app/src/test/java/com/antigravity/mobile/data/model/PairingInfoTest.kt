package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingInfoTest {

    @Test
    fun extractAndParseStandardPairingUriWithE2eeKey() {
        val raw = "agy://pair?code=0123456789abcdef&host=r-541ba9e5&lan=192.168.1.50&platform=darwin&pk=BAbcDEF1234567890_-"
        val info = PairingInfo.parseFromUri(raw)
        assertNotNull(info)
        info!!
        assertEquals("r-541ba9e5.jiuge.space", info.host)
        assertEquals(443, info.port)
        assertEquals("0123456789abcdef", info.code)
        assertTrue(info.ssl)
        assertEquals("192.168.1.50", info.lanHost)
        assertEquals("darwin", info.platform)
        assertEquals("BAbcDEF1234567890_-", info.e2eePubKey)
    }

    @Test
    fun extractPairingUriFromTerminalPrefixAndWrappedLines() {
        val raw = """
              🔗 配对 URI: agy://pair?code=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
            bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&host=r-99887766&lan=10.0.0.8&platform=linux&pk=PubKeyPartOne
            PubKeyPartTwo12345

              5分钟有效，请尽快扫码
        """.trimIndent()

        val extracted = PairingInfo.extractPairingUri(raw)
        assertEquals(
            "agy://pair?code=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&host=r-99887766&lan=10.0.0.8&platform=linux&pk=PubKeyPartOnePubKeyPartTwo12345",
            extracted
        )

        val info = PairingInfo.parseFromUri(raw)
        assertNotNull(info)
        info!!
        assertEquals("r-99887766.jiuge.space", info.host)
        assertEquals("10.0.0.8", info.lanHost)
        assertEquals("PubKeyPartOnePubKeyPartTwo12345", info.e2eePubKey)
    }

    @Test
    fun extractPairingUriDoesNotAbsorbTrailingProseOnNextLine() {
        val raw = "agy://pair?code=abc123&host=r-112233&pk=MyKey123\n5分钟有效\nvalid for 5 mins"
        val extracted = PairingInfo.extractPairingUri(raw)
        assertEquals("agy://pair?code=abc123&host=r-112233&pk=MyKey123", extracted)
    }

    @Test
    fun extractPairingUriSupportsMultigravitySchemeAndRejectsNonPairingText() {
        val raw = "点击连接 <multigravity://pair?code=ff00&host=192.168.1.10&port=58900&ssl=0>"
        val info = PairingInfo.parseFromUri(raw)
        assertNotNull(info)
        assertEquals("192.168.1.10", info!!.host)
        assertEquals(58900, info.port)
        assertEquals("ff00", info.code)

        assertNull(PairingInfo.extractPairingUri("https://example.com/pair?code=123"))
        assertNull(PairingInfo.parseFromUri("just some random clipboard text"))
    }

    @Test
    fun parseFromUriInfersCleartextOnlyForPrivateAndTailscaleOnNon443Port() {
        val tailscaleIp = PairingInfo.parseFromUri("agy://pair?code=c1&host=100.64.12.34&port=58900")!!
        assertFalse(tailscaleIp.ssl)
        assertEquals("http://100.64.12.34:58900", tailscaleIp.serverBaseUrl)

        val tailscaleDns = PairingInfo.parseFromUri("agy://pair?code=c2&host=mac.tail1234.ts.net&port=58900")!!
        assertFalse(tailscaleDns.ssl)
        assertEquals("http://mac.tail1234.ts.net:58900", tailscaleDns.serverBaseUrl)

        val localhost = PairingInfo.parseFromUri("agy://pair?code=c3&host=localhost&port=58900")!!
        assertFalse(localhost.ssl)

        val mdns = PairingInfo.parseFromUri("agy://pair?code=c4&host=studio.local&port=58900")!!
        assertFalse(mdns.ssl)

        val rfc1918B = PairingInfo.parseFromUri("agy://pair?code=c5&host=172.16.5.9&port=58900")!!
        assertFalse(rfc1918B.ssl)

        val public172 = PairingInfo.parseFromUri("agy://pair?code=c6&host=172.1.2.3&port=58900")!!
        assertTrue(public172.ssl)

        val publicDomain = PairingInfo.parseFromUri("agy://pair?code=c7&host=mgy.example.com&port=58900")!!
        assertTrue(publicDomain.ssl)
    }

    @Test
    fun fromManualInputNormalizesSchemesPortsAndManagedRelays() {
        val httpsUrl = PairingInfo.fromManualInput("https://mgy.example.com/chat", 58900, "abc", false)
        assertEquals("mgy.example.com", httpsUrl.host)
        assertEquals(443, httpsUrl.port)
        assertTrue(httpsUrl.ssl)
        assertEquals("https://mgy.example.com", httpsUrl.serverBaseUrl)

        val httpsCustomPort = PairingInfo.fromManualInput("https://mgy.example.com:8443/path", 58900, "abc", false)
        assertEquals("mgy.example.com", httpsCustomPort.host)
        assertEquals(8443, httpsCustomPort.port)
        assertTrue(httpsCustomPort.ssl)
        assertEquals("https://mgy.example.com:8443", httpsCustomPort.serverBaseUrl)

        val relayNoSsl = PairingInfo.fromManualInput("r-a1b2c3d4.hk1.multigravity.app", 58900, "abc", false)
        assertEquals("r-a1b2c3d4.hk1.multigravity.app", relayNoSsl.host)
        assertEquals(443, relayNoSsl.port)
        assertTrue(relayNoSsl.ssl)

        val compressedSubdomain = PairingInfo.fromManualInput("825a5a50", 58900, "abc", false)
        assertEquals("825a5a50.jiuge.space", compressedSubdomain.host)
        assertEquals(443, compressedSubdomain.port)
        assertTrue(compressedSubdomain.ssl)

        val port443Public = PairingInfo.fromManualInput("mgy.example.com", 443, "abc", false)
        assertTrue(port443Public.ssl)

        val tailscaleHostPort = PairingInfo.fromManualInput("100.64.10.2:58900", 58900, "abc", false)
        assertEquals("100.64.10.2", tailscaleHostPort.host)
        assertEquals(58900, tailscaleHostPort.port)
        assertFalse(tailscaleHostPort.ssl)
        assertEquals("http://100.64.10.2:58900", tailscaleHostPort.serverBaseUrl)

        val tailscaleIpv6 = PairingInfo.fromManualInput("[fd7a:115c:a1e0::1]:58900", 58900, "abc", false)
        assertEquals("fd7a:115c:a1e0::1", tailscaleIpv6.host)
        assertEquals(58900, tailscaleIpv6.port)
        assertFalse(tailscaleIpv6.ssl)
        assertEquals("http://[fd7a:115c:a1e0::1]:58900", tailscaleIpv6.serverBaseUrl)

        val publicHttpCustomPort = PairingInfo.fromManualInput("mgy.example.com", 58900, "abc", false)
        assertEquals("mgy.example.com", publicHttpCustomPort.host)
        assertEquals(58900, publicHttpCustomPort.port)
        assertFalse(publicHttpCustomPort.ssl)
    }
}
