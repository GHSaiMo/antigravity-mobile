import SwiftUI
import CoreImage.CIFilterBuiltins

// MARK: - Context & Theme

/// 分享长图卡片头部需要的会话上下文。
struct ShareCardContext: Equatable {
    var sessionTitle: String
    /// 完整模型展示名，如 "Gemini 3.8 Flash (High)" / "Claude Opus 4.6 (Thinking)"。
    var modelName: String?
    var modelIsClaude: Bool = false
    /// 会话发起时间（ISO-8601）。分享卡片显示它，而不是分享当下的时间。
    var startedAtISO: String?
    /// 紧邻该气泡之前的用户提问消息（含图片/附件，用于「包含提问」开关）。
    var previousMessage: ChatMessage?

    /// 长图里的模型名要和用户当时实际选用的模型一致：
    /// - Agent 回复：用生成这条回复的模型；
    /// - 用户提问：用紧随其后那条回复的模型；
    /// - 都没有（旧数据、未知枚举）：回退到会话当前模型。
    static func resolveModel(
        for target: ChatMessage,
        in messages: [ChatMessage],
        activeModel: String,
        activeModelName: String?
    ) -> (name: String, isClaude: Bool) {
        func nameOf(_ m: ChatMessage) -> String? {
            if let n = m.modelName, !n.isEmpty { return n }
            if let id = m.model, !id.isEmpty { return modelBadge(from: id).name }
            return nil
        }
        var source: ChatMessage?
        if target.isUser {
            if let idx = messages.firstIndex(where: { $0.id == target.id }) {
                for next in messages[(idx + 1)...] {
                    if next.isUser { break }
                    if nameOf(next) != nil { source = next; break }
                }
            }
        } else if nameOf(target) != nil {
            source = target
        }
        if let source, let name = nameOf(source) {
            return (name, ModelDefaultsLogic.isClaude(source.model ?? activeModel))
        }
        let fallback = (activeModelName?.isEmpty == false) ? activeModelName! : modelBadge(from: activeModel).name
        return (fallback, ModelDefaultsLogic.isClaude(activeModel))
    }

    /// ISO-8601 → 本地时区的 Date；解析失败返回 nil（界面直接不显示，不拿分享时间冒充）。
    static func parseStartedAt(_ iso: String?) -> Date? {
        guard let raw = iso?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else { return nil }
        let withFraction = ISO8601DateFormatter()
        withFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = withFraction.date(from: raw) { return d }
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        return plain.date(from: raw)
    }

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

// MARK: - Question content

/// 「包含上一条提问」时渲染所需的内容：与会话内用户气泡同样的 图片 / 文件卡片 / 文字 排列。
struct ShareCardQuestion {
    let text: String
    let images: [UIImage]
    let files: [AttachmentRules.ParsedFile]
}

extension ChatMessage {
    /// 用户消息里的图片附件（优先 imageUrls，缩略图来自 imageDataList）。
    var attachmentImages: [IdentifiableImage] {
        if !imageUrls.isEmpty {
            return imageUrls.enumerated().compactMap { idx, urlString in
                guard let url = URL(string: urlString) else { return nil }
                let thumb = idx < imageDataList.count ? UIImage(data: imageDataList[idx]) : nil
                return IdentifiableImage(image: thumb, url: url)
            }
        }
        return imageDataList.compactMap { data in
            guard let image = UIImage(data: data) else { return nil }
            return IdentifiableImage(image: image)
        }
    }

    /// 是否有可展示的提问内容（文字、图片或文件）。
    var hasQuestionContent: Bool {
        let parsed = AttachmentRules.parseBlock(content)
        return !parsed.body.isEmpty || !parsed.files.isEmpty || !attachmentImages.isEmpty
    }
}

// MARK: - Card View

struct MessageShareCardView: View {
    static let width: CGFloat = 390

    let content: String
    let isUserMessage: Bool
    let question: ShareCardQuestion?
    let images: [UIImage]
    let theme: ShareCardTheme
    let context: ShareCardContext
    let date: Date?

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            Divider().overlay(Color.secondary.opacity(0.4))

            if let question {
                questionBubble(question)
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
        .padding(.horizontal, 20)
        .padding(.bottom, 20)
        .padding(.top, 56) // 预留顶部：避开刘海/状态栏遮挡与圆角裁切
        .frame(width: Self.width, alignment: .leading)
        .background(theme.background)
        .environment(\.colorScheme, theme.colorScheme)
        .environment(\.isShareExport, true)
    }

    /// 复刻电脑端的提问样式：图片 / 文件 / 文字整体放进一个带底色的圆角矩形，与下面的回答区分开。
    @ViewBuilder
    private func questionBubble(_ question: ShareCardQuestion) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            if question.images.count == 1, let image = question.images.first {
                Image(uiImage: image)
                    .resizable()
                    .frame(width: Self.fittedSize(of: image, maxWidth: 240, maxHeight: 220).width,
                           height: Self.fittedSize(of: image, maxWidth: 240, maxHeight: 220).height)
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(Color.primary.opacity(0.12), lineWidth: 0.8))
            } else if question.images.count > 1 {
                HStack(spacing: 6) {
                    ForEach(Array(question.images.enumerated()), id: \.offset) { _, image in
                        Image(uiImage: image)
                            .resizable()
                            .scaledToFill()
                            .frame(width: 72, height: 72)
                            .clipped()
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(Color.primary.opacity(0.12), lineWidth: 0.8))
                    }
                }
            }
            ForEach(question.files) { file in
                MessageFileCardView(file: file, onTap: {})
            }
            if !question.text.isEmpty {
                Text(question.text)
                    .font(.system(size: 15.5))
                    .foregroundColor(.primary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(theme.questionBackground, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(Color.primary.opacity(0.10), lineWidth: 0.8))
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(context.sessionTitle.isEmpty ? "Multigravity 会话" : context.sessionTitle)
                .font(.system(size: 24, weight: .bold))
                .foregroundColor(.primary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 10) {
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
                if let date {
                    Text(Self.dateFormatter.string(from: date))
                        .font(.system(size: 13, design: .monospaced))
                        .foregroundColor(.secondary)
                }
            }
        }
    }

    private var footer: some View {
        HStack(alignment: .center) {
            HStack(spacing: 8) {
                Image("AppLogo")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 36, height: 36)
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(Color.black.opacity(0.08), lineWidth: 0.5))
                Text("Multigravity")
                    .font(.system(size: 20, weight: .bold))
                    .foregroundColor(.primary)
            }
            Spacer()
            if let qr = Self.qrImage {
                Image(uiImage: qr)
                    .interpolation(.none)
                    .resizable()
                    .frame(width: 48, height: 48)
                    .padding(4)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
        }
    }

    /// 按图片真实宽高比算出放进 maxWidth×maxHeight 后的实际尺寸，让边框/圆角紧贴图片，不留空白。
    private static func fittedSize(of image: UIImage, maxWidth: CGFloat, maxHeight: CGFloat) -> CGSize {
        let w = max(image.size.width, 1), h = max(image.size.height, 1)
        let scale = min(maxWidth / w, maxHeight / h, 1)
        return CGSize(width: w * scale, height: h * scale)
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
    @State private var includeQuestion = true
    @State private var renderedImage: UIImage?
    @State private var isRendering = false
    @State private var renderNote: String?
    @State private var hasPickedTheme = false

    private var isUserMessage: Bool { message.sender == .user }
    private var canIncludeQuestion: Bool {
        !isUserMessage && (context.previousMessage?.hasQuestionContent ?? false)
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
                        Label("包含提问", systemImage: includeQuestion ? "checkmark.circle.fill" : "circle")
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

        var questionContent: ShareCardQuestion?
        if includeQuestion, canIncludeQuestion, let previous = context.previousMessage {
            let parsed = AttachmentRules.parseBlock(previous.content)
            var questionImages: [UIImage] = []
            for item in previous.attachmentImages {
                if let image = await ImageActions.loadImage(item) { questionImages.append(image) }
            }
            questionContent = ShareCardQuestion(text: parsed.body, images: questionImages, files: parsed.files)
        }
        if Task.isCancelled { return }

        let card = MessageShareCardView(
            content: isUserMessage ? AttachmentRules.parseBlock(message.content).body : message.content,
            isUserMessage: isUserMessage,
            question: questionContent,
            images: loadedImages,
            theme: theme,
            context: context,
            date: ShareCardContext.parseStartedAt(context.startedAtISO)
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
