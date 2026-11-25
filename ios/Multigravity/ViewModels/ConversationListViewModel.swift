import Foundation
import Observation
import SwiftUI

@Observable
@MainActor
public final class ConversationListViewModel {
    public enum WorkspaceFilter: Equatable, Hashable, Sendable {
        case chat
        case workspace(String)
        case archived

        public var displayName: String {
            switch self {
            case .chat:
                return String(localized: "Chat")
            case .workspace(let name):
                return name
            case .archived:
                return String(localized: "已归档")
            }
        }

        public var isPureChat: Bool {
            if case .chat = self { return true }
            return false
        }

        public var isArchived: Bool {
            if case .archived = self { return true }
            return false
        }

        public func matches(_ item: ConversationItem) -> Bool {
            switch self {
            case .chat:
                return !item.isArchived && item.isPureChat
            case .workspace(let name):
                return !item.isArchived && !item.isPureChat && item.workspaceName == name
            case .archived:
                return item.isArchived
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
            case .archived:
                return "__archived__"
            }
        }
    }

    public var conversations: [ConversationItem] = []
    public var searchQuery: String = "" {
        didSet {
            if searchQuery != oldValue {
                searchCollapsedBranchParentIDs.removeAll()
            }
        }
    }
    /// 已手动展开旁路快问折叠组的父会话 ID 集合（默认全部折叠）。
    public var expandedBranchParentIDs: Set<String> = []
    /// 搜索状态下用户手动收起的父会话 ID 集合（搜索词变化时自动清空）。
    public var searchCollapsedBranchParentIDs: Set<String> = []
    /// 子旁路存在待审批（Action）自动展开后，用户显式手动折叠的父会话 ID 集合。
    public var manuallyCollapsedActionBranchParentIDs: Set<String> = []
    public var selectedWorkspaceFilter: WorkspaceFilter? = nil {
        didSet {
            if selectedWorkspaceFilter != .archived {
                cancelArchiveAutoExit()
            }
        }
    }
    /// True during the brief window after unarchiving the last archived item while its row
    /// finishes sliding out before exiting the `.archived` filter, preventing the empty-state flash.
    public var isAutoExitingArchive: Bool = false
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
    // Optimistic archive overrides to prevent in-flight background polls from flipping archive state back mid-animation
    private var pendingArchiveStates: [String: (archived: Bool, token: UUID)] = [:]
    private var recentlyArchivedStates: [String: (archived: Bool, settledAt: Date)] = [:]
    private let archiveSettlementWindow: TimeInterval = 5.0
    private var archiveAutoExitTask: Task<Void, Never>? = nil
    
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
    private var lastNotifiedRouteURL: String? = nil

    private func normalizedCurrentRouteURL() -> String? {
        settings.serverURL?.absoluteString
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased()
    }
    
    public init(
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.cacheManager = cacheManager ?? .shared
        self.lastNotifiedRouteURL = self.settings.serverURL?.absoluteString
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased()
        
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
                let newRoute = self.normalizedCurrentRouteURL()
                guard newRoute != self.lastNotifiedRouteURL else { return }
                self.lastNotifiedRouteURL = newRoute
                self.eventsClient.reconnect(onlyIfStarted: true)
                await self.fetchConversations(isBackgroundPoll: !self.conversations.isEmpty)
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
        recentlyArchivedStates = recentlyArchivedStates.filter { now.timeIntervalSince($0.value.settledAt) < archiveSettlementWindow }
    }

    private func cancelArchiveAutoExit() {
        archiveAutoExitTask?.cancel()
        archiveAutoExitTask = nil
        if isAutoExitingArchive {
            isAutoExitingArchive = false
        }
    }

    private func applyArchiveOverrides(_ items: [ConversationItem], fetchedAt: Date) -> [ConversationItem] {
        guard !pendingArchiveStates.isEmpty || !recentlyArchivedStates.isEmpty else { return items }
        return items.map { item in
            if let pending = pendingArchiveStates[item.id] {
                return item.isArchived == pending.archived ? item : item.withArchived(pending.archived)
            }
            if let recent = recentlyArchivedStates[item.id], fetchedAt < recent.settledAt {
                return item.isArchived == recent.archived ? item : item.withArchived(recent.archived)
            }
            return item
        }
    }
    
    /// 列表里实际可见的活跃会话（不含子代理、已归档与已删除/删除中的），不受搜索框与项目筛选影响。
    private var visibleConversations: [ConversationItem] {
        conversations.filter { item in
            !item.isSubagent &&
            !item.isArchived &&
            !pendingDeleteCascadeIDs.contains(item.id) &&
            (recentlyDeletedIDs[item.id] == nil) &&
            !cacheManager.isDeletedConversation(cascadeId: item.id)
        }
    }

    /// 已归档的会话列表（独立收纳，避免无限历史模式下淹没主列表）。
    public var archivedConversations: [ConversationItem] {
        conversations.filter { item in
            !item.isSubagent &&
            item.isArchived &&
            !pendingDeleteCascadeIDs.contains(item.id) &&
            (recentlyDeletedIDs[item.id] == nil) &&
            !cacheManager.isDeletedConversation(cascadeId: item.id)
        }
    }

    public var archivedCount: Int {
        archivedConversations.count
    }

    public var totalVisibleCount: Int {
        visibleConversations.count
    }

    /// 按当前选中的工作区/Chat/已归档过滤后的会话列表。
    public var workspaceFilteredConversations: [ConversationItem] {
        guard let filter = selectedWorkspaceFilter else { return visibleConversations }
        if filter.isArchived { return archivedConversations }
        return visibleConversations.filter { filter.matches($0) }
    }

    public var workspaceTotalCount: Int {
        workspaceFilteredConversations.count
    }

    /// 当开启工作区筛选时，其他工作区中被隐藏的待审批/等输入会话数量（方案 B 盲区兜底）。
    public var hiddenPendingApprovalCount: Int {
        guard let filter = selectedWorkspaceFilter else { return 0 }
        return visibleConversations.reduce(into: 0) { count, item in
            if item.status.needsAction && !filter.matches(item) {
                count += 1
            }
        }
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
        let archived = archivedConversations
        if !archived.isEmpty {
            let latestArchived = archived.map(\.effectiveLastModified).max() ?? .distantPast
            result.append(WorkspaceFilterOption(filter: .archived, count: archived.count, latestModified: latestArchived))
        }
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
                let allByID = Dictionary(self.conversations.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
                self.contentHits = results.compactMap { r in
                    if let existing = byID[r.cascadeId] {
                        if r.isSideQuestion == true && (!existing.isSideQuestion || existing.branchParentId == nil) {
                            let parentId = r.parentConversationId?.trimmingCharacters(in: .whitespacesAndNewlines)
                            let parent = parentId.flatMap { allByID[$0] }
                            let enriched = ConversationItem(
                                id: existing.id,
                                title: existing.title,
                                status: existing.status,
                                stepCount: existing.stepCount,
                                workspaceName: existing.workspaceName,
                                lastModified: existing.lastModified,
                                isSubagent: existing.isSubagent,
                                isUnread: existing.isUnread,
                                draftProject: existing.draftProject,
                                parentConversationId: existing.parentConversationId,
                                nestingDepth: existing.nestingDepth,
                                isPinned: existing.isPinned,
                                hasRunningTasks: existing.hasRunningTasks,
                                hasRunningSubagents: existing.hasRunningSubagents,
                                hasSwarm: existing.hasSwarm,
                                swarmWorkingCount: existing.swarmWorkingCount,
                                swarmTotalMembers: existing.swarmTotalMembers,
                                currentBranch: existing.currentBranch,
                                pullRequest: existing.pullRequest,
                                isSideQuestion: true,
                                branchParentId: parentId?.isEmpty == false ? parentId : existing.branchParentId,
                                branchParentTitle: parent?.title ?? existing.branchParentTitle,
                                forkStepIndex: existing.forkStepIndex
                            )
                            return ContentSearchHit(result: r, item: enriched)
                        }
                        return ContentSearchHit(result: r, item: existing)
                    }
                    // Antigravity 2.35 parity: preserve /btw side-question hits even when not directly listed.
                    if r.isSideQuestion == true {
                        let parentId = r.parentConversationId?.trimmingCharacters(in: .whitespacesAndNewlines)
                        let parent = parentId.flatMap { allByID[$0] }
                        if let filter = self.selectedWorkspaceFilter {
                            if let parent, !filter.matches(parent) { return nil }
                        }
                        let synth = ConversationItem(
                            id: r.cascadeId,
                            title: r.title.isEmpty ? String(localized: "未命名会话") : r.title,
                            status: .idle,
                            stepCount: 0,
                            workspaceName: parent?.workspaceName ?? r.workspaceName ?? "Chat",
                            lastModified: nil,
                            isSideQuestion: true,
                            branchParentId: parentId?.isEmpty == false ? parentId : nil,
                            branchParentTitle: parent?.title
                        )
                        return ContentSearchHit(result: r, item: synth)
                    }
                    return nil
                }
            } catch {
                guard !Task.isCancelled else { return }
                self.contentHits = []
            }
            self.isSearchingContent = false
        }
    }

    public static func sortItems(_ items: [ConversationItem]) -> [ConversationItem] {
        let flat = items.sorted { a, b in
            if a.status.needsAction != b.status.needsAction {
                return a.status.needsAction && !b.status.needsAction
            }
            if a.isPinned != b.isPinned {
                return a.isPinned && !b.isPinned
            }
            if a.status.isRunning != b.status.isRunning {
                return a.status.isRunning && !b.status.isRunning
            }
            return a.effectiveLastModified > b.effectiveLastModified
        }
        return arrangeBranchTree(flat)
    }

    /// 将旁路快问分支稳定挂载到其父会话正下方（保持父子视觉层级与树状缩进），
    /// 若父会话不在当前筛选集合内，分支会话仍作为独立行保留（不丢失）。
    public static func arrangeBranchTree(_ items: [ConversationItem]) -> [ConversationItem] {
        let allIDs = Set(items.map(\.id))
        var childrenByParent: [String: [ConversationItem]] = [:]
        var roots: [ConversationItem] = []

        for item in items {
            if item.isSideQuestion,
               let parentId = item.branchParentId?.trimmingCharacters(in: .whitespacesAndNewlines),
               !parentId.isEmpty,
               parentId != item.id,
               allIDs.contains(parentId) {
                childrenByParent[parentId, default: []].append(item)
            } else {
                roots.append(item)
            }
        }
        guard !childrenByParent.isEmpty else { return items }

        var result: [ConversationItem] = []
        result.reserveCapacity(items.count)
        var emitted = Set<String>()

        func appendWithChildren(_ node: ConversationItem) {
            guard emitted.insert(node.id).inserted else { return }
            result.append(node)
            if let children = childrenByParent[node.id] {
                for child in children {
                    appendWithChildren(child)
                }
            }
        }

        for root in roots {
            appendWithChildren(root)
        }
        // 防御性兜底：极端环引用情况下不丢失任何会话
        if result.count < items.count {
            for item in items where !emitted.contains(item.id) {
                result.append(item)
            }
        }
        return result
    }

    public struct BranchGroup: Sendable {
        public let root: ConversationItem
        public let branches: [ConversationItem]
    }

    /// 将有序会话列表归组为顶层节点及其拍平后的附属旁路列表（支持多层旁路拍平，避免套娃折叠）。
    public static func groupConversationsWithBranches(_ items: [ConversationItem]) -> [BranchGroup] {
        guard !items.isEmpty else { return [] }
        let byId = Dictionary(items.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        var branchesByRoot: [String: [ConversationItem]] = [:]
        var roots: [ConversationItem] = []
        roots.reserveCapacity(items.count)

        func resolveVisibleRootId(for item: ConversationItem) -> String? {
            guard item.isSideQuestion else { return nil }
            var currentParentId = item.branchParentId?.trimmingCharacters(in: .whitespacesAndNewlines)
            var highestVisibleAncestor: String? = nil
            var visited: Set<String> = [item.id]
            while let pid = currentParentId, !pid.isEmpty, visited.insert(pid).inserted {
                guard let parent = byId[pid] else { break }
                highestVisibleAncestor = parent.id
                if !parent.isSideQuestion {
                    break
                }
                currentParentId = parent.branchParentId?.trimmingCharacters(in: .whitespacesAndNewlines)
            }
            return highestVisibleAncestor
        }

        for item in items {
            if let rootId = resolveVisibleRootId(for: item) {
                branchesByRoot[rootId, default: []].append(item)
            } else {
                roots.append(item)
            }
        }

        var groups = roots.map { root in
            BranchGroup(root: root, branches: branchesByRoot[root.id] ?? [])
        }
        // 防御性兜底：极端环引用情况下不丢失任何会话
        var emitted = Set<String>()
        emitted.reserveCapacity(items.count)
        for group in groups {
            emitted.insert(group.root.id)
            for b in group.branches { emitted.insert(b.id) }
        }
        if emitted.count < items.count {
            for item in items where emitted.insert(item.id).inserted {
                groups.append(BranchGroup(root: item, branches: []))
            }
        }
        return groups
    }

    public func isBranchGroupExpanded(parentId: String, hasAction: Bool) -> Bool {
        let isSearching = !searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        if isSearching {
            return !searchCollapsedBranchParentIDs.contains(parentId)
        }
        if hasAction {
            return !manuallyCollapsedActionBranchParentIDs.contains(parentId)
        }
        return expandedBranchParentIDs.contains(parentId)
    }

    public func toggleBranchGroup(parentId: String, hasAction: Bool) {
        let isSearching = !searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        if isSearching {
            if searchCollapsedBranchParentIDs.contains(parentId) {
                searchCollapsedBranchParentIDs.remove(parentId)
            } else {
                searchCollapsedBranchParentIDs.insert(parentId)
            }
            return
        }
        let currentlyExpanded = isBranchGroupExpanded(parentId: parentId, hasAction: hasAction)
        if currentlyExpanded {
            expandedBranchParentIDs.remove(parentId)
            if hasAction {
                manuallyCollapsedActionBranchParentIDs.insert(parentId)
            }
        } else {
            manuallyCollapsedActionBranchParentIDs.remove(parentId)
            expandedBranchParentIDs.insert(parentId)
        }
    }

    public func resetBranchExpansionState() {
        expandedBranchParentIDs.removeAll()
        searchCollapsedBranchParentIDs.removeAll()
        manuallyCollapsedActionBranchParentIDs.removeAll()
    }

    public var filteredConversations: [ConversationItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let scoped = workspaceFilteredConversations
        if q.isEmpty { return Self.arrangeBranchTree(scoped) }
        let matched = scoped.filter {
            $0.title.lowercased().contains(q) ||
            $0.workspaceName.lowercased().contains(q) ||
            ($0.branchParentTitle?.lowercased().contains(q) ?? false) ||
            $0.id.lowercased().contains(q)
        }
        return Self.arrangeBranchTree(matched)
    }
    
    public func reloadFromCache() {
        let drafts = cacheManager.loadLocalDraftConversations()
        let cached = cacheManager.loadConversations().filter { !$0.isSubagent && !$0.isDraft }
        let draftIds = Set(drafts.map(\.id))
        let nonDraftCached = cached.filter { !draftIds.contains($0.id) }
        if !nonDraftCached.isEmpty || !drafts.isEmpty {
            let pinnedIds = cacheManager.getPinnedConversationIDs()
            let all = applyArchiveOverrides(
                (drafts + nonDraftCached).map { $0.withPinned(pinnedIds.contains($0.id)) },
                fetchedAt: .distantFuture
            )
            self.conversations = Self.sortItems(all)
            self.markReorderedNow()
        }
    }
    
    @MainActor
    public func resumeActive() async {
        let now = Date()
        guard now.timeIntervalSince(lastResumeTime) > 1.0 else { return }
        lastResumeTime = now

        ConnectionManager.shared.ensureStarted()
        ConnectionManager.shared.noteForegroundResume()
        reloadFromCache()

        if settings.isPaired && !settings.isDemoMode {
            let switchedToLan = await ConnectionManager.shared.probeLanFastIfNeeded(timeout: 0.65)
            if switchedToLan {
                lastNotifiedRouteURL = normalizedCurrentRouteURL()
                eventsClient.reconnect(onlyIfStarted: true)
            } else {
                // Don't trust an old green dot after the app was suspended: reconnect when the heartbeat went stale.
                eventsClient.reconnectIfStale()
                let probeStale = ConnectionManager.shared.lastProbeDate.map { Date().timeIntervalSince($0) > 15 } ?? true
                if probeStale || !eventsClient.isConnected || quotaResponse == nil {
                    Task {
                        await ConnectionManager.shared.probeEndpoints()
                    }
                }
            }
        } else {
            eventsClient.reconnectIfStale()
        }

        await fetchConversations(isBackgroundPoll: !conversations.isEmpty)
    }

    @MainActor
    public func pullToRefresh() async {
        guard settings.isPaired else { return }
        ConnectionManager.shared.ensureStarted()
        if !settings.isDemoMode {
            let isLinkUnhealthy = !eventsClient.isConnected || eventsClient.linkState() != .connected
            let switchedToLan = await ConnectionManager.shared.probeLanFastIfNeeded(
                force: isLinkUnhealthy,
                timeout: 0.65
            )
            if switchedToLan {
                lastNotifiedRouteURL = normalizedCurrentRouteURL()
                eventsClient.reconnect(onlyIfStarted: true)
            } else if isLinkUnhealthy {
                let before = normalizedCurrentRouteURL()
                _ = await ConnectionManager.shared.probeEndpoints()
                let after = normalizedCurrentRouteURL()
                if after != before {
                    lastNotifiedRouteURL = after
                    eventsClient.reconnect(onlyIfStarted: true)
                } else {
                    eventsClient.reconnectIfStale()
                }
            } else {
                eventsClient.reconnectIfStale()
            }
        }
        await fetchConversations(isBackgroundPoll: false)
    }
    
    public func startAutoRefresh() {
        guard settings.isPaired else { return }
        guard pollTask == nil else { return }
        ConnectionManager.shared.ensureStarted()
        eventsClient.onChange = { [weak self] in
            guard let self else { return }
            Task { await self.fetchConversations(isBackgroundPoll: true) }
        }
        eventsClient.onConnectionStateChange = { [weak self] _ in
            guard let self, self.pollTask != nil else { return }
            self.restartPollLoop()
        }
        eventsClient.onNeedRouteProbe = { [weak self] in
            guard let self, self.settings.isPaired, !self.settings.isDemoMode else { return }
            Task {
                await ConnectionManager.shared.probeEndpoints()
            }
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
        eventsClient.onNeedRouteProbe = nil
        eventsClient.stop()
        eventsClient.onChange = nil
    }

    @MainActor
    public func resetForGatewaySwitch() {
        stopAutoRefresh()
        contentSearchTask?.cancel()
        contentSearchTask = nil
        cancelArchiveAutoExit()

        conversations = []
        searchQuery = ""
        contentHits = []
        isSearchingContent = false
        selectedWorkspaceFilter = nil
        expandedBranchParentIDs.removeAll()
        searchCollapsedBranchParentIDs.removeAll()
        manuallyCollapsedActionBranchParentIDs.removeAll()
        pendingDeleteCascadeIDs.removeAll()
        recentlyDeletedIDs.removeAll()
        pendingArchiveStates.removeAll()
        recentlyArchivedStates.removeAll()

        errorMessage = nil
        isLoading = false
        quotaResponse = nil
        tokenHistory = nil
        isLoadingTokenHistory = false
        globalBackgroundTasks = []
        lastCompatRefresh = .distantPast
        lastQuotaFetchTime = .distantPast
        lastTokenHistoryFetchTime = .distantPast
        lastTokenHistorySessionKey = nil
        lastResumeTime = .distantPast
        lastNotifiedRouteURL = normalizedCurrentRouteURL()

        reloadFromCache()
        if settings.isPaired {
            startAutoRefresh()
            Task { @MainActor [weak self] in
                guard let self else { return }
                let epoch = GatewayRuntimeState.shared.epoch
                await ConnectionManager.shared.probeEndpoints()
                guard GatewayRuntimeState.shared.epoch == epoch else { return }
                await self.fetchConversations(isBackgroundPoll: !self.conversations.isEmpty)
                guard GatewayRuntimeState.shared.epoch == epoch else { return }
                await self.fetchQuotas(force: true)
            }
        }
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
        
        let startEpoch = GatewayRuntimeState.shared.epoch
        let fetchStartedAt = Date()
        do {
            let items = try await apiClient.fetchConversations(baseURL: url)
            guard GatewayRuntimeState.shared.epoch == startEpoch else { return }
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
            let nonDraftEnriched = applyArchiveOverrides(
                enriched.filter { !draftIds.contains($0.id) && !$0.isDraft },
                fetchedAt: fetchStartedAt
            )
            let sorted = Self.sortItems(drafts + nonDraftEnriched)
            let all = applyReorderBuffer(sorted: sorted, forceReorder: !isBackgroundPoll)
            updateConversationsWithAnimation(all)
            cacheManager.saveConversations(nonDraftEnriched)
            cacheManager.prewarmSessions(for: all.prefix(15).map(\.id))
            GatewayProfileStore.shared.updateActiveSummary(from: nonDraftEnriched)
            ActivityManager.shared.syncWithConversations(nonDraftEnriched)
            QuotaWidgetStore.shared.publishConversationsFromApp(items: nonDraftEnriched, isDemo: settings.isDemoMode)
            self.isLoading = false
        } catch {
            guard GatewayRuntimeState.shared.epoch == startEpoch else { return }
            // Auto-failover: if current endpoint failed, probe candidate endpoints and retry once with alternative
            if let elected = await ConnectionManager.shared.probeEndpoints(),
               GatewayRuntimeState.shared.epoch == startEpoch,
               let retryURL = URL(string: elected), retryURL != url {
                do {
                    let retryStartedAt = Date()
                    let items = try await apiClient.fetchConversations(baseURL: retryURL)
                    guard GatewayRuntimeState.shared.epoch == startEpoch else { return }
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
                    let nonDraftEnriched = applyArchiveOverrides(
                        enriched.filter { !draftIds.contains($0.id) && !$0.isDraft },
                        fetchedAt: retryStartedAt
                    )
                    let pinnedIds = cacheManager.getPinnedConversationIDs()
                    let all = (drafts + nonDraftEnriched).map { $0.withPinned(pinnedIds.contains($0.id)) }
                    let buffered = applyReorderBuffer(sorted: Self.sortItems(all), forceReorder: !isBackgroundPoll)
                    updateConversationsWithAnimation(buffered)
                    cacheManager.saveConversations(nonDraftEnriched)
                    cacheManager.prewarmSessions(for: all.prefix(15).map(\.id))
                    GatewayProfileStore.shared.updateActiveSummary(from: nonDraftEnriched)
                    ActivityManager.shared.syncWithConversations(nonDraftEnriched)
                    QuotaWidgetStore.shared.publishConversationsFromApp(items: nonDraftEnriched, isDemo: settings.isDemoMode)
                    self.isLoading = false
                    self.errorMessage = nil
                    self.lastNotifiedRouteURL = self.normalizedCurrentRouteURL()
                    self.eventsClient.reconnect(onlyIfStarted: true)
                    self.refreshAuxiliaryHomeData(baseURL: retryURL)
                    return
                } catch {
                    // Fallthrough to standard error handling
                }
            }
            guard GatewayRuntimeState.shared.epoch == startEpoch else { return }
            if conversations.isEmpty {
                self.errorMessage = error.localizedDescription
            }
            self.isLoading = false
            return
        }
        
        refreshAuxiliaryHomeData(baseURL: settings.serverURL ?? url)
    }

    /// Concurrently refreshes secondary home bar data (quota, background tasks, model catalog) without
    /// blocking pull-to-refresh retraction after the conversation list has already updated.
    @MainActor
    private func refreshAuxiliaryHomeData(baseURL: URL) {
        Task { @MainActor [weak self] in
            guard let self else { return }
            async let quotas: Void = self.fetchQuotas()
            async let tasks: Void = self.fetchGlobalBackgroundTasks()
            async let catalog: Void = ModelCatalogStore.shared.refreshIfNeeded(settings: self.settings, baseURL: baseURL)
            _ = await (quotas, tasks, catalog)
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
        let startEpoch = GatewayRuntimeState.shared.epoch
        if let tasks = try? await apiClient.fetchGlobalBackgroundTasks(baseURL: url),
           GatewayRuntimeState.shared.epoch == startEpoch {
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
        let startEpoch = GatewayRuntimeState.shared.epoch
        do {
            let fetched = try await apiClient.fetchCockpitQuotas(baseURL: url)
            guard GatewayRuntimeState.shared.epoch == startEpoch else { return }
            self.quotaResponse = fetched
            self.lastQuotaFetchTime = now
            publishQuotaToWidget()
        } catch {
            // Silently ignore quota fetch error to not disturb chat list
        }
    }

    @MainActor
    public func publishQuotaToWidget() {
        ModelPickerQuotaCache.shared.account = quotaResponse?.currentAccount
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

    private var lastTokenHistoryFetchTime: Date = .distantPast
    private var lastTokenHistorySessionKey: String? = nil
    private let tokenHistoryRefreshInterval: TimeInterval = 3600 // 1 hour

    @MainActor
    public func fetchTokenHistory(force: Bool = false) async {
        guard settings.isPaired else { return }
        guard !isLoadingTokenHistory else { return }
        guard let url = settings.serverURL else { return }
        let sessionKey = settings.isDemoMode ? "demo" : (settings.deviceToken ?? "")
        let now = Date()
        if !force && tokenHistory != nil && lastTokenHistorySessionKey == sessionKey {
            let elapsed = now.timeIntervalSince(lastTokenHistoryFetchTime)
            if elapsed >= 0 && elapsed < tokenHistoryRefreshInterval {
                return
            }
        }
        isLoadingTokenHistory = true
        defer { isLoadingTokenHistory = false }
        do {
            self.tokenHistory = try await apiClient.fetchTokenHistory(baseURL: url, force: force)
            self.lastTokenHistoryFetchTime = Date()
            self.lastTokenHistorySessionKey = sessionKey
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
        let prevActionIDs = Set(conversations.filter { $0.status.needsAction }.map(\.id))
        let hasNewPendingAction = sorted.contains { $0.status.needsAction && !prevActionIDs.contains($0.id) }
        let prevRunningIDs = Set(conversations.filter { $0.status.isRunning }.map(\.id))
        let hasNewRunning = sorted.contains { $0.status.isRunning && !prevRunningIDs.contains($0.id) }
        if forceReorder || hasNewPendingAction || hasNewRunning || conversations.isEmpty || lastReorderUptime == 0 || (now - lastReorderUptime) >= Self.reorderCooldownSeconds {
            markReorderedNow()
            return sorted
        }

        let newById = Dictionary(uniqueKeysWithValues: sorted.map { ($0.id, $0) })
        let prevIDs = conversations.map(\.id)
        let prevIDSet = Set(prevIDs)
        let preserved = prevIDs.compactMap { newById[$0] }
        let added = sorted.filter { !prevIDSet.contains($0.id) }

        let buffered = (added.filter { $0.status.needsAction } + preserved.filter { $0.status.needsAction }) +
            (added.filter { !$0.status.needsAction && $0.isPinned } + preserved.filter { !$0.status.needsAction && $0.isPinned }) +
            (added.filter { !$0.status.needsAction && !$0.isPinned && $0.status.isRunning } + preserved.filter { !$0.status.needsAction && !$0.isPinned && $0.status.isRunning }) +
            (added.filter { !$0.status.needsAction && !$0.isPinned && !$0.status.isRunning } + preserved.filter { !$0.status.needsAction && !$0.isPinned && !$0.status.isRunning })

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
        var resolved = item.withPinned(cacheManager.isPinned(cascadeId: item.id))
        if let pending = pendingArchiveStates[resolved.id], resolved.isArchived != pending.archived {
            resolved = resolved.withArchived(pending.archived)
        }
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
            withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
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
    public func toggleArchive(item: ConversationItem) {
        guard !item.isDraft else { return }
        let willArchive = !item.isArchived
        let token = UUID()
        pendingArchiveStates[item.id] = (archived: willArchive, token: token)
        recentlyArchivedStates.removeValue(forKey: item.id)

        if let idx = conversations.firstIndex(where: { $0.id == item.id }) {
            var updated = conversations
            updated[idx] = updated[idx].withArchived(willArchive)
            let sorted = Self.sortItems(updated)
            markReorderedNow()

            // When unarchiving the last item in `.archived`, remove the swiped row cleanly first
            // before exiting the filter. Switching `selectedWorkspaceFilter = nil` in the same
            // transaction keeps the swiped cell alive in `ForEach` and forces UICollectionView to
            // move it across newly inserted rows while UISwipeActionController is still dismissing,
            // causing the cell to pause/overlap near the top of the list.
            let shouldAutoExitArchive = !willArchive
                && self.selectedWorkspaceFilter == .archived
                && !sorted.contains(where: {
                    !$0.isSubagent &&
                    $0.isArchived &&
                    !self.pendingDeleteCascadeIDs.contains($0.id) &&
                    self.recentlyDeletedIDs[$0.id] == nil &&
                    !self.cacheManager.isDeletedConversation(cascadeId: $0.id)
                })

            cancelArchiveAutoExit()
            if shouldAutoExitArchive {
                self.isAutoExitingArchive = true
            }

            withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
                self.conversations = sorted
            }

            if shouldAutoExitArchive {
                archiveAutoExitTask = Task { @MainActor [weak self] in
                    try? await Task.sleep(for: .milliseconds(320))
                    guard let self, !Task.isCancelled else { return }
                    guard self.selectedWorkspaceFilter == .archived && self.archivedConversations.isEmpty else {
                        self.cancelArchiveAutoExit()
                        return
                    }
                    withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
                        self.isAutoExitingArchive = false
                        self.selectedWorkspaceFilter = nil
                    }
                }
            }

            cacheManager.saveConversations(sorted.filter { !$0.isDraft && !$0.isSubagent })
        }

        guard let url = settings.serverURL, settings.isPaired else {
            pendingArchiveStates.removeValue(forKey: item.id)
            return
        }
        Task {
            do {
                try await apiClient.setConversationArchived(cascadeId: item.id, archived: willArchive, baseURL: url)
                if self.pendingArchiveStates[item.id]?.token == token {
                    self.pendingArchiveStates.removeValue(forKey: item.id)
                    self.recentlyArchivedStates[item.id] = (archived: willArchive, settledAt: Date())
                }
            } catch {
                // Revert optimistic state if remote update fails
                if self.pendingArchiveStates[item.id]?.token == token {
                    self.pendingArchiveStates.removeValue(forKey: item.id)
                    self.recentlyArchivedStates.removeValue(forKey: item.id)
                    self.cancelArchiveAutoExit()
                    if let idx = self.conversations.firstIndex(where: { $0.id == item.id }) {
                        var reverted = self.conversations
                        reverted[idx] = reverted[idx].withArchived(!willArchive)
                        let sorted = Self.sortItems(reverted)
                        withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
                            self.conversations = sorted
                        }
                        self.cacheManager.saveConversations(sorted.filter { !$0.isDraft && !$0.isSubagent })
                    }
                }
            }
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
            self.conversations[idx] = self.conversations[idx].withTitle(trimmed)
        }
        self.cacheManager.updateConversationTitle(cascadeId: item.id, newTitle: trimmed)
        ActivityManager.shared.updateTitle(cascadeId: item.id, newTitle: trimmed)
        
        do {
            try await apiClient.renameConversation(cascadeId: item.id, newTitle: trimmed, baseURL: url)
        } catch {
            // Revert on failure
            ActivityManager.shared.updateTitle(cascadeId: item.id, newTitle: originalTitle)
            if let idx = self.conversations.firstIndex(where: { $0.id == item.id }) {
                self.conversations[idx] = self.conversations[idx].withTitle(originalTitle)
            }
            self.cacheManager.updateConversationTitle(cascadeId: item.id, newTitle: originalTitle)
            self.errorMessage = String(localized: "重命名会话失败: \(error.localizedDescription)")
        }
    }
}
