import SwiftUI
import UIKit

// MARK: - Interactive Swipe-Back Support

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
        guard let nav = nearestNavigationController else { return }
        coordinator?.navigationController = nav
        nav.interactivePopGestureRecognizer?.isEnabled = true
        nav.interactivePopGestureRecognizer?.delegate = coordinator
        
        if let topVC = nearestViewController {
            topVC.navigationItem.hidesBackButton = true
            topVC.navigationController?.navigationBar.topItem?.hidesBackButton = true
        }
        
        DispatchQueue.main.async { [weak self, weak nav] in
            guard let self, let nav else { return }
            nav.interactivePopGestureRecognizer?.isEnabled = true
            if let coordinator = self.coordinator {
                nav.interactivePopGestureRecognizer?.delegate = coordinator
            }
            if let topVC = self.nearestViewController {
                topVC.navigationItem.hidesBackButton = true
                topVC.navigationController?.navigationBar.topItem?.hidesBackButton = true
            }
        }
    }
}

struct SwipeBackEnabler: UIViewRepresentable {
    func makeCoordinator() -> Coordinator {
        Coordinator()
    }
    
    func makeUIView(context: Context) -> SwipeBackHookView {
        let view = SwipeBackHookView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        view.coordinator = context.coordinator
        DispatchQueue.main.async {
            view.configure()
        }
        return view
    }
    
    func updateUIView(_ uiView: SwipeBackHookView, context: Context) {
        uiView.coordinator = context.coordinator
        if let nav = context.coordinator.navigationController {
            if nav.topViewController === uiView.nearestViewController,
               nav.interactivePopGestureRecognizer?.delegate !== context.coordinator {
                uiView.configure()
            }
        } else {
            uiView.configure()
        }
    }

    static func dismantleUIView(_ uiView: SwipeBackHookView, coordinator: Coordinator) {
        if let nav = coordinator.navigationController,
           nav.interactivePopGestureRecognizer?.delegate === coordinator {
            nav.interactivePopGestureRecognizer?.delegate = nil
        }
    }
    
    class Coordinator: NSObject, UIGestureRecognizerDelegate {
        weak var navigationController: UINavigationController?
        
        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard let nav = navigationController,
                  nav.viewControllers.count > 1,
                  nav.transitionCoordinator == nil else {
                return false
            }
            if let pan = gestureRecognizer as? UIPanGestureRecognizer {
                let velocity = pan.velocity(in: nav.view)
                // Avoid triggering interactive pop during vertical chat scrolling near the left edge.
                if velocity != .zero && (velocity.x <= 0 || abs(velocity.y) > velocity.x * 1.2) {
                    return false
                }
            }
            return true
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            return false
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldBeRequiredToFailBy otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            return otherGestureRecognizer is UIPanGestureRecognizer && otherGestureRecognizer.view is UIScrollView
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
