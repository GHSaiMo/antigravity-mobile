import AppIntents
import Foundation

/// Interactive Live Activity intents shared between `Multigravity` and `ActivityExtension`.
/// iOS executes `perform()` inside the host `Multigravity` app process without foregrounding the UI.

nonisolated public struct ApproveAgentActionIntent: LiveActivityIntent {
    public static let title: LocalizedStringResource = "允许执行"
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
        let snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        if let interaction = snap?.pendingInteraction {
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
                title: snap?.title,
                status: "RUNNING",
                stepCount: max(1, snap?.totalSteps ?? 1),
                latestAction: "已允许，继续执行中...",
                hasPendingAction: false
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
        let snap = try? await APIClient.shared.fetchMessages(cascadeId: cascadeId, limit: 5, baseURL: baseURL)
        if let interaction = snap?.pendingInteraction {
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
                title: snap?.title,
                status: "RUNNING",
                stepCount: max(1, snap?.totalSteps ?? 1),
                latestAction: "已拒绝该操作",
                hasPendingAction: false
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
            ActivityManager.shared.endActivity(finalStatus: "CANCELLED")
        }
        #endif
        return .result()
    }
}
