import SwiftUI
import Photos

extension MessageBubbleView {
    var userAttachmentItems: [IdentifiableImage] {
        message.attachmentImages
    }
    
    var userBubble: some View {
        let parsed = AttachmentRules.parseBlock(message.content)
        let isVoiceMessage = VoiceTranscriptCleaner.isVoiceMessage(body: parsed.body, files: parsed.files)

        return VStack(alignment: .trailing, spacing: 6) {
            if !isVoiceMessage {
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
            }
            
            if isVoiceMessage {
                VoiceInteractionUserBubble(bodyText: parsed.body, onCopy: { copyWholeText(parsed.body) })
            } else if !parsed.body.isEmpty {
                SlashCommandTextRenderer.render(
                    text: parsed.body,
                    mentions: message.slashCommands ?? [],
                    iconColor: .white.withAlphaComponent(0.85)
                )
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
            if !parsed.body.isEmpty || !userAttachmentItems.isEmpty {
                shareLongImageButton
            }
            // 当前 Antigravity 不支持撤回接口时，调用方传 nil，菜单里就不显示这一项
            if let onUndo {
                Button(role: .destructive) {
                    onUndo(message)
                } label: {
                    Label("撤回", systemImage: "arrow.uturn.backward")
                }
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
        UserAsyncImageBubbleView(
            url: url,
            placeholder: placeholder,
            gallery: gallery,
            index: index
        ) { effectiveItems, effectiveIndex in
            previewGallery = ImageGalleryData(items: effectiveItems, initialIndex: effectiveIndex)
        }
    }
}

// MARK: - User Async Image Bubble View

private struct UserAsyncImageBubbleView: View {
    let url: URL
    let placeholder: UIImage?
    let gallery: [IdentifiableImage]
    let index: Int
    let onTap: ([IdentifiableImage], Int) -> Void
    
    @State private var uiImage: UIImage?
    @State private var isLoading: Bool = false
    @State private var hasFailed: Bool = false
    
    init(
        url: URL,
        placeholder: UIImage? = nil,
        gallery: [IdentifiableImage] = [],
        index: Int = 0,
        onTap: @escaping ([IdentifiableImage], Int) -> Void
    ) {
        self.url = url
        self.placeholder = placeholder
        self.gallery = gallery
        self.index = index
        self.onTap = onTap
        _uiImage = State(initialValue: placeholder ?? MarkdownImageCache.shared.image(for: url))
    }
    
    var body: some View {
        Group {
            if let displayImg = uiImage {
                let maxDisplayWidth: CGFloat = 240
                let maxDisplayHeight: CGFloat = 220
                let imgWidth = displayImg.size.width
                let imgHeight = displayImg.size.height
                
                let fittedSize: CGSize = {
                    guard imgWidth > 0, imgHeight > 0 else {
                        return CGSize(width: maxDisplayWidth, height: maxDisplayHeight)
                    }
                    let scale = min(maxDisplayWidth / imgWidth, maxDisplayHeight / imgHeight)
                    return CGSize(
                        width: max(40, imgWidth * scale),
                        height: max(30, imgHeight * scale)
                    )
                }()
                
                let effectiveGallery: [IdentifiableImage] = {
                    if gallery.isEmpty {
                        return [IdentifiableImage(image: displayImg, url: url)]
                    }
                    return gallery.enumerated().map { i, item in
                        if i == index {
                            return IdentifiableImage(image: displayImg, url: item.url ?? url)
                        }
                        return item
                    }
                }()
                
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onTap(effectiveGallery, index)
                }) {
                    Image(uiImage: displayImg)
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
                .imageContextMenu(item: IdentifiableImage(image: displayImg, url: url)) {
                    onTap(effectiveGallery, index)
                }
            } else if hasFailed {
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    Task { await loadImage() }
                }) {
                    HStack(spacing: 6) {
                        Image(systemName: "photo")
                        Text("图片加载失败，点击重试")
                    }
                    .font(.footnote)
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }
                .buttonStyle(.plain)
            } else {
                ProgressView()
                    .frame(width: 80, height: 80)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 14, style: .continuous)
                            .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                    )
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
            var req = URLRequest(url: url)
            req.cachePolicy = .returnCacheDataElseLoad
            req.timeoutInterval = 15
            let (data, response) = try await URLSession.shared.data(for: req)
            if let http = response as? HTTPURLResponse, http.statusCode >= 400 {
                hasFailed = true
                return
            }
            if let decoded = UIImage(data: data) {
                MarkdownImageCache.shared.insert(decoded, for: url)
                uiImage = decoded
                hasFailed = false
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

/// 清洗语音交互中的协议标签（<talker_agent_context>）、时间戳与格式化符号，提取干净自然的多轮语音文本。
/// 清洗语音交互中的协议标签（<talker_agent_context>、<voice_instructions> 等）、时间戳与说明性文本，提取干净自然的用户与智能体语音对话。
enum VoiceTranscriptCleaner {
    static let instructionsBlockRegex = try? NSRegularExpression(
        pattern: #"<(?:voice_instructions|instructions)>[\s\S]*?(?:</(?:voice_instructions|instructions)>|(?=<talker_agent_context>)|$)"#,
        options: .caseInsensitive
    )
    static let contextBlockRegex = try? NSRegularExpression(
        pattern: #"<talker_agent_context>([\s\S]*?)(?:</talker_agent_context>|$)"#,
        options: .caseInsensitive
    )
    static let tagRegex = try? NSRegularExpression(
        pattern: #"</?(?:voice_instructions|instructions|talker_agent_context)>"#,
        options: .caseInsensitive
    )
    static let timestampRegex = try? NSRegularExpression(
        pattern: #"^\[\d{4}[-/]\d{2}[-/]\d{2}[ T][^\]]+\]\s*|^\[\d{2}:\d{2}(?::\d{2})?\]\s*"#
    )
    static let roleRegex = try? NSRegularExpression(
        pattern: #"^(user|human|me|用户|我|talker|agent|assistant|ai|智能体|助手|语音)\s*[:：]\s*"#,
        options: .caseInsensitive
    )

    static func isVoiceMessage(body: String, files: [AttachmentRules.ParsedFile] = []) -> Bool {
        let trimmed = body.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.contains("<talker_agent_context>") || trimmed.contains("</talker_agent_context>") {
            return true
        }
        if trimmed.contains("<voice_instructions>") || trimmed.contains("</voice_instructions>") {
            return true
        }
        if trimmed.contains("talker:") && (trimmed.contains("user:") || trimmed.contains("[202")) {
            return true
        }
        if !files.isEmpty && files.allSatisfy({ ["wav", "m4a", "aac", "mp3", "caf", "opus"].contains($0.ext.lowercased()) }) && trimmed.isEmpty {
            return true
        }
        return false
    }

    static func clean(_ raw: String) -> String {
        let trimmedRaw = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedRaw.isEmpty else { return "" }

        var textToProcess = trimmedRaw
        var hasContext = false
        if let contextRegex = contextBlockRegex,
           let match = contextRegex.firstMatch(in: trimmedRaw, range: NSRange(trimmedRaw.startIndex..., in: trimmedRaw)),
           match.numberOfRanges > 1,
           let cRange = Range(match.range(at: 1), in: trimmedRaw) {
            let extracted = String(trimmedRaw[cRange]).trimmingCharacters(in: .whitespacesAndNewlines)
            if !extracted.isEmpty {
                textToProcess = extracted
                hasContext = true
            }
        }
        
        if !hasContext {
            if let instRegex = instructionsBlockRegex {
                textToProcess = instRegex.stringByReplacingMatches(in: textToProcess, range: NSRange(textToProcess.startIndex..., in: textToProcess), withTemplate: "")
            }
            if let tagRegex = tagRegex {
                textToProcess = tagRegex.stringByReplacingMatches(in: textToProcess, range: NSRange(textToProcess.startIndex..., in: textToProcess), withTemplate: "")
            }
        }

        let lines = textToProcess.components(separatedBy: .newlines)
        var cleaned: [String] = []
        var currentRole: String? = nil
        var currentLines: [String] = []
        var quoteOpen = false

        func finishCurrent() {
            if let role = currentRole, !currentLines.isEmpty {
                var text = currentLines.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
                if text.hasPrefix("\"") && text.hasSuffix("\"") && text.count >= 2 {
                    text = String(text.dropFirst().dropLast()).trimmingCharacters(in: .whitespacesAndNewlines)
                } else if text.hasPrefix("\"") {
                    text = String(text.dropFirst()).trimmingCharacters(in: .whitespacesAndNewlines)
                } else if text.hasSuffix("\"") {
                    text = String(text.dropLast()).trimmingCharacters(in: .whitespacesAndNewlines)
                }
                if !text.isEmpty {
                    let formatted = "\(role)\(text)"
                    if cleaned.last != formatted {
                        cleaned.append(formatted)
                    }
                }
            }
            currentRole = nil
            currentLines.removeAll()
            quoteOpen = false
        }

        for rawLine in lines {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.isEmpty { continue }

            var content = line
            if let timestampRegex = timestampRegex {
                let range = NSRange(content.startIndex..., in: content)
                content = timestampRegex.stringByReplacingMatches(in: content, range: range, withTemplate: "").trimmingCharacters(in: .whitespaces)
            }

            if let roleRegex = roleRegex,
               let match = roleRegex.firstMatch(in: content, range: NSRange(content.startIndex..., in: content)),
               let rRange = Range(match.range, in: content),
               match.numberOfRanges > 1,
               let groupRange = Range(match.range(at: 1), in: content) {
                finishCurrent()
                let matchedRole = String(content[groupRange]).lowercased()
                if ["user", "human", "me", "用户", "我"].contains(matchedRole) {
                    currentRole = "用户: "
                } else {
                    currentRole = "智能体: "
                }
                let rem = String(content[rRange.upperBound...]).trimmingCharacters(in: .whitespaces)
                let quoteCount = rem.filter { $0 == "\"" }.count
                quoteOpen = (quoteCount % 2 == 1)
                currentLines.append(rem)
            } else if quoteOpen && currentRole != nil {
                let quoteCount = content.filter { $0 == "\"" }.count
                if quoteCount % 2 == 1 {
                    quoteOpen = false
                }
                currentLines.append(content)
            } else if hasContext && currentRole != nil && !quoteOpen {
                currentLines.append(content)
            }
        }

        finishCurrent()
        return cleaned.joined(separator: "\n\n")
    }
}

/// 语音交互用户气泡：默认显示 Antigravity 专属 Voice 图标，点击可展开查看清洗后的对话详情
struct VoiceInteractionUserBubble: View {
    let bodyText: String
    let onCopy: () -> Void
    @State private var isExpanded: Bool = false

    private var cleanedText: String {
        VoiceTranscriptCleaner.clean(bodyText)
    }

    var body: some View {
        Button {
            if !cleanedText.isEmpty {
                withAnimation(.easeInOut(duration: 0.2)) {
                    isExpanded.toggle()
                }
            }
        } label: {
            Group {
                if !isExpanded {
                    HStack(spacing: 6) {
                        AntigravityVoiceShape()
                            .fill(Color.white)
                            .frame(width: 18, height: 18)
                        Text("语音交互")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.white)
                    }
                } else {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 6) {
                            AntigravityVoiceShape()
                                .fill(Color.white.opacity(0.85))
                                .frame(width: 16, height: 16)
                            Text("语音交互")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(.white.opacity(0.85))
                        }
                        Text(cleanedText)
                            .font(.system(size: 14.5))
                            .foregroundColor(.white)
                            .lineSpacing(3)
                            .multilineTextAlignment(.leading)
                    }
                    .frame(maxWidth: 320, alignment: .leading)
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .background(Color.indigo)
            .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        }
        .buttonStyle(.plain)
        .contextMenu {
            if !cleanedText.isEmpty {
                Button {
                    withAnimation {
                        isExpanded.toggle()
                    }
                } label: {
                    Label(isExpanded ? "折叠语音" : "展开详情", systemImage: isExpanded ? "arrow.down.right.and.arrow.up.left" : "arrow.up.left.and.arrow.down.right")
                }
                Button {
                    UIPasteboard.general.string = cleanedText
                } label: {
                    Label("复制语音内容", systemImage: "doc.on.doc")
                }
            }
        }
    }
}

