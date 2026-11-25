import AppIntents
import Foundation

/// Interactive Live Activity intents shared between `Multigravity` and `ActivityExtension`.
/// iOS executes `perform()` inside the host `Multigravity` app process without foregrounding the UI.

#if !ACTIVITY_EXTENSION
@MainActor
private func resolveIntentGatewayContext(gatewayId: String, cascadeId: String) -> GatewayRequestContext? {
    let store = GatewayProfileStore.shared
    let cleanGw = gatewayId.trimmingCharacters(in: .whitespacesAndNewlines)
    if !cleanGw.isEmpty, let profile = store.profile(matchingGatewayId: cleanGw) {
        return store.requestContext(for: profile.id)
    }
    if !cascadeId.isEmpty, let pid = store.profileId(containingCachedCascade: cascadeId) {
        return store.requestContext(for: pid)
    }
    if let activeId = store.activeProfileId {
        return store.requestContext(for: activeId)
    }
    return nil
}
#endif

nonisolated public struct ApproveAgentActionIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "允许执行"
    public static let openAppWhenRun: Bool = false
    public static var authenticationPolicy: IntentAuthenticationPolicy { .requiresAuthentication }

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    @Parameter(title: "Expected Step Index", default: -1)
    public var expectedStepIndex: Int

    @Parameter(title: "Gateway ID", default: "")
    public var gatewayId: String

    public init() {
        self.cascadeId = ""
        self.expectedStepIndex = -1
        self.gatewayId = ""
    }

    public init(cascadeId: String, expectedStepIndex: Int = -1, gatewayId: String = "") {
        self.cascadeId = cascadeId
        self.expectedStepIndex = expectedStepIndex
        self.gatewayId = gatewayId
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty else {
            return .result()
        }
        let targetCtx = resolveIntentGatewayContext(gatewayId: gatewayId, cascadeId: cascadeId)
        let snap: FetchMessagesResult?
        if let targetCtx {
            snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, context: targetCtx)
        } else if let baseURL = AppSettings.shared.serverURL {
            snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        } else {
            snap = nil
        }
        guard let snap else {
            return .result()
        }
        if let interaction = snap.pendingInteraction {
            // 1. 校验步骤序号，防止在解锁/等待期间审批内容发生切换导致盲批
            if expectedStepIndex >= 0 && interaction.stepIndex != expectedStepIndex {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "审批内容已更新，请重新确认"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
                return .result()
            }
            // 2. 多题问答、工作流编排脚本、表单等需审阅详情，禁止在锁屏后台盲目放行
            let kind = interaction.type.lowercased()
            if kind == "ask_question" || kind == "workflow_approval" || kind == "elicitation" || (interaction.questions?.isEmpty == false) {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "请打开 App 查看完整详情并确认"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
                return .result()
            }
            let approveOpt = interaction.options.first(where: {
                $0.isDeny != true && $0.id != "5" && $0.id != "__write_in__"
            }) ?? interaction.options.first
            let optId = approveOpt?.id ?? interaction.defaultOptionId ?? "1"
            let scope = approveOpt?.scope ?? 1
            do {
                if let targetCtx {
                    try await APIClient.shared.submitInteraction(
                        cascadeId: cascadeId,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: optId,
                        scope: scope,
                        allow: true,
                        writeInResponse: "",
                        skipped: false,
                        target: interaction.target,
                        questionResponses: nil,
                        context: targetCtx
                    )
                } else if let baseURL = AppSettings.shared.serverURL {
                    try await APIClient.shared.submitInteraction(
                        cascadeId: cascadeId,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: optId,
                        scope: scope,
                        allow: true,
                        writeInResponse: "",
                        skipped: false,
                        target: interaction.target,
                        questionResponses: nil,
                        baseURL: baseURL
                    )
                }
                PushNotificationManager.shared.clearDeliveredNotifications(for: cascadeId, approvalOnly: true)
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "RUNNING",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "已允许，继续执行中..."),
                        hasPendingAction: false,
                        pendingStepIndex: nil,
                        pendingActionKind: nil
                    )
                }
            } catch {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "操作失败，请重试"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
            }
        } else if snap.canProceed, let uri = snap.proceedArtifactUri, !uri.isEmpty {
            do {
                if let targetCtx {
                    try await APIClient.shared.proceedArtifact(
                        cascadeId: cascadeId,
                        artifactUri: uri,
                        model: snap.activeModel,
                        cascadeConfigRaw: snap.cascadeConfigRaw,
                        context: targetCtx
                    )
                } else if let baseURL = AppSettings.shared.serverURL {
                    try await APIClient.shared.proceedArtifact(
                        cascadeId: cascadeId,
                        artifactUri: uri,
                        model: snap.activeModel,
                        cascadeConfigRaw: snap.cascadeConfigRaw,
                        baseURL: baseURL
                    )
                }
                PushNotificationManager.shared.clearDeliveredNotifications(for: cascadeId, approvalOnly: true)
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "RUNNING",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "已允许，继续执行中..."),
                        hasPendingAction: false,
                        pendingStepIndex: nil,
                        pendingActionKind: nil
                    )
                }
            } catch {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "操作失败，请重试"),
                        hasPendingAction: true,
                        pendingStepIndex: nil,
                        pendingActionKind: "proceed"
                    )
                }
            }
        } else {
            reconcileStaleLiveActivity(cascadeId: cascadeId, snap: snap)
        }
        #endif
        return .result()
    }
}

nonisolated public struct RejectAgentActionIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "拒绝操作"
    public static let openAppWhenRun: Bool = false
    public static var authenticationPolicy: IntentAuthenticationPolicy { .requiresAuthentication }

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    @Parameter(title: "Expected Step Index", default: -1)
    public var expectedStepIndex: Int

    @Parameter(title: "Gateway ID", default: "")
    public var gatewayId: String

    public init() {
        self.cascadeId = ""
        self.expectedStepIndex = -1
        self.gatewayId = ""
    }

    public init(cascadeId: String, expectedStepIndex: Int = -1, gatewayId: String = "") {
        self.cascadeId = cascadeId
        self.expectedStepIndex = expectedStepIndex
        self.gatewayId = gatewayId
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty else {
            return .result()
        }
        let targetCtx = resolveIntentGatewayContext(gatewayId: gatewayId, cascadeId: cascadeId)
        let snap: FetchMessagesResult?
        if let targetCtx {
            snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, context: targetCtx)
        } else if let baseURL = AppSettings.shared.serverURL {
            snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        } else {
            snap = nil
        }
        guard let snap else {
            return .result()
        }
        if let interaction = snap.pendingInteraction {
            if expectedStepIndex >= 0 && interaction.stepIndex != expectedStepIndex {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "审批内容已更新，请重新确认"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
                return .result()
            }
            let denyOpt = interaction.options.first(where: { $0.isDeny == true || $0.id == "5" })
            do {
                if let targetCtx {
                    try await APIClient.shared.submitInteraction(
                        cascadeId: cascadeId,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: denyOpt?.id ?? "",
                        scope: denyOpt?.scope ?? 1,
                        allow: false,
                        writeInResponse: "",
                        skipped: denyOpt == nil,
                        target: interaction.target,
                        questionResponses: nil,
                        context: targetCtx
                    )
                } else if let baseURL = AppSettings.shared.serverURL {
                    try await APIClient.shared.submitInteraction(
                        cascadeId: cascadeId,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: denyOpt?.id ?? "",
                        scope: denyOpt?.scope ?? 1,
                        allow: false,
                        writeInResponse: "",
                        skipped: denyOpt == nil,
                        target: interaction.target,
                        questionResponses: nil,
                        baseURL: baseURL
                    )
                }
                PushNotificationManager.shared.clearDeliveredNotifications(for: cascadeId, approvalOnly: true)
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "RUNNING",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "已拒绝该操作"),
                        hasPendingAction: false,
                        pendingStepIndex: nil,
                        pendingActionKind: nil
                    )
                }
            } catch {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap.totalSteps),
                        latestAction: String(localized: "操作失败，请重试"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
            }
        } else {
            reconcileStaleLiveActivity(cascadeId: cascadeId, snap: snap)
        }
        #endif
        return .result()
    }
}

#if !ACTIVITY_EXTENSION
@MainActor
private func reconcileStaleLiveActivity(cascadeId: String, snap: FetchMessagesResult) {
    PushNotificationManager.shared.clearDeliveredNotifications(for: cascadeId, approvalOnly: true)
    guard ActivityManager.shared.findActiveActivity(for: cascadeId) != nil else { return }

    let upperStatus = snap.status.uppercased()
    let isStillRunning = upperStatus == "CASCADE_RUN_STATUS_RUNNING" || upperStatus == "RUNNING" || !snap.runningTasks.isEmpty
    if isStillRunning {
        ActivityManager.shared.updateActivity(
            cascadeId: cascadeId,
            title: snap.title,
            status: "RUNNING",
            stepCount: max(1, snap.totalSteps),
            latestAction: String(localized: "已在其他设备处理，继续执行中"),
            hasPendingAction: false,
            pendingStepIndex: nil,
            pendingActionKind: nil
        )
    } else {
        let isFailed = snap.hasError || upperStatus.contains("FAIL") || upperStatus.contains("ERROR")
        let isCancelled = upperStatus.contains("CANCEL")
        let finalStatus = isFailed ? "FAILED" : (isCancelled ? "CANCELLED" : "COMPLETED")
        ActivityManager.shared.endActivity(for: cascadeId, finalStatus: finalStatus)
    }
}
#endif

nonisolated public struct StopAgentRunIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "停止任务"
    public static let openAppWhenRun: Bool = false
    public static var authenticationPolicy: IntentAuthenticationPolicy { .requiresAuthentication }

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    @Parameter(title: "Gateway ID", default: "")
    public var gatewayId: String

    public init() {
        self.cascadeId = ""
        self.gatewayId = ""
    }

    public init(cascadeId: String, gatewayId: String = "") {
        self.cascadeId = cascadeId
        self.gatewayId = gatewayId
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty else {
            return .result()
        }
        if let targetCtx = resolveIntentGatewayContext(gatewayId: gatewayId, cascadeId: cascadeId) {
            try? await APIClient.shared.cancelTask(cascadeId: cascadeId, killBackgroundTasks: true, context: targetCtx)
        } else if let baseURL = AppSettings.shared.serverURL {
            try? await APIClient.shared.cancelTask(cascadeId: cascadeId, killBackgroundTasks: true, baseURL: baseURL)
        }
        if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
            ActivityManager.shared.endActivity(for: cascadeId, finalStatus: "CANCELLED")
        }
        #endif
        return .result()
    }
}
