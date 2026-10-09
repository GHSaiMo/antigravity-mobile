import SwiftUI
import Photos

extension MessageBubbleView {
    var fallbackAgentGalleryItems: [IdentifiableImage] {
        fallbackAgentImageURLs.compactMap { URL(string: $0) }.map { IdentifiableImage(url: $0) }
    }
    
    var agentMultiImageRow: some View {
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
                    .imageContextMenu(item: item) {
                        previewGallery = ImageGalleryData(items: fallbackAgentGalleryItems, initialIndex: index)
                    }
                }
            }
            .padding(.horizontal, 2)
            .padding(.bottom, 2)
        }
    }
    
    var agentBubble: some View {
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
            
            if !message.artifacts.isEmpty {
                VStack(spacing: 8) {
                    ForEach(message.artifacts, id: \.uri) { artifact in
                        ArtifactPreviewCardView(artifact: artifact)
                    }
                }
                .padding(.top, 2)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .contentShape(.contextMenuPreview, RoundedRectangle(cornerRadius: 18, style: .continuous))
        .contextMenu {
            let plainText = agentPlainText
            if !plainText.isEmpty {
                Button {
                    showTextSelectionSheet = true
                } label: {
                    Label("选择文本", systemImage: "selection.pin.in.out")
                }

                Button {
                    copyWholeText(plainText)
                } label: {
                    Label("复制全文", systemImage: "doc.on.doc")
                }
            }
            if let onExportMarkdown {
                Button {
                    onExportMarkdown()
                } label: {
                    Label("导出 MD", systemImage: "square.and.arrow.down")
                }
            }
            if !plainText.isEmpty || !message.imageUrls.isEmpty {
                shareLongImageButton
            }
        }
    }
    
    /// Image URLs attached to the agent message that are not already rendered
    /// from Markdown image / MEDIA syntax in `message.content`.
    var fallbackAgentImageURLs: [String] {
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
    
    func agentAsyncImageBubble(for url: URL, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
        // 与 Markdown 内嵌图片共用同一个带圆角边框的缓存图片视图，样式与用户发出的图片一致
        CachedMarkdownAsyncImageView(
            url: url,
            alt: "",
            onTap: { _ in
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                let effectiveItems = gallery.isEmpty ? [IdentifiableImage(url: url)] : gallery
                previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
            },
            onFailure: { _ in
                AnyView(
                    HStack(spacing: 6) {
                        Image(systemName: "photo")
                        Text("图片加载失败")
                    }
                    .font(.footnote)
                    .foregroundColor(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                )
            }
        )
    }
}
