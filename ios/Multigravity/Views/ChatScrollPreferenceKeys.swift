import SwiftUI

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

