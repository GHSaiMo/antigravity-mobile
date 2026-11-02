import Foundation
import ActivityKit

nonisolated public struct AudioRecordingActivityAttributes: ActivityAttributes, Sendable {
    public struct ContentState: Codable, Hashable, Sendable {
        public var recordingStartDate: Date
        public var isPaused: Bool
        public var pausedDuration: TimeInterval
        public var conversationTitle: String
        public var cascadeId: String
        
        public init(
            recordingStartDate: Date = Date(),
            isPaused: Bool = false,
            pausedDuration: TimeInterval = 0,
            conversationTitle: String = "",
            cascadeId: String = ""
        ) {
            self.recordingStartDate = recordingStartDate
            self.isPaused = isPaused
            self.pausedDuration = pausedDuration
            self.conversationTitle = conversationTitle
            self.cascadeId = cascadeId
        }
    }
    
    public var cascadeId: String
    
    public init(cascadeId: String) {
        self.cascadeId = cascadeId
    }
}
