import SwiftUI
import Foundation
import UIKit
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
    @Environment(\.currentCascadeId) private var currentCascadeId
    
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
        return String(localized: "交互预览")
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
                        baseURL: URL(string: "about:blank"),
                        reloadKey: reloadKey,
                        embedSrc: src,
                        boundCascadeId: currentCascadeId ?? "",
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
                    baseURL: URL(string: "about:blank"),
                    embedSrc: src,
                    boundCascadeId: currentCascadeId ?? ""
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
            self.errorMessage = String(localized: "未配置网关服务器")
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

// MARK: - MCP App Namespace Helper

private enum AgentEmbedSecurityPolicy {
    static func extractMcpServer(from rawURI: String) -> String {
        let trimmed = rawURI.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
        guard !trimmed.isEmpty else { return "" }
        let lower = trimmed.lowercased()
        if lower.hasPrefix("ui://") || lower.hasPrefix("mcp://") {
            guard let comps = URLComponents(string: trimmed) else { return "" }
            if let sn = comps.queryItems?.first(where: { $0.name == "serverName" })?.value?.trimmingCharacters(in: .whitespacesAndNewlines), !sn.isEmpty {
                return sn
            }
            if let sn = comps.queryItems?.first(where: { $0.name == "server" })?.value?.trimmingCharacters(in: .whitespacesAndNewlines), !sn.isEmpty {
                return sn
            }
            if let host = comps.host?.trimmingCharacters(in: .whitespacesAndNewlines), !host.isEmpty {
                return host
            }
            return ""
        }
        if let comps = URLComponents(string: trimmed) {
            if let nestedUri = comps.queryItems?.first(where: { $0.name == "uri" || $0.name == "appUri" })?.value {
                let nested = extractMcpServer(from: nestedUri)
                if !nested.isEmpty { return nested }
            }
            if let sn = comps.queryItems?.first(where: { $0.name == "serverName" || $0.name == "server" })?.value?.trimmingCharacters(in: .whitespacesAndNewlines),
               !sn.isEmpty,
               comps.path.contains("/gateway/mcp/app/") {
                return sn
            }
        }
        return ""
    }

    static func isToolAllowed(toolName: String, boundServer: String) -> Bool {
        let cleanTool = toolName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanTool.isEmpty, !boundServer.isEmpty else { return false }
        let expectedLower = boundServer.lowercased()
        if let slashIdx = cleanTool.firstIndex(of: "/") {
            let prefix = String(cleanTool[..<slashIdx]).lowercased()
            return prefix == expectedLower
        }
        if let colonIdx = cleanTool.firstIndex(of: ":") {
            let prefix = String(cleanTool[..<colonIdx]).lowercased()
            return prefix == expectedLower
        }
        if cleanTool.lowercased().hasPrefix("mcp_") {
            let expectedPrefix = "mcp_\(expectedLower)_"
            let expectedHyphenNormalized = "mcp_\(expectedLower.replacingOccurrences(of: "-", with: "_"))_"
            let lowerTool = cleanTool.lowercased()
            return lowerTool.hasPrefix(expectedPrefix) || lowerTool.hasPrefix(expectedHyphenNormalized)
        }
        return true
    }
}

// MARK: - AgentEmbed WKWebView Representable

public struct AgentEmbedWebView: UIViewRepresentable {
    public let htmlContent: String
    public let isDark: Bool
    public let baseURL: URL?
    public let reloadKey: Int
    public let embedSrc: String
    public let boundCascadeId: String
    public var onHeightChange: ((CGFloat) -> Void)? = nil
    public var onTitleDetected: ((String?) -> Void)? = nil
    
    public init(
        htmlContent: String,
        isDark: Bool,
        baseURL: URL?,
        reloadKey: Int = 0,
        embedSrc: String = "",
        boundCascadeId: String = "",
        onHeightChange: ((CGFloat) -> Void)? = nil,
        onTitleDetected: ((String?) -> Void)? = nil
    ) {
        self.htmlContent = htmlContent
        self.isDark = isDark
        self.baseURL = URL(string: "about:blank")
        self.reloadKey = reloadKey
        self.embedSrc = embedSrc
        self.boundCascadeId = boundCascadeId
        self.onHeightChange = onHeightChange
        self.onTitleDetected = onTitleDetected
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }
    
    public func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.preferences.setValue(false, forKey: "allowFileAccessFromFileURLs")
        let controller = WKUserContentController()
        
        controller.add(context.coordinator, name: "agentEmbedSize")
        controller.add(context.coordinator, name: "agentEmbedTitle")
        controller.add(context.coordinator, name: "agentEmbedBridge")
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
            context.coordinator.lastReloadKey != reloadKey {
            context.coordinator.lastHtml = htmlContent
            context.coordinator.lastIsDark = isDark
            context.coordinator.lastReloadKey = reloadKey
            loadContent(into: uiView)
        } else if context.coordinator.lastIsDark != isDark {
            context.coordinator.lastIsDark = isDark
            let js = AntigravityHTMLThemeHelper.liveThemeUpdateJS(
                isDark: isDark,
                mode: .inlineEmbed
            )
            uiView.evaluateJavaScript(js, completionHandler: nil)
        }
    }
    
    private func loadContent(into webView: WKWebView) {
        let enhancedHTML = AgentEmbedHTMLBuilder.build(html: htmlContent, isDark: isDark)
        webView.loadHTMLString(enhancedHTML, baseURL: URL(string: "about:blank"))
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
        public func webView(
            _ webView: WKWebView,
            decidePolicyFor navigationAction: WKNavigationAction
        ) async -> WKNavigationActionPolicy {
            guard let targetURL = navigationAction.request.url else {
                return .cancel
            }
            let scheme = targetURL.scheme?.lowercased() ?? ""
            if targetURL.absoluteString == "about:blank" || scheme == "about" {
                return .allow
            }
            if scheme == "file" || targetURL.isFileURL {
                return .cancel
            }
            let isMainFrame = navigationAction.targetFrame?.isMainFrame ?? true
            if isMainFrame {
                if navigationAction.navigationType == .linkActivated && (scheme == "http" || scheme == "https") {
                    await UIApplication.shared.open(targetURL)
                }
                return .cancel
            }
            if scheme == "http" || scheme == "https" || scheme == "data" || scheme == "blob" {
                return .allow
            }
            return .cancel
        }

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
            let isMainFrame = message.frameInfo.isMainFrame
            let frameScheme = message.frameInfo.request.url?.scheme?.lowercased() ?? "about"
            guard isMainFrame, frameScheme.isEmpty || frameScheme == "about" else { return }
            DispatchQueue.main.async {
                if message.name == "agentEmbedSize", let body = message.body as? [String: Any] {
                    if let height = body["height"] as? CGFloat, height > 0 {
                        self.parent.onHeightChange?(height)
                    }
                } else if message.name == "agentEmbedTitle", let body = message.body as? [String: Any] {
                    if let title = body["title"] as? String {
                        self.parent.onTitleDetected?(title)
                    }
                } else if message.name == "agentEmbedBridge", let body = message.body as? [String: Any] {
                    Self.handleBridgeAction(
                        body,
                        webView: self.webView,
                        embedSrc: self.parent.embedSrc,
                        boundCascadeId: self.parent.boundCascadeId
                    )
                }
            }
        }

        private static func toJSONValue(_ value: Any?) -> JSONValue {
            guard let value else { return .null }
            if let s = value as? String { return .string(s) }
            if let b = value as? Bool { return .bool(b) }
            if let i = value as? Int { return .int(i) }
            if let d = value as? Double { return .double(d) }
            if let arr = value as? [Any] { return .array(arr.map { toJSONValue($0) }) }
            if let dict = value as? [String: Any] {
                var out: [String: JSONValue] = [:]
                for (k, v) in dict { out[k] = toJSONValue(v) }
                return .object(out)
            }
            return .null
        }

        @MainActor
        private static func postBridgeReply(to webView: WKWebView?, id: Any?, jsonrpc: String?, ok: Bool, rawJSON: String? = nil, error: String? = nil) {
            guard let webView else { return }
            var envelope: [String: Any] = ["ok": ok]
            if let id { envelope["id"] = id }
            if let jsonrpc { envelope["jsonrpc"] = jsonrpc }
            if ok {
                if let rawJSON,
                   let data = rawJSON.data(using: .utf8),
                   let parsed = try? JSONSerialization.jsonObject(with: data) {
                    envelope["result"] = parsed
                } else {
                    envelope["result"] = ["ok": true]
                }
            } else {
                envelope["error"] = error ?? "request failed"
            }
            guard let jsonData = try? JSONSerialization.data(withJSONObject: envelope),
                  let jsonText = String(data: jsonData, encoding: .utf8) else { return }
            let js = "window.dispatchEvent(new MessageEvent('message', { data: \(jsonText) }));"
            webView.evaluateJavaScript(js, completionHandler: nil)
        }

        @MainActor
        private static func handleBridgeAction(
            _ body: [String: Any],
            webView: WKWebView?,
            embedSrc: String,
            boundCascadeId: String
        ) {
            let activeScheme = webView?.url?.scheme?.lowercased() ?? "about"
            guard activeScheme.isEmpty || activeScheme == "about" else { return }
            guard let gwUrl = AppSettings.shared.gatewayURL else { return }
            let action = (body["method"] as? String) ?? (body["type"] as? String) ?? ""
            let params = (body["params"] as? [String: Any]) ?? body
            let reqId = body["id"]
            let jsonrpc = body["jsonrpc"] as? String
            let boundServer = AgentEmbedSecurityPolicy.extractMcpServer(from: embedSrc)
            switch action {
            case "sendMessage", "ui/message":
                let text = ((params["text"] as? String) ?? (params["message"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                // Always bind strictly to the current host conversation; ignore untrusted JS cascadeId.
                let cid = !boundCascadeId.isEmpty ? boundCascadeId : ChatViewModel.activeCascadeId
                guard !text.isEmpty, !cid.isEmpty else {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "No active conversation bound")
                    return
                }
                Task { @MainActor in
                    do {
                        try await APIClient.shared.sendMessage(cascadeId: cid, text: text, baseURL: gwUrl)
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: true)
                    } catch {
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: error.localizedDescription)
                    }
                }
            case "callServerTool", "tools/call":
                let toolName = ((params["name"] as? String) ?? (params["toolName"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                let reqServer = ((params["serverName"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                let reqAppUri = ((params["appUri"] as? String) ?? (params["uri"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                guard !toolName.isEmpty else { return }
                guard !boundServer.isEmpty else {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "MCP tool execution denied outside bound MCP App")
                    return
                }
                if !reqServer.isEmpty && reqServer.caseInsensitiveCompare(boundServer) != .orderedSame {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "Cross-server MCP tool execution denied")
                    return
                }
                let reqUriServer = AgentEmbedSecurityPolicy.extractMcpServer(from: reqAppUri)
                if !reqUriServer.isEmpty && reqUriServer.caseInsensitiveCompare(boundServer) != .orderedSame {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "Cross-server MCP appUri denied")
                    return
                }
                guard AgentEmbedSecurityPolicy.isToolAllowed(toolName: toolName, boundServer: boundServer) else {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "Tool namespace mismatch for MCP server")
                    return
                }
                let args = toJSONValue(params["arguments"])
                let lockedAppUri = embedSrc.trimmingCharacters(in: .whitespacesAndNewlines)
                Task { @MainActor in
                    do {
                        let raw = try await APIClient.shared.callMcpAppTool(
                            toolName: toolName,
                            arguments: args,
                            appUri: lockedAppUri,
                            serverName: boundServer,
                            baseURL: gwUrl
                        )
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: true, rawJSON: raw)
                    } catch {
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: error.localizedDescription)
                    }
                }
            case "resources/read":
                let uri = ((params["uri"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                let reqServer = ((params["serverName"] as? String) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                guard !uri.isEmpty else { return }
                guard !boundServer.isEmpty else {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "MCP resource read denied outside bound MCP App")
                    return
                }
                if !reqServer.isEmpty && reqServer.caseInsensitiveCompare(boundServer) != .orderedSame {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "Cross-server MCP resource read denied")
                    return
                }
                let uriServer = AgentEmbedSecurityPolicy.extractMcpServer(from: uri)
                if !uriServer.isEmpty && uriServer.caseInsensitiveCompare(boundServer) != .orderedSame {
                    postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: "Cross-server MCP resource URI denied")
                    return
                }
                Task { @MainActor in
                    do {
                        let raw = try await APIClient.shared.getMcpAppResource(
                            uri: uri,
                            serverName: boundServer,
                            baseURL: gwUrl
                        )
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: true, rawJSON: raw)
                    } catch {
                        postBridgeReply(to: webView, id: reqId, jsonrpc: jsonrpc, ok: false, error: error.localizedDescription)
                    }
                }
            default:
                break
            }
        }
    }
}

// MARK: - Fullscreen Viewer

public struct AgentEmbedFullscreenViewer: View {
    public let htmlContent: String
    public let title: String
    public let baseURL: URL?
    public let embedSrc: String
    public let boundCascadeId: String
    
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme
    @State private var reloadKey: Int = 0
    
    public init(
        htmlContent: String,
        title: String,
        baseURL: URL?,
        embedSrc: String = "",
        boundCascadeId: String = ""
    ) {
        self.htmlContent = htmlContent
        self.title = title
        self.baseURL = URL(string: "about:blank")
        self.embedSrc = embedSrc
        self.boundCascadeId = boundCascadeId
    }
    
    public var body: some View {
        NavigationStack {
            AgentEmbedWebView(
                htmlContent: htmlContent,
                isDark: colorScheme == .dark,
                baseURL: URL(string: "about:blank"),
                reloadKey: reloadKey,
                embedSrc: embedSrc,
                boundCascadeId: boundCascadeId,
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

// MARK: - Shared Antigravity Generative UI HTML Theme Helper

internal enum AntigravityHTMLRenderMode {
    case inlineEmbed
    case standalone
}

internal struct AntigravityPreparedHTML {
    let html: String
    let isGenerativeUI: Bool
}

internal enum AntigravityHTMLThemeHelper {
    static let styleElementID = "antigravity-theme-vars"

    static func darkBackgroundUIColor(isDark: Bool) -> UIColor {
        isDark
            ? UIColor(red: 15.0 / 255.0, green: 17.0 / 255.0, blue: 23.0 / 255.0, alpha: 1.0) // #0f1117
            : UIColor.white
    }

    /// Detects whether an HTML document relies on Antigravity Generative UI host-injected theme variables
    /// (and is not a self-styled Marp slide deck or a page that already defines its own :root palette).
    static func usesAntigravityTheme(_ html: String) -> Bool {
        let lower = html.lowercased()
        if lower.contains("data-marpit-svg") || lower.contains("bespoke-marp") || lower.contains("marpit") {
            return false
        }
        if lower.contains("gstatic.com/antigravity") {
            return true
        }
        if lower.contains("themechange") && lower.contains("addeventlistener") {
            return true
        }
        if lower.contains("classlist.contains('light')") || lower.contains("classlist.contains(\"light\")") {
            return true
        }
        let semanticRefs = [
            "var(--background)",
            "var(--foreground)",
            "var(--card)",
            "var(--card-border)",
            "var(--content)",
            "var(--muted-foreground)",
            "var(--chart-1)",
            "var(--chart-2)",
            "var(--chart-3)",
            "var(--code-foreground)"
        ]
        let hasSemanticRef = semanticRefs.contains { lower.contains($0) }
        guard hasSemanticRef else { return false }

        // If the document itself already defines both --background and --foreground in CSS, respect its own theme.
        let selfDefinesBackground = html.range(
            of: #"--background\s*:"#,
            options: [.regularExpression, .caseInsensitive]
        ) != nil
        let selfDefinesForeground = html.range(
            of: #"--foreground\s*:"#,
            options: [.regularExpression, .caseInsensitive]
        ) != nil
        return !(selfDefinesBackground && selfDefinesForeground)
    }

    /// Builds the CSS text inside `<style id="antigravity-theme-vars">`.
    static func themeCSSBody(isDark: Bool, mode: AntigravityHTMLRenderMode, includeContainerTheme: Bool) -> String {
        let paletteDark = includeContainerTheme ? isDark : false
        let vars = """
        :root {
          --background: \(paletteDark ? "#0f1117" : "#ffffff");
          --content: \(paletteDark ? "#13161f" : "#f8fafc");
          --card: \(paletteDark ? "#181c26" : "#f4f4f5");
          --sidebar: \(paletteDark ? "#141720" : "#fafafa");
          --card-border: \(paletteDark ? "rgba(255, 255, 255, 0.10)" : "rgba(15, 23, 42, 0.10)");
          --border: \(paletteDark ? "rgba(255, 255, 255, 0.14)" : "rgba(15, 23, 42, 0.13)");
          --foreground: \(paletteDark ? "#f4f4f5" : "#18181b");
          --muted-foreground: \(paletteDark ? "#a1a1aa" : "#64748b");
          --placeholder: \(paletteDark ? "#71717a" : "#94a3b8");
          --code-foreground: \(paletteDark ? "#e4e4e7" : "#1e293b");
          --primary: \(paletteDark ? "#6e7bf2" : "#4f46e5");
          --primary-foreground: #ffffff;
          --secondary: \(paletteDark ? "#27272a" : "#e4e4e7");
          --secondary-foreground: \(paletteDark ? "#f4f4f5" : "#18181b");
          --accent: \(paletteDark ? "rgba(255, 255, 255, 0.08)" : "rgba(15, 23, 42, 0.06)");
          --accent-foreground: \(paletteDark ? "#f4f4f5" : "#18181b");
          --muted: \(paletteDark ? "#27272a" : "#f4f4f5");
          --link: \(paletteDark ? "#60a5fa" : "#2563eb");
          --success: \(paletteDark ? "#34d399" : "#16a34a");
          --warning: \(paletteDark ? "#fbbf24" : "#d97706");
          --error: \(paletteDark ? "#f87171" : "#dc2626");
          --destructive: \(paletteDark ? "#f87171" : "#dc2626");
          --destructive-foreground: #ffffff;
          --chart-1: \(paletteDark ? "#60a5fa" : "#2563eb");
          --chart-2: \(paletteDark ? "#a78bfa" : "#7c3aed");
          --chart-3: \(paletteDark ? "#2dd4bf" : "#0d9488");
          --chart-4: \(paletteDark ? "#fb923c" : "#ea580c");
          --chart-5: \(paletteDark ? "#f472b6" : "#db2777");
          --chart-6: \(paletteDark ? "#facc15" : "#ca8a04");\(includeContainerTheme ? "\n  color-scheme: \(isDark ? "dark" : "light");" : "")
        }
        """

        switch mode {
        case .inlineEmbed:
            return """
            \(vars)
            :where(*) {
              box-sizing: border-box;
            }
            :where(html, body) {
              margin: 0;
              padding: 8px;
              background: transparent;
              color: var(--foreground);
              font-size: 14px;
              font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
              -webkit-text-size-adjust: 100%;
              -webkit-tap-highlight-color: transparent;
              touch-action: pan-x pan-y;
              overscroll-behavior: none;
            }
            :where(img, svg, video) {
              max-width: 100%;
            }
            """
        case .standalone:
            guard includeContainerTheme else {
                return vars
            }
            return """
            \(vars)
            :where(*) {
              box-sizing: border-box;
            }
            :where(html, body) {
              margin: 0;
              background-color: var(--background);
              color: var(--foreground);
              font-size: 14px;
              font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
              -webkit-text-size-adjust: 100%;
              -webkit-tap-highlight-color: transparent;
            }
            """
        }
    }

    static func liveThemeUpdateJS(isDark: Bool, mode: AntigravityHTMLRenderMode) -> String {
        let cssBody = themeCSSBody(isDark: isDark, mode: mode, includeContainerTheme: true)
        let encodedCSS: String = {
            if let data = try? JSONSerialization.data(withJSONObject: [cssBody], options: []),
               let arr = String(data: data, encoding: .utf8),
               arr.count >= 2 {
                return String(arr.dropFirst().dropLast())
            }
            return "\"\""
        }()
        let addClass = isDark ? "dark" : "light"
        let removeClass = isDark ? "light" : "dark"
        return """
        (function() {
          try {
            var styleEl = document.getElementById('\(styleElementID)');
            if (!styleEl) {
              styleEl = document.createElement('style');
              styleEl.id = '\(styleElementID)';
              var head = document.head || document.documentElement;
              head.insertBefore(styleEl, head.firstChild);
            }
            styleEl.textContent = \(encodedCSS);
            var root = document.documentElement;
            if (root) {
              root.classList.remove('\(removeClass)');
              root.classList.add('\(addClass)');
            }
            window.dispatchEvent(new Event('themechange'));
          } catch (e) {}
        })();
        """
    }

    static func prepareStandaloneHTML(_ html: String, isDark: Bool) -> AntigravityPreparedHTML {
        let isGenUI = usesAntigravityTheme(html)
        let styleTag = "<style id=\"\(styleElementID)\">\n\(themeCSSBody(isDark: isDark, mode: .standalone, includeContainerTheme: isGenUI))\n</style>"
        let hasViewport = html.range(
            of: #"<meta\s+[^>]*?name\s*=\s*["']viewport["'][^>]*>"#,
            options: [.regularExpression, .caseInsensitive]
        ) != nil
        let viewportTag = hasViewport ? "" : "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0, viewport-fit=cover\">"

        var modified = html
        if isGenUI {
            modified = applyHtmlRootThemeClass(modified, isDark: isDark)
        }
        let injection = [viewportTag, styleTag].filter { !$0.isEmpty }.joined(separator: "\n")
        modified = injectIntoHead(modified, injection: injection, rootThemeClass: isGenUI ? (isDark ? "dark" : "light") : nil)
        return AntigravityPreparedHTML(html: modified, isGenerativeUI: isGenUI)
    }

    /// Safely adds `dark` or `light` ONLY to the opening `<html ...>` tag without touching descendant `class="..."` attributes.
    static func applyHtmlRootThemeClass(_ html: String, isDark: Bool) -> String {
        let darkClass = isDark ? "dark" : "light"
        let oppositeClass = isDark ? "light" : "dark"
        guard let htmlTagRegex = try? NSRegularExpression(pattern: #"<html\b([^>]*)>"#, options: [.caseInsensitive]) else {
            return html
        }
        let fullRange = NSRange(html.startIndex..<html.endIndex, in: html)
        guard let match = htmlTagRegex.firstMatch(in: html, options: [], range: fullRange),
              let tagRange = Range(match.range, in: html),
              let attrsRange = Range(match.range(at: 1), in: html) else {
            return html
        }
        let attrs = String(html[attrsRange])
        let updatedTag: String
        if let classRegex = try? NSRegularExpression(pattern: #"(\bclass\s*=\s*)(["'])([^"']*)(\2)"#, options: [.caseInsensitive]),
           let classMatch = classRegex.firstMatch(in: attrs, options: [], range: NSRange(attrs.startIndex..<attrs.endIndex, in: attrs)),
           let fullClassRange = Range(classMatch.range, in: attrs),
           let prefixRange = Range(classMatch.range(at: 1), in: attrs),
           let quoteRange = Range(classMatch.range(at: 2), in: attrs),
           let valueRange = Range(classMatch.range(at: 3), in: attrs) {
            let tokens = attrs[valueRange]
                .split(whereSeparator: { $0.isWhitespace })
                .map(String.init)
                .filter { $0 != oppositeClass && $0 != darkClass }
            let merged = ([darkClass] + tokens).joined(separator: " ")
            let quote = String(attrs[quoteRange])
            var newAttrs = attrs
            newAttrs.replaceSubrange(fullClassRange, with: "\(attrs[prefixRange])\(quote)\(merged)\(quote)")
            updatedTag = "<html\(newAttrs)>"
        } else {
            updatedTag = "<html class=\"\(darkClass)\"\(attrs)>"
        }
        var result = html
        result.replaceSubrange(tagRange, with: updatedTag)
        return result
    }

    /// Inserts `injection` at the beginning of `<head>` (right after `<meta charset=...>` if immediately present,
    /// preserving HTML5 1024-byte charset prescan while staying before page `<style>`/`<script>` tags).
    static func injectIntoHead(_ html: String, injection: String, rootThemeClass: String?) -> String {
        let block = "\n\(injection)\n"
        if let headRegex = try? NSRegularExpression(pattern: #"<head\b[^>]*>(?:\s*<meta\s+[^>]*charset[^>]*>)?"#, options: [.caseInsensitive]),
           let match = headRegex.firstMatch(in: html, options: [], range: NSRange(html.startIndex..<html.endIndex, in: html)),
           let range = Range(match.range, in: html) {
            var result = html
            result.insert(contentsOf: block, at: range.upperBound)
            return result
        }
        if let htmlRegex = try? NSRegularExpression(pattern: #"<html\b[^>]*>"#, options: [.caseInsensitive]),
           let match = htmlRegex.firstMatch(in: html, options: [], range: NSRange(html.startIndex..<html.endIndex, in: html)),
           let range = Range(match.range, in: html) {
            var result = html
            result.insert(contentsOf: "<head><meta charset=\"UTF-8\">\(block)</head>", at: range.upperBound)
            return result
        }
        let classAttr = rootThemeClass.map { " class=\"\($0)\"" } ?? ""
        return "<!DOCTYPE html><html\(classAttr)><head><meta charset=\"UTF-8\">\(block)</head><body>\(html)</body></html>"
    }
}

// MARK: - HTML Enhancement Builder

private enum AgentEmbedHTMLBuilder {
    static func build(html: String, isDark: Bool) -> String {
        let themeCSS = "<style id=\"\(AntigravityHTMLThemeHelper.styleElementID)\">\n\(AntigravityHTMLThemeHelper.themeCSSBody(isDark: isDark, mode: .inlineEmbed, includeContainerTheme: true))\n</style>"
        
        let viewportMeta = """
        <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0, user-scalable=no, viewport-fit=cover">
        """
        
        let reporterScript = """
        <script id="antigravity-embed-injected-reporter">
          (function() {
            // Bridge interactive MCP App postMessage events back to native handler
            try {
              window.__origParentPostMessage = window.parent && window.parent.postMessage ? window.parent.postMessage.bind(window.parent) : null;
              window.parent = window.parent || {};
              window.parent.postMessage = function(data) {
                try {
                  if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.agentEmbedBridge) {
                    window.webkit.messageHandlers.agentEmbedBridge.postMessage(data);
                  }
                } catch(e) {}
              };
            } catch(e) {}
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
        modified = AntigravityHTMLThemeHelper.applyHtmlRootThemeClass(modified, isDark: isDark)
        let injections = viewportMeta + "\n" + themeCSS + "\n" + reporterScript
        return AntigravityHTMLThemeHelper.injectIntoHead(modified, injection: injections, rootThemeClass: darkClass)
    }
}
