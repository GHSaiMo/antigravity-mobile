import SwiftUI
import Observation
import UIKit

@Observable
@MainActor
final class ChatScrollHUDState {
    var stepperOpacity: Double = 0.0
    var currentVisibleTurnIndex: Int = 1
    var isNearBottom: Bool = true
    @ObservationIgnored var messagesContentHeight: CGFloat = 0
    @ObservationIgnored var hasUserInteracted: Bool = false
    @ObservationIgnored var isUserDragging: Bool = false
    @ObservationIgnored var isScrollMoving: Bool = false
    @ObservationIgnored var lastRowMaterialized: Bool = false
    @ObservationIgnored var isPacingActive: Bool = false
    @ObservationIgnored weak var underlyingScrollView: UIScrollView? = nil
    @ObservationIgnored var lastFollowScrollAt: Date = .distantPast
    @ObservationIgnored var lastKnownVisibleMessageId: String? = nil
    @ObservationIgnored var lastMidBlankHealAt: Date = .distantPast
    @ObservationIgnored var lastAnchorMaxY: CGFloat = 0
    @ObservationIgnored var lastQuantizedOffsetY: CGFloat = -1
    @ObservationIgnored var lastQuantizedDistanceFromBottom: CGFloat = 0
    @ObservationIgnored var wasScrollingDownward: Bool = false
    @ObservationIgnored var lastStepperScheduleAt: Date = .distantPast
    @ObservationIgnored var stepperDimTask: Task<Void, Never>? = nil
    @ObservationIgnored var stepperHideTask: Task<Void, Never>? = nil
    @ObservationIgnored var blankHealTask: Task<Void, Never>? = nil
    @ObservationIgnored var midScrollBlankHealTask: Task<Void, Never>? = nil
    @ObservationIgnored var turnJumpTask: Task<Void, Never>? = nil
    @ObservationIgnored var lastVisibleMessageGeometries: [VisibleMessageGeometry] = []
    @ObservationIgnored var lockedTurnIndex: Int? = nil

    func cancelScrollHealAndJumpTasks() {
        blankHealTask?.cancel()
        blankHealTask = nil
        midScrollBlankHealTask?.cancel()
        midScrollBlankHealTask = nil
        turnJumpTask?.cancel()
        turnJumpTask = nil
    }
}

/// 挂在消息列表内部的轻量 UIKit 桥接：直接监听外层 `UIScrollView.contentSize` KVO，
/// 在流式吐字导致内容高度变化时，于同一个 CoreAnimation 布局事务内原子调整 `contentOffset.y`，
/// 消除先向下顶出、下一帧再 `proxy.scrollTo` 弹回引起的下半屏闪动与抖动。
struct ChatScrollBottomPinner: UIViewRepresentable {
    let scrollHUD: ChatScrollHUDState
    let canPin: Bool
    let isStreamRunning: Bool

    func makeCoordinator() -> Coordinator {
        Coordinator(scrollHUD: scrollHUD, canPin: canPin, isStreamRunning: isStreamRunning)
    }

    func makeUIView(context: Context) -> ScrollPinAnchorView {
        let view = ScrollPinAnchorView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        view.coordinator = context.coordinator
        return view
    }

    func updateUIView(_ uiView: ScrollPinAnchorView, context: Context) {
        context.coordinator.scrollHUD = scrollHUD
        context.coordinator.canPin = canPin
        context.coordinator.isStreamRunning = isStreamRunning
        if context.coordinator.scrollView == nil {
            uiView.attachIfNeeded()
        }
    }

    static func dismantleUIView(_ uiView: ScrollPinAnchorView, coordinator: Coordinator) {
        coordinator.detach()
    }

    final class ScrollPinAnchorView: UIView {
        weak var coordinator: Coordinator?

        override func didMoveToSuperview() {
            super.didMoveToSuperview()
            attachIfNeeded()
        }

        override func didMoveToWindow() {
            super.didMoveToWindow()
            attachIfNeeded()
        }

        func attachIfNeeded() {
            guard window != nil else { return }
            var cursor = superview
            while let current = cursor {
                if let sv = current as? UIScrollView {
                    coordinator?.attach(to: sv)
                    return
                }
                cursor = current.superview
            }
        }
    }

    @MainActor
    final class Coordinator: NSObject {
        var scrollHUD: ChatScrollHUDState
        var canPin: Bool
        var isStreamRunning: Bool
        weak var scrollView: UIScrollView?
        private var contentSizeObservation: NSKeyValueObservation?
        private var isApplyingOffset = false

        init(scrollHUD: ChatScrollHUDState, canPin: Bool, isStreamRunning: Bool) {
            self.scrollHUD = scrollHUD
            self.canPin = canPin
            self.isStreamRunning = isStreamRunning
        }

        func attach(to sv: UIScrollView) {
            guard scrollView !== sv else { return }
            detach()
            scrollView = sv
            scrollHUD.underlyingScrollView = sv
            contentSizeObservation = sv.observe(\.contentSize, options: [.old, .new]) { [weak self] scrollView, change in
                guard Thread.isMainThread, let oldSize = change.oldValue, let newSize = change.newValue else { return }
                MainActor.assumeIsolated {
                    self?.handleContentSizeChange(scrollView: scrollView, oldSize: oldSize, newSize: newSize)
                }
            }
        }

        func detach() {
            contentSizeObservation?.invalidate()
            contentSizeObservation = nil
            if let sv = scrollView, scrollHUD.underlyingScrollView === sv {
                scrollHUD.underlyingScrollView = nil
            }
            scrollView = nil
        }

        private func handleContentSizeChange(scrollView: UIScrollView, oldSize: CGSize, newSize: CGSize) {
            guard !isApplyingOffset else { return }
            guard canPin, isStreamRunning || scrollHUD.isPacingActive else { return }

            if scrollView.isDragging {
                scrollHUD.hasUserInteracted = true
                return
            }
            guard !scrollHUD.hasUserInteracted,
                  !scrollHUD.isUserDragging,
                  !scrollView.isTracking,
                  !scrollView.isDecelerating,
                  scrollView.refreshControl?.isRefreshing != true else {
                return
            }

            let oldH = oldSize.height
            let newH = newSize.height
            guard oldH > 0, newH > 0, abs(newH - oldH) >= 0.5 else { return }

            let boundsH = scrollView.bounds.height
            guard boundsH > 0 else { return }

            let topInset = scrollView.adjustedContentInset.top
            let bottomInset = scrollView.adjustedContentInset.bottom
            let minY = -topInset
            let oldMaxY = max(minY, oldH - boundsH + bottomInset)
            let newMaxY = max(minY, newH - boundsH + bottomInset)

            let currentY = scrollView.contentOffset.y
            let distFromOldBottom = oldMaxY - currentY
            // 仅在原本就贴近底部时同步跟随；若用户停在更上方历史位置则绝不强拉
            guard distFromOldBottom <= 140 else { return }

            let targetY = newMaxY
            guard abs(currentY - targetY) >= 0.5 else { return }

            isApplyingOffset = true
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            scrollView.contentOffset = CGPoint(x: scrollView.contentOffset.x, y: targetY)
            CATransaction.commit()
            isApplyingOffset = false
        }
    }
}

struct ChatScrollMetrics: Equatable {
    let quantizedDistanceFromBottom: CGFloat
    let quantizedOffsetY: CGFloat
    let viewportHeight: CGFloat
    let contentHeight: CGFloat
    let overshoot: CGFloat
}

struct VisibleMessageGeometry: Equatable {
    let id: String
    let minY: CGFloat
    let maxY: CGFloat
}

struct ChatVisibleMessagesPreferenceKey: PreferenceKey {
    static var defaultValue: [VisibleMessageGeometry] = []
    static func reduce(value: inout [VisibleMessageGeometry], nextValue: () -> [VisibleMessageGeometry]) {
        value.append(contentsOf: nextValue())
    }
}

