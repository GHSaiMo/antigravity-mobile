import SwiftUI
import Photos

public final class FullScreenGalleryViewController: UIViewController, UIPageViewControllerDataSource, UIPageViewControllerDelegate, UIGestureRecognizerDelegate {
    public let items: [IdentifiableImage]
    public var currentIndex: Int
    public let onDismiss: () -> Void
    
    private var pageViewController: UIPageViewController!
    private let backgroundView = UIView()
    
    override public var prefersStatusBarHidden: Bool { true }
    
    public init(items: [IdentifiableImage], initialIndex: Int, onDismiss: @escaping () -> Void) {
        self.items = items
        self.currentIndex = max(0, min(initialIndex, max(0, items.count - 1)))
        self.onDismiss = onDismiss
        super.init(nibName: nil, bundle: nil)
        modalPresentationStyle = .overFullScreen
        overrideUserInterfaceStyle = .dark
    }
    
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    
    override public func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        view.backgroundColor = .clear
        view.superview?.backgroundColor = .clear
        view.superview?.superview?.backgroundColor = .clear
    }
    
    override public func viewDidLoad() {
        super.viewDidLoad()
        overrideUserInterfaceStyle = .dark
        view.backgroundColor = .clear
        
        // Dimming backdrop
        backgroundView.frame = view.bounds
        backgroundView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        backgroundView.backgroundColor = .black
        view.addSubview(backgroundView)
        
        // Native horizontal page carousel with inter-page spacing
        pageViewController = UIPageViewController(
            transitionStyle: .scroll,
            navigationOrientation: .horizontal,
            options: [.interPageSpacing: 20]
        )
        pageViewController.dataSource = self
        pageViewController.delegate = self
        
        addChild(pageViewController)
        pageViewController.view.frame = view.bounds
        pageViewController.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        pageViewController.view.backgroundColor = .clear
        view.addSubview(pageViewController.view)
        pageViewController.didMove(toParent: self)
        
        // Set initial page
        if let initialVC = makePageVC(for: currentIndex) {
            pageViewController.setViewControllers([initialVC], direction: .forward, animated: false)
        }
        
        // Vertical pull-to-dismiss gesture recognizer
        let pan = UIPanGestureRecognizer(target: self, action: #selector(handleDismissPan(_:)))
        pan.delegate = self
        view.addGestureRecognizer(pan)
    }
    
    override public func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        backgroundView.frame = view.bounds
        pageViewController?.view.frame = view.bounds
    }
    
    private func makePageVC(for index: Int) -> SingleImagePreviewController? {
        guard items.indices.contains(index) else { return nil }
        let vc = SingleImagePreviewController(item: items[index], onSingleTap: { [weak self] in
            self?.handleClose()
        })
        vc.view.tag = index
        return vc
    }
    
    // MARK: - UIPageViewControllerDataSource
    
    public func pageViewController(_ pageViewController: UIPageViewController, viewControllerBefore viewController: UIViewController) -> UIViewController? {
        let index = viewController.view.tag
        guard index > 0 else { return nil }
        return makePageVC(for: index - 1)
    }
    
    public func pageViewController(_ pageViewController: UIPageViewController, viewControllerAfter viewController: UIViewController) -> UIViewController? {
        let index = viewController.view.tag
        guard index < items.count - 1 else { return nil }
        return makePageVC(for: index + 1)
    }
    
    // MARK: - UIPageViewControllerDelegate
    
    public func pageViewController(_ pageViewController: UIPageViewController, didFinishAnimating finished: Bool, previousViewControllers: [UIViewController], transitionCompleted completed: Bool) {
        guard completed, let currentVC = pageViewController.viewControllers?.first else { return }
        currentIndex = currentVC.view.tag
    }
    
    // MARK: - UIGestureRecognizerDelegate
    
    public func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
        guard let pan = gestureRecognizer as? UIPanGestureRecognizer else { return true }
        // Do not intercept when image is zoomed in
        if let currentVC = pageViewController.viewControllers?.first as? SingleImagePreviewController {
            if currentVC.scrollView.zoomScale > 1.01 {
                return false
            }
        }
        let velocity = pan.velocity(in: view)
        // If movement is predominantly vertical (pull down or up), take over for dismiss
        // If movement is horizontal, return FALSE so UIPageViewController handles page flipping!
        return abs(velocity.y) > abs(velocity.x) * 1.3 && abs(velocity.y) > 20
    }
    
    // MARK: - Pull to dismiss
    
    @objc private func handleDismissPan(_ pan: UIPanGestureRecognizer) {
        let translation = pan.translation(in: view)
        let velocity = pan.velocity(in: view)
        let dy = translation.y
        let dx = translation.x
        let progress = min(1.0, abs(dy) / 220.0)
        
        switch pan.state {
        case .changed:
            // Background directly fades out as you drag, revealing the background behind immediately!
            backgroundView.alpha = max(0.0, 1.0 - progress * 1.25)
            
            // Image follows finger, scales slightly down (1.0 -> ~0.72) and gradually fades
            let scale = max(0.72, 1.0 - progress * 0.28)
            let currentAlpha = max(0.25, 1.0 - progress * 0.65)
            pageViewController.view.transform = CGAffineTransform(translationX: dx * 0.35, y: dy).scaledBy(x: scale, y: scale)
            pageViewController.view.alpha = currentAlpha
            
        case .ended, .cancelled:
            let shouldDismiss = progress > 0.25 || abs(velocity.y) > 420
            if shouldDismiss {
                UIView.animate(withDuration: 0.16, delay: 0, options: [.curveEaseOut], animations: {
                    let endScale: CGFloat = 0.62
                    let extraY = dy > 0 ? 25.0 : -25.0
                    self.pageViewController.view.transform = CGAffineTransform(translationX: dx * 0.35, y: dy + extraY).scaledBy(x: endScale, y: endScale)
                    self.pageViewController.view.alpha = 0
                    self.backgroundView.alpha = 0
                }) { _ in
                    self.onDismiss()
                }
            } else {
                UIView.animate(withDuration: 0.24, delay: 0, usingSpringWithDamping: 0.86, initialSpringVelocity: 0, animations: {
                    self.pageViewController.view.transform = .identity
                    self.pageViewController.view.alpha = 1.0
                    self.backgroundView.alpha = 1.0
                })
            }
            
        default:
            break
        }
    }
    
    @objc private func handleClose() {
        UIView.animate(withDuration: 0.16, delay: 0, options: [.curveEaseOut], animations: {
            self.pageViewController.view.transform = CGAffineTransform(scaleX: 0.85, y: 0.85)
            self.pageViewController.view.alpha = 0
            self.backgroundView.alpha = 0
        }) { _ in
            self.onDismiss()
        }
    }
}
