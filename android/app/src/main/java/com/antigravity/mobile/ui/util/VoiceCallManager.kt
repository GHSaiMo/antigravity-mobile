package com.antigravity.mobile.ui.util

import android.annotation.SuppressLint
import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioManager
import android.media.AudioRecord
import android.media.AudioTrack
import android.media.MediaRecorder
import android.media.audiofx.AcousticEchoCanceler
import android.media.audiofx.NoiseSuppressor
import android.net.Uri
import android.util.Log
import com.antigravity.mobile.data.service.CloudflareOptimizedDns
import com.antigravity.mobile.data.service.LanCleartextSecurityInterceptor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.TimeUnit
import kotlin.math.sqrt

enum class VoiceCallState {
    IDLE,
    CONNECTING,
    LISTENING,
    RECONNECTING,
    FAILED
}

enum class VoiceTranscriptRole {
    USER,
    AI
}

class VoiceCallManager(private val context: Context) {

    companion object {
        /** 语音连接失败后，用户点「一键排查」时发送给智能体的预设提示词（三端保持一致）。 */
        val TROUBLESHOOT_PROMPT = """
            |【任务】排查「本机无法使用谷歌语音(Gemini Live)功能」的问题。
            |
            |【现象】
            |- 本机 Antigravity 的语音功能无法连接，界面提示 “Couldn't connect / Try again”
            |
            |【请按顺序排查】
            |1. 登录与授权：
            |   - 检查登录态的 OAuth Scope 是否完整。旧登录态可能缺少语音所需的新权限
            |   - 如需重新授权，先退出 Antigravity，将 jetski-standalone-oauth-token 文件重命名或移走，再启动并重新登录，以强制拉取完整 Scope
            |2. 代理与长连接：
            |   - 语音使用双向流式通信（bidi stream / gRPC）
            |   - 确认代理（Clash / Clash Verge / Sing-box 等）已开启 TUN 模式，或支持对 *.googleapis.com 的 HTTP/2 与 gRPC 流式转发
            |   - 查看日志是否出现 context canceled while waiting for connections（连接超时的典型特征）
            |3. 系统麦克风：
            |   - 确认系统麦克风访问权限与「允许桌面应用访问麦克风」均已开启
            |   - 确认默认录音设备正常，且没有被其他程序占用
            |4. 网关链路：检查网关语音 WebSocket 链路与日志（mgy 会话输出）
            |
            |【输出要求】
            |- 先给出根因判断与证据（日志、命令输出），再给修复步骤
            |- 能自动修复的直接修复；涉及改配置、重启服务或重新登录的，先说明再执行
            |- 修复后验证语音是否恢复
        """.trimMargin()

        val SUPPORTED_SPEEDS = listOf(0.75, 1.0, 1.25, 1.5, 1.75, 2.0)
        private const val MIC_SAMPLE_RATE = 16000
        private const val PLAYBACK_SAMPLE_RATE = 24000
        private const val CHUNK_SAMPLES = 640 // 40ms @ 16kHz
        private const val CHUNK_BYTES = CHUNK_SAMPLES * 2 // 1280 bytes

        fun resolvePlanModelInt(modelIdOrEnum: String?): Int {
            val raw = modelIdOrEnum?.trim().orEmpty()
            if (raw.isEmpty()) return 0
            raw.toIntOrNull()?.let { if (it > 0) return it }
            if (raw.startsWith("MODEL_PLACEHOLDER_M")) {
                val num = raw.removePrefix("MODEL_PLACEHOLDER_M").toIntOrNull()
                if (num != null && num > 0) return 1000 + num
            }
            return when (raw.lowercase()) {
                "gemini-3.8-flash-high" -> 1318
                "gemini-3.8-flash-medium" -> 1319
                "gemini-3.8-flash-low" -> 1320
                "gemini-3.1-pro-high" -> 1037
                "gemini-3.1-pro-low" -> 1036
                "claude-opus-4-6-thinking" -> 1026
                "claude-sonnet-4-6" -> 1035
                else -> 0
            }
        }
    }

    private val client = OkHttpClient.Builder()
        .dns(CloudflareOptimizedDns)
        .pingInterval(15, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .addInterceptor(LanCleartextSecurityInterceptor())
        .build()

    private val scope = CoroutineScope(Dispatchers.Main + SupervisorJob())
    private val ioScope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    private val _isInCall = MutableStateFlow(false)
    val isInCall: StateFlow<Boolean> = _isInCall.asStateFlow()

    /** 连接明确失败（endCall 会把状态复位为 IDLE，因此单独用事件通知 UI 弹出排查气泡）。 */
    private val _failureEvents = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val failureEvents: SharedFlow<Unit> = _failureEvents.asSharedFlow()

    private val _callState = MutableStateFlow(VoiceCallState.IDLE)
    val callState: StateFlow<VoiceCallState> = _callState.asStateFlow()

    private val _durationSeconds = MutableStateFlow(0)
    val durationSeconds: StateFlow<Int> = _durationSeconds.asStateFlow()

    private val _isMicMuted = MutableStateFlow(false)
    val isMicMuted: StateFlow<Boolean> = _isMicMuted.asStateFlow()

    private val _isSpeakerMuted = MutableStateFlow(false)
    val isSpeakerMuted: StateFlow<Boolean> = _isSpeakerMuted.asStateFlow()

    private val _playbackSpeed = MutableStateFlow(1.0)
    val playbackSpeed: StateFlow<Double> = _playbackSpeed.asStateFlow()

    private val _isAgentSpeaking = MutableStateFlow(false)
    val isAgentSpeaking: StateFlow<Boolean> = _isAgentSpeaking.asStateFlow()

    private val _soundLevels = MutableStateFlow(List(18) { 0.12f })
    val soundLevels: StateFlow<List<Float>> = _soundLevels.asStateFlow()

    private val _liveTranscriptText = MutableStateFlow("")
    val liveTranscriptText: StateFlow<String> = _liveTranscriptText.asStateFlow()

    private val _liveTranscriptRole = MutableStateFlow(VoiceTranscriptRole.USER)
    val liveTranscriptRole: StateFlow<VoiceTranscriptRole> = _liveTranscriptRole.asStateFlow()

    var onCascadeBound: ((String) -> Unit)? = null

    private var webSocket: WebSocket? = null
    private var isSocketReady = false
    private var isIntentionallyClosed = false
    private var activeCascadeId = ""

    private var audioRecord: AudioRecord? = null
    private var audioTrack: AudioTrack? = null
    private var echoCanceler: AcousticEchoCanceler? = null
    private var noiseSuppressor: NoiseSuppressor? = null

    private var micCaptureJob: Job? = null
    private var durationJob: Job? = null
    private var transcriptClearJob: Job? = null
    private var agentSpeakingDecayJob: Job? = null

    @SuppressLint("MissingPermission")
    fun startCall(
        cascadeId: String,
        modelIdOrEnum: String?,
        baseUrl: String?,
        deviceToken: String?
    ): Boolean {
        if (_isInCall.value) {
            endCall()
        }
        if (baseUrl.isNullOrBlank()) {
            _callState.value = VoiceCallState.FAILED
            return false
        }

        isIntentionallyClosed = false
        isSocketReady = false
        activeCascadeId = cascadeId
        _isInCall.value = true
        _callState.value = VoiceCallState.CONNECTING
        _durationSeconds.value = 0
        _isMicMuted.value = false
        _isSpeakerMuted.value = false
        _isAgentSpeaking.value = false
        _liveTranscriptText.value = ""
        _soundLevels.value = List(18) { 0.12f }

        if (!setupAudioPipeline()) {
            _callState.value = VoiceCallState.FAILED
            _failureEvents.tryEmit(Unit)
            endCall()
            return false
        }

        connectWebSocket(baseUrl, cascadeId, modelIdOrEnum, deviceToken)
        startDurationTimer()
        return true
    }

    fun endCall() {
        isIntentionallyClosed = true
        isSocketReady = false

        durationJob?.cancel()
        durationJob = null
        transcriptClearJob?.cancel()
        transcriptClearJob = null
        agentSpeakingDecayJob?.cancel()
        agentSpeakingDecayJob = null
        micCaptureJob?.cancel()
        micCaptureJob = null

        try {
            webSocket?.close(1000, "User hang up")
        } catch (_: Exception) {}
        webSocket = null

        teardownAudioPipeline()

        _isInCall.value = false
        _isAgentSpeaking.value = false
        _liveTranscriptText.value = ""
        _soundLevels.value = List(18) { 0.12f }
        _callState.value = VoiceCallState.IDLE
        activeCascadeId = ""
    }

    fun toggleMicMute() {
        _isMicMuted.value = !_isMicMuted.value
    }

    fun toggleSpeakerMute() {
        val next = !_isSpeakerMuted.value
        _isSpeakerMuted.value = next
        try {
            audioTrack?.setVolume(if (next) 0f else 1f)
        } catch (_: Exception) {}
    }

    fun cyclePlaybackSpeed() {
        val idx = SUPPORTED_SPEEDS.indexOf(_playbackSpeed.value)
        val next = if (idx >= 0) SUPPORTED_SPEEDS[(idx + 1) % SUPPORTED_SPEEDS.size] else 1.0
        _playbackSpeed.value = next
        sendSettingsUpdate(planModel = null, playbackSpeed = next)
    }

    fun updatePlanModel(modelIdOrEnum: String?) {
        if (!_isInCall.value) return
        val modelInt = resolvePlanModelInt(modelIdOrEnum)
        if (modelInt > 0) {
            sendSettingsUpdate(planModel = modelInt, playbackSpeed = null)
        }
    }

    @SuppressLint("MissingPermission")
    private fun setupAudioPipeline(): Boolean {
        return try {
            val audioManager = context.getSystemService(Context.AUDIO_SERVICE) as? AudioManager
            audioManager?.mode = AudioManager.MODE_IN_COMMUNICATION
            @Suppress("DEPRECATION")
            audioManager?.isSpeakerphoneOn = true

            val minInBuf = AudioRecord.getMinBufferSize(
                MIC_SAMPLE_RATE,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT
            ).coerceAtLeast(CHUNK_BYTES * 4)

            val record = AudioRecord(
                MediaRecorder.AudioSource.VOICE_COMMUNICATION,
                MIC_SAMPLE_RATE,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT,
                minInBuf
            )
            if (record.state != AudioRecord.STATE_INITIALIZED) {
                record.release()
                return false
            }

            if (AcousticEchoCanceler.isAvailable()) {
                echoCanceler = AcousticEchoCanceler.create(record.audioSessionId)?.apply { enabled = true }
            }
            if (NoiseSuppressor.isAvailable()) {
                noiseSuppressor = NoiseSuppressor.create(record.audioSessionId)?.apply { enabled = true }
            }

            val minOutBuf = AudioTrack.getMinBufferSize(
                PLAYBACK_SAMPLE_RATE,
                AudioFormat.CHANNEL_OUT_MONO,
                AudioFormat.ENCODING_PCM_16BIT
            ).coerceAtLeast(PLAYBACK_SAMPLE_RATE * 2 / 5)

            val track = AudioTrack.Builder()
                .setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_VOICE_COMMUNICATION)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                        .build()
                )
                .setAudioFormat(
                    AudioFormat.Builder()
                        .setEncoding(AudioFormat.ENCODING_PCM_16BIT)
                        .setSampleRate(PLAYBACK_SAMPLE_RATE)
                        .setChannelMask(AudioFormat.CHANNEL_OUT_MONO)
                        .build()
                )
                .setBufferSizeInBytes(minOutBuf)
                .setTransferMode(AudioTrack.MODE_STREAM)
                .build()

            track.setVolume(if (_isSpeakerMuted.value) 0f else 1f)
            track.play()
            record.startRecording()

            audioRecord = record
            audioTrack = track

            startMicCaptureLoop(record)
            true
        } catch (e: Exception) {
            Log.e("VoiceCallManager", "Audio pipeline setup failed", e)
            teardownAudioPipeline()
            false
        }
    }

    private fun startMicCaptureLoop(record: AudioRecord) {
        micCaptureJob?.cancel()
        micCaptureJob = ioScope.launch {
            val frameBuf = ByteArray(CHUNK_BYTES)
            val silenceBuf = ByteArray(CHUNK_BYTES)
            while (isActive && _isInCall.value) {
                var offset = 0
                while (offset < CHUNK_BYTES && isActive && _isInCall.value) {
                    val read = record.read(frameBuf, offset, CHUNK_BYTES - offset)
                    if (read <= 0) break
                    offset += read
                }
                if (offset < CHUNK_BYTES || !_isInCall.value) continue

                val muted = _isMicMuted.value
                if (!_isAgentSpeaking.value) {
                    val level = if (muted) 0.10f else computePcm16Rms(frameBuf).coerceIn(0.12f, 1.0f)
                    pushSoundLevel(level)
                }

                if (isSocketReady) {
                    val payload = if (muted) silenceBuf else frameBuf
                    val protoFrame = encodeMicAudioFrame(payload)
                    webSocket?.send(protoFrame.toByteString())
                }
            }
        }
    }

    private fun teardownAudioPipeline() {
        try {
            echoCanceler?.release()
        } catch (_: Exception) {}
        echoCanceler = null

        try {
            noiseSuppressor?.release()
        } catch (_: Exception) {}
        noiseSuppressor = null

        try {
            audioRecord?.stop()
        } catch (_: Exception) {}
        try {
            audioRecord?.release()
        } catch (_: Exception) {}
        audioRecord = null

        try {
            audioTrack?.pause()
            audioTrack?.flush()
            audioTrack?.release()
        } catch (_: Exception) {}
        audioTrack = null

        try {
            val audioManager = context.getSystemService(Context.AUDIO_SERVICE) as? AudioManager
            audioManager?.mode = AudioManager.MODE_NORMAL
        } catch (_: Exception) {}
    }

    private fun connectWebSocket(
        baseUrl: String,
        cascadeId: String,
        modelIdOrEnum: String?,
        deviceToken: String?
    ) {
        val cleanBase = baseUrl.trimEnd('/')
        val wsBase = when {
            cleanBase.startsWith("https://", ignoreCase = true) -> "wss://" + cleanBase.substring(8)
            cleanBase.startsWith("http://", ignoreCase = true) -> "ws://" + cleanBase.substring(7)
            else -> "ws://$cleanBase"
        }
        val planModelInt = resolvePlanModelInt(modelIdOrEnum)
        val wsUrl = Uri.parse("$wsBase/voice/ws").buildUpon()
            .appendQueryParameter("conversation_id", cascadeId)
            .appendQueryParameter("plan_model", planModelInt.toString())
            .appendQueryParameter("sample_rate", MIC_SAMPLE_RATE.toString())
            .build()
            .toString()

        val reqBuilder = Request.Builder()
            .url(wsUrl)
            .header("Sec-WebSocket-Protocol", "jetski-voice-proto")

        val token = deviceToken?.takeIf { it.isNotBlank() }
        if (token != null) {
            reqBuilder.header("Authorization", "Bearer $token")
            reqBuilder.header("x-device-token", token)
        }

        webSocket = client.newWebSocket(reqBuilder.build(), object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                isSocketReady = true
                if (_callState.value == VoiceCallState.CONNECTING) {
                    _callState.value = VoiceCallState.LISTENING
                }
                if (_playbackSpeed.value != 1.0) {
                    sendSettingsUpdate(planModel = null, playbackSpeed = _playbackSpeed.value)
                }
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                handleServerProtobufFrame(bytes.toByteArray())
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
                if (!isIntentionallyClosed) {
                    scope.launch {
                        _failureEvents.tryEmit(Unit)
                        endCall()
                    }
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.w("VoiceCallManager", "Voice WS failure: ${t.message}")
                if (!isIntentionallyClosed) {
                    scope.launch {
                        _callState.value = VoiceCallState.FAILED
                        _failureEvents.tryEmit(Unit)
                        endCall()
                    }
                }
            }
        })
    }

    private fun enqueuePcm24k(pcmBytes: ByteArray) {
        if (pcmBytes.isEmpty()) return
        val level = computePcm16Rms(pcmBytes, gain = 4.5f).coerceIn(0.16f, 1.0f)
        _isAgentSpeaking.value = true
        pushSoundLevel(level)

        try {
            audioTrack?.write(pcmBytes, 0, pcmBytes.size)
        } catch (_: Exception) {}

        agentSpeakingDecayJob?.cancel()
        agentSpeakingDecayJob = scope.launch {
            delay(320)
            _isAgentSpeaking.value = false
        }
    }

    private fun clearPlaybackQueue() {
        try {
            audioTrack?.pause()
            audioTrack?.flush()
            audioTrack?.play()
        } catch (_: Exception) {}
        _isAgentSpeaking.value = false
    }

    private fun pushSoundLevel(level: Float) {
        val next = _soundLevels.value.toMutableList()
        if (next.isNotEmpty()) {
            next.removeAt(0)
        }
        next.add(level)
        _soundLevels.value = next
    }

    private fun computePcm16Rms(bytes: ByteArray, gain: Float = 5.2f): Float {
        val sampleCount = bytes.size / 2
        if (sampleCount <= 0) return 0.12f
        var sumSq = 0f
        var i = 0
        while (i + 1 < bytes.size) {
            val lo = bytes[i].toInt() and 0xFF
            val hi = bytes[i + 1].toInt()
            val sample = ((hi shl 8) or lo).toShort()
            val norm = sample / 32768f
            sumSq += norm * norm
            i += 2
        }
        val rms = sqrt(sumSq / sampleCount)
        return rms * gain
    }

    private fun startDurationTimer() {
        durationJob?.cancel()
        durationJob = scope.launch {
            val startMs = System.currentTimeMillis()
            while (isActive && _isInCall.value) {
                delay(500)
                _durationSeconds.value = ((System.currentTimeMillis() - startMs) / 1000L).toInt()
            }
        }
    }

    private fun updateLiveTranscript(text: String, role: VoiceTranscriptRole) {
        val trimmed = text.trim()
        if (trimmed.isEmpty()) return
        _liveTranscriptRole.value = role
        _liveTranscriptText.value = trimmed
        transcriptClearJob?.cancel()
        transcriptClearJob = scope.launch {
            delay(3200)
            _liveTranscriptText.value = ""
        }
    }

    // Protobuf wire helpers

    private fun encodeMicAudioFrame(pcmBytes: ByteArray): ByteArray {
        val out = ByteArrayOutputStream(pcmBytes.size + 5)
        out.write(0x12) // field 2, length-delimited
        writeVarint(pcmBytes.size.toLong(), out)
        out.write(pcmBytes)
        return out.toByteArray()
    }

    private fun sendSettingsUpdate(planModel: Int?, playbackSpeed: Double?) {
        if (!isSocketReady) return
        val inner = ByteArrayOutputStream()
        if (planModel != null && planModel > 0) {
            inner.write(0x08) // field 1, varint
            writeVarint(planModel.toLong(), inner)
        }
        if (playbackSpeed != null) {
            inner.write(0x11) // field 2, 64-bit double
            val buf = ByteBuffer.allocate(8).order(ByteOrder.LITTLE_ENDIAN)
            buf.putDouble(playbackSpeed)
            inner.write(buf.array())
        }
        val innerBytes = inner.toByteArray()
        if (innerBytes.isEmpty()) return

        val outer = ByteArrayOutputStream(innerBytes.size + 4)
        outer.write(0x1A) // field 3, length-delimited
        writeVarint(innerBytes.size.toLong(), outer)
        outer.write(innerBytes)
        webSocket?.send(outer.toByteArray().toByteString())
    }

    private fun writeVarint(value: Long, out: ByteArrayOutputStream) {
        var v = value
        while ((v and -0x80L) != 0L) {
            out.write(((v.toInt() and 0x7F) or 0x80))
            v = v ushr 7
        }
        out.write(v.toInt() and 0x7F)
    }

    private fun handleServerProtobufFrame(bytes: ByteArray) {
        val idx = intArrayOf(0)
        while (idx[0] < bytes.size) {
            val tag = readVarint(bytes, idx) ?: break
            val fieldNumber = (tag ushr 3).toInt()
            val wireType = (tag and 0x07L).toInt()

            when (wireType) {
                0 -> readVarint(bytes, idx)
                1 -> idx[0] += 8
                5 -> idx[0] += 4
                2 -> {
                    val len = readVarint(bytes, idx)?.toInt() ?: return
                    if (len < 0 || idx[0] + len > bytes.size) return
                    val slice = bytes.copyOfRange(idx[0], idx[0] + len)
                    idx[0] += len

                    when (fieldNumber) {
                        1 -> enqueuePcm24k(slice)
                        2 -> handleSessionStatus(slice)
                        7 -> extractFirstStringField(slice)?.let {
                            updateLiveTranscript(it, VoiceTranscriptRole.AI)
                        }
                        8 -> extractFirstStringField(slice)?.let {
                            updateLiveTranscript(it, VoiceTranscriptRole.USER)
                        }
                        9 -> clearPlaybackQueue()
                    }
                }
                else -> return
            }
        }
    }

    private fun handleSessionStatus(bytes: ByteArray) {
        val idx = intArrayOf(0)
        var stateVal = 0
        var returnedCascadeId = ""

        while (idx[0] < bytes.size) {
            val tag = readVarint(bytes, idx) ?: break
            val field = (tag ushr 3).toInt()
            val wt = (tag and 0x07L).toInt()
            when (wt) {
                0 -> {
                    val v = readVarint(bytes, idx)?.toInt() ?: return
                    if (field == 1) stateVal = v
                }
                1 -> idx[0] += 8
                5 -> idx[0] += 4
                2 -> {
                    val len = readVarint(bytes, idx)?.toInt() ?: return
                    if (len < 0 || idx[0] + len > bytes.size) return
                    val slice = bytes.copyOfRange(idx[0], idx[0] + len)
                    idx[0] += len
                    if (field == 6) {
                        returnedCascadeId = slice.toString(Charsets.UTF_8)
                    }
                }
                else -> return
            }
        }

        if (returnedCascadeId.isNotEmpty() && returnedCascadeId != activeCascadeId) {
            activeCascadeId = returnedCascadeId
            scope.launch { onCascadeBound?.invoke(returnedCascadeId) }
        }

        when (stateVal) {
            1, 2 -> _callState.value = VoiceCallState.LISTENING
            3 -> _callState.value = VoiceCallState.RECONNECTING
            4, 5 -> scope.launch { endCall() }
        }
    }

    private fun extractFirstStringField(bytes: ByteArray): String? {
        val idx = intArrayOf(0)
        while (idx[0] < bytes.size) {
            val tag = readVarint(bytes, idx) ?: break
            val field = (tag ushr 3).toInt()
            val wt = (tag and 0x07L).toInt()
            when (wt) {
                0 -> readVarint(bytes, idx)
                1 -> idx[0] += 8
                5 -> idx[0] += 4
                2 -> {
                    val len = readVarint(bytes, idx)?.toInt() ?: return null
                    if (len < 0 || idx[0] + len > bytes.size) return null
                    if (field == 1) {
                        return bytes.copyOfRange(idx[0], idx[0] + len).toString(Charsets.UTF_8)
                    }
                    idx[0] += len
                }
                else -> return null
            }
        }
        return null
    }

    private fun readVarint(bytes: ByteArray, idx: IntArray): Long? {
        var result = 0L
        var shift = 0
        while (idx[0] < bytes.size) {
            val b = bytes[idx[0]].toInt() and 0xFF
            idx[0]++
            result = result or ((b and 0x7F).toLong() shl shift)
            if ((b and 0x80) == 0) return result
            shift += 7
            if (shift >= 64) return null
        }
        return null
    }
}
