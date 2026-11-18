import SwiftUI
import UIKit

// MARK: - Syntax & Share Card Skin Models

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
    public let isDefaultStyle: Bool

    public let bgHex: String
    public let bgEndHex: String
    public let fgHex: String
    public let accentHex: String
    public let accentFgHex: String

    public let bg: Color
    public let bgEnd: Color
    public let surface: Color
    public let surfaceVariant: Color
    public let border: Color
    public let fg: Color
    public let secondaryFg: Color
    public let accent: Color
    public let accentFg: Color
    public let codeBlockBg: Color
    public let syntax: ThemeSyntaxPalette

    public var storageKey: String { id }

    public var backgroundGradient: LinearGradient {
        LinearGradient(
            colors: [bg, bgEnd],
            startPoint: .topLeading,
            endPoint: .bottomTrailing
        )
    }

    public static func == (lhs: ThemePreset, rhs: ThemePreset) -> Bool {
        lhs.id == rhs.id
    }

    public func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
}

// MARK: - Hex & Color Mixing Helpers

extension UIColor {
    static func fromHex(_ hex: String) -> UIColor {
        let cleaned = hex.trimmingCharacters(in: .whitespacesAndNewlines).replacingOccurrences(of: "#", with: "")
        let full = cleaned.count == 3 ? cleaned.map { "\($0)\($0)" }.joined() : cleaned
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

// MARK: - Curated 7 Share Card Skins (2 Original + 5 Desktop Presets)

public enum ThemePresetCatalog {

    public static let appleLightSyntax = ThemeSyntaxPalette(
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

    public static let appleDarkSyntax = ThemeSyntaxPalette(
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

    private static func makePreset(
        id: String,
        name: String,
        description: String,
        isDark: Bool,
        bgHex: String,
        bgEndHex: String? = nil,
        fgHex: String,
        accentHex: String,
        accentFgHex: String = "#FFFFFF",
        surfaceHex: String? = nil,
        surfaceVariantHex: String? = nil,
        borderHex: String? = nil,
        codeBgHex: String? = nil,
        syntax: ThemeSyntaxPalette
    ) -> ThemePreset {
        let bgUI = UIColor.fromHex(bgHex)
        let bgEndUI = bgEndHex.map(UIColor.fromHex) ?? bgUI.mixed(with: .black, ratio: isDark ? 0.14 : 0.04)
        let fgUI = UIColor.fromHex(fgHex)
        let accentUI = UIColor.fromHex(accentHex)
        let accentFgUI = UIColor.fromHex(accentFgHex)
        let surfaceUI = surfaceHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .white, ratio: 0.07) : bgUI.mixed(with: .white, ratio: 0.72))
        let surfaceVarUI = surfaceVariantHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .white, ratio: 0.12) : bgUI.mixed(with: fgUI, ratio: 0.08))
        let borderUI = borderHex.map(UIColor.fromHex)
            ?? bgUI.mixed(with: fgUI, ratio: isDark ? 0.22 : 0.16)
        let secFgUI = bgUI.mixed(with: fgUI, ratio: 0.68)
        let codeBgUI = codeBgHex.map(UIColor.fromHex)
            ?? (isDark ? bgUI.mixed(with: .black, ratio: 0.16) : bgUI.mixed(with: fgUI, ratio: 0.055))

        return ThemePreset(
            id: id,
            name: name,
            description: description,
            isDark: isDark,
            isDefaultStyle: false,
            bgHex: bgHex,
            bgEndHex: bgEndHex ?? bgHex,
            fgHex: fgHex,
            accentHex: accentHex,
            accentFgHex: accentFgHex,
            bg: Color(uiColor: bgUI),
            bgEnd: Color(uiColor: bgEndUI),
            surface: Color(uiColor: surfaceUI),
            surfaceVariant: Color(uiColor: surfaceVarUI),
            border: Color(uiColor: borderUI),
            fg: Color(uiColor: fgUI),
            secondaryFg: Color(uiColor: secFgUI),
            accent: Color(uiColor: accentUI),
            accentFg: Color(uiColor: accentFgUI),
            codeBlockBg: Color(uiColor: codeBgUI),
            syntax: syntax
        )
    }

    public static let defaultDark = ThemePreset(
        id: "default-dark",
        name: "深色",
        description: "经典暗黑极客渐变",
        isDark: true,
        isDefaultStyle: true,
        bgHex: "#0F121C",
        bgEndHex: "#1F1738",
        fgHex: "#FFFFFF",
        accentHex: "#8C80FF",
        accentFgHex: "#FFFFFF",
        bg: Color(red: 0.06, green: 0.07, blue: 0.11),
        bgEnd: Color(red: 0.12, green: 0.09, blue: 0.22),
        surface: Color.white.opacity(0.09),
        surfaceVariant: Color.white.opacity(0.14),
        border: Color.white.opacity(0.12),
        fg: .white,
        secondaryFg: Color.white.opacity(0.68),
        accent: Color(red: 0.55, green: 0.50, blue: 1.0),
        accentFg: .white,
        codeBlockBg: Color(red: 0.10, green: 0.11, blue: 0.17),
        syntax: appleDarkSyntax
    )

    public static let defaultLight = ThemePreset(
        id: "default-light",
        name: "浅色",
        description: "极简纸白与柔和灰卡片",
        isDark: false,
        isDefaultStyle: true,
        bgHex: "#FCFCFA",
        bgEndHex: "#F5F5F2",
        fgHex: "#191B23",
        accentHex: "#5856D6",
        accentFgHex: "#FFFFFF",
        bg: Color(red: 0.99, green: 0.99, blue: 0.98),
        bgEnd: Color(red: 0.96, green: 0.96, blue: 0.95),
        surface: Color.white,
        surfaceVariant: Color.black.opacity(0.05),
        border: Color.black.opacity(0.08),
        fg: Color(red: 0.10, green: 0.11, blue: 0.14),
        secondaryFg: Color(red: 0.43, green: 0.45, blue: 0.50),
        accent: .indigo,
        accentFg: .white,
        codeBlockBg: Color(red: 0.96, green: 0.97, blue: 0.98),
        syntax: appleLightSyntax
    )

    public static let curatedSkins: [ThemePreset] = [
        defaultDark,
        defaultLight,
        makePreset(
            id: "solarized-light",
            name: "Solarized Light",
            description: "经典低眩光暖沙纸张",
            isDark: false,
            bgHex: "#FAF4E5",
            bgEndHex: "#F2E9D4",
            fgHex: "#435155",
            accentHex: "#CB4B16",
            surfaceHex: "#FDF9EE",
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
        ),
        makePreset(
            id: "nord",
            name: "Nord",
            description: "极地冰蓝冷调暗色",
            isDark: true,
            bgHex: "#2E3440",
            bgEndHex: "#242933",
            fgHex: "#D8DEE9",
            accentHex: "#88C0D0",
            accentFgHex: "#2E3440",
            surfaceHex: "#3B4252",
            surfaceVariantHex: "#434C5E",
            borderHex: "#4C566A",
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
            id: "tokyo-night",
            name: "Tokyo Night",
            description: "东京夜色深靛蓝与霓虹蓝",
            isDark: true,
            bgHex: "#1A1B26",
            bgEndHex: "#13141D",
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
            id: "catppuccin-dark",
            name: "Catppuccin",
            description: "Macchiato 柔和粉紫暮色",
            isDark: true,
            bgHex: "#24273A",
            bgEndHex: "#1B1E2E",
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
            id: "everforest-dark",
            name: "Everforest",
            description: "松林暗绿与柔光暖字",
            isDark: true,
            bgHex: "#2D353B",
            bgEndHex: "#232A2E",
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
        )
    ]

    public static func findByThemeKey(_ key: String, fallbackIsDark: Bool) -> ThemePreset {
        curatedSkins.first { $0.id == key }
            ?? (fallbackIsDark ? defaultDark : defaultLight)
    }
}

// MARK: - Shared Swatch & Skin Picker Sheet

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

public struct ShareCardSkinPickerSheet: View {
    public let selectedPreset: ThemePreset
    public let onSelect: (ThemePreset) -> Void
    @Environment(\.dismiss) private var dismiss

    public init(selectedPreset: ThemePreset, onSelect: @escaping (ThemePreset) -> Void) {
        self.selectedPreset = selectedPreset
        self.onSelect = onSelect
    }

    public var body: some View {
        NavigationStack {
            List {
                Section("基础风格") {
                    ForEach(ThemePresetCatalog.curatedSkins.filter { $0.isDefaultStyle }) { preset in
                        skinRow(preset)
                    }
                }
                Section("精选桌面端配色") {
                    ForEach(ThemePresetCatalog.curatedSkins.filter { !$0.isDefaultStyle }) { preset in
                        skinRow(preset)
                    }
                }
            }
            .navigationTitle("选择长图皮肤")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") { dismiss() }
                }
            }
        }
    }

    private func skinRow(_ preset: ThemePreset) -> some View {
        let isSelected = preset.id == selectedPreset.id
        return Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onSelect(preset)
        } label: {
            HStack(spacing: 12) {
                ThemeSwatchPillView(preset: preset)
                VStack(alignment: .leading, spacing: 2) {
                    Text(preset.name)
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.primary)
                    Text(preset.description)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                Spacer()
                if isSelected {
                    Image(systemName: "checkmark")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.accentColor)
                }
            }
        }
    }
}
