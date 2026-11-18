package com.antigravity.mobile.ui.theme

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

    private val desktopLightSyntax = ThemeSyntaxPalette(
        keyword = Color(0xFF0077AA),
        string = Color(0xFF5C8A00),
        comment = Color(0xFF708090),
        number = Color(0xFF990055),
        typeName = Color(0xFFDD4A68),
        function = Color(0xFFDD4A68),
        property = Color(0xFF990055),
        diffAdd = Color(0xFF5C8A00),
        diffDel = Color(0xFF990055)
    )

    private val desktopDarkSyntax = ThemeSyntaxPalette(
        keyword = Color(0xFF66D9EF),
        string = Color(0xFFA6E22E),
        comment = Color(0xFF8292A2),
        number = Color(0xFFAE81FF),
        typeName = Color(0xFFE6DB74),
        function = Color(0xFFE6DB74),
        property = Color(0xFFF92672),
        diffAdd = Color(0xFFA6E22E),
        diffDel = Color(0xFFF92672)
    )

    private fun buildAppColors(
        id: String,
        isDark: Boolean,
        bg: Color,
        fg: Color,
        accent: Color,
        accentFg: Color,
        syntax: ThemeSyntaxPalette,
        codeBlockBgOverride: Color? = null
    ): AppColors {
        val surface = if (isDark) lerp(bg, Color.White, 0.065f) else lerp(bg, Color.White, 0.72f)
        val surfaceVariant = if (isDark) lerp(bg, Color.White, 0.12f) else lerp(bg, fg, 0.08f)
        val tertiary = if (isDark) lerp(bg, Color.White, 0.16f) else lerp(bg, fg, 0.05f)
        val border = lerp(bg, fg, if (isDark) 0.22f else 0.16f)
        val separator = lerp(bg, fg, if (isDark) 0.26f else 0.18f)
        val secondaryLabel = lerp(bg, fg, 0.72f)
        val mutedLabel = lerp(bg, fg, 0.48f)
        val codeBg = codeBlockBgOverride ?: if (isDark) lerp(bg, Color.Black, 0.16f) else lerp(bg, fg, 0.055f)
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

    val lightPresets: List<ThemePreset> = listOf(
        makePreset(
            id = "default",
            name = "Apple 默认",
            description = "原生浅灰与极简白卡片",
            isDark = false,
            isDefaultGroup = true,
            bg = LightSystemGroupedBackground,
            fg = LightLabel,
            accent = LightAppleIndigo,
            accentFg = Color.White,
            syntax = appleLightSyntax,
            colorsOverride = LightAppColors.copy(themeId = "default-light")
        ),
        makePreset(
            id = "desktop-light",
            name = "Default Light",
            description = "Antigravity 桌面默认浅色",
            isDark = false,
            isDefaultGroup = true,
            bg = Color(0xFFFFFFFF),
            fg = Color(0xFF0A0A0C),
            accent = Color(0xFF2F75C3),
            syntax = desktopLightSyntax
        ),
        makePreset(
            id = "legacy-light",
            name = "Legacy Light",
            description = "经典浅灰高对比工作区",
            isDark = false,
            isDefaultGroup = true,
            bg = Color(0xFFF9F9F9),
            fg = Color(0xFF101010),
            accent = Color(0xFF007ACC),
            syntax = desktopLightSyntax
        ),
        makePreset(
            id = "catppuccin-light",
            name = "Catppuccin",
            description = "Latte 柔和薰衣草粉紫",
            isDark = false,
            isDefaultGroup = false,
            bg = Color(0xFFEAECF0),
            fg = Color(0xFF4C4F69),
            accent = Color(0xFF8839EF),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFF8839EF),
                string = Color(0xFF40A02B),
                comment = Color(0xFF7C7F93),
                number = Color(0xFFFE640B),
                typeName = Color(0xFFDF8E1D),
                function = Color(0xFF1E66F5),
                property = Color(0xFF1E66F5),
                diffAdd = Color(0xFF40A02B),
                diffDel = Color(0xFFD20F39)
            )
        ),
        makePreset(
            id = "everforest-light",
            name = "Everforest",
            description = "护眼暖米纸张与苔藓绿",
            isDark = false,
            isDefaultGroup = false,
            bg = Color(0xFFF8F0DC),
            fg = Color(0xFF5C6A72),
            accent = Color(0xFF4E7525),
            accentFg = Color(0xFFF8F0DC),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFD13835),
                string = Color(0xFF547300),
                comment = Color(0xFF708073),
                number = Color(0xFFB84692),
                typeName = Color(0xFFA67200),
                function = Color(0xFF247A58),
                property = Color(0xFF267DA8),
                diffAdd = Color(0xFF547300),
                diffDel = Color(0xFFD13835)
            )
        ),
        makePreset(
            id = "gruvbox-light",
            name = "Gruvbox Material",
            description = "复古暖羊皮纸与琥珀橙",
            isDark = false,
            isDefaultGroup = false,
            bg = Color(0xFFF9F5D7),
            fg = Color(0xFF4F3829),
            accent = Color(0xFFC35E0A),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFC14A4A),
                string = Color(0xFF6C782E),
                comment = Color(0xFF928374),
                number = Color(0xFF945E80),
                typeName = Color(0xFFB47109),
                function = Color(0xFF6C782E),
                property = Color(0xFF45707A),
                diffAdd = Color(0xFF6C782E),
                diffDel = Color(0xFFC14A4A)
            )
        ),
        makePreset(
            id = "one-light",
            name = "One Light",
            description = "经典 Atom 素净浅灰",
            isDark = false,
            isDefaultGroup = false,
            bg = Color(0xFFF5F5F5),
            fg = Color(0xFF383A42),
            accent = Color(0xFF4078F2),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFA626A4),
                string = Color(0xFF449344),
                comment = Color(0xFFA0A1A7),
                number = Color(0xFF986801),
                typeName = Color(0xFFC18401),
                function = Color(0xFF4078F2),
                property = Color(0xFFE45649),
                diffAdd = Color(0xFF449344),
                diffDel = Color(0xFFE45649)
            )
        ),
        makePreset(
            id = "solarized-light",
            name = "Solarized Light",
            description = "经典低眩光暖沙色",
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
        )
    )

    val darkPresets: List<ThemePreset> = listOf(
        makePreset(
            id = "default",
            name = "Apple 默认",
            description = "纯黑 OLED 与深灰分层",
            isDark = true,
            isDefaultGroup = true,
            bg = DarkSystemGroupedBackground,
            fg = DarkLabel,
            accent = DarkAppleIndigo,
            accentFg = Color.White,
            syntax = appleDarkSyntax,
            colorsOverride = DarkAppColors.copy(themeId = "default-dark")
        ),
        makePreset(
            id = "desktop-dark",
            name = "Default Dark",
            description = "Antigravity 桌面默认深邃黑",
            isDark = true,
            isDefaultGroup = true,
            bg = Color(0xFF070708),
            fg = Color(0xFFFFFFFF),
            accent = Color(0xFF2F71E3),
            syntax = desktopDarkSyntax
        ),
        makePreset(
            id = "legacy-dark",
            name = "Legacy Dark",
            description = "经典炭黑高可读性",
            isDark = true,
            isDefaultGroup = true,
            bg = Color(0xFF101010),
            fg = Color(0xFFCCCCCC),
            accent = Color(0xFF007ACC),
            syntax = desktopDarkSyntax
        ),
        makePreset(
            id = "tokyo-night",
            name = "Tokyo Night",
            description = "东京夜色深靛蓝与霓虹蓝",
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
            id = "nord",
            name = "Nord",
            description = "极地冰蓝冷调暗色",
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
            id = "catppuccin-dark",
            name = "Catppuccin",
            description = "Macchiato 柔和粉紫暮色",
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
            id = "dracula",
            name = "Dracula",
            description = "德古拉经典暗紫夜色",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF282A36),
            fg = Color(0xFFF8F8F2),
            accent = Color(0xFFBD93F9),
            accentFg = Color(0xFF282A36),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFFF79C6),
                string = Color(0xFFF1FA8C),
                comment = Color(0xFF6272A4),
                number = Color(0xFFBD93F9),
                typeName = Color(0xFF8BE9FD),
                function = Color(0xFF50FA7B),
                property = Color(0xFF8BE9FD),
                diffAdd = Color(0xFF50FA7B),
                diffDel = Color(0xFFFF5555)
            )
        ),
        makePreset(
            id = "everforest-dark",
            name = "Everforest",
            description = "松林暗绿与柔光暖字",
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
        ),
        makePreset(
            id = "gruvbox-dark",
            name = "Gruvbox Material",
            description = "复古炭黑与暖橘琥珀",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF141617),
            fg = Color(0xFFD4BE98),
            accent = Color(0xFFE78A4E),
            accentFg = Color(0xFF141617),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFEA6962),
                string = Color(0xFFA9B665),
                comment = Color(0xFF7C6F64),
                number = Color(0xFFD3869B),
                typeName = Color(0xFFD8A657),
                function = Color(0xFFA9B665),
                property = Color(0xFF7DAEA3),
                diffAdd = Color(0xFFA9B665),
                diffDel = Color(0xFFEA6962)
            )
        ),
        makePreset(
            id = "monokai",
            name = "Monokai",
            description = "高对比经典亮粉代码风",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF272822),
            fg = Color(0xFFF8F8F2),
            accent = Color(0xFFF92672),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFF92672),
                string = Color(0xFFE6DB74),
                comment = Color(0xFF75715E),
                number = Color(0xFFAE81FF),
                typeName = Color(0xFFA6E22E),
                function = Color(0xFFA6E22E),
                property = Color(0xFFF92672),
                diffAdd = Color(0xFFA6E22E),
                diffDel = Color(0xFFF92672)
            )
        ),
        makePreset(
            id = "one-dark-pro",
            name = "One Dark Pro",
            description = "Atom / VS Code 经典灰蓝",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF282C34),
            fg = Color(0xFFABB2BF),
            accent = Color(0xFF61AFEF),
            accentFg = Color(0xFF1E222A),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFC678DD),
                string = Color(0xFF98C379),
                comment = Color(0xFF5C6370),
                number = Color(0xFFD19A66),
                typeName = Color(0xFFE5C07B),
                function = Color(0xFF61AFEF),
                property = Color(0xFFE06C75),
                diffAdd = Color(0xFF98C379),
                diffDel = Color(0xFFE06C75)
            )
        ),
        makePreset(
            id = "solarized-dark",
            name = "Solarized Dark",
            description = "深青绿护眼学术配色",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF002B36),
            fg = Color(0xFF93A1A1),
            accent = Color(0xFF268BD2),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFF859900),
                string = Color(0xFF2AA198),
                comment = Color(0xFF586E75),
                number = Color(0xFFD33682),
                typeName = Color(0xFFB58900),
                function = Color(0xFF268BD2),
                property = Color(0xFF268BD2),
                diffAdd = Color(0xFF859900),
                diffDel = Color(0xFFDC322F)
            )
        ),
        makePreset(
            id = "vesper",
            name = "Vesper",
            description = "极简纯黑与暖杏桃点缀",
            isDark = true,
            isDefaultGroup = false,
            bg = Color(0xFF101010),
            fg = Color(0xFFFFFFFF),
            accent = Color(0xFFFFC799),
            accentFg = Color(0xFF101010),
            syntax = ThemeSyntaxPalette(
                keyword = Color(0xFFFBADFF),
                string = Color(0xFF99FFE4),
                comment = Color(0xFF7C7C7C),
                number = Color(0xFFFFC799),
                typeName = Color(0xFFFFC799),
                function = Color(0xFFFFC799),
                property = Color(0xFFFFFFFF),
                diffAdd = Color(0xFF99FFE4),
                diffDel = Color(0xFFF85149)
            )
        )
    )

    val allPresets: List<ThemePreset> = lightPresets + darkPresets

    fun findLight(id: String): ThemePreset =
        lightPresets.firstOrNull { it.id == id } ?: lightPresets.first()

    fun findDark(id: String): ThemePreset =
        darkPresets.firstOrNull { it.id == id } ?: darkPresets.first()

    fun resolve(isDark: Boolean, lightPresetId: String, darkPresetId: String): ThemePreset =
        if (isDark) findDark(darkPresetId) else findLight(lightPresetId)

    fun findByThemeId(themeId: String, fallbackIsDark: Boolean): ThemePreset = when (themeId) {
        "default-light" -> lightPresets.first()
        "default-dark" -> darkPresets.first()
        else -> allPresets.firstOrNull { it.id == themeId && it.isDark == fallbackIsDark }
            ?: allPresets.firstOrNull { it.id == themeId }
            ?: if (fallbackIsDark) darkPresets.first() else lightPresets.first()
    }
}
