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
    public func cacheFileURL(for uri: String, fileName: String, cascadeId: String? = nil) -> URL {
        let cleanUri = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        let scopeKey: String
        if let cid = cascadeId?.trimmingCharacters(in: .whitespacesAndNewlines), !cid.isEmpty, !cleanUri.contains(cid) {
            scopeKey = "\(cid)_\(cleanUri)"
        } else {
            scopeKey = cleanUri
        }
        let hash = SHA256.hash(data: Data(scopeKey.utf8))
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
    
    private func metadataURL(for fileURL: URL) -> URL {
        return fileURL.appendingPathExtension("meta.json")
    }
    
    /// Checks if a valid non-empty cached file exists for the given uri/fileName
    public func getCachedFile(for uri: String, fileName: String, cascadeId: String? = nil) -> URL? {
        let fileURL = cacheFileURL(for: uri, fileName: fileName, cascadeId: cascadeId)
        if fileManager.fileExists(atPath: fileURL.path) {
            let attributes = try? fileManager.attributesOfItem(atPath: fileURL.path)
            let size = (attributes?[.size] as? Int64) ?? 0
            if size > 0 { return fileURL }
        }
        // Fallback to unscoped check if cascadeId was supplied
        if cascadeId != nil {
            let unscopedURL = cacheFileURL(for: uri, fileName: fileName, cascadeId: nil)
            if fileManager.fileExists(atPath: unscopedURL.path) {
                let attributes = try? fileManager.attributesOfItem(atPath: unscopedURL.path)
                let size = (attributes?[.size] as? Int64) ?? 0
                if size > 0 { return unscopedURL }
            }
        }
        return nil
    }
    
    /// Saves a downloaded file into the cache location and returns the cached destination URL
    @discardableResult
    public func saveToCache(from sourceURL: URL, for uri: String, fileName: String, cascadeId: String? = nil) throws -> URL {
        let destination = cacheFileURL(for: uri, fileName: fileName, cascadeId: cascadeId)
        if fileManager.fileExists(atPath: destination.path) {
            try? fileManager.removeItem(at: destination)
        }
        try fileManager.moveItem(at: sourceURL, to: destination)
        return destination
    }
    
    // MARK: - Markdown Documents Persistent Cache
    
    /// Saves markdown document content and metadata to local disk cache
    @discardableResult
    public func saveMarkdownToCache(
        response: FileContentResponse,
        for uri: String,
        fileName: String,
        cascadeId: String? = nil
    ) throws -> (fileURL: URL, response: FileContentResponse) {
        var resolvedName = fileName.trimmingCharacters(in: .whitespacesAndNewlines)
        if resolvedName.isEmpty {
            resolvedName = "document.md"
        } else if !(resolvedName as NSString).pathExtension.lowercased().contains("md") {
            resolvedName = "\(resolvedName).md"
        }
        let fileURL = cacheFileURL(for: uri, fileName: resolvedName, cascadeId: cascadeId)
        guard let data = response.content.data(using: .utf8) else {
            throw NSError(domain: "DocumentCacheManager", code: -1, userInfo: [NSLocalizedDescriptionKey: "Invalid markdown content encoding"])
        }
        if fileManager.fileExists(atPath: fileURL.path) {
            try? fileManager.removeItem(at: fileURL)
        }
        try data.write(to: fileURL, options: .atomic)
        
        let meta = metadataURL(for: fileURL)
        if let metaData = try? JSONEncoder().encode(response) {
            if fileManager.fileExists(atPath: meta.path) {
                try? fileManager.removeItem(at: meta)
            }
            try? metaData.write(to: meta, options: .atomic)
        }
        return (fileURL, response)
    }
    
    /// Retrieves cached markdown document content and metadata if available on disk
    public func getCachedMarkdown(
        for uri: String,
        fileName: String,
        cascadeId: String? = nil
    ) -> (fileURL: URL, response: FileContentResponse)? {
        var resolvedName = fileName.trimmingCharacters(in: .whitespacesAndNewlines)
        if resolvedName.isEmpty {
            resolvedName = "document.md"
        } else if !(resolvedName as NSString).pathExtension.lowercased().contains("md") {
            resolvedName = "\(resolvedName).md"
        }
        
        let checkCandidate: (URL) -> (fileURL: URL, response: FileContentResponse)? = { url in
            guard self.fileManager.fileExists(atPath: url.path) else { return nil }
            let meta = self.metadataURL(for: url)
            if self.fileManager.fileExists(atPath: meta.path),
               let metaData = try? Data(contentsOf: meta),
               let decoded = try? JSONDecoder().decode(FileContentResponse.self, from: metaData),
               !decoded.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return (url, decoded)
            }
            if let text = try? String(contentsOf: url, encoding: .utf8), !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                let fallbackResp = FileContentResponse(
                    uri: uri,
                    filename: resolvedName,
                    content: text,
                    summary: nil,
                    requestFeedback: nil,
                    userFacing: nil
                )
                return (url, fallbackResp)
            }
            return nil
        }
        
        let fileURL = cacheFileURL(for: uri, fileName: resolvedName, cascadeId: cascadeId)
        if let hit = checkCandidate(fileURL) {
            return hit
        }
        if cascadeId != nil {
            let fallbackURL = cacheFileURL(for: uri, fileName: resolvedName, cascadeId: nil)
            if let hit = checkCandidate(fallbackURL) {
                return hit
            }
        }
        return nil
    }
    
    /// Clears all cached documents from disk
    public func clearCache() {
        try? fileManager.removeItem(at: cacheDirectory)
        try? fileManager.createDirectory(at: cacheDirectory, withIntermediateDirectories: true)
    }
}
