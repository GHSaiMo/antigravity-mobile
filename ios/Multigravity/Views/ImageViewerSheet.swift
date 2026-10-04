import SwiftUI
import Photos

public struct ImageViewerSheet: UIViewControllerRepresentable {
    public let items: [IdentifiableImage]
    public let initialIndex: Int
    @Environment(\.dismiss) private var dismiss
    
    public var image: UIImage? { items.indices.contains(initialIndex) ? items[initialIndex].image : nil }
    public var url: URL? { items.indices.contains(initialIndex) ? items[initialIndex].url : nil }
    
    public init(items: [IdentifiableImage], initialIndex: Int = 0) {
        self.items = items
        self.initialIndex = max(0, min(initialIndex, max(0, items.count - 1)))
    }
    
    public init(gallery: ImageGalleryData) {
        self.init(items: gallery.items, initialIndex: gallery.initialIndex)
    }
    
    public init(item: IdentifiableImage) {
        self.init(items: [item], initialIndex: 0)
    }
    
    public init(image: UIImage) {
        self.init(item: IdentifiableImage(image: image))
    }
    
    public init(url: URL) {
        self.init(item: IdentifiableImage(url: url))
    }
    
    public func makeUIViewController(context: Context) -> FullScreenGalleryViewController {
        FullScreenGalleryViewController(
            items: items,
            initialIndex: initialIndex,
            onDismiss: {
                var transaction = Transaction()
                transaction.disablesAnimations = true
                withTransaction(transaction) {
                    dismiss()
                }
            }
        )
    }
    
    public func updateUIViewController(_ uiViewController: FullScreenGalleryViewController, context: Context) {}
}
