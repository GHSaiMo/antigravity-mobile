import SwiftUI
import Combine
import Photos
import PhotosUI
import UIKit
import UniformTypeIdentifiers
import VisionKit

/// Represents a selectable photo album (smart album or user album) in the attachment picker.
struct PhotoAlbumItem: Identifiable, Equatable {
    nonisolated static let recentsFallbackId = "__all_photos__"

    let id: String
    let title: String
    let collection: PHAssetCollection?
    let count: Int
    let coverAsset: PHAsset?
    let isRecents: Bool

    static func == (lhs: PhotoAlbumItem, rhs: PhotoAlbumItem) -> Bool {
        lhs.id == rhs.id &&
        lhs.title == rhs.title &&
        lhs.count == rhs.count &&
        lhs.coverAsset?.localIdentifier == rhs.coverAsset?.localIdentifier &&
        lhs.isRecents == rhs.isRecents
    }
}

/// Observes the photo library, enumerates albums (image-only), and exposes a lazy PHFetchResult
/// so large photo libraries scroll smoothly without an artificial item cap.
@MainActor
final class RecentPhotosModel: NSObject, ObservableObject, PHPhotoLibraryChangeObserver {
    @Published var authorization: PHAuthorizationStatus = PHPhotoLibrary.authorizationStatus(for: .readWrite)
    @Published var albums: [PhotoAlbumItem] = []
    @Published var selectedAlbumId: String = PhotoAlbumItem.recentsFallbackId
    @Published var currentFetchResult: PHFetchResult<PHAsset>? = nil
    @Published var reloadToken: UUID = UUID()

    private var albumLoadGeneration: Int = 0

    override init() {
        super.init()
        PHPhotoLibrary.shared().register(self)
    }

    deinit {
        PHPhotoLibrary.shared().unregisterChangeObserver(self)
    }

    var hasAccess: Bool { authorization == .authorized || authorization == .limited }

    var currentAlbum: PhotoAlbumItem? {
        albums.first(where: { $0.id == selectedAlbumId }) ?? albums.first
    }

    var isShowingRecents: Bool {
        currentAlbum?.isRecents ?? true
    }

    var currentAlbumTitle: String {
        currentAlbum?.title ?? String(localized: "最近项目")
    }

    var assetCount: Int {
        currentFetchResult?.count ?? 0
    }

    func asset(at index: Int) -> PHAsset? {
        guard let result = currentFetchResult, index >= 0, index < result.count else { return nil }
        return result.object(at: index)
    }

    nonisolated static func imageFetchOptions() -> PHFetchOptions {
        let options = PHFetchOptions()
        options.predicate = NSPredicate(format: "mediaType == %d", PHAssetMediaType.image.rawValue)
        options.sortDescriptors = [NSSortDescriptor(key: "creationDate", ascending: false)]
        return options
    }

    /// 仅在已获得相册权限时静默加载最近照片；未授权时不自动弹窗，避免打断只想用文件/录音/拍照的用户。
    func start() {
        authorization = PHPhotoLibrary.authorizationStatus(for: .readWrite)
        if hasAccess {
            reload()
        }
    }

    func requestAccess() {
        Task { @MainActor in
            let status = await PHPhotoLibrary.requestAuthorization(for: .readWrite)
            self.authorization = status
            self.reload()
        }
    }

    func selectAlbum(_ album: PhotoAlbumItem) {
        guard hasAccess else { return }
        selectedAlbumId = album.id
        fetchAssetsForCurrentSelection(preferredCollection: album.collection)
    }

    func reload() {
        authorization = PHPhotoLibrary.authorizationStatus(for: .readWrite)
        guard hasAccess else {
            albums = []
            currentFetchResult = nil
            selectedAlbumId = PhotoAlbumItem.recentsFallbackId
            reloadToken = UUID()
            return
        }

        // 1. 立即加载当前选中相册（或全部图片）的照片结果，保证首屏秒开
        fetchAssetsForCurrentSelection(preferredCollection: currentAlbum?.collection)

        // 2. 后台扫描全部智能相簿与用户自定义相簿
        albumLoadGeneration += 1
        let gen = albumLoadGeneration
        let previouslySelectedId = selectedAlbumId
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            let discovered = Self.discoverImageAlbums()
            DispatchQueue.main.async {
                guard let self, self.albumLoadGeneration == gen else { return }
                self.albums = discovered
                if let existing = discovered.first(where: { $0.id == previouslySelectedId }) {
                    self.selectedAlbumId = existing.id
                    self.fetchAssetsForCurrentSelection(preferredCollection: existing.collection)
                } else if let recents = discovered.first {
                    self.selectedAlbumId = recents.id
                    self.fetchAssetsForCurrentSelection(preferredCollection: recents.collection)
                }
            }
        }
    }

    private func fetchAssetsForCurrentSelection(preferredCollection: PHAssetCollection?) {
        let options = Self.imageFetchOptions()
        if let collection = preferredCollection {
            currentFetchResult = PHAsset.fetchAssets(in: collection, options: options)
        } else {
            currentFetchResult = PHAsset.fetchAssets(with: .image, options: options)
        }
        reloadToken = UUID()
    }

    private nonisolated static func discoverImageAlbums() -> [PhotoAlbumItem] {
        let imageOptions = imageFetchOptions()
        var smartItems: [PhotoAlbumItem] = []
        var seenCollectionIDs: Set<String> = []
        var hasRecents = false

        let smartSubtypes: [(PHAssetCollectionSubtype, Bool)] = [
            (.smartAlbumUserLibrary, true),
            (.smartAlbumFavorites, false),
            (.smartAlbumScreenshots, false),
            (.smartAlbumSelfPortraits, false),
            (.smartAlbumLivePhotos, false),
            (.smartAlbumPanoramas, false)
        ]

        for (subtype, isRecents) in smartSubtypes {
            let fetched = PHAssetCollection.fetchAssetCollections(with: .smartAlbum, subtype: subtype, options: nil)
            fetched.enumerateObjects { collection, _, _ in
                guard seenCollectionIDs.insert(collection.localIdentifier).inserted else { return }
                let assets = PHAsset.fetchAssets(in: collection, options: imageOptions)
                if assets.count == 0 && !isRecents { return }
                let title: String
                if isRecents {
                    hasRecents = true
                    title = String(localized: "最近项目")
                } else {
                    title = collection.localizedTitle?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
                        ? (collection.localizedTitle ?? "")
                        : String(localized: "未命名相簿")
                }
                smartItems.append(PhotoAlbumItem(
                    id: collection.localIdentifier,
                    title: title,
                    collection: collection,
                    count: assets.count,
                    coverAsset: assets.firstObject,
                    isRecents: isRecents
                ))
            }
        }

        if !hasRecents {
            let allImages = PHAsset.fetchAssets(with: .image, options: imageOptions)
            smartItems.insert(PhotoAlbumItem(
                id: PhotoAlbumItem.recentsFallbackId,
                title: String(localized: "最近项目"),
                collection: nil,
                count: allImages.count,
                coverAsset: allImages.firstObject,
                isRecents: true
            ), at: 0)
        }

        // 用户自定义相簿 + 同步相簿，按最新照片时间倒序
        var userEntries: [(item: PhotoAlbumItem, latestDate: Date)] = []
        let userSubtypes: [PHAssetCollectionSubtype] = [.albumRegular, .albumSyncedAlbum, .albumImported]
        for subtype in userSubtypes {
            let fetched = PHAssetCollection.fetchAssetCollections(with: .album, subtype: subtype, options: nil)
            fetched.enumerateObjects { collection, _, _ in
                guard seenCollectionIDs.insert(collection.localIdentifier).inserted else { return }
                let assets = PHAsset.fetchAssets(in: collection, options: imageOptions)
                guard assets.count > 0 else { return }
                let title = collection.localizedTitle?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
                    ? (collection.localizedTitle ?? "")
                    : String(localized: "未命名相簿")
                let cover = assets.firstObject
                let item = PhotoAlbumItem(
                    id: collection.localIdentifier,
                    title: title,
                    collection: collection,
                    count: assets.count,
                    coverAsset: cover,
                    isRecents: false
                )
                userEntries.append((item, cover?.creationDate ?? .distantPast))
            }
        }

        userEntries.sort { a, b in
            if a.latestDate != b.latestDate {
                return a.latestDate > b.latestDate
            }
            return a.item.title.localizedCaseInsensitiveCompare(b.item.title) == .orderedAscending
        }

        return smartItems + userEntries.map(\.item)
    }

    nonisolated func photoLibraryDidChange(_ changeInstance: PHChange) {
        Task { @MainActor in self.reload() }
    }
}

/// Square thumbnail of a photo-library asset with request cancellation for fast scrolling.
private struct PhotoThumbnail: View {
    let asset: PHAsset
    let manager: PHCachingImageManager
    var targetPoints: CGFloat = 120

    @Environment(\.displayScale) private var displayScale
    @State private var image: UIImage?
    @State private var loadedAssetId: String?
    @State private var requestID: PHImageRequestID = PHInvalidImageRequestID

    var body: some View {
        Color(uiColor: .secondarySystemBackground)
            .aspectRatio(1, contentMode: .fit)
            .overlay {
                if let image {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFill()
                }
            }
            .clipped()
            .contentShape(Rectangle())
            .task(id: asset.localIdentifier) {
                load()
            }
            .onDisappear {
                cancelPending()
            }
    }

    private func load() {
        let currentId = asset.localIdentifier
        if loadedAssetId == currentId && image != nil { return }
        cancelPending()
        if loadedAssetId != currentId {
            image = nil
            loadedAssetId = currentId
        }
        let targetSide = targetPoints * max(displayScale, 1)
        let target = CGSize(width: targetSide, height: targetSide)
        let options = PHImageRequestOptions()
        options.deliveryMode = .opportunistic
        options.resizeMode = .fast
        options.isNetworkAccessAllowed = true
        requestID = manager.requestImage(for: asset, targetSize: target, contentMode: .aspectFill, options: options) { img, _ in
            guard let img else { return }
            DispatchQueue.main.async {
                if loadedAssetId == currentId {
                    image = img
                }
            }
        }
    }

    private func cancelPending() {
        if requestID != PHInvalidImageRequestID {
            manager.cancelImageRequest(requestID)
            requestID = PHInvalidImageRequestID
        }
    }
}

/// A single cell in the photo grid with selection badge and clean touch boundaries.
private struct PhotoGridCell: View {
    let asset: PHAsset
    let isSelected: Bool
    let selectionOrder: Int?
    let imageManager: PHCachingImageManager
    let onToggle: () -> Void

    var body: some View {
        Button(action: onToggle) {
            PhotoThumbnail(asset: asset, manager: imageManager)
                .overlay(Color.black.opacity(isSelected ? 0.25 : 0))
                .overlay(alignment: .topTrailing) {
                    ZStack {
                        if let selectionOrder {
                            Circle()
                                .fill(Color.indigo)
                            Circle()
                                .stroke(Color.white, lineWidth: 1.5)
                            Text("\(selectionOrder + 1)")
                                .font(.system(size: 12, weight: .bold))
                                .foregroundColor(.white)
                        } else {
                            Circle()
                                .fill(Color.black.opacity(0.35))
                            Circle()
                                .stroke(Color.white.opacity(0.9), lineWidth: 1.5)
                        }
                    }
                    .frame(width: 22, height: 22)
                    .shadow(color: Color.black.opacity(0.35), radius: 2, x: 0, y: 1)
                    .padding(6)
                }
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
}

/// Full-height pull-up sheet behind the chat "+" button: quick action tiles in Recents,
/// WeChat-style album switcher in the navigation title, and lazy image-only photo grid
/// with cross-album multi-selection.
struct AttachmentPickerSheet: View {
    let remainingImageSlots: Int
    /// Called with the selected photos (in selection order); the sheet dismisses itself first.
    let onPhotosPicked: ([PHAsset]) -> Void
    let onOpenAudioRecorder: () -> Void
    let onOpenCamera: () -> Void
    let onOpenDocumentScanner: () -> Void
    let onFilesPicked: ([URL]) -> Void

    @Environment(\.dismiss) private var dismiss
    @StateObject private var photos = RecentPhotosModel()
    @State private var selected: [String] = []                // PHAsset.localIdentifier, in selection order
    @State private var selectedAssetMap: [String: PHAsset] = [:] // Preserves PHAsset references across album switches
    @State private var showAlbumDrawer = false
    @State private var showFileImporter = false
    private let imageManager = PHCachingImageManager()

    private let columns = Array(repeating: GridItem(.flexible(), spacing: 6), count: 4)
    private let gridTopAnchorId = "attachment_grid_top"

    var body: some View {
        // Same structure as NewConversationSheet (NavigationStack + inline title) so the title
        // sits at the same distance from the drag indicator.
        NavigationStack {
            content
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .principal) {
                        albumTitleTrigger
                    }
                }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .onAppear { photos.start() }
        .fileImporter(
            isPresented: $showFileImporter,
            allowedContentTypes: [.item],
            allowsMultipleSelection: true
        ) { result in
            if case .success(let urls) = result, !urls.isEmpty {
                dismiss()
                onFilesPicked(urls)
            }
        }
    }

    @ViewBuilder
    private var albumTitleTrigger: some View {
        if photos.hasAccess && photos.albums.count > 1 {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.spring(response: 0.32, dampingFraction: 0.86)) {
                    showAlbumDrawer.toggle()
                }
            } label: {
                HStack(spacing: 5) {
                    Text(photos.currentAlbumTitle)
                        .font(.system(size: 16.5, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    Image(systemName: "chevron.down")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(showAlbumDrawer ? 180 : 0))
                        .animation(.spring(response: 0.28, dampingFraction: 0.82), value: showAlbumDrawer)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 5)
                .contentShape(Capsule())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(String(localized: "切换相册，当前：\(photos.currentAlbumTitle)"))
        } else {
            Text(photos.currentAlbumTitle)
                .font(.system(size: 17, weight: .semibold))
                .foregroundColor(.primary)
                .lineLimit(1)
        }
    }

    private var content: some View {
        let _ = photos.reloadToken
        return GeometryReader { geo in
            ScrollViewReader { proxy in
                ZStack(alignment: .top) {
                    ScrollView {
                        Color.clear
                            .frame(height: 0)
                            .id(gridTopAnchorId)

                        if photos.authorization == .limited {
                            limitedBanner
                        }

                        if photos.hasAccess && photos.assetCount == 0 && !photos.isShowingRecents {
                            VStack(spacing: 10) {
                                Image(systemName: "photo.on.rectangle.angled")
                                    .font(.system(size: 32))
                                    .foregroundColor(.secondary)
                                Text("该相册暂无照片")
                                    .font(.system(size: 14, weight: .medium))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity)
                            .padding(.top, 80)
                        } else {
                            LazyVGrid(columns: columns, spacing: 6) {
                                if photos.isShowingRecents {
                                    fileTile
                                    recordAudioTile
                                    if VNDocumentCameraViewController.isSupported {
                                        documentScannerTile
                                    }
                                    cameraTile
                                }
                                if !photos.hasAccess {
                                    permissionTile
                                } else {
                                    ForEach(0..<photos.assetCount, id: \.self) { index in
                                        if let asset = photos.asset(at: index) {
                                            let order = selected.firstIndex(of: asset.localIdentifier)
                                            PhotoGridCell(
                                                asset: asset,
                                                isSelected: order != nil,
                                                selectionOrder: order,
                                                imageManager: imageManager
                                            ) {
                                                toggleSelection(for: asset)
                                            }
                                            .id("\(photos.selectedAlbumId)_\(asset.localIdentifier)")
                                        }
                                    }
                                }
                            }
                            .padding(.horizontal, 12)
                            .padding(.top, 8)
                            .padding(.bottom, selected.isEmpty ? 16 : 84)
                        }
                    }
                    .background(Color(uiColor: .systemBackground))

                    // WeChat-style slide-down album drawer
                    if showAlbumDrawer {
                        Color.black.opacity(0.32)
                            .ignoresSafeArea()
                            .transition(.opacity)
                            .onTapGesture {
                                withAnimation(.spring(response: 0.3, dampingFraction: 0.88)) {
                                    showAlbumDrawer = false
                                }
                            }
                            .zIndex(1)

                        albumDrawerList(maxHeight: min(geo.size.height * 0.66, 520)) { album in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            photos.selectAlbum(album)
                            withAnimation(.spring(response: 0.3, dampingFraction: 0.88)) {
                                showAlbumDrawer = false
                            }
                            withAnimation(.easeOut(duration: 0.18)) {
                                proxy.scrollTo(gridTopAnchorId, anchor: .top)
                            }
                        }
                        .transition(.move(edge: .top).combined(with: .opacity))
                        .zIndex(2)
                    }
                }
                .clipped()
                // The confirm button floats centered over the grid once photos are selected.
                .overlay(alignment: .bottom) {
                    if !selected.isEmpty && !showAlbumDrawer {
                        footer
                            .padding(.bottom, 12)
                            .transition(.scale(scale: 0.9).combined(with: .opacity))
                    }
                }
                .animation(.easeOut(duration: 0.18), value: selected.isEmpty || showAlbumDrawer)
            }
        }
        .background(Color(uiColor: .systemBackground))
    }

    private func toggleSelection(for asset: PHAsset) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        if let order = selected.firstIndex(of: asset.localIdentifier) {
            selected.remove(at: order)
            selectedAssetMap.removeValue(forKey: asset.localIdentifier)
        } else if selected.count < remainingImageSlots {
            selected.append(asset.localIdentifier)
            selectedAssetMap[asset.localIdentifier] = asset
        } else {
            UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
        }
    }

    private func albumDrawerList(
        maxHeight: CGFloat,
        onSelect: @escaping (PhotoAlbumItem) -> Void
    ) -> some View {
        ScrollView {
            LazyVStack(spacing: 0) {
                ForEach(Array(photos.albums.enumerated()), id: \.element.id) { idx, album in
                    Button {
                        onSelect(album)
                    } label: {
                        HStack(spacing: 12) {
                            Group {
                                if let cover = album.coverAsset {
                                    PhotoThumbnail(asset: cover, manager: imageManager, targetPoints: 60)
                                } else {
                                    ZStack {
                                        Color(uiColor: .secondarySystemBackground)
                                        Image(systemName: "photo.on.rectangle")
                                            .font(.system(size: 18))
                                            .foregroundColor(.secondary)
                                    }
                                }
                            }
                            .frame(width: 52, height: 52)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

                            VStack(alignment: .leading, spacing: 3) {
                                Text(album.title)
                                    .font(.system(size: 15.5, weight: .medium))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                                Text("\(album.count)")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                            }

                            Spacer(minLength: 8)

                            if album.id == photos.selectedAlbumId {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundColor(.indigo)
                            }
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 9)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)

                    if idx < photos.albums.count - 1 {
                        Divider()
                            .padding(.leading, 80)
                    }
                }
            }
            .padding(.vertical, 6)
        }
        .frame(maxHeight: maxHeight)
        .background(Color(uiColor: .systemBackground))
        .clipShape(
            UnevenRoundedRectangle(
                topLeadingRadius: 0,
                bottomLeadingRadius: 18,
                bottomTrailingRadius: 18,
                topTrailingRadius: 0,
                style: .continuous
            )
        )
        .shadow(color: Color.black.opacity(0.14), radius: 14, x: 0, y: 6)
    }
    
    private var limitedBanner: some View {
        HStack {
            Text("已允许访问部分照片")
                .font(.system(size: 13))
                .foregroundColor(.secondary)
            Spacer()
            Button("管理") {
                if let vc = Self.topViewController() {
                    PHPhotoLibrary.shared().presentLimitedLibraryPicker(from: vc)
                }
            }
            .font(.system(size: 13, weight: .semibold))
            .tint(.indigo)
        }
        .padding(.horizontal, 16)
        .padding(.top, 4)
    }
    
    private var fileTile: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            showFileImporter = true
        } label: {
            VStack(spacing: 4) {
                Image(systemName: "paperclip")
                    .font(.system(size: 26, weight: .regular))
                    .frame(height: 32)
                Text("文件")
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.primary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(uiColor: .secondarySystemBackground))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
    
    private var recordAudioTile: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            dismiss()
            onOpenAudioRecorder()
        } label: {
            VStack(spacing: 4) {
                Image(systemName: "mic")
                    .font(.system(size: 26, weight: .regular))
                    .frame(height: 32)
                Text("录音")
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.primary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(uiColor: .secondarySystemBackground))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
    
    private var documentScannerTile: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            dismiss()
            onOpenDocumentScanner()
        } label: {
            VStack(spacing: 4) {
                Image(systemName: "doc.viewfinder")
                    .font(.system(size: 26, weight: .regular))
                    .frame(height: 32)
                Text("文稿")
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.primary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(uiColor: .secondarySystemBackground))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
    
    private var cameraTile: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            dismiss()
            onOpenCamera()
        } label: {
            VStack(spacing: 4) {
                Image(systemName: "camera")
                    .font(.system(size: 26, weight: .regular))
                    .frame(height: 32)
                Text("相机")
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.primary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(uiColor: .secondarySystemBackground))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
    
    private var permissionTile: some View {
        let isDenied = photos.authorization == .denied || photos.authorization == .restricted
        return Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            if photos.authorization == .notDetermined {
                photos.requestAccess()
            } else if let url = URL(string: UIApplication.openSettingsURLString) {
                UIApplication.shared.open(url)
            }
        } label: {
            VStack(spacing: 4) {
                Image(systemName: isDenied ? "gearshape" : "photo.on.rectangle")
                    .font(.system(size: 22))
                Text(isDenied ? String(localized: "开启相册权限") : String(localized: "访问最近照片"))
                    .font(.system(size: 11, weight: .medium))
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 4)
            }
            .foregroundColor(.indigo)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.indigo.opacity(0.08))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(Color.indigo.opacity(0.22), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
        .accessibilityLabel(isDenied ? String(localized: "前往系统设置开启相册权限") : String(localized: "授权访问相册以选择最近照片"))
    }
    
    private var footer: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            let picked = selected.compactMap { selectedAssetMap[$0] }
            dismiss()
            onPhotosPicked(picked)
        } label: {
            Text("添加 (\(selected.count))")
                .font(.system(size: 16, weight: .semibold))
                .foregroundStyle(.white)
                .padding(.horizontal, 32)
                .padding(.vertical, 13)
                .glassEffect(.regular.tint(.indigo).interactive(), in: .capsule)
        }
        .buttonStyle(.plain)
    }
    
    private static func topViewController() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let scene = scenes.first(where: { $0.activationState == .foregroundActive }) ?? scenes.first
        var top = scene?.windows.first(where: { $0.isKeyWindow })?.rootViewController
        while let presented = top?.presentedViewController { top = presented }
        return top
    }
}

/// Loads full-size images for the photos selected in `AttachmentPickerSheet`.
enum PhotoAssetLoader {
    static func loadImages(_ assets: [PHAsset], maxDimension: CGFloat = 1600) async -> [UIImage] {
        var images: [UIImage] = []
        for asset in assets {
            if let img = await loadImage(asset, maxDimension: maxDimension) {
                images.append(img)
            }
        }
        return images
    }
    
    private static func loadImage(_ asset: PHAsset, maxDimension: CGFloat) async -> UIImage? {
        await withCheckedContinuation { cont in
            var hasResumed = false
            let options = PHImageRequestOptions()
            options.deliveryMode = .highQualityFormat   // single callback
            options.resizeMode = .exact
            options.isNetworkAccessAllowed = true
            let scale = min(1, maxDimension / CGFloat(max(asset.pixelWidth, asset.pixelHeight, 1)))
            let target = CGSize(width: CGFloat(asset.pixelWidth) * scale, height: CGFloat(asset.pixelHeight) * scale)
            PHImageManager.default().requestImage(for: asset, targetSize: target, contentMode: .aspectFit, options: options) { img, _ in
                if !hasResumed {
                    hasResumed = true
                    cont.resume(returning: img)
                }
            }
        }
    }
}

// MARK: - Native Thin Glass Button Style (原生薄玻璃按钮样式)

public struct NativeThinGlassButtonStyle: ButtonStyle {
    public var tintColor: Color
    
    public init(tintColor: Color = .indigo) {
        self.tintColor = tintColor
    }
    
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(
                ZStack {
                    Capsule()
                        .fill(.thinMaterial)
                    Capsule()
                        .fill(tintColor.opacity(configuration.isPressed ? 0.20 : 0.08))
                }
            )
            .overlay(
                Capsule()
                    .stroke(
                        tintColor.opacity(configuration.isPressed ? 0.35 : 0.22),
                        lineWidth: 0.8
                    )
            )
            .overlay(
                Capsule()
                    .stroke(Color.white.opacity(0.35), lineWidth: 0.5)
            )
            .shadow(
                color: tintColor.opacity(configuration.isPressed ? 0.04 : 0.12),
                radius: configuration.isPressed ? 2 : 6,
                x: 0,
                y: configuration.isPressed ? 1 : 2.5
            )
            .scaleEffect(configuration.isPressed ? 0.98 : 1.0)
            .opacity(configuration.isPressed ? 0.88 : 1.0)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}
