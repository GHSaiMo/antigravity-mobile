import SwiftUI
import Photos

/// 图片「保存 / 分享 / 拷贝」的统一实现，供聊天流 contextMenu、全屏预览器与长图卡片共用。
enum ImageActions {
    enum SaveOutcome {
        case saved
        case denied
        case failed(String)
    }

    // MARK: - Loading

    /// 取得可操作的 UIImage：优先内存里的图，其次共享缓存，最后走网络（带 URLCache）。
    static func loadImage(_ item: IdentifiableImage) async -> UIImage? {
        if let image = item.image { return image }
        guard let url = item.url else { return nil }
        return await loadImage(url: url)
    }

    static func loadImage(url: URL) async -> UIImage? {
        if let cached = MarkdownImageCache.shared.image(for: url) { return cached }
        if url.isFileURL {
            return UIImage(contentsOfFile: url.path)
        }
        var request = URLRequest(url: url)
        request.cachePolicy = .returnCacheDataElseLoad
        request.timeoutInterval = 15
        guard let (data, response) = try? await URLSession.shared.data(for: request) else { return nil }
        if let http = response as? HTTPURLResponse, http.statusCode >= 400 { return nil }
        guard let image = UIImage(data: data) else { return nil }
        MarkdownImageCache.shared.insert(image, for: url)
        return image
    }

    // MARK: - Save

    static func saveToPhotos(_ image: UIImage, fileURL: URL? = nil) async -> SaveOutcome {
        let status = await PHPhotoLibrary.requestAuthorization(for: .addOnly)
        guard status == .authorized || status == .limited else { return .denied }
        do {
            try await PHPhotoLibrary.shared().performChanges {
                if let fileURL, fileURL.isFileURL, FileManager.default.fileExists(atPath: fileURL.path) {
                    PHAssetCreationRequest.forAsset().addResource(with: .photo, fileURL: fileURL, options: nil)
                } else {
                    PHAssetChangeRequest.creationRequestForAsset(from: image)
                }
            }
            return .saved
        } catch {
            return .failed(error.localizedDescription)
        }
    }

    /// SwiftUI 场景：保存并用 HUD + 触觉反馈提示结果。
    @MainActor
    static func saveWithFeedback(_ image: UIImage, fileURL: URL? = nil) async {
        switch await saveToPhotos(image, fileURL: fileURL) {
        case .saved:
            UINotificationFeedbackGenerator().notificationOccurred(.success)
            CopiedHUD.show("已保存到相册")
        case .denied:
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            presentPermissionAlert()
        case .failed(let message):
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            CopiedHUD.show("保存失败: \(message)")
        }
    }

    @MainActor
    static func presentPermissionAlert() {
        guard let top = topViewController() else { return }
        let alert = UIAlertController(
            title: "需要相册权限",
            message: "请在系统“设置”中允许 Multigravity 访问相册以保存图片。",
            preferredStyle: .alert
        )
        alert.addAction(UIAlertAction(title: "前往设置", style: .default) { _ in
            if let url = URL(string: UIApplication.openSettingsURLString) {
                UIApplication.shared.open(url)
            }
        })
        alert.addAction(UIAlertAction(title: "取消", style: .cancel))
        top.present(alert, animated: true)
    }

    // MARK: - Copy

    @MainActor
    static func copy(_ image: UIImage) {
        UIPasteboard.general.image = image
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        CopiedHUD.show("已拷贝图片")
    }

    // MARK: - Share

    @MainActor
    static func share(_ image: UIImage, from presenter: UIViewController? = nil, sourceView: UIView? = nil) {
        guard let host = presenter ?? topViewController() else { return }
        let controller = UIActivityViewController(activityItems: [image], applicationActivities: nil)
        if let popover = controller.popoverPresentationController {
            let anchor = sourceView ?? host.view
            popover.sourceView = anchor
            popover.sourceRect = CGRect(x: anchor?.bounds.midX ?? 0, y: anchor?.bounds.midY ?? 0, width: 0, height: 0)
            popover.permittedArrowDirections = []
        }
        host.present(controller, animated: true)
    }

    // MARK: - Convenience for SwiftUI menus

    @MainActor
    static func performSave(_ item: IdentifiableImage) {
        Task {
            guard let image = await loadImage(item) else {
                CopiedHUD.show("图片尚未加载完成")
                return
            }
            await saveWithFeedback(image, fileURL: item.url)
        }
    }

    @MainActor
    static func performShare(_ item: IdentifiableImage) {
        Task {
            guard let image = await loadImage(item) else {
                CopiedHUD.show("图片尚未加载完成")
                return
            }
            share(image)
        }
    }

    @MainActor
    static func performCopy(_ item: IdentifiableImage) {
        Task {
            guard let image = await loadImage(item) else {
                CopiedHUD.show("图片尚未加载完成")
                return
            }
            copy(image)
        }
    }

    // MARK: - Top view controller

    @MainActor
    static func topViewController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .first(where: { $0.activationState == .foregroundActive })
            ?? UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
        var top = scene?.windows.first(where: { $0.isKeyWindow })?.rootViewController
        while let presented = top?.presentedViewController {
            top = presented
        }
        return top
    }
}

/// 聊天流中图片的统一长按菜单：查看大图 / 保存 / 分享 / 拷贝。
struct ImageContextMenu: ViewModifier {
    let item: IdentifiableImage
    let onOpen: () -> Void

    func body(content: Content) -> some View {
        content.contextMenu {
            Button {
                onOpen()
            } label: {
                Label("查看大图", systemImage: "arrow.up.left.and.arrow.down.right")
            }
            Button {
                ImageActions.performSave(item)
            } label: {
                Label("保存到相册", systemImage: "square.and.arrow.down")
            }
            Button {
                ImageActions.performShare(item)
            } label: {
                Label("分享图片", systemImage: "square.and.arrow.up")
            }
            Button {
                ImageActions.performCopy(item)
            } label: {
                Label("拷贝图片", systemImage: "doc.on.doc")
            }
        }
    }
}

extension View {
    func imageContextMenu(item: IdentifiableImage, onOpen: @escaping () -> Void) -> some View {
        modifier(ImageContextMenu(item: item, onOpen: onOpen))
    }
}
