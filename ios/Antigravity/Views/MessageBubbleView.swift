import SwiftUI

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
    @State private var isThinkingExpanded: Bool = false
    
    @State private var previewGallery: ImageGalleryData? = nil
    
    public init(message: ChatMessage, isActiveToolBatch: Bool = false) {
        self.message = message
        self.isActiveToolBatch = isActiveToolBatch
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

public struct ImageViewerSheet: View {
    public let items: [IdentifiableImage]
    @State private var currentIndex: Int
    @Environment(\.dismiss) private var dismiss
    
    // Interactive vertical pull-to-dismiss state
    @State private var dismissOffset: CGFloat = 0.0
    @State private var isDraggingVertically: Bool = false
    @State private var isDraggingHorizontally: Bool = false
    
    // Zoom tracking of current page
    @State private var isCurrentZoomed: Bool = false
    
    public var image: UIImage? { items.indices.contains(currentIndex) ? items[currentIndex].image : nil }
    public var url: URL? { items.indices.contains(currentIndex) ? items[currentIndex].url : nil }
    
    public init(items: [IdentifiableImage], initialIndex: Int = 0) {
        self.items = items
        self._currentIndex = State(initialValue: max(0, min(initialIndex, max(0, items.count - 1))))
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
    
    public var body: some View {
        GeometryReader { proxy in
            ZStack {
                // Interactive backdrop dimming
                Color.black
                    .opacity(max(0.15, 1.0 - Double(abs(dismissOffset) / 320.0)))
                    .ignoresSafeArea()
                
                // Native 120Hz CoreAnimation hardware-accelerated paging
                TabView(selection: $currentIndex) {
                    ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                        ZoomableImageView(
                            item: item,
                            isSelected: index == currentIndex,
                            onZoomChanged: { zoomed in
                                if index == currentIndex {
                                    isCurrentZoomed = zoomed
                                }
                            },
                            onSingleTap: {
                                dismiss()
                            }
                        )
                        .tag(index)
                    }
                }
                .tabViewStyle(.page(indexDisplayMode: .never))
                .scrollDisabled(isCurrentZoomed)
                .ignoresSafeArea()
                .offset(y: dismissOffset)
                
                // Top controls overlay: Close button & Page index indicator
                VStack {
                    HStack {
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            dismiss()
                        }) {
                            Image(systemName: "xmark")
                                .font(.system(size: 15, weight: .bold))
                                .foregroundColor(.white)
                                .frame(width: 36, height: 36)
                                .background(Color.black.opacity(0.55))
                                .clipShape(Circle())
                        }
                        .padding(.leading, 16)
                        
                        Spacer()
                        
                        if items.count > 1 {
                            Text("\(currentIndex + 1) / \(items.count)")
                                .font(.system(size: 14, weight: .semibold, design: .rounded))
                                .foregroundColor(.white)
                                .padding(.horizontal, 14)
                                .padding(.vertical, 6)
                                .background(Color.black.opacity(0.55))
                                .clipShape(Capsule())
                        }
                        
                        Spacer()
                        
                        Color.clear
                            .frame(width: 36, height: 36)
                            .padding(.trailing, 16)
                    }
                    .padding(.top, max(proxy.safeAreaInsets.top, 20))
                    
                    Spacer()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .simultaneousGesture(
                DragGesture(minimumDistance: 6)
                    .onChanged { value in
                        guard !isCurrentZoomed else { return }
                        let dx = abs(value.translation.width)
                        let dy = abs(value.translation.height)
                        
                        if !isDraggingVertically && !isDraggingHorizontally {
                            if dy > dx * 1.4 && dy > 8 {
                                isDraggingVertically = true
                            } else if dx > dy && dx > 8 {
                                isDraggingHorizontally = true
                            }
                        }
                        
                        if isDraggingVertically {
                            dismissOffset = value.translation.height
                        }
                    }
                    .onEnded { value in
                        guard !isCurrentZoomed else { return }
                        if isDraggingVertically {
                            let dy = value.translation.height
                            let predictedDy = value.predictedEndTranslation.height
                            if abs(dy) > 85 || abs(predictedDy) > 180 {
                                dismiss()
                            } else {
                                withAnimation(.interactiveSpring(response: 0.28, dampingFraction: 0.85)) {
                                    dismissOffset = 0
                                }
                            }
                        }
                        isDraggingVertically = false
                        isDraggingHorizontally = false
                    }
            )
            .onChange(of: currentIndex) { _, _ in
                isCurrentZoomed = false
            }
        }
        .ignoresSafeArea()
        .statusBarHidden()
    }
}

public struct ZoomableImageView: View {
    public let item: IdentifiableImage
    public let isSelected: Bool
    public let onZoomChanged: (Bool) -> Void
    public let onSingleTap: () -> Void
    
    @State private var scale: CGFloat = 1.0
    @State private var lastScale: CGFloat = 1.0
    @State private var offset: CGSize = .zero
    @State private var lastOffset: CGSize = .zero
    
    public init(
        item: IdentifiableImage,
        isSelected: Bool,
        onZoomChanged: @escaping (Bool) -> Void,
        onSingleTap: @escaping () -> Void
    ) {
        self.item = item
        self.isSelected = isSelected
        self.onZoomChanged = onZoomChanged
        self.onSingleTap = onSingleTap
    }
    
    public var body: some View {
        GeometryReader { proxy in
            let containerSize = proxy.size
            let fittedSize = computeFittedSize(in: containerSize)
            
            ZStack {
                Color.clear
                
                if let uiImg = item.image {
                    Image(uiImage: uiImg)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: fittedSize.width, height: fittedSize.height)
                        .scaleEffect(scale)
                        .offset(offset)
                } else if let url = item.url {
                    AsyncImage(url: url) { phase in
                        switch phase {
                        case .empty:
                            ProgressView().tint(.white)
                        case .success(let img):
                            img
                                .resizable()
                                .aspectRatio(contentMode: .fit)
                                .frame(width: fittedSize.width, height: fittedSize.height)
                                .scaleEffect(scale)
                                .offset(offset)
                        case .failure:
                            VStack(spacing: 8) {
                                Image(systemName: "photo")
                                    .font(.largeTitle)
                                    .foregroundColor(.white.opacity(0.6))
                                Text("图片加载失败")
                                    .font(.subheadline)
                                    .foregroundColor(.white.opacity(0.8))
                            }
                        @unknown default:
                            EmptyView()
                        }
                    }
                }
            }
            .frame(width: containerSize.width, height: containerSize.height)
            .contentShape(Rectangle())
            .gesture(magnificationGesture(in: containerSize))
            .simultaneousGesture(panGesture(in: containerSize))
            .onTapGesture(count: 2) {
                handleDoubleTap()
            }
            .onTapGesture(count: 1) {
                if scale <= 1.05 {
                    onSingleTap()
                }
            }
            .onChange(of: isSelected) { _, selected in
                if !selected && scale > 1.0 {
                    resetZoom()
                }
            }
        }
    }
    
    private func computeFittedSize(in containerSize: CGSize) -> CGSize {
        if let uiImg = item.image, uiImg.size.width > 0, uiImg.size.height > 0 {
            let widthRatio = containerSize.width / uiImg.size.width
            let heightRatio = containerSize.height / uiImg.size.height
            let fitRatio = min(widthRatio, heightRatio)
            return CGSize(
                width: max(1, uiImg.size.width * fitRatio),
                height: max(1, uiImg.size.height * fitRatio)
            )
        }
        return containerSize
    }
    
    private func magnificationGesture(in containerSize: CGSize) -> some Gesture {
        MagnificationGesture()
            .onChanged { value in
                let newScale = lastScale * value
                scale = max(0.8, min(newScale, 5.0))
                onZoomChanged(scale > 1.02)
            }
            .onEnded { _ in
                withAnimation(.spring(response: 0.25, dampingFraction: 0.85)) {
                    if scale < 1.05 {
                        resetZoom()
                    } else if scale > 4.5 {
                        scale = 4.5
                        lastScale = 4.5
                        clampOffset(in: containerSize)
                        onZoomChanged(true)
                    } else {
                        lastScale = scale
                        clampOffset(in: containerSize)
                        onZoomChanged(true)
                    }
                }
            }
    }
    
    private func panGesture(in containerSize: CGSize) -> some Gesture {
        DragGesture()
            .onChanged { value in
                guard scale > 1.02 else { return }
                
                let fitted = computeFittedSize(in: containerSize)
                let maxOffsetX = max(0, (fitted.width * scale - containerSize.width) / 2)
                let maxOffsetY = max(0, (fitted.height * scale - containerSize.height) / 2)
                
                let proposedX = lastOffset.width + value.translation.width
                let proposedY = lastOffset.height + value.translation.height
                
                let clampedX: CGFloat
                if proposedX > maxOffsetX {
                    clampedX = maxOffsetX + (proposedX - maxOffsetX) * 0.35
                } else if proposedX < -maxOffsetX {
                    clampedX = -maxOffsetX + (proposedX - (-maxOffsetX)) * 0.35
                } else {
                    clampedX = proposedX
                }
                
                let clampedY: CGFloat
                if proposedY > maxOffsetY {
                    clampedY = maxOffsetY + (proposedY - maxOffsetY) * 0.35
                } else if proposedY < -maxOffsetY {
                    clampedY = -maxOffsetY + (proposedY - (-maxOffsetY)) * 0.35
                } else {
                    clampedY = proposedY
                }
                
                offset = CGSize(width: clampedX, height: clampedY)
            }
            .onEnded { _ in
                guard scale > 1.02 else { return }
                withAnimation(.spring(response: 0.25, dampingFraction: 0.85)) {
                    clampOffset(in: containerSize)
                }
            }
    }
    
    private func clampOffset(in containerSize: CGSize) {
        let fitted = computeFittedSize(in: containerSize)
        let maxOffsetX = max(0, (fitted.width * scale - containerSize.width) / 2)
        let maxOffsetY = max(0, (fitted.height * scale - containerSize.height) / 2)
        
        let clampedX = min(maxOffsetX, max(-maxOffsetX, offset.width))
        let clampedY = min(maxOffsetY, max(-maxOffsetY, offset.height))
        
        offset = CGSize(width: clampedX, height: clampedY)
        lastOffset = offset
    }
    
    private func resetZoom() {
        scale = 1.0
        lastScale = 1.0
        offset = .zero
        lastOffset = .zero
        onZoomChanged(false)
    }
    
    private func handleDoubleTap() {
        withAnimation(.spring(response: 0.25, dampingFraction: 0.85)) {
            if scale > 1.05 {
                resetZoom()
            } else {
                scale = 2.5
                lastScale = 2.5
                offset = .zero
                lastOffset = .zero
                onZoomChanged(true)
            }
        }
    }
}

