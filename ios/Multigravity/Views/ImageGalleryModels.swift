import SwiftUI
import Photos

public struct IdentifiableImage: Identifiable, Hashable {
    public let id: UUID
    public let image: UIImage?
    public let url: URL?
    
    public init(id: UUID = UUID(), image: UIImage) {
        self.id = id
        self.image = image
        self.url = nil
    }
    
    public init(id: UUID = UUID(), url: URL) {
        self.id = id
        self.image = nil
        self.url = url
    }
    
    public init(id: UUID = UUID(), image: UIImage?, url: URL?) {
        self.id = id
        self.image = image
        self.url = url
    }
    
    public func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
    
    public static func == (lhs: IdentifiableImage, rhs: IdentifiableImage) -> Bool {
        lhs.id == rhs.id
    }
}

public struct ImageGalleryData: Identifiable, Hashable {
    public let id: UUID
    public let items: [IdentifiableImage]
    public let initialIndex: Int
    
    public init(id: UUID = UUID(), items: [IdentifiableImage], initialIndex: Int = 0) {
        self.id = id
        self.items = items
        self.initialIndex = max(0, min(initialIndex, max(0, items.count - 1)))
    }
    
    public init(image: UIImage) {
        self.init(items: [IdentifiableImage(image: image)], initialIndex: 0)
    }
    
    public init(url: URL) {
        self.init(items: [IdentifiableImage(url: url)], initialIndex: 0)
    }
    
    public init(item: IdentifiableImage) {
        self.init(items: [item], initialIndex: 0)
    }
    
    public func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
    
    public static func == (lhs: ImageGalleryData, rhs: ImageGalleryData) -> Bool {
        lhs.id == rhs.id
    }
}
