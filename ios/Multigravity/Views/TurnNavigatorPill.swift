import SwiftUI

/// 聊天顶部工具栏的微型轮次导轨胶囊（点击唤起大纲抽屉）
public struct TurnNavigatorPill: View {
    public let currentTurnIndex: Int
    public let totalTurns: Int
    public let isActive: Bool
    public let action: () -> Void
    
    public init(
        currentTurnIndex: Int,
        totalTurns: Int,
        isActive: Bool = false,
        action: @escaping () -> Void
    ) {
        self.currentTurnIndex = currentTurnIndex
        self.totalTurns = totalTurns
        self.isActive = isActive
        self.action = action
    }
    
    public var body: some View {
        Button(action: {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            action()
        }) {
            HStack(spacing: 4) {
                Image(systemName: "list.bullet.indent")
                    .font(.system(size: 10.5, weight: .semibold))
                
                Text("第 \(currentTurnIndex)/\(totalTurns) 轮")
                    .font(.system(size: 11.5, weight: .semibold))
                
                Image(systemName: "chevron.down")
                    .font(.system(size: 8.5, weight: .bold))
                    .foregroundColor(.secondary)
            }
            .foregroundColor(.indigo)
            .padding(.horizontal, 9)
            .padding(.vertical, 4.5)
            .background(Color.indigo.opacity(0.1))
            .clipShape(Capsule())
            .overlay(
                Capsule()
                    .stroke(Color.indigo.opacity(0.2), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
        .accessibilityLabel("当前第 \(currentTurnIndex) 轮，共 \(totalTurns) 轮，点击展开对话大纲")
    }
}
