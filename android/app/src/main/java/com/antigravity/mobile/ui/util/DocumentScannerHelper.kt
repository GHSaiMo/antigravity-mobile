package com.antigravity.mobile.ui.util

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability

object DocumentScannerHelper {
    /**
     * Checks if Google Play Services is available on the device and ready to support
     * the ML Kit Document Scanner API.
     */
    fun isSupported(context: Context): Boolean {
        return try {
            val availability = GoogleApiAvailability.getInstance()
            availability.isGooglePlayServicesAvailable(context) == ConnectionResult.SUCCESS
        } catch (_: Throwable) {
            false
        }
    }

    /**
     * Recursively traverses context wrappers to locate the hosting Activity.
     */
    fun findActivity(context: Context): Activity? {
        var current = context
        while (current is ContextWrapper) {
            if (current is Activity) return current
            current = current.baseContext
        }
        return null
    }
}
