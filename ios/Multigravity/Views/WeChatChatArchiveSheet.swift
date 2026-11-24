import SwiftUI
import AVKit

public struct WeChatChatArchiveSheet: View {
    public let item: WeChatArchiveSessionItem
    public let apiClient: APIClient
    public let baseURL: URL?
    public let cascadeId: String
    public let onFallbackToQuickLook: () -> Void
    public let onDismiss: () -> Void

    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.dismiss) private var dismiss

    @State private var archive: WeChatChatArchive?
    @State private var isLoading: Bool
    @State private var isRefreshing: Bool = false
    @State private var errorMessage: String? = nil
    @State private var mediaRefreshToken: Int = 0

    @State private var isSearching: Bool = false
    @State private var searchQuery: String = ""
    @State private var previewGallery: ImageGalleryData? = nil
    @State private var playingVideoURL: IdentifiableURL? = nil
    @State private var previewEntryURL: IdentifiableURL? = nil
    @State private var downloadedZipURL: URL? = nil
    @State private var isDownloadingZip: Bool = false
    @State private var showDocumentExporter: Bool = false
    @State private var toastMessage: String? = nil

    // Interactive left-edge swipe-back state
    @State private var edgeSwipeOffset: CGFloat = 0
    @State private var isEdgeSwiping: Bool = false
    @State private var edgeSwipeRejected: Bool = false

    public init(
        item: WeChatArchiveSessionItem,
        apiClient: APIClient,
        baseURL: URL?,
        cascadeId: String,
        onFallbackToQuickLook: @escaping () -> Void,
        onDismiss: @escaping () -> Void
    ) {
        self.item = item
        self.apiClient = apiClient
        self.baseURL = baseURL
        self.cascadeId = cascadeId
        self.onFallbackToQuickLook = onFallbackToQuickLook
        self.onDismiss = onDismiss
        let memCached = WeChatArchiveCacheManager.shared.cachedArchiveInMemory(uri: item.uri, cascadeId: cascadeId)
        _archive = State(initialValue: memCached)
        _isLoading = State(initialValue: memCached == nil)
    }

    private var isDark: Bool { colorScheme == .dark }
    private var headerBg: Color { isDark ? Color(hex: 0x191919) : Color(hex: 0xEDEDED) }
    private var listBg: Color { isDark ? Color(hex: 0x111111) : Color(hex: 0xFFFFFF) }
    private var textPrimary: Color { isDark ? Color(hex: 0xE6E6E6) : Color(hex: 0x191919) }
    private var textMuted: Color { isDark ? Color(hex: 0x8E8E93) : Color(hex: 0x888888) }
    private var dividerColor: Color { isDark ? Color(hex: 0x262626) : Color(hex: 0xEDEDED) }
    private var cardBg: Color { isDark ? Color(hex: 0x1E1E1E) : Color(hex: 0xF3F3F3) }
    private var mentionBlue: Color { isDark ? Color(hex: 0x7D90A9) : Color(hex: 0x576B95) }

    private static let avatarPalette: [Color] = [
        Color(hex: 0x07C160),
        Color(hex: 0x10AEFF),
        Color(hex: 0xFA9D3B),
        Color(hex: 0x6467F0),
        Color(hex: 0xF4606C),
        Color(hex: 0x19B5FE),
        Color(hex: 0xA06CD5),
        Color(hex: 0x2AC993)
    ]

    private var filteredMessages: [WeChatChatMessage] {
        guard let msgs = archive?.messages else { return [] }
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !q.isEmpty else { return msgs }
        return msgs.filter {
            $0.sender.lowercased().contains(q) ||
            $0.text.lowercased().contains(q) ||
            ($0.mediaName?.lowercased().contains(q) ?? false)
        }
    }

    private var allImageEntries: [(msgId: Int, image: IdentifiableImage)] {
        guard let msgs = archive?.messages else { return [] }
        return msgs.compactMap { msg in
            guard msg.kind == "image", let entry = msg.mediaEntry else {
                return nil
            }
            let memImg = WeChatArchiveCacheManager.shared.cachedMediaImageInMemory(
                uri: item.uri,
                cascadeId: cascadeId,
                entry: entry,
                thumb: false
            )
            let localURL = WeChatArchiveCacheManager.shared.cachedMediaFileURL(
                uri: item.uri,
                cascadeId: cascadeId,
                entry: entry,
                thumb: false
            )
            let remoteURL = baseURL.flatMap {
                apiClient.zipEntryURL(uri: item.uri, entry: entry, cascadeId: cascadeId, baseURL: $0)
            }
            guard memImg != nil || localURL != nil || remoteURL != nil else {
                return nil
            }
            return (msg.id, IdentifiableImage(image: memImg, url: localURL ?? remoteURL))
        }
    }

    public var body: some View {
        GeometryReader { geo in
            let screenWidth = max(1, geo.size.width)
            let swipeProgress = min(1, max(0, edgeSwipeOffset / screenWidth))

            ZStack(alignment: .leading) {
                if edgeSwipeOffset > 0 {
                    Color.black
                        .opacity(0.28 * (1 - swipeProgress))
                        .ignoresSafeArea()
                        .allowsHitTesting(false)
                }

                VStack(spacing: 0) {
                    topBar

                    if isSearching {
                        searchBar
                    }

                    Rectangle()
                        .fill(dividerColor)
                        .frame(height: 0.5)

                    Group {
                        if isLoading {
                            VStack(spacing: 12) {
                                ProgressView()
                                Text("正在解析微信聊天记录...")
                                    .font(.system(size: 13.5))
                                    .foregroundColor(textMuted)
                            }
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .background(listBg)
                        } else if let errorMessage {
                            VStack(spacing: 14) {
                                Image(systemName: "exclamationmark.triangle")
                                    .font(.system(size: 36))
                                    .foregroundColor(.orange)
                                Text(errorMessage)
                                    .font(.system(size: 14))
                                    .foregroundColor(textMuted)
                                    .multilineTextAlignment(.center)
                                    .padding(.horizontal, 32)
                                HStack(spacing: 12) {
                                    Button("重试") {
                                        Task { await loadArchive(forceRefresh: true) }
                                    }
                                    .buttonStyle(.borderedProminent)
                                    Button("改用系统预览") {
                                        dismiss()
                                        onDismiss()
                                        onFallbackToQuickLook()
                                    }
                                    .buttonStyle(.bordered)
                                }
                            }
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .background(listBg)
                        } else if filteredMessages.isEmpty {
                            VStack(spacing: 10) {
                                Image(systemName: isSearching ? "magnifyingglass" : "bubble.left.and.bubble.right")
                                    .font(.system(size: 32))
                                    .foregroundColor(textMuted)
                                Text(isSearching ? String(localized: "没有匹配的聊天记录") : String(localized: "聊天记录为空"))
                                    .font(.system(size: 14))
                                    .foregroundColor(textMuted)
                            }
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .background(listBg)
                        } else {
                            ScrollView {
                                LazyVStack(spacing: 0) {
                                    ForEach(filteredMessages) { msg in
                                        messageRow(msg)
                                    }
                                }
                                .padding(.bottom, 24)
                            }
                            .background(listBg)
                        }
                    }
                    .background(listBg.ignoresSafeArea(edges: .bottom))
                }
                .background(listBg.ignoresSafeArea())
                .offset(x: edgeSwipeOffset)
                .shadow(
                    color: edgeSwipeOffset > 0 ? Color.black.opacity(0.18) : .clear,
                    radius: 12,
                    x: -4,
                    y: 0
                )
                .simultaneousGesture(edgeSwipeBackGesture(screenWidth: screenWidth))
            }
        }
        .overlay(alignment: .center) {
            if let toastMessage {
                Text(toastMessage)
                    .font(.system(size: 13.5, weight: .medium))
                    .foregroundColor(.white)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 10)
                    .background(Color.black.opacity(0.78))
                    .clipShape(Capsule())
                    .transition(.opacity)
            }
        }
        .fullScreenCover(item: $previewGallery) { gallery in
            ImageViewerSheet(gallery: gallery)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
        .fullScreenCover(item: $playingVideoURL) { videoItem in
            WeChatVideoPlayerOverlay(url: videoItem.url) {
                playingVideoURL = nil
            }
        }
        .sheet(item: $previewEntryURL) { entryItem in
            QuickLookPreviewSheet(url: entryItem.url) {
                previewEntryURL = nil
            }
        }
        .sheet(isPresented: $showDocumentExporter) {
            if let zipURL = downloadedZipURL {
                DocumentExporterRepresentable(url: zipURL) { _ in
                    showToast(String(localized: "已保存原压缩包"))
                }
            }
        }
        .task(id: item.uri) {
            await loadArchive(forceRefresh: false)
        }
        .onDisappear {
            onDismiss()
        }
    }

    private func edgeSwipeBackGesture(screenWidth: CGFloat) -> some Gesture {
        DragGesture(minimumDistance: 12, coordinateSpace: .global)
            .onChanged { value in
                guard value.startLocation.x <= 28 else { return }
                let dx = value.translation.width
                let dy = value.translation.height

                if !isEdgeSwiping {
                    if edgeSwipeRejected { return }
                    if abs(dy) > abs(dx) && abs(dy) > 8 {
                        edgeSwipeRejected = true
                        return
                    }
                    if dx > 10 && dx > abs(dy) * 1.25 {
                        isEdgeSwiping = true
                    } else {
                        return
                    }
                }
                edgeSwipeOffset = max(0, dx)
            }
            .onEnded { value in
                defer {
                    edgeSwipeRejected = false
                    isEdgeSwiping = false
                }
                guard isEdgeSwiping else { return }
                let threshold = min(110, screenWidth * 0.3)
                if edgeSwipeOffset > threshold || value.predictedEndTranslation.width > screenWidth * 0.45 {
                    withAnimation(.easeOut(duration: 0.18)) {
                        edgeSwipeOffset = screenWidth
                    }
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.18) {
                        var transaction = Transaction()
                        transaction.disablesAnimations = true
                        withTransaction(transaction) {
                            dismiss()
                            onDismiss()
                        }
                    }
                } else {
                    withAnimation(.interactiveSpring(response: 0.28, dampingFraction: 0.86)) {
                        edgeSwipeOffset = 0
                    }
                }
            }
    }

    private var topBar: some View {
        HStack(spacing: 12) {
            Button {
                dismiss()
                onDismiss()
            } label: {
                Image(systemName: "chevron.left")
                    .font(.system(size: 18, weight: .medium))
                    .foregroundColor(textPrimary)
                    .frame(width: 36, height: 36)
                    .contentShape(Rectangle())
            }

            Spacer(minLength: 4)

            VStack(spacing: 1.5) {
                Text(archive?.title ?? String(localized: "群聊的聊天记录"))
                    .font(.system(size: 16.5, weight: .semibold))
                    .foregroundColor(textPrimary)
                    .lineLimit(1)
                if let range = archive?.dateRange, !range.isEmpty {
                    Text(range)
                        .font(.system(size: 11.5))
                        .foregroundColor(textMuted)
                        .lineLimit(1)
                } else {
                    Text(cleanDisplayFileName(item.fileName))
                        .font(.system(size: 11))
                        .foregroundColor(textMuted)
                        .lineLimit(1)
                }
            }

            Spacer(minLength: 4)

            Menu {
                Button {
                    withAnimation(.easeInOut(duration: 0.18)) {
                        isSearching.toggle()
                        if !isSearching { searchQuery = "" }
                    }
                } label: {
                    Label(isSearching ? String(localized: "关闭搜索") : String(localized: "搜索聊天记录"), systemImage: "magnifyingglass")
                }

                Button {
                    Task { await loadArchive(forceRefresh: true) }
                } label: {
                    Label(isRefreshing ? String(localized: "正在刷新...") : String(localized: "重新拉取"), systemImage: "arrow.clockwise")
                }
                .disabled(isRefreshing || isLoading)

                if let raw = archive?.rawText, !raw.isEmpty {
                    Button {
                        UIPasteboard.general.string = raw
                        UINotificationFeedbackGenerator().notificationOccurred(.success)
                        showToast(String(localized: "已复制全部聊天记录"))
                    } label: {
                        Label("复制全部文本", systemImage: "doc.on.doc")
                    }
                }

                Button {
                    Task { await downloadAndExportOriginalZip() }
                } label: {
                    Label(isDownloadingZip ? String(localized: "正在下载压缩包...") : String(localized: "保存原始 ZIP"), systemImage: "square.and.arrow.down")
                }
                .disabled(isDownloadingZip)

                Button {
                    dismiss()
                    onDismiss()
                    onFallbackToQuickLook()
                } label: {
                    Label("查看压缩包文件", systemImage: "doc.zipper")
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 18, weight: .medium))
                    .foregroundColor(textPrimary)
                    .frame(width: 36, height: 36)
                    .contentShape(Rectangle())
            }
        }
        .padding(.horizontal, 10)
        .padding(.top, 6)
        .padding(.bottom, 10)
        .background(headerBg.ignoresSafeArea(edges: .top))
    }

    private var searchBar: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 13.5))
                .foregroundColor(textMuted)
            TextField("搜索群成员或聊天内容", text: $searchQuery)
                .font(.system(size: 14))
                .foregroundColor(textPrimary)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            if !searchQuery.isEmpty {
                Button {
                    searchQuery = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.system(size: 14))
                        .foregroundColor(textMuted)
                }
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(listBg)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .padding(.horizontal, 12)
        .padding(.bottom, 8)
        .background(headerBg)
    }

    @ViewBuilder
    private func messageRow(_ msg: WeChatChatMessage) -> some View {
        VStack(spacing: 0) {
            HStack(alignment: .top, spacing: 11) {
                avatarView(msg)

                VStack(alignment: .leading, spacing: 4) {
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text(msg.sender)
                            .font(.system(size: 13))
                            .foregroundColor(textMuted)
                            .lineLimit(1)
                        Spacer(minLength: 8)
                        Text(msg.displayTime)
                            .font(.system(size: 11.5))
                            .foregroundColor(textMuted)
                            .lineLimit(1)
                    }

                    messageBody(msg)
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 12)

            Rectangle()
                .fill(dividerColor)
                .frame(height: 0.5)
                .padding(.leading, 63)
        }
        .contentShape(Rectangle())
        .contextMenu {
            let copyable = msg.text.isEmpty ? (msg.mediaName ?? "") : msg.text
            if !copyable.isEmpty {
                Button {
                    UIPasteboard.general.string = copyable
                    UINotificationFeedbackGenerator().notificationOccurred(.success)
                    showToast(String(localized: "已复制"))
                } label: {
                    Label("复制内容", systemImage: "doc.on.doc")
                }
            }
            Button {
                let block = "\(msg.sender)  \(msg.timestamp)\n\(copyable)"
                UIPasteboard.general.string = block
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                showToast(String(localized: "已复制含时间与发送者"))
            } label: {
                Label("复制完整记录", systemImage: "text.quote")
            }
        }
    }

    private func avatarView(_ msg: WeChatChatMessage) -> some View {
        let idx = abs(msg.colorIndex) % Self.avatarPalette.count
        return ZStack {
            RoundedRectangle(cornerRadius: 5, style: .continuous)
                .fill(Self.avatarPalette[idx])
                .frame(width: 38, height: 38)
            Text(msg.senderInitial)
                .font(.system(size: 16.5, weight: .semibold))
                .foregroundColor(.white)
        }
    }

    @ViewBuilder
    private func messageBody(_ msg: WeChatChatMessage) -> some View {
        switch msg.kind {
        case "image":
            if let entry = msg.mediaEntry {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    let galleryItems = allImageEntries
                    let initialIdx = galleryItems.firstIndex(where: { $0.msgId == msg.id }) ?? 0
                    if !galleryItems.isEmpty {
                        previewGallery = ImageGalleryData(
                            items: galleryItems.map { $0.image },
                            initialIndex: initialIdx
                        )
                    }
                } label: {
                    WeChatCachedImageView(
                        uri: item.uri,
                        cascadeId: cascadeId,
                        entry: entry,
                        apiClient: apiClient,
                        baseURL: baseURL,
                        refreshToken: mediaRefreshToken,
                        placeholderBg: cardBg,
                        failureLabel: msg.mediaName ?? String(localized: "图片加载失败"),
                        textMuted: textMuted
                    )
                }
                .buttonStyle(.plain)
            } else {
                missingImageBox(label: String(localized: "图片"))
            }

        case "video":
            if let baseURL, let entry = msg.mediaEntry,
               let videoURL = apiClient.zipEntryURL(uri: item.uri, entry: entry, cascadeId: cascadeId, baseURL: baseURL) {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    playingVideoURL = IdentifiableURL(url: videoURL)
                } label: {
                    ZStack(alignment: .bottomTrailing) {
                        ZStack {
                            WeChatCachedVideoThumbView(
                                uri: item.uri,
                                cascadeId: cascadeId,
                                entry: entry,
                                apiClient: apiClient,
                                baseURL: baseURL,
                                refreshToken: mediaRefreshToken
                            )
                            Circle()
                                .fill(Color.black.opacity(0.42))
                                .frame(width: 44, height: 44)
                                .overlay(
                                    Circle().stroke(Color.white.opacity(0.88), lineWidth: 1.5)
                                )
                            Image(systemName: "play.fill")
                                .font(.system(size: 18))
                                .foregroundColor(.white)
                                .offset(x: 1.5)
                        }
                        .frame(width: 170, height: 220)
                        .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))

                        if let dur = msg.durationLabel, !dur.isEmpty {
                            Text(dur)
                                .font(.system(size: 11, weight: .medium).monospacedDigit())
                                .foregroundColor(.white)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.black.opacity(0.55))
                                .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                                .padding(6)
                        }
                    }
                }
                .buttonStyle(.plain)
            } else {
                placeholderTag(icon: "video.slash", title: String(localized: "[视频]"), subtitle: String(localized: "未随记录导出"))
            }

        case "missing_image":
            missingImageBox(label: String(localized: "未随记录导出"))

        case "missing_video":
            placeholderTag(icon: "video.slash", title: String(localized: "[视频]"), subtitle: String(localized: "未随记录导出"))

        case "sticker":
            let label = msg.text.isEmpty ? String(localized: "[动画表情]") : String(localized: "[动画表情] \(msg.text)")
            Text(label)
                .font(.system(size: 14))
                .foregroundColor(textMuted)
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .background(cardBg)
                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))

        case "file":
            Button {
                if let entry = msg.mediaEntry {
                    Task { await openZipSubFile(entry: entry, fileName: msg.mediaName ?? "file") }
                }
            } label: {
                HStack(spacing: 10) {
                    FileTypeBadge(fileName: msg.mediaName ?? "file", size: 34)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(msg.mediaName ?? msg.text)
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(textPrimary)
                            .lineLimit(2)
                        if let sz = msg.mediaSize, sz > 0 {
                            Text(formatFileSize(Int64(sz)))
                                .font(.system(size: 11.5))
                                .foregroundColor(textMuted)
                        }
                    }
                }
                .padding(10)
                .background(cardBg)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .buttonStyle(.plain)

        default:
            Text(styledMessageText(msg.text))
                .font(.system(size: 15.5))
                .foregroundColor(textPrimary)
                .lineSpacing(3)
                .textSelection(.enabled)
        }
    }

    private func missingImageBox(label: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "photo")
                .font(.system(size: 18))
                .foregroundColor(textMuted)
            Text("[图片] \(label)")
                .font(.system(size: 13))
                .foregroundColor(textMuted)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(cardBg)
        .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
    }

    private func placeholderTag(icon: String, title: String, subtitle: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: icon)
                .font(.system(size: 13))
                .foregroundColor(textMuted)
            Text("\(title) \(subtitle)")
                .font(.system(size: 13))
                .foregroundColor(textMuted)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(cardBg)
        .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
    }

    private func styledMessageText(_ raw: String) -> AttributedString {
        var attr = AttributedString(raw)
        if let regex = try? NSRegularExpression(pattern: #"@[^\s\n]+"#) {
            let ns = raw as NSString
            for match in regex.matches(in: raw, range: NSRange(location: 0, length: ns.length)) {
                if let range = Range(match.range, in: raw),
                   let attrRange = Range(range, in: attr) {
                    attr[attrRange].foregroundColor = mentionBlue
                }
            }
        }
        return attr
    }

    private func cleanDisplayFileName(_ name: String) -> String {
        if name.count > 17 {
            let idx = name.index(name.startIndex, offsetBy: 16)
            let prefix = name[name.startIndex..<idx]
            if name[idx] == "-" && prefix.allSatisfy({ $0.isHexDigit && !$0.isUppercase }) {
                return String(name[name.index(after: idx)...])
            }
        }
        return name
    }

    private func loadArchive(forceRefresh: Bool = false) async {
        if !forceRefresh {
            if let cached = await WeChatArchiveCacheManager.shared.loadArchive(uri: item.uri, cascadeId: cascadeId) {
                archive = cached
                errorMessage = nil
                isLoading = false
                return
            }
            isLoading = true
            errorMessage = nil
        } else {
            if archive == nil {
                isLoading = true
                errorMessage = nil
            } else {
                isRefreshing = true
            }
        }

        defer { isRefreshing = false }

        guard let baseURL else {
            if archive == nil {
                isLoading = false
                errorMessage = String(localized: "未连接到网关服务器")
            } else {
                showToast(String(localized: "未连接到网关服务器"))
            }
            return
        }

        do {
            let res = try await apiClient.fetchWeChatChatArchive(uri: item.uri, cascadeId: cascadeId, baseURL: baseURL)
            if !res.isWeChatArchive {
                WeChatArchiveCacheManager.shared.invalidateArchive(uri: item.uri, cascadeId: cascadeId)
                dismiss()
                onDismiss()
                onFallbackToQuickLook()
                return
            }
            if forceRefresh {
                WeChatArchiveCacheManager.shared.invalidateMedia(uri: item.uri, cascadeId: cascadeId)
                mediaRefreshToken += 1
            }
            WeChatArchiveCacheManager.shared.saveArchive(res, uri: item.uri, cascadeId: cascadeId)
            archive = res
            errorMessage = nil
            isLoading = false
            if forceRefresh {
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                showToast(String(localized: "已刷新聊天记录"))
            }
        } catch {
            if archive != nil && forceRefresh {
                showToast(String(localized: "刷新失败：\(error.localizedDescription)"))
            } else {
                errorMessage = String(localized: "解析聊天记录失败：\(error.localizedDescription)")
                isLoading = false
            }
        }
    }

    private func openZipSubFile(entry: String, fileName: String) async {
        guard let baseURL,
              let url = apiClient.zipEntryURL(uri: item.uri, entry: entry, cascadeId: cascadeId, baseURL: baseURL) else { return }
        do {
            let (localURL, _) = try await apiClient.downloadFile(uri: url.absoluteString, cascadeId: cascadeId, baseURL: baseURL)
            previewEntryURL = IdentifiableURL(url: localURL)
        } catch {
            showToast(String(localized: "打开附件失败"))
        }
    }

    private func downloadAndExportOriginalZip() async {
        guard let baseURL else { return }
        isDownloadingZip = true
        defer { isDownloadingZip = false }
        do {
            let (localURL, _) = try await apiClient.downloadFile(uri: item.uri, cascadeId: cascadeId, baseURL: baseURL)
            downloadedZipURL = localURL
            showDocumentExporter = true
        } catch {
            showToast(String(localized: "下载原始 ZIP 失败"))
        }
    }

    private func showToast(_ msg: String) {
        withAnimation { toastMessage = msg }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
            if toastMessage == msg {
                withAnimation { toastMessage = nil }
            }
        }
    }
}

private struct WeChatCachedImageView: View {
    let uri: String
    let cascadeId: String
    let entry: String
    let apiClient: APIClient
    let baseURL: URL?
    let refreshToken: Int
    let placeholderBg: Color
    let failureLabel: String
    let textMuted: Color

    @State private var image: UIImage?
    @State private var hasFailed: Bool = false

    init(
        uri: String,
        cascadeId: String,
        entry: String,
        apiClient: APIClient,
        baseURL: URL?,
        refreshToken: Int,
        placeholderBg: Color,
        failureLabel: String,
        textMuted: Color
    ) {
        self.uri = uri
        self.cascadeId = cascadeId
        self.entry = entry
        self.apiClient = apiClient
        self.baseURL = baseURL
        self.refreshToken = refreshToken
        self.placeholderBg = placeholderBg
        self.failureLabel = failureLabel
        self.textMuted = textMuted
        let mem = WeChatArchiveCacheManager.shared.cachedMediaImageInMemory(
            uri: uri,
            cascadeId: cascadeId,
            entry: entry,
            thumb: false
        )
        _image = State(initialValue: mem)
    }

    private var taskKey: String { "\(uri)|\(entry)|\(refreshToken)" }

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: 190, maxHeight: 240, alignment: .leading)
                    .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
            } else if hasFailed {
                HStack(spacing: 8) {
                    Image(systemName: "photo")
                        .font(.system(size: 18))
                        .foregroundColor(textMuted)
                    Text("[图片] \(failureLabel)")
                        .font(.system(size: 13))
                        .foregroundColor(textMuted)
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 10)
                .background(placeholderBg)
                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
            } else {
                RoundedRectangle(cornerRadius: 4, style: .continuous)
                    .fill(placeholderBg)
                    .frame(width: 140, height: 140)
                    .overlay(ProgressView())
            }
        }
        .task(id: taskKey) {
            await loadImage()
        }
    }

    private func loadImage() async {
        hasFailed = false
        if let cached = await WeChatArchiveCacheManager.shared.loadMediaImage(
            uri: uri,
            cascadeId: cascadeId,
            entry: entry,
            thumb: false
        ) {
            image = cached
            return
        }
        guard let baseURL else {
            hasFailed = true
            return
        }
        do {
            let data = try await apiClient.fetchZipEntryData(
                uri: uri,
                entry: entry,
                cascadeId: cascadeId,
                baseURL: baseURL,
                thumb: false
            )
            guard !Task.isCancelled else { return }
            if let decoded = UIImage(data: data) {
                WeChatArchiveCacheManager.shared.saveMediaData(
                    data,
                    image: decoded,
                    uri: uri,
                    cascadeId: cascadeId,
                    entry: entry,
                    thumb: false
                )
                image = decoded
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

private struct WeChatCachedVideoThumbView: View {
    let uri: String
    let cascadeId: String
    let entry: String
    let apiClient: APIClient
    let baseURL: URL?
    let refreshToken: Int

    @State private var image: UIImage?

    init(
        uri: String,
        cascadeId: String,
        entry: String,
        apiClient: APIClient,
        baseURL: URL?,
        refreshToken: Int
    ) {
        self.uri = uri
        self.cascadeId = cascadeId
        self.entry = entry
        self.apiClient = apiClient
        self.baseURL = baseURL
        self.refreshToken = refreshToken
        let mem = WeChatArchiveCacheManager.shared.cachedMediaImageInMemory(
            uri: uri,
            cascadeId: cascadeId,
            entry: entry,
            thumb: true
        )
        _image = State(initialValue: mem)
    }

    private var taskKey: String { "\(uri)|\(entry)|thumb|\(refreshToken)" }

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
            } else {
                Color(hex: 0x1C1C1E)
            }
        }
        .task(id: taskKey) {
            await loadThumb()
        }
    }

    private func loadThumb() async {
        if let cached = await WeChatArchiveCacheManager.shared.loadMediaImage(
            uri: uri,
            cascadeId: cascadeId,
            entry: entry,
            thumb: true
        ) {
            image = cached
            return
        }
        guard let baseURL else { return }
        do {
            let data = try await apiClient.fetchZipEntryData(
                uri: uri,
                entry: entry,
                cascadeId: cascadeId,
                baseURL: baseURL,
                thumb: true
            )
            guard !Task.isCancelled else { return }
            if let decoded = UIImage(data: data) {
                WeChatArchiveCacheManager.shared.saveMediaData(
                    data,
                    image: decoded,
                    uri: uri,
                    cascadeId: cascadeId,
                    entry: entry,
                    thumb: true
                )
                image = decoded
            }
        } catch {
            // Keep dark fallback background
        }
    }
}

private struct WeChatVideoPlayerOverlay: View {
    let url: URL
    let onClose: () -> Void
    @State private var player: AVPlayer? = nil

    var body: some View {
        ZStack(alignment: .topLeading) {
            Color.black.ignoresSafeArea()
            if let player {
                VideoPlayer(player: player)
                    .ignoresSafeArea()
            } else {
                ProgressView()
                    .tint(.white)
            }
            Button {
                player?.pause()
                onClose()
            } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.white)
                    .frame(width: 36, height: 36)
                    .background(Color.black.opacity(0.55))
                    .clipShape(Circle())
            }
            .padding(.top, 16)
            .padding(.leading, 16)
        }
        .onAppear {
            let p = AVPlayer(url: url)
            player = p
            p.play()
        }
        .onDisappear {
            player?.pause()
            player = nil
        }
    }
}

private extension Color {
    init(hex: UInt32) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255.0,
            green: Double((hex >> 8) & 0xFF) / 255.0,
            blue: Double(hex & 0xFF) / 255.0,
            opacity: 1.0
        )
    }
}
