package com.antigravity.mobile.ui.util

import android.content.Context
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback

class HapticManager(
    private val hapticFeedback: HapticFeedback,
    private val context: Context
) {
    private val vibrator: Vibrator? by lazy {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                val vibratorManager = context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager
                vibratorManager?.defaultVibrator
            } else {
                @Suppress("DEPRECATION")
                context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
            }
        } catch (_: Exception) {
            null
        }
    }

    /**
     * Light tick — equivalent to iOS UIImpactFeedbackGenerator(style: .light)
     */
    fun light() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && vibrator?.hasVibrator() == true) {
                vibrator?.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_TICK))
            } else {
                hapticFeedback.performHapticFeedback(HapticFeedbackType.TextHandleMove)
            }
        } catch (_: Exception) {
            hapticFeedback.performHapticFeedback(HapticFeedbackType.TextHandleMove)
        }
    }

    /**
     * Medium click — equivalent to iOS UIImpactFeedbackGenerator(style: .medium)
     */
    fun medium() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && vibrator?.hasVibrator() == true) {
                vibrator?.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_CLICK))
            } else {
                hapticFeedback.performHapticFeedback(HapticFeedbackType.LongPress)
            }
        } catch (_: Exception) {
            hapticFeedback.performHapticFeedback(HapticFeedbackType.LongPress)
        }
    }

    /**
     * Heavy confirmation feedback
     */
    fun heavy() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && vibrator?.hasVibrator() == true) {
                vibrator?.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_HEAVY_CLICK))
            } else {
                hapticFeedback.performHapticFeedback(HapticFeedbackType.LongPress)
            }
        } catch (_: Exception) {
            hapticFeedback.performHapticFeedback(HapticFeedbackType.LongPress)
        }
    }

    /**
     * Long press tactile feedback
     */
    fun longPress() {
        hapticFeedback.performHapticFeedback(HapticFeedbackType.LongPress)
    }
}

@Composable
fun rememberHaptic(): HapticManager {
    val hapticFeedback = LocalHapticFeedback.current
    val context = LocalContext.current.applicationContext
    return remember(hapticFeedback, context) {
        HapticManager(hapticFeedback, context)
    }
}

object HapticUtils {
    fun lightTap(context: Context) {
        try {
            val vibrator = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                val vm = context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager
                vm?.defaultVibrator
            } else {
                @Suppress("DEPRECATION")
                context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && vibrator?.hasVibrator() == true) {
                vibrator.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_TICK))
            }
        } catch (_: Exception) {}
    }

    fun heavyClick(context: Context) {
        try {
            val vibrator = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                val vm = context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager
                vm?.defaultVibrator
            } else {
                @Suppress("DEPRECATION")
                context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && vibrator?.hasVibrator() == true) {
                vibrator.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_HEAVY_CLICK))
            }
        } catch (_: Exception) {}
    }
}
