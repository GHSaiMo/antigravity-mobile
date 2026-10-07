import SwiftUI
import WebKit

// MARK: - In-Memory Cache for Agent Embed HTML Content

public final class AgentEmbedCache: @unchecked Sendable {
    public static let shared = AgentEmbedCache()
    private let cache = NSCache<NSString, NSString>()
    
    private init() {
        cache.countLimit = 100
    }
    
    public func content(for src: String) -> String? {
        cache.object(forKey: src as NSString) as String?
    }
    
    public func insert(_ content: String, for src: String) {
        cache.setObject(content as NSString, forKey: src as NSString)
    }
    
    public func remove(for src: String) {
        cache.removeObject(forKey: src as NSString)
    }
}

// MARK: - AgentEmbedView

public struct AgentEmbedView: View {
    public let src: String
    
    @State private var htmlContent: String? = nil
    @State private var isLoading: Bool = false
    @State private var errorMessage: String? = nil
    @State private var embedHeight: CGFloat = 340
    @State private var detectedTitle: String? = nil
    @State private var isFullscreen: Bool = false
    @State private var reloadKey: Int = 0
    
    @Environment(\.colorScheme) private var colorScheme
    
    public init(src: String) {
        self.src = src
    }
    
    private var defaultTitle: String {
        let clean = src.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
        let filename = (clean as NSString).lastPathComponent
        let nameWithoutExt = (filename as NSString).deletingPathExtension
        if !nameWithoutExt.isEmpty && nameWithoutExt != "interactive_preview" && nameWithoutExt != "widget" {
            return nameWithoutExt
        }
        return "交互预览"
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header Bar
            HStack(spacing: 8) {
                HStack(spacing: 6) {
                    Image(systemName: "slider.horizontal.3")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.indigo)
                    
                    Text(detectedTitle ?? defaultTitle)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    
                    Text("INTERACTIVE")
                        .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                        .textCase(.uppercase)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1.5)
                        .background(Color.indigo.opacity(0.12))
                        .foregroundColor(.indigo)
                        .clipShape(Capsule())
                }
                
                Spacer()
                
                // Refresh Button
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    AgentEmbedCache.shared.remove(for: src)
                    reloadKey += 1
                    Task {
                        await loadHtmlContent()
                    }
                }) {
                    Image(systemName: "arrow.clockwise")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(4)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("刷新交互组件")
                
                // Fullscreen Button
                if htmlContent != nil && errorMessage == nil {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        isFullscreen = true
                    }) {
                        Image(systemName: "arrow.up.left.and.arrow.down.right")
                            .font(.system(size: 10.5, weight: .medium))
                            .foregroundColor(.secondary)
                            .padding(4)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("全屏查看")
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 7)
            .background(Color(uiColor: .tertiarySystemBackground).opacity(0.85))
            
            Divider()
            
            // Content Area
            ZStack(alignment: .bottomTrailing) {
                if let error = errorMessage {
                    VStack(alignment: .center, spacing: 10) {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 14))
                                .foregroundColor(.orange)
                            Text("交互卡片加载失败")
                                .font(.system(size: 12.5, weight: .semibold))
                                .foregroundColor(.primary)
                        }
                        
                        Text(error)
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .lineLimit(2)
                            .padding(.horizontal, 16)
                        
                        Button(action: {
                            Task {
                                await loadHtmlContent()
                            }
                        }) {
                            Text("重试")
                                .font(.system(size: 12, weight: .medium))
                                .padding(.horizontal, 14)
                                .padding(.vertical, 5)
                                .background(Color.indigo.opacity(0.12))
                                .foregroundColor(.indigo)
                                .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                    }
                    .frame(maxWidth: .infinity)
                    .frame(height: 140)
                    .background(Color(uiColor: .secondarySystemBackground).opacity(0.3))
                } else if let html = htmlContent {
                    AgentEmbedWebView(
                        htmlContent: html,
                        isDark: colorScheme == .dark,
                        baseURL: AppSettings.shared.gatewayURL,
                        reloadKey: reloadKey,
                        onHeightChange: { height in
                            // Clamp inline height between 160 and 520
                            let target = max(160, min(height, 520))
                            withAnimation(.easeInOut(duration: 0.2)) {
                                self.embedHeight = target
                            }
                        },
                        onTitleDetected: { title in
                            if let title, !title.isEmpty, self.detectedTitle == nil {
                                withAnimation(.easeInOut(duration: 0.2)) {
                                    self.detectedTitle = title
                                }
                            }
                        }
                    )
                    .frame(height: embedHeight)
                    .frame(maxWidth: .infinity)
                    
                    // Subtle hint pill if scrollable or for fullscreen
                    if embedHeight >= 480 {
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            isFullscreen = true
                        }) {
                            HStack(spacing: 4) {
                                Image(systemName: "arrow.up.left.and.arrow.down.right")
                                    .font(.system(size: 9.5))
                                Text("全屏操作")
                                    .font(.system(size: 10, weight: .medium))
                            }
                            .foregroundColor(.secondary.opacity(0.85))
                            .padding(.horizontal, 7)
                            .padding(.vertical, 3.5)
                            .background(Color(uiColor: .systemBackground).opacity(0.85))
                            .clipShape(Capsule())
                            .shadow(color: Color.black.opacity(0.08), radius: 2, y: 1)
                        }
                        .buttonStyle(.plain)
                        .padding(8)
                    }
                } else {
                    // Loading indicator
                    VStack(spacing: 8) {
                        ProgressView()
                            .scaleEffect(0.85)
                        Text("正在加载交互组件...")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .frame(height: 160)
                }
            }
        }
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.secondary.opacity(0.2), lineWidth: 1)
        )
        .task(id: src) {
            await loadHtmlContent()
        }
        .sheet(isPresented: $isFullscreen) {
            if let html = htmlContent {
                AgentEmbedFullscreenViewer(
                    htmlContent: html,
                    title: detectedTitle ?? defaultTitle,
                    baseURL: AppSettings.shared.gatewayURL
                )
            }
        }
    }
    
    private func loadHtmlContent() async {
        if let cached = AgentEmbedCache.shared.content(for: src) {
            self.htmlContent = cached
            self.errorMessage = nil
            return
        }
        
        guard let baseURL = AppSettings.shared.gatewayURL else {
            self.errorMessage = "未配置网关服务器"
            return
        }
        
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        
        do {
            let resp = try await APIClient.shared.fetchFileContent(
                uri: src,
                baseURL: baseURL
            )
            AgentEmbedCache.shared.insert(resp.content, for: src)
            self.htmlContent = resp.content
            self.errorMessage = nil
        } catch {
            self.errorMessage = error.localizedDescription
        }
    }
}

// MARK: - AgentEmbed WKWebView Representable

public struct AgentEmbedWebView: UIViewRepresentable {
    public let htmlContent: String
    public let isDark: Bool
    public let baseURL: URL?
    public let reloadKey: Int
    public var onHeightChange: ((CGFloat) -> Void)? = nil
    public var onTitleDetected: ((String?) -> Void)? = nil
    
    public init(
        htmlContent: String,
        isDark: Bool,
        baseURL: URL?,
        reloadKey: Int = 0,
        onHeightChange: ((CGFloat) -> Void)? = nil,
        onTitleDetected: ((String?) -> Void)? = nil
    ) {
        self.htmlContent = htmlContent
        self.isDark = isDark
        self.baseURL = baseURL
        self.reloadKey = reloadKey
        self.onHeightChange = onHeightChange
        self.onTitleDetected = onTitleDetected
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }
    
    public func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        let controller = WKUserContentController()
        
        controller.add(context.coordinator, name: "agentEmbedSize")
        controller.add(context.coordinator, name: "agentEmbedTitle")
        config.userContentController = controller
        
        let webView = WKWebView(frame: .zero, configuration: config)
        webView.isOpaque = false
        webView.backgroundColor = .clear
        webView.scrollView.backgroundColor = .clear
        webView.scrollView.showsVerticalScrollIndicator = true
        webView.scrollView.showsHorizontalScrollIndicator = false
        webView.scrollView.bounces = true
        
        // Disable native zoom to keep content fixed
        webView.scrollView.minimumZoomScale = 1.0
        webView.scrollView.maximumZoomScale = 1.0
        webView.scrollView.bouncesZoom = false
        webView.scrollView.pinchGestureRecognizer?.isEnabled = false
        webView.scrollView.delegate = context.coordinator
        webView.navigationDelegate = context.coordinator
        
        context.coordinator.webView = webView
        context.coordinator.disableZoomGestures()
        loadContent(into: webView)
        return webView
    }
    
    public func updateUIView(_ uiView: WKWebView, context: Context) {
        context.coordinator.parent = self
        context.coordinator.disableZoomGestures()
        if context.coordinator.lastHtml != htmlContent ||
            context.coordinator.lastIsDark != isDark ||
            context.coordinator.lastReloadKey != reloadKey {
            context.coordinator.lastHtml = htmlContent
            context.coordinator.lastIsDark = isDark
            context.coordinator.lastReloadKey = reloadKey
            loadContent(into: uiView)
        }
    }
    
    private func loadContent(into webView: WKWebView) {
        let enhancedHTML = AgentEmbedHTMLBuilder.build(html: htmlContent, isDark: isDark)
        webView.loadHTMLString(enhancedHTML, baseURL: baseURL)
    }
    
    public final class Coordinator: NSObject, WKScriptMessageHandler, UIScrollViewDelegate, WKNavigationDelegate {
        var parent: AgentEmbedWebView
        weak var webView: WKWebView?
        var lastHtml: String = ""
        var lastIsDark: Bool = false
        var lastReloadKey: Int = 0
        
        init(_ parent: AgentEmbedWebView) {
            self.parent = parent
            self.lastHtml = parent.htmlContent
            self.lastIsDark = parent.isDark
            self.lastReloadKey = parent.reloadKey
        }
        
        // MARK: - UIScrollViewDelegate (Disable Zooming)
        public func viewForZooming(in scrollView: UIScrollView) -> UIView? {
            return nil
        }
        
        // MARK: - WKNavigationDelegate & Gesture Control
        public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            disableZoomGestures()
        }
        
        public func disableZoomGestures() {
            guard let webView = self.webView else { return }
            disableGestures(in: webView)
            
            // Re-check after brief delays to disable dynamically attached WKContentView recognizers
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                if let wv = self?.webView {
                    self?.disableGestures(in: wv)
                }
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { [weak self] in
                if let wv = self?.webView {
                    self?.disableGestures(in: wv)
                }
            }
        }
        
        private func disableGestures(in view: UIView) {
            for recognizer in view.gestureRecognizers ?? [] {
                if let tap = recognizer as? UITapGestureRecognizer, tap.numberOfTapsRequired == 2 {
                    tap.isEnabled = false
                }
                if recognizer is UIPinchGestureRecognizer {
                    recognizer.isEnabled = false
                }
            }
            for subview in view.subviews {
                disableGestures(in: subview)
            }
        }
        
        public func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
            DispatchQueue.main.async {
                if message.name == "agentEmbedSize", let body = message.body as? [String: Any] {
                    if let height = body["height"] as? CGFloat, height > 0 {
                        self.parent.onHeightChange?(height)
                    }
                } else if message.name == "agentEmbedTitle", let body = message.body as? [String: Any] {
                    if let title = body["title"] as? String {
                        self.parent.onTitleDetected?(title)
                    }
                }
            }
        }
    }
}

// MARK: - Fullscreen Viewer

public struct AgentEmbedFullscreenViewer: View {
    public let htmlContent: String
    public let title: String
    public let baseURL: URL?
    
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme
    @State private var reloadKey: Int = 0
    
    public init(htmlContent: String, title: String, baseURL: URL?) {
        self.htmlContent = htmlContent
        self.title = title
        self.baseURL = baseURL
    }
    
    public var body: some View {
        NavigationStack {
            AgentEmbedWebView(
                htmlContent: htmlContent,
                isDark: colorScheme == .dark,
                baseURL: baseURL,
                reloadKey: reloadKey,
                onHeightChange: nil,
                onTitleDetected: nil
            )
            .background(Color(uiColor: .systemBackground))
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("完成") {
                        dismiss()
                    }
                    .font(.system(size: 15, weight: .semibold))
                }
                
                ToolbarItem(placement: .primaryAction) {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        reloadKey += 1
                    }) {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 14))
                    }
                }
            }
        }
    }
}

// MARK: - HTML Enhancement Builder

private enum AgentEmbedHTMLBuilder {
    static func build(html: String, isDark: Bool) -> String {
        // Theme variables matching Antigravity Generative UI standards
        let themeCSS = """
        <style id="antigravity-embed-injected-theme">
          :root {
            --background: \(isDark ? "#18181b" : "#ffffff");
            --foreground: \(isDark ? "#f4f4f5" : "#18181b");
            --muted-foreground: \(isDark ? "#a1a1aa" : "#71717a");
            --card: \(isDark ? "#27272a" : "#f4f4f5");
            --sidebar: \(isDark ? "#18181b" : "#fafafa");
            --border: \(isDark ? "rgba(255, 255, 255, 0.12)" : "rgba(0, 0, 0, 0.10)");
            --primary: \(isDark ? "#6366f1" : "#4f46e5");
            --primary-foreground: #ffffff;
            --secondary: \(isDark ? "#27272a" : "#e4e4e7");
            --secondary-foreground: \(isDark ? "#f4f4f5" : "#18181b");
            --accent: \(isDark ? "rgba(255, 255, 255, 0.08)" : "rgba(0, 0, 0, 0.05)");
            color-scheme: \(isDark ? "dark" : "light");
          }
          * {
            box-sizing: border-box;
          }
          html, body {
            margin: 0;
            padding: 8px;
            background: transparent !important;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            -webkit-text-size-adjust: 100%;
            -webkit-tap-highlight-color: transparent;
            touch-action: pan-x pan-y;
            overscroll-behavior: none;
          }
          img, svg, video {
            max-width: 100% !important;
          }
        </style>
        """
        
        let viewportMeta = """
        <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0, user-scalable=no, viewport-fit=cover">
        """
        
        let reporterScript = """
        <script id="antigravity-embed-injected-reporter">
          (function() {
            // Prevent pinch-to-zoom gestures in WebKit/Safari
            document.addEventListener('gesturestart', function(e) { e.preventDefault(); }, { passive: false });
            document.addEventListener('gesturechange', function(e) { e.preventDefault(); }, { passive: false });
            document.addEventListener('gestureend', function(e) { e.preventDefault(); });

            function reportSize() {
              try {
                var body = document.body;
                var html = document.documentElement;
                if (!body) return;
                var card = body.firstElementChild || body;
                var rect = card.getBoundingClientRect();
                var h = Math.ceil(rect.height || card.scrollHeight || 0) + 16;
                if (!h || h < 80) {
                  h = Math.max(body.scrollHeight, body.offsetHeight, html.clientHeight, html.scrollHeight, html.offsetHeight);
                }
                if (h > 0 && window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.agentEmbedSize) {
                  window.webkit.messageHandlers.agentEmbedSize.postMessage({ height: h });
                }
              } catch(e) {}
            }

            function reportTitle() {
              try {
                var el = document.querySelector('h1, h2, h3, h4, title');
                if (el && el.innerText) {
                  var t = el.innerText.trim();
                  if (t.length > 0 && t.length < 50 && window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.agentEmbedTitle) {
                    window.webkit.messageHandlers.agentEmbedTitle.postMessage({ title: t });
                  }
                }
              } catch(e) {}
            }

            if (document.readyState === 'complete' || document.readyState === 'interactive') {
              reportSize();
              reportTitle();
            } else {
              window.addEventListener('DOMContentLoaded', function() { reportSize(); reportTitle(); });
              window.addEventListener('load', function() { reportSize(); reportTitle(); });
            }

            if (window.ResizeObserver && document.body) {
              var ro = new ResizeObserver(function() { reportSize(); });
              ro.observe(document.body);
            }
            setTimeout(reportSize, 120);
            setTimeout(reportSize, 400);
            setTimeout(reportSize, 1200);
          })();
        </script>
        """
        
        var modified = html
        
        // Strip any existing viewport meta tags to prevent conflicting zoom/scale settings
        if let vpRegex = try? NSRegularExpression(pattern: #"<meta\s+[^>]*?name=["']viewport["'][^>]*>"#, options: .caseInsensitive) {
            let range = NSRange(modified.startIndex..<modified.endIndex, in: modified)
            modified = vpRegex.stringByReplacingMatches(in: modified, options: [], range: range, withTemplate: "")
        }
        
        let darkClass = isDark ? "dark" : "light"
        
        // Ensure <html class="..."> has proper dark/light theme class
        if modified.contains("<html") {
            if modified.contains("class=\"") {
                modified = modified.replacingOccurrences(of: "class=\"", with: "class=\"\(darkClass) ")
            } else {
                modified = modified.replacingOccurrences(of: "<html", with: "<html class=\"\(darkClass)\"")
            }
        }
        
        // Inject into <head> if present, otherwise prepend
        let injections = "\n" + viewportMeta + "\n" + themeCSS + "\n" + reporterScript + "\n"
        if let headRange = modified.range(of: "<head>", options: .caseInsensitive) {
            modified.insert(contentsOf: injections, at: headRange.upperBound)
        } else if let htmlRange = modified.range(of: "<html>", options: .caseInsensitive) {
            modified.insert(contentsOf: "<head>" + injections + "</head>", at: htmlRange.upperBound)
        } else {
            modified = "<!DOCTYPE html><html class=\"\(darkClass)\"><head>" + injections + "</head><body>" + modified + "</body></html>"
        }
        
        return modified
    }
}
