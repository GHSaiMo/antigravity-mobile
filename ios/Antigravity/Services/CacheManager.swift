import Foundation

public nonisolated struct CachedChatSession: Codable, Sendable {
    public let cascadeId: String
    public let status: String
    public let duration: String
    public let stepCount: Int
    public let totalTools: Int
    public let hasMore: Bool
    public let nextOffset: Int
    public let messages: [ChatMessage]
    public let title: String?
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    public let runningTasks: [RunningTaskItem]?
    public let savedAt: Date
    
    public init(
        cascadeId: String,
        status: String,
        duration: String,
        stepCount: Int,
        totalTools: Int,
        hasMore: Bool,
        nextOffset: Int,
        messages: [ChatMessage],
        title: String? = nil,
        cascadeConfigRaw: String? = nil,
        canProceed: Bool? = nil,
        proceedArtifactUri: String? = nil,
        pendingInteraction: PendingInteraction? = nil,
        queuedMessages: [QueuedMessageItem]? = nil,
        runningTasks: [RunningTaskItem]? = nil,
        savedAt: Date = Date()
    ) {
        self.cascadeId = cascadeId
        self.status = status
        self.duration = duration
        self.stepCount = stepCount
        self.totalTools = totalTools
        self.hasMore = hasMore
        self.nextOffset = nextOffset
        self.messages = messages
        self.title = title
        self.cascadeConfigRaw = cascadeConfigRaw
        self.canProceed = canProceed
        self.proceedArtifactUri = proceedArtifactUri
        self.pendingInteraction = pendingInteraction
        self.queuedMessages = queuedMessages
        self.runningTasks = runningTasks
        self.savedAt = savedAt
    }
}

public final class CacheManager: @unchecked Sendable {
    public static let shared = CacheManager()
    
    private let cacheDir: URL
    private let lock = NSLock()
    private let ioQueue = DispatchQueue(label: "com.antigravity.mobile.cache.io", qos: .utility)
    
    private var memConversations: [ConversationItem]?
    private var memSessions: [String: CachedChatSession] = [:]
    private var memLastViewDates: [String: Date] = [:]
    
    public func getLastViewDate(for cascadeId: String) -> Date? {
        lock.lock()
        defer { lock.unlock() }
        if let d = memLastViewDates[cascadeId] {
            return d
        }
        if let ts = UserDefaults.standard.object(forKey: "ag_last_view_\(cascadeId)") as? Date {
            memLastViewDates[cascadeId] = ts
            return ts
        }
        return nil
    }
    
    public func markConversationAsRead(cascadeId: String) {
        lock.lock()
        let now = Date()
        memLastViewDates[cascadeId] = now
        UserDefaults.standard.set(now, forKey: "ag_last_view_\(cascadeId)")
        
        if var items = memConversations, let idx = items.firstIndex(where: { $0.id == cascadeId }) {
            let old = items[idx]
            items[idx] = ConversationItem(
                id: old.id,
                title: old.title,
                status: old.status,
                stepCount: old.stepCount,
                workspaceName: old.workspaceName,
                lastModified: old.lastModified,
                isSubagent: old.isSubagent,
                isUnread: false
            )
            memConversations = items
        }
        lock.unlock()
    }
    
    private init() {
        let fm = FileManager.default
        let base = fm.urls(for: .cachesDirectory, in: .userDomainMask).first ?? fm.temporaryDirectory
        let dir = base.appendingPathComponent("AntigravityCache", isDirectory: true)
        self.cacheDir = dir
        try? fm.createDirectory(at: dir.appendingPathComponent("sessions", isDirectory: true), withIntermediateDirectories: true)
    }
    
    // MARK: - Conversations List Cache
    
    public func saveConversations(_ items: [ConversationItem]) {
        let clean = items.filter { !$0.isSubagent }
        lock.lock()
        memConversations = clean
        lock.unlock()
        
        guard let data = try? JSONEncoder().encode(clean) else { return }
        let fileURL = cacheDir.appendingPathComponent("conversations.json")
        ioQueue.async {
            try? data.write(to: fileURL, options: .atomic)
        }
    }
    
    public func loadConversations() -> [ConversationItem] {
        lock.lock()
        if let mem = memConversations {
            let filtered = mem.filter { !$0.isSubagent }
            memConversations = filtered
            lock.unlock()
            return filtered
        }
        lock.unlock()
        
        let fileURL = cacheDir.appendingPathComponent("conversations.json")
        guard let data = try? Data(contentsOf: fileURL),
              let items = try? JSONDecoder().decode([ConversationItem].self, from: data) else {
            return []
        }
        
        let filtered = items.filter { !$0.isSubagent }
        lock.lock()
        memConversations = filtered
        lock.unlock()
        
        // If legacy subagents were pruned, rewrite clean data to disk asynchronously
        if filtered.count != items.count {
            if let cleanData = try? JSONEncoder().encode(filtered) {
                ioQueue.async {
                    try? cleanData.write(to: fileURL, options: .atomic)
                }
            }
        }
        
        return filtered
    }
    
    public func updateConversationTitle(cascadeId: String, newTitle: String) {
        lock.lock()
        defer { lock.unlock() }
        guard !newTitle.isEmpty, newTitle != "未命名会话" else { return }
        
        var items = memConversations ?? []
        if items.isEmpty {
            let fileURL = cacheDir.appendingPathComponent("conversations.json")
            if let data = try? Data(contentsOf: fileURL),
               let loaded = try? JSONDecoder().decode([ConversationItem].self, from: data) {
                items = loaded
            }
        }
        
        if let idx = items.firstIndex(where: { $0.id == cascadeId }) {
            let old = items[idx]
            items[idx] = ConversationItem(
                id: old.id,
                title: newTitle,
                status: old.status,
                stepCount: old.stepCount,
                workspaceName: old.workspaceName,
                lastModified: old.lastModified
            )
            memConversations = items
            if let data = try? JSONEncoder().encode(items) {
                let fileURL = cacheDir.appendingPathComponent("conversations.json")
                ioQueue.async {
                    try? data.write(to: fileURL, options: .atomic)
                }
            }
        }
    }
    
    public func updateConversationStatus(cascadeId: String, status: ConversationItem.ConversationStatus) {
        lock.lock()
        defer { lock.unlock() }
        
        var items = memConversations ?? []
        if items.isEmpty {
            let fileURL = cacheDir.appendingPathComponent("conversations.json")
            if let data = try? Data(contentsOf: fileURL),
               let loaded = try? JSONDecoder().decode([ConversationItem].self, from: data) {
                items = loaded
            }
        }
        
        if let idx = items.firstIndex(where: { $0.id == cascadeId }) {
            let old = items[idx]
            if old.status != status {
                items[idx] = ConversationItem(
                    id: old.id,
                    title: old.title,
                    status: status,
                    stepCount: old.stepCount,
                    workspaceName: old.workspaceName,
                    lastModified: old.lastModified
                )
                memConversations = items
                if let data = try? JSONEncoder().encode(items) {
                    let fileURL = cacheDir.appendingPathComponent("conversations.json")
                    ioQueue.async {
                        try? data.write(to: fileURL, options: .atomic)
                    }
                }
            }
        }
    }
    
    public func upsertConversation(_ item: ConversationItem) {
        if item.isSubagent {
            return
        }
        lock.lock()
        defer { lock.unlock() }
        
        var items = memConversations ?? []
        if items.isEmpty {
            let fileURL = cacheDir.appendingPathComponent("conversations.json")
            if let data = try? Data(contentsOf: fileURL),
               let loaded = try? JSONDecoder().decode([ConversationItem].self, from: data) {
                items = loaded
            }
        }
        
        if let idx = items.firstIndex(where: { $0.id == item.id }) {
            items[idx] = item
        } else {
            items.insert(item, at: 0)
        }
        memConversations = items
        if let data = try? JSONEncoder().encode(items) {
            let fileURL = cacheDir.appendingPathComponent("conversations.json")
            ioQueue.async {
                try? data.write(to: fileURL, options: .atomic)
            }
        }
    }
    
    public func deleteConversation(cascadeId: String) {
        lock.lock()
        defer { lock.unlock() }
        
        var items = memConversations ?? []
        if items.isEmpty {
            let fileURL = cacheDir.appendingPathComponent("conversations.json")
            if let data = try? Data(contentsOf: fileURL),
               let loaded = try? JSONDecoder().decode([ConversationItem].self, from: data) {
                items = loaded
            }
        }
        
        items.removeAll(where: { $0.id == cascadeId })
        memConversations = items
        memSessions.removeValue(forKey: cascadeId)
        memLastViewDates.removeValue(forKey: cascadeId)
        UserDefaults.standard.removeObject(forKey: "ag_last_view_\(cascadeId)")
        
        let convFileURL = cacheDir.appendingPathComponent("conversations.json")
        let sessionFileURL = cacheDir.appendingPathComponent("sessions/\(cascadeId).json")
        
        if let data = try? JSONEncoder().encode(items) {
            ioQueue.async {
                try? data.write(to: convFileURL, options: .atomic)
                try? FileManager.default.removeItem(at: sessionFileURL)
            }
        } else {
            ioQueue.async {
                try? FileManager.default.removeItem(at: sessionFileURL)
            }
        }
    }
    
    // MARK: - Chat Session Cache
    
    public func saveSession(_ session: CachedChatSession) {
        lock.lock()
        memSessions[session.cascadeId] = session
        lock.unlock()
        
        guard let data = try? JSONEncoder().encode(session) else { return }
        let fileURL = cacheDir.appendingPathComponent("sessions/\(session.cascadeId).json")
        ioQueue.async {
            try? data.write(to: fileURL, options: .atomic)
        }
    }
    
    public func loadSession(for cascadeId: String) -> CachedChatSession? {
        lock.lock()
        if let mem = memSessions[cascadeId] {
            lock.unlock()
            return mem
        }
        lock.unlock()
        
        let fileURL = cacheDir.appendingPathComponent("sessions/\(cascadeId).json")
        guard let data = try? Data(contentsOf: fileURL),
              let session = try? JSONDecoder().decode(CachedChatSession.self, from: data) else {
            return nil
        }
        
        lock.lock()
        memSessions[cascadeId] = session
        lock.unlock()
        return session
    }
    
    public func prewarmSessions(for cascadeIds: [String]) {
        guard !cascadeIds.isEmpty else { return }
        ioQueue.async { [weak self] in
            guard let self = self else { return }
            for cid in cascadeIds {
                self.lock.lock()
                let exists = self.memSessions[cid] != nil
                self.lock.unlock()
                if exists { continue }
                
                let fileURL = self.cacheDir.appendingPathComponent("sessions/\(cid).json")
                if let data = try? Data(contentsOf: fileURL),
                   let session = try? JSONDecoder().decode(CachedChatSession.self, from: data) {
                    self.lock.lock()
                    self.memSessions[cid] = session
                    self.lock.unlock()
                }
            }
        }
    }
    
    public func clearCache() {
        lock.lock()
        memConversations = nil
        memSessions.removeAll()
        lock.unlock()
        
        let targetDir = cacheDir
        ioQueue.async {
            let fm = FileManager.default
            try? fm.removeItem(at: targetDir)
            try? fm.createDirectory(at: targetDir.appendingPathComponent("sessions", isDirectory: true), withIntermediateDirectories: true)
        }
    }
}
