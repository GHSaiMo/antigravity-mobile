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
                }
            }
        }
    }

    private func loadImage() async {
        if let cached = MarkdownImageCache.shared.image(for: url) {
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
