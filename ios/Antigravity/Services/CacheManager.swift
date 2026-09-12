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

public extension Notification.Name {
    static let conversationDraftChanged = Notification.Name("com.antigravity.mobile.draftChanged")
}

public final class CacheManager: @unchecked Sendable {
    public static let shared = CacheManager()
    
    private let cacheDir: URL
    private let lock = NSRecursiveLock()
    private let ioQueue = DispatchQueue(label: "com.antigravity.mobile.cache.io", qos: .utility)
    
    private var memConversations: [ConversationItem]?
    private var memSessions: [String: CachedChatSession] = [:]
    private var memLastViewDates: [String: Date] = [:]
    private var memDrafts: [String: String] = [:]
    private var memDraftImages: [String: [Data]] = [:]
    private var memLocalDraftSessions: [String: LocalDraftSession]?
    
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
        clearDraft(key: cascadeId)
        
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
        memDrafts.removeAll()
        memDraftImages.removeAll()
        memLocalDraftSessions?.removeAll()
        lock.unlock()
        
        let targetDir = cacheDir
        ioQueue.async {
            let fm = FileManager.default
            try? fm.removeItem(at: targetDir)
            try? fm.createDirectory(at: targetDir.appendingPathComponent("sessions", isDirectory: true), withIntermediateDirectories: true)
            try? fm.createDirectory(at: targetDir.appendingPathComponent("draft_images", isDirectory: true), withIntermediateDirectories: true)
        }
    }
    
    // MARK: - Drafts Cache
    
    public func getDraft(for key: String) -> String {
        guard !key.isEmpty else { return "" }
        lock.lock()
        defer { lock.unlock() }
        if let draft = memDrafts[key] {
            return draft
        }
        if let saved = UserDefaults.standard.string(forKey: "ag_draft_\(key)") {
            memDrafts[key] = saved
            return saved
        }
        return ""
    }
    
    public func hasDraft(for key: String) -> Bool {
        return !getDraft(for: key).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || hasDraftImages(for: key)
    }
    
    public func saveDraft(key: String, text: String) {
        guard !key.isEmpty else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty && !hasDraftImages(for: key) {
            clearDraft(key: key)
            return
        }
        
        lock.lock()
        let old = memDrafts[key]
        memDrafts[key] = text
        UserDefaults.standard.set(text, forKey: "ag_draft_\(key)")
        
        if key.hasPrefix("local_draft_") {
            ensureLocalDraftSessionsLoaded()
            if var session = memLocalDraftSessions?[key] {
                session.draftText = text
                session.updatedAt = Date()
                memLocalDraftSessions?[key] = session
                persistDraftSessionsToDisk()
            }
        }
        lock.unlock()
        
        if old != text {
            DispatchQueue.main.async {
                NotificationCenter.default.post(name: .conversationDraftChanged, object: key)
            }
        }
    }
    
    public func clearDraft(key: String) {
        guard !key.isEmpty else { return }
        lock.lock()
        let hadValue = (memDrafts[key] != nil) || (UserDefaults.standard.object(forKey: "ag_draft_\(key)") != nil) || hasDraftImages(for: key)
        memDrafts.removeValue(forKey: key)
        UserDefaults.standard.removeObject(forKey: "ag_draft_\(key)")
        if key.hasPrefix("local_draft_") {
            ensureLocalDraftSessionsLoaded()
            memLocalDraftSessions?.removeValue(forKey: key)
            persistDraftSessionsToDisk()
        }
        lock.unlock()
        
        clearDraftImages(key: key)
        
        if hadValue {
            DispatchQueue.main.async {
                NotificationCenter.default.post(name: .conversationDraftChanged, object: key)
            }
        }
    }
    
    // MARK: - Draft Images Cache
    
    private func draftImagesDir(for key: String) -> URL {
        let safeKey = key.replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: ":", with: "_")
        return cacheDir.appendingPathComponent("draft_images/\(safeKey)", isDirectory: true)
    }
    
    public func saveDraftImages(key: String, images: [Data]) {
        guard !key.isEmpty else { return }
        lock.lock()
        let oldImages = memDraftImages[key] ?? []
        memDraftImages[key] = images
        lock.unlock()
        
        let dir = draftImagesDir(for: key)
        ioQueue.async {
            let fm = FileManager.default
            if images.isEmpty {
                try? fm.removeItem(at: dir)
            } else {
                try? fm.createDirectory(at: dir, withIntermediateDirectories: true)
                if let existing = try? fm.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil) {
                    for file in existing {
                        try? fm.removeItem(at: file)
                    }
                }
                for (i, data) in images.enumerated() {
                    let file = dir.appendingPathComponent("\(i).jpg")
                    try? data.write(to: file, options: .atomic)
                }
            }
        }
        
        if oldImages.count != images.count {
            DispatchQueue.main.async {
                NotificationCenter.default.post(name: .conversationDraftChanged, object: key)
            }
        }
    }
    
    public func getDraftImages(for key: String) -> [Data] {
        guard !key.isEmpty else { return [] }
        lock.lock()
        if let mem = memDraftImages[key] {
            lock.unlock()
            return mem
        }
        lock.unlock()
        
        let dir = draftImagesDir(for: key)
        let fm = FileManager.default
        guard let files = try? fm.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil) else {
            return []
        }
        let sorted = files.sorted {
            let n1 = Int($0.deletingPathExtension().lastPathComponent) ?? 0
            let n2 = Int($1.deletingPathExtension().lastPathComponent) ?? 0
            return n1 < n2
        }
        var loaded: [Data] = []
        for file in sorted {
            if let d = try? Data(contentsOf: file) {
                loaded.append(d)
            }
        }
        lock.lock()
        memDraftImages[key] = loaded
        lock.unlock()
        return loaded
    }
    
    public func hasDraftImages(for key: String) -> Bool {
        guard !key.isEmpty else { return false }
        lock.lock()
        if let mem = memDraftImages[key] {
            let count = mem.count
            lock.unlock()
            return count > 0
        }
        lock.unlock()
        
        let dir = draftImagesDir(for: key)
        let fm = FileManager.default
        if let files = try? fm.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil), !files.isEmpty {
            return true
        }
        return false
    }
    
    public func clearDraftImages(key: String) {
        guard !key.isEmpty else { return }
        lock.lock()
        let hadImages = !(memDraftImages[key]?.isEmpty ?? true)
        memDraftImages.removeValue(forKey: key)
        lock.unlock()
        
        let dir = draftImagesDir(for: key)
        ioQueue.async {
            try? FileManager.default.removeItem(at: dir)
        }
        
        if hadImages {
            DispatchQueue.main.async {
                NotificationCenter.default.post(name: .conversationDraftChanged, object: key)
            }
        }
    }
    
    // MARK: - Local Draft Sessions Cache
    
    private func ensureLocalDraftSessionsLoaded() {
        if memLocalDraftSessions != nil { return }
        let fileURL = cacheDir.appendingPathComponent("draft_sessions.json")
        guard let data = try? Data(contentsOf: fileURL),
              let items = try? JSONDecoder().decode([LocalDraftSession].self, from: data) else {
            memLocalDraftSessions = [:]
            return
        }
        var dict: [String: LocalDraftSession] = [:]
        for item in items {
            dict[item.id] = item
        }
        memLocalDraftSessions = dict
    }
    
    private func persistDraftSessionsToDisk() {
        guard let dict = memLocalDraftSessions else { return }
        let list = Array(dict.values)
        guard let data = try? JSONEncoder().encode(list) else { return }
        let fileURL = cacheDir.appendingPathComponent("draft_sessions.json")
        ioQueue.async {
            try? data.write(to: fileURL, options: .atomic)
        }
    }
    
    public func createLocalDraftSession(project: ProjectItem) -> LocalDraftSession {
        let session = LocalDraftSession(project: project)
        lock.lock()
        ensureLocalDraftSessionsLoaded()
        memLocalDraftSessions?[session.id] = session
        persistDraftSessionsToDisk()
        lock.unlock()
        return session
    }
    
    public func saveLocalDraftSession(_ session: LocalDraftSession) {
        lock.lock()
        ensureLocalDraftSessionsLoaded()
        let trimmed = session.draftText.trimmingCharacters(in: .whitespacesAndNewlines)
        let hasImages = hasDraftImages(for: session.id)
        if trimmed.isEmpty && !hasImages {
            memLocalDraftSessions?.removeValue(forKey: session.id)
        } else {
            memLocalDraftSessions?[session.id] = session
        }
        persistDraftSessionsToDisk()
        lock.unlock()
    }
    
    public func getLocalDraftSession(id: String) -> LocalDraftSession? {
        lock.lock()
        defer { lock.unlock() }
        ensureLocalDraftSessionsLoaded()
        return memLocalDraftSessions?[id]
    }
    
    public func loadLocalDraftSessions() -> [LocalDraftSession] {
        lock.lock()
        defer { lock.unlock() }
        ensureLocalDraftSessionsLoaded()
        guard let dict = memLocalDraftSessions else { return [] }
        return dict.values.filter { session in
            let text = memDrafts[session.id] ?? UserDefaults.standard.string(forKey: "ag_draft_\(session.id)") ?? session.draftText
            let hasImages = hasDraftImages(for: session.id)
            return !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || hasImages
        }.sorted { $0.updatedAt > $1.updatedAt }
    }
    
    public func loadLocalDraftConversations() -> [ConversationItem] {
        let active = loadLocalDraftSessions()
        return active.map { session in
            var s = session
            let text = getDraft(for: session.id)
            if !text.isEmpty {
                s.draftText = text
            }
            return s.toConversationItem()
        }
    }
    
    public func deleteLocalDraftSession(id: String) {
        lock.lock()
        ensureLocalDraftSessionsLoaded()
        memLocalDraftSessions?.removeValue(forKey: id)
        persistDraftSessionsToDisk()
        lock.unlock()
        clearDraftImages(key: id)
    }
}
