import SwiftUI
import WebKit

public struct MermaidDiagramView: View {
    public let code: String
    
    public enum ViewMode: String, CaseIterable, Identifiable {
        case diagram = "图表"
        case code = "代码"
        public var id: String { rawValue }
    }
    
    @State private var viewMode: ViewMode = .diagram
    @State private var diagramHeight: CGFloat = 200
    @State private var renderError: String? = nil
    @State private var isFullscreen: Bool = false
    @State private var isCopied: Bool = false
    @Environment(\.colorScheme) private var colorScheme
    
    public init(code: String) {
        self.code = code
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header Bar
            HStack(spacing: 8) {
                HStack(spacing: 5) {
                    Image(systemName: "point.topleft.down.to.point.bottomright.curvepath")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.secondary)
                    Text("MERMAID")
                        .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                        .foregroundColor(.secondary)
                }
                
                Spacer()
                
                // Mode Switcher (Diagram / Code)
                HStack(spacing: 2) {
                    ForEach(ViewMode.allCases) { mode in
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.16)) {
                                viewMode = mode
                            }
                        }) {
                            Text(mode.rawValue)
                                .font(.system(size: 11, weight: viewMode == mode ? .semibold : .regular))
                                .padding(.horizontal, 7)
                                .padding(.vertical, 3)
                                .background(viewMode == mode ? Color(uiColor: .systemBackground) : Color.clear)
                                .foregroundColor(viewMode == mode ? .primary : .secondary)
                                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                                .shadow(color: viewMode == mode ? Color.black.opacity(0.08) : Color.clear, radius: 1, y: 0.5)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(2)
                .background(Color(uiColor: .quaternarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                
                // Fullscreen button (only in diagram mode)
                if viewMode == .diagram && renderError == nil {
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
                    .accessibilityLabel("全屏查看图表")
                }
                
                // Copy Button
                Button(action: {
                    UIPasteboard.general.string = code
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation {
                        isCopied = true
                    }
                    DispatchQueue.main.asyncAfter(deadline: .now() + 1.8) {
                        isCopied = false
                    }
                }) {
                    HStack(spacing: 3) {
                        Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                            .font(.system(size: 10))
                        Text(isCopied ? "已复制" : "复制")
                            .font(.system(size: 11))
                    }
                    .foregroundColor(isCopied ? .green : .secondary)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(Color(uiColor: .tertiarySystemBackground).opacity(0.85))
            
            Divider()
            
            // Content
            if viewMode == .diagram {
                if let error = renderError {
                    // Fallback on Mermaid syntax error
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 13))
                                .foregroundColor(.orange)
                            Text("Mermaid 语法解析失败")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(.primary)
                            Spacer()
                            Button("查看源码") {
                                viewMode = .code
                            }
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.blue)
                        }
                        
                        Text(error)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(.secondary)
                            .lineLimit(4)
                    }
                    .padding(12)
                    .background(Color(uiColor: .secondarySystemBackground).opacity(0.5))
                } else {
                    // Rendered Diagram
                    ZStack(alignment: .bottomTrailing) {
                        MermaidWebView(
                            code: code,
                            isDark: colorScheme == .dark,
                            onHeightChange: { height in
                                // Clamp between 120 and 520 for inline display
                                let target = max(120, min(height, 520))
                                withAnimation(.easeInOut(duration: 0.2)) {
                                    self.diagramHeight = target
                                }
                            },
                            onError: { err in
                                withAnimation {
                                    self.renderError = err
                                }
                            }
                        )
                        .frame(height: diagramHeight)
                        .frame(maxWidth: .infinity)
                        
                        // Subtle hint for zoom / fullscreen
                        HStack(spacing: 4) {
                            Image(systemName: "hand.draw")
                                .font(.system(size: 9.5))
                            Text("双指可缩放")
                                .font(.system(size: 10, weight: .medium))
                        }
                        .foregroundColor(.secondary.opacity(0.75))
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color(uiColor: .systemBackground).opacity(0.7))
                        .clipShape(Capsule())
                        .padding(8)
                    }
                }
            } else {
                // Code View
                ScrollView(.horizontal, showsIndicators: true) {
                    Text(code)
                        .font(.system(size: 12.5, design: .monospaced))
                        .textSelection(.enabled)
                        .padding(10)
                }
                .fixedSize(horizontal: false, vertical: true)
            }
        }
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(Color.secondary.opacity(0.2), lineWidth: 1)
        )
        .sheet(isPresented: $isFullscreen) {
            MermaidFullscreenViewer(code: code)
        }
    }
}

// MARK: - Mermaid WKWebView Wrapper

public struct MermaidWebView: UIViewRepresentable {
    public let code: String
    public let isDark: Bool
    public var onHeightChange: ((CGFloat) -> Void)? = nil
    public var onError: ((String) -> Void)? = nil
    
    public init(
        code: String,
        isDark: Bool,
        onHeightChange: ((CGFloat) -> Void)? = nil,
        onError: ((String) -> Void)? = nil
    ) {
        self.code = code
        self.isDark = isDark
        self.onHeightChange = onHeightChange
        self.onError = onError
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }
    
    public func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        let controller = WKUserContentController()
        
        controller.add(context.coordinator, name: "sizeNotifier")
        controller.add(context.coordinator, name: "errorNotifier")
        config.userContentController = controller
        
        let webView = WKWebView(frame: .zero, configuration: config)
        webView.isOpaque = false
        webView.backgroundColor = .clear
        webView.scrollView.backgroundColor = .clear
        webView.scrollView.showsVerticalScrollIndicator = false
        webView.scrollView.showsHorizontalScrollIndicator = true
        webView.scrollView.bounces = false
        
        context.coordinator.webView = webView
        loadContent(into: webView)
        return webView
    }
    
    public func updateUIView(_ uiView: WKWebView, context: Context) {
        context.coordinator.parent = self
        if context.coordinator.lastCode != code || context.coordinator.lastIsDark != isDark {
            context.coordinator.lastCode = code
            context.coordinator.lastIsDark = isDark
            loadContent(into: uiView)
        }
    }
    
    private func loadContent(into webView: WKWebView) {
        let html = MermaidHTMLTemplate.buildHTML(code: code, isDark: isDark, isFullscreen: false)
        
        let baseURL = Bundle.main.url(forResource: "mermaid.min", withExtension: "js")?.deletingLastPathComponent()
            ?? Bundle.main.resourceURL
            ?? Bundle.main.bundleURL
        
        webView.loadHTMLString(html, baseURL: baseURL)
    }
    
    public final class Coordinator: NSObject, WKScriptMessageHandler {
        var parent: MermaidWebView
        weak var webView: WKWebView?
        var lastCode: String = ""
        var lastIsDark: Bool = false
        
        init(_ parent: MermaidWebView) {
            self.parent = parent
            self.lastCode = parent.code
            self.lastIsDark = parent.isDark
        }
        
        public func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
            DispatchQueue.main.async {
                if message.name == "sizeNotifier", let body = message.body as? [String: Any] {
                    if let height = body["height"] as? CGFloat, height > 0 {
                        self.parent.onHeightChange?(height)
                    }
                } else if message.name == "errorNotifier", let body = message.body as? [String: Any] {
                    if let err = body["error"] as? String {
                        self.parent.onError?(err)
                    }
                }
            }
        }
    }
}

// MARK: - Mermaid Fullscreen Viewer Sheet

public struct MermaidFullscreenViewer: View {
    public let code: String
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme
    @State private var isCopied: Bool = false
    
    public init(code: String) {
        self.code = code
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                MermaidWebView(
                    code: code,
                    isDark: colorScheme == .dark,
                    onHeightChange: nil,
                    onError: nil
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(Color(uiColor: .systemBackground))
            }
            .navigationTitle("Mermaid 架构图")
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
                        UIPasteboard.general.string = code
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        withAnimation {
                            isCopied = true
                        }
                        DispatchQueue.main.asyncAfter(deadline: .now() + 1.8) {
                            isCopied = false
                        }
                    }) {
                        HStack(spacing: 4) {
                            Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                            Text(isCopied ? "已复制" : "复制源码")
                        }
                        .font(.system(size: 14))
                        .foregroundColor(isCopied ? .green : .blue)
                    }
                }
            }
        }
    }
}

// MARK: - HTML Template Generator

private enum MermaidHTMLTemplate {
    static func buildHTML(code: String, isDark: Bool, isFullscreen: Bool) -> String {
        let jsonCode: String = {
            if let data = try? JSONEncoder().encode(code), let str = String(data: data, encoding: .utf8) {
                return str
            }
            return "\"\""
        }()
        
        return """
        <!DOCTYPE html>
        <html>
        <head>
          <meta charset="utf-8">
          <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=5.0, user-scalable=yes">
          <style>
            :root {
              color-scheme: light dark;
            }
            * {
              box-sizing: border-box;
              -webkit-touch-callout: none;
            }
            html, body {
              margin: 0;
              padding: 0;
              width: 100%;
              min-height: 100%;
              background: transparent;
              font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
              display: flex;
              justify-content: center;
              align-items: center;
            }
            #wrapper {
              width: 100%;
              display: flex;
              justify-content: center;
              align-items: center;
              padding: 10px;
              overflow-x: auto;
              -webkit-overflow-scrolling: touch;
            }
            #container {
              display: inline-block;
              max-width: 100%;
              text-align: center;
            }
            svg {
              max-width: 100%;
              height: auto !important;
              display: block;
              margin: 0 auto;
            }
            body.fullscreen #wrapper {
              padding: 24px 16px;
            }
            body.fullscreen svg {
              max-width: 95vw;
            }
          </style>
          <script src="mermaid.min.js"></script>
        </head>
        <body class="\(isFullscreen ? "fullscreen" : "")">
          <div id="wrapper">
            <div id="container"></div>
          </div>
          <script>
            (function() {
              const isDark = \(isDark ? "true" : "false");
              const rawCode = \(jsonCode);
              
              if (typeof mermaid === 'undefined') {
                if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.errorNotifier) {
                  window.webkit.messageHandlers.errorNotifier.postMessage({ error: "未能加载 mermaid.min.js 引擎" });
                }
                return;
              }
              
              try {
                mermaid.initialize({
                  startOnLoad: false,
                  securityLevel: 'loose',
                  theme: isDark ? 'dark' : 'default',
                  themeVariables: isDark ? {
                    darkMode: true,
                    background: 'transparent',
                    primaryColor: '#1e293b',
                    primaryTextColor: '#f8fafc',
                    primaryBorderColor: '#475569',
                    lineColor: '#94a3b8',
                    secondaryColor: '#334155',
                    tertiaryColor: '#0f172a'
                  } : {
                    darkMode: false,
                    background: 'transparent',
                    primaryColor: '#f1f5f9',
                    primaryTextColor: '#0f172a',
                    primaryBorderColor: '#cbd5e1',
                    lineColor: '#64748b',
                    secondaryColor: '#f8fafc',
                    tertiaryColor: '#ffffff'
                  }
                });

                const id = 'mermaid_' + Math.random().toString(36).substring(2, 9);
                mermaid.render(id, rawCode).then(function(result) {
                  const container = document.getElementById('container');
                  container.innerHTML = result.svg;
                  
                  setTimeout(function() {
                    let h = 200, w = 300;
                    const svgEl = container.querySelector('svg');
                    if (svgEl) {
                      const rect = svgEl.getBoundingClientRect();
                      h = Math.ceil(rect.height || svgEl.clientHeight || 200) + 20;
                      w = Math.ceil(rect.width || svgEl.clientWidth || 300);
                    } else {
                      h = Math.ceil(document.body.scrollHeight || 200);
                    }
                    if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.sizeNotifier) {
                      window.webkit.messageHandlers.sizeNotifier.postMessage({ height: h, width: w });
                    }
                  }, 60);
                }).catch(function(err) {
                  const msg = (err && err.message) ? err.message : String(err);
                  if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.errorNotifier) {
                    window.webkit.messageHandlers.errorNotifier.postMessage({ error: msg });
                  }
                });
              } catch (e) {
                if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.errorNotifier) {
                  window.webkit.messageHandlers.errorNotifier.postMessage({ error: String(e) });
                }
              }
            })();
          </script>
        </body>
        </html>
        """
    }
}
