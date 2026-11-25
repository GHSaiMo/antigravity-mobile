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
import java.io.BufferedOutputStream
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.io.InputStream
import java.security.MessageDigest
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import okhttp3.Call
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response

/**
 * Direct-distribution in-app update manager & APK installer (GitHub Releases + multi-mirror racing).
 *
 * Strictly compiled ONLY into the `direct` Gradle flavor. The `play` flavor compiles a zero-permission
 * no-op implementation so Google Play policy scanners never see APK download or installation APIs.
 */
object AppUpdateInstaller {
    private const val TAG = "AppUpdateInstaller"
    private const val USER_AGENT = "Multigravity-Android"
    private const val PREFS_NAME = "agy_standard_prefs"
    private const val KEY_LAST_CHECK_MS = "app_update_last_check_ms"
    private const val BACKGROUND_CHECK_INTERVAL_MS = 6L * 60L * 60L * 1000L // 6 hours
    private const val UPDATES_SUBDIR = "updates"

    // Racing & stall-watchdog parameters for smooth downloads across both Mainland China and overseas.
    private const val RACE_PROLOGUE_BYTES = 64 * 1024 // 64 KB real APK bytes proves actual bandwidth
    private const val RACE_TIMEOUT_MS = 10_000L
    private const val RACE_WAVE_SIZE = 4
    private const val DIRECT_HEAD_START_MS = 250L
    private const val MIRROR_HEAD_START_MS = 350L
    private const val STREAM_BUFFER_SIZE = 64 * 1024
    private const val OUTPUT_BUFFER_SIZE = 128 * 1024
    private const val PROGRESS_EMIT_INTERVAL_MS = 150L
    private const val STALL_TIMEOUT_WITH_BACKUP_MS = 6_000L
    private const val STALL_TIMEOUT_LAST_RESORT_MS = 15_000L
    private const val LOW_SPEED_GRACE_MS = 3_500L
    private const val LOW_SPEED_WINDOW_MS = 5_000L
    private const val MIN_ACCEPTABLE_BYTES_PER_SEC = 40L * 1024L
    private const val LOW_SPEED_MIN_REMAINING_BYTES = 512L * 1024L

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
            .connectTimeout(4, TimeUnit.SECONDS)
            .readTimeout(6, TimeUnit.SECONDS)
            .callTimeout(10, TimeUnit.SECONDS)
            .followRedirects(true)
            .followSslRedirects(true)
            .build()
    }

    private val downloadClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .readTimeout(10, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .followRedirects(true)
            .followSslRedirects(true)
            .build()
    }

    private data class DownloadCandidate(
        val url: String,
        val usingMirror: Boolean
    )

    private class RacedStream(
        val candidate: DownloadCandidate,
        val call: Call,
        val response: Response,
        val input: InputStream,
        val prologue: ByteArray,
        val prologueLength: Int,
        val contentLength: Long,
        val prologueElapsedMs: Long
    ) {
        fun closeQuietly() {
            runCatching { input.close() }
            runCatching { response.close() }
            runCatching { call.cancel() }
        }
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
            val allCandidates = buildDownloadCandidates(release)
            val failedUrls = LinkedHashSet<String>()

            var lastError: Throwable? = null
            var checksumMismatchCount = 0
            var downloadedSha256: String? = null
            val expectedHash = release.expectedSha256?.lowercase()

            while (failedUrls.size < allCandidates.size) {
                val remaining = allCandidates.filter { it.url !in failedUrls }
                if (remaining.isEmpty()) break
                val wave = remaining.take(RACE_WAVE_SIZE)

                _state.value = AppUpdateState.Downloading(
                    release = release,
                    downloadedBytes = 0L,
                    totalBytes = release.sizeBytes,
                    usingMirror = release.usedMirrorForCheck,
                    speedBytesPerSec = 0L,
                    selectingChannel = true
                )

                val raced = try {
                    raceForFastestStream(wave, preferMirror = release.usedMirrorForCheck)
                } catch (ce: CancellationException) {
                    tempFile.delete()
                    _state.value = AppUpdateState.Available(release)
                    return@launch
                }

                if (raced == null) {
                    wave.forEach { failedUrls.add(it.url) }
                    lastError = IOException("All candidates in wave failed or timed out")
                    continue
                }

                val hasBackupCandidates = allCandidates.any {
                    it.url != raced.candidate.url && it.url !in failedUrls
                }

                try {
                    val sha256 = streamFromRacedWithWatchdog(
                        raced = raced,
                        tempFile = tempFile,
                        release = release,
                        hasBackupCandidates = hasBackupCandidates
                    )
                    if (!expectedHash.isNullOrBlank() && sha256 != expectedHash) {
                        Log.e(
                            TAG,
                            "APK SHA-256 mismatch from ${raced.candidate.url}! expected=$expectedHash actual=$sha256"
                        )
                        checksumMismatchCount++
                        tempFile.delete()
                        failedUrls.add(raced.candidate.url)
                        lastError = IOException("SHA-256 mismatch")
                        continue
                    }
                    downloadedSha256 = sha256
                    lastError = null
                    break
                } catch (ce: CancellationException) {
                    tempFile.delete()
                    _state.value = AppUpdateState.Available(release)
                    return@launch
                } catch (e: Exception) {
                    Log.w(
                        TAG,
                        "APK stream failed (mirror=${raced.candidate.usingMirror}, url=${raced.candidate.url}): ${e.message}"
                    )
                    tempFile.delete()
                    failedUrls.add(raced.candidate.url)
                    lastError = e
                } finally {
                    raced.closeQuietly()
                }
            }

            if (lastError != null || downloadedSha256 == null || !tempFile.exists()) {
                val errMsg = if (checksumMismatchCount > 0 && checksumMismatchCount == failedUrls.size) {
                    "安装包校验未通过，请重试或前往 GitHub 手动下载".loc
                } else {
                    "下载安装包失败，请检查网络或前往浏览器下载".loc
                }
                _state.value = AppUpdateState.Error(
                    message = errMsg,
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

    private fun buildDownloadCandidates(release: AppReleaseInfo): List<DownloadCandidate> {
        val direct = DownloadCandidate(release.directDownloadUrl, usingMirror = false)
        val mirrorUrls = release.mirrorDownloadUrls
            .ifEmpty { AppUpdateParser.buildMirrorUrls(release.directDownloadUrl) }
            .ifEmpty { listOf(release.mirrorDownloadUrl) }
            .filter { it.isNotBlank() && it != release.directDownloadUrl }
            .distinct()
        val mirrors = mirrorUrls.map { DownloadCandidate(it, usingMirror = true) }
        return if (release.usedMirrorForCheck) {
            mirrors + direct
        } else {
            listOf(direct) + mirrors
        }
    }

    private suspend fun fetchLatestRelease(): Result<AppReleaseInfo> {
        // 1. Primary: Official GitHub Releases API (already carries official asset SHA-256 `digest`).
        val directResult = runCatching {
            val req = Request.Builder()
                .url(AppUpdateParser.GITHUB_LATEST_API_URL)
                .header("Accept", "application/vnd.github+json")
                .header("User-Agent", USER_AGENT)
                .build()
            probeClient.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) error("HTTP ${resp.code}")
                val body = resp.body?.string().orEmpty()
                val (release, checksumsUrl) = AppUpdateParser.parseGitHubRelease(body)
                    ?: error("No APK asset in latest release")
                if (!release.expectedSha256.isNullOrBlank()) {
                    release
                } else {
                    val sha256 = checksumsUrl?.let { fetchTextOrNull(it) }
                        ?.let { AppUpdateParser.parseApkChecksum(it)?.second }
                    release.copy(expectedSha256 = sha256)
                }
            }
        }
        if (directResult.isSuccess) return directResult

        // 2. Fallback: Race China mirrors for `checksums.txt` (+ tagged `release_notes.md`).
        return runCatching {
            val checksumsText = fetchFirstText(AppUpdateParser.buildLatestChecksumsMirrorUrls()) { text ->
                AppUpdateParser.parseApkChecksum(text) != null
            } ?: error("Mirror checksums unavailable")
            val parsedBase = AppUpdateParser.buildReleaseFromChecksums(checksumsText)
                ?: error("Could not parse APK release from mirror checksums")
            val notesText = fetchFirstText(AppUpdateParser.buildReleaseNotesMirrorUrls(parsedBase.tagName)) { text ->
                text.isNotBlank() && !text.trimStart().startsWith("<!DOCTYPE", ignoreCase = true)
            }.orEmpty()
            parsedBase.copy(releaseNotes = notesText.trim())
        }
    }

    private fun fetchTextOrNull(url: String): String? {
        return runCatching {
            val req = Request.Builder()
                .url(url)
                .header("User-Agent", USER_AGENT)
                .build()
            probeClient.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) return@use null
                resp.body?.string()
            }
        }.getOrNull()
    }

    private suspend fun fetchFirstText(
        urls: List<String>,
        validator: (String) -> Boolean
    ): String? = coroutineScope {
        if (urls.isEmpty()) return@coroutineScope null
        val resultDeferred = CompletableDeferred<String?>()
        val activeCalls = CopyOnWriteArrayList<Call>()
        val remaining = AtomicInteger(urls.size)

        val jobs = urls.map { url ->
            launch(Dispatchers.IO) {
                var call: Call? = null
                try {
                    if (resultDeferred.isCompleted) return@launch
                    val req = Request.Builder()
                        .url(url)
                        .header("User-Agent", USER_AGENT)
                        .build()
                    val createdCall = probeClient.newCall(req)
                    call = createdCall
                    activeCalls.add(createdCall)
                    if (resultDeferred.isCompleted) {
                        createdCall.cancel()
                        return@launch
                    }
                    createdCall.execute().use { resp ->
                        if (!resp.isSuccessful) return@launch
                        val text = resp.body?.string() ?: return@launch
                        if (validator(text)) {
                            resultDeferred.complete(text)
                        }
                    }
                } catch (ce: CancellationException) {
                    throw ce
                } catch (_: Exception) {
                    // Try other mirrors
                } finally {
                    if (remaining.decrementAndGet() == 0 && !resultDeferred.isCompleted) {
                        resultDeferred.complete(null)
                    }
                }
            }
        }

        try {
            withTimeoutOrNull(8_000L) { resultDeferred.await() }
        } finally {
            resultDeferred.complete(null)
            jobs.forEach { it.cancel() }
            activeCalls.forEach { runCatching { it.cancel() } }
        }
    }

    private fun hasApkZipMagic(bytes: ByteArray, length: Int): Boolean {
        return length >= 4 &&
            bytes[0] == 0x50.toByte() &&
            bytes[1] == 0x4B.toByte() &&
            bytes[2] == 0x03.toByte() &&
            bytes[3] == 0x04.toByte()
    }

    /**
     * Races candidate channels in parallel and picks the first one that delivers a real 64 KB
     * APK/ZIP prologue (`PK\x03\x04`). Winning connection stays open so zero bytes or RTT are wasted.
     */
    private suspend fun raceForFastestStream(
        candidates: List<DownloadCandidate>,
        preferMirror: Boolean
    ): RacedStream? = coroutineScope {
        if (candidates.isEmpty()) return@coroutineScope null
        val winnerDeferred = CompletableDeferred<RacedStream?>()
        val winningStream = AtomicReference<RacedStream?>(null)
        val raceClosed = AtomicBoolean(false)
        val activeCalls = CopyOnWriteArrayList<Call>()
        val remaining = AtomicInteger(candidates.size)
        val hasDirect = candidates.any { !it.usingMirror }
        val hasMirror = candidates.any { it.usingMirror }

        val racerJobs = candidates.map { candidate ->
            launch(Dispatchers.IO) {
                var resp: Response? = null
                var call: Call? = null
                var ownershipTransferred = false
                try {
                    if (!preferMirror && candidate.usingMirror && hasDirect) {
                        delay(DIRECT_HEAD_START_MS)
                    } else if (preferMirror && !candidate.usingMirror && hasMirror) {
                        delay(MIRROR_HEAD_START_MS)
                    }
                    if (winnerDeferred.isCompleted || raceClosed.get()) return@launch

                    val req = Request.Builder()
                        .url(candidate.url)
                        .header("User-Agent", USER_AGENT)
                        .build()
                    val createdCall = downloadClient.newCall(req)
                    call = createdCall
                    activeCalls.add(createdCall)
                    if (winnerDeferred.isCompleted || raceClosed.get()) {
                        createdCall.cancel()
                        return@launch
                    }

                    val startMs = System.currentTimeMillis()
                    val response = createdCall.execute()
                    resp = response
                    if (!response.isSuccessful) {
                        return@launch
                    }
                    val contentType = response.header("Content-Type").orEmpty()
                    if (contentType.contains("text/html", ignoreCase = true)) {
                        return@launch
                    }
                    val body = response.body ?: return@launch
                    val input = body.byteStream()
                    val prologue = ByteArray(RACE_PROLOGUE_BYTES)
                    var readTotal = 0
                    while (readTotal < RACE_PROLOGUE_BYTES) {
                        if (winnerDeferred.isCompleted || raceClosed.get() || !isActive) return@launch
                        val n = input.read(prologue, readTotal, RACE_PROLOGUE_BYTES - readTotal)
                        if (n < 0) break
                        if (n > 0) {
                            readTotal += n
                            if (readTotal >= 4 && !hasApkZipMagic(prologue, readTotal)) {
                                return@launch
                            }
                        }
                    }
                    if (readTotal < 4 || !hasApkZipMagic(prologue, readTotal)) {
                        return@launch
                    }

                    val elapsedMs = (System.currentTimeMillis() - startMs).coerceAtLeast(1L)
                    val stream = RacedStream(
                        candidate = candidate,
                        call = createdCall,
                        response = response,
                        input = input,
                        prologue = prologue,
                        prologueLength = readTotal,
                        contentLength = body.contentLength(),
                        prologueElapsedMs = elapsedMs
                    )
                    if (!raceClosed.get() && winningStream.compareAndSet(null, stream)) {
                        ownershipTransferred = true
                        if (raceClosed.get()) {
                            if (winningStream.compareAndSet(stream, null)) {
                                stream.closeQuietly()
                            }
                        } else {
                            winnerDeferred.complete(stream)
                        }
                    } else {
                        ownershipTransferred = true
                        stream.closeQuietly()
                    }
                } catch (ce: CancellationException) {
                    throw ce
                } catch (e: Exception) {
                    Log.d(TAG, "Race candidate failed (${candidate.url}): ${e.message}")
                } finally {
                    if (!ownershipTransferred) {
                        runCatching { resp?.close() }
                        runCatching { call?.cancel() }
                    }
                    if (remaining.decrementAndGet() == 0 && !winnerDeferred.isCompleted) {
                        winnerDeferred.complete(null)
                    }
                }
            }
        }

        var handedOff = false
        try {
            val winner = withTimeoutOrNull(RACE_TIMEOUT_MS) {
                winnerDeferred.await()
            }
            raceClosed.set(true)
            winnerDeferred.complete(null)
            val claimed = if (winner != null && winningStream.compareAndSet(winner, null)) {
                handedOff = true
                winner
            } else {
                null
            }
            val keepCall = claimed?.call
            racerJobs.forEach { it.cancel() }
            activeCalls.forEach { c ->
                if (c !== keepCall) runCatching { c.cancel() }
            }
            claimed
        } finally {
            raceClosed.set(true)
            winnerDeferred.complete(null)
            racerJobs.forEach { it.cancel() }
            val leftover = winningStream.getAndSet(null)
            if (!handedOff) {
                activeCalls.forEach { runCatching { it.cancel() } }
                leftover?.closeQuietly()
            } else if (leftover != null) {
                leftover.closeQuietly()
            }
        }
    }

    private suspend fun streamFromRacedWithWatchdog(
        raced: RacedStream,
        tempFile: File,
        release: AppReleaseInfo,
        hasBackupCandidates: Boolean
    ): String = coroutineScope {
        val total = raced.contentLength.takeIf { it > 0L } ?: release.sizeBytes
        val digest = MessageDigest.getInstance("SHA-256")
        val streamStartMs = System.currentTimeMillis()
        val downloadedRef = AtomicLong(0L)
        val lastReadMsRef = AtomicLong(streamStartMs)
        val abortReason = AtomicReference<String?>(null)
        val stallTimeoutMs = if (hasBackupCandidates) {
            STALL_TIMEOUT_WITH_BACKUP_MS
        } else {
            STALL_TIMEOUT_LAST_RESORT_MS
        }

        val cancelHandle = currentCoroutineContext().job.invokeOnCompletion {
            runCatching { raced.call.cancel() }
        }

        val watchdogJob = launch(Dispatchers.Default) {
            val samples = ArrayDeque<Pair<Long, Long>>()
            samples.addLast(streamStartMs to 0L)
            while (isActive) {
                delay(1_000L)
                val now = System.currentTimeMillis()
                val curBytes = downloadedRef.get()
                if (now - lastReadMsRef.get() >= stallTimeoutMs) {
                    abortReason.compareAndSet(null, "Stream stalled for ${stallTimeoutMs / 1000}s")
                    runCatching { raced.call.cancel() }
                    return@launch
                }
                samples.addLast(now to curBytes)
                while (samples.size > 6) {
                    samples.removeFirst()
                }
                if (hasBackupCandidates && now - streamStartMs >= LOW_SPEED_GRACE_MS) {
                    val oldest = samples.first()
                    val windowMs = now - oldest.first
                    if (windowMs >= LOW_SPEED_WINDOW_MS) {
                        val windowSpeed = ((curBytes - oldest.second) * 1000L) / windowMs
                        val remainingBytes = if (total > 0L) (total - curBytes) else Long.MAX_VALUE
                        if (windowSpeed < MIN_ACCEPTABLE_BYTES_PER_SEC &&
                            remainingBytes > LOW_SPEED_MIN_REMAINING_BYTES
                        ) {
                            abortReason.compareAndSet(
                                null,
                                "Channel too slow (${windowSpeed / 1024} KB/s)"
                            )
                            runCatching { raced.call.cancel() }
                            return@launch
                        }
                    }
                }
            }
        }

        try {
            var downloaded = 0L
            val initialSpeed = (raced.prologueLength.toLong() * 1000L) / raced.prologueElapsedMs
            var smoothedSpeed = initialSpeed
            var speedWindowStartMs = streamStartMs
            var speedWindowStartBytes = 0L
            var lastEmitMs = 0L

            BufferedOutputStream(FileOutputStream(tempFile), OUTPUT_BUFFER_SIZE).use { output ->
                // 1. Flush the already-downloaded 64 KB prologue immediately.
                output.write(raced.prologue, 0, raced.prologueLength)
                digest.update(raced.prologue, 0, raced.prologueLength)
                downloaded = raced.prologueLength.toLong()
                downloadedRef.set(downloaded)
                speedWindowStartBytes = downloaded
                val afterPrologueMs = System.currentTimeMillis()
                lastReadMsRef.set(afterPrologueMs)
                lastEmitMs = afterPrologueMs

                _state.value = AppUpdateState.Downloading(
                    release = release,
                    downloadedBytes = downloaded,
                    totalBytes = total,
                    usingMirror = raced.candidate.usingMirror,
                    speedBytesPerSec = smoothedSpeed,
                    selectingChannel = false
                )

                // 2. Continue streaming from the exact same open socket.
                val buffer = ByteArray(STREAM_BUFFER_SIZE)
                while (true) {
                    if (!currentCoroutineContext().isActive) {
                        throw CancellationException("Download cancelled")
                    }
                    val read = raced.input.read(buffer)
                    if (read < 0) break
                    if (read == 0) continue

                    output.write(buffer, 0, read)
                    digest.update(buffer, 0, read)
                    downloaded += read
                    val now = System.currentTimeMillis()
                    downloadedRef.set(downloaded)
                    lastReadMsRef.set(now)

                    val windowElapsed = now - speedWindowStartMs
                    if (windowElapsed >= 1_000L) {
                        val instantSpeed = ((downloaded - speedWindowStartBytes) * 1000L) / windowElapsed
                        smoothedSpeed = if (smoothedSpeed <= 0L) {
                            instantSpeed
                        } else {
                            (smoothedSpeed * 4L + instantSpeed * 6L) / 10L
                        }
                        speedWindowStartMs = now
                        speedWindowStartBytes = downloaded
                    }

                    if (now - lastEmitMs >= PROGRESS_EMIT_INTERVAL_MS) {
                        lastEmitMs = now
                        _state.value = AppUpdateState.Downloading(
                            release = release,
                            downloadedBytes = downloaded,
                            totalBytes = total,
                            usingMirror = raced.candidate.usingMirror,
                            speedBytesPerSec = smoothedSpeed,
                            selectingChannel = false
                        )
                    }
                }
                output.flush()
            }

            if (downloaded < 1024L) {
                throw IOException("Downloaded file too small ($downloaded bytes)")
            }
            if (raced.contentLength > 0L && downloaded < raced.contentLength) {
                throw IOException("Truncated APK download ($downloaded / ${raced.contentLength})")
            }

            _state.value = AppUpdateState.Downloading(
                release = release,
                downloadedBytes = downloaded,
                totalBytes = if (total > 0L) total else downloaded,
                usingMirror = raced.candidate.usingMirror,
                speedBytesPerSec = smoothedSpeed,
                selectingChannel = false
            )
            digest.digest().joinToString("") { "%02x".format(it) }
        } catch (e: IOException) {
            if (!currentCoroutineContext().isActive) {
                throw CancellationException("Download cancelled", e)
            }
            val reason = abortReason.get()
            if (reason != null) {
                throw IOException(reason, e)
            }
            throw e
        } finally {
            watchdogJob.cancel()
            cancelHandle.dispose()
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
