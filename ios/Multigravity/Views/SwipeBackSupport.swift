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
        uiView.configure()
    }
    
    class Coordinator: NSObject, UIGestureRecognizerDelegate {
        weak var navigationController: UINavigationController?
        
        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard let nav = navigationController else { return false }
            return nav.viewControllers.count > 1
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            return true
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
