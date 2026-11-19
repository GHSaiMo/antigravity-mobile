import Foundation
import ActivityKit
import UIKit

@MainActor
public final class ActivityManager {
    public static let shared = ActivityManager()
    
    private var activeActivitiesByCascade: [String: Activity<AgentActivityAttributes>] = [:]
    private var observersStarted = false
    private var pushToStartTask: Task<Void, Never>?
    private var activityUpdatesTask: Task<Void, Never>?
    private var activityTokenTasks: [String: Task<Void, Never>] = [:]
    private var activityStateTasks: [String: Task<Void, Never>] = [:]
    private var startingCascadeIds: Set<String> = []
    private var activityStartDates: [String: Date] = [:]
    private var locallyDismissedCascadeIds: Set<String> = []
    
    private static let optimisticStartGraceSeconds: TimeInterval = 8.0
    /// Max concurrent session cards (needs-action first, then most recently active).
    static let maxConcurrentCards = 3
    
    public init() {
        deduplicateActivities()
    }

    private func isAlive(_ state: ActivityState) -> Bool {
        state == .active || state == .stale
    }

    private func rankActivity(_ lhs: Activity<AgentActivityAttributes>, _ rhs: Activity<AgentActivityAttributes>) -> Bool {
        if (lhs.activityState == .active) != (rhs.activityState == .active) {
            return lhs.activityState == .active
        }
        if lhs.content.state.lastUpdated != rhs.content.state.lastUpdated {
            return lhs.content.state.lastUpdated > rhs.content.state.lastUpdated
        }
        if lhs.content.state.stepCount != rhs.content.state.stepCount {
            return lhs.content.state.stepCount > rhs.content.state.stepCount
        }
        return (lhs.pushToken != nil) && (rhs.pushToken == nil)
    }

    /// Starts observing ActivityKit push-to-start tokens and active Live Activity update tokens
    /// so the local gateway can remotely launch, update, and end Dynamic Island sessions via APNs.
    public func startRemoteTokenObservers() {
        guard !observersStarted else { return }
        observersStarted = true

        // 1. Observe Push-to-Start token (allows gateway to launch Live Activity even when app is closed)
        pushToStartTask = Task {
            for await tokenData in Activity<AgentActivityAttributes>.pushToStartTokenUpdates {
                let hex = tokenData.map { String(format: "%02x", $0) }.joined()
                PushNotificationManager.shared.updateLiveActivityStartToken(hex)
            }
        }

        // 2. Deduplicate and attach observers to any already-alive activities on launch
        reconcileActiveActivities()

        // 3. Observe newly created activities (including ones launched remotely via Push-to-Start!)
        activityUpdatesTask = Task { [weak self] in
            for await activity in Activity<AgentActivityAttributes>.activityUpdates {
                guard let self else { break }
                let cid = activity.attributes.cascadeId
                if self.isAlive(activity.activityState) {
                    self.locallyDismissedCascadeIds.remove(cid)
                    self.deduplicateActivities(for: cid)
                    if let winner = self.findActiveActivity(for: cid) {
                        self.observePushToken(for: winner)
                        self.observeStateUpdates(for: winner)
                        if let existingToken = winner.pushToken {
                            let hex = existingToken.map { String(format: "%02x", $0) }.joined()
                            PushNotificationManager.shared.updateLiveActivityToken(cascadeId: cid, tokenHex: hex)
                        }
                    }
                } else {
                    self.handleActivityEnded(activity, wasDismissed: activity.activityState == .dismissed)
                }
            }
        }
    }

    /// Reconciles active Live Activities on launch or foreground resume: ensures 1 activity per cascade and fresh token observers.
    public func reconcileActiveActivities() {
        deduplicateActivities()
        for activity in activeActivitiesByCascade.values where isAlive(activity.activityState) {
            observePushToken(for: activity)
            observeStateUpdates(for: activity)
            if let existingToken = activity.pushToken {
                let hex = existingToken.map { String(format: "%02x", $0) }.joined()
                PushNotificationManager.shared.updateLiveActivityToken(cascadeId: activity.attributes.cascadeId, tokenHex: hex)
            }
        }
    }

    private func observePushToken(for activity: Activity<AgentActivityAttributes>) {
        let activityId = activity.id
        let cascadeId = activity.attributes.cascadeId
        guard activityTokenTasks[activityId] == nil else { return }

        // Report current pushToken immediately if already populated
        if let existingToken = activity.pushToken {
            let hex = existingToken.map { String(format: "%02x", $0) }.joined()
            PushNotificationManager.shared.updateLiveActivityToken(cascadeId: cascadeId, tokenHex: hex)
        }

        activityTokenTasks[activityId] = Task { [weak self] in
            for await tokenData in activity.pushTokenUpdates {
                guard let self else { break }
                guard self.activeActivitiesByCascade[cascadeId]?.id == activityId else { continue }
                let hex = tokenData.map { String(format: "%02x", $0) }.joined()
                PushNotificationManager.shared.updateLiveActivityToken(cascadeId: cascadeId, tokenHex: hex)
            }
            self?.activityTokenTasks.removeValue(forKey: activityId)
        }
    }

    private func observeStateUpdates(for activity: Activity<AgentActivityAttributes>) {
        let activityId = activity.id
        guard activityStateTasks[activityId] == nil else { return }

        activityStateTasks[activityId] = Task { [weak self] in
            for await state in activity.activityStateUpdates {
                if state == .ended || state == .dismissed {
                    self?.handleActivityEnded(activity, wasDismissed: state == .dismissed)
                    break
                }
            }
            self?.activityStateTasks.removeValue(forKey: activityId)
        }
    }

    private func handleActivityEnded(_ activity: Activity<AgentActivityAttributes>, wasDismissed: Bool) {
        let activityId = activity.id
        let cid = activity.attributes.cascadeId
        activityTokenTasks[activityId]?.cancel()
        activityTokenTasks.removeValue(forKey: activityId)
        activityStateTasks[activityId]?.cancel()
        activityStateTasks.removeValue(forKey: activityId)

        guard activeActivitiesByCascade[cid]?.id == activityId else { return }

        if let replacement = Activity<AgentActivityAttributes>.activities
            .filter({ isAlive($0.activityState) && $0.attributes.cascadeId == cid && $0.id != activityId })
            .sorted(by: rankActivity)
            .first {
            activeActivitiesByCascade[cid] = replacement
            observePushToken(for: replacement)
            observeStateUpdates(for: replacement)
            return
        }

        activeActivitiesByCascade.removeValue(forKey: cid)
        activityStartDates.removeValue(forKey: cid)
        PushNotificationManager.shared.dropCachedLiveActivityToken(cascadeId: cid)
        if wasDismissed {
            locallyDismissedCascadeIds.insert(cid)
        }
    }
    
    public var isLiveActivityEnabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }
    
    public var currentActivity: Activity<AgentActivityAttributes>? {
        let alive = activeActivitiesByCascade.values.filter { isAlive($0.activityState) }
        if let best = alive.sorted(by: rankActivity).first {
            return best
        }
        deduplicateActivities()
        return activeActivitiesByCascade.values.sorted(by: rankActivity).first
    }
    
    public var currentCascadeId: String? {
        currentActivity?.attributes.cascadeId
    }
    
    public var hasActiveActivity: Bool {
        currentActivity != nil
    }

    public func hasActiveActivity(for cascadeId: String) -> Bool {
        findActiveActivity(for: cascadeId) != nil
    }
    
    public func findActiveActivity(for cascadeId: String) -> Activity<AgentActivityAttributes>? {
        guard !cascadeId.isEmpty else { return nil }
        if let tracked = activeActivitiesByCascade[cascadeId], isAlive(tracked.activityState) {
            return tracked
        }
        let matching = Activity<AgentActivityAttributes>.activities
            .filter { isAlive($0.activityState) && $0.attributes.cascadeId == cascadeId }
            .sorted(by: rankActivity)
        if let best = matching.first {
            activeActivitiesByCascade[cascadeId] = best
            return best
        }
        activeActivitiesByCascade.removeValue(forKey: cascadeId)
        return nil
    }
    
    /// Ensures each working cascadeId has at most ONE alive Live Activity without terminating other working cascades.
    public func deduplicateActivities(for targetCascadeId: String? = nil) {
        let aliveActivities = Activity<AgentActivityAttributes>.activities.filter { activity in
            guard isAlive(activity.activityState) else { return false }
            if let targetCascadeId {
                return activity.attributes.cascadeId == targetCascadeId
            }
            return true
        }

        if targetCascadeId == nil {
            let aliveCids = Set(aliveActivities.map { $0.attributes.cascadeId })
            for cid in Array(activeActivitiesByCascade.keys) where !aliveCids.contains(cid) {
                activeActivitiesByCascade.removeValue(forKey: cid)
            }
        }

        let grouped = Dictionary(grouping: aliveActivities, by: { $0.attributes.cascadeId })
        for (cid, group) in grouped {
            let sorted = group.sorted(by: rankActivity)
            guard let primary = sorted.first else { continue }
            activeActivitiesByCascade[cid] = primary
            observePushToken(for: primary)
            observeStateUpdates(for: primary)

            for duplicate in sorted.dropFirst() {
                activityTokenTasks[duplicate.id]?.cancel()
                activityTokenTasks.removeValue(forKey: duplicate.id)
                activityStateTasks[duplicate.id]?.cancel()
                activityStateTasks.removeValue(forKey: duplicate.id)
                Task {
                    await duplicate.end(nil, dismissalPolicy: .immediate)
                }
            }
        }
    }
    
    public func startActivity(
        title: String,
        cascadeId: String,
        status: String = "RUNNING",
        stepCount: Int = 1,
        latestAction: String = String(localized: "开始执行任务..."),
        runningTaskCount: Int = 0,
        runningTasks: [AgentTaskSnapshot] = [],
        activeTaskTitle: String? = nil,
        activeTaskCommand: String? = nil,
        hasPendingAction: Bool = false,
        pendingStepIndex: Int? = nil,
        pendingActionKind: String? = nil
    ) {
        guard isLiveActivityEnabled,
              !cascadeId.isEmpty,
              !cascadeId.hasPrefix("local_draft_"),
              !cascadeId.hasPrefix("draft_") else { return }

        locallyDismissedCascadeIds.remove(cascadeId)
        deduplicateActivities(for: cascadeId)

        // 1. If an alive activity already exists for this conversation, update in-place to avoid duplicates
        if findActiveActivity(for: cascadeId) != nil {
            updateActivity(
                cascadeId: cascadeId,
                title: title,
                status: status,
                stepCount: stepCount,
                latestAction: latestAction,
                runningTaskCount: runningTaskCount,
                runningTasks: runningTasks,
                activeTaskTitle: activeTaskTitle,
                activeTaskCommand: activeTaskCommand,
                hasPendingAction: hasPendingAction,
                pendingStepIndex: pendingStepIndex,
                pendingActionKind: pendingActionKind
            )
            return
        }

        // Cap concurrent cards: needs-action sessions outrank routine ones.
        if activeActivitiesByCascade.count >= Self.maxConcurrentCards {
            guard hasPendingAction else { return }
            let victim = activeActivitiesByCascade
                .filter { !$0.value.content.state.hasPendingAction }
                .min { (activityStartDates[$0.key] ?? .distantPast) < (activityStartDates[$1.key] ?? .distantPast) }?.key
            guard let victim else { return }
            endActivity(for: victim, finalStatus: "COMPLETED", dismissalPolicy: .immediate)
        }

        // Guard against re-entrant duplicate requests for the same cascade
        guard !startingCascadeIds.contains(cascadeId) else { return }
        startingCascadeIds.insert(cascadeId)
        defer { startingCascadeIds.remove(cascadeId) }
        
        let resolvedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let safeTitle = resolvedTitle.isEmpty ? "Multigravity Agent" : resolvedTitle
        let attributes = AgentActivityAttributes(conversationTitle: safeTitle, cascadeId: cascadeId)
        let initialContentState = AgentActivityAttributes.ContentState(
            conversationTitle: safeTitle,
            status: status,
            stepCount: max(1, stepCount),
            latestAction: latestAction.isEmpty ? (runningTaskCount > 0 ? String(localized: "正在执行后台任务...") : String(localized: "正在执行...")) : latestAction,
            runningTaskCount: runningTaskCount,
            runningTasks: runningTasks,
            activeTaskTitle: activeTaskTitle,
            activeTaskCommand: activeTaskCommand,
            hasPendingAction: hasPendingAction,
            pendingStepIndex: pendingStepIndex,
            pendingActionKind: pendingActionKind,
            lastUpdated: Date()
        )
        
        do {
            let activity = try Activity.request(
                attributes: attributes,
                content: .init(state: initialContentState, staleDate: nil),
                pushType: .token
            )
            activeActivitiesByCascade[cascadeId] = activity
            activityStartDates[cascadeId] = Date()
            observePushToken(for: activity)
            observeStateUpdates(for: activity)
            deduplicateActivities(for: cascadeId)
        } catch {
            print("[LiveActivity] Failed to start for \(cascadeId): \(error)")
        }
    }
    
    public func updateActivity(
        cascadeId: String? = nil,
        title: String? = nil,
        status: String,
        stepCount: Int,
        latestAction: String,
        runningTaskCount: Int = 0,
        runningTasks: [AgentTaskSnapshot] = [],
        activeTaskTitle: String? = nil,
        activeTaskCommand: String? = nil,
        hasPendingAction: Bool = false,
        pendingStepIndex: Int? = nil,
        pendingActionKind: String? = nil
    ) {
        let activity: Activity<AgentActivityAttributes>?
        if let cid = cascadeId, !cid.isEmpty {
            activity = findActiveActivity(for: cid)
        } else {
            activity = currentActivity
        }
        guard let activity, isAlive(activity.activityState) else { return }
        
        let actionText: String
        if !latestAction.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            actionText = latestAction
        } else if runningTaskCount > 0 {
            actionText = String(localized: "正在执行后台任务...")
        } else if stepCount > 0 {
            actionText = String(localized: "正在执行第 \(stepCount) 步...")
        } else {
            actionText = String(localized: "正在执行...")
        }
        
        let resolvedTitle: String
        if let newTitle = title?.trimmingCharacters(in: .whitespacesAndNewlines), !newTitle.isEmpty {
            resolvedTitle = newTitle
        } else if !activity.content.state.conversationTitle.isEmpty {
            resolvedTitle = activity.content.state.conversationTitle
        } else {
            resolvedTitle = activity.attributes.conversationTitle
        }
        
        let updatedState = AgentActivityAttributes.ContentState(
            conversationTitle: resolvedTitle,
            status: status,
            stepCount: stepCount,
            latestAction: actionText,
            runningTaskCount: runningTaskCount,
            runningTasks: runningTasks,
            activeTaskTitle: activeTaskTitle,
            activeTaskCommand: activeTaskCommand,
            hasPendingAction: hasPendingAction,
            pendingStepIndex: hasPendingAction ? pendingStepIndex : nil,
            pendingActionKind: hasPendingAction ? pendingActionKind : nil,
            lastUpdated: Date()
        )
        
        Task {
            await activity.update(.init(state: updatedState, staleDate: nil))
        }
    }
    
    public func updateTitle(cascadeId: String, newTitle: String) {
        guard let activity = findActiveActivity(for: cascadeId) else { return }
        updateActivity(
            cascadeId: cascadeId,
            title: newTitle,
            status: activity.content.state.status,
            stepCount: activity.content.state.stepCount,
            latestAction: activity.content.state.latestAction,
            runningTaskCount: activity.content.state.runningTaskCount,
            runningTasks: activity.content.state.runningTasks,
            activeTaskTitle: activity.content.state.activeTaskTitle,
            activeTaskCommand: activity.content.state.activeTaskCommand,
            hasPendingAction: activity.content.state.hasPendingAction,
            pendingStepIndex: activity.content.state.pendingStepIndex,
            pendingActionKind: activity.content.state.pendingActionKind
        )
    }

    public func endActivity(
        for cascadeId: String? = nil,
        finalStatus: String = "COMPLETED",
        dismissalPolicy: ActivityUIDismissalPolicy = .after(Date().addingTimeInterval(4))
    ) {
        guard let targetCascadeId = cascadeId?.trimmingCharacters(in: .whitespacesAndNewlines),
              !targetCascadeId.isEmpty else {
            endAllActivities(finalStatus: finalStatus, dismissalPolicy: dismissalPolicy)
            return
        }

        let matching = Activity<AgentActivityAttributes>.activities.filter {
            isAlive($0.activityState) && $0.attributes.cascadeId == targetCascadeId
        }
        activeActivitiesByCascade.removeValue(forKey: targetCascadeId)
        activityStartDates.removeValue(forKey: targetCascadeId)
        locallyDismissedCascadeIds.remove(targetCascadeId)
        PushNotificationManager.shared.clearLiveActivityToken(cascadeId: targetCascadeId)

        guard !matching.isEmpty else { return }
        endActivitiesList(matching, finalStatus: finalStatus, dismissalPolicy: dismissalPolicy)
    }

    public func endAllActivities(
        finalStatus: String = "CANCELLED",
        dismissalPolicy: ActivityUIDismissalPolicy = .immediate
    ) {
        let alive = Activity<AgentActivityAttributes>.activities.filter { isAlive($0.activityState) }
        let cids = Set(alive.map { $0.attributes.cascadeId }).union(activeActivitiesByCascade.keys)
        activeActivitiesByCascade.removeAll()
        activityStartDates.removeAll()
        locallyDismissedCascadeIds.removeAll()
        for cid in cids {
            PushNotificationManager.shared.clearLiveActivityToken(cascadeId: cid)
        }
        guard !alive.isEmpty else { return }
        endActivitiesList(alive, finalStatus: finalStatus, dismissalPolicy: dismissalPolicy)
    }

    private func endActivitiesList(
        _ activities: [Activity<AgentActivityAttributes>],
        finalStatus: String,
        dismissalPolicy: ActivityUIDismissalPolicy
    ) {
        let summary: String
        switch finalStatus {
        case "COMPLETED":
            summary = String(localized: "任务已完成")
        case "CANCELLED":
            summary = String(localized: "任务已终止")
        case "FAILED":
            summary = String(localized: "执行遇到错误")
        default:
            summary = String(localized: "执行已结束")
        }

        for activity in activities {
            activityTokenTasks[activity.id]?.cancel()
            activityTokenTasks.removeValue(forKey: activity.id)
            activityStateTasks[activity.id]?.cancel()
            activityStateTasks.removeValue(forKey: activity.id)

            let finalTitle = !activity.content.state.conversationTitle.isEmpty
                ? activity.content.state.conversationTitle
                : activity.attributes.conversationTitle
            let finalState = AgentActivityAttributes.ContentState(
                conversationTitle: finalTitle,
                status: finalStatus,
                stepCount: activity.content.state.stepCount,
                latestAction: summary,
                runningTaskCount: 0,
                runningTasks: [],
                activeTaskTitle: nil,
                activeTaskCommand: nil,
                hasPendingAction: false,
                lastUpdated: Date()
            )

            Task {
                await activity.end(
                    .init(state: finalState, staleDate: nil),
                    dismissalPolicy: dismissalPolicy
                )
            }
        }
    }

    /// Reconciles Live Activities with the authoritative server conversation list:
    /// guarantees 1 dedicated Live Activity per working session and ends finished/orphaned ones.
    public func syncWithConversations(_ conversations: [ConversationItem]) {
        guard AppSettings.shared.enableLiveActivities, isLiveActivityEnabled else {
            endAllActivities(finalStatus: "CANCELLED", dismissalPolicy: .immediate)
            return
        }

        deduplicateActivities()

        let convById = Dictionary(conversations.map { ($0.id, $0) }, uniquingKeysWith: { _, new in new })
        let now = Date()

        // 1. Reconcile existing alive activities against server state
        let aliveSnapshot = Array(activeActivitiesByCascade)
        for (cid, activity) in aliveSnapshot {
            if let item = convById[cid] {
                let isWorking = item.status.isRunning || item.status.needsAction || item.hasRunningSubagents
                if !isWorking {
                    locallyDismissedCascadeIds.remove(cid)
                    if let startedAt = activityStartDates[cid],
                       now.timeIntervalSince(startedAt) < Self.optimisticStartGraceSeconds {
                        continue
                    }
                    let finalStatus = item.status.isError ? "FAILED" : "COMPLETED"
                    endActivity(for: cid, finalStatus: finalStatus)
                } else {
                    let currentTitle = activity.content.state.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
                    let incomingTitle = item.title.trimmingCharacters(in: .whitespacesAndNewlines)
                    if (currentTitle.isEmpty || currentTitle == "未命名会话" || currentTitle == "Multigravity Agent"),
                       !incomingTitle.isEmpty, incomingTitle != "未命名会话" {
                        updateTitle(cascadeId: cid, newTitle: incomingTitle)
                    }
                }
            } else if !conversations.isEmpty {
                if let startedAt = activityStartDates[cid],
                   now.timeIntervalSince(startedAt) < Self.optimisticStartGraceSeconds {
                    continue
                }
                endActivity(for: cid, finalStatus: "COMPLETED")
            }
        }

        // 2. Rank working sessions (needs-action first, then most recent) and keep only the top N cards
        var working: [ConversationItem] = []
        for item in conversations {
            let isWorking = item.status.isRunning || item.status.needsAction || item.hasRunningSubagents
            if !isWorking {
                locallyDismissedCascadeIds.remove(item.id)
                continue
            }
            guard !item.isSubagent, !item.isDraft else { continue }
            working.append(item)
        }
        working.sort {
            if $0.status.needsAction != $1.status.needsAction { return $0.status.needsAction }
            return ($0.lastModified ?? .distantPast) > ($1.lastModified ?? .distantPast)
        }
        let allowed = Set(working.prefix(Self.maxConcurrentCards).map { $0.id })
        if !conversations.isEmpty {
            for cid in Array(activeActivitiesByCascade.keys) where !allowed.contains(cid) {
                if let item = convById[cid], !(item.status.isRunning || item.status.needsAction || item.hasRunningSubagents) {
                    continue // finished sessions are handled above
                }
                if let startedAt = activityStartDates[cid],
                   now.timeIntervalSince(startedAt) < Self.optimisticStartGraceSeconds, convById[cid] == nil {
                    continue
                }
                endActivity(for: cid, finalStatus: "COMPLETED", dismissalPolicy: .immediate)
            }
        }

        // 3. Ensure the allowed working sessions have 1 dedicated Live Activity when app is in foreground
        for item in working.prefix(Self.maxConcurrentCards) {
            guard UIApplication.shared.applicationState == .active else { continue }
            guard !locallyDismissedCascadeIds.contains(item.id) else { continue }
            if findActiveActivity(for: item.id) == nil {
                let status = item.status.needsAction ? "WAITING_APPROVAL" : "RUNNING"
                let actionText = item.status.needsAction
                    ? String(localized: "等待用户确认")
                    : String(localized: "正在执行任务...")
                startActivity(
                    title: item.title,
                    cascadeId: item.id,
                    status: status,
                    stepCount: max(1, item.stepCount),
                    latestAction: actionText,
                    hasPendingAction: item.status.needsAction
                )
            }
        }
    }
    
    // MARK: - Audio Recording Live Activity
    
    private var _recordingActivity: Activity<AudioRecordingActivityAttributes>?
    
    public var currentRecordingActivity: Activity<AudioRecordingActivityAttributes>? {
        if let act = _recordingActivity, act.activityState == .active {
            return act
        }
        let active = Activity<AudioRecordingActivityAttributes>.activities.first(where: { $0.activityState == .active })
        _recordingActivity = active
        return active
    }
    
    public func startRecordingActivity(
        cascadeId: String,
        title: String,
        startDate: Date = Date()
    ) {
        guard isLiveActivityEnabled else { return }
        cleanUpRecordingActivities()
        
        let attributes = AudioRecordingActivityAttributes(cascadeId: cascadeId)
        let initialContentState = AudioRecordingActivityAttributes.ContentState(
            recordingStartDate: startDate,
            isPaused: false,
            pausedDuration: 0,
            conversationTitle: title,
            cascadeId: cascadeId
        )
        
        do {
            let activity = try Activity.request(
                attributes: attributes,
                content: .init(state: initialContentState, staleDate: nil, relevanceScore: 100.0)
            )
            self._recordingActivity = activity
        } catch {
            print("[LiveActivity] Failed to start recording activity: \(error)")
        }
    }
    
    public func updateRecordingActivity(
        isPaused: Bool,
        pausedDuration: TimeInterval,
        title: String? = nil
    ) {
        guard let activity = currentRecordingActivity, activity.activityState == .active else { return }
        
        let startDate: Date
        if isPaused {
            startDate = activity.content.state.recordingStartDate
        } else {
            startDate = Date().addingTimeInterval(-pausedDuration)
        }
        
        let resolvedTitle = title ?? activity.content.state.conversationTitle
        let updatedState = AudioRecordingActivityAttributes.ContentState(
            recordingStartDate: startDate,
            isPaused: isPaused,
            pausedDuration: pausedDuration,
            conversationTitle: resolvedTitle,
            cascadeId: activity.attributes.cascadeId
        )
        
        Task {
            await activity.update(.init(state: updatedState, staleDate: nil, relevanceScore: 100.0))
        }
    }
    
    public func endRecordingActivity() {
        let activeActivities = Activity<AudioRecordingActivityAttributes>.activities.filter { $0.activityState == .active }
        self._recordingActivity = nil
        for activity in activeActivities {
            Task {
                await activity.end(nil, dismissalPolicy: .immediate)
            }
        }
    }
    
    public func cleanUpRecordingActivities() {
        let activeActivities = Activity<AudioRecordingActivityAttributes>.activities.filter { $0.activityState == .active }
        for activity in activeActivities {
            Task {
                await activity.end(nil, dismissalPolicy: .immediate)
            }
        }
        self._recordingActivity = nil
    }
}
