import SwiftUI
import Photos

extension MessageBubbleView {
    var userAttachmentItems: [IdentifiableImage] {
        message.attachmentImages
    }
    
    var userBubble: some View {
        let parsed = AttachmentRules.parseBlock(message.content)
        return VStack(alignment: .trailing, spacing: 6) {
            if userAttachmentItems.count == 1, let singleItem = userAttachmentItems.first {
                if let url = singleItem.url {
                    userAsyncImageBubble(for: url, placeholder: singleItem.image, gallery: userAttachmentItems, index: 0)
                } else if let uiImg = singleItem.image {
                    userImageBubble(for: uiImg, gallery: userAttachmentItems, index: 0)
                }
            } else if userAttachmentItems.count > 1 {
                userMultiImageRow
            }
            
            // Files the user attached: the gateway appends an attachment block to the message text.
            if !parsed.files.isEmpty {
                VStack(alignment: .trailing, spacing: 6) {
                    ForEach(parsed.files) { file in
                        MessageFileCardView(file: file) {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            openURL(URL(fileURLWithPath: file.path))
                        }
                    }
                }
            }
            
            if !parsed.body.isEmpty {
                Text(parsed.body)
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
            if !parsed.body.isEmpty {
                Button {
                    copyWholeText(parsed.body)
                } label: {
                    Label("复制", systemImage: "doc.on.doc")
                }
            }
            if !parsed.body.isEmpty || !userAttachmentItems.isEmpty {
                shareLongImageButton
            }
            Button(role: .destructive) {
                onUndo?(message)
            } label: {
                Label("撤回", systemImage: "arrow.uturn.backward")
            }
        }
    }
    
    var userMultiImageRow: some View {
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
                    .imageContextMenu(item: item) {
                        previewGallery = ImageGalleryData(items: userAttachmentItems, initialIndex: index)
                    }
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
                        .imageContextMenu(item: item) {
                            previewGallery = ImageGalleryData(items: userAttachmentItems, initialIndex: index)
                        }
                    }
                }
                .padding(.horizontal, 2)
                .padding(.bottom, 2)
            }
            .defaultScrollAnchor(.trailing)
        }
    }
    
    func thumbnailView(for item: IdentifiableImage) -> some View {
        Group {
            if let url = item.url {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .empty:
                        if let uiImg = item.image {
                            Image(uiImage: uiImg)
                                .resizable()
                                .scaledToFill()
                                .frame(width: 72, height: 72)
                                .clipped()
                        } else {
                            ProgressView()
                                .frame(width: 72, height: 72)
                        }
                    case .success(let image):
                        image
                            .resizable()
                            .scaledToFill()
                            .frame(width: 72, height: 72)
                            .clipped()
                    case .failure:
                        if let uiImg = item.image {
                            Image(uiImage: uiImg)
                                .resizable()
                                .scaledToFill()
                                .frame(width: 72, height: 72)
                                .clipped()
                        } else {
                            Image(systemName: "photo")
                                .font(.system(size: 24))
                                .foregroundColor(.secondary)
                                .frame(width: 72, height: 72)
                        }
                    @unknown default:
                        EmptyView()
                    }
                }
            } else if let uiImg = item.image {
                Image(uiImage: uiImg)
                    .resizable()
                    .scaledToFill()
                    .frame(width: 72, height: 72)
                    .clipped()
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
    
    func userImageBubble(for uiImg: UIImage, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
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
        .imageContextMenu(item: IdentifiableImage(image: uiImg)) {
            let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: uiImg)] : gallery
            previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
        }
    }
    
    func userAsyncImageBubble(for url: URL, placeholder: UIImage? = nil, gallery: [IdentifiableImage] = [], index: Int = 0) -> some View {
        AsyncImage(url: url) { phase in
            switch phase {
            case .empty:
                if let placeholder = placeholder {
                    Image(uiImage: placeholder)
                        .resizable()
                        .scaledToFit()
                        .frame(maxWidth: 240, maxHeight: 220, alignment: .trailing)
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                        )
                } else {
                    ProgressView()
                        .frame(width: 140, height: 140)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
            case .success(let image):
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: placeholder, url: url)] : gallery
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
                .imageContextMenu(item: IdentifiableImage(image: placeholder, url: url)) {
                    let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: placeholder, url: url)] : gallery
                    previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
                }
            case .failure:
                if let placeholder = placeholder {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: placeholder, url: url)] : gallery
                        previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
                    }) {
                        Image(uiImage: placeholder)
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
                    .imageContextMenu(item: IdentifiableImage(image: placeholder, url: url)) {
                        let effectiveItems = gallery.isEmpty ? [IdentifiableImage(image: placeholder, url: url)] : gallery
                        previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: index)
                    }
                } else {
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
                }
            @unknown default:
                EmptyView()
            }
        }
    }
    
}
