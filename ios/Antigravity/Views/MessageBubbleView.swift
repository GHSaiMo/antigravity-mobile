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
    public let message: ChatMessage
    public let isActiveToolBatch: Bool
    @State private var isThinkingExpanded: Bool = false
    
    @State private var previewImage: IdentifiableImage? = nil
    
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
        .fullScreenCover(item: $previewImage) { item in
            ImageViewerSheet(item: item)
        }
    }
    
    private var userBubble: some View {
        VStack(alignment: .trailing, spacing: 6) {
            // Attached user images (e.g. screenshots)
            if !message.imageDataList.isEmpty {
                ForEach(Array(message.imageDataList.enumerated()), id: \.offset) { _, data in
                    if let uiImg = UIImage(data: data) {
                        userImageBubble(for: uiImg)
                    }
                }
            }
            
            // Attached user image URLs (if any)
            if !message.imageUrls.isEmpty {
                ForEach(message.imageUrls, id: \.self) { urlString in
                    if let url = URL(string: urlString) {
                        userAsyncImageBubble(for: url)
                    }
                }
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
    
    private func userImageBubble(for uiImg: UIImage) -> some View {
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
            previewImage = IdentifiableImage(image: uiImg)
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
    
    private func userAsyncImageBubble(for url: URL) -> some View {
        AsyncImage(url: url) { phase in
            switch phase {
            case .empty:
                ProgressView()
                    .frame(width: 140, height: 140)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            case .success(let image):
                Button(action: {
                    previewImage = IdentifiableImage(url: url)
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
    
    private var agentBubble: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Attached / parsed images if present
            if !message.imageUrls.isEmpty {
                ForEach(message.imageUrls, id: \.self) { urlString in
                    if let url = URL(string: urlString) {
                        AsyncImage(url: url) { phase in
                            switch phase {
                            case .empty:
                                ProgressView()
                                    .frame(maxWidth: .infinity, minHeight: 120)
                            case .success(let image):
                                Button(action: {
                                    previewImage = IdentifiableImage(url: url)
                                }) {
                                    image
                                        .resizable()
                                        .scaledToFit()
                                        .clipShape(RoundedRectangle(cornerRadius: 12))
                                }
                                .buttonStyle(.plain)
                            case .failure:
                                HStack {
                                    Image(systemName: "photo")
                                    Text("图片加载失败")
                                }
                                .font(.footnote)
                                .foregroundColor(.secondary)
                            @unknown default:
                                EmptyView()
                            }
                        }
                        .frame(maxHeight: 260)
                    }
                }
            }
            
            // Rich Markdown content (Headings, horizontal-scroll tables, code blocks, lists)
            MarkdownContentView(content: message.content)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
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

public struct IdentifiableImage: Identifiable {
    public let id = UUID()
    public let image: UIImage?
    public let url: URL?
    
    public init(image: UIImage) {
        self.image = image
        self.url = nil
    }
    
    public init(url: URL) {
        self.image = nil
        self.url = url
    }
}

public struct ImageViewerSheet: View {
    public let image: UIImage?
    public let url: URL?
    @Environment(\.dismiss) private var dismiss
    
    @State private var scale: CGFloat = 1.0
    @State private var lastScale: CGFloat = 1.0
    @State private var offset: CGSize = .zero
    @State private var lastOffset: CGSize = .zero
    @State private var dismissOffset: CGFloat = 0.0
    
    public init(image: UIImage) {
        self.image = image
        self.url = nil
    }
    
    public init(url: URL) {
        self.image = nil
        self.url = url
    }
    
    public init(item: IdentifiableImage) {
        self.image = item.image
        self.url = item.url
    }
    
    public var body: some View {
        GeometryReader { proxy in
            let screenSize = proxy.size
            
            let fittedSize: CGSize = {
                if let img = image, img.size.width > 0, img.size.height > 0 {
                    let widthRatio = screenSize.width / img.size.width
                    let heightRatio = screenSize.height / img.size.height
                    let fitScale = min(widthRatio, heightRatio)
                    return CGSize(
                        width: max(1, img.size.width * fitScale),
                        height: max(1, img.size.height * fitScale)
                    )
                }
                return screenSize
            }()
            
            ZStack {
                Color.black
                    .opacity(max(0.15, 1.0 - Double(abs(dismissOffset) / 320.0)))
                    .ignoresSafeArea()
                
                Group {
                    if let image = image {
                        Image(uiImage: image)
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(width: fittedSize.width, height: fittedSize.height)
                    } else if let url = url {
                        AsyncImage(url: url) { phase in
                            switch phase {
                            case .empty:
                                ProgressView()
                                    .tint(.white)
                            case .success(let img):
                                img
                                    .resizable()
                                    .aspectRatio(contentMode: .fit)
                                    .frame(maxWidth: screenSize.width, maxHeight: screenSize.height)
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
                .scaleEffect(scale)
                .offset(x: offset.width, y: offset.height + dismissOffset)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .contentShape(Rectangle())
            .gesture(
                magnificationGesture(containerSize: screenSize, fittedSize: fittedSize)
                    .simultaneously(with: dragGesture(containerSize: screenSize, fittedSize: fittedSize))
            )
            .onTapGesture(count: 2) {
                handleDoubleTap()
            }
            .onTapGesture(count: 1) {
                dismiss()
            }
        }
        .ignoresSafeArea()
        .statusBarHidden()
    }
    
    private func magnificationGesture(containerSize: CGSize, fittedSize: CGSize) -> some Gesture {
        MagnificationGesture()
            .onChanged { value in
                let newScale = lastScale * value
                scale = max(0.8, min(newScale, 5.0))
            }
            .onEnded { _ in
                withAnimation(.spring(response: 0.28, dampingFraction: 0.82)) {
                    if scale < 1.0 {
                        scale = 1.0
                        lastScale = 1.0
                        offset = .zero
                        lastOffset = .zero
                    } else if scale > 5.0 {
                        scale = 5.0
                        lastScale = 5.0
                        clampOffset(containerSize: containerSize, fittedSize: fittedSize)
                    } else {
                        lastScale = scale
                        clampOffset(containerSize: containerSize, fittedSize: fittedSize)
                    }
                }
            }
    }
    
    private func dragGesture(containerSize: CGSize, fittedSize: CGSize) -> some Gesture {
        DragGesture()
            .onChanged { value in
                if scale > 1.01 {
                    let maxOffsetX = max(0, (fittedSize.width * scale - containerSize.width) / 2)
                    let maxOffsetY = max(0, (fittedSize.height * scale - containerSize.height) / 2)
                    
                    let proposedX = lastOffset.width + value.translation.width
                    let proposedY = lastOffset.height + value.translation.height
                    
                    let clampedX: CGFloat
                    if proposedX > maxOffsetX {
                        clampedX = maxOffsetX + (proposedX - maxOffsetX) * 0.3
                    } else if proposedX < -maxOffsetX {
                        clampedX = -maxOffsetX + (proposedX - (-maxOffsetX)) * 0.3
                    } else {
                        clampedX = proposedX
                    }
                    
                    let clampedY: CGFloat
                    if proposedY > maxOffsetY {
                        clampedY = maxOffsetY + (proposedY - maxOffsetY) * 0.3
                    } else if proposedY < -maxOffsetY {
                        clampedY = -maxOffsetY + (proposedY - (-maxOffsetY)) * 0.3
                    } else {
                        clampedY = proposedY
                    }
                    
                    offset = CGSize(width: clampedX, height: clampedY)
                } else {
                    dismissOffset = value.translation.height
                }
            }
            .onEnded { value in
                if scale > 1.01 {
                    withAnimation(.spring(response: 0.28, dampingFraction: 0.82)) {
                        clampOffset(containerSize: containerSize, fittedSize: fittedSize)
                    }
                } else {
                    if abs(value.translation.height) > 70 || abs(value.predictedEndTranslation.height) > 160 {
                        dismiss()
                    } else {
                        withAnimation(.spring(response: 0.28, dampingFraction: 0.82)) {
                            dismissOffset = 0
                        }
                    }
                }
            }
    }
    
    private func clampOffset(containerSize: CGSize, fittedSize: CGSize) {
        let maxOffsetX = max(0, (fittedSize.width * scale - containerSize.width) / 2)
        let maxOffsetY = max(0, (fittedSize.height * scale - containerSize.height) / 2)
        
        let clampedX = min(maxOffsetX, max(-maxOffsetX, offset.width))
        let clampedY = min(maxOffsetY, max(-maxOffsetY, offset.height))
        
        offset = CGSize(width: clampedX, height: clampedY)
        lastOffset = offset
    }
    
    private func handleDoubleTap() {
        withAnimation(.spring(response: 0.28, dampingFraction: 0.82)) {
            if scale > 1.05 {
                scale = 1.0
                lastScale = 1.0
                offset = .zero
                lastOffset = .zero
                dismissOffset = 0
            } else {
                scale = 2.5
                lastScale = 2.5
                offset = .zero
                lastOffset = .zero
                dismissOffset = 0
            }
        }
    }
}

