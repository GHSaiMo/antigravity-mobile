import Foundation

public final class ProjectCacheManager: @unchecked Sendable {
    public static let shared = ProjectCacheManager()
    
    private let cacheFileURL: URL
    private let lock = NSLock()
    private let ioQueue = DispatchQueue(label: "com.antigravity.mobile.projects.cache.io", qos: .utility)
    
    private var memProjects: [ProjectItem]?
    
    private init() {
        let fm = FileManager.default
        let base = fm.urls(for: .cachesDirectory, in: .userDomainMask).first ?? fm.temporaryDirectory
        let dir = base.appendingPathComponent("AntigravityCache", isDirectory: true)
        try? fm.createDirectory(at: dir, withIntermediateDirectories: true)
        self.cacheFileURL = dir.appendingPathComponent("projects.json")
    }
    
    /// Loads cached projects from in-memory cache or local disk storage (projects.json).
    /// Returns an empty array if projects have never been fetched and cached before.
    public func loadProjects() -> [ProjectItem] {
        lock.lock()
        if let mem = memProjects, !mem.isEmpty {
            lock.unlock()
            return mem
        }
        lock.unlock()
        
        if let data = try? Data(contentsOf: cacheFileURL) {
            let decoder = JSONDecoder()
            decoder.dateDecodingStrategy = .iso8601
            if let decoded = try? decoder.decode([ProjectItem].self, from: data), !decoded.isEmpty {
                lock.lock()
                memProjects = decoded
                lock.unlock()
                return decoded
            }
        }
        
        return []
    }
    
    /// Persists project list to in-memory cache and asynchronously writes to local disk.
    public func saveProjects(_ items: [ProjectItem]) {
        guard !items.isEmpty else { return }
        
        lock.lock()
        memProjects = items
        lock.unlock()
        
        let fileURL = cacheFileURL
        ioQueue.async {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            if let data = try? encoder.encode(items) {
                try? data.write(to: fileURL, options: .atomic)
            }
        }
    }
    
    /// Asynchronously fetches upstream projects from the gateway and saves them to local cache.
    @discardableResult
    public func fetchAndCacheProjects(baseURL: URL? = nil) async -> [ProjectItem] {
        guard let url = baseURL ?? AppSettings.shared.gatewayURL else {
            return loadProjects()
        }
        
        do {
            let fetched = try await APIClient.shared.fetchProjects(baseURL: url)
            if !fetched.isEmpty {
                saveProjects(fetched)
                return fetched
            }
        } catch {
            // Keep existing local disk cache if network request fails
        }
        
        return loadProjects()
    }
}
