import SwiftUI
import Combine
import Photos
import PhotosUI
import UIKit
import UniformTypeIdentifiers
import VisionKit

/// Observes the photo library and exposes the most recent photos for the "+" panel grid.
@MainActor
final class RecentPhotosModel: NSObject, ObservableObject, PHPhotoLibraryChangeObserver {
    @Published var authorization: PHAuthorizationStatus = PHPhotoLibrary.authorizationStatus(for: .readWrite)
    @Published var assets: [PHAsset] = []
    
    static let limit = 150
    private var result: PHFetchResult<PHAsset>?
    
    override init() {
        super.init()
        PHPhotoLibrary.shared().register(self)
    }
    
    deinit {
        PHPhotoLibrary.shared().unregisterChangeObserver(self)
    }
    
    var hasAccess: Bool { authorization == .authorized || authorization == .limited }
    
    func start() {
        if authorization == .notDetermined {
            PHPhotoLibrary.requestAuthorization(for: .readWrite) { [weak self] status in
                Task { @MainActor in
                    self?.authorization = status
                    self?.reload()
                }
            }
        } else {
            reload()
        }
    }
    
    func reload() {
        authorization = PHPhotoLibrary.authorizationStatus(for: .readWrite)
        guard hasAccess else { assets = []; return }
        let options = PHFetchOptions()
        options.sortDescriptors = [NSSortDescriptor(key: "creationDate", ascending: false)]
        options.fetchLimit = Self.limit
        let fetched = PHAsset.fetchAssets(with: options)
        result = fetched
        var list: [PHAsset] = []
        fetched.enumerateObjects { asset, _, _ in list.append(asset) }
        assets = list
    }
    
    nonisolated func photoLibraryDidChange(_ changeInstance: PHChange) {
        Task { @MainActor in self.reload() }
    }
}

/// Square thumbnail of a photo-library asset.
private struct PhotoThumbnail: View {
    let asset: PHAsset
    let manager: PHCachingImageManager
    @State private var image: UIImage?
    
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
            .onAppear { load() }
    }
    
    private func load() {
        guard image == nil else { return }
        let scale = UIScreen.main.scale
        let targetSide = 120 * scale
        let target = CGSize(width: targetSide, height: targetSide)
        let options = PHImageRequestOptions()
        options.deliveryMode = .opportunistic
        options.resizeMode = .fast
        options.isNetworkAccessAllowed = true
        manager.requestImage(for: asset, targetSize: target, contentMode: .aspectFill, options: options) { img, _ in
            if let img {
                DispatchQueue.main.async { image = img }
            }
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
                .overlay(alignment: .bottomLeading) {
                    if asset.mediaType == .video {
                        let sec = Int(asset.duration)
                        let text = String(format: "%d:%02d", sec / 60, sec % 60)
                        HStack(spacing: 3) {
                            Image(systemName: "video.fill")
                                .font(.system(size: 8))
                            Text(text)
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                        }
                        .foregroundColor(.white)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 2)
                        .background(Color.black.opacity(0.6))
                        .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        .padding(5)
                    }
                }
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
}

/// Full-height pull-up sheet behind the chat "+" button: a camera tile followed by the recent
/// photos (multi-select), with a pinned "添加文件" row at the bottom. Presented as a large sheet
/// with the standard drag indicator, like the other sheets in the app.
struct AttachmentPickerSheet: View {
    let remainingImageSlots: Int
    /// Called with the selected photos (in selection order); the sheet dismisses itself first.
    let onPhotosPicked: ([PHAsset]) -> Void
    let onOpenCamera: () -> Void
    let onOpenDocumentScanner: () -> Void
    let onFilesPicked: ([URL]) -> Void
    
    @Environment(\.dismiss) private var dismiss
    @StateObject private var photos = RecentPhotosModel()
    @State private var selected: [String] = []   // PHAsset.localIdentifier, in selection order
    @State private var showFileImporter = false
    private let imageManager = PHCachingImageManager()
    
    private let columns = Array(repeating: GridItem(.flexible(), spacing: 6), count: 4)
    
    var body: some View {
        // Same structure as NewConversationSheet (NavigationStack + inline title) so the title
        // sits at the same distance from the drag indicator.
        NavigationStack {
            content
                .navigationTitle("最近项目")
                .navigationBarTitleDisplayMode(.inline)
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
    
    private var content: some View {
        VStack(spacing: 0) {
            ScrollView {
                if photos.authorization == .limited {
                    limitedBanner
                }
                LazyVGrid(columns: columns, spacing: 6) {
                    fileTile
                    cameraTile
                    if VNDocumentCameraViewController.isSupported {
                        documentScannerTile
                    }
                    if !photos.hasAccess {
                        permissionTile
                    }
                    ForEach(photos.assets, id: \.localIdentifier) { asset in
                        let order = selected.firstIndex(of: asset.localIdentifier)
                        PhotoGridCell(
                            asset: asset,
                            isSelected: order != nil,
                            selectionOrder: order,
                            imageManager: imageManager
                        ) {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            if let order {
                                selected.remove(at: order)
                            } else if selected.count < remainingImageSlots {
                                selected.append(asset.localIdentifier)
                            } else {
                                UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
                            }
                        }
                    }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
            }
        }
        .background(Color(uiColor: .systemBackground))
        // The confirm button floats centered over the grid once photos are selected.
        .overlay(alignment: .bottom) {
            if !selected.isEmpty {
                footer
                    .padding(.bottom, 12)
                    .transition(.scale(scale: 0.9).combined(with: .opacity))
            }
        }
        .animation(.easeOut(duration: 0.18), value: selected.isEmpty)
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
                Text("扫描文稿")
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
        Button {
            if photos.authorization == .notDetermined {
                photos.start()
            } else if let url = URL(string: UIApplication.openSettingsURLString) {
                UIApplication.shared.open(url)
            }
        } label: {
            VStack(spacing: 4) {
                Image(systemName: "photo.on.rectangle")
                    .font(.system(size: 22))
                Text("允许访问照片")
                    .font(.system(size: 11))
                    .multilineTextAlignment(.center)
            }
            .foregroundColor(.secondary)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color(uiColor: .secondarySystemBackground))
            .aspectRatio(1, contentMode: .fit)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
        .buttonStyle(.plain)
    }
    
    private var footer: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            let byId = Dictionary(uniqueKeysWithValues: photos.assets.map { ($0.localIdentifier, $0) })
            let picked = selected.compactMap { byId[$0] }
            dismiss()
            onPhotosPicked(picked)
        } label: {
            Text("添加 (\(selected.count))")
                .font(.system(size: 16, weight: .semibold))
                .foregroundStyle(.white)
                .padding(.horizontal, 32)
                .padding(.vertical, 13)
                .glassEffect(.regular.tint(.blue).interactive(), in: .capsule)
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
