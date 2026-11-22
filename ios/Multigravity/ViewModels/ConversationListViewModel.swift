import Foundation
import Observation
import SwiftUI

@Observable
@MainActor
public final class ConversationListViewModel {
    public enum WorkspaceFilter: Equatable, Hashable, Sendable {
        case chat
        case workspace(String)

        public var displayName: String {
            switch self {
            case .chat:
                return "Chat"
            case .workspace(let name):
                return name
            }
        }

        public var isPureChat: Bool {
            if case .chat = self { return true }
            return false
        }

        public func matches(_ item: ConversationItem) -> Bool {
            switch self {
            case .chat:
                return item.isPureChat
            case .workspace(let name):
                return !item.isPureChat && item.workspaceName == name
            }
        }
    }

    public struct WorkspaceFilterOption: Identifiable, Hashable, Sendable {
        public let filter: WorkspaceFilter
        public let count: Int
        public let latestModified: Date

        public var id: String {
            switch filter {
            case .chat:
                return "__pure_chat__"
            case .workspace(let name):
                return "ws:\(name)"
            }
        }
    }

    public var conversations: [ConversationItem] = []
    public var searchQuery: String = ""
    public var selectedWorkspaceFilter: WorkspaceFilter? = nil
    public var isLoading: Bool = false
    public var errorMessage: String? = nil
    
    public var quotaResponse: CockpitQuotaResponse? = nil
    public var tokenHistory: TokenHistoryResponse? = nil
    public var isLoadingTokenHistory: Bool = false
    public var globalBackgroundTasks: [GlobalBackgroundTaskItem] = []
    
    private let apiClient: APIClient
    private let settings: AppSettings
    private let cacheManager: CacheManager
    
    // Tombstones to prevent race conditions & data reflow from polling while deleting
    private var pendingDeleteCascadeIDs: Set<String> = []
    private var recentlyDeletedIDs: [String: Date] = [:]
    private let tombstoneTTL: TimeInterval = 600.0 // 10 minutes
    
    private var pollTask: Task<Void, Never>? = nil
    /// Gateway push hints for list changes; polling is the fallback and a slow safety net.
    private let eventsClient = ConversationEventsClient()
    /// Heartbeat monitor backing the connection dot in the home quota bar.
    public var linkMonitor: ConversationEventsClient { eventsClient }
    /// While the event socket is up, pushed hints drive refreshes and polling only runs this slowly.
    private static let eventsSafetyPollSeconds = 60
    /// Cooldown window for background list reordering so concurrent running sessions don't swap rows rapidly.
    private static let reorderCooldownSeconds: TimeInterval = 8.0
    private var lastReorderUptime: TimeInterval = 0
    private var pendingReorderTask: Task<Void, Never>? = nil
    private var lastCompatRefresh: Date = .distantPast
    private var lastResumeTime: Date = .distantPast
    
    public init(
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.cacheManager = cacheManager ?? .shared
        
        // Immediate local cache restore (filtering out subagents, drafts from cached, and deduplicating)
        let drafts = self.cacheManager.loadLocalDraftConversations()
        let cached = self.cacheManager.loadConversations().filter { !$0.isSubagent && !$0.isDraft }
        let draftIds = Set(drafts.map(\.id))
        let nonDraftCached = cached.filter { !draftIds.contains($0.id) }
        if !nonDraftCached.isEmpty || !drafts.isEmpty {
            let pinnedIds = self.cacheManager.getPinnedConversationIDs()
            let all = (drafts + nonDraftCached).map { $0.withPinned(pinnedIds.contains($0.id)) }
            self.conversations = Self.sortItems(all)
            self.lastReorderUptime = ProcessInfo.processInfo.systemUptime
            self.cacheManager.prewarmSessions(for: nonDraftCached.prefix(15).map(\.id))
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
        
        NotificationCenter.default.addObserver(
            forName: .conversationDraftChanged,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.reloadFromCache()
            }
        }
        
        NotificationCenter.default.addObserver(
            forName: .conversationDraftDeleted,
            object: nil,
            queue: .main
        ) { [weak self] notif in
            MainActor.assumeIsolated {
                guard let self = self else { return }
                if let draftId = notif.object as? String {
                    self.conversations.removeAll(where: { $0.id == draftId })
                }
                self.reloadFromCache()
            }
        }
        
        NotificationCenter.default.addObserver(
            forName: .conversationUpserted,
            object: nil,
            queue: .main
        ) { [weak self] notif in
            MainActor.assumeIsolated {
                guard let self = self else { return }
                if let item = notif.object as? ConversationItem {
                    self.upsertConversation(item)
                }
            }
        }
    }
    
    private func purgeExpiredTombstones() {
        let now = Date()
        recentlyDeletedIDs = recentlyDeletedIDs.filter { now.timeIntervalSince($0.value) < tombstoneTTL }
    }
    
    /// 列表里实际可见的会话（不含子代理与已删除/删除中的），不受搜索框与项目筛选影响。
    private var visibleConversations: [ConversationItem] {
        conversations.filter { item in
            !item.isSubagent &&
            !pendingDeleteCascadeIDs.contains(item.id) &&
            (recentlyDeletedIDs[item.id] == nil) &&
            !cacheManager.isDeletedConversation(cascadeId: item.id)
        }
    }

    public var totalVisibleCount: Int {
        visibleConversations.count
    }

    /// 按当前选中的工作区/Chat 过滤后的会话列表。
    public var workspaceFilteredConversations: [ConversationItem] {
        guard let filter = selectedWorkspaceFilter else { return visibleConversations }
        return visibleConversations.filter { filter.matches($0) }
    }

    public var workspaceTotalCount: Int {
        workspaceFilteredConversations.count
    }

    /// 供筛选条下拉菜单使用的工作区列表（含 Chat 与各项目，按最近活跃倒序排列）。
    public var availableWorkspaceFilters: [WorkspaceFilterOption] {
        var chatCount = 0
        var chatLatest: Date = .distantPast
        var workspaces: [String: (count: Int, latest: Date)] = [:]

        for item in visibleConversations {
            let modified = item.effectiveLastModified
            if item.isPureChat {
                chatCount += 1
                if modified > chatLatest { chatLatest = modified }
            } else {
                let name = item.workspaceName
                if var entry = workspaces[name] {
                    entry.count += 1
                    if modified > entry.latest { entry.latest = modified }
                    workspaces[name] = entry
                } else {
                    workspaces[name] = (1, modified)
                }
            }
        }

        var result: [WorkspaceFilterOption] = []
        if chatCount > 0 {
            result.append(WorkspaceFilterOption(filter: .chat, count: chatCount, latestModified: chatLatest))
        }
        let sortedProjects = workspaces
            .map { WorkspaceFilterOption(filter: .workspace($0.key), count: $0.value.count, latestModified: $0.value.latest) }
            .sorted { a, b in
                if a.latestModified != b.latestModified {
                    return a.latestModified > b.latestModified
                }
                return a.filter.displayName.localizedCaseInsensitiveCompare(b.filter.displayName) == .orderedAscending
            }
        result.append(contentsOf: sortedProjects)
        return result
    }

    // MARK: - 会话内容搜索（language_server SearchConversations）

    /// 内容命中，与会话条目配对，点击后直接进入该会话。
    public struct ContentSearchHit: Identifiable {
        public var id: String { item.id }
        public let result: ConversationSearchResult
        public let item: ConversationItem
    }

    public var contentHits: [ContentSearchHit] = []
    public var isSearchingContent: Bool = false
    private var contentSearchTask: Task<Void, Never>? = nil

    /// 输入停顿 350ms 后再查；少于 2 个字符不查（单字命中太泛）。由视图在 searchQuery 变化时调用。
    public func scheduleContentSearch() {
        contentSearchTask?.cancel()
        let query = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        // 当前 Antigravity 没有 SearchConversations 时只保留按标题过滤，不再发必然失败的请求
        guard GatewayCompatStore.shared.isAvailable(GatewayFeature.search),
              query.unicodeScalars.count >= 2, let baseURL = settings.serverURL else {
            contentHits = []
            isSearchingContent = false
            return
        }
        isSearchingContent = true
        contentSearchTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 350_000_000)
            guard !Task.isCancelled, let self else { return }
            do {
                let results = try await self.apiClient.searchConversations(query: query, baseURL: baseURL)
                guard !Task.isCancelled else { return }
                let byID = Dictionary(self.workspaceFilteredConversations.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
                self.contentHits = results.compactMap { r in
                    byID[r.cascadeId].map { ContentSearchHit(result: r, item: $0) }
                }
            } catch {
                guard !Task.isCancelled else { return }
                self.contentHits = []
            }
            self.isSearchingContent = false
        }
    }

    public static func sortItems(_ items: [ConversationItem]) -> [ConversationItem] {
        items.sorted { a, b in
            if a.isPinned != b.isPinned {
                return a.isPinned && !b.isPinned
            }
            return a.effectiveLastModified > b.effectiveLastModified
        }
    }

    public var filteredConversations: [ConversationItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let scoped = workspaceFilteredConversations
        if q.isEmpty { return scoped }
        return scoped.filter {
            $0.title.lowercased().contains(q) ||
            $0.workspaceName.lowercased().contains(q) ||
            $0.id.lowercased().contains(q)
        }
    }
    
    public func reloadFromCache() {
        let drafts = cacheManager.loadLocalDraftConversations()
        let cached = cacheManager.loadConversations().filter { !$0.isSubagent && !$0.isDraft }
        let draftIds = Set(drafts.map(\.id))
        let nonDraftCached = cached.filter { !draftIds.contains($0.id) }
        if !nonDraftCached.isEmpty || !drafts.isEmpty {
            let pinnedIds = cacheManager.getPinnedConversationIDs()
            let all = (drafts + nonDraftCached).map { $0.withPinned(pinnedIds.contains($0.id)) }
            self.conversations = Self.sortItems(all)
            self.markReorderedNow()
        }
    }
    
    @MainActor
    public func resumeActive() async {
        let now = Date()
        guard now.timeIntervalSince(lastResumeTime) > 1.0 else { return }
        lastResumeTime = now
        
        // Don't trust an old green dot after the app was suspended: reconnect when the heartbeat went stale.
        eventsClient.reconnectIfStale()
        reloadFromCache()
        await fetchConversations(isBackgroundPoll: !conversations.isEmpty)
    }
    
    public func startAutoRefresh() {
        guard settings.isPaired else { return }
        guard pollTask == nil else { return }
        eventsClient.onChange = { [weak self] in
            guard let self else { return }
            Task { await self.fetchConversations(isBackgroundPoll: true) }
        }
        eventsClient.onConnectionStateChange = { [weak self] _ in
            guard let self, self.pollTask != nil else { return }
            self.restartPollLoop()
        }
        eventsClient.start(baseURL: { [weak self] in self?.settings.serverURL })
        restartPollLoop()
    }

    private func restartPollLoop() {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { break }
                let connectedAtStart = self.eventsClient.isConnected
                let hasRunning = self.conversations.contains(where: { $0.status.isRunning || $0.status.needsAction })
                let waitSeconds = connectedAtStart ? Self.eventsSafetyPollSeconds : (hasRunning ? 4 : 10)
                try? await Task.sleep(nanoseconds: UInt64(waitSeconds) * 1_000_000_000)
                guard !Task.isCancelled else { break }
                guard self.settings.isPaired else { break }
                await self.fetchConversations(isBackgroundPoll: true)
            }
        }
    }
    
    public func stopAutoRefresh() {
        pollTask?.cancel()
        pollTask = nil
        pendingReorderTask?.cancel()
        pendingReorderTask = nil
        eventsClient.onConnectionStateChange = nil
        eventsClient.stop()
        eventsClient.onChange = nil
    }
    
    @MainActor
    public func fetchConversations(isBackgroundPoll: Bool = false) async {
        if conversations.isEmpty {
            let drafts = cacheManager.loadLocalDraftConversations()
            let cached = cacheManager.loadConversations().filter { !$0.isSubagent && !$0.isDraft }
            let draftIds = Set(drafts.map(\.id))
            let nonDraftCached = cached.filter { !draftIds.contains($0.id) }
            if !nonDraftCached.isEmpty || !drafts.isEmpty {
                self.conversations = drafts + nonDraftCached
                self.cacheManager.prewarmSessions(for: nonDraftCached.prefix(15).map(\.id))
            }
        }
        
        guard settings.isPaired else {
            return
        }
        
        guard let url = settings.serverURL else {
            if conversations.isEmpty {
                self.errorMessage = String(localized: "请在设置中配置有效的服务器地址")
            }
            return
        }
        
        if !isBackgroundPoll && conversations.isEmpty {
            self.isLoading = true
        }
        if !isBackgroundPoll {
            self.errorMessage = nil
        }
        
        // 升级自检结果随列表刷新（轮询很频繁，自检结果只在 Antigravity 重启/升级时才变，30 秒内不重复请求）
        if Date().timeIntervalSince(lastCompatRefresh) > 30 {
            lastCompatRefresh = Date()
            Task { await apiClient.refreshGatewayCompat(baseURL: url) }
        }
        
        do {
            let items = try await apiClient.fetchConversations(baseURL: url)
            purgeExpiredTombstones()
            let cleaned = items.filter { item in
                !item.isSubagent &&
                !item.isDraft &&
                !self.pendingDeleteCascadeIDs.contains(item.id) &&
                (self.recentlyDeletedIDs[item.id] == nil) &&
                !self.cacheManager.isDeletedConversation(cascadeId: item.id)
            }
            let existingMap = Dictionary(self.conversations.map { ($0.id, $0.title) }, uniquingKeysWith: { _, new in new })
            let enriched = cleaned.map { item -> ConversationItem in
                let currentT = item.title.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
                if currentT.isEmpty || currentT == "未命名会话" {
                    if let knownTitle = existingMap[item.id], !knownTitle.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty, knownTitle != "未命名会话" {
                        return item.withTitle(knownTitle)
                    }
                    if let cachedTitle = self.cacheManager.cachedSessionTitle(for: item.id), !cachedTitle.isEmpty {
                        return item.withTitle(cachedTitle)
                    }
                }
                return item
            }
            let drafts = cacheManager.loadLocalDraftConversations()
            let draftIds = Set(drafts.map(\.id))
            let nonDraftEnriched = enriched.filter { !draftIds.contains($0.id) && !$0.isDraft }
            let sorted = Self.sortItems(drafts + nonDraftEnriched)
            let all = applyReorderBuffer(sorted: sorted, forceReorder: !isBackgroundPoll)
            updateConversationsWithAnimation(all)
            cacheManager.saveConversations(nonDraftEnriched)
            cacheManager.prewarmSessions(for: all.prefix(15).map(\.id))
            ActivityManager.shared.syncWithConversations(nonDraftEnriched)
            QuotaWidgetStore.shared.publishConversationsFromApp(items: nonDraftEnriched, isDemo: settings.isDemoMode)
            self.isLoading = false
        } catch {
            // Auto-failover: if current endpoint failed, probe candidate endpoints and retry once with alternative
            if let elected = await ConnectionManager.shared.probeEndpoints(),
               let retryURL = URL(string: elected), retryURL != url {
                do {
                    let items = try await apiClient.fetchConversations(baseURL: retryURL)
                    purgeExpiredTombstones()
                    let cleaned = items.filter { item in
                        !item.isSubagent &&
                        !item.isDraft &&
                        !self.pendingDeleteCascadeIDs.contains(item.id) &&
                        (self.recentlyDeletedIDs[item.id] == nil) &&
                        !self.cacheManager.isDeletedConversation(cascadeId: item.id)
                    }
                    let existingMap = Dictionary(self.conversations.map { ($0.id, $0.title) }, uniquingKeysWith: { _, new in new })
                    let enriched = cleaned.map { item -> ConversationItem in
                        let currentT = item.title.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
                        if currentT.isEmpty || currentT == "未命名会话" {
                            if let knownTitle = existingMap[item.id], !knownTitle.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty, knownTitle != "未命名会话" {
                                return item.withTitle(knownTitle)
                            }
                            if let cachedTitle = self.cacheManager.cachedSessionTitle(for: item.id), !cachedTitle.isEmpty {
                                return item.withTitle(cachedTitle)
                            }
                        }
                        return item
                    }
                    let drafts = cacheManager.loadLocalDraftConversations()
                    let draftIds = Set(drafts.map(\.id))
                    let nonDraftEnriched = enriched.filter { !draftIds.contains($0.id) && !$0.isDraft }
                    let pinnedIds = cacheManager.getPinnedConversationIDs()
                    let all = (drafts + nonDraftEnriched).map { $0.withPinned(pinnedIds.contains($0.id)) }
                    let buffered = applyReorderBuffer(sorted: Self.sortItems(all), forceReorder: !isBackgroundPoll)
                    updateConversationsWithAnimation(buffered)
                    cacheManager.saveConversations(nonDraftEnriched)
                    cacheManager.prewarmSessions(for: all.prefix(15).map(\.id))
                    ActivityManager.shared.syncWithConversations(nonDraftEnriched)
                    QuotaWidgetStore.shared.publishConversationsFromApp(items: nonDraftEnriched, isDemo: settings.isDemoMode)
                    self.isLoading = false
                    self.errorMessage = nil
                    await fetchQuotas()
                    await ModelCatalogStore.shared.refreshIfNeeded(settings: settings, baseURL: retryURL)
                    return
                } catch {
                    // Fallthrough to standard error handling
                }
            }
            if conversations.isEmpty {
                self.errorMessage = error.localizedDescription
            }
            self.isLoading = false
        }
        
        await fetchQuotas()
        await fetchGlobalBackgroundTasks()
        if let url = settings.serverURL {
            await ModelCatalogStore.shared.refreshIfNeeded(settings: settings, baseURL: url)
        }
    }

    @MainActor
    public func updateGlobalBackgroundTasks(_ tasks: [GlobalBackgroundTaskItem]) {
        let wasEmpty = self.globalBackgroundTasks.isEmpty
        let isNowEmpty = tasks.isEmpty
        if wasEmpty != isNowEmpty {
            withAnimation(.spring(response: 0.42, dampingFraction: 0.72)) {
                self.globalBackgroundTasks = tasks
            }
        } else {
            self.globalBackgroundTasks = tasks
        }
    }

    @MainActor
    public func fetchGlobalBackgroundTasks() async {
        guard settings.isPaired, let url = settings.serverURL else { return }
        if let tasks = try? await apiClient.fetchGlobalBackgroundTasks(baseURL: url) {
            updateGlobalBackgroundTasks(tasks)
        }
    }
    
    private var lastQuotaFetchTime: Date = .distantPast
    private let quotaRefreshInterval: TimeInterval = 300 // 5 minutes
    
    @MainActor
    public func fetchQuotas(force: Bool = false) async {
        guard settings.isPaired else { return }
        let now = Date()
        if !force && now.timeIntervalSince(lastQuotaFetchTime) < quotaRefreshInterval && quotaResponse != nil {
            return
        }
        guard let url = settings.serverURL else { return }
        do {
            self.quotaResponse = try await apiClient.fetchCockpitQuotas(baseURL: url)
            self.lastQuotaFetchTime = now
            publishQuotaToWidget()
        } catch {
            // Silently ignore quota fetch error to not disturb chat list
        }
    }

    @MainActor
    public func publishQuotaToWidget() {
        settings.syncWidgetConfiguration(reloadWidget: false)
        let emailMasked = UserDefaults.standard.bool(forKey: "cockpit_email_masked")
        QuotaWidgetStore.shared.publishFromApp(
            response: quotaResponse,
            preferredBucket: settings.selectedQuotaBucket,
            paceCursorEnabled: settings.weeklyPaceCursorEnabled,
            isEmailMasked: emailMasked,
            isDemo: settings.isDemoMode
        )
    }

    @MainActor
    public func fetchTokenHistory(force: Bool = false) async {
        guard settings.isPaired else { return }
        if !force && tokenHistory != nil && isLoadingTokenHistory { return }
        guard let url = settings.serverURL else { return }
        isLoadingTokenHistory = true
        defer { isLoadingTokenHistory = false }
        do {
            self.tokenHistory = try await apiClient.fetchTokenHistory(baseURL: url, force: force)
        } catch {
            // Silently ignore token history error
        }
    }
    
    @MainActor
    public func switchCockpitAccount(id: String) async throws {
        guard let url = settings.serverURL else { return }
        try await apiClient.switchCockpitAccount(baseURL: url, accountId: id)
        await fetchQuotas(force: true)
        await ModelCatalogStore.shared.refresh(settings: settings, baseURL: url, force: true)
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
            publishQuotaToWidget()
            return
        }
        
        // 2. Poll for updated data if background batch refresh across accounts takes longer (e.g. 5 accounts take ~70-90s)
        let startTime = Date()
        while Date().timeIntervalSince(startTime) < 90 {
            try await Task.sleep(nanoseconds: 1_500_000_000)
            if let res = try? await apiClient.fetchCockpitQuotas(baseURL: url) {
                if res.updatedAt > initialUpdatedAt {
                    self.quotaResponse = res
                    self.lastQuotaFetchTime = Date()
                    publishQuotaToWidget()
                    return
                }
            }
        }
        
        // Final fallback fetch
        if let res = try? await apiClient.fetchCockpitQuotas(baseURL: url) {
            self.quotaResponse = res
            self.lastQuotaFetchTime = Date()
            publishQuotaToWidget()
        }
    }
    
    private func markReorderedNow() {
        lastReorderUptime = ProcessInfo.processInfo.systemUptime
        pendingReorderTask?.cancel()
        pendingReorderTask = nil
    }

    private func applyReorderBuffer(sorted: [ConversationItem], forceReorder: Bool) -> [ConversationItem] {
        let now = ProcessInfo.processInfo.systemUptime
        if forceReorder || conversations.isEmpty || lastReorderUptime == 0 || (now - lastReorderUptime) >= Self.reorderCooldownSeconds {
            markReorderedNow()
            return sorted
        }

        let newById = Dictionary(uniqueKeysWithValues: sorted.map { ($0.id, $0) })
        let prevIDs = conversations.map(\.id)
        let prevIDSet = Set(prevIDs)
        let preserved = prevIDs.compactMap { newById[$0] }
        let added = sorted.filter { !prevIDSet.contains($0.id) }

        let buffered = (added.filter { $0.isPinned } + preserved.filter { $0.isPinned }) +
            (added.filter { !$0.isPinned } + preserved.filter { !$0.isPinned })

        let orderChanged = buffered.map(\.id) != sorted.map(\.id)
        if orderChanged {
            if pendingReorderTask == nil {
                let remaining = max(0.05, Self.reorderCooldownSeconds - (now - lastReorderUptime))
                pendingReorderTask = Task { [weak self] in
                    try? await Task.sleep(nanoseconds: UInt64(remaining * 1_000_000_000))
                    guard !Task.isCancelled else { return }
                    self?.flushPendingReorder()
                }
            }
        } else {
            pendingReorderTask?.cancel()
            pendingReorderTask = nil
        }
        return buffered
    }

    private func flushPendingReorder() {
        pendingReorderTask = nil
        guard !conversations.isEmpty else { return }
        let sorted = Self.sortItems(conversations)
        lastReorderUptime = ProcessInfo.processInfo.systemUptime
        if sorted.map(\.id) != conversations.map(\.id) {
            updateConversationsWithAnimation(sorted)
            cacheManager.saveConversations(sorted.filter { !$0.isDraft && !$0.isSubagent })
        }
    }

    private func updateConversationsWithAnimation(_ newItems: [ConversationItem]) {
        let oldIDs = self.conversations.map(\.id)
        let newIDs = newItems.map(\.id)
        if oldIDs != newIDs && !oldIDs.isEmpty {
            withAnimation(.spring(response: 0.42, dampingFraction: 0.72)) {
                self.conversations = newItems
            }
        } else {
            self.conversations = newItems
        }
    }

    @MainActor
    public func upsertConversation(_ item: ConversationItem) {
        guard !item.isSubagent && !item.isDraft && !pendingDeleteCascadeIDs.contains(item.id) && recentlyDeletedIDs[item.id] == nil else { return }
        let resolved = item.withPinned(cacheManager.isPinned(cascadeId: item.id))
        var updated = conversations
        if let idx = updated.firstIndex(where: { $0.id == resolved.id }) {
            updated[idx] = resolved
        } else {
            updated.append(resolved)
        }
        let sorted = Self.sortItems(updated)
        markReorderedNow()
        updateConversationsWithAnimation(sorted)
    }
    
    @MainActor
    public func togglePin(item: ConversationItem) {
        let newPinned = cacheManager.togglePin(cascadeId: item.id)
        if let idx = conversations.firstIndex(where: { $0.id == item.id }) {
            var updated = conversations
            updated[idx] = updated[idx].withPinned(newPinned)
            let sorted = Self.sortItems(updated)
            markReorderedNow()
            withAnimation(.spring(response: 0.42, dampingFraction: 0.72)) {
                self.conversations = sorted
            }
            cacheManager.saveConversations(sorted.filter { !$0.isDraft && !$0.isSubagent })
        }
        
        guard let url = settings.serverURL, settings.isPaired else { return }
        Task {
            try? await apiClient.setConversationPinned(cascadeId: item.id, pinned: newPinned, baseURL: url)
        }
    }
    
    @MainActor
    public func deleteConversation(item: ConversationItem) async {
        if item.isDraft {
            cacheManager.deleteLocalDraftSession(id: item.id)
            cacheManager.clearDraft(key: item.id)
            self.conversations.removeAll(where: { $0.id == item.id })
            NotificationCenter.default.post(name: .conversationDraftDeleted, object: item.id)
            return
        }
        
        guard let url = settings.serverURL else {
            self.errorMessage = String(localized: "请在设置中配置有效的服务器地址")
            return
        }
        
        // Immediately record tombstone to prevent polling/concurrent responses from reviving it
        pendingDeleteCascadeIDs.insert(item.id)
        recentlyDeletedIDs[item.id] = Date()
        
        let originalConversations = self.conversations
        // Optimistic UI removal
        self.conversations.removeAll(where: { $0.id == item.id })
        self.cacheManager.deleteConversation(cascadeId: item.id)
        if ActivityManager.shared.hasActiveActivity(for: item.id) {
            ActivityManager.shared.endActivity(for: item.id, finalStatus: "CANCELLED")
        }
        
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
            self.errorMessage = String(localized: "删除会话失败: \(error.localizedDescription)")
        }
    }
    
    @MainActor
    public func renameConversation(item: ConversationItem, newTitle: String) async {
        guard !item.isDraft else { return }
        let trimmed = newTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        guard trimmed != item.title else { return }
        
        guard let url = settings.serverURL else {
            self.errorMessage = String(localized: "请在设置中配置有效的服务器地址")
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
        ActivityManager.shared.updateTitle(cascadeId: item.id, newTitle: trimmed)
        
        do {
            try await apiClient.renameConversation(cascadeId: item.id, newTitle: trimmed, baseURL: url)
        } catch {
            // Revert on failure
            ActivityManager.shared.updateTitle(cascadeId: item.id, newTitle: originalTitle)
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
            self.errorMessage = String(localized: "重命名会话失败: \(error.localizedDescription)")
        }
    }
}
