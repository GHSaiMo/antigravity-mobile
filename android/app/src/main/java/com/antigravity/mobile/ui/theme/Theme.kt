package com.antigravity.mobile.ui.theme

import android.app.Activity
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat

val LocalAppColors = staticCompositionLocalOf<AppColors> {
    error("No AppColors provided")
}

object AntigravityTheme {
    val colors: AppColors
        @Composable
        @ReadOnlyComposable
        get() = LocalAppColors.current
}

private val LightMaterialColorScheme = lightColorScheme(
    primary = LightAppleIndigo,
    onPrimary = LightSecondarySystemGroupedBackground,
    secondary = LightAppleGreen,
    error = LightAppleRed,
    background = LightSystemGroupedBackground,
    onBackground = LightLabel,
    surface = LightSecondarySystemGroupedBackground,
    onSurface = LightLabel,
    surfaceVariant = LightSecondarySystemBackground,
    onSurfaceVariant = LightSecondaryLabel,
    surfaceContainer = LightSecondarySystemGroupedBackground,
    surfaceContainerLow = LightSystemGroupedBackground,
    surfaceContainerHigh = LightSecondarySystemGroupedBackground,
    outline = LightBorder
)

private val DarkMaterialColorScheme = darkColorScheme(
    primary = DarkAppleIndigo,
    onPrimary = DarkLabel,
    secondary = DarkAppleGreen,
    error = DarkAppleRed,
    background = DarkSystemGroupedBackground,
    onBackground = DarkLabel,
    surface = DarkSecondarySystemGroupedBackground,
    onSurface = DarkLabel,
    surfaceVariant = DarkSecondarySystemBackground,
    onSurfaceVariant = DarkSecondaryLabel,
    surfaceContainer = DarkSecondarySystemBackground,
    surfaceContainerLow = DarkSecondarySystemGroupedBackground,
    surfaceContainerHigh = DarkSecondarySystemBackground,
    outline = DarkBorder
)

private val NordMaterialColorScheme = darkColorScheme(
    primary = NordFrostAccent,
    onPrimary = NordBackground,
    secondary = Color(0xFFA3BE8C),
    error = Color(0xFFBF616A),
    background = NordBackground,
    onBackground = NordForeground,
    surface = NordSurface,
    onSurface = NordForeground,
    surfaceVariant = NordSurfaceVariant,
    onSurfaceVariant = NordSecondaryForeground,
    surfaceContainer = NordSurfaceVariant,
    surfaceContainerLow = NordBackground,
    surfaceContainerHigh = NordSurface,
    outline = NordBorder
)

@Composable
fun AntigravityTheme(
    themeMode: String = "system",
    content: @Composable () -> Unit
) {
    val isSystemDark = isSystemInDarkTheme()
    val normMode = themeMode.lowercase()
    val isNord = normMode == "nord"
    val isDark = when (normMode) {
        "light" -> false
        "dark", "nord" -> true
        else -> isSystemDark
    }

    val appColors = when {
        isNord -> NordAppColors
        isDark -> DarkAppColors
        else -> LightAppColors
    }
    val materialColors = when {
        isNord -> NordMaterialColorScheme
        isDark -> DarkMaterialColorScheme
        else -> LightMaterialColorScheme
    }

    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as? Activity)?.window ?: return@SideEffect
            val insetsController = WindowCompat.getInsetsController(window, view)
            insetsController.isAppearanceLightStatusBars = !isDark
            insetsController.isAppearanceLightNavigationBars = !isDark
        }
    }

    CompositionLocalProvider(LocalAppColors provides appColors) {
        MaterialTheme(
            colorScheme = materialColors,
            typography = Typography,
            content = content
        )
    }
}
