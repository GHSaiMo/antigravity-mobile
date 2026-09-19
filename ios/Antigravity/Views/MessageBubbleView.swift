import SwiftUI
import Photos

// Shared Agent Avatar
public struct AgentAvatarView: View {
    public init() {}
    
    public var body: some View {
        Circle()
            .fill(Color.blue.opacity(0.12))
            .frame(width: 28, height: 28)
            .overlay(
                Image(systemName: "sparkles")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.blue)
            )
    }
}

// Shared Agent Activity Pulse Dots
public struct AgentActivityDotsView: View {
    @State private var dotPhase: Int = 0
    @State private var timer: Timer?
    
    public init() {}
    
    public var body: some View {
        HStack(spacing: 3) {
            ForEach(0..<3) { idx in
                Circle()
                    .fill(Color.blue.opacity(dotPhase == idx ? 0.9 : 0.25))
                    .frame(width: 4, height: 4)
                    .scaleEffect(dotPhase == idx ? 1.3 : 0.8)
                    .animation(.easeInOut(duration: 0.35), value: dotPhase)
            }
        }
        .onAppear {
            timer = Timer.scheduledTimer(withTimeInterval: 0.35, repeats: true) { _ in
                dotPhase = (dotPhase + 1) % 3
            }
        }
        .onDisappear {
            timer?.invalidate()
            timer = nil
        }
    }
}

public struct MessageBubbleView: View {
    @Environment(\.openURL) private var openURL
    public let message: ChatMessage
    public let isActiveToolBatch: Bool
    public let onUndo: ((ChatMessage) -> Void)?
    @State private var isThinkingExpanded: Bool = false
    
    @State private var previewGallery: ImageGalleryData? = nil
    
    public init(message: ChatMessage, isActiveToolBatch: Bool = false, onUndo: ((ChatMessage) -> Void)? = nil) {
        self.message = message
        self.isActiveToolBatch = isActiveToolBatch
        self.onUndo = onUndo
    }
    
    public var body: some View {
        Group {
            switch message.sender {
            case .user:
                HStack(alignment: .bottom, spacing: 8) {
                    Spacer(minLength: 40)
                    userBubble
                }
            case .agent:
                HStack(alignment: .bottom, spacing: 8) {
                    agentBubble
                    Spacer(minLength: 20)
                }
            case .toolBatch(let count, let tools):
                if isActiveToolBatch {
                    activeToolBatchCard(count: count, tools: tools)
                } else {
                    ToolBatchAccordionView(count: count, tools: tools)
                }
            case .error:
                errorCard
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 4)
        .fullScreenCover(item: $previewGallery) { gallery in
            ImageViewerSheet(gallery: gallery)
                .presentationBackground(.clear)
        }
    }
    
    private var userAttachmentItems: [IdentifiableImage] {
        var result: [IdentifiableImage] = []
        for data in message.imageDataList {
            if let uiImg = UIImage(data: data) {
                result.append(IdentifiableImage(image: uiImg))
            }
        }
        for urlString in message.imageUrls {
            if let url = URL(string: urlString) {
                result.append(IdentifiableImage(url: url))
            }
        }
        return result
    }
    
    private var userBubble: some View {
        VStack(alignment: .trailing, spacing: 6) {
            if userAttachmentItems.count == 1, let singleItem = userAttachmentItems.first {
                if let uiImg = singleItem.image {
                    userImageBubble(for: uiImg, gallery: userAttachmentItems, index: 0)
                } else if let url = singleItem.url {
                    userAsyncImageBubble(for: url, gallery: userAttachmentItems, index: 0)
                }
            } else if userAttachmentItems.count > 1 {
                userMultiImageRow
            }
            
            if !message.content.isEmpty {
                Text(message.content)
                    .font(.system(size: 15.5))
                    .foregroundColor(.white)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(Color.indigo)
                    .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
            }
        }
        .contentShape(.contextMenuPreview, RoundedRectangle(cornerRadius: 18, style: .continuous))
        .contextMenu {
            Button(role: .destructive) {
                onUndo?(message)
            } label: {
                Label("撤回", systemImage: "arrow.uturn.backward")
            }
        }
    }
    
    private var userMultiImageRow: some View {
        ViewThatFits(in: .horizontal) {
            // Priority 1: Fits horizontally on screen -> Natural intrinsic width HStack, flush right-aligned with bubble
            HStack(spacing: 6) {
                ForEach(Array(userAttachmentItems.enumerated()), id: \.element.id) { index, item in
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        previewGallery = ImageGalleryData(items: userAttachmentItems, initialIndex: index)
                    }) {
                        thumbnailView(for: item)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 2)
            .padding(.bottom, 2)
            
            // Priority 2: Overflows screen width -> Horizontal ScrollView anchored to trailing edge
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(Array(userAttachmentItems.enumerated()), id: \.element.id) { index, item in
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            previewGallery = ImageGalleryData(items: userAttachmentItems, initialIndex: index)
                        }) {
                            thumbnailView(for: item)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, 2)
                .padding(.bottom, 2)
            }
            .defaultScrollAnchor(.trailing)
        }
    }
    
    private func thumbnailView(for item: IdentifiableImage) -> some View {
        Group {
            if let uiImg = item.image {
                Image(uiImage: uiImg)
                    .resizable()
                    .scaledToFill()
                    .frame(width: 72, height: 72)
                    .clipped()
            } else if let url = item.url {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .empty:
                        ProgressView()
                            .frame(width: 72, height: 72)
                    case .success(let image):
                        image
                            .resizable()
                            .scaledToFill()
                            .frame(width: 72, height: 72)
                            .clipped()
                    case .failure:
                        Image(systemName: "photo")
                            .font(.system(size: 24))
                            .foregroundColor(.secondary)
                            .frame(width: 72, height: 72)
                    @unknown default:
                        EmptyView()
                    }
                }
            }
        }
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
        )
        .shadow(color: Color.black.opacity(0.06), radius: 2, x: 0, y: 1)
    }
    
    private func userImageBubble(for uiImg: UIImage, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
        let maxDisplayWidth: CGFloat = 240
        let maxDisplayHeight: CGFloat = 220
        
        let imgWidth = uiImg.size.width
        let imgHeight = uiImg.size.height
        
        let fittedSize: CGSize = {
            guard imgWidth > 0, imgHeight > 0 else {
                return CGSize(width: maxDisplayWidth, height: maxDisplayHeight)
            }
            let widthRatio = maxDisplayWidth / imgWidth
            let heightRatio = maxDisplayHeight / imgHeight
            let scale = min(widthRatio, heightRatio)
            return CGSize(
                width: max(40, imgWidth * scale),
                height: max(30, imgHeight * scale)
            )
        }()
        
        return Button(action: {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: uiImg)] : gallery
            previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
        }) {
            Image(uiImage: uiImg)
                .resizable()
                .scaledToFit()
                .frame(width: fittedSize.width, height: fittedSize.height)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                        .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                )
                .shadow(color: Color.black.opacity(0.06), radius: 3, x: 0, y: 1.5)
        }
        .buttonStyle(.plain)
    }
    
    private func userAsyncImageBubble(for url: URL, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
        AsyncImage(url: url) { phase in
            switch phase {
            case .empty:
                ProgressView()
                    .frame(width: 140, height: 140)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            case .success(let image):
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    let effectiveItems = gallery.isEmpty ? [IdentifiableImage(url: url)] : gallery
                    previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
                }) {
                    image
                        .resizable()
                        .scaledToFit()
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                        )
                        .shadow(color: Color.black.opacity(0.06), radius: 3, x: 0, y: 1.5)
                        .frame(maxWidth: 240, maxHeight: 220, alignment: .trailing)
                }
                .buttonStyle(.plain)
            case .failure:
                HStack(spacing: 6) {
                    Image(systemName: "photo")
                    Text("图片加载失败")
                }
                .font(.footnote)
                .foregroundColor(.secondary)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            @unknown default:
                EmptyView()
            }
        }
    }
    
    private var fallbackAgentGalleryItems: [IdentifiableImage] {
        fallbackAgentImageURLs.compactMap { URL(string: $0) }.map { IdentifiableImage(url: $0) }
    }
    
    private var agentMultiImageRow: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(Array(fallbackAgentGalleryItems.enumerated()), id: \.element.id) { index, item in
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        previewGallery = ImageGalleryData(items: fallbackAgentGalleryItems, initialIndex: index)
                    }) {
                        thumbnailView(for: item)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 2)
            .padding(.bottom, 2)
        }
    }
    
    private var agentBubble: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !message.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                MarkdownContentView(content: message.content, onImageTap: { url in
                    previewGallery = ImageGalleryData(url: url)
                })
            }
            
            if fallbackAgentImageURLs.count == 1, let url = URL(string: fallbackAgentImageURLs[0]) {
                agentAsyncImageBubble(for: url, gallery: fallbackAgentGalleryItems, index: 0)
            } else if fallbackAgentImageURLs.count > 1 {
                agentMultiImageRow
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
    }
    
    /// Image URLs attached to the agent message that are not already rendered
    /// from Markdown image / MEDIA syntax in `message.content`.
    private var fallbackAgentImageURLs: [String] {
        guard !message.imageUrls.isEmpty else { return [] }
        let content = message.content
        var seen = Set<String>()
        return message.imageUrls.filter { resolved in
            guard seen.insert(resolved).inserted else { return false }
            if content.contains(resolved) { return false }
            guard let comps = URLComponents(string: resolved),
                  let uri = comps.queryItems?.first(where: { $0.name == "uri" })?.value else {
                return true
            }
            if content.contains(uri) { return false }
            if let decoded = uri.removingPercentEncoding, content.contains(decoded) {
                return false
            }
            return true
        }
    }
    
    private func agentAsyncImageBubble(for url: URL, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
        AsyncImage(url: url) { phase in
            switch phase {
            case .empty:
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 120, maxHeight: 160, alignment: .leading)
            case .success(let image):
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    let effectiveItems = gallery.isEmpty ? [IdentifiableImage(url: url)] : gallery
                    previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
                }) {
                    image
                        .resizable()
                        .scaledToFit()
                        .frame(maxWidth: .infinity, maxHeight: 280, alignment: .leading)
                }
                .buttonStyle(.plain)
            case .failure:
                HStack(spacing: 6) {
                    Image(systemName: "photo")
                    Text("图片加载失败")
                }
                .font(.footnote)
                .foregroundColor(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            @unknown default:
                EmptyView()
            }
        }
    }
    
    private struct ToolContentHeightPreferenceKey: PreferenceKey {
        static var defaultValue: CGFloat = 0
        static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
            let next = nextValue()
            if next > 0 {
                value = next
            }
        }
    }
    
    private struct NoTapAnimationButtonStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
        }
    }
    
    public struct ToolBatchAccordionView: View {
        public let count: Int
        public let tools: [String]
        @State private var isExpanded: Bool = false
        @State private var contentHeight: CGFloat = 0
        
        public init(count: Int, tools: [String]) {
            self.count = count
            self.tools = tools
        }
        
        private var resolvedTools: [String] {
            if !tools.isEmpty {
                return tools
            }
            let actualCount = max(count, 1)
            return (0..<actualCount).map { _ in "tool_call" }
        }
        
        private var summaryText: String {
            let actualCount = max(count, tools.count)
            let uniqueNames = Array(NSOrderedSet(array: tools)).compactMap { $0 as? String }
            if !uniqueNames.isEmpty {
                let namesSummary = uniqueNames.joined(separator: ", ")
                return "已思考并执行 \(actualCount) 项操作 (\(namesSummary))"
            }
            return "已思考并执行 \(actualCount) 项操作"
        }
        
        public var body: some View {
            VStack(alignment: .leading, spacing: 0) {
                // Header Capsule: stays strictly static, never dims on tap, chevron rotates smoothly
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.32, dampingFraction: 0.95)) {
                        isExpanded.toggle()
                    }
                }) {
                    HStack(spacing: 8) {
                        Image(systemName: "bolt.fill")
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundColor(.orange)
                        
                        Text(summaryText)
                            .font(.system(size: 12.5, weight: .medium))
                            .foregroundColor(.primary.opacity(0.85))
                            .lineLimit(1)
                        
                        Spacer(minLength: 6)
                        
                        Image(systemName: "chevron.down")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(.secondary.opacity(0.7))
                            .rotationEffect(.degrees(isExpanded ? 180 : 0))
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(Capsule())
                    .overlay(
                        Capsule()
                            .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                    )
                }
                .buttonStyle(NoTapAnimationButtonStyle())
                .zIndex(2)
                
                // Expandable tool details: strictly below the header, unfolds downward on expand, folds upward on collapse
                VStack(alignment: .leading, spacing: 5) {
                    ForEach(Array(resolvedTools.enumerated()), id: \.offset) { idx, toolName in
                        HStack(spacing: 8) {
                            Image(systemName: "puzzlepiece.extension.fill")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundColor(Color(red: 0.3, green: 0.85, blue: 0.4))
                            
                            Text(toolName)
                                .font(.system(size: 12, weight: .semibold, design: .monospaced))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                            
                            Spacer()
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 7)
                        .background(Color(uiColor: .tertiarySystemFill).opacity(0.8))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 8, style: .continuous)
                                .stroke(Color.primary.opacity(0.05), lineWidth: 0.5)
                        )
                    }
                }
                .fixedSize(horizontal: false, vertical: true)
                .background(
                    GeometryReader { geo in
                        Color.clear.preference(
                            key: ToolContentHeightPreferenceKey.self,
                            value: geo.size.height
                        )
                    }
                )
                .padding(.top, isExpanded ? 6 : 0)
                .frame(
                    maxWidth: .infinity,
                    maxHeight: isExpanded ? (contentHeight > 0 ? contentHeight + 6 : nil) : 0,
                    alignment: .topLeading
                )
                .opacity(isExpanded ? 1 : 0)
                .clipped()
                .allowsHitTesting(isExpanded)
                .zIndex(1)
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
            .onPreferenceChange(ToolContentHeightPreferenceKey.self) { height in
                if height > 0 {
                    contentHeight = height
                }
            }
        }
    }
    
    private func activeToolBatchCard(count: Int, tools: [String]) -> some View {
        HStack(alignment: .top, spacing: 10) {
            AgentAvatarView()
            
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Text("Agent 正在思考与执行")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.primary)
                    
                    AgentActivityDotsView()
                }
                
                HStack(spacing: 6) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 10))
                        .foregroundColor(.orange)
                    
                    Text("已执行 \(count) 项操作")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.primary.opacity(0.85))
                    
                    if !tools.isEmpty {
                        Text("(\(tools.prefix(3).joined(separator: ", "))\(tools.count > 3 ? "..." : ""))")
                            .font(.system(size: 10.5, design: .monospaced))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 20)
        }
    }
    
    private var errorCard: some View {
        HStack(alignment: .top, spacing: 10) {
            Circle()
                .fill(Color.red.opacity(0.15))
                .frame(width: 28, height: 28)
                .overlay(
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.red)
                )
            
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Text("error")
                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                        .textCase(.uppercase)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.red.opacity(0.18))
                        .foregroundColor(.red)
                        .clipShape(Capsule())
                    
                    Text("执行遇到错误")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.red)
                }
                
                Text(message.content)
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundColor(.primary.opacity(0.9))
                    .lineSpacing(3)
                    .textSelection(.enabled)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color.red.opacity(0.08))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color.red.opacity(0.25), lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 20)
        }
    }
}

public struct IdentifiableImage: Identifiable, Hashable {
    public let id: UUID
    public let image: UIImage?
    public let url: URL?
    
    public init(id: UUID = UUID(), image: UIImage) {
        self.id = id
        self.image = image
        self.url = nil
    }
    
    public init(id: UUID = UUID(), url: URL) {
        self.id = id
        self.image = nil
        self.url = url
    }
    
    public func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
    
    public static func == (lhs: IdentifiableImage, rhs: IdentifiableImage) -> Bool {
        lhs.id == rhs.id
    }
}

public struct ImageGalleryData: Identifiable, Hashable {
    public let id: UUID
    public let items: [IdentifiableImage]
    public let initialIndex: Int
    
    public init(id: UUID = UUID(), items: [IdentifiableImage], initialIndex: Int = 0) {
        self.id = id
        self.items = items
        self.initialIndex = max(0, min(initialIndex, max(0, items.count - 1)))
    }
    
    public init(image: UIImage) {
        self.init(items: [IdentifiableImage(image: image)], initialIndex: 0)
    }
    
    public init(url: URL) {
        self.init(items: [IdentifiableImage(url: url)], initialIndex: 0)
    }
    
    public init(item: IdentifiableImage) {
        self.init(items: [item], initialIndex: 0)
    }
    
    public func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
    
    public static func == (lhs: ImageGalleryData, rhs: ImageGalleryData) -> Bool {
        lhs.id == rhs.id
    }
}

public struct ImageViewerSheet: UIViewControllerRepresentable {
    public let items: [IdentifiableImage]
    public let initialIndex: Int
    @Environment(\.dismiss) private var dismiss
    
    public var image: UIImage? { items.indices.contains(initialIndex) ? items[initialIndex].image : nil }
    public var url: URL? { items.indices.contains(initialIndex) ? items[initialIndex].url : nil }
    
    public init(items: [IdentifiableImage], initialIndex: Int = 0) {
        self.items = items
        self.initialIndex = max(0, min(initialIndex, max(0, items.count - 1)))
    }
    
    public init(gallery: ImageGalleryData) {
        self.init(items: gallery.items, initialIndex: gallery.initialIndex)
    }
    
    public init(item: IdentifiableImage) {
        self.init(items: [item], initialIndex: 0)
    }
    
    public init(image: UIImage) {
        self.init(item: IdentifiableImage(image: image))
    }
    
    public init(url: URL) {
        self.init(item: IdentifiableImage(url: url))
    }
    
    public func makeUIViewController(context: Context) -> FullScreenGalleryViewController {
        FullScreenGalleryViewController(
            items: items,
            initialIndex: initialIndex,
            onDismiss: {
                var transaction = Transaction()
                transaction.disablesAnimations = true
                withTransaction(transaction) {
                    dismiss()
                }
            }
        )
    }
    
    public func updateUIViewController(_ uiViewController: FullScreenGalleryViewController, context: Context) {}
}

public final class FullScreenGalleryViewController: UIViewController, UIPageViewControllerDataSource, UIPageViewControllerDelegate, UIGestureRecognizerDelegate {
    public let items: [IdentifiableImage]
    public var currentIndex: Int
    public let onDismiss: () -> Void
    
    private var pageViewController: UIPageViewController!
    private let backgroundView = UIView()
    
    override public var prefersStatusBarHidden: Bool { true }
    
    public init(items: [IdentifiableImage], initialIndex: Int, onDismiss: @escaping () -> Void) {
        self.items = items
        self.currentIndex = max(0, min(initialIndex, max(0, items.count - 1)))
        self.onDismiss = onDismiss
        super.init(nibName: nil, bundle: nil)
        modalPresentationStyle = .overFullScreen
    }
    
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    
    override public func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        view.backgroundColor = .clear
        view.superview?.backgroundColor = .clear
        view.superview?.superview?.backgroundColor = .clear
    }
    
    override public func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .clear
        
        // Dimming backdrop
        backgroundView.frame = view.bounds
        backgroundView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        backgroundView.backgroundColor = .black
        view.addSubview(backgroundView)
        
        // Native horizontal page carousel with inter-page spacing
        pageViewController = UIPageViewController(
            transitionStyle: .scroll,
            navigationOrientation: .horizontal,
            options: [.interPageSpacing: 20]
        )
        pageViewController.dataSource = self
        pageViewController.delegate = self
        
        addChild(pageViewController)
        pageViewController.view.frame = view.bounds
        pageViewController.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        pageViewController.view.backgroundColor = .clear
        view.addSubview(pageViewController.view)
        pageViewController.didMove(toParent: self)
        
        // Set initial page
        if let initialVC = makePageVC(for: currentIndex) {
            pageViewController.setViewControllers([initialVC], direction: .forward, animated: false)
        }
        
        // Vertical pull-to-dismiss gesture recognizer
        let pan = UIPanGestureRecognizer(target: self, action: #selector(handleDismissPan(_:)))
        pan.delegate = self
        view.addGestureRecognizer(pan)
    }
    
    private func makePageVC(for index: Int) -> SingleImagePreviewController? {
        guard items.indices.contains(index) else { return nil }
        let vc = SingleImagePreviewController(item: items[index], onSingleTap: { [weak self] in
            self?.handleClose()
        })
        vc.view.tag = index
        return vc
    }
    
    // MARK: - UIPageViewControllerDataSource
    
    public func pageViewController(_ pageViewController: UIPageViewController, viewControllerBefore viewController: UIViewController) -> UIViewController? {
        let index = viewController.view.tag
        guard index > 0 else { return nil }
        return makePageVC(for: index - 1)
    }
    
    public func pageViewController(_ pageViewController: UIPageViewController, viewControllerAfter viewController: UIViewController) -> UIViewController? {
        let index = viewController.view.tag
        guard index < items.count - 1 else { return nil }
        return makePageVC(for: index + 1)
    }
    
    // MARK: - UIPageViewControllerDelegate
    
    public func pageViewController(_ pageViewController: UIPageViewController, didFinishAnimating finished: Bool, previousViewControllers: [UIViewController], transitionCompleted completed: Bool) {
        guard completed, let currentVC = pageViewController.viewControllers?.first else { return }
        currentIndex = currentVC.view.tag
    }
    
    // MARK: - UIGestureRecognizerDelegate
    
    public func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
        guard let pan = gestureRecognizer as? UIPanGestureRecognizer else { return true }
        // Do not intercept when image is zoomed in
        if let currentVC = pageViewController.viewControllers?.first as? SingleImagePreviewController {
            if currentVC.scrollView.zoomScale > 1.01 {
                return false
            }
        }
        let velocity = pan.velocity(in: view)
        // If movement is predominantly vertical (pull down or up), take over for dismiss
        // If movement is horizontal, return FALSE so UIPageViewController handles page flipping!
        return abs(velocity.y) > abs(velocity.x) * 1.3 && abs(velocity.y) > 20
    }
    
    // MARK: - Pull to dismiss
    
    @objc private func handleDismissPan(_ pan: UIPanGestureRecognizer) {
        let translation = pan.translation(in: view)
        let velocity = pan.velocity(in: view)
        let dy = translation.y
        let dx = translation.x
        let progress = min(1.0, abs(dy) / 220.0)
        
        switch pan.state {
        case .changed:
            // Background directly fades out as you drag, revealing the background behind immediately!
            backgroundView.alpha = max(0.0, 1.0 - progress * 1.25)
            
            // Image follows finger, scales slightly down (1.0 -> ~0.72) and gradually fades
            let scale = max(0.72, 1.0 - progress * 0.28)
            let currentAlpha = max(0.25, 1.0 - progress * 0.65)
            pageViewController.view.transform = CGAffineTransform(translationX: dx * 0.35, y: dy).scaledBy(x: scale, y: scale)
            pageViewController.view.alpha = currentAlpha
            
        case .ended, .cancelled:
            let shouldDismiss = progress > 0.25 || abs(velocity.y) > 420
            if shouldDismiss {
                UIView.animate(withDuration: 0.16, delay: 0, options: [.curveEaseOut], animations: {
                    let endScale: CGFloat = 0.62
                    let extraY = dy > 0 ? 25.0 : -25.0
                    self.pageViewController.view.transform = CGAffineTransform(translationX: dx * 0.35, y: dy + extraY).scaledBy(x: endScale, y: endScale)
                    self.pageViewController.view.alpha = 0
                    self.backgroundView.alpha = 0
                }) { _ in
                    self.onDismiss()
                }
            } else {
                UIView.animate(withDuration: 0.24, delay: 0, usingSpringWithDamping: 0.86, initialSpringVelocity: 0, animations: {
                    self.pageViewController.view.transform = .identity
                    self.pageViewController.view.alpha = 1.0
                    self.backgroundView.alpha = 1.0
                })
            }
            
        default:
            break
        }
    }
    
    @objc private func handleClose() {
        UIView.animate(withDuration: 0.16, delay: 0, options: [.curveEaseOut], animations: {
            self.pageViewController.view.transform = CGAffineTransform(scaleX: 0.85, y: 0.85)
            self.pageViewController.view.alpha = 0
            self.backgroundView.alpha = 0
        }) { _ in
            self.onDismiss()
        }
    }
}

final class SingleImagePreviewController: UIViewController, UIScrollViewDelegate, UIPopoverPresentationControllerDelegate {
    let item: IdentifiableImage
    let onSingleTap: () -> Void
    
    let scrollView = UIScrollView()
    let imageView = UIImageView()
    let spinner = UIActivityIndicatorView(style: .large)
    
    init(item: IdentifiableImage, onSingleTap: @escaping () -> Void) {
        self.item = item
        self.onSingleTap = onSingleTap
        super.init(nibName: nil, bundle: nil)
    }
    
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    
    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .clear
        
        scrollView.frame = view.bounds
        scrollView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        scrollView.delegate = self
        scrollView.minimumZoomScale = 1.0
        scrollView.maximumZoomScale = 4.5
        scrollView.showsHorizontalScrollIndicator = false
        scrollView.showsVerticalScrollIndicator = false
        scrollView.contentInsetAdjustmentBehavior = .never
        view.addSubview(scrollView)
        
        imageView.contentMode = .scaleAspectFit
        imageView.clipsToBounds = true
        scrollView.addSubview(imageView)
        
        spinner.color = .white
        spinner.hidesWhenStopped = true
        view.addSubview(spinner)
        
        // Double-tap to zoom in/out
        let doubleTap = UITapGestureRecognizer(target: self, action: #selector(handleDoubleTap(_:)))
        doubleTap.numberOfTapsRequired = 2
        view.addGestureRecognizer(doubleTap)
        
        // Single-tap to dismiss
        let singleTap = UITapGestureRecognizer(target: self, action: #selector(handleSingleTap))
        singleTap.numberOfTapsRequired = 1
        singleTap.require(toFail: doubleTap)
        view.addGestureRecognizer(singleTap)
        
        // Long-press to save image to album
        let longPress = UILongPressGestureRecognizer(target: self, action: #selector(handleLongPress(_:)))
        longPress.minimumPressDuration = 0.4
        view.addGestureRecognizer(longPress)
        
        loadImage()
    }
    
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        spinner.center = CGPoint(x: view.bounds.midX, y: view.bounds.midY)
        updateImageFrame()
    }
    
    private func loadImage() {
        if let img = item.image {
            imageView.image = img
            updateImageFrame()
        } else if let url = item.url {
            if url.isFileURL, let data = try? Data(contentsOf: url), let img = UIImage(data: data) {
                imageView.image = img
                updateImageFrame()
                return
            }
            
            spinner.startAnimating()
            URLSession.shared.dataTask(with: url) { [weak self] data, _, _ in
                guard let self = self, let data = data, let img = UIImage(data: data) else {
                    DispatchQueue.main.async {
                        self?.spinner.stopAnimating()
                    }
                    return
                }
                DispatchQueue.main.async {
                    self.spinner.stopAnimating()
                    self.imageView.image = img
                    self.updateImageFrame()
                }
            }.resume()
        }
    }
    
    func updateImageFrame() {
        guard let img = imageView.image, img.size.width > 0, img.size.height > 0 else {
            imageView.frame = view.bounds
            scrollView.contentSize = view.bounds.size
            return
        }
        
        let boundsSize = view.bounds.size
        guard boundsSize.width > 0, boundsSize.height > 0 else { return }
        
        let widthRatio = boundsSize.width / img.size.width
        let heightRatio = boundsSize.height / img.size.height
        let fitRatio = min(widthRatio, heightRatio)
        
        let fittedWidth = img.size.width * fitRatio
        let fittedHeight = img.size.height * fitRatio
        
        let originX = max(0, (boundsSize.width - fittedWidth) / 2)
        let originY = max(0, (boundsSize.height - fittedHeight) / 2)
        
        imageView.frame = CGRect(x: originX, y: originY, width: fittedWidth, height: fittedHeight)
        scrollView.contentSize = boundsSize
    }
    
    func scrollViewDidZoom(_ scrollView: UIScrollView) {
        let boundsSize = scrollView.bounds.size
        var frameToCenter = imageView.frame
        
        if frameToCenter.size.width < boundsSize.width {
            frameToCenter.origin.x = (boundsSize.width - frameToCenter.size.width) / 2
        } else {
            frameToCenter.origin.x = 0
        }
        
        if frameToCenter.size.height < boundsSize.height {
            frameToCenter.origin.y = (boundsSize.height - frameToCenter.size.height) / 2
        } else {
            frameToCenter.origin.y = 0
        }
        
        imageView.frame = frameToCenter
    }
    
    func viewForZooming(in scrollView: UIScrollView) -> UIView? {
        return imageView
    }
    
    @objc private func handleDoubleTap(_ gesture: UITapGestureRecognizer) {
        if scrollView.zoomScale > 1.05 {
            scrollView.setZoomScale(1.0, animated: true)
        } else {
            let point = gesture.location(in: imageView)
            let zoomWidth = view.bounds.width / 2.5
            let zoomHeight = view.bounds.height / 2.5
            let zoomRect = CGRect(
                x: point.x - (zoomWidth / 2.0),
                y: point.y - (zoomHeight / 2.0),
                width: zoomWidth,
                height: zoomHeight
            )
            scrollView.zoom(to: zoomRect, animated: true)
        }
    }
    
    @objc private func handleSingleTap() {
        if scrollView.zoomScale <= 1.05 {
            onSingleTap()
        }
    }
    
    // MARK: - Long Press & Save to Album
    
    @objc private func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
        guard gesture.state == .began else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        let alert = UIAlertController(title: nil, message: nil, preferredStyle: .actionSheet)
        alert.addAction(UIAlertAction(title: "保存到相册", style: .default) { [weak self] _ in
            self?.saveToPhotosAlbum()
        })
        alert.addAction(UIAlertAction(title: "取消", style: .cancel, handler: nil))
        
        if let popover = alert.popoverPresentationController {
            popover.sourceView = view
            popover.sourceRect = CGRect(x: view.bounds.midX, y: view.bounds.midY, width: 0, height: 0)
            popover.permittedArrowDirections = []
            popover.delegate = self
        }
        
        present(alert, animated: true)
    }
    
    // MARK: - UIPopoverPresentationControllerDelegate
    
    public func adaptivePresentationStyle(for controller: UIPresentationController) -> UIModalPresentationStyle {
        return .none
    }
    
    public func adaptivePresentationStyle(for controller: UIPresentationController, traitCollection: UITraitCollection) -> UIModalPresentationStyle {
        return .none
    }
    
    private func saveToPhotosAlbum() {
        guard let image = imageView.image else {
            showToast(message: "图片正在加载，请稍后重试", isError: true)
            return
        }
        
        PHPhotoLibrary.requestAuthorization(for: .addOnly) { [weak self] status in
            guard status == .authorized || status == .limited else {
                DispatchQueue.main.async {
                    self?.showPermissionAlert()
                }
                return
            }
            
            if let url = self?.item.url, url.isFileURL, FileManager.default.fileExists(atPath: url.path) {
                PHPhotoLibrary.shared().performChanges {
                    let request = PHAssetCreationRequest.forAsset()
                    request.addResource(with: .photo, fileURL: url, options: nil)
                } completionHandler: { success, error in
                    DispatchQueue.main.async {
                        if success {
                            UINotificationFeedbackGenerator().notificationOccurred(.success)
                            self?.showToast(message: "已保存到相册", isError: false)
                        } else {
                            UINotificationFeedbackGenerator().notificationOccurred(.error)
                            self?.showToast(message: "保存失败: \(error?.localizedDescription ?? "未知错误")", isError: true)
                        }
                    }
                }
            } else {
                PHPhotoLibrary.shared().performChanges {
                    PHAssetChangeRequest.creationRequestForAsset(from: image)
                } completionHandler: { success, error in
                    DispatchQueue.main.async {
                        if success {
                            UINotificationFeedbackGenerator().notificationOccurred(.success)
                            self?.showToast(message: "已保存到相册", isError: false)
                        } else {
                            UINotificationFeedbackGenerator().notificationOccurred(.error)
                            self?.showToast(message: "保存失败: \(error?.localizedDescription ?? "未知错误")", isError: true)
                        }
                    }
                }
            }
        }
    }
    
    private func showPermissionAlert() {
        let alert = UIAlertController(
            title: "需要相册权限",
            message: "请在系统“设置”中允许 Antigravity 访问相册以保存图片。",
            preferredStyle: .alert
        )
        alert.addAction(UIAlertAction(title: "前往设置", style: .default) { _ in
            if let settingsURL = URL(string: UIApplication.openSettingsURLString),
               UIApplication.shared.canOpenURL(settingsURL) {
                UIApplication.shared.open(settingsURL)
            }
        })
        alert.addAction(UIAlertAction(title: "取消", style: .cancel, handler: nil))
        present(alert, animated: true)
    }
    
    private func showToast(message: String, isError: Bool = false) {
        guard let hostView = self.parent?.view ?? self.view else { return }
        
        hostView.subviews.filter { $0.tag == 998811 }.forEach { $0.removeFromSuperview() }
        
        let container = UIView()
        container.tag = 998811
        container.backgroundColor = UIColor.black.withAlphaComponent(0.85)
        container.layer.cornerRadius = 18
        container.layer.masksToBounds = true
        container.translatesAutoresizingMaskIntoConstraints = false
        
        let iconName = isError ? "exclamationmark.circle.fill" : "checkmark.circle.fill"
        let iconColor: UIColor = isError ? .systemRed : .systemGreen
        let iconView = UIImageView(image: UIImage(systemName: iconName))
        iconView.tintColor = iconColor
        iconView.translatesAutoresizingMaskIntoConstraints = false
        
        let label = UILabel()
        label.text = message
        label.textColor = .white
        label.font = UIFont.systemFont(ofSize: 14, weight: .medium)
        label.textAlignment = .center
        label.translatesAutoresizingMaskIntoConstraints = false
        
        let stack = UIStackView(arrangedSubviews: [iconView, label])
        stack.axis = .horizontal
        stack.spacing = 8
        stack.alignment = .center
        stack.translatesAutoresizingMaskIntoConstraints = false
        
        container.addSubview(stack)
        hostView.addSubview(container)
        
        NSLayoutConstraint.activate([
            stack.topAnchor.constraint(equalTo: container.topAnchor, constant: 10),
            stack.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -10),
            stack.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 16),
            stack.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -16),
            iconView.widthAnchor.constraint(equalToConstant: 18),
            iconView.heightAnchor.constraint(equalToConstant: 18),
            container.centerXAnchor.constraint(equalTo: hostView.centerXAnchor),
            container.bottomAnchor.constraint(equalTo: hostView.safeAreaLayoutGuide.bottomAnchor, constant: -50)
        ])
        
        container.alpha = 0
        container.transform = CGAffineTransform(scaleX: 0.9, y: 0.9)
        
        UIView.animate(withDuration: 0.2, delay: 0, options: [.curveEaseOut], animations: {
            container.alpha = 1.0
            container.transform = .identity
        }) { _ in
            UIView.animate(withDuration: 0.25, delay: 2.0, options: [.curveEaseIn], animations: {
                container.alpha = 0
                container.transform = CGAffineTransform(scaleX: 0.9, y: 0.9)
            }) { _ in
                container.removeFromSuperview()
            }
        }
    }
}

