package com.antigravity.mobile.data.service

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * Metadata for an Android APK release fetched from GitHub Releases or mirror fallback.
 */
data class AppReleaseInfo(
    val tagName: String,
    val versionName: String,
    val apkFileName: String,
    val directDownloadUrl: String,
    val mirrorDownloadUrl: String,
    val expectedSha256: String? = null,
    val sizeBytes: Long = -1L,
    val releaseNotes: String = "",
    val releasePageUrl: String = "",
    val usedMirrorForCheck: Boolean = false
)

/**
 * State machine shared by the restrained Settings update row and optional badge indicator.
 */
sealed class AppUpdateState {
    data object Idle : AppUpdateState()
    data object Checking : AppUpdateState()
    data class UpToDate(
        val currentVersion: String,
        val checkedAtMs: Long
    ) : AppUpdateState()

    data class Available(
        val release: AppReleaseInfo
    ) : AppUpdateState()

    data class Downloading(
        val release: AppReleaseInfo,
        val downloadedBytes: Long,
        val totalBytes: Long,
        val usingMirror: Boolean
    ) : AppUpdateState()

    data class ReadyToInstall(
        val release: AppReleaseInfo,
        val apkFilePath: String,
        val signatureDiffers: Boolean = false
    ) : AppUpdateState()

    data class NeedsInstallPermission(
        val release: AppReleaseInfo,
        val apkFilePath: String,
        val signatureDiffers: Boolean = false
    ) : AppUpdateState()

    data class Error(
        val message: String,
        val release: AppReleaseInfo? = null
    ) : AppUpdateState()
}

@Serializable
internal data class GitHubReleasePayload(
    @SerialName("tag_name") val tagName: String = "",
    @SerialName("html_url") val htmlUrl: String = "",
    val body: String? = null,
    val draft: Boolean = false,
    val prerelease: Boolean = false,
    val assets: List<GitHubReleaseAssetPayload> = emptyList()
)

@Serializable
internal data class GitHubReleaseAssetPayload(
    val name: String = "",
    val size: Long = -1L,
    @SerialName("browser_download_url") val browserDownloadUrl: String = ""
)

/**
 * Pure-Kotlin helpers for SemVer comparison, GitHub Release JSON parsing,
 * checksums.txt parsing, and mirror URL generation.
 */
object AppUpdateParser {
    const val REPO_SLUG = "GHSaiMo/antigravity-mobile"
    const val GITHUB_LATEST_API_URL = "https://api.github.com/repos/$REPO_SLUG/releases/latest"
    const val MIRROR_PREFIX = "https://ghfast.top/"
    const val MIRROR_LATEST_CHECKSUMS_URL =
        "${MIRROR_PREFIX}https://github.com/$REPO_SLUG/releases/latest/download/checksums.txt"
    const val MIRROR_RELEASE_NOTES_URL =
        "${MIRROR_PREFIX}https://raw.githubusercontent.com/$REPO_SLUG/main/.github/release_notes.md"

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
    }

    private val checksumLineRegex = Regex("""^\s*([0-9a-fA-F]{64})\s+\*?(\S+)\s*$""")
    private val apkVersionRegex = Regex("""(?i)multigravity-v?(\d+(?:\.\d+)+[a-zA-Z0-9._-]*)\.apk$""")

    fun normalizeVersion(raw: String): String {
        return raw.trim().removePrefix("v").removePrefix("V").trim()
    }

    /**
     * Returns true iff [remote] is strictly newer than [current] under SemVer rules.
     * Examples: "v1.2.7" > "1.2.6", "1.2.10" > "1.2.9", "1.2.7" > "1.2.7-beta.1".
     */
    fun isNewerVersion(remote: String, current: String): Boolean {
        val rNorm = normalizeVersion(remote)
        val cNorm = normalizeVersion(current)
        if (rNorm.isBlank() || cNorm.isBlank() || rNorm.equals(cNorm, ignoreCase = true)) {
            return false
        }

        val (rCore, rPre) = splitPreRelease(rNorm)
        val (cCore, cPre) = splitPreRelease(cNorm)
        val rNums = parseCoreNumbers(rCore)
        val cNums = parseCoreNumbers(cCore)
        if (rNums.isEmpty() || cNums.isEmpty()) return false

        val maxLen = maxOf(rNums.size, cNums.size)
        for (i in 0 until maxLen) {
            val rv = rNums.getOrElse(i) { 0 }
            val cv = cNums.getOrElse(i) { 0 }
            if (rv != cv) return rv > cv
        }

        // Same numeric core: stable release is newer than a pre-release.
        if (rPre == null && cPre != null) return true
        if (rPre != null && cPre == null) return false
        if (rPre != null && cPre != null) return rPre > cPre
        return false
    }

    private fun splitPreRelease(version: String): Pair<String, String?> {
        val clean = version.substringBefore('+')
        val dashIdx = clean.indexOf('-')
        return if (dashIdx >= 0) {
            clean.substring(0, dashIdx) to clean.substring(dashIdx + 1)
        } else {
            clean to null
        }
    }

    private fun parseCoreNumbers(core: String): List<Int> {
        val parts = core.split('.')
        val nums = ArrayList<Int>(parts.size)
        for (part in parts) {
            val v = part.toIntOrNull() ?: return emptyList()
            nums.add(v)
        }
        return nums
    }

    fun buildMirrorUrl(githubUrl: String): String {
        val trimmed = githubUrl.trim()
        return if (trimmed.startsWith(MIRROR_PREFIX)) trimmed else "$MIRROR_PREFIX$trimmed"
    }

    /**
     * Parses `checksums.txt` and returns `(apkFileName, lowercaseSha256)` for the Android APK line.
     */
    fun parseApkChecksum(checksumsText: String): Pair<String, String>? {
        for (line in checksumsText.lineSequence()) {
            val match = checksumLineRegex.matchEntire(line) ?: continue
            val hash = match.groupValues[1].lowercase()
            val fileName = match.groupValues[2].substringAfterLast('/')
            if (fileName.endsWith(".apk", ignoreCase = true)) {
                return fileName to hash
            }
        }
        return null
    }

    fun extractVersionFromApkFileName(apkFileName: String): String? {
        val base = apkFileName.substringAfterLast('/')
        val match = apkVersionRegex.find(base) ?: return null
        return normalizeVersion(match.groupValues[1])
    }

    /**
     * Parses a GitHub `/releases/latest` JSON payload into an [AppReleaseInfo] + optional checksums asset URL.
     */
    fun parseGitHubRelease(
        jsonBody: String
    ): Pair<AppReleaseInfo, String?>? {
        val payload = runCatching {
            json.decodeFromString<GitHubReleasePayload>(jsonBody)
        }.getOrNull() ?: return null

        if (payload.draft || payload.prerelease) return null
        val apkAsset = payload.assets.firstOrNull { asset ->
            asset.name.endsWith(".apk", ignoreCase = true) &&
                asset.browserDownloadUrl.startsWith("https://")
        } ?: return null

        val tag = payload.tagName.trim().ifBlank {
            extractVersionFromApkFileName(apkAsset.name)?.let { "v$it" } ?: return null
        }
        val version = normalizeVersion(tag)
        if (version.isBlank()) return null

        val checksumsUrl = payload.assets.firstOrNull { asset ->
            asset.name.equals("checksums.txt", ignoreCase = true) &&
                asset.browserDownloadUrl.startsWith("https://")
        }?.browserDownloadUrl

        val release = AppReleaseInfo(
            tagName = tag,
            versionName = version,
            apkFileName = apkAsset.name,
            directDownloadUrl = apkAsset.browserDownloadUrl,
            mirrorDownloadUrl = buildMirrorUrl(apkAsset.browserDownloadUrl),
            expectedSha256 = null,
            sizeBytes = apkAsset.size,
            releaseNotes = payload.body.orEmpty().trim(),
            releasePageUrl = payload.htmlUrl.ifBlank {
                "https://github.com/$REPO_SLUG/releases/tag/$tag"
            },
            usedMirrorForCheck = false
        )
        return release to checksumsUrl
    }

    /**
     * Builds an [AppReleaseInfo] purely from `checksums.txt` (used when GitHub API is unreachable in Mainland China).
     */
    fun buildReleaseFromChecksums(
        checksumsText: String,
        releaseNotes: String = ""
    ): AppReleaseInfo? {
        val (apkFileName, sha256) = parseApkChecksum(checksumsText) ?: return null
        val version = extractVersionFromApkFileName(apkFileName) ?: return null
        val tag = "v$version"
        val directUrl = "https://github.com/$REPO_SLUG/releases/download/$tag/$apkFileName"
        return AppReleaseInfo(
            tagName = tag,
            versionName = version,
            apkFileName = apkFileName,
            directDownloadUrl = directUrl,
            mirrorDownloadUrl = buildMirrorUrl(directUrl),
            expectedSha256 = sha256,
            sizeBytes = -1L,
            releaseNotes = releaseNotes.trim(),
            releasePageUrl = "https://github.com/$REPO_SLUG/releases/tag/$tag",
            usedMirrorForCheck = true
        )
    }

    /**
     * Extracts a clean, concise mobile-friendly release note snippet.
     * When bilingual (Chinese + English) notes are separated by `---` or an English heading,
     * selects the language matching [preferChinese].
     */
    fun formatReleaseNotes(raw: String, preferChinese: Boolean, maxLines: Int = 12): String {
        val trimmed = raw.trim()
        if (trimmed.isBlank()) return ""

        val englishMarkerRegex = Regex("""(?m)^#{1,3}\s+.*(English|What's New|Highlights|Release Notes).*$""")
        val markerMatch = englishMarkerRegex.find(trimmed)
        val section = if (markerMatch != null && markerMatch.range.first > 20) {
            if (preferChinese) {
                trimmed.substring(0, markerMatch.range.first).trim()
            } else {
                trimmed.substring(markerMatch.range.first).trim()
            }
        } else {
            trimmed
        }

        val cleanedLines = section.lineSequence()
            .map { line ->
                line.trim()
                    .replace(Regex("""^#{1,6}\s+"""), "")
                    .replace("**", "")
                    .replace("`", "")
            }
            .filter { line ->
                line.isNotBlank() &&
                    line != "---" &&
                    !line.startsWith("<!--") &&
                    !line.startsWith("![")
            }
            .take(maxLines)
            .toList()

        return cleanedLines.joinToString("\n")
    }
}
