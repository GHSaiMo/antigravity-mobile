import SwiftUI
import UIKit
import ObjectiveC.runtime

// MARK: - Interactive Swipe-Back Support

private var systemEdgePopDelegateKey: UInt8 = 0
private var systemContentPopDelegateKey: UInt8 = 0

private final class WeakGestureDelegateBox: NSObject {
    weak var delegate: UIGestureRecognizerDelegate?

    init(_ delegate: UIGestureRecognizerDelegate) {
        self.delegate = delegate
    }
}

final class SwipeBackHookView: UIView {
    weak var coordinator: SwipeBackEnabler.Coordinator?
    
    override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil {
            configure()
        }
    }
    
    override func didMoveToSuperview() {
        super.didMoveToSuperview()
        if superview != nil {
            configure()
        }
    }
    
    func configure() {
        guard let nav = nearestNavigationController, let coordinator else { return }
        coordinator.navigationController = nav
        coordinator.hostView = self
        install(on: nav, coordinator: coordinator)
        
        if let topVC = nearestViewController {
            topVC.navigationItem.hidesBackButton = true
            topVC.navigationController?.navigationBar.topItem?.hidesBackButton = true
        }
        
        DispatchQueue.main.async { [weak self, weak nav] in
            guard let self, let nav, let coordinator = self.coordinator else { return }
            guard nav.topViewController === self.nearestViewController else { return }
            self.install(on: nav, coordinator: coordinator)
            if let topVC = self.nearestViewController {
                topVC.navigationItem.hidesBackButton = true
                topVC.navigationController?.navigationBar.topItem?.hidesBackButton = true
            }
        }
    }

    private func install(on nav: UINavigationController, coordinator: SwipeBackEnabler.Coordinator) {
        if let edgePop = nav.interactivePopGestureRecognizer {
            if let current = edgePop.delegate, current !== coordinator {
                if !(current is SwipeBackEnabler.Coordinator) {
                    objc_setAssociatedObject(
                        nav,
                        &systemEdgePopDelegateKey,
                        WeakGestureDelegateBox(current),
                        .OBJC_ASSOCIATION_RETAIN_NONATOMIC
                    )
                }
                coordinator.previousEdgeDelegate = current
            }
            edgePop.isEnabled = true
            edgePop.delegate = coordinator
        }

        if let contentPop = nav.interactiveContentPopGestureRecognizer {
            if let current = contentPop.delegate, current !== coordinator {
                if !(current is SwipeBackEnabler.Coordinator) {
                    objc_setAssociatedObject(
                        nav,
                        &systemContentPopDelegateKey,
                        WeakGestureDelegateBox(current),
                        .OBJC_ASSOCIATION_RETAIN_NONATOMIC
                    )
                }
                coordinator.previousContentDelegate = current
            }
            contentPop.isEnabled = true
            contentPop.delegate = coordinator
        }
    }
}

struct SwipeBackEnabler: UIViewRepresentable {
    var isDisabled: Bool = false

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }
    
    func makeUIView(context: Context) -> SwipeBackHookView {
        let view = SwipeBackHookView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        context.coordinator.isDisabled = isDisabled
        view.coordinator = context.coordinator
        DispatchQueue.main.async {
            view.configure()
        }
        return view
    }
    
    func updateUIView(_ uiView: SwipeBackHookView, context: Context) {
        context.coordinator.isDisabled = isDisabled
        uiView.coordinator = context.coordinator
        if let nav = context.coordinator.navigationController {
            guard nav.topViewController === uiView.nearestViewController else { return }
            let edgeNeedsFix = nav.interactivePopGestureRecognizer?.delegate !== context.coordinator
                || nav.interactivePopGestureRecognizer?.isEnabled != true
            let contentNeedsFix = nav.interactiveContentPopGestureRecognizer?.delegate !== context.coordinator
                || nav.interactiveContentPopGestureRecognizer?.isEnabled != true
            if edgeNeedsFix || contentNeedsFix {
                uiView.configure()
            }
        } else {
            uiView.configure()
        }
    }

    static func dismantleUIView(_ uiView: SwipeBackHookView, coordinator: Coordinator) {
        guard let nav = coordinator.navigationController else { return }
        if let edgePop = nav.interactivePopGestureRecognizer, edgePop.delegate === coordinator {
            edgePop.delegate = resolvedRestoreDelegate(
                previous: coordinator.previousEdgeDelegate,
                excluding: coordinator,
                systemKey: &systemEdgePopDelegateKey,
                on: nav
            )
        }
        if let contentPop = nav.interactiveContentPopGestureRecognizer, contentPop.delegate === coordinator {
            contentPop.delegate = resolvedRestoreDelegate(
                previous: coordinator.previousContentDelegate,
                excluding: coordinator,
                systemKey: &systemContentPopDelegateKey,
                on: nav
            )
        }
    }

    private static func resolvedRestoreDelegate(
        previous: UIGestureRecognizerDelegate?,
        excluding coordinator: Coordinator,
        systemKey: UnsafeRawPointer,
        on nav: UINavigationController
    ) -> UIGestureRecognizerDelegate? {
        let systemDelegate = (objc_getAssociatedObject(nav, systemKey) as? WeakGestureDelegateBox)?.delegate
        guard let previous, previous !== coordinator else {
            return systemDelegate
        }
        if let prevCoord = previous as? Coordinator, prevCoord.hostView?.window == nil {
            return systemDelegate
        }
        return previous
    }
    
    class Coordinator: NSObject, UIGestureRecognizerDelegate {
        /// Comfortable leading-edge hit zone (pt) for iOS 26/27 `interactiveContentPopGestureRecognizer`.
        /// On iOS 26/27, `interactivePopGestureRecognizer` only activates at the extreme bezel (~0-5pt),
        /// while touches at ~6-36pt fall through to `interactiveContentPopGestureRecognizer`.
        /// Scoping content-pop to the leading edge preserves mid-screen horizontal gestures (carousels,
        /// code/table scroll views, long-press slide pickers, and angled vertical chat scrolling).
        private static let leadingEdgeWidth: CGFloat = 36

        weak var navigationController: UINavigationController?
        weak var hostView: SwipeBackHookView?
        weak var previousEdgeDelegate: UIGestureRecognizerDelegate?
        weak var previousContentDelegate: UIGestureRecognizerDelegate?
        var isDisabled: Bool = false
        private var contentTouchDownTime: CFTimeInterval = 0

        private func isContentPop(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            gestureRecognizer === navigationController?.interactiveContentPopGestureRecognizer
        }

        private func isSiblingPop(_ other: UIGestureRecognizer, of gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard let nav = navigationController else { return false }
            if other === nav.interactivePopGestureRecognizer && other !== gestureRecognizer {
                return true
            }
            if other === nav.interactiveContentPopGestureRecognizer && other !== gestureRecognizer {
                return true
            }
            return false
        }

        private func hasBlockingInteractiveContainer(at point: CGPoint, in rootView: UIView) -> Bool {
            var current: UIView? = rootView.hitTest(point, with: nil)
            while let view = current, view !== rootView {
                if view is UITextView || view is UITextField || view is UISlider || view is UISwitch {
                    return true
                }
                let className = String(describing: type(of: view))
                if className.contains("WKWebView") || className.contains("WKContentView") {
                    return true
                }
                if let scrollView = view as? UIScrollView, scrollView.isScrollEnabled {
                    let canScrollHorizontally = scrollView.contentSize.width > scrollView.bounds.width + 1
                    let minOffsetX = -scrollView.adjustedContentInset.left
                    if canScrollHorizontally && scrollView.contentOffset.x > minOffsetX + 1 {
                        return true
                    }
                }
                current = view.superview
            }
            return false
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
            guard !isDisabled,
                  let nav = navigationController,
                  nav.viewControllers.count > 1 else {
                return false
            }
            if isContentPop(gestureRecognizer) {
                let loc = touch.location(in: nav.view)
                let isRTL = nav.view.effectiveUserInterfaceLayoutDirection == .rightToLeft
                let leadingDistance = isRTL ? (nav.view.bounds.width - loc.x) : loc.x
                guard leadingDistance >= 0, leadingDistance <= Self.leadingEdgeWidth else {
                    return false
                }
                if nav.navigationBar.frame.contains(loc) {
                    return false
                }
                if hasBlockingInteractiveContainer(at: loc, in: nav.view) {
                    return false
                }
                contentTouchDownTime = CACurrentMediaTime()
            }
            return true
        }
        
        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard !isDisabled,
                  let nav = navigationController,
                  nav.viewControllers.count > 1,
                  nav.transitionCoordinator == nil else {
                return false
            }
            if isContentPop(gestureRecognizer),
               contentTouchDownTime > 0,
               CACurrentMediaTime() - contentTouchDownTime > 0.25 {
                // Ignore long-press-then-drag gestures (e.g. slide pickers or context menus near the left edge).
                return false
            }
            if let pan = gestureRecognizer as? UIPanGestureRecognizer {
                let isRTL = nav.view.effectiveUserInterfaceLayoutDirection == .rightToLeft
                let velocity = pan.velocity(in: nav.view)
                // Avoid triggering interactive pop during vertical chat scrolling near the left edge.
                if velocity != .zero {
                    let forwardVelocity = isRTL ? -velocity.x : velocity.x
                    if forwardVelocity <= 0 || abs(velocity.y) > forwardVelocity * 1.2 {
                        return false
                    }
                } else {
                    let translation = pan.translation(in: nav.view)
                    if translation != .zero {
                        let forwardTranslation = isRTL ? -translation.x : translation.x
                        if forwardTranslation <= 0 || abs(translation.y) > forwardTranslation * 1.2 {
                            return false
                        }
                    }
                }
            }
            return true
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            if isSiblingPop(otherGestureRecognizer, of: gestureRecognizer) {
                return false
            }
            // Prevent UIScrollView from scrolling concurrently while an interactive pop is active.
            if otherGestureRecognizer is UIPanGestureRecognizer && otherGestureRecognizer.view is UIScrollView {
                return false
            }
            // Allow SwiftUI's internal responder/relationship recognizers (notably UIKitResponderGestureRecognizer,
            // which enters .began on touch-down before edge/content pop crosses its pan threshold).
            // Returning false unconditionally blocks interactive pop from starting on iOS 26/27.
            return true
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldBeRequiredToFailBy otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            guard otherGestureRecognizer is UIPanGestureRecognizer,
                  let scrollView = otherGestureRecognizer.view as? UIScrollView else {
                return false
            }
            if !isContentPop(gestureRecognizer) {
                return true
            }
            let canScrollHorizontally = scrollView.contentSize.width > scrollView.bounds.width + 1
            return !canScrollHorizontally
        }
    }
}

private extension UIView {
    var nearestViewController: UIViewController? {
        var responder: UIResponder? = self
        while let next = responder?.next {
            if let vc = next as? UIViewController {
                return vc
            }
            responder = next
        }
        return nil
    }
    
    var nearestNavigationController: UINavigationController? {
        var responder: UIResponder? = self
        while let next = responder?.next {
            if let nav = next as? UINavigationController {
                return nav
            }
            if let vc = next as? UIViewController, let nav = vc.navigationController {
                return nav
            }
            responder = next
        }
        return nil
    }
}
