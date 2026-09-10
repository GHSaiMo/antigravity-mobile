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
        
        // Immediate local cache restore (filtering out subagents)
        let cached = self.cacheManager.loadConversations().filter { !$0.isSubagent }
        if !cached.isEmpty {
            self.conversations = cached
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
    
    public var filteredConversations: [ConversationItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let nonSubagents = conversations.filter { !$0.isSubagent }
        if q.isEmpty { return nonSubagents }
        return nonSubagents.filter {
            $0.title.lowercased().contains(q) ||
            $0.workspaceName.lowercased().contains(q) ||
            $0.id.lowercased().contains(q)
        }
    }
    
    public func reloadFromCache() {
        let cached = cacheManager.loadConversations().filter { !$0.isSubagent }
        if !cached.isEmpty {
            self.conversations = cached
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
            let cached = cacheManager.loadConversations().filter { !$0.isSubagent }
            if !cached.isEmpty {
                self.conversations = cached
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
            let cleaned = items.filter { !$0.isSubagent }
            self.conversations = cleaned
            cacheManager.saveConversations(cleaned)
            cacheManager.prewarmSessions(for: cleaned.prefix(15).map(\.id))
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
    
    public func switchCockpitAccount(id: String) async throws {
        guard let url = settings.serverURL else { return }
        try await apiClient.switchCockpitAccount(baseURL: url, accountId: id)
        await fetchQuotas(force: true)
    }
}
