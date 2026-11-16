import SwiftUI
import UIKit

@MainActor
@Observable
public final class WorkflowSlidePickerState {
    public var isPresented: Bool = false
    public var isInteractiveTapMode: Bool = false
    public var anchorRect: CGRect = .zero
    public var containerSize: CGSize = .zero
    public var quickTemplates: [WorkflowTemplateRecord] = []
    public var hoveredIndex: Int? = nil
    public var hasEnteredMenu: Bool = false

    private let selectionHaptic = UISelectionFeedbackGenerator()

    public enum Layout {
        static let defaultRowHeight: CGFloat = 34
        static let compactRowHeight: CGFloat = 30
        static let minRowHeight: CGFloat = 24
        static let defaultRowSpacing: CGFloat = 3
        static let compactRowSpacing: CGFloat = 2
        static let cardPadding: CGFloat = 5
        static let cardMinWidth: CGFloat = 208
        static let cardMaxWidth: CGFloat = 276
        static let gapBetweenButtonAndMenu: CGFloat = 8
        static let horizontalMargin: CGFloat = 12
        static let topSafeMargin: CGFloat = 12
    }

    public init() {}

    public var isEmptyState: Bool {
        quickTemplates.isEmpty
    }

    public var rowCount: Int {
        max(quickTemplates.count, 1)
    }

    public func open() {
        let items = WorkflowTemplateStore.shared.templates.filter(\.inQuickMenu)
        self.quickTemplates = items
        self.hoveredIndex = nil
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
        hoveredIndex = nil
        hasEnteredMenu = false
    }

    private var availableHeight: CGFloat {
        if anchorRect.minY > Layout.topSafeMargin + Layout.gapBetweenButtonAndMenu + 60 {
            return anchorRect.minY - Layout.gapBetweenButtonAndMenu - Layout.topSafeMargin
        }
        if containerSize.height > 120 {
            return containerSize.height - Layout.topSafeMargin * 2
        }
        return 420
    }

    public var rowSpacing: CGFloat {
        rowCount > 7 ? Layout.compactRowSpacing : Layout.defaultRowSpacing
    }

    public var rowHeight: CGFloat {
        if isEmptyState {
            return 36
        }
        let count = rowCount
        let preferred = count > 7 ? Layout.compactRowHeight : Layout.defaultRowHeight
        let fixedOverhead = Layout.cardPadding * 2 + CGFloat(max(0, count - 1)) * rowSpacing
        let maxRow = (availableHeight - fixedOverhead) / CGFloat(count)
        return min(preferred, max(Layout.minRowHeight, floor(maxRow)))
    }

    public var contentNeedsScroll: Bool {
        let count = rowCount
        let naturalHeight = CGFloat(count) * rowHeight
            + CGFloat(max(0, count - 1)) * rowSpacing
            + Layout.cardPadding * 2
        return naturalHeight > availableHeight + 1
    }

    public var cardSize: CGSize {
        let count = rowCount
        let naturalHeight = CGFloat(count) * rowHeight
            + CGFloat(max(0, count - 1)) * rowSpacing
            + Layout.cardPadding * 2
        let height = min(naturalHeight, availableHeight)
        let maxRoom = containerSize.width > 0
            ? containerSize.width - Layout.horizontalMargin * 2
            : Layout.cardMaxWidth
        let minW = isEmptyState ? 236 : Layout.cardMinWidth
        let upperWidth = min(Layout.cardMaxWidth, max(minW, maxRoom))
        let preferredWidth = containerSize.width > 0 ? containerSize.width * (isEmptyState ? 0.68 : 0.56) : minW
        let width = min(upperWidth, max(minW, preferredWidth))
        return CGSize(width: width, height: height)
    }

    public var cardRect: CGRect {
        let size = cardSize
        let maxLeftX = max(
            Layout.horizontalMargin,
            containerSize.width - size.width - Layout.horizontalMargin
        )
        let minX = containerSize.width > 0
            ? min(max(Layout.horizontalMargin, anchorRect.minX), maxLeftX)
            : max(Layout.horizontalMargin, anchorRect.minX)
        let minY = max(Layout.topSafeMargin, anchorRect.minY - Layout.gapBetweenButtonAndMenu - size.height)
        return CGRect(x: minX, y: minY, width: size.width, height: size.height)
    }

    public func updateDrag(at point: CGPoint) {
        guard isPresented else { return }
        let rect = cardRect
        let hitRect = CGRect(
            x: rect.minX - 28,
            y: rect.minY - 24,
            width: rect.width + 56,
            height: rect.height + 30
        )
        if hitRect.contains(point) {
            hasEnteredMenu = true
            let localY = point.y - (rect.minY + Layout.cardPadding)
            let step = rowHeight + rowSpacing
            let idx = min(max(0, Int(floor(localY / step))), rowCount - 1)
            if hoveredIndex != idx {
                hoveredIndex = idx
                selectionHaptic.selectionChanged()
            }
            return
        }

        if anchorRect.insetBy(dx: -16, dy: -12).contains(point) {
            clearHover()
            return
        }

        clearHover()
    }

    public func endDrag(onSelectTemplate: (String) -> Void, onOpenSetup: () -> Void) {
        guard isPresented else { return }
        if let idx = hoveredIndex {
            if isEmptyState {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                dismiss()
                onOpenSetup()
                return
            }
            if quickTemplates.indices.contains(idx) {
                let chosen = quickTemplates[idx]
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                onSelectTemplate(chosen.templatePrompt)
                dismiss()
                return
            }
        }

        if !hasEnteredMenu {
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }

    private func clearHover() {
        if hoveredIndex != nil {
            hoveredIndex = nil
        }
    }
}

public struct WorkflowSlidePickerOverlay: View {
    @Bindable var state: WorkflowSlidePickerState
    let onSelectTemplate: (String) -> Void
    let onOpenSetup: () -> Void

    public var body: some View {
        if state.isPresented {
            ZStack(alignment: .topLeading) {
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

                let rect = state.cardRect
                menuCard(size: rect.size)
                    .position(x: rect.midX, y: rect.midY)
                    .transition(
                        .asymmetric(
                            insertion: .opacity.combined(with: .scale(scale: 0.94, anchor: .bottomLeading)),
                            removal: .opacity
                        )
                    )
            }
            .zIndex(500)
        }
    }

    @ViewBuilder
    private func menuCard(size: CGSize) -> some View {
        let content = VStack(alignment: .leading, spacing: state.rowSpacing) {
            if state.isEmptyState {
                let isHovered = state.hoveredIndex == 0
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    state.dismiss()
                    onOpenSetup()
                } label: {
                    HStack(spacing: 6) {
                        Text(isHovered ? String(localized: "暂未配置快捷模板 · 松手去设置") : String(localized: "暂未配置快捷模板 · 点击去设置"))
                            .font(.system(size: 13, weight: isHovered ? .semibold : .medium))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                        Spacer(minLength: 4)
                        Text("›")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.primary.opacity(0.65))
                    }
                    .padding(.horizontal, 10)
                    .frame(maxWidth: .infinity)
                    .frame(height: state.rowHeight)
                    .background(isHovered ? Color.primary.opacity(0.12) : Color.clear)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(isHovered ? Color.primary.opacity(0.32) : Color.clear, lineWidth: isHovered ? 1.25 : 1)
                    )
                }
                .buttonStyle(.plain)
            } else {
                ForEach(Array(state.quickTemplates.enumerated()), id: \.element.id) { index, item in
                    let isHovered = state.hoveredIndex == index
                    let iconColor = WorkflowTemplateStyle.color(for: item.color)
                    let badgeSize: CGFloat = state.rowHeight < 29 ? 18 : 22
                    let iconSize: CGFloat = state.rowHeight < 29 ? 11 : 13
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSelectTemplate(item.templatePrompt)
                        state.dismiss()
                    } label: {
                        HStack(spacing: 8) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .fill(iconColor.opacity(0.15))
                                    .frame(width: badgeSize, height: badgeSize)
                                WorkflowVectorIconView(iconKey: item.icon, size: iconSize, color: iconColor)
                            }
                            Text(item.title)
                                .font(.system(size: state.rowHeight < 29 ? 12.5 : 13.5, weight: isHovered ? .semibold : .medium))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                                .minimumScaleFactor(0.8)
                            Spacer(minLength: 4)
                        }
                        .padding(.horizontal, 10)
                        .frame(maxWidth: .infinity)
                        .frame(height: state.rowHeight)
                        .background(isHovered ? Color.primary.opacity(0.12) : Color.clear)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 8, style: .continuous)
                                .stroke(isHovered ? Color.primary.opacity(0.32) : Color.clear, lineWidth: isHovered ? 1.25 : 1)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(WorkflowSlidePickerState.Layout.cardPadding)

        Group {
            if state.contentNeedsScroll {
                ScrollView(.vertical, showsIndicators: false) {
                    content
                }
            } else {
                content
            }
        }
        .frame(width: size.width, height: size.height)
        .background(
            ZStack {
                Rectangle().fill(.ultraThinMaterial)
                Color(uiColor: .systemBackground).opacity(0.80)
            }
        )
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }
}

private enum WorkflowChipGesturePhase: Equatable {
    case inactive
    case pressing
    case active(CGPoint?)
}

public struct WorkflowChipButton: View {
    @Bindable var pickerState: WorkflowSlidePickerState
    let onOpenSheet: () -> Void
    let onSelectTemplate: (String) -> Void
    let onOpenSetup: () -> Void
    var onWillOpen: (() -> Void)? = nil

    @GestureState private var gesturePhase: WorkflowChipGesturePhase = .inactive

    public init(
        pickerState: WorkflowSlidePickerState,
        onOpenSheet: @escaping () -> Void,
        onSelectTemplate: @escaping (String) -> Void,
        onOpenSetup: @escaping () -> Void,
        onWillOpen: (() -> Void)? = nil
    ) {
        self.pickerState = pickerState
        self.onOpenSheet = onOpenSheet
        self.onSelectTemplate = onSelectTemplate
        self.onOpenSetup = onOpenSetup
        self.onWillOpen = onWillOpen
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

        Text("Workflow")
            .font(.system(size: 13, weight: .medium))
            .foregroundColor(.primary)
            .padding(.horizontal, 14)
            .frame(height: 32)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
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
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onOpenSheet()
                }
            }
            .gesture(longPressThenDrag)
            .onChange(of: gesturePhase) { oldPhase, newPhase in
                switch newPhase {
                case .active(let point):
                    if !pickerState.isPresented {
                        onWillOpen?()
                        pickerState.open()
                    }
                    if let point {
                        pickerState.updateDrag(at: point)
                    }
                case .inactive:
                    if case .active = oldPhase {
                        pickerState.endDrag(
                            onSelectTemplate: onSelectTemplate,
                            onOpenSetup: onOpenSetup
                        )
                    }
                case .pressing:
                    break
                }
            }
            .accessibilityLabel("Workflow 模板，单击打开管理页，长按上滑快捷输入")
    }
}
