import SwiftUI

// Animated thinking & executing bubble displayed while waiting for Agent response
public struct AgentThinkingBubbleView: View {
    public let focusText: String?
    public let startedAt: String?

    public init(focusText: String? = nil, startedAt: String? = nil) {
        self.focusText = focusText
        self.startedAt = startedAt
    }
    
    public var body: some View {
        if let focus = focusText?.trimmingCharacters(in: .whitespacesAndNewlines), !focus.isEmpty {
            ActiveFocusToolRowView(
                focusText: focus,
                startedAt: startedAt,
                isExpandable: false
            )
            .padding(.horizontal, 16)
            .padding(.vertical, 2)
        } else {
            HStack(alignment: .top, spacing: 10) {
                AgentAvatarView()
                
                HStack(spacing: 8) {
                    Text("Agent 正在思考与执行")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.primary)
                    
                    AgentActivityDotsView()
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                
                Spacer(minLength: 40)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 2)
        }
    }
}
