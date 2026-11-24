import SwiftUI

// Native desktop-parity muted working status row displayed while waiting for Agent response
public struct AgentThinkingBubbleView: View {
    public let focusText: String?
    public let startedAt: String?

    public init(focusText: String? = nil, startedAt: String? = nil) {
        self.focusText = focusText
        self.startedAt = startedAt
    }

    private var resolvedFocus: String {
        if let focus = focusText?.trimmingCharacters(in: .whitespacesAndNewlines), !focus.isEmpty {
            return focus
        }
        return "Working"
    }

    public var body: some View {
        ActiveFocusToolRowView(
            focusText: resolvedFocus,
            startedAt: startedAt,
            isExpandable: false
        )
        .padding(.horizontal, 16)
        .padding(.vertical, 2)
    }
}
