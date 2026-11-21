import SwiftUI
import Observation

@Observable
@MainActor
final class ChatScrollHUDState {
    var stepperOpacity: Double = 0.0
    var currentVisibleTurnIndex: Int = 1
    var isNearBottom: Bool = true
    @ObservationIgnored var messagesContentHeight: CGFloat = 0
    @ObservationIgnored var hasUserInteracted: Bool = false
    @ObservationIgnored var isUserDragging: Bool = false
    @ObservationIgnored var isScrollMoving: Bool = false
    @ObservationIgnored var lastRowMaterialized: Bool = false
    @ObservationIgnored var lastFollowScrollAt: Date = .distantPast
    @ObservationIgnored var lastKnownVisibleMessageId: String? = nil
    @ObservationIgnored var lastMidBlankHealAt: Date = .distantPast
    @ObservationIgnored var lastAnchorMaxY: CGFloat = 0
    @ObservationIgnored var lastQuantizedOffsetY: CGFloat = -1
    @ObservationIgnored var lastQuantizedDistanceFromBottom: CGFloat = 0
    @ObservationIgnored var wasScrollingDownward: Bool = false
    @ObservationIgnored var lastStepperScheduleAt: Date = .distantPast
    @ObservationIgnored var stepperDimTask: Task<Void, Never>? = nil
    @ObservationIgnored var stepperHideTask: Task<Void, Never>? = nil
    @ObservationIgnored var blankHealTask: Task<Void, Never>? = nil
    @ObservationIgnored var midScrollBlankHealTask: Task<Void, Never>? = nil
    @ObservationIgnored var turnJumpTask: Task<Void, Never>? = nil
    @ObservationIgnored var lastVisibleMessageGeometries: [VisibleMessageGeometry] = []
    @ObservationIgnored var lockedTurnIndex: Int? = nil

    func cancelScrollHealAndJumpTasks() {
        blankHealTask?.cancel()
        blankHealTask = nil
        midScrollBlankHealTask?.cancel()
        midScrollBlankHealTask = nil
        turnJumpTask?.cancel()
        turnJumpTask = nil
    }
}

struct ChatScrollMetrics: Equatable {
    let quantizedDistanceFromBottom: CGFloat
    let quantizedOffsetY: CGFloat
    let viewportHeight: CGFloat
    let contentHeight: CGFloat
    let overshoot: CGFloat
}

struct VisibleMessageGeometry: Equatable {
    let id: String
    let minY: CGFloat
    let maxY: CGFloat
}

struct ChatVisibleMessagesPreferenceKey: PreferenceKey {
    static var defaultValue: [VisibleMessageGeometry] = []
    static func reduce(value: inout [VisibleMessageGeometry], nextValue: () -> [VisibleMessageGeometry]) {
        value.append(contentsOf: nextValue())
    }
}

