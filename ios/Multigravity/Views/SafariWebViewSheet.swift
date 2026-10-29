import SwiftUI
import SafariServices

/// 应用内 Safari 沉浸式安全浏览器（保留 Safari Cookie、钥匙串密码自动填充与 Reader 阅读模式）
public struct SafariWebViewSheet: UIViewControllerRepresentable {
    public let url: URL
    public let onDismiss: (() -> Void)?
    
    public init(url: URL, onDismiss: (() -> Void)? = nil) {
        self.url = url
        self.onDismiss = onDismiss
    }
    
    public func makeUIViewController(context: Context) -> SFSafariViewController {
        let configuration = SFSafariViewController.Configuration()
        configuration.entersReaderIfAvailable = false
        configuration.barCollapsingEnabled = true
        
        let controller = SFSafariViewController(url: url, configuration: configuration)
        controller.preferredControlTintColor = UIColor.systemIndigo
        controller.dismissButtonStyle = .done
        controller.delegate = context.coordinator
        return controller
    }
    
    public func updateUIViewController(_ uiViewController: SFSafariViewController, context: Context) {}
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(onDismiss: onDismiss)
    }
    
    public final class Coordinator: NSObject, SFSafariViewControllerDelegate {
        private let onDismiss: (() -> Void)?
        
        public init(onDismiss: (() -> Void)?) {
            self.onDismiss = onDismiss
        }
        
        public func safariViewControllerDidFinish(_ controller: SFSafariViewController) {
            onDismiss?()
        }
    }
}

public struct IdentifiableURL: Identifiable, Sendable, Hashable {
    public var id: String { url.absoluteString }
    public let url: URL
    
    public init(url: URL) {
        self.url = url
    }
}
