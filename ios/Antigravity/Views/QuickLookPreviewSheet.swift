import SwiftUI
import QuickLook
import WebKit

// MARK: - Native QuickLook Presentation Sheet (PPTX, DOCX, XLSX, PDF, KEY)

public struct QuickLookPreviewSheet: View {
    public let url: URL
    public let title: String
    public let onDismiss: () -> Void
    @State private var isSharing: Bool = false
    
    public init(url: URL, title: String = "", onDismiss: @escaping () -> Void) {
        self.url = url
        let fallback = url.lastPathComponent.removingPercentEncoding ?? url.lastPathComponent
        self.title = title.isEmpty ? fallback : title
        self.onDismiss = onDismiss
    }
    
    public var body: some View {
        VStack(spacing: 0) {
            // Floating grab handle hinting pull-down dismissal
            Capsule()
                .fill(Color(uiColor: .tertiaryLabel))
                .frame(width: 38, height: 5)
                .padding(.top, 10)
                .padding(.bottom, 12)
            
            // Header bar
            HStack {
                Button("完成") {
                    onDismiss()
                }
                .font(.system(size: 16, weight: .semibold))
                .frame(width: 60, alignment: .leading)
                
                Spacer()
                
                Text(title)
                    .font(.system(size: 17, weight: .bold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                
                Spacer()
                
                Button {
                    isSharing = true
                } label: {
                    Image(systemName: "square.and.arrow.up")
                        .font(.system(size: 16, weight: .semibold))
                }
                .frame(width: 60, alignment: .trailing)
                .accessibilityLabel("发送")
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            
            Divider()
            
            QuickLookControllerRepresentable(url: url)
                .ignoresSafeArea(edges: .bottom)
        }
        .sheet(isPresented: $isSharing) {
            ShareSheetView(activityItems: [url])
        }
    }
}

public struct QuickLookControllerRepresentable: UIViewControllerRepresentable {
    public let url: URL
    
    public init(url: URL) {
        self.url = url
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(url: url)
    }
    
    public func makeUIViewController(context: Context) -> QuickLookContainerViewController {
        let container = QuickLookContainerViewController(url: url)
        container.qlController.dataSource = context.coordinator
        container.qlController.delegate = context.coordinator
        return container
    }
    
    public func updateUIViewController(_ uiViewController: QuickLookContainerViewController, context: Context) {
        context.coordinator.url = url
        uiViewController.url = url
    }
    
    public final class Coordinator: NSObject, QLPreviewControllerDataSource, QLPreviewControllerDelegate {
        var url: URL
        
        init(url: URL) {
            self.url = url
        }
        
        public func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
            return 1
        }
        
        public func previewController(_ controller: QLPreviewController, previewItemAt index: Int) -> QLPreviewItem {
            return url as NSURL
        }
    }
}

public final class QuickLookContainerViewController: UIViewController {
    let qlController = QLPreviewController()
    var url: URL {
        didSet {
            qlController.reloadData()
        }
    }
    
    init(url: URL) {
        self.url = url
        super.init(nibName: nil, bundle: nil)
    }
    
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }
    
    public override func viewDidLoad() {
        super.viewDidLoad()
        addChild(qlController)
        view.addSubview(qlController.view)
        qlController.view.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            qlController.view.topAnchor.constraint(equalTo: view.topAnchor),
            qlController.view.bottomAnchor.constraint(equalTo: view.bottomAnchor),
            qlController.view.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            qlController.view.trailingAnchor.constraint(equalTo: view.trailingAnchor)
        ])
        qlController.didMove(toParent: self)
    }
}

// MARK: - Interactive HTML Presentation Sheet (WKWebView)

public struct HTMLPreviewSheet: View {
    public let url: URL
    public let title: String
    public let onDismiss: () -> Void
    @State private var isSharing: Bool = false
    
    public init(url: URL, title: String = "", onDismiss: @escaping () -> Void) {
        self.url = url
        let fallback = url.lastPathComponent.removingPercentEncoding ?? url.lastPathComponent
        self.title = title.isEmpty ? fallback : title
        self.onDismiss = onDismiss
    }
    
    public var body: some View {
        VStack(spacing: 0) {
            // Floating grab handle hinting pull-down dismissal
            Capsule()
                .fill(Color(uiColor: .tertiaryLabel))
                .frame(width: 38, height: 5)
                .padding(.top, 10)
                .padding(.bottom, 12)
            
            // Header bar
            HStack {
                Button("完成") {
                    onDismiss()
                }
                .font(.system(size: 16, weight: .semibold))
                .frame(width: 60, alignment: .leading)
                
                Spacer()
                
                Text(title)
                    .font(.system(size: 17, weight: .bold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                
                Spacer()
                
                Button {
                    isSharing = true
                } label: {
                    Image(systemName: "square.and.arrow.up")
                        .font(.system(size: 16, weight: .semibold))
                }
                .frame(width: 60, alignment: .trailing)
                .accessibilityLabel("发送")
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            
            Divider()
            
            HTMLWebViewRepresentable(url: url)
                .ignoresSafeArea(edges: .bottom)
        }
        .sheet(isPresented: $isSharing) {
            ShareSheetView(activityItems: [url])
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
