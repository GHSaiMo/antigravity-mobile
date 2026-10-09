import SwiftUI
import Photos

final class SingleImagePreviewController: UIViewController, UIScrollViewDelegate, UIPopoverPresentationControllerDelegate {
    let item: IdentifiableImage
    let onSingleTap: () -> Void
    
    let scrollView = UIScrollView()
    let imageView = UIImageView()
    let spinner = UIActivityIndicatorView(style: .large)
    
    init(item: IdentifiableImage, onSingleTap: @escaping () -> Void) {
        self.item = item
        self.onSingleTap = onSingleTap
        super.init(nibName: nil, bundle: nil)
    }
    
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    
    override func viewDidLoad() {
        super.viewDidLoad()
        overrideUserInterfaceStyle = .dark
        view.backgroundColor = .clear
        
        scrollView.frame = view.bounds
        scrollView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        scrollView.delegate = self
        scrollView.minimumZoomScale = 1.0
        scrollView.maximumZoomScale = 4.5
        scrollView.showsHorizontalScrollIndicator = false
        scrollView.showsVerticalScrollIndicator = false
        scrollView.contentInsetAdjustmentBehavior = .never
        view.addSubview(scrollView)
        
        imageView.contentMode = .scaleAspectFit
        imageView.clipsToBounds = true
        scrollView.addSubview(imageView)
        
        spinner.color = .white
        spinner.hidesWhenStopped = true
        view.addSubview(spinner)
        
        // Double-tap to zoom in/out
        let doubleTap = UITapGestureRecognizer(target: self, action: #selector(handleDoubleTap(_:)))
        doubleTap.numberOfTapsRequired = 2
        view.addGestureRecognizer(doubleTap)
        
        // Single-tap to dismiss
        let singleTap = UITapGestureRecognizer(target: self, action: #selector(handleSingleTap))
        singleTap.numberOfTapsRequired = 1
        singleTap.require(toFail: doubleTap)
        view.addGestureRecognizer(singleTap)
        
        // Long-press: 保存图片 / 分享图片
        let longPress = UILongPressGestureRecognizer(target: self, action: #selector(handleLongPress(_:)))
        longPress.minimumPressDuration = 0.4
        view.addGestureRecognizer(longPress)
        
        loadImage()
    }
    
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        spinner.center = CGPoint(x: view.bounds.midX, y: view.bounds.midY)
        updateImageFrame()
    }
    
    private func loadImage() {
        if let img = item.image {
            imageView.image = img
            updateImageFrame()
        }
        
        guard let url = item.url else { return }
        
        if url.isFileURL, let data = try? Data(contentsOf: url), let img = UIImage(data: data) {
            imageView.image = img
            updateImageFrame()
            return
        }
        
        if item.image == nil {
            spinner.startAnimating()
        }
        URLSession.shared.dataTask(with: url) { [weak self] data, _, _ in
            guard let self = self, let data = data, let img = UIImage(data: data) else {
                DispatchQueue.main.async {
                    self?.spinner.stopAnimating()
                }
                return
            }
            DispatchQueue.main.async {
                self.spinner.stopAnimating()
                self.imageView.image = img
                self.updateImageFrame()
            }
        }.resume()
    }
    
    func updateImageFrame() {
        guard let img = imageView.image, img.size.width > 0, img.size.height > 0 else {
            imageView.frame = view.bounds
            scrollView.contentSize = view.bounds.size
            return
        }
        
        let boundsSize = view.bounds.size
        guard boundsSize.width > 0, boundsSize.height > 0 else { return }
        
        let widthRatio = boundsSize.width / img.size.width
        let heightRatio = boundsSize.height / img.size.height
        let fitRatio = min(widthRatio, heightRatio)
        
        let fittedWidth = img.size.width * fitRatio
        let fittedHeight = img.size.height * fitRatio
        
        let originX = max(0, (boundsSize.width - fittedWidth) / 2)
        let originY = max(0, (boundsSize.height - fittedHeight) / 2)
        
        imageView.frame = CGRect(x: originX, y: originY, width: fittedWidth, height: fittedHeight)
        scrollView.contentSize = boundsSize
    }
    
    func scrollViewDidZoom(_ scrollView: UIScrollView) {
        let boundsSize = scrollView.bounds.size
        var frameToCenter = imageView.frame
        
        if frameToCenter.size.width < boundsSize.width {
            frameToCenter.origin.x = (boundsSize.width - frameToCenter.size.width) / 2
        } else {
            frameToCenter.origin.x = 0
        }
        
        if frameToCenter.size.height < boundsSize.height {
            frameToCenter.origin.y = (boundsSize.height - frameToCenter.size.height) / 2
        } else {
            frameToCenter.origin.y = 0
        }
        
        imageView.frame = frameToCenter
    }
    
    func viewForZooming(in scrollView: UIScrollView) -> UIView? {
        return imageView
    }
    
    @objc private func handleDoubleTap(_ gesture: UITapGestureRecognizer) {
        if scrollView.zoomScale > 1.05 {
            scrollView.setZoomScale(1.0, animated: true)
        } else {
            let point = gesture.location(in: imageView)
            let zoomWidth = view.bounds.width / 2.5
            let zoomHeight = view.bounds.height / 2.5
            let zoomRect = CGRect(
                x: point.x - (zoomWidth / 2.0),
                y: point.y - (zoomHeight / 2.0),
                width: zoomWidth,
                height: zoomHeight
            )
            scrollView.zoom(to: zoomRect, animated: true)
        }
    }
    
    @objc private func handleSingleTap() {
        if scrollView.zoomScale <= 1.05 {
            onSingleTap()
        }
    }
    
    // MARK: - Long Press & Save to Album
    
    @objc private func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
        guard gesture.state == .began else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        let alert = UIAlertController(title: nil, message: nil, preferredStyle: .actionSheet)
        alert.addAction(UIAlertAction(title: "保存图片", style: .default) { [weak self] _ in
            self?.saveToPhotosAlbum()
        })
        alert.addAction(UIAlertAction(title: "分享图片", style: .default) { [weak self] _ in
            self?.shareImage()
        })
        alert.addAction(UIAlertAction(title: "取消", style: .cancel, handler: nil))
        
        if let popover = alert.popoverPresentationController {
            popover.sourceView = view
            popover.sourceRect = CGRect(x: view.bounds.midX, y: view.bounds.midY, width: 0, height: 0)
            popover.permittedArrowDirections = []
            popover.delegate = self
        }
        
        present(alert, animated: true)
    }
    
    // MARK: - UIPopoverPresentationControllerDelegate
    
    public func adaptivePresentationStyle(for controller: UIPresentationController) -> UIModalPresentationStyle {
        return .none
    }
    
    public func adaptivePresentationStyle(for controller: UIPresentationController, traitCollection: UITraitCollection) -> UIModalPresentationStyle {
        return .none
    }
    
    private func saveToPhotosAlbum() {
        guard let image = imageView.image else {
            showToast(message: "图片正在加载，请稍后重试", isError: true)
            return
        }
        let fileURL = item.url
        Task { @MainActor [weak self] in
            switch await ImageActions.saveToPhotos(image, fileURL: fileURL) {
            case .saved:
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                self?.showToast(message: "已保存到相册", isError: false)
            case .denied:
                self?.showPermissionAlert()
            case .failed(let message):
                UINotificationFeedbackGenerator().notificationOccurred(.error)
                self?.showToast(message: "保存失败: \(message)", isError: true)
            }
        }
    }
    
    private func shareImage() {
        guard let image = imageView.image else {
            showToast(message: "图片正在加载，请稍后重试", isError: true)
            return
        }
        ImageActions.share(image, from: self, sourceView: view)
    }
    
    private func copyImage() {
        guard let image = imageView.image else {
            showToast(message: "图片正在加载，请稍后重试", isError: true)
            return
        }
        UIPasteboard.general.image = image
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        showToast(message: "已拷贝图片", isError: false)
    }
    
    private func showPermissionAlert() {
        let alert = UIAlertController(
            title: "需要相册权限",
            message: "请在系统“设置”中允许 Multigravity 访问相册以保存图片。",
            preferredStyle: .alert
        )
        alert.addAction(UIAlertAction(title: "前往设置", style: .default) { _ in
            if let settingsURL = URL(string: UIApplication.openSettingsURLString),
               UIApplication.shared.canOpenURL(settingsURL) {
                UIApplication.shared.open(settingsURL)
            }
        })
        alert.addAction(UIAlertAction(title: "取消", style: .cancel, handler: nil))
        present(alert, animated: true)
    }
    
    private func showToast(message: String, isError: Bool = false) {
        guard let hostView = self.parent?.view ?? self.view else { return }
        
        hostView.subviews.filter { $0.tag == 998811 }.forEach { $0.removeFromSuperview() }
        
        let container = UIView()
        container.tag = 998811
        container.backgroundColor = UIColor.black.withAlphaComponent(0.85)
        container.layer.cornerRadius = 18
        container.layer.masksToBounds = true
        container.translatesAutoresizingMaskIntoConstraints = false
        
        let iconName = isError ? "exclamationmark.circle.fill" : "checkmark.circle.fill"
        let iconColor: UIColor = isError ? .systemRed : .systemGreen
        let iconView = UIImageView(image: UIImage(systemName: iconName))
        iconView.tintColor = iconColor
        iconView.translatesAutoresizingMaskIntoConstraints = false
        
        let label = UILabel()
        label.text = message
        label.textColor = .white
        label.font = UIFont.systemFont(ofSize: 14, weight: .medium)
        label.textAlignment = .center
        label.translatesAutoresizingMaskIntoConstraints = false
        
        let stack = UIStackView(arrangedSubviews: [iconView, label])
        stack.axis = .horizontal
        stack.spacing = 8
        stack.alignment = .center
        stack.translatesAutoresizingMaskIntoConstraints = false
        
        container.addSubview(stack)
        hostView.addSubview(container)
        
        NSLayoutConstraint.activate([
            stack.topAnchor.constraint(equalTo: container.topAnchor, constant: 10),
            stack.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -10),
            stack.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 16),
            stack.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -16),
            iconView.widthAnchor.constraint(equalToConstant: 18),
            iconView.heightAnchor.constraint(equalToConstant: 18),
            container.centerXAnchor.constraint(equalTo: hostView.centerXAnchor),
            container.bottomAnchor.constraint(equalTo: hostView.safeAreaLayoutGuide.bottomAnchor, constant: -50)
        ])
        
        container.alpha = 0
        container.transform = CGAffineTransform(scaleX: 0.9, y: 0.9)
        
        UIView.animate(withDuration: 0.2, delay: 0, options: [.curveEaseOut], animations: {
            container.alpha = 1.0
            container.transform = .identity
        }) { _ in
            UIView.animate(withDuration: 0.25, delay: 2.0, options: [.curveEaseIn], animations: {
                container.alpha = 0
                container.transform = CGAffineTransform(scaleX: 0.9, y: 0.9)
            }) { _ in
                container.removeFromSuperview()
            }
        }
    }
}
