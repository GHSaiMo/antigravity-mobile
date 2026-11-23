package com.antigravity.mobile.data.service

import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.util.Log
import androidx.core.content.FileProvider
import com.antigravity.mobile.ui.util.loc
import java.io.File
import java.io.FileOutputStream
import java.security.MessageDigest
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * Direct-distribution in-app update manager & APK installer (GitHub Releases + China mirror fallback).
 *
 * Strictly compiled ONLY into the `direct` Gradle flavor. The `play` flavor compiles a zero-permission
 * no-op implementation so Google Play policy scanners never see APK download or installation APIs.
 */
object AppUpdateInstaller {
    private const val TAG = "AppUpdateInstaller"
    private const val PREFS_NAME = "agy_standard_prefs"
    private const val KEY_LAST_CHECK_MS = "app_update_last_check_ms"
    private const val BACKGROUND_CHECK_INTERVAL_MS = 6L * 60L * 60L * 1000L // 6 hours
    private const val UPDATES_SUBDIR = "updates"

    const val isSupported: Boolean = true

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private val _state = MutableStateFlow<AppUpdateState>(AppUpdateState.Idle)
    val state: StateFlow<AppUpdateState> = _state.asStateFlow()

    @Volatile
    private var checkJob: Job? = null

    @Volatile
    private var downloadJob: Job? = null

    private val probeClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .readTimeout(8, TimeUnit.SECONDS)
            .followRedirects(true)
            .followSslRedirects(true)
            .build()
    }

    private val downloadClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(8, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .followRedirects(true)
            .followSslRedirects(true)
            .build()
    }

    fun cleanupOldApks(context: Context) {
        scope.launch {
            runCatching {
                val dir = File(context.applicationContext.cacheDir, UPDATES_SUBDIR)
                if (dir.exists() && dir.isDirectory) {
                    val currentVer = getInstalledVersionName(context.applicationContext)
                    dir.listFiles()?.forEach { file ->
                        val fileVer = AppUpdateParser.extractVersionFromApkFileName(file.name)
                        if (file.name.endsWith(".tmp") ||
                            fileVer == null ||
                            !AppUpdateParser.isNewerVersion(fileVer, currentVer)
                        ) {
                            file.delete()
                        }
                    }
                }
            }
        }
    }

    /**
     * Silent throttled background check (at most once per 6h). Never surfaces errors to the user.
     */
    fun checkInBackgroundIfNeeded(context: Context) {
        val appContext = context.applicationContext
        cleanupOldApks(appContext)

        val currentState = _state.value
        if (currentState is AppUpdateState.Checking ||
            currentState is AppUpdateState.Downloading ||
            currentState is AppUpdateState.Available ||
            currentState is AppUpdateState.ReadyToInstall ||
            currentState is AppUpdateState.NeedsInstallPermission
        ) {
            return
        }

        val prefs = appContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        val lastCheck = prefs.getLong(KEY_LAST_CHECK_MS, 0L)
        val now = System.currentTimeMillis()
        if (now - lastCheck < BACKGROUND_CHECK_INTERVAL_MS) {
            return
        }

        checkNow(appContext, silent = true)
    }

    /**
     * Checks latest GitHub Release (with automatic `ghfast.top` mirror fallback for Mainland China).
     */
    fun checkNow(
        context: Context,
        silent: Boolean = false,
        onComplete: ((AppUpdateState) -> Unit)? = null
    ) {
        val appContext = context.applicationContext
        val currentState = _state.value
        if (currentState is AppUpdateState.Downloading) {
            onComplete?.invoke(currentState)
            return
        }
        if (checkJob?.isActive == true) return

        if (!silent) {
            _state.value = AppUpdateState.Checking
        }

        checkJob = scope.launch {
            val installedVersion = getInstalledVersionName(appContext)
            val result = fetchLatestRelease()
            val now = System.currentTimeMillis()
            appContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .edit()
                .putLong(KEY_LAST_CHECK_MS, now)
                .apply()

            val nextState: AppUpdateState = result.fold(
                onSuccess = { release ->
                    if (AppUpdateParser.isNewerVersion(release.versionName, installedVersion)) {
                        // Reuse already-downloaded verified APK if present in cache.
                        val cachedApk = File(File(appContext.cacheDir, UPDATES_SUBDIR), release.apkFileName)
                        if (cachedApk.exists() && verifyApkIntegrity(appContext, cachedApk, release.expectedSha256)) {
                            val sigDiff = doesApkSignatureDiffer(appContext, cachedApk)
                            AppUpdateState.ReadyToInstall(release, cachedApk.absolutePath, sigDiff)
                        } else {
                            AppUpdateState.Available(release)
                        }
                    } else {
                        AppUpdateState.UpToDate(installedVersion, now)
                    }
                },
                onFailure = { err ->
                    Log.w(TAG, "Update check failed (silent=$silent): ${err.message}")
                    if (silent) {
                        _state.value
                    } else {
                        AppUpdateState.Error("检查更新失败，请稍后重试".loc)
                    }
                }
            )

            _state.value = nextState
            if (onComplete != null) {
                withContext(Dispatchers.Main) {
                    onComplete(nextState)
                }
            }
        }
    }

    fun downloadAndInstall(context: Context, release: AppReleaseInfo) {
        val appContext = context.applicationContext
        downloadJob?.cancel()
        downloadJob = scope.launch {
            val updatesDir = File(appContext.cacheDir, UPDATES_SUBDIR).apply { mkdirs() }
            // Clear stale APKs before downloading.
            updatesDir.listFiles()?.forEach { it.delete() }

            val targetFile = File(updatesDir, release.apkFileName)
            val tempFile = File(updatesDir, "${release.apkFileName}.tmp")

            val urlsToTry = if (release.usedMirrorForCheck) {
                listOf(release.mirrorDownloadUrl to true, release.directDownloadUrl to false)
            } else {
                listOf(release.directDownloadUrl to false, release.mirrorDownloadUrl to true)
            }

            var lastError: Throwable? = null
            var downloadedSha256: String? = null

            for ((url, isMirror) in urlsToTry) {
                try {
                    _state.value = AppUpdateState.Downloading(
                        release = release,
                        downloadedBytes = 0L,
                        totalBytes = release.sizeBytes,
                        usingMirror = isMirror
                    )
                    downloadedSha256 = streamDownloadWithSha256(url, tempFile, release, isMirror)
                    lastError = null
                    break
                } catch (ce: CancellationException) {
                    tempFile.delete()
                    _state.value = AppUpdateState.Available(release)
                    return@launch
                } catch (e: Exception) {
                    Log.w(TAG, "APK download failed (mirror=$isMirror): ${e.message}")
                    tempFile.delete()
                    lastError = e
                }
            }

            if (lastError != null || downloadedSha256 == null || !tempFile.exists()) {
                _state.value = AppUpdateState.Error(
                    message = "下载安装包失败，请检查网络或前往浏览器下载".loc,
                    release = release
                )
                return@launch
            }

            // 1. Verify SHA-256 against official checksums.txt whenever available.
            val expectedHash = release.expectedSha256?.lowercase()
            if (!expectedHash.isNullOrBlank() && downloadedSha256 != expectedHash) {
                Log.e(TAG, "APK SHA-256 mismatch! expected=$expectedHash actual=$downloadedSha256")
                tempFile.delete()
                _state.value = AppUpdateState.Error(
                    message = "安装包校验未通过，请重试或前往 GitHub 手动下载".loc,
                    release = release
                )
                return@launch
            }

            if (!tempFile.renameTo(targetFile)) {
                tempFile.copyTo(targetFile, overwrite = true)
                tempFile.delete()
            }

            // 2. Verify APK package name matches this app.
            if (!verifyApkPackageMatches(appContext, targetFile)) {
                targetFile.delete()
                _state.value = AppUpdateState.Error(
                    message = "安装包结构异常，已取消安装".loc,
                    release = release
                )
                return@launch
            }

            val signatureDiffers = doesApkSignatureDiffer(appContext, targetFile)

            // 3. Check Android unknown-sources permission before triggering system installer.
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
                !appContext.packageManager.canRequestPackageInstalls()
            ) {
                _state.value = AppUpdateState.NeedsInstallPermission(
                    release = release,
                    apkFilePath = targetFile.absolutePath,
                    signatureDiffers = signatureDiffers
                )
                return@launch
            }

            _state.value = AppUpdateState.ReadyToInstall(
                release = release,
                apkFilePath = targetFile.absolutePath,
                signatureDiffers = signatureDiffers
            )
            withContext(Dispatchers.Main) {
                launchSystemInstaller(context, targetFile)
            }
        }
    }

    fun cancelDownload() {
        downloadJob?.cancel()
        downloadJob = null
        val cur = _state.value
        if (cur is AppUpdateState.Downloading) {
            _state.value = AppUpdateState.Available(cur.release)
        }
    }

    fun openInstallPermissionSettings(context: Context) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val intent = Intent(
                Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                Uri.parse("package:${context.packageName}")
            ).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            runCatching { context.startActivity(intent) }
        }
    }

    fun resumePendingInstall(context: Context): Boolean {
        val cur = _state.value
        val (release, path, sigDiff) = when (cur) {
            is AppUpdateState.ReadyToInstall -> Triple(cur.release, cur.apkFilePath, cur.signatureDiffers)
            is AppUpdateState.NeedsInstallPermission -> Triple(cur.release, cur.apkFilePath, cur.signatureDiffers)
            else -> return false
        }
        val apkFile = File(path)
        if (!apkFile.exists()) {
            _state.value = AppUpdateState.Available(release)
            return false
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            !context.packageManager.canRequestPackageInstalls()
        ) {
            openInstallPermissionSettings(context)
            return false
        }
        _state.value = AppUpdateState.ReadyToInstall(release, path, sigDiff)
        return launchSystemInstaller(context, apkFile)
    }

    fun dismissError() {
        val cur = _state.value
        if (cur is AppUpdateState.Error) {
            _state.value = if (cur.release != null) {
                AppUpdateState.Available(cur.release)
            } else {
                AppUpdateState.Idle
            }
        }
    }

    private fun launchSystemInstaller(context: Context, apkFile: File): Boolean {
        return runCatching {
            val contentUri = FileProvider.getUriForFile(
                context,
                "${context.packageName}.fileprovider",
                apkFile
            )
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(contentUri, "application/vnd.android.package-archive")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(intent)
            true
        }.onFailure { e ->
            Log.e(TAG, "Failed to start system APK installer: ${e.message}")
        }.getOrDefault(false)
    }

    private fun fetchLatestRelease(): Result<AppReleaseInfo> {
        // 1. Primary: Official GitHub Releases API
        val directResult = runCatching {
            val req = Request.Builder()
                .url(AppUpdateParser.GITHUB_LATEST_API_URL)
                .header("Accept", "application/vnd.github+json")
                .header("User-Agent", "Multigravity-Android")
                .build()
            probeClient.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) error("HTTP ${resp.code}")
                val body = resp.body?.string().orEmpty()
                val (release, checksumsUrl) = AppUpdateParser.parseGitHubRelease(body)
                    ?: error("No APK asset in latest release")
                val sha256 = checksumsUrl?.let { fetchTextOrNull(it) }
                    ?.let { AppUpdateParser.parseApkChecksum(it)?.second }
                release.copy(expectedSha256 = sha256)
            }
        }
        if (directResult.isSuccess) return directResult

        // 2. Fallback: China mirror `ghfast.top` via checksums.txt (+ best-effort release notes)
        return runCatching {
            val checksumsText = fetchTextOrNull(AppUpdateParser.MIRROR_LATEST_CHECKSUMS_URL)
                ?: error("Mirror checksums unavailable")
            val notesText = fetchTextOrNull(AppUpdateParser.MIRROR_RELEASE_NOTES_URL).orEmpty()
            AppUpdateParser.buildReleaseFromChecksums(checksumsText, notesText)
                ?: error("Could not parse APK release from mirror checksums")
        }
    }

    private fun fetchTextOrNull(url: String): String? {
        return runCatching {
            val req = Request.Builder()
                .url(url)
                .header("User-Agent", "Multigravity-Android")
                .build()
            probeClient.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) return@use null
                resp.body?.string()
            }
        }.getOrNull()
    }

    private fun streamDownloadWithSha256(
        url: String,
        tempFile: File,
        release: AppReleaseInfo,
        usingMirror: Boolean
    ): String {
        val req = Request.Builder()
            .url(url)
            .header("User-Agent", "Multigravity-Android")
            .build()
        downloadClient.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("Download HTTP ${resp.code}")
            val body = resp.body ?: error("Empty response body")
            val total = body.contentLength().takeIf { it > 0L } ?: release.sizeBytes
            val digest = MessageDigest.getInstance("SHA-256")
            var downloaded = 0L
            var lastEmitMs = 0L

            body.byteStream().use { input ->
                FileOutputStream(tempFile).use { output ->
                    val buffer = ByteArray(16 * 1024)
                    while (true) {
                        val read = input.read(buffer)
                        if (read < 0) break
                        if (read == 0) continue
                        output.write(buffer, 0, read)
                        digest.update(buffer, 0, read)
                        downloaded += read

                        val now = System.currentTimeMillis()
                        if (now - lastEmitMs >= 120L) {
                            lastEmitMs = now
                            _state.value = AppUpdateState.Downloading(
                                release = release,
                                downloadedBytes = downloaded,
                                totalBytes = total,
                                usingMirror = usingMirror
                            )
                        }
                    }
                    output.flush()
                }
            }

            if (downloaded < 1024L) {
                error("Downloaded file too small ($downloaded bytes)")
            }
            _state.value = AppUpdateState.Downloading(
                release = release,
                downloadedBytes = downloaded,
                totalBytes = if (total > 0L) total else downloaded,
                usingMirror = usingMirror
            )
            return digest.digest().joinToString("") { "%02x".format(it) }
        }
    }

    private fun verifyApkIntegrity(context: Context, apkFile: File, expectedSha256: String?): Boolean {
        if (!apkFile.exists() || apkFile.length() < 1024L) return false
        if (!expectedSha256.isNullOrBlank()) {
            val actual = runCatching {
                val digest = MessageDigest.getInstance("SHA-256")
                apkFile.inputStream().use { input ->
                    val buf = ByteArray(16 * 1024)
                    while (true) {
                        val r = input.read(buf)
                        if (r < 0) break
                        if (r > 0) digest.update(buf, 0, r)
                    }
                }
                digest.digest().joinToString("") { "%02x".format(it) }
            }.getOrNull() ?: return false
            if (!actual.equals(expectedSha256, ignoreCase = true)) return false
        }
        return verifyApkPackageMatches(context, apkFile)
    }

    private fun verifyApkPackageMatches(context: Context, apkFile: File): Boolean {
        val archiveInfo = getArchivePackageInfo(context, apkFile, withSignatures = false) ?: return false
        return archiveInfo.packageName == context.packageName
    }

    private fun doesApkSignatureDiffer(context: Context, apkFile: File): Boolean {
        return runCatching {
            val installedDigests = getInstalledCertSha256Set(context)
            val archiveDigests = getArchiveCertSha256Set(context, apkFile)
            if (installedDigests.isEmpty() || archiveDigests.isEmpty()) return false
            installedDigests.intersect(archiveDigests).isEmpty()
        }.getOrDefault(false)
    }

    @Suppress("DEPRECATION")
    private fun getInstalledCertSha256Set(context: Context): Set<String> {
        val pm = context.packageManager
        val info: PackageInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            pm.getPackageInfo(
                context.packageName,
                PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong())
            )
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            pm.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)
        } else {
            pm.getPackageInfo(context.packageName, PackageManager.GET_SIGNATURES)
        }
        return extractCertSha256Set(info)
    }

    private fun getArchiveCertSha256Set(context: Context, apkFile: File): Set<String> {
        val info = getArchivePackageInfo(context, apkFile, withSignatures = true) ?: return emptySet()
        return extractCertSha256Set(info)
    }

    @Suppress("DEPRECATION")
    private fun getArchivePackageInfo(
        context: Context,
        apkFile: File,
        withSignatures: Boolean
    ): PackageInfo? {
        val pm = context.packageManager
        val flags = if (!withSignatures) {
            0
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            PackageManager.GET_SIGNING_CERTIFICATES
        } else {
            PackageManager.GET_SIGNATURES
        }
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            pm.getPackageArchiveInfo(
                apkFile.absolutePath,
                PackageManager.PackageInfoFlags.of(flags.toLong())
            )
        } else {
            pm.getPackageArchiveInfo(apkFile.absolutePath, flags)
        }
    }

    @Suppress("DEPRECATION")
    private fun extractCertSha256Set(info: PackageInfo): Set<String> {
        val sigs = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val signingInfo = info.signingInfo
            if (signingInfo == null) {
                emptyArray()
            } else if (signingInfo.hasMultipleSigners()) {
                signingInfo.apkContentsSigners
            } else {
                signingInfo.signingCertificateHistory
            }
        } else {
            info.signatures ?: emptyArray()
        }
        return sigs.map { sig ->
            MessageDigest.getInstance("SHA-256")
                .digest(sig.toByteArray())
                .joinToString("") { "%02x".format(it) }
        }.toSet()
    }

    private fun getInstalledVersionName(context: Context): String {
        return runCatching {
            val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                context.packageManager.getPackageInfo(
                    context.packageName,
                    PackageManager.PackageInfoFlags.of(0)
                )
            } else {
                @Suppress("DEPRECATION")
                context.packageManager.getPackageInfo(context.packageName, 0)
            }
            info.versionName ?: "1.0.0"
        }.getOrDefault("1.0.0")
    }
}
