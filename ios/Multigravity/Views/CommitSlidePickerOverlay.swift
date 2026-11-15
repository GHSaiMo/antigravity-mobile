import SwiftUI
import UIKit

@MainActor
@Observable
public final class CommitSlidePickerState {
    public var isPresented: Bool = false
    public var isInteractiveTapMode: Bool = false
    public var anchorRect: CGRect = .zero
    public var containerSize: CGSize = .zero
    public var hoveredIndex: Int? = nil
    public var hasEnteredMenu: Bool = false

    public let options = ["Commit and Push", "Commit Only"]
    private let selectionHaptic = UISelectionFeedbackGenerator()

    public enum Layout {
        static let rowHeight: CGFloat = 34
        static let rowSpacing: CGFloat = 3
        static let cardPadding: CGFloat = 5
        static let cardWidth: CGFloat = 172
        static let gapBetweenButtonAndMenu: CGFloat = 8
        static let horizontalMargin: CGFloat = 12
        static let topSafeMargin: CGFloat = 12
    }

    public init() {}

    public func open() {
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

    public var cardSize: CGSize {
        let count = options.count
        let height = CGFloat(count) * Layout.rowHeight
            + CGFloat(count - 1) * Layout.rowSpacing
            + Layout.cardPadding * 2
        return CGSize(width: Layout.cardWidth, height: height)
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
            let step = Layout.rowHeight + Layout.rowSpacing
            let idx = min(max(0, Int(floor(localY / step))), options.count - 1)
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

    public func endDrag(onSelect: (String) -> Void) {
        guard isPresented else { return }
        if let idx = hoveredIndex, options.indices.contains(idx) {
            let chosen = options[idx]
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onSelect(chosen)
            dismiss()
            return
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

public struct CommitSlidePickerOverlay: View {
    @Bindable var state: CommitSlidePickerState
    let onSelect: (String) -> Void

    public init(state: CommitSlidePickerState, onSelect: @escaping (String) -> Void) {
        self.state = state
        self.onSelect = onSelect
    }

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
                VStack(alignment: .leading, spacing: CommitSlidePickerState.Layout.rowSpacing) {
                    ForEach(Array(state.options.enumerated()), id: \.offset) { index, title in
                        let isHovered = state.hoveredIndex == index
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            onSelect(title)
                            state.dismiss()
                        } label: {
                            HStack {
                                Text(title)
                                    .font(.system(size: 13.5, weight: isHovered ? .semibold : .medium))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                                Spacer()
                            }
                            .padding(.horizontal, 12)
                            .frame(maxWidth: .infinity)
                            .frame(height: CommitSlidePickerState.Layout.rowHeight)
                            .background(isHovered ? Color.primary.opacity(0.12) : Color.clear)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .stroke(isHovered ? Color.primary.opacity(0.25) : Color.clear, lineWidth: 1)
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(CommitSlidePickerState.Layout.cardPadding)
                .frame(width: rect.width, height: rect.height)
                .background(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .fill(Color(uiColor: .secondarySystemGroupedBackground).opacity(0.96))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                )
                .shadow(color: Color.black.opacity(0.14), radius: 10, x: 0, y: 4)
                .offset(x: rect.minX, y: rect.minY)
                .transition(.asymmetric(
                    insertion: .scale(scale: 0.94, anchor: .bottomLeading).combined(with: .opacity),
                    removal: .opacity
                ))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .zIndex(500)
        }
    }
}

private enum CommitChipGesturePhase: Equatable {
    case inactive
    case pressing
    case active(CGPoint?)
}

public struct CommitChipButton: View {
    let label: String
    let canCommit: Bool
    @Bindable var pickerState: CommitSlidePickerState
    let onTap: () -> Void
    let onSelectAction: (String) -> Void
    var onWillOpen: (() -> Void)? = nil

    @GestureState private var gesturePhase: CommitChipGesturePhase = .inactive

    public init(
        label: String,
        canCommit: Bool,
        pickerState: CommitSlidePickerState,
        onTap: @escaping () -> Void,
        onSelectAction: @escaping (String) -> Void,
        onWillOpen: (() -> Void)? = nil
    ) {
        self.label = label
        self.canCommit = canCommit
        self.pickerState = pickerState
        self.onTap = onTap
        self.onSelectAction = onSelectAction
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

        let baseButton = Text(label)
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
                    onTap()
                }
            }

        if canCommit {
            baseButton
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
                            pickerState.endDrag(onSelect: onSelectAction)
                        }
                    case .pressing:
                        break
                    }
                }
        } else {
            baseButton
        }
    }
}
