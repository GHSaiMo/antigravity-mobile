package com.antigravity.mobile.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf

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

    CompositionLocalProvider(LocalAppColors provides appColors) {
        MaterialTheme(
            colorScheme = materialColors,
            typography = Typography,
            content = content
        )
    }
}
