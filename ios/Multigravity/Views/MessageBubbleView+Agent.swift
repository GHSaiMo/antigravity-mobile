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
            if let think = message.thinking?.trimmingCharacters(in: .whitespacesAndNewlines), !think.isEmpty {
                VStack(alignment: .leading, spacing: 0) {
                    Button {
                        withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                            isThinkingExpanded.toggle()
                        }
                    } label: {
                        HStack(spacing: 6) {
                            Image(systemName: "brain.head.profile")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundColor(.indigo)
                            Text(message.thinkingDuration.map { String(localized: "已深度思考 · \($0)") } ?? String(localized: "已深度思考"))
                                .font(.system(size: 11.5, weight: .medium))
                                .foregroundColor(.secondary)
                            Image(systemName: "chevron.down")
                                .font(.system(size: 9.5, weight: .semibold))
                                .foregroundColor(.secondary)
                                .rotationEffect(.degrees(isThinkingExpanded ? 180 : 0))
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .background(Color.indigo.opacity(0.08))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .zIndex(2)

                    AccordionRevealContainer(isExpanded: isThinkingExpanded, topSpacing: 6) {
                        Text(think)
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                            .textSelection(.enabled)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color(uiColor: .tertiarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    }
                    .zIndex(1)
                }
                .frame(maxWidth: .infinity, alignment: .topLeading)
            }

            if !message.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                MarkdownContentView(content: message.content, isStreaming: isStreaming, onImageTap: { url in
                    previewGallery = ImageGalleryData(url: url)
                })
                InlineGitHubPRPillsSection(content: message.content)
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

            if let onInsertToParent, !agentPlainText.isEmpty {
                HStack {
                    Button {
                        onInsertToParent(agentPlainText)
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "arrow.turn.down.left")
                                .font(.system(size: 10.5, weight: .semibold))
                            Text("插入主对话")
                                .font(.system(size: 11.5, weight: .semibold))
                        }
                        .foregroundColor(.indigo)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .background(Color.indigo.opacity(0.10))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    Spacer(minLength: 0)
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
            if !plainText.isEmpty, let onInsertToParent {
                Section {
                    Button {
                        onInsertToParent(plainText)
                    } label: {
                        Label("插入主对话", systemImage: "arrow.turn.down.left")
                    }
                }
            }
            if !plainText.isEmpty, let onNarrate {
                Section {
                    Button {
                        onNarrate(plainText)
                    } label: {
                        Label("语音播报", systemImage: "speaker.wave.2")
                    }
                }
            }

            if (onFork != nil && message.effectiveStepIndex != nil) || (!plainText.isEmpty && onAskBtw != nil) {
                Section {
                    if let onFork, message.effectiveStepIndex != nil {
                        Button {
                            onFork(message)
                        } label: {
                            Label("分叉会话", systemImage: "arrow.triangle.branch")
                        }
                    }
                    if !plainText.isEmpty, let onAskBtw {
                        Button {
                            onAskBtw(message.effectiveStepIndex, plainText)
                        } label: {
                            Label("旁路快问", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                        }
                    }
                }
            }

            if !plainText.isEmpty || onExportMarkdown != nil || !message.imageUrls.isEmpty {
                Section {
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
                            Label("导出MD", systemImage: "square.and.arrow.down")
                        }
                    }
                    if !plainText.isEmpty || !message.imageUrls.isEmpty {
                        shareLongImageButton
                    }
                }
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
