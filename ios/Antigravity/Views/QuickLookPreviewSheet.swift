import SwiftUI
import QuickLook
import WebKit

// MARK: - Native QuickLook Presentation Sheet (PPTX, DOCX, XLSX, PDF, KEY)

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
        controller.navigationItem.leftBarButtonItem = doneItem
        
        let shareItem = UIBarButtonItem(
            image: UIImage(systemName: "square.and.arrow.up"),
            style: .plain,
            target: context.coordinator,
            action: #selector(Coordinator.shareTapped)
        )
        shareItem.accessibilityLabel = "发送"
        controller.navigationItem.rightBarButtonItem = shareItem
        
        let nav = UINavigationController(rootViewController: controller)
        nav.navigationBar.prefersLargeTitles = false
        context.coordinator.navController = nav
        return nav
    }
    
    public func updateUIViewController(_ uiViewController: UINavigationController, context: Context) {
        context.coordinator.parent = self
        context.coordinator.navController = uiViewController
        if let ql = uiViewController.topViewController as? QLPreviewController {
            ql.reloadData()
        }
    }
    
    public final class Coordinator: NSObject, QLPreviewControllerDataSource, QLPreviewControllerDelegate {
        var parent: QuickLookPreviewSheet
        weak var navController: UINavigationController?
        
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
        
        @objc func shareTapped() {
            let activityVC = UIActivityViewController(activityItems: [parent.url], applicationActivities: nil)
            if let popover = activityVC.popoverPresentationController, let rightBtn = navController?.topViewController?.navigationItem.rightBarButtonItem {
                popover.barButtonItem = rightBtn
            }
            navController?.present(activityVC, animated: true)
        }
    }
}

// MARK: - Interactive HTML Presentation Sheet (WKWebView)

public struct HTMLPreviewSheet: View {
    public let url: URL
    public let title: String
    public let onDismiss: () -> Void
    @State private var isSharing: Bool = false
    
    public init(url: URL, title: String, onDismiss: @escaping () -> Void) {
        self.url = url
        self.title = title
        self.onDismiss = onDismiss
    }
    
    public var body: some View {
        NavigationStack {
            HTMLWebViewRepresentable(url: url)
                .ignoresSafeArea(edges: .bottom)
                .navigationTitle(title.isEmpty ? "网页文档" : title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("完成") {
                            onDismiss()
                        }
                        .fontWeight(.semibold)
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            isSharing = true
                        } label: {
                            Image(systemName: "square.and.arrow.up")
                        }
                        .accessibilityLabel("发送")
                    }
                }
                .sheet(isPresented: $isSharing) {
                    ShareSheetView(activityItems: [url])
                }
        }
    }
}

public struct HTMLWebViewRepresentable: UIViewRepresentable {
    public let url: URL
    
    public func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.preferences.setValue(true, forKey: "allowFileAccessFromFileURLs")
        let webView = WKWebView(frame: .zero, configuration: config)
        webView.allowsBackForwardNavigationGestures = true
        webView.scrollView.contentInsetAdjustmentBehavior = .automatic
        webView.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        return webView
    }
    
    public func updateUIView(_ uiView: WKWebView, context: Context) {}
}

// MARK: - Activity View Controller for Sharing files

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
