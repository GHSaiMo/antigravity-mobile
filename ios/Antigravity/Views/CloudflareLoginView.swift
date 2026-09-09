import SwiftUI
import WebKit

public struct CloudflareLoginView: View {
    @Environment(\.dismiss) private var dismiss
    public let serverURL: URL
    public let onLoginSuccess: () -> Void
    
    public init(serverURL: URL, onLoginSuccess: @escaping () -> Void) {
        self.serverURL = serverURL
        self.onLoginSuccess = onLoginSuccess
    }
    
    public var body: some View {
        NavigationStack {
            WebViewContainer(url: serverURL, onAuthSuccess: {
                UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
                onLoginSuccess()
                dismiss()
            })
            .navigationTitle("Cloudflare 验证")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("取消") {
                        dismiss()
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("已完成") {
                        syncCookies()
                        onLoginSuccess()
                        dismiss()
                    }
                    .fontWeight(.semibold)
                }
            }
        }
    }
    
    private func syncCookies() {
        WKWebsiteDataStore.default().httpCookieStore.getAllCookies { cookies in
            for cookie in cookies {
                HTTPCookieStorage.shared.setCookie(cookie)
            }
        }
    }
}

private struct WebViewContainer: UIViewRepresentable {
    let url: URL
    let onAuthSuccess: () -> Void
    
    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()
        let webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = context.coordinator
        let request = URLRequest(url: url)
        webView.load(request)
        return webView
    }
    
    func updateUIView(_ uiView: WKWebView, context: Context) {}
    
    func makeCoordinator() -> Coordinator {
        Coordinator(parent: self)
    }
    
    class Coordinator: NSObject, WKNavigationDelegate {
        let parent: WebViewContainer
        
        init(parent: WebViewContainer) {
            self.parent = parent
        }
        
        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            // Check if CF_Authorization cookie is present
            webView.configuration.websiteDataStore.httpCookieStore.getAllCookies { cookies in
                var hasAuth = false
                for cookie in cookies {
                    HTTPCookieStorage.shared.setCookie(cookie)
                    if cookie.name.contains("CF_Authorization") {
                        hasAuth = true
                    }
                }
                
                // If auth cookie found or landed on app HTML
                if hasAuth {
                    DispatchQueue.main.async {
                        self.parent.onAuthSuccess()
                    }
                }
            }
        }
    }
}
