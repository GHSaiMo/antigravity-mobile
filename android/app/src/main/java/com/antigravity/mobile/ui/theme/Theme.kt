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
    outline = DarkBorder
)

@Composable
fun AntigravityTheme(
    themeMode: String = "system",
    content: @Composable () -> Unit
) {
    val isSystemDark = isSystemInDarkTheme()
    val isDark = when (themeMode.lowercase()) {
        "light" -> false
        "dark" -> true
        else -> isSystemDark
    }

    val appColors = if (isDark) DarkAppColors else LightAppColors
    val materialColors = if (isDark) DarkMaterialColorScheme else LightMaterialColorScheme

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
