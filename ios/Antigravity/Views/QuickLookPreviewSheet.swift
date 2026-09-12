import SwiftUI
import QuickLook

public struct QuickLookPreviewSheet: UIViewControllerRepresentable {
    public let url: URL
    public let onDismiss: (() -> Void)?
    
    public init(url: URL, onDismiss: (() -> Void)? = nil) {
        self.url = url
        self.onDismiss = onDismiss
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(parent: self)
    }
    
    public func makeUIViewController(context: Context) -> UINavigationController {
        let controller = QLPreviewController()
        controller.dataSource = context.coordinator
        controller.delegate = context.coordinator
        
        let doneItem = UIBarButtonItem(
            title: "完成",
            style: .done,
            target: context.coordinator,
            action: #selector(Coordinator.doneTapped)
        )
        controller.navigationItem.rightBarButtonItem = doneItem
        
        let nav = UINavigationController(rootViewController: controller)
        nav.navigationBar.prefersLargeTitles = false
        return nav
    }
    
    public func updateUIViewController(_ uiViewController: UINavigationController, context: Context) {
        context.coordinator.parent = self
        if let ql = uiViewController.topViewController as? QLPreviewController {
            ql.reloadData()
        }
    }
    
    public final class Coordinator: NSObject, QLPreviewControllerDataSource, QLPreviewControllerDelegate {
        var parent: QuickLookPreviewSheet
        
        init(parent: QuickLookPreviewSheet) {
            self.parent = parent
        }
        
        public func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
            return 1
        }
        
        public func previewController(_ controller: QLPreviewController, previewItemAt index: Int) -> QLPreviewItem {
            return parent.url as NSURL
        }
        
        public func previewControllerDidDismiss(_ controller: QLPreviewController) {
            parent.onDismiss?()
        }
        
        @objc func doneTapped() {
            parent.onDismiss?()
        }
    }
}

// Activity View Controller for Sharing files
public struct ShareSheetView: UIViewControllerRepresentable {
    public let activityItems: [Any]
    public let onDismiss: (() -> Void)?
    
    public init(activityItems: [Any], onDismiss: (() -> Void)? = nil) {
        self.activityItems = activityItems
        self.onDismiss = onDismiss
    }
    
    public func makeUIViewController(context: Context) -> UIActivityViewController {
        let controller = UIActivityViewController(activityItems: activityItems, applicationActivities: nil)
        controller.completionWithItemsHandler = { _, _, _, _ in
            self.onDismiss?()
        }
        return controller
    }
    
    public func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}
