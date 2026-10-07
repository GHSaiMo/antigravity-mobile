import Foundation
import Observation

/// A file received from another app (WeChat "用其他应用打开", Files, ...) that is waiting for a destination.
public struct SharedFile: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let size: Int64
    public let url: URL
    public let isImage: Bool
    /// Non-nil when the file cannot be attached; it stays visible in the sheet but is never delivered.
    public let rejectReason: String?
    
    public var isDeliverable: Bool { rejectReason == nil }
}

/// Staging area for files handed to the app from outside.
///
/// iOS delivers "open in" documents as copies in `Documents/Inbox`, which the system may clear, so
/// they are moved into app caches right away. After the user picks a destination, the files are
/// handed to that chat (`ChatView` consumes them) which runs the normal attach pipeline
/// (validation, draft copy, background upload).
@Observable
@MainActor
public final class ShareInbox {
    public static let shared = ShareInbox()
    
    public private(set) var files: [SharedFile] = []
    /// Whether the destination sheet is open (it can be dismissed while files stay staged).
    public var showSheet = false
    /// Bumped on every delivery so an already-open chat notices new files.
    public private(set) var deliveryTick = 0
    private var deliveries: [String: [SharedFile]] = [:]
    
    private static let maxStaged = 20
    private static let imageExtensions: Set<String> = ["jpg", "jpeg", "png", "heic", "heif", "gif", "webp", "bmp", "tiff"]
    
    private init() {}
    
    private var root: URL {
        let base = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory
        return base.appendingPathComponent("pending_share", isDirectory: true)
    }
    
    /// Removes staging leftovers that no list or pending delivery refers to. Call at launch.
    public func cleanupStale() {
        let fm = FileManager.default
        let live = Set((files + deliveries.values.flatMap { $0 }).map { $0.url.deletingLastPathComponent().lastPathComponent })
        for dir in (try? fm.contentsOfDirectory(at: root, includingPropertiesForKeys: nil)) ?? [] where !live.contains(dir.lastPathComponent) {
            try? fm.removeItem(at: dir)
        }
        // Delivered-but-never-consumed copies the system left in Documents/Inbox.
        if let docs = fm.urls(for: .documentDirectory, in: .userDomainMask).first {
            try? fm.removeItem(at: docs.appendingPathComponent("Inbox", isDirectory: true))
        }
    }
    
    /// Moves files delivered by "open in" into staging and opens the destination sheet.
    public func receive(_ urls: [URL]) {
        let fm = FileManager.default
        var added: [SharedFile] = []
        for url in urls where url.isFileURL {
            guard files.count + added.count < Self.maxStaged else { break }
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            
            let name = url.lastPathComponent
            let dir = root.appendingPathComponent(UUID().uuidString, isDirectory: true)
            try? fm.createDirectory(at: dir, withIntermediateDirectories: true)
            let target = dir.appendingPathComponent(name)
            do {
                try fm.copyItem(at: url, to: target)
            } catch {
                try? fm.removeItem(at: dir)
                added.append(SharedFile(id: UUID().uuidString, name: name, size: 0, url: target, isImage: false, rejectReason: "无法读取文件：\(name)"))
                continue
            }
            // The Inbox copy is ours to delete; anything else (in-place access) must be left alone.
            if url.path.contains("/Inbox/") { try? fm.removeItem(at: url) }
            
            let size = ((try? fm.attributesOfItem(atPath: target.path))?[.size] as? NSNumber)?.int64Value ?? 0
            let ext = (name as NSString).pathExtension.lowercased()
            let isImage = Self.imageExtensions.contains(ext)
            let reason = isImage ? (size <= 0 ? "文件为空：\(name)" : nil) : AttachmentRules.validate(name: name, size: size, existing: [])
            if reason != nil { try? fm.removeItem(at: dir) }
            added.append(SharedFile(id: UUID().uuidString, name: name, size: size, url: target, isImage: isImage, rejectReason: reason))
        }
        guard !added.isEmpty else { return }
        files.append(contentsOf: added)
        showSheet = true
    }
    
    public func remove(_ id: String) {
        if let f = files.first(where: { $0.id == id }) {
            try? FileManager.default.removeItem(at: f.url.deletingLastPathComponent())
        }
        files.removeAll { $0.id == id }
        if files.isEmpty { showSheet = false }
    }
    
    public func discardAll() {
        files = []
        showSheet = false
        try? FileManager.default.removeItem(at: root)
    }
    
    /// Hands the deliverable files to a conversation; its `ChatView` picks them up via `takeDelivery`.
    public func deliver(to conversationId: String) {
        let ok = files.filter { $0.isDeliverable }
        guard !ok.isEmpty else { return }
        deliveries[conversationId, default: []].append(contentsOf: ok)
        files = []
        showSheet = false
        deliveryTick += 1
    }
    
    public func takeDelivery(for conversationId: String) -> [SharedFile] {
        deliveries.removeValue(forKey: conversationId) ?? []
    }
}
