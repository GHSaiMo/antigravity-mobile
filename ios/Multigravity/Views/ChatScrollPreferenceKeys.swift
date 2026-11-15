import SwiftUI
import Observation

@Observable
@MainActor
final class ChatScrollHUDState {
    var stepperOpacity: Double = 0.0
    var currentVisibleTurnIndex: Int = 1
    var isNearBottom: Bool = true
    @ObservationIgnored var lastAnchorMaxY: CGFloat = 0
    @ObservationIgnored var lastStepperScheduleAt: Date = .distantPast
    @ObservationIgnored var stepperDimTask: Task<Void, Never>? = nil
    @ObservationIgnored var stepperHideTask: Task<Void, Never>? = nil
    @ObservationIgnored var lastVisibleMessageGeometries: [VisibleMessageGeometry] = []
    @ObservationIgnored var lockedTurnIndex: Int? = nil
}

struct ChatBottomAnchorOffsetPreferenceKey: PreferenceKey {
    static var defaultValue: CGFloat = .infinity
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = nextValue()
    }
}

struct ChatContentHeightPreferenceKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
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

