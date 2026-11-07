import Foundation

/// Represents an asynchronous background task currently running in Antigravity.
public struct RunningTaskItem: Identifiable, Sendable, Codable, Hashable {
    public let id: String
    public let stepIndex: Int
    public let toolName: String?
    public let commandLine: String
    public let toolSummary: String?
    public let toolAction: String?
    public let logUri: String?
    public let startedAt: String?
    /// Standing daemon (launched with `IsDaemon=true`, e.g. a dev server or recurring cron).
    /// Daemons stay RUNNING across turns, so they must not keep the conversation in a running state.
    /// Optional so payloads from older gateways (without the field) still decode.
    public let isDaemon: Bool?

    public init(
        id: String,
        stepIndex: Int,
        toolName: String? = nil,
        commandLine: String,
        toolSummary: String? = nil,
        toolAction: String? = nil,
        logUri: String? = nil,
        startedAt: String? = nil,
        isDaemon: Bool? = nil
    ) {
        self.id = id
        self.stepIndex = stepIndex
        self.toolName = toolName
        self.commandLine = commandLine
        self.toolSummary = toolSummary
        self.toolAction = toolAction
        self.logUri = logUri
        self.startedAt = startedAt
        self.isDaemon = isDaemon
    }

    /// Whether this task blocks the current turn (i.e. is not a standing daemon).
    public var blocksTurn: Bool { isDaemon != true }
}

public extension Array where Element == RunningTaskItem {
    /// Tasks that block the current turn; standing daemons are excluded.
    var blockingTasks: [RunningTaskItem] { filter(\.blocksTurn) }

    /// Whether any task blocks the current turn (daemon-only lists return false).
    var hasBlockingTasks: Bool { contains(where: \.blocksTurn) }
}
