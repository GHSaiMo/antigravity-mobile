import SwiftUI
import CoreImage.CIFilterBuiltins

// MARK: - Context & Theme

/// 分享长图卡片头部需要的会话上下文。
struct ShareCardContext: Equatable {
    var sessionTitle: String
    /// 完整模型展示名，如 "Gemini 3.8 Flash" / "Opus 4.6 Thinking"。
    var modelName: String?
    var modelIsClaude: Bool = false
    /// 紧邻该气泡之前的用户提问（用于「包含提问」开关）。
    var previousQuestion: String?

    /// 把 `gemini-3.8-flash-high` / `claude-opus-4-6-thinking` 这类内部模型 ID 转成展示名。
    static func modelBadge(from raw: String) -> (name: String, isClaude: Bool) {
        let isClaude = raw.lowercased().contains("claude") || raw.contains("M26")
        let effortSuffixes: Set<String> = ["high", "medium", "low"]
        var tokens = raw.split(separator: "-").map(String.init)
        if tokens.first?.lowercased() == "claude" { tokens.removeFirst() }
        tokens.removeAll { effortSuffixes.contains($0.lowercased()) }
        var parts: [String] = []
        for token in tokens {
            let isDigits = !token.isEmpty && token.allSatisfy { $0.isNumber }
            if isDigits, let last = parts.last, last.allSatisfy({ $0.isNumber || $0 == "." }) {
                parts[parts.count - 1] = last + "." + token
            } else if token.first?.isLetter == true {
                parts.append(token.prefix(1).uppercased() + token.dropFirst())
            } else {
                parts.append(token)
            }
        }
        var name = parts.joined(separator: " ")
        if !isClaude, !name.lowercased().hasPrefix("gemini") { name = "Gemini " + name }
        return (name.isEmpty ? (isClaude ? "Claude" : "Gemini") : name, isClaude)
    }
}

enum ShareCardTheme: String {
    case dark
    case light

    var colorScheme: ColorScheme { self == .light ? .light : .dark }

    var accent: Color { self == .light ? Color.indigo : Color(red: 0.55, green: 0.50, blue: 1.0) }

    var background: LinearGradient {
        switch self {
        case .dark:
            return LinearGradient(
                colors: [Color(red: 0.06, green: 0.07, blue: 0.11), Color(red: 0.12, green: 0.09, blue: 0.22)],
                startPoint: .topLeading, endPoint: .bottomTrailing
            )
        case .light:
            return LinearGradient(
                colors: [Color(red: 0.99, green: 0.99, blue: 0.98), Color(red: 0.96, green: 0.96, blue: 0.95)],
                startPoint: .top, endPoint: .bottom
            )
        }
    }

    /// 问答对中「提问」块的底色。
    var questionBackground: Color {
        self == .light ? Color.black.opacity(0.05) : Color.white.opacity(0.09)
    }
}

// MARK: - Card View

struct MessageShareCardView: View {
    static let width: CGFloat = 390

    let content: String
    let isUserMessage: Bool
    let question: String?
    let images: [UIImage]
    let theme: ShareCardTheme
    let context: ShareCardContext
    let date: Date

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            Divider().overlay(Color.secondary.opacity(0.4))

            if let question, !question.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text("提问")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(theme.accent)
                    Text(question)
                        .font(.system(size: 14.5))
                        .foregroundColor(.primary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(theme.questionBackground)
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            }

            ForEach(Array(images.enumerated()), id: \.offset) { _, image in
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: .infinity, maxHeight: 360, alignment: .leading)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            }

            if isUserMessage {
                Text(content)
                    .font(.system(size: 15.5))
                    .foregroundColor(.primary)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                MarkdownContentView(content: content)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }

            Divider().overlay(Color.secondary.opacity(0.4))
            footer
        }
        .padding(20)
        .frame(width: Self.width, alignment: .leading)
        .background(theme.background)
        .environment(\.colorScheme, theme.colorScheme)
        .environment(\.isShareExport, true)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(context.sessionTitle.isEmpty ? "Multigravity 会话" : context.sessionTitle)
                .font(.system(size: 24, weight: .bold))
                .foregroundColor(.primary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 10) {
                Text(Self.dateFormatter.string(from: date))
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundColor(.secondary)
                if let model = context.modelName, !model.isEmpty {
                    let tint: Color = context.modelIsClaude ? .orange : .blue
                    Text(model)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(tint)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .background(tint.opacity(0.14), in: Capsule())
                        .overlay(Capsule().stroke(tint.opacity(0.35), lineWidth: 1))
                }
            }
        }
    }

    private var footer: some View {
        HStack(alignment: .center) {
            HStack(spacing: 8) {
                Image(systemName: "sparkles")
                    .font(.system(size: 15, weight: .bold))
                    .foregroundColor(.white)
                    .frame(width: 32, height: 32)
                    .background(theme.accent, in: RoundedRectangle(cornerRadius: 9, style: .continuous))
                Text("Multigravity")
                    .font(.system(size: 18, weight: .bold))
                    .foregroundColor(.primary)
            }
            Spacer()
            if let qr = Self.qrImage {
                Image(uiImage: qr)
                    .interpolation(.none)
                    .resizable()
                    .frame(width: 60, height: 60)
                    .padding(5)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
        }
    }

    static let landingURL = "https://mgy.jiuge.space"

    private static let qrImage: UIImage? = {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(landingURL.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let scaled = output.transformed(by: CGAffineTransform(scaleX: 10, y: 10))
        guard let cg = CIContext().createCGImage(scaled, from: scaled.extent) else { return nil }
        return UIImage(cgImage: cg)
    }()

    private static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd HH:mm"
        return f
    }()
}

// MARK: - Sheet

struct MessageShareCardSheet: View {
    let message: ChatMessage
    let context: ShareCardContext
    /// 需要一并放入卡片的图片（用户附件 / Agent 回复附带图），顺序即展示顺序。
    let extraImages: [IdentifiableImage]

    @Environment(\.displayScale) private var displayScale
    @Environment(\.colorScheme) private var systemScheme

    @State private var theme: ShareCardTheme = .dark
    @State private var includeQuestion = false
    @State private var renderedImage: UIImage?
    @State private var isRendering = false
    @State private var renderNote: String?
    @State private var hasPickedTheme = false

    private var isUserMessage: Bool { message.sender == .user }
    private var canIncludeQuestion: Bool {
        !isUserMessage && !(context.previousQuestion ?? "").isEmpty
    }

    private struct RenderKey: Equatable {
        let theme: ShareCardTheme
        let includeQuestion: Bool
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 8) {
                preview
                if let renderNote {
                    Text(renderNote)
                        .font(.footnote)
                        .foregroundColor(.secondary)
                }
            }
            .safeAreaInset(edge: .bottom) { bottomBar }
            .navigationTitle("分享长图")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        guard let image = renderedImage else { return }
                        Task { await ImageActions.saveWithFeedback(image) }
                    } label: {
                        Image(systemName: "square.and.arrow.down")
                    }
                    .accessibilityLabel("保存到相册")
                    .disabled(renderedImage == nil || isRendering)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        guard let image = renderedImage else { return }
                        ImageActions.share(image)
                    } label: {
                        Image(systemName: "square.and.arrow.up")
                    }
                    .accessibilityLabel("分享")
                    .disabled(renderedImage == nil || isRendering)
                }
            }
        }
        .onAppear {
            if !hasPickedTheme {
                theme = systemScheme == .dark ? .dark : .light
                hasPickedTheme = true
            }
        }
        .task(id: RenderKey(theme: theme, includeQuestion: includeQuestion)) {
            await render()
        }
    }

    private var preview: some View {
        ScrollView {
            Group {
                if let renderedImage {
                    Image(uiImage: renderedImage)
                        .resizable()
                        .scaledToFit()
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                        .shadow(color: .black.opacity(0.15), radius: 8, y: 2)
                        .opacity(isRendering ? 0.5 : 1)
                } else {
                    ProgressView()
                        .frame(maxWidth: .infinity, minHeight: 240)
                }
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 80)
        }
    }

    /// 底部左右两枚原生毛玻璃胶囊：左 = 浅/深色切换，右 = 是否包含上一条提问。
    private var bottomBar: some View {
        GlassEffectContainer(spacing: 12) {
            HStack {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    theme = theme == .dark ? .light : .dark
                } label: {
                    Label(theme == .dark ? "深色" : "浅色", systemImage: theme == .dark ? "moon.fill" : "sun.max.fill")
                        .font(.system(size: 15, weight: .semibold))
                        .padding(.horizontal, 16)
                        .frame(height: 44)
                }
                .buttonStyle(.plain)
                .glassEffect(.regular.interactive(), in: .capsule)

                Spacer()

                if canIncludeQuestion {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        includeQuestion.toggle()
                    } label: {
                        Label("包含上一条提问", systemImage: includeQuestion ? "checkmark.circle.fill" : "circle")
                            .font(.system(size: 15, weight: .semibold))
                            .padding(.horizontal, 16)
                            .frame(height: 44)
                    }
                    .buttonStyle(.plain)
                    .glassEffect(includeQuestion ? .regular.tint(.accentColor.opacity(0.35)).interactive() : .regular.interactive(), in: .capsule)
                }
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 8)
        }
    }

    // MARK: - Rendering

    @MainActor
    private func render() async {
        isRendering = true
        defer { isRendering = false }

        // 先把正文里的网络图片拉进共享缓存，避免离屏渲染出现空白占位。
        await Self.preloadMarkdownImages(in: message.content)
        var loadedImages: [UIImage] = []
        for item in extraImages {
            if let image = await ImageActions.loadImage(item) { loadedImages.append(image) }
        }
        if Task.isCancelled { return }

        let card = MessageShareCardView(
            content: isUserMessage ? AttachmentRules.parseBlock(message.content).body : message.content,
            isUserMessage: isUserMessage,
            question: includeQuestion ? context.previousQuestion : nil,
            images: loadedImages,
            theme: theme,
            context: context,
            date: Date()
        )

        let renderer = ImageRenderer(content: card)
        renderer.isOpaque = true
        var scale = max(displayScale, 2)
        renderer.scale = scale
        var image = renderer.uiImage

        // 超长图防 OOM：像素高度超过上限时降低 scale（最低 1x）。
        let maxPixelHeight: CGFloat = 8192
        if let current = image, current.size.height * scale > maxPixelHeight {
            scale = max(1, maxPixelHeight / current.size.height)
            renderer.scale = scale
            image = renderer.uiImage
            if let reduced = image, reduced.size.height * scale > maxPixelHeight {
                renderNote = "内容很长，已降低清晰度导出"
            } else {
                renderNote = "内容较长，已适当降低清晰度"
            }
        } else {
            renderNote = nil
        }

        if let image { renderedImage = image }
    }

    private static func preloadMarkdownImages(in content: String) async {
        let blocks = MarkdownBlockCache.shared.blocks(for: content)
        var urls: [URL] = []
        for block in blocks {
            if case .image(_, _, let raw) = block, let url = resolveImageURL(raw) {
                urls.append(url)
            }
        }
        await withTaskGroup(of: Void.self) { group in
            for url in urls {
                group.addTask { _ = await ImageActions.loadImage(url: url) }
            }
        }
    }

    private static func resolveImageURL(_ raw: String) -> URL? {
        if let base = AppSettings.shared.gatewayURL {
            let resolved = APIClient.shared.resolveMediaURL(raw, baseURL: base)
            if let url = URL(string: resolved) { return url }
        }
        return URL(string: raw)
    }
}
