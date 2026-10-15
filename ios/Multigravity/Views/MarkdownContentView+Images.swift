import SwiftUI
import AVFoundation

// MARK: - Video URL Detection

public func isVideoURL(_ raw: String) -> Bool {
    let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    let clean = lower.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
    let videoExtensions = [".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv", ".3gp", ".ts", ".flv"]
    for ext in videoExtensions {
        if clean.hasSuffix(ext) || clean.contains("\(ext)?") || clean.contains("\(ext)#") {
            return true
        }
    }
    if let comps = URLComponents(string: clean),
       let uri = comps.queryItems?.first(where: { $0.name == "uri" })?.value?.lowercased() {
        for ext in videoExtensions {
            if uri.hasSuffix(ext) || uri.contains(ext) {
                return true
            }
        }
    }
    return false
}

// MARK: - Video Thumbnail Memory Cache

public final class VideoThumbnailCache: @unchecked Sendable {
    public static let shared = VideoThumbnailCache()
    private let cache = NSCache<NSURL, UIImage>()
    
    private init() {
        cache.countLimit = 60
        cache.totalCostLimit = 40 * 1024 * 1024 // 40MB
    }
    
    public func image(for url: URL) -> UIImage? {
        cache.object(forKey: url as NSURL)
    }
    
    public func insert(_ image: UIImage, for url: URL) {
        let cost = Int(image.size.width * image.size.height * 4)
        cache.setObject(image, forKey: url as NSURL, cost: min(cost, 5 * 1024 * 1024))
    }
}

// MARK: - Markdown Image Memory Cache

public final class MarkdownImageCache: @unchecked Sendable {
    public static let shared = MarkdownImageCache()
    private let cache = NSCache<NSURL, UIImage>()

    private init() {
        cache.countLimit = 150
        cache.totalCostLimit = 60 * 1024 * 1024 // 60MB
    }

    public func image(for url: URL) -> UIImage? {
        cache.object(forKey: url as NSURL)
    }

    public func insert(_ image: UIImage, for url: URL) {
        let cost = Int(image.size.width * image.size.height * 4)
        cache.setObject(image, forKey: url as NSURL, cost: min(cost, 10 * 1024 * 1024))
    }
}

// MARK: - Cached Markdown Async Image View

struct CachedMarkdownAsyncImageView: View {
    let url: URL
    let alt: String
    let onTap: (URL) -> Void
    let onFailure: (String) -> AnyView

    @State private var uiImage: UIImage?
    @State private var isLoading = false
    @State private var hasFailed = false

    init(url: URL, alt: String, onTap: @escaping (URL) -> Void, onFailure: @escaping (String) -> AnyView) {
        self.url = url
        self.alt = alt
        self.onTap = onTap
        self.onFailure = onFailure
        // 同步命中缓存，保证离屏渲染（长图卡片）首帧就有图。
        _uiImage = State(initialValue: MarkdownImageCache.shared.image(for: url))
    }

    var body: some View {
        Group {
            if let uiImage {
                Button {
                    onTap(url)
                } label: {
                    // aspectRatio(.fit) 的视图尺寸就是实际图片尺寸，边框才能紧贴图片；scaledToFit 会撑满提议尺寸
                    Image(uiImage: uiImage)
                        .resizable()
                        .aspectRatio(uiImage.size, contentMode: .fit)
                        .frame(maxHeight: 280)
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                        )
                        .shadow(color: Color.black.opacity(0.06), radius: 3, x: 0, y: 1.5)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(.plain)
                .imageContextMenu(item: IdentifiableImage(image: uiImage, url: url)) {
                    onTap(url)
                }
                .accessibilityLabel(alt.isEmpty ? "图片" : alt)
            } else if hasFailed {
                onFailure(alt)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 120, maxHeight: 160, alignment: .leading)
                    .task(id: url) {
                        await loadImage()
                    }
            }
        }
        .onAppear {
            if uiImage == nil && !hasFailed {
                if let cached = MarkdownImageCache.shared.image(for: url) {
                    uiImage = cached
                } else {
                    var request = URLRequest(url: url)
                    request.cachePolicy = .returnCacheDataElseLoad
                    if let cachedData = URLCache.shared.cachedResponse(for: request)?.data,
                       let cached = UIImage(data: cachedData) {
                        MarkdownImageCache.shared.insert(cached, for: url)
                        uiImage = cached
                    }
                }
            }
        }
    }

    private func loadImage() async {
        if let cached = MarkdownImageCache.shared.image(for: url) {
            uiImage = cached
            return
        }
        var request = URLRequest(url: url)
        request.cachePolicy = .returnCacheDataElseLoad
        if let cachedData = URLCache.shared.cachedResponse(for: request)?.data,
           let cached = UIImage(data: cachedData) {
            MarkdownImageCache.shared.insert(cached, for: url)
            uiImage = cached
            return
        }
        if isLoading { return }
        isLoading = true
        defer { isLoading = false }

        do {
            var request = URLRequest(url: url)
            request.cachePolicy = .returnCacheDataElseLoad
            request.timeoutInterval = 15
            let (data, response) = try await URLSession.shared.data(for: request)
            if let http = response as? HTTPURLResponse, http.statusCode >= 400 {
                hasFailed = true
                return
            }
            if let decoded = UIImage(data: data) {
                MarkdownImageCache.shared.insert(decoded, for: url)
                uiImage = decoded
            } else {
                hasFailed = true
            }
        } catch {
            if !Task.isCancelled {
                hasFailed = true
            }
        }
    }
}

// MARK: - Markdown Video Preview Card

public struct MarkdownVideoPreviewCard: View {
    public let url: URL
    public let title: String
    public let rawUri: String
    
    @State private var posterImage: UIImage? = nil
    @State private var durationText: String? = nil
    @State private var isLoading = false
    @Environment(\.openURL) private var openURL
    
    public init(url: URL, title: String, rawUri: String) {
        self.url = url
        self.title = title
        self.rawUri = rawUri
        _posterImage = State(initialValue: VideoThumbnailCache.shared.image(for: url))
    }
    
    private var displayTitle: String {
        if !title.isEmpty && title != "视频预览" && title != "文档详情" {
            return title
        }
        let fn = url.lastPathComponent.removingPercentEncoding ?? url.lastPathComponent
        return (fn.isEmpty || fn == "raw") ? "MP4 视频" : fn
    }
    
    public var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            openURL(url)
        } label: {
            ZStack {
                // Background poster / placeholder
                if let posterImage {
                    Image(uiImage: posterImage)
                        .resizable()
                        .aspectRatio(contentMode: .fill)
                        .frame(maxWidth: .infinity)
                        .frame(height: 200)
                        .clipped()
                } else {
                    LinearGradient(
                        colors: [Color(red: 0.12, green: 0.14, blue: 0.18), Color(red: 0.06, green: 0.08, blue: 0.10)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                    .frame(maxWidth: .infinity)
                    .frame(height: 200)
                    .overlay(
                        VStack(spacing: 8) {
                            if isLoading {
                                ProgressView()
                                    .tint(.white.opacity(0.8))
                            } else {
                                Image(systemName: "video.fill")
                                    .font(.system(size: 32))
                                    .foregroundColor(.white.opacity(0.4))
                            }
                        }
                    )
                }
                
                // Dark vignette gradient overlay for text readability
                LinearGradient(
                    colors: [Color.black.opacity(0.12), Color.clear, Color.black.opacity(0.75)],
                    startPoint: .top,
                    endPoint: .bottom
                )
                
                // Centered frosted-glass Play button
                Circle()
                    .fill(.ultraThinMaterial)
                    .frame(width: 52, height: 52)
                    .overlay(
                        Circle()
                            .stroke(Color.white.opacity(0.35), lineWidth: 1)
                    )
                    .overlay(
                        Image(systemName: "play.fill")
                            .font(.system(size: 22, weight: .bold))
                            .foregroundColor(.white)
                            .offset(x: 2)
                    )
                    .shadow(color: Color.black.opacity(0.35), radius: 8, x: 0, y: 3)
                
                // Bottom metadata overlay
                VStack {
                    Spacer()
                    HStack(spacing: 8) {
                        Image(systemName: "video.fill")
                            .font(.system(size: 12))
                            .foregroundColor(.white.opacity(0.9))
                        
                        Text(displayTitle)
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.white)
                            .lineLimit(1)
                        
                        Spacer()
                        
                        HStack(spacing: 4) {
                            Text(durationText ?? "MP4")
                                .font(.system(size: 11, weight: .semibold, design: .rounded))
                                .foregroundColor(.white.opacity(0.95))
                        }
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.black.opacity(0.55))
                        .clipShape(Capsule())
                        .overlay(
                            Capsule().stroke(Color.white.opacity(0.2), lineWidth: 0.6)
                        )
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 10)
                }
            }
            .frame(maxWidth: .infinity)
            .frame(height: 200)
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
            )
            .shadow(color: Color.black.opacity(0.08), radius: 4, x: 0, y: 2)
        }
        .buttonStyle(.plain)
        .contextMenu {
            Button {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                openURL(url)
            } label: {
                Label("播放视频", systemImage: "play.circle")
            }
            ShareLink(item: url, preview: SharePreview(displayTitle)) {
                Label("分享视频", systemImage: "square.and.arrow.up")
            }
        }
        .task(id: url) {
            await loadThumbnailAndDuration()
        }
    }
    
    private func loadThumbnailAndDuration() async {
        if let cached = VideoThumbnailCache.shared.image(for: url) {
            self.posterImage = cached
            return
        }
        
        isLoading = true
        defer { isLoading = false }
        
        let asset = AVURLAsset(url: url)
        
        // Load duration
        if let duration = try? await asset.load(.duration) {
            let seconds = CMTimeGetSeconds(duration)
            if seconds.isFinite && seconds > 0 {
                let mins = Int(seconds) / 60
                let secs = Int(seconds) % 60
                self.durationText = String(format: "%02d:%02d", mins, secs)
            }
        }
        
        // Generate poster frame
        let generator = AVAssetImageGenerator(asset: asset)
        generator.appliesPreferredTrackTransform = true
        generator.maximumSize = CGSize(width: 800, height: 800)
        
        let targetTime = CMTime(seconds: 0.5, preferredTimescale: 600)
        do {
            let (cgImage, _) = try await generator.image(at: targetTime)
            let uiImg = UIImage(cgImage: cgImage)
            VideoThumbnailCache.shared.insert(uiImg, for: url)
            self.posterImage = uiImg
        } catch {
            if let (cgImage, _) = try? await generator.image(at: .zero) {
                let uiImg = UIImage(cgImage: cgImage)
                VideoThumbnailCache.shared.insert(uiImg, for: url)
                self.posterImage = uiImg
            }
        }
    }
}

extension MarkdownContentView {
    // MARK: - Embedded Markdown Images
    
    func resolvedImageURL(from raw: String) -> URL? {
        if let base = AppSettings.shared.gatewayURL {
            let resolved = APIClient.shared.resolveMediaURL(raw, baseURL: base)
            if let url = URL(string: resolved) {
                return url
            }
        }
        return URL(string: raw)
    }
    
    func handleImageTap(url: URL) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        if let onImageTap {
            onImageTap(url)
        } else {
            internalPreviewImage = IdentifiableImage(url: url)
        }
    }
    
    @ViewBuilder
    func markdownImageView(alt: String, urlString: String) -> some View {
        if let url = resolvedImageURL(from: urlString) {
            if isVideoURL(urlString) || isVideoURL(url.absoluteString) {
                MarkdownVideoPreviewCard(
                    url: url,
                    title: alt.isEmpty ? (url.lastPathComponent.isEmpty ? "视频预览" : url.lastPathComponent) : alt,
                    rawUri: urlString
                )
            } else {
                CachedMarkdownAsyncImageView(
                    url: url,
                    alt: alt,
                    onTap: { handleImageTap(url: $0) },
                    onFailure: { AnyView(markdownImageFailure(alt: $0)) }
                )
            }
        } else {
            markdownImageFailure(alt: alt)
        }
    }
    
    func markdownImageFailure(alt: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "photo")
            Text(alt.isEmpty ? "图片加载失败" : alt)
                .lineLimit(2)
        }
        .font(.footnote)
        .foregroundColor(.secondary)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityLabel(alt.isEmpty ? "图片加载失败" : alt)
    }
    
}

// MARK: - Share Export Support

private struct ShareExportKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    /// 为 true 时表示正在为长图卡片离屏渲染：避免 ScrollView / WebView 等离屏渲染为空白的控件。
    var isShareExport: Bool {
        get { self[ShareExportKey.self] }
        set { self[ShareExportKey.self] = newValue }
    }
}

/// 正常情况下横向滚动；导出长图时直接铺开，避免 ImageRenderer 无法渲染 ScrollView。
struct ExportAwareHorizontalScroll<Content: View>: View {
    @Environment(\.isShareExport) private var isShareExport
    @ViewBuilder let content: () -> Content

    var body: some View {
        if isShareExport {
            content()
        } else {
            ScrollView(.horizontal, showsIndicators: true) {
                content()
            }
        }
    }
}

extension MarkdownContentView {
    func shareExportPlaceholder(_ text: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "rectangle.on.rectangle.angled")
            Text(text)
        }
        .font(.footnote)
        .foregroundColor(.secondary)
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.12))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}
