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
        HStack(alignment: .bottom, spacing: 8) {
            switch message.sender {
            case .user:
                Spacer(minLength: 40)
                userBubble
            case .agent:
                agentBubble
                Spacer(minLength: 20)
            case .toolBatch(let count, let tools):
                if isActiveToolBatch {
                    activeToolBatchCard(count: count, tools: tools)
                } else {
                    toolBatchBanner(count: count, tools: tools)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 4)
        .sheet(item: $previewImage) { item in
            ImageViewerSheet(image: item.image)
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
                                image
                                    .resizable()
                                    .scaledToFit()
                                    .clipShape(RoundedRectangle(cornerRadius: 12))
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
    
    private func toolBatchBanner(count: Int, tools: [String]) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "bolt.fill")
                .font(.system(size: 10))
                .foregroundColor(.orange)
            
            Text("已思考并执行 \(count) 项操作")
                .font(.system(size: 11.5, weight: .medium))
                .foregroundColor(.secondary)
            
            if !tools.isEmpty {
                Text("(\(tools.prefix(2).joined(separator: ", "))\(tools.count > 2 ? "..." : ""))")
                    .font(.system(size: 10.5, design: .monospaced))
                    .foregroundColor(.secondary.opacity(0.7))
                    .lineLimit(1)
            }
            
            Spacer()
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 4)
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.6))
        .clipShape(Capsule())
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
}

public struct IdentifiableImage: Identifiable {
    public let id = UUID()
    public let image: UIImage
    
    public init(image: UIImage) {
        self.image = image
    }
}

public struct ImageViewerSheet: View {
    public let image: UIImage
    @Environment(\.dismiss) private var dismiss
    
    public init(image: UIImage) {
        self.image = image
    }
    
    public var body: some View {
        NavigationStack {
            ZStack {
                Color.black.ignoresSafeArea()
                
                ScrollView([.horizontal, .vertical], showsIndicators: false) {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFit()
                        .padding()
                }
            }
            .navigationTitle("图片详情")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: { dismiss() }) {
                        Image(systemName: "xmark.circle.fill")
                            .font(.system(size: 20))
                            .foregroundColor(.white.opacity(0.85))
                    }
                }
            }
            .toolbarColorScheme(.dark, for: .navigationBar)
        }
    }
}

