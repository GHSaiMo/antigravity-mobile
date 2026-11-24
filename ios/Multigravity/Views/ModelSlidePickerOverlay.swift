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
}

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

    public enum Layout {
        static let defaultRowHeight: CGFloat = 34
        static let compactRowHeight: CGFloat = 30
        static let minRowHeight: CGFloat = 24
        static let defaultRowSpacing: CGFloat = 3
        static let compactRowSpacing: CGFloat = 2
        static let cardPadding: CGFloat = 5
        static let leftCardCompactMinWidth: CGFloat = 148
        static let leftCardMinWidth: CGFloat = 186
        static let leftCardMaxWidth: CGFloat = 226
        static let rightCardWidth: CGFloat = 104
        static let compactWidthThreshold: CGFloat = 420
        static let gapBetweenButtonAndMenu: CGFloat = 8
        static let gapBetweenColumns: CGFloat = 8
        static let horizontalMargin: CGFloat = 12
        static let topSafeMargin: CGFloat = 12
        static let columnLockHysteresis: CGFloat = 6
    }

    public var isCompactWidth: Bool {
        containerSize.width <= 0 || containerSize.width < Layout.compactWidthThreshold
    }

    public init() {}

    /// 长按打开全模型一级菜单：按 Gemini(蓝) -> Claude(橙) -> Orchestrator(钛灰) -> 其他 列出全部可用模型。
    public func open(activeModel: String) {
        let families = ModelCatalogStore.shared.modelFamilies()
        var builtRows: [ModelPickerRow] = []
        var prevProvider: ModelProviderKind? = nil

        for family in families {
            let kind = ModelDefaultsLogic.providerKind(forID: family.variants.first?.id ?? family.provider)
            let isFirstInGroup = prevProvider != nil && prevProvider != kind

            if !family.hasEffortVariants && family.variants.count > 1 && kind != .orchestrator {
                // Multi-variant non-effort family (non-Orchestrator): flatten into direct level-1 rows
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
                let compactTitle: String?
                if kind == .orchestrator {
                    if ordered.count > 1 {
                        fullTitle = family.name.isEmpty ? "Orchestrator" : family.name
                        compactTitle = "Orch"
                    } else {
                        let raw = ordered.first?.rawName.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                        fullTitle = raw.isEmpty ? (family.name.isEmpty ? "Orchestrator" : family.name) : raw
                        let short = ModelDefaultsLogic.formatFullNameToCompact(fullTitle)
                        compactTitle = short.isEmpty ? "Orch" : short
                    }
                } else {
                    fullTitle = family.name
                    compactTitle = nil
                }
                builtRows.append(
                    ModelPickerRow(
                        id: family.id,
                        title: fullTitle,
                        compactTitle: compactTitle,
                        providerKind: kind,
                        isGroupStart: isFirstInGroup,
                        variantsTopToBottom: ordered,
                        defaultVariantIndex: family.defaultVariantIndexTopToBottom,
                        hasSubmenu: ordered.count > 1,
                        usesEffortLabels: family.hasEffortVariants
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
        self.selectionHaptic.prepare()
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

    public var rightRowHeight: CGFloat { Layout.defaultRowHeight }
    public var rightRowSpacing: CGFloat { Layout.defaultRowSpacing }

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
        let maxRoomForLeft = containerSize.width > 0
            ? containerSize.width - Layout.rightCardWidth - Layout.gapBetweenColumns - Layout.horizontalMargin * 2
            : Layout.leftCardMaxWidth
        let minLeft = min(Layout.leftCardMinWidth, max(Layout.leftCardCompactMinWidth, maxRoomForLeft))
        let upperWidth = min(Layout.leftCardMaxWidth, max(minLeft, maxRoomForLeft))
        let preferredWidth = containerSize.width > 0 ? containerSize.width * 0.54 : minLeft
        let width = min(upperWidth, max(minLeft, preferredWidth))
        return CGSize(width: width, height: height)
    }

    public var leftCardRect: CGRect {
        let size = leftCardSize
        let maxLeftX = max(
            Layout.horizontalMargin,
            containerSize.width - size.width - Layout.rightCardWidth - Layout.gapBetweenColumns - Layout.horizontalMargin
        )
        let minX = containerSize.width > 0
            ? min(max(Layout.horizontalMargin, anchorRect.minX), maxLeftX)
            : max(Layout.horizontalMargin, anchorRect.minX)
        let minY = max(Layout.topSafeMargin, anchorRect.minY - Layout.gapBetweenButtonAndMenu - size.height)
        return CGRect(x: minX, y: minY, width: size.width, height: size.height)
    }

    public func leftRowCenterY(at index: Int) -> CGFloat {
        let rawY = leftCardRect.minY + Layout.cardPadding + CGFloat(index) * (leftRowHeight + leftRowSpacing) + leftRowHeight / 2
        return min(max(leftCardRect.minY + leftRowHeight / 2, rawY), leftCardRect.maxY - leftRowHeight / 2)
    }

    public func rightCardRect(for rowIndex: Int) -> CGRect? {
        guard rows.indices.contains(rowIndex), rows[rowIndex].hasSubmenu else { return nil }
        let row = rows[rowIndex]
        let count = row.variantsTopToBottom.count
        guard count > 1 else { return nil }

        let height = CGFloat(count) * rightRowHeight
            + CGFloat(max(0, count - 1)) * rightRowSpacing
            + Layout.cardPadding * 2
        let anchorIdx = min(max(0, row.defaultVariantIndex), count - 1)
        let anchorOffset = Layout.cardPadding + CGFloat(anchorIdx) * (rightRowHeight + rightRowSpacing) + rightRowHeight / 2
        let rawMinY = leftRowCenterY(at: rowIndex) - anchorOffset
        let maxMinY = containerSize.height > 0 ? max(Layout.topSafeMargin, containerSize.height - height - 6) : rawMinY
        let minY = min(max(Layout.topSafeMargin, rawMinY), maxMinY)
        let minX = leftCardRect.maxX + Layout.gapBetweenColumns
        return CGRect(x: minX, y: minY, width: Layout.rightCardWidth, height: height)
    }

    // MARK: - Continuous Drag Hit-Testing

    public func updateDrag(at point: CGPoint) {
        guard isPresented, !rows.isEmpty else { return }
        let leftRect = leftCardRect

        if isInRightColumn, let rowIdx = hoveredRowIndex, let rightRect = rightCardRect(for: rowIdx) {
            if point.x < leftRect.maxX - Layout.columnLockHysteresis {
                isInRightColumn = false
                // Fall through to left column evaluation
            } else if point.x > rightRect.maxX + 64 || point.y < rightRect.minY - 56 || point.y > rightRect.maxY + 56 {
                clearHover()
                return
            } else {
                let count = rows[rowIdx].variantsTopToBottom.count
                let localY = point.y - (rightRect.minY + Layout.cardPadding)
                let step = rightRowHeight + rightRowSpacing
                let vIdx = min(max(0, Int(floor(localY / step))), count - 1)
                if hoveredVariantIndex != vIdx {
                    hoveredVariantIndex = vIdx
                    selectionHaptic.selectionChanged()
                }
                return
            }
        }

        // Check if finger slid horizontally right from an active multi-variant row into the effort column
        if let rowIdx = hoveredRowIndex,
           rows[rowIdx].hasSubmenu,
           let rightRect = rightCardRect(for: rowIdx),
           point.x > leftRect.maxX + 2,
           point.x <= rightRect.maxX + 48,
           point.y >= rightRect.minY - 24,
           point.y <= rightRect.maxY + 24 {
            isInRightColumn = true
            hasEnteredMenu = true
            let count = rows[rowIdx].variantsTopToBottom.count
            let localY = point.y - (rightRect.minY + Layout.cardPadding)
            let step = rightRowHeight + rightRowSpacing
            let vIdx = min(max(0, Int(floor(localY / step))), count - 1)
            if hoveredVariantIndex != vIdx {
                hoveredVariantIndex = vIdx
                selectionHaptic.selectionChanged()
            }
            return
        }

        // Left column hit zone (generous horizontal/top tolerance so sliding up feels effortless)
        let leftHitRect = CGRect(
            x: leftRect.minX - 28,
            y: leftRect.minY - 24,
            width: leftRect.width + 40,
            height: leftRect.height + 30
        )
        if leftHitRect.contains(point) {
            hasEnteredMenu = true
            isInRightColumn = false
            let localY = point.y - (leftRect.minY + Layout.cardPadding)
            let step = leftRowHeight + leftRowSpacing
            let idx = min(max(0, Int(floor(localY / step))), rows.count - 1)
            if hoveredRowIndex != idx {
                hoveredRowIndex = idx
                hoveredVariantIndex = rows[idx].defaultVariantIndex
                selectionHaptic.selectionChanged()
            } else if hoveredVariantIndex != rows[idx].defaultVariantIndex {
                hoveredVariantIndex = rows[idx].defaultVariantIndex
            }
            return
        }

        // Finger still on the trigger button: keep hovered nil so lifting enters tap mode
        if anchorRect.insetBy(dx: -16, dy: -12).contains(point) {
            isInRightColumn = false
            clearHover()
            return
        }

        isInRightColumn = false
        clearHover()
    }

    public func endDrag(onSelect: (String) -> Void) {
        guard isPresented else { return }
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
            // User held and released on the button without sliding up: stay open for tap selection
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

/// 模型选择器行内余量的只读缓存：由首页额度拉取（`publishQuotaToWidget`）同步当前账号额度，零额外请求。
@MainActor
@Observable
public final class ModelPickerQuotaCache {
    public static let shared = ModelPickerQuotaCache()
    public var account: CockpitAccountQuota? = nil

    private init() {}

    /// 返回该行对应额度池剩余百分比（0...100）；无数据或无法归属时返回 nil。
    /// Gemini / Orchestrator 归 Gemini 池（Argon 若网关单独下发则用 Argon 桶），Claude 归 Claude 池。
    public func remainingPercent(for row: ModelPickerRow, weekly: Bool) -> Double? {
        guard let acc = account else { return nil }
        let bucket: CockpitQuotaBucket?
        switch row.providerKind {
        case .claude:
            bucket = weekly ? acc.claudeWeekly : (acc.claude5h ?? acc.claudeWeekly)
        case .gemini, .orchestrator:
            let isArgon = row.title.lowercased().contains("argon")
                || row.variantsTopToBottom.contains { $0.id.lowercased().contains("argon") }
            if isArgon, let argon = acc.gemini4Argon {
                bucket = argon
            } else {
                bucket = weekly ? acc.geminiWeekly : (acc.gemini5h ?? acc.geminiWeekly)
            }
        case .other:
            bucket = nil
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

public struct ModelSlidePickerOverlay: View {
    @Bindable var state: ModelSlidePickerState
    var isRunning: Bool = false
    let onSelectModel: (String) -> Void

    private func accentColor(for kind: ModelProviderKind) -> Color {
        kind.accentColor
    }

    private func quotaPercent(for row: ModelPickerRow) -> Double? {
        let settings = AppSettings.shared
        guard settings.modelPickerQuotaEnabled else { return nil }
        return ModelPickerQuotaCache.shared.remainingPercent(for: row, weekly: settings.modelPickerQuotaWeekly)
    }

    public var body: some View {
        if state.isPresented {
            ZStack(alignment: .topLeading) {
                // Keep ZStack origin aligned 1:1 with "chatRoot" (without expanding into safe areas)
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

                // Left upward menu: All models color-coded by provider (Blue / Orange / Purple)
                let leftRect = state.leftCardRect
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
                    .position(x: leftRect.midX, y: max(18, leftRect.minY - 16))
                    .allowsHitTesting(false)
                    .transition(.opacity)
                }
                leftColumnCard(size: leftRect.size)
                    .position(x: leftRect.midX, y: leftRect.midY)
                    .transition(
                        .asymmetric(
                            insertion: .opacity.combined(with: .scale(scale: 0.94, anchor: .bottomLeading)),
                            removal: .opacity
                        )
                    )

                // Right column: Thinking intensity Low -> Medium -> High -> Extra (top to bottom), Medium aligned to hovered row
                if let hoveredIdx = state.hoveredRowIndex,
                   state.rows.indices.contains(hoveredIdx),
                   let rightRect = state.rightCardRect(for: hoveredIdx) {
                    let row = state.rows[hoveredIdx]
                    rightColumnCard(for: row, rowIndex: hoveredIdx, width: rightRect.width)
                        .position(x: rightRect.midX, y: rightRect.midY)
                        .transition(.opacity.combined(with: .scale(scale: 0.95, anchor: .leading)))
                }
            }
            .zIndex(500)
        }
    }

    @ViewBuilder
    private func leftColumnCard(size: CGSize) -> some View {
        let content = VStack(alignment: .leading, spacing: state.leftRowSpacing) {
            ForEach(Array(state.rows.enumerated()), id: \.element.id) { index, row in
                let rowColor = accentColor(for: row.providerKind)
                let isHovered = state.hoveredRowIndex == index
                let isCurrentActive = row.variantsTopToBottom.contains(where: {
                    $0.id.caseInsensitiveCompare(state.activeModelID) == .orderedSame
                })

                Button {
                    if !row.hasSubmenu, let only = row.variantsTopToBottom.first {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSelectModel(only.id)
                        state.dismiss()
                    } else if state.hoveredRowIndex == index {
                        // Second tap on same family picks default (Medium)
                        let defIdx = min(max(0, row.defaultVariantIndex), row.variantsTopToBottom.count - 1)
                        if row.variantsTopToBottom.indices.contains(defIdx) {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            onSelectModel(row.variantsTopToBottom[defIdx].id)
                            state.dismiss()
                        }
                    } else {
                        UISelectionFeedbackGenerator().selectionChanged()
                        state.hoveredRowIndex = index
                        state.hoveredVariantIndex = row.defaultVariantIndex
                    }
                } label: {
                    HStack(spacing: 6) {
                        if isCurrentActive {
                            Image(systemName: "checkmark")
                                .font(.system(size: 11, weight: .bold))
                                .foregroundColor(rowColor)
                        }
                        let displayTitle = state.isCompactWidth ? (row.compactTitle ?? row.title) : row.title
                        Text(displayTitle)
                            .font(.system(size: state.leftRowHeight < 29 ? 12.5 : 13.5, weight: (isHovered || isCurrentActive) ? .semibold : .medium))
                            .foregroundColor(rowColor)
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                        Spacer(minLength: 4)
                        if let pct = quotaPercent(for: row) {
                            let pctColor = pct < 20 ? Color.red : rowColor
                            Text("\(Int(pct.rounded()))%")
                                .font(.system(size: 10.5, weight: .semibold).monospacedDigit())
                                .foregroundColor(pctColor.opacity(isHovered || isCurrentActive ? 1.0 : 0.85))
                                .lineLimit(1)
                                .fixedSize()
                                .padding(.horizontal, 5)
                                .frame(height: 16)
                                .background(Capsule().fill(pctColor.opacity(0.12)))
                        }
                        if row.hasSubmenu {
                            Image(systemName: "chevron.right")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(rowColor.opacity(isHovered ? 0.95 : 0.65))
                        }
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity)
                    .frame(height: state.leftRowHeight)
                    .background(rowBackground(color: rowColor, isHovered: isHovered, isCurrentActive: isCurrentActive))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(rowBorderColor(color: rowColor, isHovered: isHovered, isCurrentActive: isCurrentActive), lineWidth: isHovered ? 1.25 : 1)
                    )
                }
                .buttonStyle(.plain)
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
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }

    @ViewBuilder
    private func rightColumnCard(for row: ModelPickerRow, rowIndex: Int, width: CGFloat) -> some View {
        let rowColor = accentColor(for: row.providerKind)
        VStack(alignment: .leading, spacing: state.rightRowSpacing) {
            ForEach(Array(row.variantsTopToBottom.enumerated()), id: \.element.id) { vIdx, variant in
                let isHovered = state.hoveredRowIndex == rowIndex && state.hoveredVariantIndex == vIdx
                let isCurrentActive = variant.id.caseInsensitiveCompare(state.activeModelID) == .orderedSame
                let label = row.usesEffortLabels ? variant.effortDisplayLabel : variant.label

                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onSelectModel(variant.id)
                    state.dismiss()
                } label: {
                    HStack(spacing: 4) {
                        Text(label)
                            .font(.system(size: 13.5, weight: (isHovered || isCurrentActive) ? .semibold : .medium))
                            .foregroundColor(rowColor)
                            .lineLimit(1)
                        Spacer(minLength: 2)
                        if isCurrentActive {
                            Image(systemName: "checkmark")
                                .font(.system(size: 10.5, weight: .bold))
                                .foregroundColor(rowColor)
                        }
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity)
                    .frame(height: state.rightRowHeight)
                    .background(rowBackground(color: rowColor, isHovered: isHovered, isCurrentActive: isCurrentActive))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(rowBorderColor(color: rowColor, isHovered: isHovered, isCurrentActive: isCurrentActive), lineWidth: isHovered ? 1.25 : 1)
                    )
                }
                .buttonStyle(.plain)
            }
        }
        .padding(ModelSlidePickerState.Layout.cardPadding)
        .frame(width: width)
        .background(cardSurface(tint: rowColor))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(rowColor.opacity(0.35), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }

    @ViewBuilder
    private func cardSurface(tint: Color?) -> some View {
        ZStack {
            Rectangle().fill(.ultraThinMaterial)
            Color(uiColor: .systemBackground).opacity(0.80)
            if let tint {
                tint.opacity(0.08)
            }
        }
    }

    private func rowBackground(color: Color, isHovered: Bool, isCurrentActive: Bool) -> Color {
        if isHovered {
            return color.opacity(0.24)
        }
        if isCurrentActive {
            return color.opacity(0.11)
        }
        return Color.clear
    }

    private func rowBorderColor(color: Color, isHovered: Bool, isCurrentActive: Bool) -> Color {
        if isHovered {
            return color.opacity(0.80)
        }
        if isCurrentActive {
            return color.opacity(0.42)
        }
        return Color.clear
    }
}

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

    public var body: some View {
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
            .font(.system(size: 13, weight: .medium))
            .foregroundColor(foregroundColor)
            .padding(.horizontal, 11)
            .frame(height: 32)
            .background(backgroundColor)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(borderColor, lineWidth: 1)
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
            .accessibilityLabel("切换模型，当前 \(displayName)，长按上滑选择模型与思考强度")
    }

    private func dismissLongPressTip() {
        guard showLongPressTip else { return }
        withAnimation(.easeOut(duration: 0.18)) {
            showLongPressTip = false
        }
        UserDefaults.standard.set(true, forKey: "has_seen_model_long_press_tooltip")
    }
}
