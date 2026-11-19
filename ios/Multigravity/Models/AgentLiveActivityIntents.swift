import AppIntents
import Foundation

/// Interactive Live Activity intents shared between `Multigravity` and `ActivityExtension`.
/// iOS executes `perform()` inside the host `Multigravity` app process without foregrounding the UI.

nonisolated public struct ApproveAgentActionIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "允许执行"
    public static let openAppWhenRun: Bool = false
    public static var authenticationPolicy: IntentAuthenticationPolicy { .requiresAuthentication }

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    @Parameter(title: "Expected Step Index", default: -1)
    public var expectedStepIndex: Int

    public init() {
        self.cascadeId = ""
        self.expectedStepIndex = -1
    }

    public init(cascadeId: String, expectedStepIndex: Int = -1) {
        self.cascadeId = cascadeId
        self.expectedStepIndex = expectedStepIndex
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty, let baseURL = AppSettings.shared.serverURL else {
            return .result()
        }
        let snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        if let interaction = snap?.pendingInteraction {
            // 1. 校验步骤序号，防止在解锁/等待期间审批内容发生切换导致盲批
            if expectedStepIndex >= 0 && interaction.stepIndex != expectedStepIndex {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap?.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap?.totalSteps ?? 1),
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
                        title: snap?.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap?.totalSteps ?? 1),
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
            try? await APIClient.shared.submitInteraction(
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
        } else if snap?.canProceed == true, let uri = snap?.proceedArtifactUri, !uri.isEmpty {
            try? await APIClient.shared.proceedArtifact(
                cascadeId: cascadeId,
                artifactUri: uri,
                model: snap?.activeModel,
                cascadeConfigRaw: snap?.cascadeConfigRaw,
                baseURL: baseURL
            )
        }
        if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
            ActivityManager.shared.updateActivity(
                cascadeId: cascadeId,
                title: snap?.title,
                status: "RUNNING",
                stepCount: max(1, snap?.totalSteps ?? 1),
                latestAction: String(localized: "已允许，继续执行中..."),
                hasPendingAction: false,
                pendingStepIndex: nil,
                pendingActionKind: nil
            )
        }
        #endif
        return .result()
    }
}

nonisolated public struct RejectAgentActionIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "拒绝操作"
    public static let openAppWhenRun: Bool = false

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    @Parameter(title: "Expected Step Index", default: -1)
    public var expectedStepIndex: Int

    public init() {
        self.cascadeId = ""
        self.expectedStepIndex = -1
    }

    public init(cascadeId: String, expectedStepIndex: Int = -1) {
        self.cascadeId = cascadeId
        self.expectedStepIndex = expectedStepIndex
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty, let baseURL = AppSettings.shared.serverURL else {
            return .result()
        }
        let snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        if let interaction = snap?.pendingInteraction {
            if expectedStepIndex >= 0 && interaction.stepIndex != expectedStepIndex {
                if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
                    ActivityManager.shared.updateActivity(
                        cascadeId: cascadeId,
                        title: snap?.title,
                        status: "WAITING_APPROVAL",
                        stepCount: max(1, snap?.totalSteps ?? 1),
                        latestAction: String(localized: "审批内容已更新，请重新确认"),
                        hasPendingAction: true,
                        pendingStepIndex: interaction.stepIndex,
                        pendingActionKind: interaction.type
                    )
                }
                return .result()
            }
            let denyOpt = interaction.options.first(where: { $0.isDeny == true || $0.id == "5" })
            try? await APIClient.shared.submitInteraction(
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
        if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
            ActivityManager.shared.updateActivity(
                cascadeId: cascadeId,
                title: snap?.title,
                status: "RUNNING",
                stepCount: max(1, snap?.totalSteps ?? 1),
                latestAction: String(localized: "已拒绝该操作"),
                hasPendingAction: false,
                pendingStepIndex: nil,
                pendingActionKind: nil
            )
        }
        #endif
        return .result()
    }
}

nonisolated public struct StopAgentRunIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "停止任务"
    public static let openAppWhenRun: Bool = false

    @Parameter(title: "Cascade ID")
    public var cascadeId: String

    public init() {
        self.cascadeId = ""
    }

    public init(cascadeId: String) {
        self.cascadeId = cascadeId
    }

    @MainActor
    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        guard !cascadeId.isEmpty, let baseURL = AppSettings.shared.serverURL else {
            return .result()
        }
        try? await APIClient.shared.cancelTask(cascadeId: cascadeId, killBackgroundTasks: true, baseURL: baseURL)
        if ActivityManager.shared.findActiveActivity(for: cascadeId) != nil {
            ActivityManager.shared.endActivity(for: cascadeId, finalStatus: "CANCELLED")
        }
        #endif
        return .result()
    }
}
