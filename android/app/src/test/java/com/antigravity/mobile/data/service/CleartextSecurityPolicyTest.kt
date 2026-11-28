package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.util.L10n
import okhttp3.Call
import okhttp3.Connection
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.util.concurrent.TimeUnit

class CleartextSecurityPolicyTest {

    private class FakeChain(private val req: Request) : Interceptor.Chain {
        override fun request(): Request = req
        override fun proceed(request: Request): Response = Response.Builder()
            .request(request)
            .protocol(Protocol.HTTP_1_1)
            .code(200)
            .message("OK")
            .body("{}".toResponseBody("application/json".toMediaType()))
            .build()
        override fun connection(): Connection? = null
        override fun call(): Call = throw UnsupportedOperationException()
        override fun connectTimeoutMillis(): Int = 1000
        override fun withConnectTimeout(timeout: Int, unit: TimeUnit): Interceptor.Chain = this
        override fun readTimeoutMillis(): Int = 1000
        override fun withReadTimeout(timeout: Int, unit: TimeUnit): Interceptor.Chain = this
        override fun writeTimeoutMillis(): Int = 1000
        override fun withWriteTimeout(timeout: Int, unit: TimeUnit): Interceptor.Chain = this
    }

    private val interceptor = LanCleartextSecurityInterceptor()

    @Test
    fun interceptorAllowsLanTailscaleAndPrivateIpv6Cleartext() {
        val allowed = listOf(
            "http://192.168.1.50:58900/api/v1/auth/pair",
            "http://10.0.0.8:58900/healthz",
            "http://172.16.0.1:58900/healthz",
            "http://172.31.255.1:58900/healthz",
            "http://127.0.0.1:58900/healthz",
            "http://localhost:58900/healthz",
            "http://mac-studio.local:58900/healthz",
            "http://100.64.0.1:58900/api/v1/auth/pair",
            "http://100.127.255.254:58900/api/v1/auth/pair",
            "http://my-mac.tail1234.ts.net:58900/api/v1/auth/pair",
            "http://[fd7a:115c:a1e0::1]:58900/api/v1/auth/pair",
            "http://[fd00::10]:58900/healthz",
            "http://[fe80::1]:58900/healthz",
            "https://r-a1b2c3d4.hk1.multigravity.app/api/v1/e2ee/pair",
            "https://mgy.example.com/api/v1/auth/pair"
        )
        for (url in allowed) {
            val resp = interceptor.intercept(FakeChain(Request.Builder().url(url).build()))
            assertEquals("Expected 200 for $url", 200, resp.code)
        }
    }

    @Test
    fun interceptorRejectsPublicCleartextWithLocalizedGuidance() {
        val rejected = listOf(
            "http://mgy.example.com:58900/api/v1/auth/pair" to "mgy.example.com",
            "http://r-a1b2c3d4.hk1.multigravity.app:58900/api/v1/auth/pair" to "r-a1b2c3d4.hk1.multigravity.app",
            "http://evilts.net:58900/healthz" to "evilts.net",
            "http://100.63.0.1:58900/healthz" to "100.63.0.1",
            "http://100.128.0.1:58900/healthz" to "100.128.0.1",
            "http://172.32.0.1:58900/healthz" to "172.32.0.1",
            "http://8.8.8.8:58900/healthz" to "8.8.8.8",
            "http://[2408:8207::1]:58900/healthz" to "2408:8207::1"
        )
        try {
            L10n.setLanguagePreference("zh")
            for ((url, expectedHost) in rejected) {
                try {
                    interceptor.intercept(FakeChain(Request.Builder().url(url).build()))
                    fail("Expected CleartextPublicHostException for $url")
                } catch (e: CleartextPublicHostException) {
                    assertEquals(expectedHost, e.host)
                    assertTrue(e.message.orEmpty().contains("HTTPS / SSL"))
                }
            }

            L10n.setLanguagePreference("en")
            try {
                interceptor.intercept(FakeChain(Request.Builder().url("http://mgy.example.com:58900/healthz").build()))
                fail("Expected CleartextPublicHostException")
            } catch (e: CleartextPublicHostException) {
                assertTrue(e.message.orEmpty().contains("Enable HTTPS / SSL"))
            }

            L10n.setLanguagePreference("ja")
            try {
                interceptor.intercept(FakeChain(Request.Builder().url("http://mgy.example.com:58900/healthz").build()))
                fail("Expected CleartextPublicHostException")
            } catch (e: CleartextPublicHostException) {
                assertTrue(e.message.orEmpty().contains("HTTPS / SSL"))
            }
        } finally {
            L10n.setLanguagePreference("system")
        }
    }

    @Test
    fun tailscaleIpv6IsClassifiedAsTailscaleNotLanOnly() {
        assertTrue(ConnectionManager.isTailscaleHost("fd7a:115c:a1e0::1"))
        assertFalse(ConnectionManager.isLanHost("fd7a:115c:a1e0::1"))
        assertTrue(ConnectionManager.isPrivateOrTailscaleHost("fd7a:115c:a1e0::1"))

        assertTrue(ConnectionManager.isLanHost("fd00::1234"))
        assertFalse(ConnectionManager.isTailscaleHost("fd00::1234"))
        assertTrue(ConnectionManager.isPrivateOrTailscaleHost("fd00::1234"))

        assertFalse(ConnectionManager.isPrivateOrTailscaleHost("2408:8207::1"))
        assertFalse(ConnectionManager.isPrivateOrTailscaleHost("fc00:evil.com"))
    }

    @Test
    fun normalizeUrlUsesHttpForPrivateAndTailscaleAndHttpsForPublic() {
        assertEquals("http://192.168.1.50:58900", PreferencesManager.normalizeUrl("192.168.1.50:58900"))
        assertEquals("http://100.64.12.34:58900", PreferencesManager.normalizeUrl("100.64.12.34:58900"))
        assertEquals("http://mac.tail1234.ts.net:58900", PreferencesManager.normalizeUrl("mac.tail1234.ts.net:58900"))
        assertEquals("http://[fd7a:115c:a1e0::1]", PreferencesManager.normalizeUrl("fd7a:115c:a1e0::1"))
        assertEquals("https://mgy.example.com", PreferencesManager.normalizeUrl("mgy.example.com"))
        assertEquals("https://mgy.example.com:8443", PreferencesManager.normalizeUrl("mgy.example.com:8443/"))
        assertEquals("http://mgy.example.com:58900", PreferencesManager.normalizeUrl("http://mgy.example.com:58900/"))
        assertNull(PreferencesManager.normalizeUrl("javascript://alert(1)"))
    }
}
