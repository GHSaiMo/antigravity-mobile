import SwiftUI
import UIKit

public struct ModelPickerRow: Identifiable, Equatable {
    public let id: String
    public let title: String
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
        static let rowHeight: CGFloat = 36
        static let rowSpacing: CGFloat = 4
        static let cardPadding: CGFloat = 5
        static let leftCardMinWidth: CGFloat = 176
        static let leftCardMaxWidth: CGFloat = 220
        static let rightCardWidth: CGFloat = 112
        static let gapBetweenButtonAndMenu: CGFloat = 8
        static let gapBetweenColumns: CGFloat = 8
        static let horizontalMargin: CGFloat = 12
        static let topSafeMargin: CGFloat = 12
        static let columnLockHysteresis: CGFloat = 6
    }

    public init() {}

    private static func displayMenuTitle(for rawTitle: String) -> String {
        let trimmed = rawTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        for prefix in ["Gemini ", "Claude "] {
            if trimmed.count > prefix.count,
               trimmed.lowercased().hasPrefix(prefix.lowercased()) {
                return String(trimmed.dropFirst(prefix.count))
            }
        }
        return trimmed
    }

    public func open(provider: ModelProviderKind, activeModel: String) {
        let families = ModelCatalogStore.shared.modelFamilies(for: provider.rawValue)
        let builtRows: [ModelPickerRow]
        if families.count == 1,
           let only = families.first,
           !only.hasEffortVariants,
           only.variants.count > 1 {
            // Single non-effort family (e.g. Orchestrator Carbon / Argon): show options directly in upward menu
            builtRows = only.variants.map { variant in
                ModelPickerRow(
                    id: variant.id,
                    title: Self.displayMenuTitle(for: variant.rawName),
                    variantsTopToBottom: [variant],
                    defaultVariantIndex: 0,
                    hasSubmenu: false,
                    usesEffortLabels: false
                )
            }
        } else {
            builtRows = families.map { family in
                let ordered = family.variantsTopToBottom
                return ModelPickerRow(
                    id: family.id,
                    title: Self.displayMenuTitle(for: family.name),
                    variantsTopToBottom: ordered,
                    defaultVariantIndex: family.defaultVariantIndexTopToBottom,
                    hasSubmenu: ordered.count > 1,
                    usesEffortLabels: family.hasEffortVariants
                )
            }
        }
        guard !builtRows.isEmpty else { return }

        self.providerKind = provider
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

    public var leftCardSize: CGSize {
        let count = max(rows.count, 1)
        let height = CGFloat(count) * Layout.rowHeight
            + CGFloat(max(0, count - 1)) * Layout.rowSpacing
            + Layout.cardPadding * 2
        let preferredWidth = containerSize.width > 0 ? containerSize.width * 0.52 : Layout.leftCardMinWidth
        let width = min(Layout.leftCardMaxWidth, max(Layout.leftCardMinWidth, preferredWidth))
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
        leftCardRect.minY + Layout.cardPadding + CGFloat(index) * (Layout.rowHeight + Layout.rowSpacing) + Layout.rowHeight / 2
    }

    public func rightCardRect(for rowIndex: Int) -> CGRect? {
        guard rows.indices.contains(rowIndex), rows[rowIndex].hasSubmenu else { return nil }
        let row = rows[rowIndex]
        let count = row.variantsTopToBottom.count
        guard count > 1 else { return nil }

        let height = CGFloat(count) * Layout.rowHeight
            + CGFloat(max(0, count - 1)) * Layout.rowSpacing
            + Layout.cardPadding * 2
        let anchorIdx = min(max(0, row.defaultVariantIndex), count - 1)
        let anchorOffset = Layout.cardPadding + CGFloat(anchorIdx) * (Layout.rowHeight + Layout.rowSpacing) + Layout.rowHeight / 2
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
                let step = Layout.rowHeight + Layout.rowSpacing
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
            let step = Layout.rowHeight + Layout.rowSpacing
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
            let step = Layout.rowHeight + Layout.rowSpacing
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

public struct ModelSlidePickerOverlay: View {
    @Bindable var state: ModelSlidePickerState
    let onSelectModel: (String) -> Void

    private var accentColor: Color {
        switch state.providerKind {
        case .claude: return .orange
        case .orchestrator: return .purple
        case .other: return .teal
        case .gemini: return .blue
        }
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

                // Left upward menu: Model families
                let leftRect = state.leftCardRect
                leftColumnCard(width: leftRect.width)
                    .position(x: leftRect.midX, y: leftRect.midY)
                    .transition(
                        .asymmetric(
                            insertion: .opacity.combined(with: .scale(scale: 0.94, anchor: .bottomLeading)),
                            removal: .opacity
                        )
                    )

                // Right column: Thinking intensity (Low -> Medium -> High -> Extra bottom-to-top, Medium aligned to hovered row)
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
    private func leftColumnCard(width: CGFloat) -> some View {
        VStack(alignment: .leading, spacing: ModelSlidePickerState.Layout.rowSpacing) {
            ForEach(Array(state.rows.enumerated()), id: \.element.id) { index, row in
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
                                .foregroundColor(accentColor)
                        }
                        Text(row.title)
                            .font(.system(size: 13.5, weight: (isHovered || isCurrentActive) ? .semibold : .medium))
                            .foregroundColor(accentColor)
                            .lineLimit(1)
                            .minimumScaleFactor(0.85)
                        Spacer(minLength: 4)
                        if row.hasSubmenu {
                            Image(systemName: "chevron.right")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(accentColor.opacity(isHovered ? 0.95 : 0.65))
                        }
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity)
                    .frame(height: ModelSlidePickerState.Layout.rowHeight)
                    .background(rowBackground(isHovered: isHovered, isCurrentActive: isCurrentActive))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(rowBorderColor(isHovered: isHovered, isCurrentActive: isCurrentActive), lineWidth: isHovered ? 1.25 : 1)
                    )
                }
                .buttonStyle(.plain)
            }
        }
        .padding(ModelSlidePickerState.Layout.cardPadding)
        .frame(width: width)
        .background(cardSurface)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(accentColor.opacity(0.35), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }

    @ViewBuilder
    private func rightColumnCard(for row: ModelPickerRow, rowIndex: Int, width: CGFloat) -> some View {
        VStack(alignment: .leading, spacing: ModelSlidePickerState.Layout.rowSpacing) {
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
                            .foregroundColor(accentColor)
                            .lineLimit(1)
                        Spacer(minLength: 2)
                        if isCurrentActive {
                            Image(systemName: "checkmark")
                                .font(.system(size: 10.5, weight: .bold))
                                .foregroundColor(accentColor)
                        }
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity)
                    .frame(height: ModelSlidePickerState.Layout.rowHeight)
                    .background(rowBackground(isHovered: isHovered, isCurrentActive: isCurrentActive))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(rowBorderColor(isHovered: isHovered, isCurrentActive: isCurrentActive), lineWidth: isHovered ? 1.25 : 1)
                    )
                }
                .buttonStyle(.plain)
            }
        }
        .padding(ModelSlidePickerState.Layout.cardPadding)
        .frame(width: width)
        .background(cardSurface)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(accentColor.opacity(0.35), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }

    @ViewBuilder
    private var cardSurface: some View {
        ZStack {
            Rectangle().fill(.ultraThinMaterial)
            Color(uiColor: .systemBackground).opacity(0.76)
            accentColor.opacity(0.08)
        }
    }

    private func rowBackground(isHovered: Bool, isCurrentActive: Bool) -> Color {
        if isHovered {
            return accentColor.opacity(0.24)
        }
        if isCurrentActive {
            return accentColor.opacity(0.11)
        }
        return Color.clear
    }

    private func rowBorderColor(isHovered: Bool, isCurrentActive: Bool) -> Color {
        if isHovered {
            return accentColor.opacity(0.80)
        }
        if isCurrentActive {
            return accentColor.opacity(0.42)
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
            .onTapGesture {
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
                    if !pickerState.isPresented {
                        pickerState.open(provider: providerKind, activeModel: activeModelID)
                    }
                    if let point {
                        pickerState.updateDrag(at: point)
                    }
                case .inactive:
                    if case .active = oldPhase {
                        pickerState.endDrag(onSelect: onSelectModel)
                    }
                case .pressing:
                    break
                }
            }
            .accessibilityLabel("切换模型，当前 \(displayName)，长按上滑选择模型与思考强度")
    }
}
