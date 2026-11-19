package com.antigravity.mobile.ui.util

import com.antigravity.mobile.ui.util.loc
import android.annotation.SuppressLint
import android.content.Context
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaPlayer
import android.media.MediaRecorder
import android.speech.tts.TextToSpeech
import android.util.Log
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.synthesizeNarrationAudio
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString.Companion.toByteString
import org.json.JSONObject
import java.io.File
import java.net.URLEncoder
import java.util.Locale

class AudioDictationManager(private val apiClient: ApiClient) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var audioRecord: AudioRecord? = null
    private var captureJob: Job? = null
    private var webSocket: WebSocket? = null

    private val _isDictating = MutableStateFlow(false)
    val isDictating: StateFlow<Boolean> = _isDictating.asStateFlow()

    private var committedText: String = ""
    private var currentInterim: String = ""

    @SuppressLint("MissingPermission")
    fun start(
        cascadeId: String,
        initialText: String = "",
        onTranscriptUpdate: (String) -> Unit,
        onError: (String) -> Unit
    ) {
        if (_isDictating.value) return
        committedText = initialText.trim()
        currentInterim = ""
        val base = apiClient.currentBaseUrl ?: run {
            onError("未配置网关地址".loc)
            return
        }
        val wsBase = base.replaceFirst("https://", "wss://").replaceFirst("http://", "ws://").trimEnd('/')
        val wsUrl = "$wsBase/gateway/audio/dictate?cascadeId=${URLEncoder.encode(cascadeId, "UTF-8")}&mimeType=${URLEncoder.encode("audio/pcm;rate=16000", "UTF-8")}"

        val sampleRate = 16000
        val minBuf = AudioRecord.getMinBufferSize(
            sampleRate,
            AudioFormat.CHANNEL_IN_MONO,
            AudioFormat.ENCODING_PCM_16BIT
        ).coerceAtLeast(3200)

        val record = try {
            AudioRecord(
                MediaRecorder.AudioSource.MIC,
                sampleRate,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT,
                minBuf * 2
            )
        } catch (e: Exception) {
            onError("无法访问麦克风: ${e.message}".loc)
            return
        }

        if (record.state != AudioRecord.STATE_INITIALIZED) {
            record.release()
            onError("麦克风初始化失败".loc)
            return
        }

        _isDictating.value = true
        audioRecord = record

        val req = apiClient.buildAuthorizedRequest(wsUrl).build()
        webSocket = apiClient.okHttpClient.newWebSocket(req, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                try {
                    val obj = JSONObject(text)
                    val err = obj.optString("error")
                    if (err.isNotBlank()) {
                        onError(err)
                        stop()
                        return
                    }
                    val t = obj.optString("text")
                        .ifBlank { obj.optString("transcript") }
                        .ifBlank { obj.optJSONObject("transcription")?.optString("text").orEmpty() }
                        .ifBlank { obj.optJSONObject("update")?.optString("text").orEmpty() }
                    val isFinal = obj.optBoolean("isFinal", false) ||
                        obj.optJSONObject("transcription")?.optBoolean("isFinal", false) ?: false
                    if (t.isNotBlank()) {
                        if (isFinal) {
                            committedText = joinText(committedText, t)
                            currentInterim = ""
                            onTranscriptUpdate(committedText)
                        } else {
                            currentInterim = t
                            val merged = joinText(committedText, currentInterim)
                            onTranscriptUpdate(merged)
                        }
                    }
                } catch (_: Exception) {}
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.w("Dictation", "WS failed: ${t.message}")
                if (_isDictating.value) {
                    onError("语音识别连接中断".loc)
                    stop()
                }
            }
        })

        try {
            record.startRecording()
        } catch (e: Exception) {
            onError("启动录音失败: ${e.message}".loc)
            stop()
            return
        }

        captureJob = scope.launch {
            val buf = ByteArray(3200)
            while (isActive && _isDictating.value) {
                val read = record.read(buf, 0, buf.size)
                if (read > 0) {
                    webSocket?.send(buf.toByteString(0, read))
                }
            }
        }
    }

    fun stop() {
        if (!_isDictating.value && audioRecord == null) return
        _isDictating.value = false
        captureJob?.cancel()
        captureJob = null
        try {
            audioRecord?.stop()
        } catch (_: Exception) {}
        try {
            audioRecord?.release()
        } catch (_: Exception) {}
        audioRecord = null
        try {
            webSocket?.send("""{"type":"end"}""")
            webSocket?.close(1000, "done")
        } catch (_: Exception) {}
        webSocket = null
    }

    private fun joinText(base: String, append: String): String {
        val b = base.trim()
        val a = append.trim()
        if (b.isEmpty()) return a
        if (a.isEmpty()) return b
        if (base.endsWith(" ") || base.endsWith("\n") || append.startsWith(" ")) return "$base$append"
        val lastB = b.last()
        val firstA = a.first()
        return if (isCJK(lastB) || isCJK(firstA)) "$b$a" else "$b $a"
    }

    private fun isCJK(c: Char): Boolean {
        val code = c.code
        return (code in 0x4E00..0x9FFF) || (code in 0x3400..0x4DBF) || (code in 0x3000..0x303F) || (code in 0xFF00..0xFFEF)
    }
}

class NarrationAudioPlayer(private val context: Context, private val apiClient: ApiClient) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var mediaPlayer: MediaPlayer? = null
    private var tts: TextToSpeech? = null
    private var playJob: Job? = null

    private val _isPlaying = MutableStateFlow(false)
    val isPlaying: StateFlow<Boolean> = _isPlaying.asStateFlow()

    private val _statusLabel = MutableStateFlow("")
    val statusLabel: StateFlow<String> = _statusLabel.asStateFlow()

    fun play(text: String) {
        val cleaned = text.trim()
        if (cleaned.isEmpty()) return
        stop()
        _isPlaying.value = true
        _statusLabel.value = "正在合成语音…".loc
        playJob = scope.launch {
            val res = apiClient.synthesizeNarrationAudio(cleaned)
            res.onSuccess { bytes ->
                try {
                    val tmpFile = File.createTempFile("narration_", ".m4a", context.cacheDir)
                    tmpFile.writeBytes(bytes)
                    val mp = MediaPlayer().apply {
                        setDataSource(tmpFile.absolutePath)
                        setOnCompletionListener {
                            tmpFile.delete()
                            stop()
                        }
                        setOnErrorListener { _, _, _ ->
                            tmpFile.delete()
                            speakWithSystemTts(cleaned)
                            true
                        }
                        prepare()
                        start()
                    }
                    mediaPlayer = mp
                    _statusLabel.value = "语音播报中".loc
                } catch (_: Exception) {
                    speakWithSystemTts(cleaned)
                }
            }.onFailure {
                speakWithSystemTts(cleaned)
            }
        }
    }

    private fun speakWithSystemTts(text: String) {
        _statusLabel.value = "系统朗读中".loc
        var engine: TextToSpeech? = null
        engine = TextToSpeech(context) { status ->
            if (status == TextToSpeech.SUCCESS) {
                engine?.language = Locale.SIMPLIFIED_CHINESE
                engine?.speak(text, TextToSpeech.QUEUE_FLUSH, null, "narration")
            } else {
                stop()
            }
        }
        tts = engine
    }

    fun stop() {
        playJob?.cancel()
        playJob = null
        try {
            mediaPlayer?.stop()
        } catch (_: Exception) {}
        try {
            mediaPlayer?.release()
        } catch (_: Exception) {}
        mediaPlayer = null
        try {
            tts?.stop()
            tts?.shutdown()
        } catch (_: Exception) {}
        tts = null
        _isPlaying.value = false
        _statusLabel.value = ""
    }
}
