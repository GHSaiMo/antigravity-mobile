package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.PairingInfo
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

import android.util.Log

sealed interface PairingUiState {
    data object Idle : PairingUiState
    data class Pairing(val message: String) : PairingUiState
    data object Success : PairingUiState
    data class Error(val message: String) : PairingUiState
}

class PairingViewModel(
    private val apiClient: ApiClient,
    private val prefs: PreferencesManager,
    private var onPreheatData: (suspend () -> Unit)? = null
) : ViewModel() {

    private val _uiState = MutableStateFlow<PairingUiState>(
        if (prefs.isPaired()) PairingUiState.Success else PairingUiState.Idle
    )
    val uiState: StateFlow<PairingUiState> = _uiState.asStateFlow()

    fun setPreheatAction(action: suspend () -> Unit) {
        onPreheatData = action
    }

    fun pairWithUri(uriString: String) {
        val info = PairingInfo.parseFromUri(uriString)
        if (info == null) {
            _uiState.value = PairingUiState.Error("无法识别配对链接，请扫描正确的 agy://pair 二维码")
            return
        }

        executePairing(info)
    }

    fun pairWithHostAndCode(host: String, port: Int, code: String, ssl: Boolean) {
        val info = PairingInfo(
            host = host.trim(),
            port = port,
            code = code.trim(),
            ssl = ssl
        )
        executePairing(info)
    }

    private fun executePairing(info: PairingInfo) {
        _uiState.value = PairingUiState.Pairing("正在向网关验证并签发设备证书...")
        viewModelScope.launch {
            val result = apiClient.pair(info)
            result.onSuccess {
                _uiState.value = PairingUiState.Pairing("正在同步会话与工作区...")
                try {
                    onPreheatData?.invoke()
                } catch (e: Exception) {
                    Log.w("PairingViewModel", "Failed to preheat data: ${e.message}")
                }
                _uiState.value = PairingUiState.Success
            }.onFailure { err ->
                _uiState.value = PairingUiState.Error(err.message ?: "配对失败，请检查网络和网关")
            }
        }
    }

    fun resetState() {
        _uiState.value = PairingUiState.Idle
    }
}
