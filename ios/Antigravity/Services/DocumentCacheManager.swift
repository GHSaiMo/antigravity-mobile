import Foundation
import CryptoKit

public final class DocumentCacheManager: @unchecked Sendable {
    public static let shared = DocumentCacheManager()
    
    private let fileManager = FileManager.default
    private let cacheDirectory: URL
    
    private init() {
        let baseCaches = fileManager.urls(for: .cachesDirectory, in: .userDomainMask).first ?? fileManager.temporaryDirectory
        let docsDir = baseCaches.appendingPathComponent("antigravity_documents", isDirectory: true)
        try? fileManager.createDirectory(at: docsDir, withIntermediateDirectories: true)
        self.cacheDirectory = docsDir
    }
    
    /// Computes deterministic target URL in the cache directory based on uri and file name
    public func cacheFileURL(for uri: String, fileName: String) -> URL {
        let cleanUri = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        let hash = SHA256.hash(data: Data(cleanUri.utf8))
        let hashPrefix = hash.compactMap { String(format: "%02x", $0) }.prefix(8).joined()
        
        let rawExt = (fileName as NSString).pathExtension
        let ext = rawExt.isEmpty ? (cleanUri as NSString).pathExtension : rawExt
        let rawBase = (fileName as NSString).deletingPathExtension
        let base = (rawBase.isEmpty ? "document" : rawBase)
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: ":", with: "_")
            .replacingOccurrences(of: "?", with: "_")
            .replacingOccurrences(of: "&", with: "_")
        
        let finalFileName = ext.isEmpty ? "\(base)_\(hashPrefix)" : "\(base)_\(hashPrefix).\(ext)"
        return cacheDirectory.appendingPathComponent(finalFileName)
    }
    
    /// Checks if a valid non-empty cached file exists for the given uri/fileName
    public func getCachedFile(for uri: String, fileName: String) -> URL? {
        let fileURL = cacheFileURL(for: uri, fileName: fileName)
        guard fileManager.fileExists(atPath: fileURL.path) else { return nil }
        let attributes = try? fileManager.attributesOfItem(atPath: fileURL.path)
        let size = (attributes?[.size] as? Int64) ?? 0
        return size > 0 ? fileURL : nil
    }
    
    /// Saves a downloaded file into the cache location and returns the cached destination URL
    @discardableResult
    public func saveToCache(from sourceURL: URL, for uri: String, fileName: String) throws -> URL {
        let destination = cacheFileURL(for: uri, fileName: fileName)
        if fileManager.fileExists(atPath: destination.path) {
            try? fileManager.removeItem(at: destination)
        }
        try fileManager.moveItem(at: sourceURL, to: destination)
        return destination
    }
    
    /// Clears all cached documents from disk
    public func clearCache() {
        try? fileManager.removeItem(at: cacheDirectory)
        try? fileManager.createDirectory(at: cacheDirectory, withIntermediateDirectories: true)
    }
}
