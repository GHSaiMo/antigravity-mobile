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
        assertEquals(
            "f67cc2d455e1a24aad0396a8720459eadc2269062bcbb8a58cb060c1c1ee8125",
            release.expectedSha256
        )
        assertTrue(release.usedMirrorForCheck)
    }

    @Test
    fun `parseGitHubRelease parses latest release json and ignores prerelease`() {
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
            "https://github.com/GHSaiMo/antigravity-mobile/releases/download/v1.2.7/checksums.txt",
            checksumsUrl
        )

        val zhNotes = AppUpdateParser.formatReleaseNotes(release.releaseNotes, preferChinese = true)
        assertTrue(zhNotes.contains("新增应用内检查更新"))
        assertFalse(zhNotes.contains("Added in-app updater"))

        val enNotes = AppUpdateParser.formatReleaseNotes(release.releaseNotes, preferChinese = false)
        assertTrue(enNotes.contains("Added in-app updater"))
        assertFalse(enNotes.contains("新增应用内检查更新"))

        val prereleaseJson = json.replace("\"prerelease\": false", "\"prerelease\": true")
        assertNull(AppUpdateParser.parseGitHubRelease(prereleaseJson))
    }
}
