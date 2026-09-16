import UIKit
import Photos
import ZLPhotoBrowser

@MainActor
public final class ZLPhotoPickerBridge {
    public static let shared = ZLPhotoPickerBridge()
    
    // 保持对当前活动 picker 的引用，避免被提前释放
    private var activePicker: ZLPhotoPicker?
    
    private init() {
        configureDefaults()
    }
    
    private func configureDefaults() {
        let config = ZLPhotoConfiguration.default()
        // 只允许图片，不允许视频
        config.allowSelectImage = true
        config.allowSelectVideo = false
        
        // 核心：相册中首格展示相机拍照（微信同款）
        config.allowTakePhotoInLibrary = true
        config.cameraConfiguration.allowTakePhoto = true
        config.cameraConfiguration.allowRecordVideo = false
        
        // 禁用图片与视频编辑，保证快速高效选择
        config.allowEditImage = false
        config.allowEditVideo = false
        
        // UI 配置
        let uiConfig = ZLPhotoUIConfiguration.default()
        // 倒序展示：最新的照片在最上面，相机固定在第 1 个格子（index 0）
        uiConfig.sortAscending = false
        uiConfig.showSelectedMask = true
        uiConfig.themeColor = .systemIndigo
    }
    
    /// 打开相册与相机（首格即相机拍照）
    /// - Parameters:
    ///   - maxCount: 最多可选照片数
    ///   - onImagesPicked: 选取或拍照完成后的图片回调
    public func present(maxCount: Int = 5, onImagesPicked: @escaping ([UIImage]) -> Void) {
        guard maxCount > 0 else { return }
        
        guard let topVC = getTopViewController() else {
            return
        }
        
        let config = ZLPhotoConfiguration.default()
        config.maxSelectCount = maxCount
        
        let picker = ZLPhotoPicker()
        self.activePicker = picker
        
        picker.selectImageBlock = { [weak self] results, isOriginal in
            self?.activePicker = nil
            let images = results.map { $0.image }
            guard !images.isEmpty else { return }
            onImagesPicked(images)
        }
        
        picker.cancelBlock = { [weak self] in
            self?.activePicker = nil
        }
        
        picker.showPhotoLibrary(sender: topVC)
    }
    
    /// 获取当前最顶层的 UIViewController
    private func getTopViewController() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let activeScene = scenes.first(where: { $0.activationState == .foregroundActive }) ?? scenes.first
        guard let window = activeScene?.windows.first(where: { $0.isKeyWindow }) ?? activeScene?.windows.first else {
            return nil
        }
        
        var top = window.rootViewController
        while let presented = top?.presentedViewController {
            top = presented
        }
        return top
    }
}
