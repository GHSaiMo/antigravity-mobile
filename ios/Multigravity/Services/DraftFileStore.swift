import Foundation

/// Persists the files attached to a draft: the local copies plus a `meta.json` describing them.
///
/// Lives in Application Support (not Caches) so that a pending upload is not purged by the system.
public final class DraftFileStore: @unchecked Sendable {
    public static let shared = DraftFileStore()
    
    private let root: URL
    private let lock = NSLock()
    
    private init() {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        root = base.appendingPathComponent("draft_files", isDirectory: true)
        try? FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    }
    
    public func directory(for key: String) -> URL {
        let safe = key.replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: ":", with: "_")
        return root.appendingPathComponent(safe, isDirectory: true)
    }
    
    public func localURL(for file: DraftFile, key: String) -> URL {
        directory(for: key).appendingPathComponent(file.fileName)
    }
    
    public func save(key: String, files: [DraftFile]) {
        guard !key.isEmpty else { return }
        lock.lock(); defer { lock.unlock() }
        let dir = directory(for: key)
        let meta = dir.appendingPathComponent("meta.json")
        if files.isEmpty {
            try? FileManager.default.removeItem(at: dir)
            return
        }
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        if let data = try? JSONEncoder().encode(files) {
            try? data.write(to: meta, options: .atomic)
        }
    }
    
    /// Loads a draft's files. Entries that were not fully uploaded and whose local copy is gone are dropped.
    public func load(key: String) -> [DraftFile] {
        guard !key.isEmpty else { return [] }
        lock.lock(); defer { lock.unlock() }
        let dir = directory(for: key)
        guard let data = try? Data(contentsOf: dir.appendingPathComponent("meta.json")),
              let files = try? JSONDecoder().decode([DraftFile].self, from: data) else { return [] }
        return files.filter {
            $0.isUploaded || FileManager.default.fileExists(atPath: dir.appendingPathComponent($0.fileName).path)
        }
    }
    
    public func hasFiles(for key: String) -> Bool {
        !load(key: key).isEmpty
    }
    
    /// Removes the metadata and local copies of a draft's files.
    public func clear(key: String) {
        guard !key.isEmpty else { return }
        lock.lock(); defer { lock.unlock() }
        try? FileManager.default.removeItem(at: directory(for: key))
    }
}
