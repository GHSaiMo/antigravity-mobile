import SwiftUI

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
                    Image(uiImage: uiImage)
                        .resizable()
                        .scaledToFit()
                        .frame(maxWidth: .infinity, maxHeight: 280, alignment: .leading)
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
            CachedMarkdownAsyncImageView(
                url: url,
                alt: alt,
                onTap: { handleImageTap(url: $0) },
                onFailure: { AnyView(markdownImageFailure(alt: $0)) }
            )
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
