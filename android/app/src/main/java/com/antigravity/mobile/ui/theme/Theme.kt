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

private fun buildMaterialScheme(appColors: AppColors): androidx.compose.material3.ColorScheme {
    return if (appColors.isDark) {
        darkColorScheme(
            primary = appColors.accentIndigo,
            onPrimary = appColors.userBubbleText,
            secondary = appColors.accentGreen,
            error = appColors.accentRed,
            background = appColors.background,
            onBackground = appColors.textPrimary,
            surface = appColors.surface,
            onSurface = appColors.textPrimary,
            surfaceVariant = appColors.surfaceVariant,
            onSurfaceVariant = appColors.textSecondary,
            surfaceContainer = appColors.surfaceVariant,
            surfaceContainerLow = appColors.background,
            surfaceContainerHigh = appColors.surface,
            outline = appColors.border
        )
    } else {
        lightColorScheme(
            primary = appColors.accentIndigo,
            onPrimary = appColors.userBubbleText,
            secondary = appColors.accentGreen,
            error = appColors.accentRed,
            background = appColors.background,
            onBackground = appColors.textPrimary,
            surface = appColors.surface,
            onSurface = appColors.textPrimary,
            surfaceVariant = appColors.surfaceVariant,
            onSurfaceVariant = appColors.textSecondary,
            surfaceContainer = appColors.surface,
            surfaceContainerLow = appColors.background,
            surfaceContainerHigh = appColors.surface,
            outline = appColors.border
        )
    }
}

@Composable
fun AntigravityTheme(
    themeMode: String = "system",
    lightPresetId: String = "default",
    darkPresetId: String = "default",
    content: @Composable () -> Unit
) {
    val isSystemDark = isSystemInDarkTheme()
    val normMode = themeMode.lowercase()
    val isLegacyNord = normMode == "nord"
    val isDark = when (normMode) {
        "light" -> false
        "dark", "nord" -> true
        else -> isSystemDark
    }

    val effectiveDarkPreset = if (isLegacyNord) "nord" else darkPresetId
    val preset = ThemePresetCatalog.resolve(
        isDark = isDark,
        lightPresetId = lightPresetId,
        darkPresetId = effectiveDarkPreset
    )
    val appColors = preset.appColors
    val materialColors = when (preset.appColors.themeId) {
        "default-light" -> LightMaterialColorScheme
        "default-dark" -> DarkMaterialColorScheme
        "nord" -> NordMaterialColorScheme
        else -> buildMaterialScheme(appColors)
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
