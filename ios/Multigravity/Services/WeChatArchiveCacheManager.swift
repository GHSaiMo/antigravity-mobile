import Foundation
import CryptoKit
#if canImport(UIKit)
import UIKit
#endif

/// Persistent local cache for parsed WeChat chat archives and their embedded media
/// (images & video thumbnails), with a configurable default TTL (7 days).
public final class WeChatArchiveCacheManager: @unchecked Sendable {
    public static let shared = WeChatArchiveCacheManager()

    /// Default cache expiration interval: 7 days.
    public static let defaultTTL: TimeInterval = 7 * 24 * 60 * 60

    public var ttl: TimeInterval = WeChatArchiveCacheManager.defaultTTL

    private nonisolated struct CachedArchiveEnvelope: Codable, Sendable {
        let savedAt: Date
        let archive: WeChatChatArchive
    }

    #if canImport(UIKit)
    private final class CachedImageBox {
        let image: UIImage
        let savedAt: Date
        init(image: UIImage, savedAt: Date) {
            self.image = image
            self.savedAt = savedAt
        }
    }
    private let memImages = NSCache<NSString, CachedImageBox>()
    #endif

    private let fileManager = FileManager.default
    private let baseRootDirectory: URL
    private let lock = NSRecursiveLock()
    private let ioQueue = DispatchQueue(label: "com.antigravity.mobile.wechat-cache.io", qos: .utility)

    private var storageNamespace: String = ""
    private var memArchives: [String: CachedArchiveEnvelope] = [:]
    private var mediaKeysByArchive: [String: Set<String>] = [:]

    private func rootDir(forNamespace ns: String) -> URL {
        if ns.isEmpty { return baseRootDirectory }
        return baseRootDirectory.appendingPathComponent("profiles", isDirectory: true).appendingPathComponent(ns, isDirectory: true)
    }

    private var rootDirectory: URL {
        lock.lock()
        let ns = storageNamespace
        lock.unlock()
        return rootDir(forNamespace: ns)
    }

    private var archivesDirectory: URL {
        rootDirectory.appendingPathComponent("archives", isDirectory: true)
    }

    private var mediaDirectory: URL {
        rootDirectory.appendingPathComponent("media", isDirectory: true)
    }

    public func switchStorageNamespace(_ namespace: String) {
        lock.lock()
        storageNamespace = namespace
        memArchives.removeAll()
        mediaKeysByArchive.removeAll()
        #if canImport(UIKit)
        memImages.removeAllObjects()
        #endif
        lock.unlock()
        ensureDirectories()
    }

    public func clearStorage(forNamespace namespace: String) {
        lock.lock()
        if storageNamespace == namespace {
            memArchives.removeAll()
            mediaKeysByArchive.removeAll()
            #if canImport(UIKit)
            memImages.removeAllObjects()
            #endif
        }
        lock.unlock()
        let target = rootDir(forNamespace: namespace)
        ioQueue.async { [fileManager] in
            if namespace.isEmpty {
                try? fileManager.removeItem(at: target.appendingPathComponent("archives", isDirectory: true))
                try? fileManager.removeItem(at: target.appendingPathComponent("media", isDirectory: true))
            } else {
                try? fileManager.removeItem(at: target)
            }
        }
    }

    private init() {
        let baseCaches = fileManager.urls(for: .cachesDirectory, in: .userDomainMask).first ?? fileManager.temporaryDirectory
        let root = baseCaches
            .appendingPathComponent("antigravity_documents", isDirectory: true)
            .appendingPathComponent("wechat_archives", isDirectory: true)
        self.baseRootDirectory = root

        ensureDirectories()

        #if canImport(UIKit)
        memImages.countLimit = 180
        memImages.totalCostLimit = 60 * 1024 * 1024 // 60 MB

        NotificationCenter.default.addObserver(
            forName: UIApplication.didReceiveMemoryWarningNotification,
            object: nil,
            queue: nil
        ) { [weak self] _ in
            self?.handleMemoryWarning()
        }
        #endif

        ioQueue.async { [weak self] in
            self?.pruneExpiredEntriesSync()
        }
    }

    private func ensureDirectories() {
        try? fileManager.createDirectory(at: archivesDirectory, withIntermediateDirectories: true)
        try? fileManager.createDirectory(at: mediaDirectory, withIntermediateDirectories: true)
    }

    public func handleMemoryWarning() {
        lock.lock()
        memArchives.removeAll()
        mediaKeysByArchive.removeAll()
        #if canImport(UIKit)
        memImages.removeAllObjects()
        #endif
        lock.unlock()
    }

    // MARK: - Key Generation

    private func normalizeURI(_ uri: String) -> String {
        var cleanPath = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        if cleanPath.hasPrefix("MEDIA:") {
            cleanPath = String(cleanPath.dropFirst(6)).trimmingCharacters(in: .whitespaces)
        }
        if cleanPath.hasPrefix("file://") {
            cleanPath = String(cleanPath.dropFirst(7))
        }
        return cleanPath.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
    }

    private func sha256Hex(_ input: String) -> String {
        let digest = SHA256.hash(data: Data(input.utf8))
        return digest.prefix(16).map { String(format: "%02x", $0) }.joined()
    }

    private func archiveKey(uri: String, cascadeId: String?) -> String {
        let cleanUri = normalizeURI(uri)
        let scopeKey: String
        if let cid = cascadeId?.trimmingCharacters(in: .whitespacesAndNewlines),
           !cid.isEmpty,
           !cleanUri.contains(cid) {
            scopeKey = "\(cid)_\(cleanUri)"
        } else {
            scopeKey = cleanUri
        }
        return sha256Hex(scopeKey)
    }

    private func archiveFileURL(forKey key: String) -> URL {
        archivesDirectory.appendingPathComponent("\(key).json")
    }

    private func archiveMediaDirURL(forKey key: String) -> URL {
        mediaDirectory.appendingPathComponent(key, isDirectory: true)
    }

    private func mediaCacheKey(archiveKey: String, entry: String, thumb: Bool) -> String {
        let suffix = thumb ? "thumb" : "full"
        return "\(archiveKey)_\(sha256Hex("\(entry)|\(suffix)"))"
    }

    private func mediaFileURL(archiveKey: String, entry: String, thumb: Bool) -> URL {
        let entryHash = sha256Hex("\(entry)|\(thumb ? "thumb" : "full")")
        let rawExt = (entry as NSString).pathExtension.lowercased()
        let isSafeExt = !rawExt.isEmpty && rawExt.count <= 6 && rawExt.allSatisfy { $0.isASCII && ($0.isLetter || $0.isNumber) }
        let ext = thumb ? "jpg" : (isSafeExt ? rawExt : "bin")
        return archiveMediaDirURL(forKey: archiveKey).appendingPathComponent("\(entryHash).\(ext)")
    }

    // MARK: - Archive JSON Cache

    /// Synchronous in-memory lookup for zero-flicker initial state.
    public func cachedArchiveInMemory(uri: String, cascadeId: String?) -> WeChatChatArchive? {
        let key = archiveKey(uri: uri, cascadeId: cascadeId)
        lock.lock()
        defer { lock.unlock() }
        guard let envelope = memArchives[key] else { return nil }
        if Date().timeIntervalSince(envelope.savedAt) <= ttl {
            return envelope.archive
        }
        memArchives.removeValue(forKey: key)
        ioQueue.async { [weak self] in
            self?.removeArchiveFromDiskSync(key: key)
        }
        return nil
    }

    /// Loads a non-expired cached archive from memory or disk.
    public func loadArchive(uri: String, cascadeId: String?) async -> WeChatChatArchive? {
        if let mem = cachedArchiveInMemory(uri: uri, cascadeId: cascadeId) {
            return mem
        }
        let key = archiveKey(uri: uri, cascadeId: cascadeId)
        let currentTTL = ttl
        return await withCheckedContinuation { continuation in
            ioQueue.async { [weak self] in
                guard let self else {
                    continuation.resume(returning: nil)
                    return
                }
                let fileURL = self.archiveFileURL(forKey: key)
                guard self.fileManager.fileExists(atPath: fileURL.path),
                      let data = try? Data(contentsOf: fileURL),
                      let envelope = try? JSONDecoder().decode(CachedArchiveEnvelope.self, from: data) else {
                    continuation.resume(returning: nil)
                    return
                }
                if Date().timeIntervalSince(envelope.savedAt) <= currentTTL {
                    self.lock.lock()
                    self.memArchives[key] = envelope
                    self.lock.unlock()
                    continuation.resume(returning: envelope.archive)
                } else {
                    self.removeArchiveFromDiskSync(key: key)
                    continuation.resume(returning: nil)
                }
            }
        }
    }

    /// Saves a parsed WeChat chat archive to memory and disk.
    public func saveArchive(_ archive: WeChatChatArchive, uri: String, cascadeId: String?) {
        guard archive.isWeChatArchive else { return }
        let key = archiveKey(uri: uri, cascadeId: cascadeId)
        let envelope = CachedArchiveEnvelope(savedAt: Date(), archive: archive)

        lock.lock()
        memArchives[key] = envelope
        lock.unlock()

        ioQueue.async { [weak self] in
            guard let self else { return }
            self.ensureDirectories()
            if let data = try? JSONEncoder().encode(envelope) {
                let fileURL = self.archiveFileURL(forKey: key)
                try? data.write(to: fileURL, options: .atomic)
            }
        }
    }

    /// Invalidates cached archive JSON and associated embedded media for a single archive.
    public func invalidateArchive(uri: String, cascadeId: String?) {
        let key = archiveKey(uri: uri, cascadeId: cascadeId)
        lock.lock()
        memArchives.removeValue(forKey: key)
        #if canImport(UIKit)
        if let keys = mediaKeysByArchive.removeValue(forKey: key) {
            for mk in keys {
                memImages.removeObject(forKey: mk as NSString)
            }
        }
        #endif
        lock.unlock()

        ioQueue.async { [weak self] in
            self?.removeArchiveFromDiskSync(key: key)
        }
    }

    /// Invalidates only embedded media cache for a single archive (e.g. during force refresh).
    public func invalidateMedia(uri: String, cascadeId: String?) {
        let key = archiveKey(uri: uri, cascadeId: cascadeId)
        lock.lock()
        #if canImport(UIKit)
        if let keys = mediaKeysByArchive.removeValue(forKey: key) {
            for mk in keys {
                memImages.removeObject(forKey: mk as NSString)
            }
        }
        #endif
        lock.unlock()

        ioQueue.async { [weak self] in
            guard let self else { return }
            let dir = self.archiveMediaDirURL(forKey: key)
            try? self.fileManager.removeItem(at: dir)
        }
    }

    // MARK: - Embedded Media (Image / Video Thumbnail) Cache

    #if canImport(UIKit)
    public func cachedMediaImageInMemory(uri: String, cascadeId: String?, entry: String, thumb: Bool) -> UIImage? {
        let aKey = archiveKey(uri: uri, cascadeId: cascadeId)
        let mKey = mediaCacheKey(archiveKey: aKey, entry: entry, thumb: thumb)
        guard let box = memImages.object(forKey: mKey as NSString) else { return nil }
        if Date().timeIntervalSince(box.savedAt) <= ttl {
            return box.image
        }
        memImages.removeObject(forKey: mKey as NSString)
        return nil
    }

    public func cachedMediaFileURL(uri: String, cascadeId: String?, entry: String, thumb: Bool) -> URL? {
        let aKey = archiveKey(uri: uri, cascadeId: cascadeId)
        let url = mediaFileURL(archiveKey: aKey, entry: entry, thumb: thumb)
        guard fileManager.fileExists(atPath: url.path),
              let attrs = try? fileManager.attributesOfItem(atPath: url.path),
              let size = attrs[.size] as? Int64, size > 0 else {
            return nil
        }
        if let modDate = attrs[.modificationDate] as? Date,
           Date().timeIntervalSince(modDate) > ttl {
            try? fileManager.removeItem(at: url)
            return nil
        }
        return url
    }

    public func loadMediaImage(uri: String, cascadeId: String?, entry: String, thumb: Bool) async -> UIImage? {
        if let mem = cachedMediaImageInMemory(uri: uri, cascadeId: cascadeId, entry: entry, thumb: thumb) {
            return mem
        }
        let aKey = archiveKey(uri: uri, cascadeId: cascadeId)
        let mKey = mediaCacheKey(archiveKey: aKey, entry: entry, thumb: thumb)
        let fileURL = mediaFileURL(archiveKey: aKey, entry: entry, thumb: thumb)
        let currentTTL = ttl

        return await withCheckedContinuation { continuation in
            ioQueue.async { [weak self] in
                guard let self else {
                    continuation.resume(returning: nil)
                    return
                }
                guard self.fileManager.fileExists(atPath: fileURL.path),
                      let attrs = try? self.fileManager.attributesOfItem(atPath: fileURL.path),
                      let size = attrs[.size] as? Int64, size > 0 else {
                    continuation.resume(returning: nil)
                    return
                }
                let modDate = (attrs[.modificationDate] as? Date) ?? Date()
                if Date().timeIntervalSince(modDate) > currentTTL {
                    try? self.fileManager.removeItem(at: fileURL)
                    continuation.resume(returning: nil)
                    return
                }
                guard let data = try? Data(contentsOf: fileURL),
                      let image = UIImage(data: data) else {
                    try? self.fileManager.removeItem(at: fileURL)
                    continuation.resume(returning: nil)
                    return
                }
                self.storeMemoryImage(image, savedAt: modDate, archiveKey: aKey, mediaKey: mKey)
                continuation.resume(returning: image)
            }
        }
    }

    public func saveMediaData(_ data: Data, image: UIImage, uri: String, cascadeId: String?, entry: String, thumb: Bool) {
        guard !data.isEmpty else { return }
        let aKey = archiveKey(uri: uri, cascadeId: cascadeId)
        let mKey = mediaCacheKey(archiveKey: aKey, entry: entry, thumb: thumb)
        let dirURL = archiveMediaDirURL(forKey: aKey)
        let fileURL = mediaFileURL(archiveKey: aKey, entry: entry, thumb: thumb)

        storeMemoryImage(image, savedAt: Date(), archiveKey: aKey, mediaKey: mKey)

        ioQueue.async { [weak self] in
            guard let self else { return }
            try? self.fileManager.createDirectory(at: dirURL, withIntermediateDirectories: true)
            try? data.write(to: fileURL, options: .atomic)
        }
    }

    private func storeMemoryImage(_ image: UIImage, savedAt: Date, archiveKey: String, mediaKey: String) {
        let cost = Int(image.size.width * image.size.height * image.scale * image.scale * 4)
        let box = CachedImageBox(image: image, savedAt: savedAt)
        memImages.setObject(box, forKey: mediaKey as NSString, cost: min(max(cost, 1), 10 * 1024 * 1024))
        lock.lock()
        mediaKeysByArchive[archiveKey, default: []].insert(mediaKey)
        lock.unlock()
    }
    #endif

    // MARK: - Maintenance & Cleanup

    /// Clears all cached WeChat archives and media from memory and disk.
    public func clearCache() {
        lock.lock()
        memArchives.removeAll()
        mediaKeysByArchive.removeAll()
        #if canImport(UIKit)
        memImages.removeAllObjects()
        #endif
        lock.unlock()

        ioQueue.async { [weak self] in
            guard let self else { return }
            try? self.fileManager.removeItem(at: self.rootDirectory)
            self.ensureDirectories()
        }
    }

    private func removeArchiveFromDiskSync(key: String) {
        try? fileManager.removeItem(at: archiveFileURL(forKey: key))
        try? fileManager.removeItem(at: archiveMediaDirURL(forKey: key))
    }

    private func pruneExpiredEntriesSync() {
        let now = Date()
        let currentTTL = ttl

        // 1. Prune expired archive JSON files (and their media folders)
        if let files = try? fileManager.contentsOfDirectory(
            at: archivesDirectory,
            includingPropertiesForKeys: [.contentModificationDateKey],
            options: [.skipsHiddenFiles]
        ) {
            for fileURL in files where fileURL.pathExtension == "json" {
                let key = fileURL.deletingPathExtension().lastPathComponent
                var isExpired = false
                if let data = try? Data(contentsOf: fileURL),
                   let envelope = try? JSONDecoder().decode(CachedArchiveEnvelope.self, from: data) {
                    if now.timeIntervalSince(envelope.savedAt) > currentTTL {
                        isExpired = true
                    }
                } else if let attrs = try? fileManager.attributesOfItem(atPath: fileURL.path),
                          let modDate = attrs[.modificationDate] as? Date,
                          now.timeIntervalSince(modDate) > currentTTL {
                    isExpired = true
                }
                if isExpired {
                    removeArchiveFromDiskSync(key: key)
                }
            }
        }

        // 2. Prune expired or orphaned media files
        if let dirs = try? fileManager.contentsOfDirectory(
            at: mediaDirectory,
            includingPropertiesForKeys: nil,
            options: [.skipsHiddenFiles]
        ) {
            for dirURL in dirs {
                if let mediaFiles = try? fileManager.contentsOfDirectory(
                    at: dirURL,
                    includingPropertiesForKeys: [.contentModificationDateKey],
                    options: [.skipsHiddenFiles]
                ) {
                    var remaining = mediaFiles.count
                    for mf in mediaFiles {
                        if let attrs = try? fileManager.attributesOfItem(atPath: mf.path),
                           let modDate = attrs[.modificationDate] as? Date,
                           now.timeIntervalSince(modDate) > currentTTL {
                            try? fileManager.removeItem(at: mf)
                            remaining -= 1
                        }
                    }
                    if remaining <= 0 {
                        try? fileManager.removeItem(at: dirURL)
                    }
                }
            }
        }
    }
}
