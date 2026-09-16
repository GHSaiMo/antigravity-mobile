package com.antigravity.mobile.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color

// Light Palette (1:1 Apple HIG / iOS System Colors)
val LightSystemGroupedBackground = Color(0xFFF2F2F7)
val LightSecondarySystemGroupedBackground = Color(0xFFFFFFFF)
val LightSecondarySystemBackground = Color(0xFFE5E5EA)
val LightTertiarySystemBackground = Color(0xFFF2F2F7)
val LightSeparator = Color(0xFFD1D1D6)
val LightBorder = Color(0x1F000000)

val LightLabel = Color(0xFF000000)
val LightSecondaryLabel = Color(0xFF6C6C70)
val LightTertiaryLabel = Color(0xFF8E8E93)

val LightAppleIndigo = Color(0xFF5856D6)
val LightAppleBlue = Color(0xFF007AFF)
val LightAppleGreen = Color(0xFF34C759)
val LightAppleOrange = Color(0xFFFF9500)
val LightAppleRed = Color(0xFFFF3B30)
val LightAppleYellow = Color(0xFFFFCC00)
val LightApplePurple = Color(0xFFAF52DE)
val LightCodeBlockBg = Color(0xFFF6F8FA)

// Dark Palette (1:1 Apple HIG / iOS System Colors)
val DarkSystemGroupedBackground = Color(0xFF000000)
val DarkSecondarySystemGroupedBackground = Color(0xFF1C1C1E)
val DarkSecondarySystemBackground = Color(0xFF2C2C2E)
val DarkTertiarySystemBackground = Color(0xFF2C2C2E)
val DarkSeparator = Color(0xFF38383A)
val DarkBorder = Color(0x26FFFFFF)

val DarkLabel = Color(0xFFFFFFFF)
val DarkSecondaryLabel = Color(0xFF8E8E93)
val DarkTertiaryLabel = Color(0xFF636366)

val DarkAppleIndigo = Color(0xFF5E5CE6)
val DarkAppleBlue = Color(0xFF0A84FF)
val DarkAppleGreen = Color(0xFF30D158)
val DarkAppleOrange = Color(0xFFFF9F0A)
val DarkAppleRed = Color(0xFFFF453A)
val DarkAppleYellow = Color(0xFFFFD60A)
val DarkApplePurple = Color(0xFFBF5AF2)
val DarkCodeBlockBg = Color(0xFF161B22)

// Common Accent references for backward compatibility
val AccentBlue = LightAppleBlue
val AccentGreen = LightAppleGreen
val AccentRed = LightAppleRed
val AccentYellow = LightAppleOrange
val AccentPurple = LightApplePurple
val TextPrimary = LightLabel
val TextSecondary = LightSecondaryLabel
val TextMuted = LightTertiaryLabel
val UserBubbleBg = LightAppleIndigo
val AgentBubbleBg = LightSecondarySystemGroupedBackground
val DarkBackground = DarkSystemGroupedBackground
val DarkSurface = DarkSecondarySystemGroupedBackground
val DarkSurfaceVariant = DarkSecondarySystemBackground

@Immutable
data class AppColors(
    val isDark: Boolean,
    val background: Color,
    val surface: Color,
    val surfaceVariant: Color,
    val tertiaryBackground: Color,
    val border: Color,
    val separator: Color,
    val textPrimary: Color,
    val textSecondary: Color,
    val textMuted: Color,
    val userBubbleBg: Color,
    val userBubbleText: Color,
    val agentBubbleBg: Color,
    val agentBubbleText: Color,
    val accentBlue: Color,
    val accentIndigo: Color,
    val accentGreen: Color,
    val accentOrange: Color,
    val accentRed: Color,
    val accentYellow: Color,
    val accentPurple: Color,
    val codeBlockBg: Color,
    val searchBarBg: Color,
    val inputBarBg: Color,
    val cardShadow: Color
)

val LightAppColors = AppColors(
    isDark = false,
    background = LightSystemGroupedBackground,
    surface = LightSecondarySystemGroupedBackground,
    surfaceVariant = LightSecondarySystemBackground,
    tertiaryBackground = LightTertiarySystemBackground,
    border = LightBorder,
    separator = LightSeparator,
    textPrimary = LightLabel,
    textSecondary = LightSecondaryLabel,
    textMuted = LightTertiaryLabel,
    userBubbleBg = LightAppleIndigo,
    userBubbleText = Color.White,
    agentBubbleBg = LightSecondarySystemGroupedBackground,
    agentBubbleText = LightLabel,
    accentBlue = LightAppleBlue,
    accentIndigo = LightAppleIndigo,
    accentGreen = LightAppleGreen,
    accentOrange = LightAppleOrange,
    accentRed = LightAppleRed,
    accentYellow = LightAppleYellow,
    accentPurple = LightApplePurple,
    codeBlockBg = LightCodeBlockBg,
    searchBarBg = Color(0xFFE3E3E8),
    inputBarBg = LightSecondarySystemGroupedBackground,
    cardShadow = Color(0x0A000000)
)

val DarkAppColors = AppColors(
    isDark = true,
    background = DarkSystemGroupedBackground,
    surface = DarkSecondarySystemGroupedBackground,
    surfaceVariant = DarkSecondarySystemBackground,
    tertiaryBackground = DarkTertiarySystemBackground,
    border = DarkBorder,
    separator = DarkSeparator,
    textPrimary = DarkLabel,
    textSecondary = DarkSecondaryLabel,
    textMuted = DarkTertiaryLabel,
    userBubbleBg = DarkAppleIndigo,
    userBubbleText = Color.White,
    agentBubbleBg = DarkSecondarySystemGroupedBackground,
    agentBubbleText = DarkLabel,
    accentBlue = DarkAppleBlue,
    accentIndigo = DarkAppleIndigo,
    accentGreen = DarkAppleGreen,
    accentOrange = DarkAppleOrange,
    accentRed = DarkAppleRed,
    accentYellow = DarkAppleYellow,
    accentPurple = DarkApplePurple,
    codeBlockBg = DarkCodeBlockBg,
    searchBarBg = Color(0xFF1C1C1E),
    inputBarBg = Color(0xFF121212),
    cardShadow = Color(0x33000000)
)
