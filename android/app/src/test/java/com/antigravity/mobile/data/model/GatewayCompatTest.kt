package com.antigravity.mobile.data.model

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class GatewayCompatTest {
    @After
    fun tearDown() = GatewayCompatStore.resetForTest()

    @Test
    fun parsesCompatFromStatusPayload() {
        val body = """{"status":"connected","os":"darwin","upstream":{"pid":1},
            "compat":{"version":"2.40.1","checked":true,"coreOk":true,"unavailable":["export","slash"],"missingRpcs":["ConvertTrajectoryToMarkdown","GetSlashCommands"]}}"""
        val c = GatewayCompat.fromStatusJson(body)
        assertEquals("2.40.1", c.version)
        assertTrue(c.checked)
        assertFalse(c.isAvailable(GatewayFeature.EXPORT))
        assertFalse(c.isAvailable(GatewayFeature.SLASH))
        assertTrue(c.isAvailable(GatewayFeature.SEARCH))
        assertFalse(c.isIncompatible)
        assertEquals(listOf("GetSlashCommands"), c.missingRpcs.drop(1))
    }

    @Test
    fun oldGatewayWithoutCompatFieldIsPermissive() {
        val c = GatewayCompat.fromStatusJson("""{"status":"connected"}""")
        assertFalse(c.checked)
        assertTrue(c.isAvailable(GatewayFeature.SEARCH))
        assertFalse(c.isIncompatible)
    }

    @Test
    fun garbageIsPermissive() {
        for (body in listOf("", "not json", "[]", """{"compat":"oops"}""", """{"compat":null}""")) {
            val c = GatewayCompat.fromStatusJson(body)
            assertTrue("body=$body", c.isAvailable(GatewayFeature.REVERT))
            assertFalse("body=$body", c.isIncompatible)
        }
    }

    @Test
    fun uncheckedNeverHidesAnything() {
        // 网关读不到二进制：即使字段里带了 unavailable 也不应隐藏（防御性：checked=false 的结果本不该带它）
        val c = GatewayCompat(checked = false, unavailable = listOf("search"))
        assertTrue(c.isAvailable(GatewayFeature.SEARCH))
    }

    @Test
    fun coreMissingShowsBannerButOptionalDoesNot() {
        val optional = GatewayCompat(checked = true, coreOk = true, unavailable = listOf("search"))
        assertFalse(optional.isIncompatible)
        val core = GatewayCompat(version = "9.0.0", checked = true, coreOk = false, unavailable = listOf("core"))
        assertTrue(core.isIncompatible)
        assertTrue(core.bannerText.contains("9.0.0"))
        assertTrue(core.bannerText.contains("升级网关"))
        // 版本未知时文案也要通顺
        assertFalse(GatewayCompat(checked = true, coreOk = false).bannerText.contains("版本（"))
    }

    @Test
    fun storeSharesLatestResult() {
        assertTrue(GatewayCompatStore.isAvailable(GatewayFeature.CHANGES))
        GatewayCompatStore.update(GatewayCompat(checked = true, unavailable = listOf("changes")))
        assertFalse(GatewayCompatStore.isAvailable(GatewayFeature.CHANGES))
        assertTrue(GatewayCompatStore.isAvailable(GatewayFeature.SEARCH))
        assertNull(GatewayCompatStore.state.value.version)
    }
}
