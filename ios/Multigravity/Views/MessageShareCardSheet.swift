import SwiftUI

// MARK: - Context & Theme

/// 分享长图卡片头部需要的会话上下文。
struct ShareCardContext: Equatable {
    var sessionTitle: String
    var modelName: String?
    /// 紧邻该气泡之前的用户提问（用于「包含提问」开关）。
    var previousQuestion: String?
}

enum ShareCardTheme: String, CaseIterable, Identifiable {
    case darkGlass
    case cleanLight
    case titanium

    var id: String { rawValue }

    var title: String {
        switch self {
        case .darkGlass: return "暗黑极客"
        case .cleanLight: return "极简纸白"
        case .titanium: return "渐变钛金"
        }
    }

    var colorScheme: ColorScheme {
        self == .cleanLight ? .light : .dark
    }

    var accent: Color {
        switch self {
        case .darkGlass: return Color(red: 0.55, green: 0.50, blue: 1.0)
        case .cleanLight: return Color.indigo
        case .titanium: return Color(red: 0.80, green: 0.84, blue: 0.92)
        }
    }

    var background: LinearGradient {
        switch self {
        case .darkGlass:
            return LinearGradient(
                colors: [Color(red: 0.06, green: 0.07, blue: 0.11), Color(red: 0.12, green: 0.09, blue: 0.22)],
                startPoint: .topLeading, endPoint: .bottomTrailing
            )
        case .cleanLight:
            return LinearGradient(
                colors: [Color(red: 0.99, green: 0.99, blue: 0.98), Color(red: 0.96, green: 0.96, blue: 0.95)],
                startPoint: .top, endPoint: .bottom
            )
        case .titanium:
            return LinearGradient(
                colors: [Color(red: 0.16, green: 0.18, blue: 0.22), Color(red: 0.36, green: 0.39, blue: 0.45)],
                startPoint: .topLeading, endPoint: .bottomTrailing
            )
        }
    }

    /// 问答对中「提问」块的底色。
    var questionBackground: Color {
        switch self {
        case .cleanLight: return Color.black.opacity(0.05)
        default: return Color.white.opacity(0.09)
        }
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
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "sparkles")
                    .font(.system(size: 13, weight: .bold))
                    .foregroundColor(.white)
                    .frame(width: 26, height: 26)
                    .background(theme.accent, in: RoundedRectangle(cornerRadius: 7, style: .continuous))
                Text("Multigravity")
                    .font(.system(size: 16, weight: .bold))
                    .foregroundColor(.primary)
                Spacer()
                Text(Self.dateFormatter.string(from: date))
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(.secondary)
            }
            HStack(spacing: 8) {
                if !context.sessionTitle.isEmpty {
                    Text(context.sessionTitle)
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                if let model = context.modelName, !model.isEmpty {
                    Text(model)
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(theme.accent)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 2)
                        .background(theme.accent.opacity(0.16), in: Capsule())
                }
            }
        }
    }

    private var footer: some View {
        HStack {
            Spacer()
            Text("Generated by Multigravity · Powering AI Workflows")
                .font(.system(size: 10.5))
                .foregroundColor(.secondary)
            Spacer()
        }
    }

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

    @Environment(\.dismiss) private var dismiss
    @Environment(\.displayScale) private var displayScale

    @State private var theme: ShareCardTheme = .darkGlass
    @State private var includeQuestion = false
    @State private var renderedImage: UIImage?
    @State private var isRendering = false
    @State private var renderNote: String?
    @State private var hasPickedTheme = false

    @Environment(\.colorScheme) private var systemScheme

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
            VStack(spacing: 14) {
                preview

                if let renderNote {
                    Text(renderNote)
                        .font(.footnote)
                        .foregroundColor(.secondary)
                }

                Picker("主题", selection: $theme) {
                    ForEach(ShareCardTheme.allCases) { item in
                        Text(item.title).tag(item)
                    }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)

                if canIncludeQuestion {
                    Toggle("包含上一条提问", isOn: $includeQuestion)
                        .padding(.horizontal, 20)
                }

                actionBar
            }
            .padding(.bottom, 12)
            .navigationTitle("分享长图")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("关闭") { dismiss() }
                }
            }
        }
        .onAppear {
            if !hasPickedTheme {
                theme = systemScheme == .dark ? .darkGlass : .cleanLight
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
        }
    }

    private var actionBar: some View {
        HStack(spacing: 10) {
            actionButton("保存相册", systemImage: "square.and.arrow.down") {
                guard let image = renderedImage else { return }
                Task { await ImageActions.saveWithFeedback(image) }
            }
            actionButton("拷贝", systemImage: "doc.on.doc") {
                guard let image = renderedImage else { return }
                ImageActions.copy(image)
            }
            actionButton("系统分享", systemImage: "square.and.arrow.up", prominent: true) {
                guard let image = renderedImage else { return }
                ImageActions.share(image)
            }
        }
        .padding(.horizontal, 16)
        .disabled(renderedImage == nil || isRendering)
    }

    @ViewBuilder
    private func actionButton(_ title: String, systemImage: String, prominent: Bool = false, action: @escaping () -> Void) -> some View {
        let label = Label(title, systemImage: systemImage)
            .font(.system(size: 14, weight: .semibold))
            .frame(maxWidth: .infinity)
            .padding(.vertical, 6)
        if prominent {
            Button(action: action) { label }.buttonStyle(.borderedProminent)
        } else {
            Button(action: action) { label }.buttonStyle(.bordered)
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
