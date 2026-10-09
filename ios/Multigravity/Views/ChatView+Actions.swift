import SwiftUI
import PhotosUI
import AVFoundation
import Photos

extension ChatView {
    func handleCancel() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            await viewModel.cancelTask()
        }
    }
    
    func handleSend() {
        guard !viewModel.isSending else { return }
        let text = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        let images = viewModel.selectedImageData
        let files = viewModel.selectedFiles
        let slash = viewModel.selectedSlashCommand
        guard !text.isEmpty || !images.isEmpty || !files.isEmpty || slash != nil else { return }
        // Files are uploaded in the background; wait until they have all reached the gateway.
        guard files.allSatisfy({ $0.isUploaded }) else { return }
        hasUserInteracted = false
        viewModel.inputText = ""
        viewModel.selectedImageData = []
        viewModel.selectedFiles = []
        viewModel.selectedSlashCommand = nil
        Task {
            let success = await viewModel.sendMessage(text: text, images: images, files: files, slashCommand: slash?.name)
            if !success {
                if let slash { viewModel.selectedSlashCommand = slash }
                if !images.isEmpty { viewModel.selectedImageData = images }
                if !files.isEmpty {
                    viewModel.selectedFiles = files
                    viewModel.persistDraftFiles()
                }
            }
        }
    }
    
    /// Converts the photos picked in the "+" panel and adds them to the draft.
    func handlePickedAssets(_ assets: [PHAsset]) {
        Task {
            let images = await PhotoAssetLoader.loadImages(assets)
            let compressed = images.compactMap { compressAndResizeImage($0) }
            guard !compressed.isEmpty else { return }
            viewModel.appendDraftImages(compressed)
        }
    }
    
    func removeImage(at index: Int) {
        viewModel.removeDraftImage(at: index)
    }
    
    func openPhotoLibraryWithCamera() {
        let maxCount = 5
        let currentCount = viewModel.selectedImageData.count
        let remaining = max(0, maxCount - currentCount)
        guard remaining > 0 else {
            UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
            return
        }
        
        ZLPhotoPickerBridge.shared.present(maxCount: remaining) { pickedImages in
            var compressedList: [Data] = []
            for img in pickedImages {
                if let data = compressAndResizeImage(img) {
                    compressedList.append(data)
                }
            }
            guard !compressedList.isEmpty else { return }
            viewModel.appendDraftImages(compressedList)
        }
    }
    
    func compressAndResizeImage(_ uiImage: UIImage) -> Data? {
        let maxDim: CGFloat = 1600
        let size = uiImage.size
        let targetImage: UIImage
        if size.width > maxDim || size.height > maxDim {
            let ratio = min(maxDim / size.width, maxDim / size.height)
            let newSize = CGSize(width: size.width * ratio, height: size.height * ratio)
            let format = UIGraphicsImageRendererFormat()
            format.scale = 1.0
            let renderer = UIGraphicsImageRenderer(size: newSize, format: format)
            targetImage = renderer.image { _ in
                uiImage.draw(in: CGRect(origin: .zero, size: newSize))
            }
        } else {
            targetImage = uiImage
        }
        return targetImage.jpegData(compressionQuality: 0.65)
    }
    
    func handleCapturedImage(_ uiImage: UIImage) {
        if let data = compressAndResizeImage(uiImage) {
            viewModel.appendDraftImages([data])
        }
    }
    
    func handleCameraAction() {
        guard UIImagePickerController.isSourceTypeAvailable(.camera) else {
            showCameraUnavailableAlert = true
            return
        }
        
        let status = AVCaptureDevice.authorizationStatus(for: .video)
        switch status {
        case .authorized:
            showCameraPicker = true
        case .notDetermined:
            AVCaptureDevice.requestAccess(for: .video) { granted in
                DispatchQueue.main.async {
                    if granted {
                        showCameraPicker = true
                    }
                }
            }
        case .denied, .restricted:
            showCameraPermissionAlert = true
        @unknown default:
            showCameraPicker = true
        }
    }
    
    func insertCommitAndPush() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let toAppend = "Commit and Push"
        if viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            viewModel.inputText = toAppend
        } else {
            viewModel.inputText += "\n" + toAppend
        }
        hasUserInteracted = false
        isInputFocused = true
    }
    
    func handleContinue() {
        guard !viewModel.isSending && !viewModel.isActivelyRunning else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        isInputFocused = false
        let text = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        let textToSend = text.isEmpty ? "Continue" : "\(text)\nContinue"
        viewModel.inputText = ""
        Task {
            await viewModel.sendMessage(text: textToSend)
        }
    }
    
    func insertContinue() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let toAppend = "Continue"
        if viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            viewModel.inputText = toAppend
        } else {
            viewModel.inputText += "\n" + toAppend
        }
        isInputFocused = true
    }
    
    func handleProceed() {
        isInputFocused = false
        Task {
            await viewModel.proceedArtifact()
        }
    }
    
}
