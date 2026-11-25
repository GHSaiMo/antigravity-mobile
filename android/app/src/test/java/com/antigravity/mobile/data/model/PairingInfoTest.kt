package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
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
}
