import SwiftUI

public struct QueuedMessagesCardView: View {
    public let items: [QueuedMessageItem]
    public let onSendNow: @MainActor @Sendable (QueuedMessageItem) -> Void
    public let onEdit: @MainActor @Sendable (QueuedMessageItem) -> Void
    public let onDelete: @MainActor @Sendable (QueuedMessageItem) -> Void
    public let onToggleExpand: (@MainActor @Sendable (Bool) -> Void)?
    
    @State private var isExpanded: Bool = true
    @State private var tappedActionItemIds: Set<String> = []
    @State private var previewImage: IdentifiableImage? = nil
    
    public init(
        items: [QueuedMessageItem],
        onSendNow: @escaping @MainActor @Sendable (QueuedMessageItem) -> Void,
        onEdit: @escaping @MainActor @Sendable (QueuedMessageItem) -> Void,
        onDelete: @escaping @MainActor @Sendable (QueuedMessageItem) -> Void,
        onToggleExpand: (@MainActor @Sendable (Bool) -> Void)? = nil
    ) {
        self.items = items
        self.onSendNow = onSendNow
        self.onEdit = onEdit
        self.onDelete = onDelete
        self.onToggleExpand = onToggleExpand
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header Row
            HStack(spacing: 8) {
                Text("队列")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                
                // Count Badge
                Text("\(items.count)")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2.5)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(Capsule())
                
                Text("当前任务完成后自动发送")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                
                Spacer()
                
                // Expand / Collapse Toggle
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                        isExpanded.toggle()
                    }
                    onToggleExpand?(isExpanded)
                }) {
                    Image(systemName: "chevron.down")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : 180))
                        .frame(width: 28, height: 28)
                }
                .buttonStyle(.plain)
                .help(isExpanded ? "折叠队列" : "展开队列")
            }
            .zIndex(10)
            
            // Expandable Content
            if isExpanded {
                VStack(spacing: 6) {
                    ForEach(items) { item in
                        let isItemActionDisabled = tappedActionItemIds.contains(item.id)
                        QueuedMessageRowView(
                            item: item,
                            isActionDisabled: isItemActionDisabled,
                            onSendNow: {
                                guard !tappedActionItemIds.contains(item.id) else { return }
                                tappedActionItemIds.insert(item.id)
                                onSendNow(item)
                            },
                            onEdit: {
                                guard !tappedActionItemIds.contains(item.id) else { return }
                                tappedActionItemIds.insert(item.id)
                                onEdit(item)
                            },
                            onDelete: {
                                guard !tappedActionItemIds.contains(item.id) else { return }
                                tappedActionItemIds.insert(item.id)
                                onDelete(item)
                            },
                            onPreviewImage: { image in
                                previewImage = image
                            }
                        )
                        
                        if item.id != items.last?.id {
                            Divider().opacity(0.5)
                        }
                    }
                }
                .clipped()
                .zIndex(1)
                .transition(
                    .asymmetric(
                        insertion: .opacity.combined(with: .scale(scale: 0.98, anchor: .top)),
                        removal: .opacity
                    )
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .clipped()
        .padding(.horizontal, 16)
        .padding(.vertical, 13)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
        )
        .shadow(color: Color.black.opacity(0.12), radius: 10, x: 0, y: 4)
        .padding(.horizontal, 12)
        .padding(.bottom, 6)
        .onChange(of: items) { _, newItems in
            let validIds = Set(newItems.map(\.id))
            tappedActionItemIds = tappedActionItemIds.intersection(validIds)
        }
        .fullScreenCover(item: $previewImage) { item in
            ImageViewerSheet(item: item)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
    }
}

// MARK: - Row View

private struct QueuedMessageRowView: View {
    let item: QueuedMessageItem
    let isActionDisabled: Bool
    let onSendNow: () -> Void
    let onEdit: () -> Void
    let onDelete: () -> Void
    let onPreviewImage: (IdentifiableImage) -> Void
    
    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            // Attached thumbnail(s) if present (matching desktop Antigravity)
            if item.hasAttachments {
                QueuedAttachmentThumbnailsView(item: item, onPreviewImage: onPreviewImage)
            }
            
            let displayText = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
            Text(displayText.isEmpty ? (item.hasAttachments ? "图片" : "") : item.text)
                .font(.system(size: 14))
                .lineSpacing(2)
                .foregroundColor(.primary)
                .lineLimit(3)
                .frame(maxWidth: .infinity, alignment: .leading)
            
            // Action Decorators (Send Now, Edit, Delete)
            HStack(spacing: 6) {
                // Send Now
                Button(action: onSendNow) {
                    Image(systemName: "arrow.right.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.accentColor.opacity(isActionDisabled ? 0.4 : 1.0))
                }
                .buttonStyle(.plain)
                .disabled(isActionDisabled)
                .help("立即发送")
                
                // Edit
                Button(action: onEdit) {
                    Image(systemName: "pencil.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.secondary.opacity(isActionDisabled ? 0.4 : 1.0))
                }
                .buttonStyle(.plain)
                .disabled(isActionDisabled)
                .help("编辑")
                
                // Delete
                Button(action: onDelete) {
                    Image(systemName: "trash.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.red.opacity(isActionDisabled ? 0.35 : 0.85))
                }
                .buttonStyle(.plain)
                .disabled(isActionDisabled)
                .help("删除")
            }
        }
        .padding(.vertical, 6)
    }
}

// MARK: - Attachment Thumbnails View

private struct QueuedAttachmentThumbnailsView: View {
    let item: QueuedMessageItem
    let onPreviewImage: (IdentifiableImage) -> Void
    
    @State private var decodedImages: [UIImage] = []
    
    var body: some View {
        HStack(spacing: 4) {
            if !decodedImages.isEmpty {
                ForEach(Array(decodedImages.prefix(2).enumerated()), id: \.offset) { _, uiImg in
                    Button(action: {
                        onPreviewImage(IdentifiableImage(image: uiImg))
                    }) {
                        Image(uiImage: uiImg)
                            .resizable()
                            .scaledToFill()
                            .frame(width: 30, height: 30)
                            .background(Color(uiColor: .tertiarySystemFill))
                            .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                            )
                    }
                    .buttonStyle(.plain)
                }
            } else if let urls = item.imageUrls, !urls.isEmpty {
                ForEach(Array(urls.prefix(2).enumerated()), id: \.offset) { _, urlStr in
                    if let url = URL(string: urlStr) {
                        Button(action: {
                            onPreviewImage(IdentifiableImage(url: url))
                        }) {
                            AsyncImage(url: url) { phase in
                                switch phase {
                                case .success(let img):
                                    img
                                        .resizable()
                                        .scaledToFill()
                                case .failure:
                                    Image(systemName: "photo")
                                        .font(.system(size: 13))
                                        .foregroundColor(.secondary)
                                case .empty:
                                    ProgressView()
                                        .scaleEffect(0.6)
                                @unknown default:
                                    EmptyView()
                                }
                            }
                            .frame(width: 30, height: 30)
                            .background(Color(uiColor: .tertiarySystemFill))
                            .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
            } else {
                // Placeholder while decoding in background
                RoundedRectangle(cornerRadius: 6, style: .continuous)
                    .fill(Color(uiColor: .tertiarySystemFill))
                    .frame(width: 30, height: 30)
                    .overlay(
                        Image(systemName: "photo")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                    )
            }
        }
        .task(id: item.media) {
            decodedImages = item.decodedImages()
        }
    }
}
