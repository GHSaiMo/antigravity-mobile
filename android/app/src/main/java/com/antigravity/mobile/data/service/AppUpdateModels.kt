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
    val mirrorDownloadUrls: List<String> = listOf(mirrorDownloadUrl),
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
        val usingMirror: Boolean,
        val speedBytesPerSec: Long = 0L,
        val selectingChannel: Boolean = false
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
    val digest: String? = null,
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
    val DOWNLOAD_MIRROR_PREFIXES: List<String> = listOf(
        "https://edgeone.gh-proxy.com/",
        "https://gh-proxy.org/",
        "https://ghfast.top/",
        "https://ghproxy.net/"
    )
    val RAW_MIRROR_PREFIXES: List<String> = listOf(
        "https://ghfast.top/",
        "https://ghproxy.net/"
    )
    const val GITHUB_LATEST_CHECKSUMS_URL =
        "https://github.com/$REPO_SLUG/releases/latest/download/checksums.txt"
    const val MIRROR_LATEST_CHECKSUMS_URL =
        "${MIRROR_PREFIX}$GITHUB_LATEST_CHECKSUMS_URL"
    const val MIRROR_RELEASE_NOTES_URL =
        "${MIRROR_PREFIX}https://raw.githubusercontent.com/$REPO_SLUG/main/.github/release_notes.md"

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
    }

    private val checksumLineRegex = Regex("""^\s*([0-9a-fA-F]{64})\s+\*?(\S+)\s*$""")
    private val sha256DigestRegex = Regex("""^sha256:([0-9a-fA-F]{64})$""", RegexOption.IGNORE_CASE)
    private val apkVersionRegex = Regex("""(?i)multigravity-v?(\d+(?:\.\d+)+[a-zA-Z0-9._-]*)\.apk$""")
    private val headingRegex = Regex("""^#{1,3}\s+(.+)$""")
    private val anyHeadingPrefixRegex = Regex("""^#{1,6}\s+""")
    private val markdownLinkRegex = Regex("""\[([^\]]+)\]\((https?://[^)\s]+)\)""")
    private val nestedBulletRegex = Regex("""^(\s{2,})[-*]\s+(.*)$""")
    private val topBulletRegex = Regex("""^[-*]\s+(.*)$""")
    private val blockquotePrefixRegex = Regex("""^>+\s*""")
    private val horizontalRuleRegex = Regex("""^[-*_]{3,}$""")

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

    fun buildMirrorUrl(githubUrl: String, prefix: String = MIRROR_PREFIX): String {
        val trimmed = githubUrl.trim()
        return if (trimmed.startsWith(prefix)) trimmed else "$prefix$trimmed"
    }

    fun buildMirrorUrls(githubUrl: String): List<String> {
        val trimmed = githubUrl.trim()
        if (trimmed.isBlank()) return emptyList()
        return DOWNLOAD_MIRROR_PREFIXES.map { prefix ->
            buildMirrorUrl(trimmed, prefix)
        }.distinct()
    }

    fun buildLatestChecksumsMirrorUrls(): List<String> {
        return buildMirrorUrls(GITHUB_LATEST_CHECKSUMS_URL)
    }

    fun buildReleaseNotesMirrorUrls(tagName: String): List<String> {
        val normalizedTag = tagName.trim().takeIf { it.isNotBlank() }
        val rawUrls = buildList {
            if (normalizedTag != null) {
                add("https://raw.githubusercontent.com/$REPO_SLUG/$normalizedTag/.github/release_notes.md")
            }
            add("https://raw.githubusercontent.com/$REPO_SLUG/main/.github/release_notes.md")
        }
        return rawUrls.flatMap { rawUrl ->
            RAW_MIRROR_PREFIXES.map { prefix -> buildMirrorUrl(rawUrl, prefix) }
        }.distinct()
    }

    fun parseGitHubAssetDigest(digest: String?): String? {
        val raw = digest?.trim() ?: return null
        val match = sha256DigestRegex.matchEntire(raw) ?: return null
        return match.groupValues[1].lowercase()
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
            mirrorDownloadUrls = buildMirrorUrls(apkAsset.browserDownloadUrl),
            expectedSha256 = parseGitHubAssetDigest(apkAsset.digest),
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
            mirrorDownloadUrls = buildMirrorUrls(directUrl),
            expectedSha256 = sha256,
            sizeBytes = -1L,
            releaseNotes = releaseNotes.trim(),
            releasePageUrl = "https://github.com/$REPO_SLUG/releases/tag/$tag",
            usedMirrorForCheck = true
        )
    }

    private enum class NoteLang { ZH, EN, JA }

    private fun detectSectionHeadingLang(
        trimmedLine: String,
        includeFallbackEnglishKeywords: Boolean
    ): NoteLang? {
        val headingMatch = headingRegex.matchEntire(trimmedLine) ?: return null
        val title = headingMatch.groupValues[1].trim()
        val lower = title.lowercase()
        return when {
            title.contains("🇯🇵") ||
                title.contains("日本語") ||
                title.contains("リリースノート") ||
                lower.contains("japanese") -> NoteLang.JA
            title.contains("🇨🇳") ||
                title.contains("简体中文") ||
                title.contains("繁體中文") ||
                title.contains("中文") ||
                title.contains("更新说明") ||
                title.contains("更新日志") -> NoteLang.ZH
            title.contains("🇺🇸") ||
                title.contains("🇬🇧") ||
                lower.contains("english") ||
                lower.contains("release notes") ||
                lower.contains("what's new") -> NoteLang.EN
            includeFallbackEnglishKeywords && lower.contains("highlights") -> NoteLang.EN
            else -> null
        }
    }

    private fun splitLanguageSections(
        lines: List<String>,
        includeFallbackEnglishKeywords: Boolean
    ): Pair<List<String>, Map<NoteLang, List<String>>> {
        val preamble = mutableListOf<String>()
        val sections = LinkedHashMap<NoteLang, MutableList<String>>()
        var currentLang: NoteLang? = null

        for (line in lines) {
            val detected = detectSectionHeadingLang(line.trim(), includeFallbackEnglishKeywords)
            if (detected != null) {
                currentLang = detected
                sections.getOrPut(detected) { mutableListOf() }
                continue
            }
            if (currentLang == null) {
                preamble.add(line)
            } else {
                sections.getOrPut(currentLang) { mutableListOf() }.add(line)
            }
        }
        return preamble to sections
    }

    private fun cleanNoteLine(rawLine: String): String {
        val trimmed = rawLine.trim()
        if (trimmed.isBlank() ||
            horizontalRuleRegex.matches(trimmed) ||
            trimmed.startsWith("<!--") ||
            trimmed.startsWith("![")
        ) {
            return ""
        }

        val withoutQuote = blockquotePrefixRegex.replace(trimmed, "").trim()
        if (withoutQuote.isBlank() || horizontalRuleRegex.matches(withoutQuote)) {
            return ""
        }

        val isNestedBullet = !trimmed.startsWith(">") && nestedBulletRegex.matches(rawLine)
        val normalizedStructure = when {
            isNestedBullet -> {
                val content = nestedBulletRegex.matchEntire(rawLine)?.groupValues?.getOrNull(2).orEmpty().trim()
                "  · $content"
            }
            topBulletRegex.matches(withoutQuote) -> {
                val content = topBulletRegex.matchEntire(withoutQuote)?.groupValues?.getOrNull(1).orEmpty().trim()
                "• $content"
            }
            else -> anyHeadingPrefixRegex.replace(withoutQuote, "")
        }

        return markdownLinkRegex.replace(normalizedStructure, "$1")
            .replace("**", "")
            .replace("`", "")
            .trimEnd()
    }

    /**
     * Extracts a clean, concise mobile-friendly release note snippet in the user's selected language.
     * Supports bilingual (Chinese + English) and trilingual (Chinese + English + Japanese) notes
     * including releases with a top multilingual banner preamble:
     * - Chinese users see only the Chinese section;
     * - Japanese users see only the Japanese section (falling back to English if absent);
     * - English/other users see only the English section.
     */
    fun formatReleaseNotes(
        raw: String,
        preferChinese: Boolean,
        maxLines: Int = 60,
        preferJapanese: Boolean = false
    ): String {
        val normalized = raw.replace("\r\n", "\n").replace('\r', '\n').trim()
        if (normalized.isBlank()) return ""

        val allLines = normalized.split('\n')
        var (preamble, sections) = splitLanguageSections(allLines, includeFallbackEnglishKeywords = false)
        if (NoteLang.EN !in sections) {
            val retry = splitLanguageSections(allLines, includeFallbackEnglishKeywords = true)
            if (NoteLang.EN in retry.second) {
                preamble = retry.first
                sections = retry.second
            }
        }

        val nonEmptySections = sections.filterValues { lines ->
            lines.any { cleanNoteLine(it).isNotBlank() }
        }
        val hasUsablePreamble = preamble.any { cleanNoteLine(it).isNotBlank() }

        val targetLines: List<String> = when {
            preferChinese -> {
                nonEmptySections[NoteLang.ZH]
                    ?: if (hasUsablePreamble && (NoteLang.EN in nonEmptySections || NoteLang.JA in nonEmptySections)) {
                        preamble
                    } else {
                        nonEmptySections[NoteLang.EN]
                            ?: nonEmptySections.values.firstOrNull()
                            ?: allLines
                    }
            }
            preferJapanese -> {
                nonEmptySections[NoteLang.JA]
                    ?: nonEmptySections[NoteLang.EN]
                    ?: nonEmptySections.values.firstOrNull()
                    ?: allLines
            }
            else -> {
                nonEmptySections[NoteLang.EN]
                    ?: if (hasUsablePreamble && nonEmptySections.isEmpty()) {
                        preamble
                    } else {
                        nonEmptySections.values.firstOrNull()
                            ?: allLines
                    }
            }
        }

        val cleanedLines = targetLines.asSequence()
            .map(::cleanNoteLine)
            .filter { it.isNotBlank() }
            .take(maxLines)
            .toList()

        return cleanedLines.joinToString("\n")
    }
}
