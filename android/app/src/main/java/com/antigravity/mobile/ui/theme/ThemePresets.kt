package com.antigravity.mobile.ui.theme

import com.antigravity.mobile.ui.util.loc
import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp

@Immutable
data class ThemeSyntaxPalette(
    val keyword: Color,
    val string: Color,
    val comment: Color,
    val number: Color,
    val typeName: Color,
    val function: Color,
    val property: Color,
    val diffAdd: Color,
    val diffDel: Color
)

@Immutable
data class ThemePreset(
    val id: String,
    val name: String,
    val description: String,
    val isDark: Boolean,
    val isDefaultGroup: Boolean,
    val bg: Color,
    val fg: Color,
    val accent: Color,
    val accentFg: Color,
    val syntax: ThemeSyntaxPalette,
    val appColors: AppColors
)

/**
 * 精简后统一复用的长图分享皮肤目录（2 款原生基础风格 + 5 款精选桌面端配色，共 7 款）：
 * 消息长图分享与 Token 历史报告长图共同复用该目录与选定皮肤。
 */
object ThemePresetCatalog {

    private val appleLightSyntax = ThemeSyntaxPalette(
        keyword = Color(0xFFCF222E),
        string = Color(0xFF0A3069),
        comment = Color(0xFF6E7781),
        number = Color(0xFF0550AE),
        typeName = Color(0xFF953800),
        function = Color(0xFF8250DF),
        property = Color(0xFF116329),
        diffAdd = Color(0xFF1A7F37),
        diffDel = Color(0xFFCF222E)
    )

    private val appleDarkSyntax = ThemeSyntaxPalette(
        keyword = Color(0xFFFF7B72),
        string = Color(0xFFA5D6FF),
        comment = Color(0xFF8B949E),
        number = Color(0xFF79C0FF),
        typeName = Color(0xFFFFA657),
        function = Color(0xFFD2A8FF),
        property = Color(0xFF7EE787),
        diffAdd = Color(0xFF56D364),
        diffDel = Color(0xFFF85149)
    )

    fun appleSyntax(isDark: Boolean): ThemeSyntaxPalette =
        if (isDark) appleDarkSyntax else appleLightSyntax

    private fun buildAppColors(
        id: String,
        isDark: Boolean,
        bg: Color,
        fg: Color,
        accent: Color,
        accentFg: Color,
        syntax: ThemeSyntaxPalette
    ): AppColors {
        val surface = if (isDark) lerp(bg, Color.White, 0.065f) else lerp(bg, Color.White, 0.72f)
        val surfaceVariant = if (isDark) lerp(bg, Color.White, 0.12f) else lerp(bg, fg, 0.08f)
        val tertiary = if (isDark) lerp(bg, Color.White, 0.16f) else lerp(bg, fg, 0.05f)
        val border = lerp(bg, fg, if (isDark) 0.22f else 0.16f)
        val separator = lerp(bg, fg, if (isDark) 0.26f else 0.18f)
        val secondaryLabel = lerp(bg, fg, 0.72f)
        val mutedLabel = lerp(bg, fg, 0.48f)
        val codeBg = if (isDark) lerp(bg, Color.Black, 0.16f) else lerp(bg, fg, 0.055f)
        val searchBg = if (isDark) surface else lerp(bg, fg, 0.07f)
        val inputBg = if (isDark) lerp(bg, Color.Black, 0.12f) else surface

        return AppColors(
            isDark = isDark,
            background = bg,
            surface = surface,
            surfaceVariant = surfaceVariant,
            tertiaryBackground = tertiary,
            border = border,
            separator = separator,
            textPrimary = fg,
            textSecondary = secondaryLabel,
            textMuted = mutedLabel,
            userBubbleBg = accent,
            userBubbleText = accentFg,
            agentBubbleBg = surface,
            agentBubbleText = fg,
            accentBlue = accent,
            accentIndigo = accent,
            accentGreen = syntax.diffAdd,
            accentOrange = syntax.number,
            accentRed = syntax.diffDel,
            accentYellow = syntax.typeName,
            accentPurple = syntax.keyword,
            accentOrchestrator = if (isDark) lerp(bg, fg, 0.75f) else lerp(bg, fg, 0.65f),
            codeBlockBg = codeBg,
            searchBarBg = searchBg,
            inputBarBg = inputBg,
            cardShadow = if (isDark) Color(0x40000000) else Color(0x0D000000),
            useNord = id == "nord",
            themeId = id
        )
    }

    private fun makePreset(
        id: String,
        name: String,
        description: String,
        isDark: Boolean,
        isDefaultGroup: Boolean,
        bg: Color,
        fg: Color,
        accent: Color,
        accentFg: Color = Color.White,
        syntax: ThemeSyntaxPalette,
        colorsOverride: AppColors? = null
    ): ThemePreset = ThemePreset(
        id = id,
        name = name,
        description = description,
        isDark = isDark,
        isDefaultGroup = isDefaultGroup,
        bg = bg,
        fg = fg,
        accent = accent,
        accentFg = accentFg,
        syntax = syntax,
        appColors = colorsOverride ?: buildAppColors(id, isDark, bg, fg, accent, accentFg, syntax)
    )

    val defaultDark: ThemePreset = makePreset(
        id = "default-dark",
        name = "深色".loc,
        description = "经典深邃紫蓝渐变".loc,
        isDark = true,
        isDefaultGroup = true,
        bg = Color(0xFF0F121C),
        fg = Color.White,
        accent = DarkAppleIndigo,
        accentFg = Color.White,
        syntax = appleDarkSyntax,
        colorsOverride = DarkAppColors.copy(themeId = "default-dark")
    )

    val defaultLight: ThemePreset = makePreset(
        id = "default-light",
        name = "浅色".loc,
        description = "极简暖白纸感排版".loc,
        isDark = false,
        isDefaultGroup = true,
        bg = Color(0xFFFCFCFA),
        fg = LightLabel,
        accent = LightAppleIndigo,
        accentFg = Color.White,
        syntax = appleLightSyntax,
        colorsOverride = LightAppColors.copy(themeId = "default-light")
    )

    val defaultSkins: List<ThemePreset> = listOf(defaultDark, defaultLight)

    val desktopSkins: List<ThemePreset> = listOf(
        makePreset(
            id = "solarized-light",
            name = "Solarized Light",
            description = "经典低眩光暖沙色".loc,
            isDark = false,
            isDefaultGroup = false,
            bg = Color(0xFFFAF4E5),
            fg = Color(0xFF435155),
            accent = Color(0xFFCB4B16),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFF859900),
                string = Color(0xFF2AA198),
                comment = Color(0xFF93A1A1),
                number = Color(0xFFD33682),
                typeName = Color(0xFFB58900),
                function = Color(0xFF268BD2),
                property = Color(0xFF268BD2),
                diffAdd = Color(0xFF859900),
                diffDel = Color(0xFFDC322F)
            )
        ),
        makePreset(
            id = "nord",
            name = "Nord",
            description = "极地冰蓝冷调暗色".loc,
            isDark = true,
            isDefaultGroup = false,
            bg = NordBackground,
            fg = NordForeground,
            accent = NordFrostAccent,
            accentFg = NordBackground,
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFF81A1C1),
                string = Color(0xFFA3BE8C),
                comment = Color(0xFF616E88),
                number = Color(0xFFB48EAD),
                typeName = Color(0xFF8FBCBB),
                function = Color(0xFF88C0D0),
                property = Color(0xFF8FBCBB),
                diffAdd = Color(0xFFA3BE8C),
                diffDel = Color(0xFFBF616A)
            ),
            colorsOverride = NordAppColors.copy(themeId = "nord")
        ),
        makePreset(
            id = "tokyo-night",
            name = "Tokyo Night",
            description = "东京夜色深靛蓝与霓虹蓝".loc,
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF1A1B26),
            fg = Color(0xFFA9B1D6),
            accent = Color(0xFF7AA2F7),
            accentFg = Color(0xFF1A1B26),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFBB9AF7),
                string = Color(0xFF9ECE6A),
                comment = Color(0xFF565F89),
                number = Color(0xFFFF9E64),
                typeName = Color(0xFF2AC3DE),
                function = Color(0xFF7AA2F7),
                property = Color(0xFF7AA2F7),
                diffAdd = Color(0xFF9ECE6A),
                diffDel = Color(0xFFF7768E)
            )
        ),
        makePreset(
            id = "catppuccin-dark",
            name = "Catppuccin",
            description = "Macchiato 柔和粉紫暮色".loc,
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF24273A),
            fg = Color(0xFFCAD3F5),
            accent = Color(0xFFC6A0F6),
            accentFg = Color(0xFF24273A),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFC6A0F6),
                string = Color(0xFFA6DA95),
                comment = Color(0xFF939AB7),
                number = Color(0xFFF5A97F),
                typeName = Color(0xFFEED49F),
                function = Color(0xFF8AADF4),
                property = Color(0xFF8AADF4),
                diffAdd = Color(0xFFA6DA95),
                diffDel = Color(0xFFED8796)
            )
        ),
        makePreset(
            id = "everforest-dark",
            name = "Everforest",
            description = "松林暗绿与柔光暖字".loc,
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF2D353B),
            fg = Color(0xFFD3C6AA),
            accent = Color(0xFFA7C080),
            accentFg = Color(0xFF232A2E),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFE67E80),
                string = Color(0xFFA7C080),
                comment = Color(0xFF9AA69D),
                number = Color(0xFFD699B6),
                typeName = Color(0xFFDBBC7F),
                function = Color(0xFF83C092),
                property = Color(0xFF7FBBB3),
                diffAdd = Color(0xFFA7C080),
                diffDel = Color(0xFFE67E80)
            )
        )
    )

    val curatedSkins: List<ThemePreset> = defaultSkins + desktopSkins

    fun findByThemeId(themeId: String, fallbackIsDark: Boolean): ThemePreset = when (themeId) {
        "default-light" -> defaultLight
        "default-dark" -> defaultDark
        else -> curatedSkins.firstOrNull { it.id == themeId }
            ?: if (fallbackIsDark) defaultDark else defaultLight
    }
}
