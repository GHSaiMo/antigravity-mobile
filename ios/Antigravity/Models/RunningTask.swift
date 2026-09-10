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

    public init(
        id: String,
        stepIndex: Int,
        toolName: String? = nil,
        commandLine: String,
        toolSummary: String? = nil,
        toolAction: String? = nil,
        logUri: String? = nil,
        startedAt: String? = nil
    ) {
        self.id = id
        self.stepIndex = stepIndex
        self.toolName = toolName
        self.commandLine = commandLine
        self.toolSummary = toolSummary
        self.toolAction = toolAction
        self.logUri = logUri
        self.startedAt = startedAt
    }
}
