package com.antigravity.mobile.data.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AppUpdateParserTest {

    @Test
    fun `isNewerVersion compares semantic versions accurately`() {
        assertTrue(AppUpdateParser.isNewerVersion("v1.2.7", "1.2.6"))
        assertTrue(AppUpdateParser.isNewerVersion("1.2.10", "1.2.9"))
        assertTrue(AppUpdateParser.isNewerVersion("1.3.0", "1.2.99"))
        assertTrue(AppUpdateParser.isNewerVersion("2.0", "1.9.9"))
        assertTrue(AppUpdateParser.isNewerVersion("1.2.7", "1.2.7-beta.1"))

        assertFalse(AppUpdateParser.isNewerVersion("1.2.6", "1.2.6"))
        assertFalse(AppUpdateParser.isNewerVersion("v1.2.6", "1.2.6"))
        assertFalse(AppUpdateParser.isNewerVersion("1.2.5", "1.2.6"))
        assertFalse(AppUpdateParser.isNewerVersion("1.2.7-beta.1", "1.2.7"))
        assertFalse(AppUpdateParser.isNewerVersion("", "1.2.6"))
        assertFalse(AppUpdateParser.isNewerVersion("invalid", "1.2.6"))
    }

    @Test
    fun `parseApkChecksum extracts apk filename and lowercase sha256`() {
        val sample = """
            F67CC2D455E1A24AAD0396A8720459EADC2269062BCBB8A58CB060C1C1EE8125  Multigravity-v1.2.6.apk
            5aba1bfe2e236edc399bfd9edac03a973edc9765f2db465bf7cc2861f75b6b42  multigravity-darwin-amd64.tar.gz
        """.trimIndent()

        val parsed = AppUpdateParser.parseApkChecksum(sample)
        assertNotNull(parsed)
        assertEquals("Multigravity-v1.2.6.apk", parsed!!.first)
        assertEquals(
            "f67cc2d455e1a24aad0396a8720459eadc2269062bcbb8a58cb060c1c1ee8125",
            parsed.second
        )
        assertEquals("1.2.6", AppUpdateParser.extractVersionFromApkFileName(parsed.first))
    }

    @Test
    fun `buildReleaseFromChecksums builds complete mirror fallback release`() {
        val checksums = """
            f67cc2d455e1a24aad0396a8720459eadc2269062bcbb8a58cb060c1c1ee8125  Multigravity-v1.2.7.apk
        """.trimIndent()

        val release = AppUpdateParser.buildReleaseFromChecksums(checksums, "## 更新日志\n- 新增应用内更新")
        assertNotNull(release)
        assertEquals("v1.2.7", release!!.tagName)
        assertEquals("1.2.7", release.versionName)
        assertEquals("Multigravity-v1.2.7.apk", release.apkFileName)
        assertEquals(
            "https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk",
            release.directDownloadUrl
        )
        assertEquals(
            "https://ghfast.top/https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk",
            release.mirrorDownloadUrl
        )
        assertTrue(release.mirrorDownloadUrls.contains("https://edgeone.gh-proxy.com/https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk"))
        assertTrue(release.mirrorDownloadUrls.contains("https://gh-proxy.org/https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk"))
        assertTrue(release.mirrorDownloadUrls.contains("https://ghfast.top/https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk"))
        assertTrue(release.mirrorDownloadUrls.contains("https://ghproxy.net/https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk"))
        assertEquals(
            "f67cc2d455e1a24aad0396a8720459eadc2269062bcbb8a58cb060c1c1ee8125",
            release.expectedSha256
        )
        assertTrue(release.usedMirrorForCheck)
    }

    @Test
    fun `parseGitHubRelease parses latest release json, asset digest, and ignores prerelease`() {
        val json = """
            {
              "tag_name": "v1.2.7",
              "html_url": "https://github.com/GHSaiMo/antigravity-mobile/releases/tag/v1.2.7",
              "draft": false,
              "prerelease": false,
              "body": "## 简体中文\n- 新增应用内检查更新\n\n---\n\n## English\n- Added in-app updater",
              "assets": [
                {
                  "name": "checksums.txt",
                  "size": 512,
                  "browser_download_url": "https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/checksums.txt"
                },
                {
                  "name": "Multigravity-v1.2.7.apk",
                  "size": 14680064,
                  "digest": "sha256:EBC7975D168A5927B05392F8970DD54D46334CC8D4CBAFC6D4887EC71CD479DF",
                  "browser_download_url": "https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/Multigravity-v1.2.7.apk"
                }
              ]
            }
        """.trimIndent()

        val result = AppUpdateParser.parseGitHubRelease(json)
        assertNotNull(result)
        val (release, checksumsUrl) = result!!
        assertEquals("1.2.7", release.versionName)
        assertEquals(14680064L, release.sizeBytes)
        assertFalse(release.usedMirrorForCheck)
        assertEquals(
            "ebc7975d168a5927b05392f8970dd54d46334cc8d4cbafc6d4887ec71cd479df",
            release.expectedSha256
        )
        assertEquals(
            "https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/checksums.txt",
            checksumsUrl
        )
        assertTrue(release.mirrorDownloadUrls.size >= 4)

        val zhNotes = AppUpdateParser.formatReleaseNotes(release.releaseNotes, preferChinese = true)
        assertTrue(zhNotes.contains("新增应用内检查更新"))
        assertFalse(zhNotes.contains("Added in-app updater"))

        val enNotes = AppUpdateParser.formatReleaseNotes(release.releaseNotes, preferChinese = false)
        assertTrue(enNotes.contains("Added in-app updater"))
        assertFalse(enNotes.contains("新增应用内检查更新"))

        // Japanese user with bilingual (ZH+EN) notes falls back to English, never Chinese
        val jaFallbackNotes = AppUpdateParser.formatReleaseNotes(
            release.releaseNotes,
            preferChinese = false,
            preferJapanese = true
        )
        assertTrue(jaFallbackNotes.contains("Added in-app updater"))
        assertFalse(jaFallbackNotes.contains("新增应用内检查更新"))

        // Trilingual notes select Japanese for JA, English (without JA) for EN, Chinese for ZH
        val trilingual = "## 简体中文\n- 中文说明\n\n---\n\n## English\n- English notes\n\n---\n\n## 日本語\n- 日本語ノート"
        assertTrue(AppUpdateParser.formatReleaseNotes(trilingual, preferChinese = false, preferJapanese = true).contains("日本語ノート"))
        val enOnly = AppUpdateParser.formatReleaseNotes(trilingual, preferChinese = false, preferJapanese = false)
        assertTrue(enOnly.contains("English notes"))
        assertFalse(enOnly.contains("日本語ノート"))
        assertFalse(enOnly.contains("中文说明"))

        val prereleaseJson = json.replace("\"prerelease\": false", "\"prerelease\": true")
        assertNull(AppUpdateParser.parseGitHubRelease(prereleaseJson))
    }

    @Test
    fun `formatReleaseNotes strips trilingual preamble and shows only active language`() {
        val realStyleNotes = """
            # 🚀 Multigravity v1.2.7

            > 🎉 **Multigravity iOS 版现已正式上架 [Apple App Store](https://apps.apple.com/app/id6835508934)！**
            >
            > 🎉 **Multigravity for iOS is officially available on the [App Store](https://apps.apple.com/app/id6835508934)!**
            >
            > 🎉 **Multigravity iOS 版は Apple App Store で正式配信中！**

            ---

            ## 🇨🇳 中文更新说明

            > ⚠️ **Android 升级提示**：自本版本起启用独立私有签名。
            > **存量 `v1.2.6` 用户**：请先卸载旧版再安装新版。

            ### ✨ 核心亮点与新特性

            - 🌍 **全端三语深度打磨**：
              - 支持自动协商语言。
            - 📦 **全端版本号对齐**：Android `1.2.7`。

            ---

            ## 🇺🇸 English Release Notes

            > ⚠️ **Android Upgrade Notice**: New private signing key.

            ### ✨ Highlights & New Features

            - 🌍 **Full Trilingual Polish**:
              - Automatic language negotiation.

            ---

            ## 🇯🇵 日本語リリースノート

            > ⚠️ **Android アップグレードのご案内**：新しい専用署名鍵を使用。

            - 🌍 **全クライアント 3 言語対応**：自動言語切替を追加。
        """.trimIndent()

        val zh = AppUpdateParser.formatReleaseNotes(realStyleNotes, preferChinese = true, preferJapanese = false)
        assertTrue(zh.contains("Android 升级提示"))
        assertTrue(zh.contains("全端三语深度打磨"))
        assertTrue(zh.contains("  · 支持自动协商语言。"))
        assertTrue(zh.contains("全端版本号对齐"))
        assertFalse(zh.contains("中文更新说明"))
        assertFalse(zh.contains("officially available"))
        assertFalse(zh.contains("正式配信中"))
        assertFalse(zh.contains("Apple App Store"))
        assertFalse(zh.contains("English Release Notes"))
        assertFalse(zh.contains("Full Trilingual Polish"))
        assertFalse(zh.contains("日本語リリースノート"))
        assertFalse(zh.contains("> "))

        val en = AppUpdateParser.formatReleaseNotes(realStyleNotes, preferChinese = false, preferJapanese = false)
        assertTrue(en.contains("Android Upgrade Notice"))
        assertTrue(en.contains("Full Trilingual Polish"))
        assertTrue(en.contains("  · Automatic language negotiation."))
        assertFalse(en.contains("English Release Notes"))
        assertFalse(en.contains("中文更新说明"))
        assertFalse(en.contains("全端三语深度打磨"))
        assertFalse(en.contains("日本語リリースノート"))
        assertFalse(en.contains("正式配信中"))

        val ja = AppUpdateParser.formatReleaseNotes(realStyleNotes, preferChinese = false, preferJapanese = true)
        assertTrue(ja.contains("Android アップグレードのご案内"))
        assertTrue(ja.contains("全クライアント 3 言語対応"))
        assertFalse(ja.contains("日本語リリースノート"))
        assertFalse(ja.contains("全端三语深度打磨"))
        assertFalse(ja.contains("Full Trilingual Polish"))
        assertFalse(ja.contains("officially available"))
    }

    @Test
    fun `formatReleaseNotes handles legacy preamble without chinese heading and strips markdown links`() {
        val legacyNotes = """
            - 修复连接稳定性，详情见 [官网](https://multigravity.app)

            ---

            ## English
            - Improved connection stability, see [Website](https://multigravity.app)
        """.trimIndent()

        val zh = AppUpdateParser.formatReleaseNotes(legacyNotes, preferChinese = true)
        assertEquals("• 修复连接稳定性，详情见 官网", zh)

        val en = AppUpdateParser.formatReleaseNotes(legacyNotes, preferChinese = false)
        assertEquals("• Improved connection stability, see Website", en)
    }

    @Test
    fun `parseGitHubAssetDigest validates sha256 format`() {
        assertEquals(
            "ebc7975d168a5927b05392f8970dd54d46334cc8d4cbafc6d4887ec71cd479df",
            AppUpdateParser.parseGitHubAssetDigest("sha256:EBC7975D168A5927B05392F8970DD54D46334CC8D4CBAFC6D4887EC71CD479DF")
        )
        assertNull(AppUpdateParser.parseGitHubAssetDigest("md5:123456"))
        assertNull(AppUpdateParser.parseGitHubAssetDigest("sha256:short"))
        assertNull(AppUpdateParser.parseGitHubAssetDigest(null))
    }
}
