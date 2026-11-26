import SwiftUI
import UIKit

public struct ModelPickerRow: Identifiable, Equatable {
    public let id: String
    public let title: String
    public let compactTitle: String?
    public let providerKind: ModelProviderKind
    public let isGroupStart: Bool
    public let variantsTopToBottom: [ModelVariant]
    public let defaultVariantIndex: Int
    public let hasSubmenu: Bool
    public let usesEffortLabels: Bool

    public var hasInlineEffort: Bool {
        variantsTopToBottom.count > 1
    }
}

public enum ModelQuotaPoolKind: Equatable, Sendable {
    case gemini
    case claude
}

// MARK: - Shared Effort Template & Provider Palettes

private struct CosmicStarSpec {
    let x: CGFloat
    let y: CGFloat
    let radius: CGFloat
    let phase: Double
    let speed: Double
    let isSparkle: Bool
}

private let sharedCosmicStars: [CosmicStarSpec] = [
    CosmicStarSpec(x: 0.07, y: 0.28, radius: 1.05, phase: 0.2, speed: 1.4, isSparkle: false),
    CosmicStarSpec(x: 0.13, y: 0.72, radius: 0.85, phase: 1.7, speed: 1.1, isSparkle: false),
    CosmicStarSpec(x: 0.21, y: 0.34, radius: 1.45, phase: 3.1, speed: 1.7, isSparkle: true),
    CosmicStarSpec(x: 0.28, y: 0.66, radius: 0.90, phase: 4.5, speed: 1.2, isSparkle: false),
    CosmicStarSpec(x: 0.35, y: 0.22, radius: 1.10, phase: 0.9, speed: 1.5, isSparkle: false),
    CosmicStarSpec(x: 0.43, y: 0.76, radius: 1.25, phase: 2.3, speed: 1.3, isSparkle: false),
    CosmicStarSpec(x: 0.50, y: 0.30, radius: 0.85, phase: 5.1, speed: 1.8, isSparkle: false),
    CosmicStarSpec(x: 0.57, y: 0.62, radius: 1.55, phase: 1.2, speed: 1.6, isSparkle: true),
    CosmicStarSpec(x: 0.64, y: 0.24, radius: 0.95, phase: 3.8, speed: 1.1, isSparkle: false),
    CosmicStarSpec(x: 0.70, y: 0.74, radius: 1.10, phase: 0.4, speed: 1.4, isSparkle: false),
    CosmicStarSpec(x: 0.76, y: 0.38, radius: 0.80, phase: 2.8, speed: 1.9, isSparkle: false),
    CosmicStarSpec(x: 0.82, y: 0.68, radius: 1.45, phase: 4.1, speed: 1.5, isSparkle: true),
    CosmicStarSpec(x: 0.88, y: 0.26, radius: 1.00, phase: 5.6, speed: 1.2, isSparkle: false),
    CosmicStarSpec(x: 0.93, y: 0.54, radius: 1.20, phase: 1.9, speed: 1.6, isSparkle: false),
    CosmicStarSpec(x: 0.46, y: 0.48, radius: 0.75, phase: 2.6, speed: 1.3, isSparkle: false),
    CosmicStarSpec(x: 0.61, y: 0.44, radius: 0.85, phase: 4.9, speed: 1.7, isSparkle: false),
]

struct EffortStylePalette {
    let accent: Color
    let lowFill: Color
    let medColors: [Color]
    let highColors: [Color]
    let cosmicColors: [Color]
    let starColor: Color
    let glowColor: Color
    let chipRimColors: [Color]

    static func palette(for kind: ModelProviderKind, isDark: Bool) -> EffortStylePalette {
        switch kind {
        case .gemini:
            return EffortStylePalette(
                accent: .blue,
                lowFill: Color(red: 0x25 / 255.0, green: 0x63 / 255.0, blue: 0xEB / 255.0).opacity(isDark ? 0.24 : 0.15),
                medColors: isDark
                    ? [
                        Color(red: 0x1E / 255.0, green: 0x40 / 255.0, blue: 0xAF / 255.0).opacity(0.46),
                        Color(red: 0x3B / 255.0, green: 0x82 / 255.0, blue: 0xF6 / 255.0).opacity(0.62)
                    ]
                    : [
                        Color(red: 0x60 / 255.0, green: 0xA5 / 255.0, blue: 0xFA / 255.0).opacity(0.28),
                        Color(red: 0x25 / 255.0, green: 0x63 / 255.0, blue: 0xEB / 255.0).opacity(0.42)
                    ],
                highColors: [
                    Color(red: 0x1D / 255.0, green: 0x4E / 255.0, blue: 0xD8 / 255.0),
                    Color(red: 0x25 / 255.0, green: 0x63 / 255.0, blue: 0xEB / 255.0),
                    Color(red: 0x38 / 255.0, green: 0xBD / 255.0, blue: 0xF8 / 255.0)
                ],
                cosmicColors: [
                    Color(red: 0x13 / 255.0, green: 0x17 / 255.0, blue: 0x4A / 255.0),
                    Color(red: 0x1E / 255.0, green: 0x3A / 255.0, blue: 0x8A / 255.0),
                    Color(red: 0x4F / 255.0, green: 0x46 / 255.0, blue: 0xE5 / 255.0),
                    Color(red: 0x38 / 255.0, green: 0xBD / 255.0, blue: 0xF8 / 255.0)
                ],
                starColor: .white,
                glowColor: Color(red: 0x38 / 255.0, green: 0xBD / 255.0, blue: 0xF8 / 255.0),
                chipRimColors: [
                    Color(red: 0x38 / 255.0, green: 0xBD / 255.0, blue: 0xF8 / 255.0),
                    Color(red: 0x63 / 255.0, green: 0x66 / 255.0, blue: 0xF1 / 255.0),
                    Color(red: 0x25 / 255.0, green: 0x63 / 255.0, blue: 0xEB / 255.0),
                    Color(red: 0x38 / 255.0, green: 0xBD / 255.0, blue: 0xF8 / 255.0)
                ]
            )
        case .claude:
            return EffortStylePalette(
                accent: .orange,
                lowFill: Color(red: 0xEA / 255.0, green: 0x58 / 255.0, blue: 0x0C / 255.0).opacity(isDark ? 0.24 : 0.15),
                medColors: isDark
                    ? [
                        Color(red: 0x9A / 255.0, green: 0x34 / 255.0, blue: 0x12 / 255.0).opacity(0.46),
                        Color(red: 0xF9 / 255.0, green: 0x73 / 255.0, blue: 0x16 / 255.0).opacity(0.62)
                    ]
                    : [
                        Color(red: 0xFD / 255.0, green: 0xBA / 255.0, blue: 0x74 / 255.0).opacity(0.34),
                        Color(red: 0xEA / 255.0, green: 0x58 / 255.0, blue: 0x0C / 255.0).opacity(0.42)
                    ],
                highColors: [
                    Color(red: 0xC2 / 255.0, green: 0x41 / 255.0, blue: 0x0C / 255.0),
                    Color(red: 0xEA / 255.0, green: 0x58 / 255.0, blue: 0x0C / 255.0),
                    Color(red: 0xFB / 255.0, green: 0x92 / 255.0, blue: 0x3C / 255.0)
                ],
                cosmicColors: [
                    Color(red: 0x45 / 255.0, green: 0x1A / 255.0, blue: 0x03 / 255.0),
                    Color(red: 0x9A / 255.0, green: 0x34 / 255.0, blue: 0x12 / 255.0),
                    Color(red: 0xEA / 255.0, green: 0x58 / 255.0, blue: 0x0C / 255.0),
                    Color(red: 0xFB / 255.0, green: 0xBF / 255.0, blue: 0x24 / 255.0)
                ],
                starColor: Color(red: 0xFE / 255.0, green: 0xF3 / 255.0, blue: 0xC7 / 255.0),
                glowColor: Color(red: 0xFB / 255.0, green: 0x92 / 255.0, blue: 0x3C / 255.0),
                chipRimColors: [
                    Color(red: 0xFB / 255.0, green: 0xBF / 255.0, blue: 0x24 / 255.0),
                    Color(red: 0xF9 / 255.0, green: 0x73 / 255.0, blue: 0x16 / 255.0),
                    Color(red: 0xC2 / 255.0, green: 0x41 / 255.0, blue: 0x0C / 255.0),
                    Color(red: 0xFB / 255.0, green: 0xBF / 255.0, blue: 0x24 / 255.0)
                ]
            )
        case .orchestrator:
            return EffortStylePalette(
                accent: kind.accentColor,
                lowFill: Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0).opacity(isDark ? 0.28 : 0.14),
                medColors: isDark
                    ? [
                        Color(red: 0x33 / 255.0, green: 0x41 / 255.0, blue: 0x55 / 255.0).opacity(0.55),
                        Color(red: 0x64 / 255.0, green: 0x74 / 255.0, blue: 0x8B / 255.0).opacity(0.68)
                    ]
                    : [
                        Color(red: 0x94 / 255.0, green: 0xA3 / 255.0, blue: 0xB8 / 255.0).opacity(0.30),
                        Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0).opacity(0.42)
                    ],
                highColors: isDark
                    ? [
                        Color(red: 0x33 / 255.0, green: 0x41 / 255.0, blue: 0x55 / 255.0),
                        Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0),
                        Color(red: 0x94 / 255.0, green: 0xA3 / 255.0, blue: 0xB8 / 255.0)
                    ]
                    : [
                        Color(red: 0x1E / 255.0, green: 0x29 / 255.0, blue: 0x3B / 255.0),
                        Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0),
                        Color(red: 0x64 / 255.0, green: 0x74 / 255.0, blue: 0x8B / 255.0)
                    ],
                cosmicColors: [
                    Color(red: 0x0B / 255.0, green: 0x10 / 255.0, blue: 0x1C / 255.0),
                    Color(red: 0x1E / 255.0, green: 0x29 / 255.0, blue: 0x3B / 255.0),
                    Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0),
                    Color(red: 0x94 / 255.0, green: 0xA3 / 255.0, blue: 0xB8 / 255.0)
                ],
                starColor: Color(red: 0xF1 / 255.0, green: 0xF5 / 255.0, blue: 0xF9 / 255.0),
                glowColor: Color(red: 0xCB / 255.0, green: 0xD5 / 255.0, blue: 0xE1 / 255.0),
                chipRimColors: [
                    Color(red: 0xE2 / 255.0, green: 0xE8 / 255.0, blue: 0xF0 / 255.0),
                    Color(red: 0x94 / 255.0, green: 0xA3 / 255.0, blue: 0xB8 / 255.0),
                    Color(red: 0x47 / 255.0, green: 0x55 / 255.0, blue: 0x69 / 255.0),
                    Color(red: 0xE2 / 255.0, green: 0xE8 / 255.0, blue: 0xF0 / 255.0)
                ]
            )
        case .other:
            return EffortStylePalette(
                accent: .teal,
                lowFill: Color(red: 0x0D / 255.0, green: 0x94 / 255.0, blue: 0x88 / 255.0).opacity(isDark ? 0.24 : 0.15),
                medColors: isDark
                    ? [
                        Color(red: 0x11 / 255.0, green: 0x5E / 255.0, blue: 0x59 / 255.0).opacity(0.48),
                        Color(red: 0x14 / 255.0, green: 0xB8 / 255.0, blue: 0xA6 / 255.0).opacity(0.62)
                    ]
                    : [
                        Color(red: 0x5E / 255.0, green: 0xEA / 255.0, blue: 0xD4 / 255.0).opacity(0.30),
                        Color(red: 0x0D / 255.0, green: 0x94 / 255.0, blue: 0x88 / 255.0).opacity(0.42)
                    ],
                highColors: [
                    Color(red: 0x0F / 255.0, green: 0x76 / 255.0, blue: 0x6E / 255.0),
                    Color(red: 0x0D / 255.0, green: 0x94 / 255.0, blue: 0x88 / 255.0),
                    Color(red: 0x2D / 255.0, green: 0xD4 / 255.0, blue: 0xBF / 255.0)
                ],
                cosmicColors: [
                    Color(red: 0x04 / 255.0, green: 0x2F / 255.0, blue: 0x2E / 255.0),
                    Color(red: 0x0F / 255.0, green: 0x76 / 255.0, blue: 0x6E / 255.0),
                    Color(red: 0x0D / 255.0, green: 0x94 / 255.0, blue: 0x88 / 255.0),
                    Color(red: 0x5E / 255.0, green: 0xEA / 255.0, blue: 0xD4 / 255.0)
                ],
                starColor: Color(red: 0xCC / 255.0, green: 0xFB / 255.0, blue: 0xF1 / 255.0),
                glowColor: Color(red: 0x2D / 255.0, green: 0xD4 / 255.0, blue: 0xBF / 255.0),
                chipRimColors: [
                    Color(red: 0x5E / 255.0, green: 0xEA / 255.0, blue: 0xD4 / 255.0),
                    Color(red: 0x14 / 255.0, green: 0xB8 / 255.0, blue: 0xA6 / 255.0),
                    Color(red: 0x0F / 255.0, green: 0x76 / 255.0, blue: 0x6E / 255.0),
                    Color(red: 0x5E / 255.0, green: 0xEA / 255.0, blue: 0xD4 / 255.0)
                ]
            )
        }
    }

    static func fillFraction(index: Int, count: Int) -> CGFloat {
        guard count > 1 else { return 1.0 }
        let clamped = min(max(0, index), count - 1)
        switch count {
        case 2:
            return clamped == 0 ? 0.50 : 1.0
        case 3:
            return [0.35, 0.68, 1.0][clamped]
        case 4:
            return [0.28, 0.52, 0.76, 1.0][clamped]
        default:
            return 0.24 + 0.76 * CGFloat(clamped) / CGFloat(count - 1)
        }
    }
}

private struct CosmicStarfieldView: View {
    let palette: EffortStylePalette
    let animated: Bool
    var isSubtleResting: Bool = false

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        let shouldAnimate = animated && !reduceMotion
        if shouldAnimate {
            TimelineView(.animation(minimumInterval: 1.0 / 30.0)) { timeline in
                drawLayer(time: timeline.date.timeIntervalSinceReferenceDate)
            }
        } else {
            drawLayer(time: 0.75)
        }
    }

    @ViewBuilder
    private func drawLayer(time: TimeInterval) -> some View {
        ZStack {
            LinearGradient(
                colors: palette.cosmicColors,
                startPoint: .leading,
                endPoint: .trailing
            )
            .opacity(isSubtleResting ? 0.78 : 1.0)

            Canvas(opaque: false, rendersAsynchronously: true) { context, size in
                guard size.width > 1, size.height > 1 else { return }

                // Gentle sweeping aurora sheen
                if animated && !reduceMotion {
                    let cycle = (time.truncatingRemainder(dividingBy: 2.8)) / 2.8
                    let sweepX = size.width * (CGFloat(cycle) * 1.8 - 0.4)
                    let bandWidth = max(36, size.width * 0.34)
                    let rect = CGRect(x: sweepX - bandWidth / 2, y: 0, width: bandWidth, height: size.height)
                    let shimmer = Gradient(colors: [
                        .clear,
                        palette.starColor.opacity(0.20),
                        palette.glowColor.opacity(0.14),
                        .clear
                    ])
                    context.fill(
                        Path(rect),
                        with: .linearGradient(
                            shimmer,
                            startPoint: CGPoint(x: rect.minX, y: 0),
                            endPoint: CGPoint(x: rect.maxX, y: size.height * 0.45)
                        )
                    )
                }

                // Deterministic starfield with subtle asynchronous twinkle
                for star in sharedCosmicStars {
                    let cx = star.x * size.width
                    let cy = star.y * size.height
                    let wave = (animated && !reduceMotion)
                        ? (0.45 + 0.55 * (0.5 + 0.5 * sin(time * star.speed * 2.2 + star.phase)))
                        : 0.85
                    let baseAlpha = (isSubtleResting ? 0.55 : 0.95) * wave

                    // Soft halo
                    let haloR = star.radius * 2.2
                    let haloRect = CGRect(x: cx - haloR, y: cy - haloR, width: haloR * 2, height: haloR * 2)
                    context.fill(Path(ellipseIn: haloRect), with: .color(palette.glowColor.opacity(baseAlpha * 0.28)))

                    // Bright star core
                    let coreRect = CGRect(x: cx - star.radius, y: cy - star.radius, width: star.radius * 2, height: star.radius * 2)
                    context.fill(Path(ellipseIn: coreRect), with: .color(palette.starColor.opacity(baseAlpha)))

                    // 4-point cross sparkle on hero stars
                    if star.isSparkle && !isSubtleResting {
                        let arm = star.radius * 2.5
                        var cross = Path()
                        cross.move(to: CGPoint(x: cx - arm, y: cy))
                        cross.addLine(to: CGPoint(x: cx + arm, y: cy))
                        cross.move(to: CGPoint(x: cx, y: cy - arm))
                        cross.addLine(to: CGPoint(x: cx, y: cy + arm))
                        context.stroke(cross, with: .color(palette.starColor.opacity(baseAlpha * 0.65)), lineWidth: 0.7)
                    }
                }
            }
        }
    }
}

// MARK: - Picker State & Gesture Physics

@MainActor
@Observable
public final class ModelSlidePickerState {
    public var isPresented: Bool = false
    public var isInteractiveTapMode: Bool = false
    public var anchorRect: CGRect = .zero
    public var containerSize: CGSize = .zero
    public var providerKind: ModelProviderKind = .gemini
    public var rows: [ModelPickerRow] = []
    public var hoveredRowIndex: Int? = nil
    public var hoveredVariantIndex: Int? = nil
    public var isInRightColumn: Bool = false
    public var hasEnteredMenu: Bool = false
    public var activeModelID: String = ""

    private let selectionHaptic = UISelectionFeedbackGenerator()
    private let xhighImpactHaptic = UIImpactFeedbackGenerator(style: .rigid)
    private var lastDragPoint: CGPoint? = nil
    private var isHorizontalRowLocked: Bool = false

    public enum Layout {
        static let defaultRowHeight: CGFloat = 36
        static let compactRowHeight: CGFloat = 31
        static let minRowHeight: CGFloat = 25
        static let defaultRowSpacing: CGFloat = 3
        static let compactRowSpacing: CGFloat = 2
        static let cardPadding: CGFloat = 5
        static let leftCardCompactMinWidth: CGFloat = 184
        static let leftCardMinWidth: CGFloat = 212
        static let leftCardMaxWidth: CGFloat = 248
        static let rightCardCompactWidth: CGFloat = 104
        static let rightCardWidth: CGFloat = 116
        static let rightCardWideWidth: CGFloat = 124
        static let compactWidthThreshold: CGFloat = 420
        static let gapBetweenButtonAndMenu: CGFloat = 8
        static let gapBetweenColumns: CGFloat = 8
        static let horizontalMargin: CGFloat = 12
        static let topSafeMargin: CGFloat = 12
        static let tierBoundaryHysteresis: CGFloat = 6
        static let horizontalRowLockVerticalPad: CGFloat = 15
    }

    public var isCompactWidth: Bool {
        containerSize.width <= 0 || containerSize.width < Layout.compactWidthThreshold
    }

    public var rightQuotaWidth: CGFloat {
        if containerSize.width > 0 && containerSize.width < 380 {
            return Layout.rightCardCompactWidth
        }
        if containerSize.width >= 420 {
            return Layout.rightCardWideWidth
        }
        return Layout.rightCardWidth
    }

    public init() {}

    /// 长按打开全模型单列菜单：按 Gemini(蓝) -> Claude(橙) -> Orchestrator(钛灰) -> 其他 分组，
    /// 选中行内直接按 X 位置调节思考强度（Low / Medium / High / XHigh），右侧伴随显示 Gemini/Claude 共享额度。
    public func open(activeModel: String) {
        let families = ModelCatalogStore.shared.modelFamilies()
        var builtRows: [ModelPickerRow] = []
        var prevProvider: ModelProviderKind? = nil

        for family in families {
            let kind = ModelProviderKind(rawValue: family.provider.lowercased())
                ?? ModelDefaultsLogic.providerKind(forID: family.variants.first?.id ?? family.provider)
            let isFirstInGroup = prevProvider != nil && prevProvider != kind

            if !family.hasEffortVariants && family.variants.count > 1 && kind != .orchestrator {
                for (vIndex, variant) in family.variants.enumerated() {
                    let displayTitle = variant.rawName.trimmingCharacters(in: .whitespacesAndNewlines)
                    builtRows.append(
                        ModelPickerRow(
                            id: variant.id,
                            title: displayTitle.isEmpty ? family.name : displayTitle,
                            compactTitle: nil,
                            providerKind: kind,
                            isGroupStart: vIndex == 0 ? isFirstInGroup : false,
                            variantsTopToBottom: [variant],
                            defaultVariantIndex: 0,
                            hasSubmenu: false,
                            usesEffortLabels: false
                        )
                    )
                }
            } else {
                let ordered = family.variantsTopToBottom
                let fullTitle: String
                if kind == .orchestrator {
                    let source = family.name.isEmpty ? (ordered.first?.rawName ?? "Orchestrator") : family.name
                    fullTitle = ModelDefaultsLogic.formatOrchestratorSlidePickerTitle(source)
                } else {
                    fullTitle = family.name
                }
                builtRows.append(
                    ModelPickerRow(
                        id: family.id,
                        title: fullTitle,
                        compactTitle: nil,
                        providerKind: kind,
                        isGroupStart: isFirstInGroup,
                        variantsTopToBottom: ordered,
                        defaultVariantIndex: family.defaultVariantIndexTopToBottom,
                        hasSubmenu: ordered.count > 1,
                        usesEffortLabels: family.hasEffortVariants || ordered.contains(where: { $0.effortRank != nil })
                    )
                )
            }
            prevProvider = kind
        }
        guard !builtRows.isEmpty else { return }

        self.providerKind = ModelDefaultsLogic.providerKind(forID: activeModel)
        self.activeModelID = activeModel
        self.rows = builtRows
        self.hoveredRowIndex = nil
        self.hoveredVariantIndex = nil
        self.isInRightColumn = false
        self.hasEnteredMenu = false
        self.isInteractiveTapMode = false
        self.lastDragPoint = nil
        self.isHorizontalRowLocked = false
        self.selectionHaptic.prepare()
        self.xhighImpactHaptic.prepare()
        withAnimation(.spring(response: 0.22, dampingFraction: 0.85)) {
            self.isPresented = true
        }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
    }

    public func dismiss() {
        withAnimation(.easeOut(duration: 0.14)) {
            isPresented = false
        }
        isInteractiveTapMode = false
        hoveredRowIndex = nil
        hoveredVariantIndex = nil
        isInRightColumn = false
        hasEnteredMenu = false
        lastDragPoint = nil
        isHorizontalRowLocked = false
    }

    public func activeVariantIndex(in row: ModelPickerRow) -> Int? {
        row.variantsTopToBottom.firstIndex {
            $0.id.caseInsensitiveCompare(activeModelID) == .orderedSame
        }
    }

    public var highlightedQuotaPool: ModelQuotaPoolKind? {
        if let idx = hoveredRowIndex, rows.indices.contains(idx) {
            return ModelPickerQuotaCache.quotaPool(for: rows[idx])
        }
        if let activeRow = rows.first(where: { activeVariantIndex(in: $0) != nil }) {
            return ModelPickerQuotaCache.quotaPool(for: activeRow)
        }
        return nil
    }

    // MARK: - Geometry

    private var availableLeftHeight: CGFloat {
        if anchorRect.minY > Layout.topSafeMargin + Layout.gapBetweenButtonAndMenu + 60 {
            return anchorRect.minY - Layout.gapBetweenButtonAndMenu - Layout.topSafeMargin
        }
        if containerSize.height > 120 {
            return containerSize.height - Layout.topSafeMargin * 2
        }
        return 420
    }

    public var leftRowSpacing: CGFloat {
        rows.count > 7 ? Layout.compactRowSpacing : Layout.defaultRowSpacing
    }

    public var leftRowHeight: CGFloat {
        let count = max(rows.count, 1)
        let preferred = rows.count > 7 ? Layout.compactRowHeight : Layout.defaultRowHeight
        let fixedOverhead = Layout.cardPadding * 2 + CGFloat(max(0, count - 1)) * leftRowSpacing
        let maxRow = (availableLeftHeight - fixedOverhead) / CGFloat(count)
        return min(preferred, max(Layout.minRowHeight, floor(maxRow)))
    }

    public var leftContentNeedsScroll: Bool {
        let count = max(rows.count, 1)
        let naturalHeight = CGFloat(count) * leftRowHeight
            + CGFloat(max(0, count - 1)) * leftRowSpacing
            + Layout.cardPadding * 2
        return naturalHeight > availableLeftHeight + 1
    }

    public var leftCardSize: CGSize {
        let count = max(rows.count, 1)
        let naturalHeight = CGFloat(count) * leftRowHeight
            + CGFloat(max(0, count - 1)) * leftRowSpacing
            + Layout.cardPadding * 2
        let height = min(naturalHeight, availableLeftHeight)
        let reservedRight = rightQuotaWidth + Layout.gapBetweenColumns
        let maxRoomForLeft = containerSize.width > 0
            ? containerSize.width - reservedRight - Layout.horizontalMargin * 2
            : Layout.leftCardMaxWidth
        let minLeft = min(Layout.leftCardMinWidth, max(Layout.leftCardCompactMinWidth, maxRoomForLeft))
        let upperWidth = min(Layout.leftCardMaxWidth, max(minLeft, maxRoomForLeft))
        let preferredWidth = containerSize.width > 0 ? maxRoomForLeft : Layout.leftCardMaxWidth
        let width = min(upperWidth, max(minLeft, preferredWidth))
        return CGSize(width: width, height: height)
    }

    public var leftCardRect: CGRect {
        let size = leftCardSize
        let reservedRight = rightQuotaWidth + Layout.gapBetweenColumns
        let maxLeftX = max(
            Layout.horizontalMargin,
            containerSize.width - size.width - reservedRight - Layout.horizontalMargin
        )
        // On phone widths (< 520pt), anchor near the left margin so sliding straight up from the
        // model chip (~x=106pt) naturally lands at ~40% width (Medium tier).
        let minX: CGFloat
        if containerSize.width > 0 && containerSize.width < 520 {
            minX = Layout.horizontalMargin
        } else if containerSize.width > 0 {
            minX = min(max(Layout.horizontalMargin, anchorRect.minX), maxLeftX)
        } else {
            minX = Layout.horizontalMargin
        }
        let minY = max(Layout.topSafeMargin, anchorRect.minY - Layout.gapBetweenButtonAndMenu - size.height)
        return CGRect(x: minX, y: minY, width: size.width, height: size.height)
    }

    public func leftRowCenterY(at index: Int) -> CGFloat {
        let rawY = leftCardRect.minY + Layout.cardPadding + CGFloat(index) * (leftRowHeight + leftRowSpacing) + leftRowHeight / 2
        return min(max(leftCardRect.minY + leftRowHeight / 2, rawY), leftCardRect.maxY - leftRowHeight / 2)
    }

    public func rightQuotaOrigin(cardHeight: CGFloat) -> CGPoint {
        let leftRect = leftCardRect
        let width = rightQuotaWidth
        let maxMinX = containerSize.width > 0
            ? max(Layout.horizontalMargin, containerSize.width - width - Layout.horizontalMargin)
            : leftRect.maxX + Layout.gapBetweenColumns
        let minX = min(leftRect.maxX + Layout.gapBetweenColumns, maxMinX)
        let minY = max(Layout.topSafeMargin, leftRect.maxY - cardHeight)
        return CGPoint(x: minX + width / 2, y: minY + cardHeight / 2)
    }

    // MARK: - Continuous Single-Column Inline Scrub Hit-Testing

    public func variantIndex(forRow rowIndex: Int, localX: CGFloat, trackWidth: CGFloat, currentVariant: Int?) -> Int {
        guard rows.indices.contains(rowIndex) else { return 0 }
        let count = rows[rowIndex].variantsTopToBottom.count
        guard count > 1, trackWidth > 1 else { return 0 }
        if localX <= 0 { return 0 }
        if localX >= trackWidth { return count - 1 }

        let segW = trackWidth / CGFloat(count)
        if let cur = currentVariant, cur >= 0 && cur < count {
            let curMin = CGFloat(cur) * segW - Layout.tierBoundaryHysteresis
            let curMax = CGFloat(cur + 1) * segW + Layout.tierBoundaryHysteresis
            if localX >= curMin && localX <= curMax {
                return cur
            }
        }
        return min(max(0, Int(floor(localX / segW))), count - 1)
    }

    public func triggerHaptic(for row: ModelPickerRow, variantIndex: Int) {
        if row.variantsTopToBottom.indices.contains(variantIndex),
           row.variantsTopToBottom[variantIndex].effortRank == .extra {
            xhighImpactHaptic.impactOccurred(intensity: 0.78)
        } else {
            selectionHaptic.selectionChanged()
        }
    }

    public func updateDrag(at point: CGPoint) {
        guard isPresented, !rows.isEmpty else { return }
        let leftRect = leftCardRect
        let prevPoint = lastDragPoint
        lastDragPoint = point

        // Cancel zone: back on trigger chip, below menu, far above, or far left
        if anchorRect.insetBy(dx: -16, dy: -12).contains(point)
            || point.y > leftRect.maxY + 18
            || point.y < leftRect.minY - 52
            || point.x < leftRect.minX - 48 {
            isHorizontalRowLocked = false
            clearHover()
            return
        }

        // Right overshoot stays active (clamped to max tier) so slamming XHigh never accidentally cancels
        let rightLimit = containerSize.width > 0 ? containerSize.width + 24 : leftRect.maxX + 180
        guard point.x >= leftRect.minX - 32 && point.x <= rightLimit else {
            isHorizontalRowLocked = false
            clearHover()
            return
        }

        hasEnteredMenu = true

        // Directional row-lock: prevent arcing thumb from slipping onto adjacent rows while scrubbing horizontally
        let resolvedRowIdx: Int
        if let curIdx = hoveredRowIndex, rows.indices.contains(curIdx), rows[curIdx].hasInlineEffort {
            if let prev = prevPoint {
                let dx = abs(point.x - prev.x)
                let dy = abs(point.y - prev.y)
                if dx > dy * 1.35 && dx > 2.0 {
                    isHorizontalRowLocked = true
                }
            }
            let centerY = leftRowCenterY(at: curIdx)
            let halfH = leftRowHeight * 0.5
            let verticalTolerance = isHorizontalRowLocked
                ? (halfH + Layout.horizontalRowLockVerticalPad)
                : (halfH + 1.5)
            if abs(point.y - centerY) <= verticalTolerance {
                resolvedRowIdx = curIdx
            } else {
                isHorizontalRowLocked = false
                let localY = point.y - (leftRect.minY + Layout.cardPadding)
                let step = leftRowHeight + leftRowSpacing
                resolvedRowIdx = min(max(0, Int(floor(localY / step))), rows.count - 1)
            }
        } else {
            isHorizontalRowLocked = false
            let localY = point.y - (leftRect.minY + Layout.cardPadding)
            let step = leftRowHeight + leftRowSpacing
            resolvedRowIdx = min(max(0, Int(floor(localY / step))), rows.count - 1)
        }

        let row = rows[resolvedRowIdx]
        let trackMinX = leftRect.minX + Layout.cardPadding
        let trackWidth = max(1, leftRect.width - Layout.cardPadding * 2)
        let localX = point.x - trackMinX
        let sameRow = (hoveredRowIndex == resolvedRowIdx)
        let nextVariantIdx = variantIndex(
            forRow: resolvedRowIdx,
            localX: localX,
            trackWidth: trackWidth,
            currentVariant: sameRow ? hoveredVariantIndex : nil
        )

        let changed = (hoveredRowIndex != resolvedRowIdx) || (hoveredVariantIndex != nextVariantIdx)
        hoveredRowIndex = resolvedRowIdx
        hoveredVariantIndex = nextVariantIdx
        if changed {
            triggerHaptic(for: row, variantIndex: nextVariantIdx)
        }
    }

    public func endDrag(onSelect: (String) -> Void) {
        guard isPresented else { return }
        lastDragPoint = nil
        isHorizontalRowLocked = false

        if let rowIdx = hoveredRowIndex, rows.indices.contains(rowIdx) {
            let row = rows[rowIdx]
            let variantIdx: Int
            if let vIdx = hoveredVariantIndex, row.variantsTopToBottom.indices.contains(vIdx) {
                variantIdx = vIdx
            } else {
                variantIdx = min(max(0, row.defaultVariantIndex), row.variantsTopToBottom.count - 1)
            }
            if row.variantsTopToBottom.indices.contains(variantIdx) {
                let chosen = row.variantsTopToBottom[variantIdx]
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                onSelect(chosen.id)
            }
            dismiss()
            return
        }

        if !hasEnteredMenu {
            // Held and released on trigger button without sliding up: stay open for tap/inline scrub
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }

    private func clearHover() {
        if hoveredRowIndex != nil || hoveredVariantIndex != nil {
            hoveredRowIndex = nil
            hoveredVariantIndex = nil
        }
    }
}

// MARK: - Quota Cache (Consolidated Gemini & Claude Pools Only)

/// 模型选择器额度伴随卡的只读缓存：由首页额度拉取（`publishQuotaToWidget`）同步当前账号额度，零额外请求。
@MainActor
@Observable
public final class ModelPickerQuotaCache {
    public static let shared = ModelPickerQuotaCache()
    public var account: CockpitAccountQuota? = nil

    private init() {}

    private static let claudeOrchestratorTokens = ["claude", "opus", "sonnet", "haiku", "anthropic"]

    public static func usesClaudeQuotaPool(for row: ModelPickerRow) -> Bool {
        if let explicit = row.variantsTopToBottom
            .compactMap({ $0.quotaPool?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() })
            .first(where: { !$0.isEmpty }) {
            if explicit == "claude" { return true }
            if explicit == "gemini" { return false }
        }
        let titleLower = row.title.lowercased()
        if claudeOrchestratorTokens.contains(where: { titleLower.contains($0) }) {
            return true
        }
        for variant in row.variantsTopToBottom {
            let idLower = variant.id.lowercased()
            let rawLower = variant.rawName.lowercased()
            if ModelDefaultsLogic.isClaude(idLower)
                || claudeOrchestratorTokens.contains(where: { idLower.contains($0) || rawLower.contains($0) }) {
                return true
            }
        }
        return false
    }

    public static func quotaPool(for row: ModelPickerRow) -> ModelQuotaPoolKind? {
        switch row.providerKind {
        case .claude:
            return .claude
        case .gemini:
            return .gemini
        case .orchestrator:
            return usesClaudeQuotaPool(for: row) ? .claude : .gemini
        case .other:
            return nil
        }
    }

    /// 兼容保留：返回该行对应共享池剩余百分比（0...100，仅 Gemini / Claude 双池）。
    public func remainingPercent(for row: ModelPickerRow, weekly: Bool) -> Double? {
        guard let acc = account, let pool = Self.quotaPool(for: row) else { return nil }
        let bucket: CockpitQuotaBucket?
        switch pool {
        case .claude:
            bucket = weekly ? acc.claudeWeekly : (acc.claude5h ?? acc.claudeWeekly)
        case .gemini:
            bucket = weekly ? acc.geminiWeekly : (acc.gemini5h ?? acc.geminiWeekly)
        }
        guard let pct = bucket?.remainingPercent, pct.isFinite else { return nil }
        return min(100, max(0, pct))
    }
}

public extension ModelProviderKind {
    /// 厂商专属强调色（Gemini 蓝 / Claude 橙 / Orchestrator 石墨钛灰 / Other 青）。
    var accentColor: Color {
        switch self {
        case .claude: return .orange
        case .orchestrator:
            return Color(
                uiColor: UIColor { traits in
                    traits.userInterfaceStyle == .dark
                        ? UIColor(red: 0xCB / 255.0, green: 0xD5 / 255.0, blue: 0xE1 / 255.0, alpha: 1.0)
                        : UIColor(red: 0x33 / 255.0, green: 0x41 / 255.0, blue: 0x55 / 255.0, alpha: 1.0)
                }
            )
        case .other: return .teal
        case .gemini: return .blue
        }
    }
}

// MARK: - Main Overlay View

public struct ModelSlidePickerOverlay: View {
    @Bindable var state: ModelSlidePickerState
    var isRunning: Bool = false
    let onSelectModel: (String) -> Void

    @Environment(\.colorScheme) private var colorScheme
    @State private var rightQuotaMeasuredHeight: CGFloat = 148

    private var isDark: Bool { colorScheme == .dark }

    private var shouldShowQuotaCard: Bool {
        guard AppSettings.shared.modelPickerQuotaEnabled,
              let acc = ModelPickerQuotaCache.shared.account else {
            return false
        }
        return acc.gemini5h != nil || acc.geminiWeekly != nil || acc.claude5h != nil || acc.claudeWeekly != nil
    }

    public var body: some View {
        if state.isPresented {
            ZStack(alignment: .topLeading) {
                // Backdrop aligned 1:1 with "chatRoot"
                Color.clear
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(
                        Color.black.opacity(state.isInteractiveTapMode ? 0.08 : 0.03)
                            .ignoresSafeArea()
                    )
                    .contentShape(Rectangle())
                    .onTapGesture {
                        state.dismiss()
                    }

                let leftRect = state.leftCardRect

                // Top floating HUD: live drag preview (prevents thumb occlusion) + running hint
                topFloatingPills(leftRect: leftRect)

                // Left column: Single-column models with inline effort scrubber
                leftColumnCard(size: leftRect.size)
                    .position(x: leftRect.midX, y: leftRect.midY)
                    .transition(
                        .asymmetric(
                            insertion: .opacity.combined(with: .scale(scale: 0.94, anchor: .bottomLeading)),
                            removal: .opacity
                        )
                    )

                // Right companion card: Consolidated Gemini & Claude quotas (5h + Weekly + pace cursor)
                if shouldShowQuotaCard, let acc = ModelPickerQuotaCache.shared.account {
                    let origin = state.rightQuotaOrigin(cardHeight: rightQuotaMeasuredHeight)
                    rightQuotaCompanionCard(account: acc, width: state.rightQuotaWidth)
                        .onGeometryChange(for: CGFloat.self) { proxy in
                            proxy.size.height
                        } action: { newHeight in
                            if newHeight > 20 {
                                rightQuotaMeasuredHeight = newHeight
                            }
                        }
                        .position(x: origin.x, y: origin.y)
                        .allowsHitTesting(false)
                        .transition(.opacity.combined(with: .scale(scale: 0.95, anchor: .bottomLeading)))
                }
            }
            .zIndex(500)
        }
    }

    @ViewBuilder
    private func topFloatingPills(leftRect: CGRect) -> some View {
        let showLivePreview = !state.isInteractiveTapMode
            && state.hoveredRowIndex != nil
            && state.rows.indices.contains(state.hoveredRowIndex ?? -1)

        VStack(alignment: .leading, spacing: 5) {
            if isRunning {
                HStack(spacing: 5) {
                    Image(systemName: "clock.arrow.circlepath")
                        .font(.system(size: 10.5, weight: .semibold))
                    Text("运行中 · 切换下轮生效")
                        .font(.system(size: 11, weight: .medium))
                }
                .foregroundColor(.orange)
                .padding(.horizontal, 10)
                .padding(.vertical, 4)
                .background(cardSurface(tint: .orange))
                .clipShape(Capsule())
                .overlay(Capsule().stroke(Color.orange.opacity(0.28), lineWidth: 0.8))
            }

            if showLivePreview,
               let rIdx = state.hoveredRowIndex,
               state.rows.indices.contains(rIdx) {
                let row = state.rows[rIdx]
                let vIdx = min(max(0, state.hoveredVariantIndex ?? row.defaultVariantIndex), max(0, row.variantsTopToBottom.count - 1))
                let variant = row.variantsTopToBottom.indices.contains(vIdx) ? row.variantsTopToBottom[vIdx] : nil
                let rank = variant?.effortRank
                let palette = EffortStylePalette.palette(for: row.providerKind, isDark: isDark)
                let effortText: String? = row.hasInlineEffort
                    ? (row.usesEffortLabels ? (rank?.shortLabel ?? variant?.effortDisplayLabel) : variant?.label)
                    : nil
                let usesDarkBackdrop = (rank == .high || rank == .extra)

                HStack(spacing: 6) {
                    Text(row.title)
                        .font(.system(size: 12, weight: .semibold))
                        .lineLimit(1)
                    if let effortText, !effortText.isEmpty {
                        Text("·")
                            .font(.system(size: 12, weight: .bold))
                            .opacity(0.65)
                        Text(effortText)
                            .font(.system(size: 11.5, weight: .bold))
                    }
                }
                .foregroundColor(usesDarkBackdrop ? .white : palette.accent)
                .padding(.horizontal, 11)
                .padding(.vertical, 5)
                .background {
                    if rank == .extra {
                        CosmicStarfieldView(palette: palette, animated: true)
                    } else if rank == .high {
                        LinearGradient(colors: palette.highColors, startPoint: .leading, endPoint: .trailing)
                    } else if rank == .medium {
                        ZStack {
                            cardSurface(tint: nil)
                            LinearGradient(colors: palette.medColors, startPoint: .leading, endPoint: .trailing)
                        }
                    } else {
                        ZStack {
                            cardSurface(tint: nil)
                            palette.lowFill
                        }
                    }
                }
                .clipShape(Capsule())
                .overlay(
                    Capsule().stroke(
                        rank == .extra
                            ? palette.glowColor.opacity(0.75)
                            : (rank == .high ? Color.white.opacity(0.28) : palette.accent.opacity(0.45)),
                        lineWidth: rank == .extra ? 1.1 : 0.85
                    )
                )
                .shadow(
                    color: rank == .extra ? palette.glowColor.opacity(0.35) : Color.black.opacity(0.12),
                    radius: rank == .extra ? 8 : 5,
                    x: 0,
                    y: 2
                )
            }
        }
        .fixedSize()
        .position(
            x: leftRect.midX,
            y: max(22, leftRect.minY - (isRunning && showLivePreview ? 32 : 18))
        )
        .allowsHitTesting(false)
        .animation(.easeOut(duration: 0.12), value: state.hoveredRowIndex)
        .animation(.easeOut(duration: 0.12), value: state.hoveredVariantIndex)
    }

    @ViewBuilder
    private func leftColumnCard(size: CGSize) -> some View {
        let content = VStack(alignment: .leading, spacing: state.leftRowSpacing) {
            ForEach(Array(state.rows.enumerated()), id: \.element.id) { index, row in
                inlineModelRow(index: index, row: row)
                    .overlay(alignment: .top) {
                        if row.isGroupStart {
                            Rectangle()
                                .fill(Color.primary.opacity(0.09))
                                .frame(height: 0.75)
                                .padding(.horizontal, 6)
                                .offset(y: -(state.leftRowSpacing / 2))
                        }
                    }
            }
        }
        .padding(ModelSlidePickerState.Layout.cardPadding)

        Group {
            if state.leftContentNeedsScroll {
                ScrollView(.vertical, showsIndicators: false) {
                    content
                }
            } else {
                content
            }
        }
        .frame(width: size.width, height: size.height)
        .background(cardSurface(tint: nil))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.15), radius: 14, x: 0, y: 6)
    }

    @ViewBuilder
    private func inlineModelRow(index: Int, row: ModelPickerRow) -> some View {
        let palette = EffortStylePalette.palette(for: row.providerKind, isDark: isDark)
        let isHovered = state.hoveredRowIndex == index
        let activeVIdx = state.activeVariantIndex(in: row)
        let isCurrentActive = activeVIdx != nil

        let displayVariantIdx: Int? = {
            if isHovered {
                return min(max(0, state.hoveredVariantIndex ?? row.defaultVariantIndex), max(0, row.variantsTopToBottom.count - 1))
            }
            if isCurrentActive {
                return activeVIdx
            }
            return nil
        }()

        let displayVariant: ModelVariant? = displayVariantIdx.flatMap {
            row.variantsTopToBottom.indices.contains($0) ? row.variantsTopToBottom[$0] : nil
        }
        let effortRank: ModelEffortRank? = displayVariant?.effortRank
        let count = row.variantsTopToBottom.count
        let fillFraction: CGFloat = {
            guard isHovered || isCurrentActive else { return 0 }
            if !row.hasInlineEffort { return 1.0 }
            return EffortStylePalette.fillFraction(index: displayVariantIdx ?? 0, count: count)
        }()

        let isRichDarkFill = isHovered && (effortRank == .high || effortRank == .extra)
        let isRestingCosmic = !isHovered && isCurrentActive && effortRank == .extra
        let titleColor: Color = {
            if isRichDarkFill || isRestingCosmic {
                return .white
            }
            if isHovered && effortRank == .medium && isDark {
                return .white
            }
            return palette.accent
        }()

        let effortBadgeText: String? = {
            guard row.hasInlineEffort, let v = displayVariant else { return nil }
            return row.usesEffortLabels ? (v.effortRank?.shortLabel ?? v.effortDisplayLabel) : v.label
        }()

        GeometryReader { geo in
            let rowWidth = geo.size.width
            let rowHeight = geo.size.height
            let fillWidth = max(0, min(rowWidth, rowWidth * fillFraction))

            ZStack(alignment: .leading) {
                // Unfilled track tint when focused on a multi-tier row
                if isHovered && row.hasInlineEffort {
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .fill(palette.accent.opacity(isDark ? 0.11 : 0.07))
                } else if isCurrentActive && !isHovered && !isRestingCosmic {
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .fill(palette.accent.opacity(isDark ? 0.13 : 0.08))
                }

                // Active/hovered effort progress fill layer
                if fillWidth > 0.5 {
                    effortFillView(
                        palette: palette,
                        rank: effortRank,
                        isHovered: isHovered,
                        isCurrentActive: isCurrentActive,
                        hasInlineEffort: row.hasInlineEffort
                    )
                    .frame(width: fillWidth, height: rowHeight)
                    .overlay(alignment: .trailing) {
                        // Sleek luminous leading edge cap when partially filled
                        if isHovered && row.hasInlineEffort && fillFraction < 0.99 {
                            RoundedRectangle(cornerRadius: 1.5, style: .continuous)
                                .fill(effortRank == .high ? Color.white.opacity(0.88) : palette.accent.opacity(0.85))
                                .frame(width: 2.5, height: max(12, rowHeight - 10))
                                .padding(.trailing, 1.5)
                        }
                    }
                    .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
                    .animation(.spring(response: 0.16, dampingFraction: 0.88), value: fillFraction)
                }

                // Foreground row content: checkmark + title + effort badge / idle dots
                HStack(spacing: 5) {
                    if isCurrentActive {
                        Image(systemName: "checkmark")
                            .font(.system(size: 10.5, weight: .bold))
                            .foregroundColor(titleColor)
                    }
                    let displayTitle = state.isCompactWidth ? (row.compactTitle ?? row.title) : row.title
                    Text(displayTitle)
                        .font(.system(
                            size: state.leftRowHeight < 29 ? 12.5 : 13.5,
                            weight: (isHovered || isCurrentActive) ? .semibold : .medium
                        ))
                        .foregroundColor(titleColor)
                        .shadow(color: (isRichDarkFill || isRestingCosmic) ? Color.black.opacity(0.28) : .clear, radius: 1, x: 0, y: 0.5)
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)

                    Spacer(minLength: 4)

                    if let badge = effortBadgeText {
                        let badgeOnFill = fillFraction >= 0.96 || isRestingCosmic
                        Text(badge)
                            .font(.system(size: 10.5, weight: effortRank == .extra ? .bold : .semibold))
                            .foregroundColor(
                                badgeOnFill && (isRichDarkFill || isRestingCosmic)
                                    ? .white
                                    : (isHovered && effortRank == .medium && isDark && fillFraction >= 0.96 ? .white : palette.accent)
                            )
                            .lineLimit(1)
                            .fixedSize()
                            .padding(.horizontal, 6)
                            .frame(height: 17)
                            .background(
                                Capsule().fill(
                                    badgeOnFill && (isRichDarkFill || isRestingCosmic)
                                        ? Color.white.opacity(0.22)
                                        : palette.accent.opacity(isDark ? 0.22 : 0.14)
                                )
                            )
                    } else if row.hasInlineEffort {
                        // Subtle idle dots hinting how many effort stops this model supports
                        HStack(spacing: 2.5) {
                            ForEach(0..<min(count, 4), id: \.self) { _ in
                                Circle()
                                    .fill(palette.accent.opacity(0.32))
                                    .frame(width: 3, height: 3)
                            }
                        }
                    }
                }
                .padding(.horizontal, 10)
                .padding(.bottom, (isHovered && row.hasInlineEffort) ? 3 : 0)
                .frame(width: rowWidth, height: rowHeight)

                // Bottom micro-node rail when hovered on an adjustable row
                if isHovered && row.hasInlineEffort {
                    let activeStop = displayVariantIdx ?? 0
                    HStack(spacing: 0) {
                        ForEach(0..<count, id: \.self) { stopIdx in
                            let isSelectedStop = stopIdx == activeStop
                            let isReached = stopIdx <= activeStop
                            let dotColor: Color = {
                                if isRichDarkFill {
                                    return isSelectedStop ? .white : (isReached ? Color.white.opacity(0.55) : palette.accent.opacity(0.35))
                                }
                                return isSelectedStop ? palette.accent : (isReached ? palette.accent.opacity(0.55) : palette.accent.opacity(0.24))
                            }()
                            Capsule()
                                .fill(dotColor)
                                .frame(width: isSelectedStop ? 12 : 4, height: isSelectedStop ? 3 : 2.5)
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .padding(.horizontal, 6)
                    .padding(.bottom, 2.5)
                    .frame(width: rowWidth, height: rowHeight, alignment: .bottom)
                    .allowsHitTesting(false)
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .stroke(
                        rowBorderColor(
                            palette: palette,
                            rank: effortRank,
                            isHovered: isHovered,
                            isCurrentActive: isCurrentActive
                        ),
                        lineWidth: isHovered ? (effortRank == .extra ? 1.35 : 1.15) : (isCurrentActive ? 0.9 : 0)
                    )
            )
            .shadow(
                color: (isHovered && effortRank == .extra) ? palette.glowColor.opacity(0.34) : .clear,
                radius: 7,
                x: 0,
                y: 0
            )
            .contentShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
            .onTapGesture(coordinateSpace: .local) { location in
                guard state.isInteractiveTapMode else { return }
                if !row.hasInlineEffort, let only = row.variantsTopToBottom.first {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onSelectModel(only.id)
                    state.dismiss()
                    return
                }
                if state.hoveredRowIndex == index {
                    let chosenIdx = state.variantIndex(
                        forRow: index,
                        localX: location.x,
                        trackWidth: rowWidth,
                        currentVariant: nil
                    )
                    if row.variantsTopToBottom.indices.contains(chosenIdx) {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSelectModel(row.variantsTopToBottom[chosenIdx].id)
                        state.dismiss()
                    }
                } else {
                    // First tap on unfocused multi-tier row: focus it at active/default tier so user
                    // doesn't accidentally commit Low by tapping the model name on the left.
                    let initialIdx = activeVIdx ?? row.defaultVariantIndex
                    state.hoveredRowIndex = index
                    state.hoveredVariantIndex = initialIdx
                    state.triggerHaptic(for: row, variantIndex: initialIdx)
                }
            }
            .highPriorityGesture(
                DragGesture(minimumDistance: 8)
                    .onChanged { value in
                        guard state.isInteractiveTapMode, row.hasInlineEffort else { return }
                        guard abs(value.translation.width) >= abs(value.translation.height) else { return }
                        let vIdx = state.variantIndex(
                            forRow: index,
                            localX: value.location.x,
                            trackWidth: rowWidth,
                            currentVariant: state.hoveredRowIndex == index ? state.hoveredVariantIndex : nil
                        )
                        let changed = state.hoveredRowIndex != index || state.hoveredVariantIndex != vIdx
                        state.hoveredRowIndex = index
                        state.hoveredVariantIndex = vIdx
                        if changed {
                            state.triggerHaptic(for: row, variantIndex: vIdx)
                        }
                    }
                    .onEnded { value in
                        guard state.isInteractiveTapMode, row.hasInlineEffort else { return }
                        guard abs(value.translation.width) >= abs(value.translation.height) else { return }
                        let chosenIdx = state.variantIndex(
                            forRow: index,
                            localX: value.location.x,
                            trackWidth: rowWidth,
                            currentVariant: nil
                        )
                        if row.variantsTopToBottom.indices.contains(chosenIdx) {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            onSelectModel(row.variantsTopToBottom[chosenIdx].id)
                            state.dismiss()
                        }
                    },
                including: state.isInteractiveTapMode && row.hasInlineEffort ? .gesture : .none
            )
        }
        .frame(maxWidth: .infinity)
        .frame(height: state.leftRowHeight)
    }

    @ViewBuilder
    private func effortFillView(
        palette: EffortStylePalette,
        rank: ModelEffortRank?,
        isHovered: Bool,
        isCurrentActive: Bool,
        hasInlineEffort: Bool
    ) -> some View {
        if !hasInlineEffort {
            palette.accent.opacity(isHovered ? (isDark ? 0.28 : 0.20) : (isDark ? 0.14 : 0.10))
        } else if !isHovered && isCurrentActive {
            if rank == .extra {
                CosmicStarfieldView(palette: palette, animated: false, isSubtleResting: true)
            } else if rank == .high {
                LinearGradient(colors: palette.highColors, startPoint: .leading, endPoint: .trailing)
                    .opacity(isDark ? 0.34 : 0.24)
            } else if rank == .medium {
                LinearGradient(colors: palette.medColors, startPoint: .leading, endPoint: .trailing)
                    .opacity(0.65)
            } else {
                palette.lowFill.opacity(0.75)
            }
        } else {
            switch rank {
            case .extra:
                CosmicStarfieldView(palette: palette, animated: true)
            case .high:
                LinearGradient(
                    colors: palette.highColors,
                    startPoint: .leading,
                    endPoint: .trailing
                )
            case .medium:
                LinearGradient(
                    colors: palette.medColors,
                    startPoint: .leading,
                    endPoint: .trailing
                )
            case .low, .none:
                palette.lowFill
            }
        }
    }

    private func rowBorderColor(
        palette: EffortStylePalette,
        rank: ModelEffortRank?,
        isHovered: Bool,
        isCurrentActive: Bool
    ) -> Color {
        if isHovered {
            if rank == .extra { return palette.glowColor.opacity(0.88) }
            if rank == .high { return palette.accent.opacity(0.85) }
            return palette.accent.opacity(0.70)
        }
        if isCurrentActive {
            if rank == .extra { return palette.glowColor.opacity(0.62) }
            return palette.accent.opacity(0.40)
        }
        return .clear
    }

    // MARK: - Right Companion Quota Card (Gemini + Claude, 5h + Weekly + Pace Cursor)

    @ViewBuilder
    private func rightQuotaCompanionCard(account: CockpitAccountQuota, width: CGFloat) -> some View {
        let highlighted = state.highlightedQuotaPool
        let hasHoverHighlight = state.hoveredRowIndex != nil && highlighted != nil
        let isFree = account.isFreeTier
        let showPace = AppSettings.shared.weeklyPaceCursorEnabled

        VStack(alignment: .leading, spacing: 6) {
            if account.gemini5h != nil || account.geminiWeekly != nil {
                quotaPoolModule(
                    title: "Gemini",
                    accent: .blue,
                    bucket5h: isFree ? nil : account.gemini5h,
                    bucketWeekly: account.geminiWeekly,
                    tier: account.subscriptionTier,
                    showPace: showPace,
                    isFocused: highlighted == .gemini,
                    isDimmed: hasHoverHighlight && highlighted != .gemini
                )
            }

            if account.claude5h != nil || account.claudeWeekly != nil {
                quotaPoolModule(
                    title: "Claude",
                    accent: .orange,
                    bucket5h: isFree ? nil : account.claude5h,
                    bucketWeekly: account.claudeWeekly,
                    tier: account.subscriptionTier,
                    showPace: showPace,
                    isFocused: highlighted == .claude,
                    isDimmed: hasHoverHighlight && highlighted != .claude
                )
            }
        }
        .padding(6)
        .frame(width: width)
        .background(cardSurface(tint: nil))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
        .animation(.easeOut(duration: 0.14), value: highlighted)
    }

    @ViewBuilder
    private func quotaPoolModule(
        title: String,
        accent: Color,
        bucket5h: CockpitQuotaBucket?,
        bucketWeekly: CockpitQuotaBucket?,
        tier: String?,
        showPace: Bool,
        isFocused: Bool,
        isDimmed: Bool
    ) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 4) {
                Circle()
                    .fill(accent)
                    .frame(width: 5.5, height: 5.5)
                Text(title)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(accent)
                    .lineLimit(1)
                Spacer(minLength: 2)
                if let resetShort = compactResetLabel(bucket5h?.resetFriendly) {
                    Text(resetShort)
                        .font(.system(size: 9, weight: .medium).monospacedDigit())
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
            }

            if let b5 = bucket5h {
                quotaMiniBarRow(
                    label: "5h",
                    bucket: b5,
                    accent: accent,
                    paceInfo: nil
                )
            }

            if let bw = bucketWeekly {
                let pace = showPace ? bw.calculateWeeklyPace(totalWindowHours: 168.0) : nil
                quotaMiniBarRow(
                    label: String(localized: "周"),
                    bucket: bw,
                    accent: accent,
                    paceInfo: pace
                )
            }
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 6)
        .background(
            RoundedRectangle(cornerRadius: 9, style: .continuous)
                .fill(isFocused ? accent.opacity(isDark ? 0.16 : 0.10) : Color.primary.opacity(isDark ? 0.05 : 0.03))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 9, style: .continuous)
                .stroke(isFocused ? accent.opacity(0.68) : Color.primary.opacity(0.06), lineWidth: isFocused ? 1.0 : 0.6)
        )
        .opacity(isDimmed ? 0.48 : 1.0)
    }

    @ViewBuilder
    private func quotaMiniBarRow(
        label: String,
        bucket: CockpitQuotaBucket,
        accent: Color,
        paceInfo: WeeklyPaceInfo?
    ) -> some View {
        let pct = min(100, max(0, bucket.remainingPercent.isFinite ? bucket.remainingPercent : 0))
        let isLow = pct < 20.0
        let barColor = isLow ? Color.red : accent

        VStack(alignment: .leading, spacing: 2.5) {
            HStack(spacing: 2) {
                Text(label)
                    .font(.system(size: 9.5, weight: .medium))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 2)
                Text("\(Int(pct.rounded()))%")
                    .font(.system(size: 10.5, weight: .semibold).monospacedDigit())
                    .foregroundColor(isLow ? .red : .primary)
                    .lineLimit(1)
            }

            GeometryReader { geo in
                let w = geo.size.width
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(Color.primary.opacity(isDark ? 0.14 : 0.09))
                        .frame(height: 4.5)

                    Capsule()
                        .fill(barColor)
                        .frame(width: max(0, min(w, w * CGFloat(pct / 100.0))), height: 4.5)

                    if let pace = paceInfo {
                        let expectedFrac = CGFloat(min(100, max(0, pace.expectedRemainingPercent)) / 100.0)
                        let needleX = max(0, min(w - 1.5, w * expectedFrac - 0.75))
                        let needleColor = pace.isOverPacing
                            ? Color.orange
                            : (isDark ? Color.white.opacity(0.92) : Color.primary.opacity(0.75))
                        RoundedRectangle(cornerRadius: 0.75, style: .continuous)
                            .fill(needleColor)
                            .frame(width: 1.5, height: 7.5)
                            .shadow(color: Color.black.opacity(0.32), radius: 0.8, x: 0, y: 0.3)
                            .offset(x: needleX)
                    }
                }
                .frame(height: 7.5)
            }
            .frame(height: 7.5)
        }
    }

    private func compactResetLabel(_ raw: String?) -> String? {
        guard let text = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !text.isEmpty else {
            return nil
        }
        let ignored = ["已就绪", "就绪", "未知", "配额充足", "ready", "unknown"]
        if ignored.contains(where: { text.caseInsensitiveCompare($0) == .orderedSame }) {
            return nil
        }
        let pattern = #"(\d+)\s*(天|日|d|小时|小時|h|分钟|分鐘|分|m)"#
        if let regex = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]),
           let match = regex.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)),
           let numRange = Range(match.range(at: 1), in: text),
           let unitRange = Range(match.range(at: 2), in: text) {
            let num = String(text[numRange])
            let rawUnit = text[unitRange].lowercased()
            let unit: String
            if rawUnit == "天" || rawUnit == "日" || rawUnit == "d" {
                unit = "d"
            } else if rawUnit == "小时" || rawUnit == "小時" || rawUnit == "h" {
                unit = "h"
            } else {
                unit = "m"
            }
            return "\(num)\(unit)"
        }
        return nil
    }

    @ViewBuilder
    private func cardSurface(tint: Color?) -> some View {
        ZStack {
            Rectangle().fill(.ultraThinMaterial)
            Color(uiColor: .systemBackground).opacity(0.82)
            if let tint {
                tint.opacity(0.08)
            }
        }
    }
}

// MARK: - Model Switch Chip Button (with Subtle XHigh Shimmer Border)

private enum ModelChipGesturePhase: Equatable {
    case inactive
    case pressing
    case active(CGPoint?)
}

public struct ModelSwitchChipButton: View {
    let displayName: String
    let foregroundColor: Color
    let backgroundColor: Color
    let borderColor: Color
    let providerKind: ModelProviderKind
    let activeModelID: String
    @Bindable var pickerState: ModelSlidePickerState
    let onToggle: () -> Void
    let onSelectModel: (String) -> Void

    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @GestureState private var gesturePhase: ModelChipGesturePhase = .inactive
    @State private var showLongPressTip: Bool = false

    public init(
        displayName: String,
        foregroundColor: Color,
        backgroundColor: Color,
        borderColor: Color,
        providerKind: ModelProviderKind,
        activeModelID: String,
        pickerState: ModelSlidePickerState,
        onToggle: @escaping () -> Void,
        onSelectModel: @escaping (String) -> Void
    ) {
        self.displayName = displayName
        self.foregroundColor = foregroundColor
        self.backgroundColor = backgroundColor
        self.borderColor = borderColor
        self.providerKind = providerKind
        self.activeModelID = activeModelID
        self.pickerState = pickerState
        self.onToggle = onToggle
        self.onSelectModel = onSelectModel
    }

    private var isXHighActive: Bool {
        ModelDefaultsLogic.effortRank(forID: activeModelID) == .extra
    }

    public var body: some View {
        let isDark = colorScheme == .dark
        let palette = EffortStylePalette.palette(for: providerKind, isDark: isDark)
        let isXHigh = isXHighActive

        let longPressThenDrag = LongPressGesture(minimumDuration: 0.20, maximumDistance: 12)
            .sequenced(before: DragGesture(minimumDistance: 0, coordinateSpace: .named("chatRoot")))
            .updating($gesturePhase) { value, state, _ in
                switch value {
                case .first(true):
                    state = .pressing
                case .second(true, let drag):
                    state = .active(drag?.location)
                default:
                    state = .inactive
                }
            }

        Text(displayName)
            .font(.system(size: 13, weight: isXHigh ? .semibold : .medium))
            .foregroundColor(foregroundColor)
            .padding(.horizontal, 11)
            .frame(height: 32)
            .background {
                if isXHigh {
                    LinearGradient(
                        colors: [
                            palette.cosmicColors.first?.opacity(isDark ? 0.34 : 0.14) ?? backgroundColor,
                            palette.glowColor.opacity(isDark ? 0.22 : 0.12)
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                } else {
                    backgroundColor
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay {
                if isXHigh {
                    xhighChipShimmerBorder(palette: palette)
                } else {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(borderColor, lineWidth: 1)
                }
            }
            .shadow(
                color: isXHigh ? palette.glowColor.opacity(isDark ? 0.36 : 0.26) : .clear,
                radius: isXHigh ? 4.5 : 0,
                x: 0,
                y: 0
            )
            .scaleEffect(gesturePhase == .pressing ? 0.96 : 1.0)
            .animation(.easeOut(duration: 0.12), value: gesturePhase == .pressing)
            .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .onGeometryChange(for: CGRect.self) { proxy in
                proxy.frame(in: .named("chatRoot"))
            } action: { newRect in
                pickerState.anchorRect = newRect
            }
            .overlay(alignment: .topLeading) {
                if showLongPressTip {
                    VStack(alignment: .leading, spacing: 0) {
                        HStack(spacing: 5) {
                            Image(systemName: "hand.tap.fill")
                                .font(.system(size: 10, weight: .semibold))
                            Text(String(localized: "长按上滑可切换更多模型"))
                                .font(.system(size: 11, weight: .medium))
                        }
                        .foregroundColor(.white)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 6)
                        .background(
                            RoundedRectangle(cornerRadius: 8, style: .continuous)
                                .fill(Color(uiColor: .darkGray).opacity(0.92))
                                .shadow(color: .black.opacity(0.18), radius: 6, x: 0, y: 3)
                        )

                        Image(systemName: "arrowtriangle.down.fill")
                            .font(.system(size: 7))
                            .foregroundColor(Color(uiColor: .darkGray).opacity(0.92))
                            .offset(x: 14, y: -1)
                    }
                    .offset(y: -36)
                    .fixedSize()
                    .contentShape(Rectangle())
                    .onTapGesture {
                        dismissLongPressTip()
                    }
                    .transition(.asymmetric(
                        insertion: .scale(scale: 0.85, anchor: .bottomLeading).combined(with: .opacity),
                        removal: .opacity
                    ))
                    .zIndex(200)
                }
            }
            .onAppear {
                let hasSeen = UserDefaults.standard.bool(forKey: "has_seen_model_long_press_tooltip")
                if !hasSeen {
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.6) {
                        withAnimation(.spring(response: 0.38, dampingFraction: 0.75)) {
                            showLongPressTip = true
                        }
                        DispatchQueue.main.asyncAfter(deadline: .now() + 5.0) {
                            dismissLongPressTip()
                        }
                    }
                }
            }
            .onTapGesture {
                dismissLongPressTip()
                if pickerState.isPresented {
                    pickerState.dismiss()
                } else {
                    onToggle()
                }
            }
            .gesture(longPressThenDrag)
            .onChange(of: gesturePhase) { oldPhase, newPhase in
                switch newPhase {
                case .active(let point):
                    dismissLongPressTip()
                    if !pickerState.isPresented {
                        pickerState.open(activeModel: activeModelID)
                    }
                    if let point {
                        pickerState.updateDrag(at: point)
                    }
                case .inactive:
                    if case .active = oldPhase {
                        pickerState.endDrag(onSelect: onSelectModel)
                    }
                case .pressing:
                    dismissLongPressTip()
                }
            }
            .onChange(of: pickerState.isPresented) { _, presented in
                if presented {
                    dismissLongPressTip()
                }
            }
            .accessibilityLabel("切换模型，当前 \(displayName)，长按上滑并左右滑动调节模型与思考强度")
    }

    @ViewBuilder
    private func xhighChipShimmerBorder(palette: EffortStylePalette) -> some View {
        if reduceMotion {
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(
                    LinearGradient(
                        colors: palette.chipRimColors,
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    ),
                    lineWidth: 1.35
                )
        } else {
            TimelineView(.animation(minimumInterval: 1.0 / 24.0)) { timeline in
                let t = timeline.date.timeIntervalSinceReferenceDate
                let angle = Angle.degrees((t * 48.0).truncatingRemainder(dividingBy: 360.0))
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(
                        AngularGradient(
                            gradient: Gradient(colors: palette.chipRimColors),
                            center: .center,
                            angle: angle
                        ),
                        lineWidth: 1.35
                    )
            }
        }
    }

    private func dismissLongPressTip() {
        guard showLongPressTip else { return }
        withAnimation(.easeOut(duration: 0.18)) {
            showLongPressTip = false
        }
        UserDefaults.standard.set(true, forKey: "has_seen_model_long_press_tooltip")
    }
}
