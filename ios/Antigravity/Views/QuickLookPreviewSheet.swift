import SwiftUI
import QuickLook
import WebKit
import Photos
import LinkPresentation

// MARK: - Frosted Glass Share Button (Unified Modern Blur Style)

public struct FrostedShareButton: View {
    public let action: () -> Void
    
    public init(action: @escaping () -> Void) {
        self.action = action
    }
    
    public var body: some View {
        Button(action: action) {
            Image(systemName: "square.and.arrow.up")
                .font(.system(size: 14.5, weight: .semibold))
                .foregroundColor(.primary)
                .frame(width: 34, height: 34)
                .background(.ultraThinMaterial)
                .clipShape(Circle())
                .overlay(
                    Circle()
                        .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                )
                .shadow(color: Color.black.opacity(0.06), radius: 3, x: 0, y: 1.5)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("分享")
    }
}

// MARK: - Native QuickLook Presentation Sheet (PPTX, DOCX, XLSX, PDF, KEY)

public struct QuickLookPreviewSheet: View {
    public let url: URL
    public let title: String
    public let onDismiss: () -> Void
    @State private var isSharing: Bool = false
    @State private var isSavingPhoto: Bool = false
    @State private var savePhotoSuccessMessage: String? = nil
    @State private var savePhotoErrorMessage: String? = nil
    
    public init(url: URL, title: String = "", onDismiss: @escaping () -> Void) {
        self.url = url
        let fallback = url.lastPathComponent.removingPercentEncoding ?? url.lastPathComponent
        self.title = title.isEmpty ? fallback : title
        self.onDismiss = onDismiss
    }
    
    @ViewBuilder
    private var savePhotoButton: some View {
        Button(action: saveToPhotosAlbum) {
            Group {
                if isSavingPhoto {
                    ProgressView()
                        .scaleEffect(0.8)
                } else {
                    Image(systemName: "square.and.arrow.down")
                        .font(.system(size: 14.5, weight: .semibold))
                        .foregroundColor(.primary)
                }
            }
            .frame(width: 34, height: 34)
            .background(.ultraThinMaterial)
            .clipShape(Circle())
            .overlay(
                Circle()
                    .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
            )
            .shadow(color: Color.black.opacity(0.06), radius: 3, x: 0, y: 1.5)
        }
        .buttonStyle(.plain)
        .disabled(isSavingPhoto)
        .accessibilityLabel("保存到相册")
    }
    
    public var body: some View {
        NavigationStack {
            Group {
                if isImageFile(url: url) {
                    HighResolutionImageViewer(url: url)
                        .ignoresSafeArea(edges: .bottom)
                } else {
                    QuickLookControllerRepresentable(url: url)
                        .ignoresSafeArea(edges: .bottom)
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: 8) {
                        if isImageFile(url: url) {
                            savePhotoButton
                        }
                        FrostedShareButton {
                            isSharing = true
                        }
                    }
                }
            }
            .presentationDragIndicator(.visible)
        }
        .presentationDragIndicator(.visible)
        .sheet(isPresented: $isSharing) {
            let items: [Any] = isImageFile(url: url) ? [ImageActivityItemSource(fileURL: url, title: title)] : [url]
            ShareSheetView(activityItems: items)
        }
        .overlay(alignment: .center) {
            if let msg = savePhotoSuccessMessage {
                VStack(spacing: 12) {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 40, weight: .semibold))
                        .foregroundColor(.green)
                    Text(msg)
                        .font(.system(size: 14.5, weight: .medium))
                        .foregroundColor(.primary)
                        .multilineTextAlignment(.center)
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 20)
                .background(.ultraThinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                )
                .shadow(color: Color.black.opacity(0.18), radius: 20, x: 0, y: 8)
                .transition(.scale(scale: 0.85).combined(with: .opacity))
            } else if let err = savePhotoErrorMessage {
                VStack(spacing: 12) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 40, weight: .semibold))
                        .foregroundColor(.orange)
                    Text(err)
                        .font(.system(size: 14.5, weight: .medium))
                        .foregroundColor(.primary)
                        .multilineTextAlignment(.center)
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 20)
                .background(.ultraThinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                )
                .shadow(color: Color.black.opacity(0.18), radius: 20, x: 0, y: 8)
                .transition(.scale(scale: 0.85).combined(with: .opacity))
            }
        }
        .animation(.spring(response: 0.3, dampingFraction: 0.78), value: savePhotoSuccessMessage)
        .animation(.spring(response: 0.3, dampingFraction: 0.78), value: savePhotoErrorMessage)
    }
    
    private func isImageFile(url: URL) -> Bool {
        let ext = url.pathExtension.lowercased()
        let imageExtensions = ["png", "jpg", "jpeg", "webp", "gif", "heic", "heif", "bmp", "svg", "tiff", "tif"]
        if imageExtensions.contains(ext) { return true }
        if let comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
           let uri = comps.queryItems?.first(where: { $0.name == "uri" })?.value {
            let uriExt = (uri as NSString).pathExtension.lowercased()
            if imageExtensions.contains(uriExt) { return true }
        }
        return false
    }
    
    private func saveToPhotosAlbum() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        isSavingPhoto = true
        
        PHPhotoLibrary.requestAuthorization(for: .addOnly) { status in
            guard status == .authorized || status == .limited else {
                DispatchQueue.main.async {
                    self.isSavingPhoto = false
                    self.savePhotoErrorMessage = "未获得相册权限，请在系统设置中开启"
                    DispatchQueue.main.asyncAfter(deadline: .now() + 3.0) {
                        self.savePhotoErrorMessage = nil
                    }
                }
                return
            }
            
            PHPhotoLibrary.shared().performChanges {
                let request = PHAssetCreationRequest.forAsset()
                request.addResource(with: .photo, fileURL: self.url, options: nil)
            } completionHandler: { success, error in
                DispatchQueue.main.async {
                    self.isSavingPhoto = false
                    if success {
                        UINotificationFeedbackGenerator().notificationOccurred(.success)
                        self.savePhotoSuccessMessage = "已保存至相册，可在微信中直接发原图"
                        DispatchQueue.main.asyncAfter(deadline: .now() + 3.0) {
                            self.savePhotoSuccessMessage = nil
                        }
                    } else {
                        UINotificationFeedbackGenerator().notificationOccurred(.error)
                        self.savePhotoErrorMessage = "保存失败: \(error?.localizedDescription ?? "未知错误")"
                        DispatchQueue.main.asyncAfter(deadline: .now() + 3.0) {
                            self.savePhotoErrorMessage = nil
                        }
                    }
                }
            }
        }
    }
}

public struct QuickLookControllerRepresentable: UIViewControllerRepresentable {
    public let url: URL
    
    public init(url: URL) {
        self.url = url
    }
    
    public func makeCoordinator() -> Coordinator {
        Coordinator(url: url)
    }
    
    public func makeUIViewController(context: Context) -> QuickLookContainerViewController {
        let container = QuickLookContainerViewController(url: url)
        container.qlController.dataSource = context.coordinator
        container.qlController.delegate = context.coordinator
        return container
    }
    
    public func updateUIViewController(_ uiViewController: QuickLookContainerViewController, context: Context) {
        context.coordinator.url = url
        uiViewController.url = url
    }
    
    public final class Coordinator: NSObject, QLPreviewControllerDataSource, QLPreviewControllerDelegate {
        var url: URL
        
        init(url: URL) {
            self.url = url
        }
        
        public func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
            return 1
        }
        
        public func previewController(_ controller: QLPreviewController, previewItemAt index: Int) -> QLPreviewItem {
            return url as NSURL
        }
    }
}

public final class QuickLookContainerViewController: UIViewController {
    let qlController = QLPreviewController()
    var url: URL {
        didSet {
            qlController.reloadData()
        }
    }
    
    init(url: URL) {
        self.url = url
        super.init(nibName: nil, bundle: nil)
    }
    
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }
    
    public override func viewDidLoad() {
        super.viewDidLoad()
        addChild(qlController)
        view.addSubview(qlController.view)
        qlController.view.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            qlController.view.topAnchor.constraint(equalTo: view.topAnchor),
            qlController.view.bottomAnchor.constraint(equalTo: view.bottomAnchor),
            qlController.view.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            qlController.view.trailingAnchor.constraint(equalTo: view.trailingAnchor)
        ])
        qlController.didMove(toParent: self)
    }
}

// MARK: - Native High-Resolution Image Presentation Sheet (PNG, JPG, WEBP, GIF, etc.)

public struct HighResolutionImageViewer: UIViewControllerRepresentable {
    public let url: URL
    
    public init(url: URL) {
        self.url = url
    }
    
    public func makeUIViewController(context: Context) -> HighResolutionImageViewController {
        return HighResolutionImageViewController(url: url)
    }
    
    public func updateUIViewController(_ uiViewController: HighResolutionImageViewController, context: Context) {
        if uiViewController.url != url {
            uiViewController.updateURL(url)
        }
    }
}

public final class HighResolutionImageViewController: UIViewController, UIScrollViewDelegate {
    public private(set) var url: URL
    private let scrollView = UIScrollView()
    private let containerView = UIView()
    private let loadingIndicator = UIActivityIndicatorView(style: .medium)
    
    private struct ImageSlice {
        let image: UIImage
        let pixelRect: CGRect
    }
    
    private var slices: [ImageSlice] = []
    private var sliceImageViews: [UIImageView] = []
    private var originalImageSize: CGSize = .zero
    private var isLoaded: Bool = false
    private var initialScrollDone: Bool = false
    
    public init(url: URL) {
        self.url = url
        super.init(nibName: nil, bundle: nil)
    }
    
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }
    
    public override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .systemBackground
        
        scrollView.delegate = self
        scrollView.minimumZoomScale = 1.0
        scrollView.maximumZoomScale = 5.0
        scrollView.zoomScale = 1.0
        scrollView.showsVerticalScrollIndicator = true
        scrollView.showsHorizontalScrollIndicator = false
        scrollView.alwaysBounceVertical = true
        scrollView.contentInsetAdjustmentBehavior = .never
        scrollView.backgroundColor = .systemBackground
        
        view.addSubview(scrollView)
        scrollView.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            scrollView.topAnchor.constraint(equalTo: view.topAnchor),
            scrollView.bottomAnchor.constraint(equalTo: view.bottomAnchor),
            scrollView.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            scrollView.trailingAnchor.constraint(equalTo: view.trailingAnchor)
        ])
        
        containerView.backgroundColor = .clear
        scrollView.addSubview(containerView)
        
        loadingIndicator.hidesWhenStopped = true
        view.addSubview(loadingIndicator)
        loadingIndicator.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            loadingIndicator.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            loadingIndicator.centerYAnchor.constraint(equalTo: view.centerYAnchor)
        ])
        
        let doubleTap = UITapGestureRecognizer(target: self, action: #selector(handleDoubleTap(_:)))
        doubleTap.numberOfTapsRequired = 2
        scrollView.addGestureRecognizer(doubleTap)
        
        loadImage()
    }
    
    public func updateURL(_ newURL: URL) {
        self.url = newURL
        self.isLoaded = false
        self.initialScrollDone = false
        loadImage()
    }
    
    private func loadImage() {
        loadingIndicator.startAnimating()
        let targetURL = self.url
        
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            guard let self = self else { return }
            var loadedImage: UIImage? = nil
            if let data = try? Data(contentsOf: targetURL) {
                loadedImage = UIImage(data: data)
            } else if let img = UIImage(contentsOfFile: targetURL.path) {
                loadedImage = img
            }
            
            guard let rawImage = loadedImage else {
                DispatchQueue.main.async {
                    self.loadingIndicator.stopAnimating()
                }
                return
            }
            
            let image = self.normalizeOrientation(rawImage)
            let generatedSlices = self.generateSlices(for: image)
            
            DispatchQueue.main.async {
                guard self.url == targetURL else { return }
                self.setupWithSlices(generatedSlices, originalSize: image.size)
            }
        }
    }
    
    private func normalizeOrientation(_ image: UIImage) -> UIImage {
        if image.imageOrientation == .up { return image }
        UIGraphicsBeginImageContextWithOptions(image.size, false, image.scale)
        image.draw(in: CGRect(origin: .zero, size: image.size))
        let normalized = UIGraphicsGetImageFromCurrentImageContext() ?? image
        UIGraphicsEndImageContext()
        return normalized
    }
    
    private func generateSlices(for image: UIImage) -> [ImageSlice] {
        guard let cgImage = image.cgImage else {
            return [ImageSlice(image: image, pixelRect: CGRect(origin: .zero, size: image.size))]
        }
        
        let totalPixelWidth = cgImage.width
        let totalPixelHeight = cgImage.height
        guard totalPixelWidth > 0, totalPixelHeight > 0 else { return [] }
        
        // Chunk height of 2048 px guarantees zero GPU downsampling and negligible memory overhead
        let maxSliceHeight = 2048
        if totalPixelHeight <= maxSliceHeight {
            return [ImageSlice(image: image, pixelRect: CGRect(x: 0, y: 0, width: totalPixelWidth, height: totalPixelHeight))]
        }
        
        var slices: [ImageSlice] = []
        var currentY = 0
        while currentY < totalPixelHeight {
            let sliceH = min(maxSliceHeight, totalPixelHeight - currentY)
            let rect = CGRect(x: 0, y: currentY, width: totalPixelWidth, height: sliceH)
            if let cropped = cgImage.cropping(to: rect) {
                let sliceImg = UIImage(cgImage: cropped, scale: image.scale, orientation: .up)
                slices.append(ImageSlice(image: sliceImg, pixelRect: rect))
            }
            currentY += sliceH
        }
        return slices
    }
    
    private func setupWithSlices(_ slices: [ImageSlice], originalSize: CGSize) {
        self.slices = slices
        self.originalImageSize = originalSize
        self.isLoaded = true
        self.loadingIndicator.stopAnimating()
        
        containerView.subviews.forEach { $0.removeFromSuperview() }
        sliceImageViews.removeAll()
        
        for slice in slices {
            let iv = UIImageView(image: slice.image)
            iv.contentMode = .scaleToFill
            iv.clipsToBounds = true
            containerView.addSubview(iv)
            sliceImageViews.append(iv)
        }
        
        layoutImageViews()
        
        if !initialScrollDone {
            scrollView.contentOffset = .zero
            initialScrollDone = true
        }
    }
    
    public override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        if scrollView.zoomScale == 1.0 {
            layoutImageViews()
        }
    }
    
    private func layoutImageViews() {
        guard isLoaded, !slices.isEmpty, originalImageSize.width > 0, originalImageSize.height > 0 else { return }
        let viewportWidth = view.bounds.width
        let viewportHeight = view.bounds.height
        guard viewportWidth > 0, viewportHeight > 0 else { return }
        
        let scale = viewportWidth / originalImageSize.width
        let totalDisplayHeight = originalImageSize.height * scale
        
        containerView.frame = CGRect(x: 0, y: 0, width: viewportWidth, height: totalDisplayHeight)
        scrollView.contentSize = CGSize(width: viewportWidth, height: totalDisplayHeight)
        
        var currentY: CGFloat = 0
        for (idx, slice) in slices.enumerated() {
            guard idx < sliceImageViews.count else { break }
            let iv = sliceImageViews[idx]
            let sliceDisplayHeight = slice.pixelRect.height * scale
            iv.frame = CGRect(x: 0, y: currentY, width: viewportWidth, height: sliceDisplayHeight)
            currentY += sliceDisplayHeight
        }
        
        if totalDisplayHeight <= viewportHeight {
            let offsetY = (viewportHeight - totalDisplayHeight) / 2.0
            scrollView.contentInset = UIEdgeInsets(top: offsetY, left: 0, bottom: offsetY, right: 0)
        } else {
            scrollView.contentInset = .zero
        }
    }
    
    // MARK: - UIScrollViewDelegate
    
    public func viewForZooming(in scrollView: UIScrollView) -> UIView? {
        return containerView
    }
    
    public func scrollViewDidZoom(_ scrollView: UIScrollView) {
        let offsetX = max((scrollView.bounds.width - scrollView.contentSize.width) * 0.5, 0)
        let offsetY = max((scrollView.bounds.height - scrollView.contentSize.height) * 0.5, 0)
        containerView.center = CGPoint(
            x: scrollView.contentSize.width * 0.5 + offsetX,
            y: scrollView.contentSize.height * 0.5 + offsetY
        )
    }
    
    @objc private func handleDoubleTap(_ gesture: UITapGestureRecognizer) {
        if scrollView.zoomScale > 1.1 {
            scrollView.setZoomScale(1.0, animated: true)
        } else {
            let pointInContainer = gesture.location(in: containerView)
            let targetScale: CGFloat = min(2.5, scrollView.maximumZoomScale)
            let width = scrollView.bounds.width / targetScale
            let height = scrollView.bounds.height / targetScale
            let x = pointInContainer.x - (width / 2.0)
            let y = pointInContainer.y - (height / 2.0)
            let zoomRect = CGRect(x: x, y: y, width: width, height: height)
            scrollView.zoom(to: zoomRect, animated: true)
        }
    }
}

// MARK: - Interactive HTML Presentation Sheet (WKWebView)

public struct HTMLPreviewSheet: View {
    public let url: URL
    public let title: String
    public let onDismiss: () -> Void
    @State private var isSharing: Bool = false
    
    public init(url: URL, title: String = "", onDismiss: @escaping () -> Void) {
        self.url = url
        let fallback = url.lastPathComponent.removingPercentEncoding ?? url.lastPathComponent
        self.title = title.isEmpty ? fallback : title
        self.onDismiss = onDismiss
    }
    
    public var body: some View {
        NavigationStack {
            HTMLWebViewRepresentable(url: url)
                .ignoresSafeArea(edges: .bottom)
                .navigationTitle(title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        FrostedShareButton {
                            isSharing = true
                        }
                    }
                }
        }
        .presentationDragIndicator(.visible)
        .sheet(isPresented: $isSharing) {
            ShareSheetView(activityItems: [url])
        }
    }
}

public struct HTMLWebViewRepresentable: UIViewRepresentable {
    public let url: URL
    
    public func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.preferences.setValue(false, forKey: "allowFileAccessFromFileURLs")
        let prefs = WKWebpagePreferences()
        prefs.allowsContentJavaScript = false
        config.defaultWebpagePreferences = prefs
        let webView = WKWebView(frame: .zero, configuration: config)
        webView.allowsBackForwardNavigationGestures = false
        webView.scrollView.contentInsetAdjustmentBehavior = .automatic
        webView.loadFileURL(url, allowingReadAccessTo: url)
        return webView
    }
    
    public func updateUIView(_ uiView: WKWebView, context: Context) {}
}

// MARK: - Activity View Controller for Sharing files

public struct ShareSheetView: UIViewControllerRepresentable {
    public let activityItems: [Any]
    public let onDismiss: (() -> Void)?
    
    public init(activityItems: [Any], onDismiss: (() -> Void)? = nil) {
        self.activityItems = activityItems
        self.onDismiss = onDismiss
    }
    
    public func makeUIViewController(context: Context) -> UIActivityViewController {
        let controller = UIActivityViewController(activityItems: activityItems, applicationActivities: nil)
        controller.completionWithItemsHandler = { _, _, _, _ in
            self.onDismiss?()
        }
        return controller
    }
    
    public func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}

// MARK: - Safe Image Activity Item Source (Prevent WeChat Extension Jetsam OOM)

public final class ImageActivityItemSource: NSObject, UIActivityItemSource {
    public let fileURL: URL
    public let title: String
    
    public init(fileURL: URL, title: String) {
        self.fileURL = fileURL
        self.title = title
        super.init()
    }
    
    public func activityViewControllerPlaceholderItem(_ activityViewController: UIActivityViewController) -> Any {
        return fileURL
    }
    
    public func activityViewController(_ activityViewController: UIActivityViewController, itemForActivityType activityType: UIActivity.ActivityType?) -> Any? {
        let typeString = activityType?.rawValue.lowercased() ?? ""
        let isWeChat = typeString.contains("tencent") || typeString.contains("xin")
        
        if isWeChat {
            // Check if image is extremely tall (e.g. > 3000px) which will crash WeChat's 60MB extension sandbox
            if let data = try? Data(contentsOf: fileURL),
               let source = CGImageSourceCreateWithData(data as CFData, nil),
               let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
               let height = properties[kCGImagePropertyPixelHeight] as? Int,
               let width = properties[kCGImagePropertyPixelWidth] as? Int,
               height > 3000 || width > 3000 {
                
                // Downscale proportionally so max dimension is 3000px for safe memory consumption inside WeChat's extension
                let maxDim = 3000
                let options: [CFString: Any] = [
                    kCGImageSourceThumbnailMaxPixelSize: maxDim,
                    kCGImageSourceCreateThumbnailFromImageAlways: true,
                    kCGImageSourceCreateThumbnailWithTransform: true
                ]
                if let cgDownscaled = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) {
                    let downscaledImg = UIImage(cgImage: cgDownscaled)
                    if let jpegData = downscaledImg.jpegData(compressionQuality: 0.85) {
                        let tempPath = (NSTemporaryDirectory() as NSString).appendingPathComponent("wechat_\(fileURL.lastPathComponent).jpg")
                        let tempURL = URL(fileURLWithPath: tempPath)
                        try? jpegData.write(to: tempURL)
                        return tempURL
                    }
                }
            }
        }
        
        // For Save to Photos, AirDrop, Files, Copy, etc., always provide the 100% original full-res file!
        return fileURL
    }
    
    public func activityViewControllerLinkMetadata(_ activityViewController: UIActivityViewController) -> LPLinkMetadata? {
        let metadata = LPLinkMetadata()
        metadata.title = title
        
        // Pre-render a lightweight 300px thumbnail so WeChat and system share sheet don't decode the entire huge image for preview
        if let data = try? Data(contentsOf: fileURL),
           let source = CGImageSourceCreateWithData(data as CFData, nil) {
            let options: [CFString: Any] = [
                kCGImageSourceThumbnailMaxPixelSize: 300,
                kCGImageSourceCreateThumbnailFromImageAlways: true,
                kCGImageSourceCreateThumbnailWithTransform: true
            ]
            if let cgThumb = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) {
                let thumb = UIImage(cgImage: cgThumb)
                metadata.iconProvider = NSItemProvider(object: thumb)
                metadata.imageProvider = NSItemProvider(object: thumb)
            }
        }
        return metadata
    }
}
