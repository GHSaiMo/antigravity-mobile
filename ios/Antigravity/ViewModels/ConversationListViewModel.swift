import Foundation
import Observation

@Observable
@MainActor
public final class ConversationListViewModel {
    public var conversations: [ConversationItem] = []
    public var searchQuery: String = ""
    public var isLoading: Bool = false
    public var errorMessage: String? = nil
    
    public var quotaResponse: CockpitQuotaResponse? = nil
    
    private let apiClient: APIClient
    private let settings: AppSettings
    private let cacheManager: CacheManager
    
    // Tombstones to prevent race conditions & data reflow from polling while deleting
    private var pendingDeleteCascadeIDs: Set<String> = []
    private var recentlyDeletedIDs: [String: Date] = [:]
    private let tombstoneTTL: TimeInterval = 30.0
    
    private var pollTask: Task<Void, Never>? = nil
    private var lastResumeTime: Date = .distantPast
    
    public init(
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.cacheManager = cacheManager ?? .shared
        
        // Immediate local cache restore (filtering out subagents and loading drafts)
        let drafts = self.cacheManager.loadLocalDraftConversations()
        let cached = self.cacheManager.loadConversations().filter { !$0.isSubagent }
        if !cached.isEmpty || !drafts.isEmpty {
            self.conversations = drafts + cached
            self.cacheManager.prewarmSessions(for: cached.prefix(15).map(\.id))
        }
        
        NotificationCenter.default.addObserver(
            forName: .networkRoutingPreferenceChanged,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            guard let self = self else { return }
            Task { @MainActor in
                await self.fetchConversations()
            }
        }
    }
    
    private func purgeExpiredTombstones() {
        let now = Date()
        recentlyDeletedIDs = recentlyDeletedIDs.filter { now.timeIntervalSince($0.value) < tombstoneTTL }
    }
    
    public var filteredConversations: [ConversationItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let nonSubagents = conversations.filter { item in
            !item.isSubagent &&
            !pendingDeleteCascadeIDs.contains(item.id) &&
            (recentlyDeletedIDs[item.id] == nil)
        }
        if q.isEmpty { return nonSubagents }
        return nonSubagents.filter {
            $0.title.lowercased().contains(q) ||
            $0.workspaceName.lowercased().contains(q) ||
            $0.id.lowercased().contains(q)
        }
    }
    
    public func reloadFromCache() {
        let drafts = cacheManager.loadLocalDraftConversations()
        let cached = cacheManager.loadConversations().filter { !$0.isSubagent }
        if !cached.isEmpty || !drafts.isEmpty {
            self.conversations = drafts + cached
        }
    }
    
    @MainActor
    public func resumeActive() async {
        let now = Date()
        guard now.timeIntervalSince(lastResumeTime) > 1.0 else { return }
        lastResumeTime = now
        
        reloadFromCache()
        await fetchConversations(isBackgroundPoll: !conversations.isEmpty)
    }
    
    public func startAutoRefresh() {
        guard pollTask == nil else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                let hasRunning = self?.conversations.contains(where: { $0.status.isRunning || $0.status.needsAction }) ?? false
                let delaySeconds: UInt64 = hasRunning ? 4 : 10
                try? await Task.sleep(nanoseconds: delaySeconds * 1_000_000_000)
                guard let self, !Task.isCancelled else { break }
                await self.fetchConversations(isBackgroundPoll: true)
            }
        }
    }
    
    public func stopAutoRefresh() {
        pollTask?.cancel()
        pollTask = nil
    }
    
    @MainActor
    public func fetchConversations(isBackgroundPoll: Bool = false) async {
        if conversations.isEmpty {
            let drafts = cacheManager.loadLocalDraftConversations()
            let cached = cacheManager.loadConversations().filter { !$0.isSubagent }
            if !cached.isEmpty || !drafts.isEmpty {
                self.conversations = drafts + cached
                self.cacheManager.prewarmSessions(for: cached.prefix(15).map(\.id))
            }
        }
        
        guard let url = settings.serverURL else {
            if conversations.isEmpty {
                self.errorMessage = "请在设置中配置有效的服务器地址"
            }
            return
        }
        
        if !isBackgroundPoll && conversations.isEmpty {
            self.isLoading = true
        }
        if !isBackgroundPoll {
            self.errorMessage = nil
        }
        
        do {
            let items = try await apiClient.fetchConversations(baseURL: url)
            purgeExpiredTombstones()
            let cleaned = items.filter { item in
                !item.isSubagent &&
                !self.pendingDeleteCascadeIDs.contains(item.id) &&
                (self.recentlyDeletedIDs[item.id] == nil)
            }
            let existingMap = Dictionary(self.conversations.map { ($0.id, $0.title) }, uniquingKeysWith: { _, new in new })
            let enriched = cleaned.map { item -> ConversationItem in
                let currentT = item.title.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
                if currentT.isEmpty || currentT == "未命名会话" {
                    if let knownTitle = existingMap[item.id], !knownTitle.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty, knownTitle != "未命名会话" {
                        return item.withTitle(knownTitle)
                    }
                    if let cached = self.cacheManager.loadSession(for: item.id) {
                        if let st = cached.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines), !st.isEmpty, st != "未命名会话" {
                            return item.withTitle(st)
                        } else if let firstUserMsg = cached.messages.first(where: { $0.isUser }),
                                  let prompt = firstUserMsg.content.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).components(separatedBy: CharacterSet.newlines).first(where: { !$0.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty }) {
                            let trimmed = prompt.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
                            let derived = String(trimmed.prefix(36))
                            if !derived.isEmpty {
                                return item.withTitle(derived)
                            }
                        }
                    }
                }
                return item
            }
            let drafts = cacheManager.loadLocalDraftConversations()
            self.conversations = drafts + enriched
            cacheManager.saveConversations(enriched)
            cacheManager.prewarmSessions(for: enriched.prefix(15).map(\.id))
            self.isLoading = false
        } catch {
            if conversations.isEmpty {
                self.errorMessage = error.localizedDescription
            }
            self.isLoading = false
        }
        
        await fetchQuotas()
    }
    
    private var lastQuotaFetchTime: Date = .distantPast
    private let quotaRefreshInterval: TimeInterval = 300 // 5 minutes
    
    @MainActor
    public func fetchQuotas(force: Bool = false) async {
        let now = Date()
        if !force && now.timeIntervalSince(lastQuotaFetchTime) < quotaRefreshInterval && quotaResponse != nil {
            return
        }
        guard let url = settings.serverURL else { return }
        do {
            self.quotaResponse = try await apiClient.fetchCockpitQuotas(baseURL: url)
            self.lastQuotaFetchTime = now
        } catch {
            // Silently ignore quota fetch error to not disturb chat list
        }
    }
    
    @MainActor
    public func switchCockpitAccount(id: String) async throws {
        guard let url = settings.serverURL else { return }
        try await apiClient.switchCockpitAccount(baseURL: url, accountId: id)
        await fetchQuotas(force: true)
    }
    
    @MainActor
    public func refreshCockpitQuotas() async throws {
        guard let url = settings.serverURL else { return }
        let initialUpdatedAt = self.quotaResponse?.updatedAt ?? 0
        
        // 1. Trigger refresh on gateway (gateway will wait up to 10s for updates)
        let directRes = try? await apiClient.refreshCockpitQuotas(baseURL: url)
        if let direct = directRes, direct.updatedAt > initialUpdatedAt {
            self.quotaResponse = direct
            self.lastQuotaFetchTime = Date()
            return
        }
        
        // 2. Poll for updated data if background batch refresh across accounts takes longer
        let startTime = Date()
        while Date().timeIntervalSince(startTime) < 20 {
            try await Task.sleep(nanoseconds: 1_500_000_000)
            if let res = try? await apiClient.fetchCockpitQuotas(baseURL: url) {
                if res.updatedAt > initialUpdatedAt {
                    self.quotaResponse = res
                    self.lastQuotaFetchTime = Date()
                    return
                }
            }
        }
        
        // Final fallback fetch
        if let res = try? await apiClient.fetchCockpitQuotas(baseURL: url) {
            self.quotaResponse = res
            self.lastQuotaFetchTime = Date()
        }
    }
    
    @MainActor
    public func deleteConversation(item: ConversationItem) async {
        if item.isDraft {
            cacheManager.deleteLocalDraftSession(id: item.id)
            cacheManager.clearDraft(key: item.id)
            self.conversations.removeAll(where: { $0.id == item.id })
            return
        }
        
        guard let url = settings.serverURL else {
            self.errorMessage = "请在设置中配置有效的服务器地址"
            return
        }
        
        // Immediately record tombstone to prevent polling/concurrent responses from reviving it
        pendingDeleteCascadeIDs.insert(item.id)
        recentlyDeletedIDs[item.id] = Date()
        
        let originalConversations = self.conversations
        // Optimistic UI removal
        self.conversations.removeAll(where: { $0.id == item.id })
        self.cacheManager.deleteConversation(cascadeId: item.id)
        
        do {
            try await apiClient.deleteConversation(cascadeId: item.id, baseURL: url)
            // Succeeded: remove from active pending, but retain in recentlyDeletedIDs for tombstoneTTL
            self.pendingDeleteCascadeIDs.remove(item.id)
        } catch {
            // Revert on failure
            self.pendingDeleteCascadeIDs.remove(item.id)
            self.recentlyDeletedIDs.removeValue(forKey: item.id)
            self.conversations = originalConversations
            self.cacheManager.saveConversations(originalConversations)
            self.errorMessage = "删除会话失败: \(error.localizedDescription)"
        }
    }
    
    @MainActor
    public func renameConversation(item: ConversationItem, newTitle: String) async {
        guard !item.isDraft else { return }
        let trimmed = newTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        guard trimmed != item.title else { return }
        
        guard let url = settings.serverURL else {
            self.errorMessage = "请在设置中配置有效的服务器地址"
            return
        }
        
        let originalTitle = item.title
        // Optimistic UI update
        if let idx = self.conversations.firstIndex(where: { $0.id == item.id }) {
            let old = self.conversations[idx]
            self.conversations[idx] = ConversationItem(
                id: old.id,
                title: trimmed,
                status: old.status,
                stepCount: old.stepCount,
                workspaceName: old.workspaceName,
                lastModified: old.lastModified,
                isSubagent: old.isSubagent,
                isUnread: old.isUnread
            )
        }
        self.cacheManager.updateConversationTitle(cascadeId: item.id, newTitle: trimmed)
        
        do {
            try await apiClient.renameConversation(cascadeId: item.id, newTitle: trimmed, baseURL: url)
        } catch {
            // Revert on failure
            if let idx = self.conversations.firstIndex(where: { $0.id == item.id }) {
                let old = self.conversations[idx]
                self.conversations[idx] = ConversationItem(
                    id: old.id,
                    title: originalTitle,
                    status: old.status,
                    stepCount: old.stepCount,
                    workspaceName: old.workspaceName,
                    lastModified: old.lastModified,
                    isSubagent: old.isSubagent,
                    isUnread: old.isUnread
                )
            }
            self.cacheManager.updateConversationTitle(cascadeId: item.id, newTitle: originalTitle)
            self.errorMessage = "重命名会话失败: \(error.localizedDescription)"
        }
    }
}
