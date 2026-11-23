package com.antigravity.mobile.data.service

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Google Play flavor no-op stub.
 *
 * Updates for the `play` flavor are managed exclusively by Google Play Store.
 * Keeping this implementation completely free of APK download / package installation APIs
 * ensures full compliance with Google Play's Device and Network Abuse policy.
 */
object AppUpdateInstaller {
    const val isSupported: Boolean = false

    private val _state = MutableStateFlow<AppUpdateState>(AppUpdateState.Idle)
    val state: StateFlow<AppUpdateState> = _state.asStateFlow()

    fun cleanupOldApks(context: Context) = Unit

    fun checkInBackgroundIfNeeded(context: Context) = Unit

    fun checkNow(
        context: Context,
        silent: Boolean = false,
        onComplete: ((AppUpdateState) -> Unit)? = null
    ) {
        onComplete?.invoke(AppUpdateState.Idle)
    }

    fun downloadAndInstall(context: Context, release: AppReleaseInfo) = Unit

    fun cancelDownload() = Unit

    fun openInstallPermissionSettings(context: Context) = Unit

    fun resumePendingInstall(context: Context): Boolean = false

    fun dismissError() = Unit
}
