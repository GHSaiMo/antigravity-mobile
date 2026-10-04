import SwiftUI

// Animated thinking & executing bubble displayed while waiting for Agent response
public struct AgentThinkingBubbleView: View {
    public init() {}
    
    public var body: some View {
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
