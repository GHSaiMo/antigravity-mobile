package com.antigravity.mobile.ui.util

import android.content.Context
import android.media.MediaRecorder
import android.net.Uri
import android.os.Build
import android.util.Log
import androidx.core.content.FileProvider
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class AudioRecorderManager(private val context: Context) {
    private var mediaRecorder: MediaRecorder? = null
    private var currentFile: File? = null
    private var timerJob: Job? = null
    private val scope = CoroutineScope(Dispatchers.Main)

    private val _isRecording = MutableStateFlow(false)
    val isRecording: StateFlow<Boolean> = _isRecording.asStateFlow()

    private val _isPaused = MutableStateFlow(false)
    val isPaused: StateFlow<Boolean> = _isPaused.asStateFlow()

    private val _durationSeconds = MutableStateFlow(0)
    val durationSeconds: StateFlow<Int> = _durationSeconds.asStateFlow()

    private val _soundLevels = MutableStateFlow(List(16) { 0.12f })
    val soundLevels: StateFlow<List<Float>> = _soundLevels.asStateFlow()

    fun startRecording(): Boolean {
        return try {
            val dir = File(context.cacheDir, "audio_records").apply { mkdirs() }
            val timeStamp = SimpleDateFormat("yyyyMMdd_HHmmss", Locale.getDefault()).format(Date())
            val file = File(dir, "录音_${timeStamp}.m4a")
            currentFile = file

            val recorder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                MediaRecorder(context)
            } else {
                @Suppress("DEPRECATION")
                MediaRecorder()
            }

            recorder.apply {
                setAudioSource(MediaRecorder.AudioSource.MIC)
                setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
                setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
                setAudioSamplingRate(44100)
                setAudioEncodingBitRate(64000)
                setAudioChannels(1)
                setOutputFile(file.absolutePath)
                prepare()
                start()
            }

            mediaRecorder = recorder
            _isRecording.value = true
            _isPaused.value = false
            _durationSeconds.value = 0
            _soundLevels.value = List(16) { 0.12f }
            startMonitoring()
            true
        } catch (e: Exception) {
            Log.e("AudioRecorderManager", "Failed to start recording", e)
            cleanup()
            false
        }
    }

    private fun startMonitoring() {
        timerJob?.cancel()
        timerJob = scope.launch {
            var counter = 0
            while (isActive && _isRecording.value) {
                delay(50)
                if (!_isPaused.value) {
                    counter++
                    if (counter % 20 == 0) {
                        _durationSeconds.value += 1
                    }
                    val recorder = mediaRecorder
                    val amp = try {
                        recorder?.maxAmplitude ?: 0
                    } catch (_: Exception) { 0 }
                    val normalized = (amp / 25000f).coerceIn(0.12f, 1.0f)
                    val currentList = _soundLevels.value.toMutableList()
                    if (currentList.isNotEmpty()) {
                        currentList.removeAt(0)
                        currentList.add(normalized)
                        _soundLevels.value = currentList
                    }
                }
            }
        }
    }

    fun togglePause() {
        val recorder = mediaRecorder ?: return
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            try {
                if (_isPaused.value) {
                    recorder.resume()
                    _isPaused.value = false
                } else {
                    recorder.pause()
                    _isPaused.value = true
                }
            } catch (e: Exception) {
                Log.e("AudioRecorderManager", "Failed to toggle pause", e)
            }
        }
    }

    fun stopRecording(): Uri? {
        timerJob?.cancel()
        timerJob = null
        try {
            mediaRecorder?.apply {
                stop()
                release()
            }
        } catch (e: Exception) {
            Log.e("AudioRecorderManager", "Error stopping recorder", e)
        }
        mediaRecorder = null
        _isRecording.value = false
        _isPaused.value = false

        val file = currentFile ?: return null
        return try {
            FileProvider.getUriForFile(
                context,
                "${context.packageName}.fileprovider",
                file
            )
        } catch (e: Exception) {
            Log.e("AudioRecorderManager", "Failed to get URI for recorded file", e)
            Uri.fromFile(file)
        }
    }

    fun cancelRecording() {
        cleanup()
    }

    private fun cleanup() {
        timerJob?.cancel()
        timerJob = null
        try {
            mediaRecorder?.apply {
                stop()
                release()
            }
        } catch (_: Exception) {}
        mediaRecorder = null
        _isRecording.value = false
        _isPaused.value = false
        currentFile?.delete()
        currentFile = null
    }
}
