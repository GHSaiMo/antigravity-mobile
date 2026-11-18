import SwiftUI
import UIKit

// MARK: - Syntax & Theme Preset Models

public struct ThemeSyntaxPalette: Equatable, Sendable {
    public let keyword: UIColor
    public let string: UIColor
    public let comment: UIColor
    public let number: UIColor
    public let typeName: UIColor
    public let function: UIColor
    public let property: UIColor
    public let diffAdd: UIColor
    public let diffDel: UIColor

    public init(
        keyword: UIColor,
        string: UIColor,
        comment: UIColor,
        number: UIColor,
        typeName: UIColor,
        function: UIColor,
        property: UIColor,
        diffAdd: UIColor,
        diffDel: UIColor
    ) {
        self.keyword = keyword
        self.string = string
        self.comment = comment
        self.number = number
        self.typeName = typeName
        self.function = function
        self.property = property
        self.diffAdd = diffAdd
        self.diffDel = diffDel
    }
}

public struct ThemePreset: Identifiable, Hashable, Sendable {
    public let id: String
    public let name: String
    public let description: String
    public let isDark: Bool
    public let isDefaultGroup: Bool
    public let isDefaultStyle: Bool

    public let bgHex: String
    public let fgHex: String
    public let accentHex: String
    public let accentFgHex: String

    public let bg: Color
    public let surface: Color
    public let surfaceVariant: Color
    public let border: Color
    public let fg: Color
    public let secondaryFg: Color
    public let accent: Color
    public let accentFg: Color
    public let userBubbleBg: Color
    public let userBubbleText: Color
    public let codeBlockBg: Color
    public let syntax: ThemeSyntaxPalette

    /// Unique key distinguishing default-light vs default-dark for caches and ShareCard skins.
    public var storageKey: String {
        if id == "default" {
            return isDark ? "default-dark" : "default-light"
        }
        return id
    }

    public static func == (lhs: ThemePreset, rhs: ThemePreset) -> Bool {
        lhs.storageKey == rhs.storageKey
    }

    public func hash(into hasher: inout Hasher) {
        hasher.combine(storageKey)
    }
}

// MARK: - Hex & Color Mixing Helpers

extension UIColor {
    static func fromHex(_ hex: String) -> UIColor {
        let cleaned = hex.trimmingCharacters(in: .whitespacesAndNewlines).replacingOccurrences(of: "#", with: "")
        let full: String
        if cleaned.count == 3 {
            full = cleaned.map { "\($0)\($0)" }.joined()
        } else {
            full = cleaned
        }
        var rgb: UInt64 = 0
        guard full.count == 6, Scanner(string: full).scanHexInt64(&rgb) else {
            return .gray
        }
        return UIColor(
            red: CGFloat((rgb >> 16) & 0xFF) / 255.0,
            green: CGFloat((rgb >> 8) & 0xFF) / 255.0,
            blue: CGFloat(rgb & 0xFF) / 255.0,
            alpha: 1.0
        )
    }

    func mixed(with other: UIColor, ratio: CGFloat) -> UIColor {
        var r1: CGFloat = 0, g1: CGFloat = 0, b1: CGFloat = 0, a1: CGFloat = 0
        var r2: CGFloat = 0, g2: CGFloat = 0, b2: CGFloat = 0, a2: CGFloat = 0
        getRed(&r1, green: &g1, blue: &b1, alpha: &a1)
        other.getRed(&r2, green: &g2, blue: &b2, alpha: &a2)
        let t = max(0, min(1, ratio))
        return UIColor(
            red: r1 * (1 - t) + r2 * t,
            green: g1 * (1 - t) + g2 * t,
            blue: b1 * (1 - t) + b2 * t,
            alpha: a1 * (1 - t) + a2 * t
        )
    }
}

// MARK: - Catalog (Aligned 1:1 with Antigravity 2.33.0 Desktop)

public enum ThemePresetCatalog {

    private static let appleLightSyntax = ThemeSyntaxPalette(
        keyword: UIColor.fromHex("#CF222E"),
        string: UIColor.fromHex("#0A3069"),
        comment: UIColor.fromHex("#6E7781"),
        number: UIColor.fromHex("#0550AE"),
        typeName: UIColor.fromHex("#953800"),
        function: UIColor.fromHex("#8250DF"),
        property: UIColor.fromHex("#116329"),
        diffAdd: UIColor.fromHex("#1A7F37"),
        diffDel: UIColor.fromHex("#CF222E")
    )

    private static let appleDarkSyntax = ThemeSyntaxPalette(
        keyword: UIColor.fromHex("#FF7B72"),
        string: UIColor.fromHex("#A5D6FF"),
        comment: UIColor.fromHex("#8B949E"),
        number: UIColor.fromHex("#79C0FF"),
        typeName: UIColor.fromHex("#FFA657"),
        function: UIColor.fromHex("#D2A8FF"),
        property: UIColor.fromHex("#7EE787"),
        diffAdd: UIColor.fromHex("#56D364"),
        diffDel: UIColor.fromHex("#F85149")
    )

    private static let desktopLightSyntax = ThemeSyntaxPalette(
        keyword: UIColor.fromHex("#0077AA"),
        string: UIColor.fromHex("#5C8A00"),
        comment: UIColor.fromHex("#708090"),
        number: UIColor.fromHex("#990055"),
        typeName: UIColor.fromHex("#DD4A68"),
        function: UIColor.fromHex("#DD4A68"),
        property: UIColor.fromHex("#990055"),
        diffAdd: UIColor.fromHex("#5C8A00"),
        diffDel: UIColor.fromHex("#990055")
    )

    private static let desktopDarkSyntax = ThemeSyntaxPalette(
        keyword: UIColor.fromHex("#66D9EF"),
        string: UIColor.fromHex("#A6E22E"),
        comment: UIColor.fromHex("#8292A2"),
        number: UIColor.fromHex("#AE81FF"),
        typeName: UIColor.fromHex("#E6DB74"),
        function: UIColor.fromHex("#E6DB74"),
        property: UIColor.fromHex("#F92672"),
        diffAdd: UIColor.fromHex("#A6E22E"),
        diffDel: UIColor.fromHex("#F92672")
    )

    private static func makePreset(
        id: String,
        name: String,
        description: String,
        isDark: Bool,
        isDefaultGroup: Bool,
        bgHex: String,
        fgHex: String,
        accentHex: String,
        accentFgHex: String = "#FFFFFF",
        surfaceHex: String? = nil,
        surfaceVariantHex: String? = nil,
        borderHex: String? = nil,
        userBubbleHex: String? = nil,
        userBubbleTextHex: String? = nil,
        codeBgHex: String? = nil,
        syntax: ThemeSyntaxPalette
    ) -> ThemePreset {
        let bgUI = UIColor.fromHex(bgHex)
        let fgUI = UIColor.fromHex(fgHex)
        let accentUI = UIColor.fromHex(accentHex)
        let accentFgUI = UIColor.fromHex(accentFgHex)
        let surfaceUI = surfaceHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .white, ratio: 0.065) : bgUI.mixed(with: .white, ratio: 0.72))
        let surfaceVarUI = surfaceVariantHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .white, ratio: 0.12) : bgUI.mixed(with: fgUI, ratio: 0.08))
        let borderUI = borderHex.map(UIColor.fromHex)
            ?? bgUI.mixed(with: fgUI, ratio: isDark ? 0.22 : 0.16)
        let secFgUI = bgUI.mixed(with: fgUI, ratio: 0.70)
        let codeBgUI = codeBgHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .black, ratio: 0.16) : bgUI.mixed(with: fgUI, ratio: 0.055))
        let userBgUI = userBubbleHex.map(UIColor.fromHex) ?? accentUI
        let userTextUI = userBubbleTextHex.map(UIColor.fromHex) ?? accentFgUI

        return ThemePreset(
            id: id,
            name: name,
            description: description,
            isDark: isDark,
            isDefaultGroup: isDefaultGroup,
            isDefaultStyle: false,
            bgHex: bgHex,
            fgHex: fgHex,
            accentHex: accentHex,
            accentFgHex: accentFgHex,
            bg: Color(uiColor: bgUI),
            surface: Color(uiColor: surfaceUI),
            surfaceVariant: Color(uiColor: surfaceVarUI),
            border: Color(uiColor: borderUI),
            fg: Color(uiColor: fgUI),
            secondaryFg: Color(uiColor: secFgUI),
            accent: Color(uiColor: accentUI),
            accentFg: Color(uiColor: accentFgUI),
            userBubbleBg: Color(uiColor: userBgUI),
            userBubbleText: Color(uiColor: userTextUI),
            codeBlockBg: Color(uiColor: codeBgUI),
            syntax: syntax
        )
    }

    public static let defaultLight = ThemePreset(
        id: "default",
        name: "Apple 默认",
        description: "原生浅灰与极简白卡片",
        isDark: false,
        isDefaultGroup: true,
        isDefaultStyle: true,
        bgHex: "#F2F2F7",
        fgHex: "#000000",
        accentHex: "#5856D6",
        accentFgHex: "#FFFFFF",
        bg: Color(uiColor: .systemGroupedBackground),
        surface: Color(uiColor: .secondarySystemGroupedBackground),
        surfaceVariant: Color(uiColor: .tertiarySystemBackground),
        border: Color.secondary.opacity(0.22),
        fg: .primary,
        secondaryFg: .secondary,
        accent: .indigo,
        accentFg: .white,
        userBubbleBg: .indigo,
        userBubbleText: .white,
        codeBlockBg: Color(uiColor: .tertiarySystemBackground),
        syntax: appleLightSyntax
    )

    public static let defaultDark = ThemePreset(
        id: "default",
        name: "Apple 默认",
        description: "纯黑 OLED 与深灰分层",
        isDark: true,
        isDefaultGroup: true,
        isDefaultStyle: true,
        bgHex: "#000000",
        fgHex: "#FFFFFF",
        accentHex: "#5E5CE6",
        accentFgHex: "#FFFFFF",
        bg: Color(uiColor: .systemGroupedBackground),
        surface: Color(uiColor: .secondarySystemGroupedBackground),
        surfaceVariant: Color(uiColor: .tertiarySystemBackground),
        border: Color.secondary.opacity(0.24),
        fg: .primary,
        secondaryFg: .secondary,
        accent: .indigo,
        accentFg: .white,
        userBubbleBg: .indigo,
        userBubbleText: .white,
        codeBlockBg: Color(uiColor: .tertiarySystemBackground),
        syntax: appleDarkSyntax
    )

    public static let lightPresets: [ThemePreset] = [
        defaultLight,
        makePreset(
            id: "desktop-light",
            name: "Default Light",
            description: "Antigravity 桌面默认浅色",
            isDark: false,
            isDefaultGroup: true,
            bgHex: "#FFFFFF",
            fgHex: "#0A0A0C",
            accentHex: "#2F75C3",
            syntax: desktopLightSyntax
        ),
        makePreset(
            id: "legacy-light",
            name: "Legacy Light",
            description: "经典浅灰高对比工作区",
            isDark: false,
            isDefaultGroup: true,
            bgHex: "#F9F9F9",
            fgHex: "#101010",
            accentHex: "#007ACC",
            syntax: desktopLightSyntax
        ),
        makePreset(
            id: "catppuccin-light",
            name: "Catppuccin",
            description: "Latte 柔和薰衣草粉紫",
            isDark: false,
            isDefaultGroup: false,
            bgHex: "#EAECF0",
            fgHex: "#4C4F69",
            accentHex: "#8839EF",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#8839EF"),
                string: UIColor.fromHex("#40A02B"),
                comment: UIColor.fromHex("#7C7F93"),
                number: UIColor.fromHex("#FE640B"),
                typeName: UIColor.fromHex("#DF8E1D"),
                function: UIColor.fromHex("#1E66F5"),
                property: UIColor.fromHex("#1E66F5"),
                diffAdd: UIColor.fromHex("#40A02B"),
                diffDel: UIColor.fromHex("#D20F39")
            )
        ),
        makePreset(
            id: "everforest-light",
            name: "Everforest",
            description: "护眼暖米纸张与苔藓绿",
            isDark: false,
            isDefaultGroup: false,
            bgHex: "#F8F0DC",
            fgHex: "#5C6A72",
            accentHex: "#4E7525",
            accentFgHex: "#F8F0DC",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#D13835"),
                string: UIColor.fromHex("#547300"),
                comment: UIColor.fromHex("#708073"),
                number: UIColor.fromHex("#B84692"),
                typeName: UIColor.fromHex("#A67200"),
                function: UIColor.fromHex("#247A58"),
                property: UIColor.fromHex("#267DA8"),
                diffAdd: UIColor.fromHex("#547300"),
                diffDel: UIColor.fromHex("#D13835")
            )
        ),
        makePreset(
            id: "gruvbox-light",
            name: "Gruvbox Material",
            description: "复古暖羊皮纸与琥珀橙",
            isDark: false,
            isDefaultGroup: false,
            bgHex: "#F9F5D7",
            fgHex: "#4F3829",
            accentHex: "#C35E0A",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#C14A4A"),
                string: UIColor.fromHex("#6C782E"),
                comment: UIColor.fromHex("#928374"),
                number: UIColor.fromHex("#945E80"),
                typeName: UIColor.fromHex("#B47109"),
                function: UIColor.fromHex("#6C782E"),
                property: UIColor.fromHex("#45707A"),
                diffAdd: UIColor.fromHex("#6C782E"),
                diffDel: UIColor.fromHex("#C14A4A")
            )
        ),
        makePreset(
            id: "one-light",
            name: "One Light",
            description: "经典 Atom 素净浅灰",
            isDark: false,
            isDefaultGroup: false,
            bgHex: "#F5F5F5",
            fgHex: "#383A42",
            accentHex: "#4078F2",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#A626A4"),
                string: UIColor.fromHex("#449344"),
                comment: UIColor.fromHex("#A0A1A7"),
                number: UIColor.fromHex("#986801"),
                typeName: UIColor.fromHex("#C18401"),
                function: UIColor.fromHex("#4078F2"),
                property: UIColor.fromHex("#E45649"),
                diffAdd: UIColor.fromHex("#449344"),
                diffDel: UIColor.fromHex("#E45649")
            )
        ),
        makePreset(
            id: "solarized-light",
            name: "Solarized Light",
            description: "经典低眩光暖沙色",
            isDark: false,
            isDefaultGroup: false,
            bgHex: "#FAF4E5",
            fgHex: "#435155",
            accentHex: "#CB4B16",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#859900"),
                string: UIColor.fromHex("#2AA198"),
                comment: UIColor.fromHex("#93A1A1"),
                number: UIColor.fromHex("#D33682"),
                typeName: UIColor.fromHex("#B58900"),
                function: UIColor.fromHex("#268BD2"),
                property: UIColor.fromHex("#268BD2"),
                diffAdd: UIColor.fromHex("#859900"),
                diffDel: UIColor.fromHex("#DC322F")
            )
        )
    ]

    public static let darkPresets: [ThemePreset] = [
        defaultDark,
        makePreset(
            id: "desktop-dark",
            name: "Default Dark",
            description: "Antigravity 桌面默认深邃黑",
            isDark: true,
            isDefaultGroup: true,
            bgHex: "#070708",
            fgHex: "#FFFFFF",
            accentHex: "#2F71E3",
            syntax: desktopDarkSyntax
        ),
        makePreset(
            id: "legacy-dark",
            name: "Legacy Dark",
            description: "经典炭黑高可读性",
            isDark: true,
            isDefaultGroup: true,
            bgHex: "#101010",
            fgHex: "#CCCCCC",
            accentHex: "#007ACC",
            syntax: desktopDarkSyntax
        ),
        makePreset(
            id: "tokyo-night",
            name: "Tokyo Night",
            description: "东京夜色深靛蓝与霓虹蓝",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#1A1B26",
            fgHex: "#A9B1D6",
            accentHex: "#7AA2F7",
            accentFgHex: "#1A1B26",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#BB9AF7"),
                string: UIColor.fromHex("#9ECE6A"),
                comment: UIColor.fromHex("#565F89"),
                number: UIColor.fromHex("#FF9E64"),
                typeName: UIColor.fromHex("#2AC3DE"),
                function: UIColor.fromHex("#7AA2F7"),
                property: UIColor.fromHex("#7AA2F7"),
                diffAdd: UIColor.fromHex("#9ECE6A"),
                diffDel: UIColor.fromHex("#F7768E")
            )
        ),
        makePreset(
            id: "nord",
            name: "Nord",
            description: "极地冰蓝冷调暗色",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#2E3440",
            fgHex: "#D8DEE9",
            accentHex: "#88C0D0",
            accentFgHex: "#2E3440",
            surfaceHex: "#3B4252",
            surfaceVariantHex: "#434C5E",
            borderHex: "#4C566A",
            userBubbleHex: "#5E81AC",
            userBubbleTextHex: "#ECEFF4",
            codeBgHex: "#272C36",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#81A1C1"),
                string: UIColor.fromHex("#A3BE8C"),
                comment: UIColor.fromHex("#616E88"),
                number: UIColor.fromHex("#B48EAD"),
                typeName: UIColor.fromHex("#8FBCBB"),
                function: UIColor.fromHex("#88C0D0"),
                property: UIColor.fromHex("#8FBCBB"),
                diffAdd: UIColor.fromHex("#A3BE8C"),
                diffDel: UIColor.fromHex("#BF616A")
            )
        ),
        makePreset(
            id: "catppuccin-dark",
            name: "Catppuccin",
            description: "Macchiato 柔和粉紫暮色",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#24273A",
            fgHex: "#CAD3F5",
            accentHex: "#C6A0F6",
            accentFgHex: "#24273A",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#C6A0F6"),
                string: UIColor.fromHex("#A6DA95"),
                comment: UIColor.fromHex("#939AB7"),
                number: UIColor.fromHex("#F5A97F"),
                typeName: UIColor.fromHex("#EED49F"),
                function: UIColor.fromHex("#8AADF4"),
                property: UIColor.fromHex("#8AADF4"),
                diffAdd: UIColor.fromHex("#A6DA95"),
                diffDel: UIColor.fromHex("#ED8796")
            )
        ),
        makePreset(
            id: "dracula",
            name: "Dracula",
            description: "德古拉经典暗紫夜色",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#282A36",
            fgHex: "#F8F8F2",
            accentHex: "#BD93F9",
            accentFgHex: "#282A36",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#FF79C6"),
                string: UIColor.fromHex("#F1FA8C"),
                comment: UIColor.fromHex("#6272A4"),
                number: UIColor.fromHex("#BD93F9"),
                typeName: UIColor.fromHex("#8BE9FD"),
                function: UIColor.fromHex("#50FA7B"),
                property: UIColor.fromHex("#8BE9FD"),
                diffAdd: UIColor.fromHex("#50FA7B"),
                diffDel: UIColor.fromHex("#FF5555")
            )
        ),
        makePreset(
            id: "everforest-dark",
            name: "Everforest",
            description: "松林暗绿与柔光暖字",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#2D353B",
            fgHex: "#D3C6AA",
            accentHex: "#A7C080",
            accentFgHex: "#232A2E",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#E67E80"),
                string: UIColor.fromHex("#A7C080"),
                comment: UIColor.fromHex("#9AA69D"),
                number: UIColor.fromHex("#D699B6"),
                typeName: UIColor.fromHex("#DBBC7F"),
                function: UIColor.fromHex("#83C092"),
                property: UIColor.fromHex("#7FBBB3"),
                diffAdd: UIColor.fromHex("#A7C080"),
                diffDel: UIColor.fromHex("#E67E80")
            )
        ),
        makePreset(
            id: "gruvbox-dark",
            name: "Gruvbox Material",
            description: "复古炭黑与暖橘琥珀",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#141617",
            fgHex: "#D4BE98",
            accentHex: "#E78A4E",
            accentFgHex: "#141617",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#EA6962"),
                string: UIColor.fromHex("#A9B665"),
                comment: UIColor.fromHex("#7C6F64"),
                number: UIColor.fromHex("#D3869B"),
                typeName: UIColor.fromHex("#D8A657"),
                function: UIColor.fromHex("#A9B665"),
                property: UIColor.fromHex("#7DAEA3"),
                diffAdd: UIColor.fromHex("#A9B665"),
                diffDel: UIColor.fromHex("#EA6962")
            )
        ),
        makePreset(
            id: "monokai",
            name: "Monokai",
            description: "高对比经典亮粉代码风",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#272822",
            fgHex: "#F8F8F2",
            accentHex: "#F92672",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#F92672"),
                string: UIColor.fromHex("#E6DB74"),
                comment: UIColor.fromHex("#75715E"),
                number: UIColor.fromHex("#AE81FF"),
                typeName: UIColor.fromHex("#A6E22E"),
                function: UIColor.fromHex("#A6E22E"),
                property: UIColor.fromHex("#F92672"),
                diffAdd: UIColor.fromHex("#A6E22E"),
                diffDel: UIColor.fromHex("#F92672")
            )
        ),
        makePreset(
            id: "one-dark-pro",
            name: "One Dark Pro",
            description: "Atom / VS Code 经典灰蓝",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#282C34",
            fgHex: "#ABB2BF",
            accentHex: "#61AFEF",
            accentFgHex: "#1E222A",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#C678DD"),
                string: UIColor.fromHex("#98C379"),
                comment: UIColor.fromHex("#5C6370"),
                number: UIColor.fromHex("#D19A66"),
                typeName: UIColor.fromHex("#E5C07B"),
                function: UIColor.fromHex("#61AFEF"),
                property: UIColor.fromHex("#E06C75"),
                diffAdd: UIColor.fromHex("#98C379"),
                diffDel: UIColor.fromHex("#E06C75")
            )
        ),
        makePreset(
            id: "solarized-dark",
            name: "Solarized Dark",
            description: "深青绿护眼学术配色",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#002B36",
            fgHex: "#93A1A1",
            accentHex: "#268BD2",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#859900"),
                string: UIColor.fromHex("#2AA198"),
                comment: UIColor.fromHex("#586E75"),
                number: UIColor.fromHex("#D33682"),
                typeName: UIColor.fromHex("#B58900"),
                function: UIColor.fromHex("#268BD2"),
                property: UIColor.fromHex("#268BD2"),
                diffAdd: UIColor.fromHex("#859900"),
                diffDel: UIColor.fromHex("#DC322F")
            )
        ),
        makePreset(
            id: "vesper",
            name: "Vesper",
            description: "极简纯黑与暖杏桃点缀",
            isDark: true,
            isDefaultGroup: false,
            bgHex: "#101010",
            fgHex: "#FFFFFF",
            accentHex: "#FFC799",
            accentFgHex: "#101010",
            syntax: ThemeSyntaxPalette(
                keyword: UIColor.fromHex("#FBADFF"),
                string: UIColor.fromHex("#99FFE4"),
                comment: UIColor.fromHex("#7C7C7C"),
                number: UIColor.fromHex("#FFC799"),
                typeName: UIColor.fromHex("#FFC799"),
                function: UIColor.fromHex("#FFC799"),
                property: UIColor.fromHex("#FFFFFF"),
                diffAdd: UIColor.fromHex("#99FFE4"),
                diffDel: UIColor.fromHex("#F85149")
            )
        )
    ]

    public static var allPresets: [ThemePreset] { lightPresets + darkPresets }

    public static func findLight(_ id: String) -> ThemePreset {
        lightPresets.first { $0.id == id } ?? defaultLight
    }

    public static func findDark(_ id: String) -> ThemePreset {
        darkPresets.first { $0.id == id } ?? defaultDark
    }

    public static func resolve(isDark: Bool, lightPresetId: String, darkPresetId: String) -> ThemePreset {
        isDark ? findDark(darkPresetId) : findLight(lightPresetId)
    }

    public static func findByThemeKey(_ key: String, fallbackIsDark: Bool) -> ThemePreset {
        switch key {
        case "default-light": return defaultLight
        case "default-dark": return defaultDark
        default:
            return allPresets.first { $0.storageKey == key || ($0.id == key && $0.isDark == fallbackIsDark) }
                ?? allPresets.first { $0.id == key }
                ?? (fallbackIsDark ? defaultDark : defaultLight)
        }
    }
}

// MARK: - SwiftUI Environment Key

private struct AppThemePresetKey: EnvironmentKey {
    static let defaultValue: ThemePreset? = nil
}

extension EnvironmentValues {
    public var appThemePreset: ThemePreset? {
        get { self[AppThemePresetKey.self] }
        set { self[AppThemePresetKey.self] = newValue }
    }
}

// MARK: - Shared Theme Swatch Views

public struct ThemeSwatchDotsView: View {
    public let preset: ThemePreset

    public init(preset: ThemePreset) {
        self.preset = preset
    }

    public var body: some View {
        HStack(spacing: 3) {
            Circle().fill(preset.bg).frame(width: 9, height: 9)
                .overlay(Circle().stroke(Color.gray.opacity(0.35), lineWidth: 0.5))
            Circle().fill(preset.fg).frame(width: 9, height: 9)
                .overlay(Circle().stroke(Color.gray.opacity(0.35), lineWidth: 0.5))
            Circle().fill(preset.accent).frame(width: 9, height: 9)
                .overlay(Circle().stroke(Color.gray.opacity(0.35), lineWidth: 0.5))
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 3)
        .background(Color.secondary.opacity(0.12), in: Capsule())
    }
}

public struct ThemeSwatchPillView: View {
    public let preset: ThemePreset

    public init(preset: ThemePreset) {
        self.preset = preset
    }

    public var body: some View {
        HStack(spacing: 0) {
            RoundedRectangle(cornerRadius: 2, style: .continuous)
                .fill(preset.accent)
                .frame(width: 4, height: 14)
            Spacer(minLength: 0)
            Circle()
                .fill(preset.fg)
                .frame(width: 9, height: 9)
        }
        .padding(.horizontal, 5)
        .frame(width: 34, height: 24)
        .background(preset.bg, in: RoundedRectangle(cornerRadius: 7, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 7, style: .continuous)
                .stroke(Color.secondary.opacity(0.28), lineWidth: 0.8)
        )
    }
}
