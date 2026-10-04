import SwiftUI

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
            AsyncImage(url: url) { phase in
                switch phase {
                case .empty:
                    ProgressView()
                        .frame(maxWidth: .infinity, minHeight: 120, maxHeight: 160, alignment: .leading)
                case .success(let image):
                    Button {
                        handleImageTap(url: url)
                    } label: {
                        image
                            .resizable()
                            .scaledToFit()
                            .frame(maxWidth: .infinity, maxHeight: 280, alignment: .leading)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(alt.isEmpty ? "图片" : alt)
                case .failure:
                    markdownImageFailure(alt: alt)
                @unknown default:
                    EmptyView()
                }
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
