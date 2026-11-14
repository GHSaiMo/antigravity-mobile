import SwiftUI

/// 垂直纵向双钮跳跃导航条 (Prompt Jump Stepper)
/// 44×44pt 毛玻璃质感双圆钮堆叠，专用于在长篇会话中快速切轮跳转与直达最新。
public struct PromptJumpStepperView: View {
    public let currentTurn: Int
    public let totalTurns: Int
    public let isAtTop: Bool
    public let isAtLastTurn: Bool
    
    public let onJumpPrevious: () -> Void
    public let onJumpNextOrBottom: () -> Void
    public let onForceJumpBottom: () -> Void
    
    public init(
        currentTurn: Int,
        totalTurns: Int,
        isAtTop: Bool,
        isAtLastTurn: Bool,
        onJumpPrevious: @escaping () -> Void,
        onJumpNextOrBottom: @escaping () -> Void,
        onForceJumpBottom: @escaping () -> Void
    ) {
        self.currentTurn = currentTurn
        self.totalTurns = totalTurns
        self.isAtTop = isAtTop
        self.isAtLastTurn = isAtLastTurn
        self.onJumpPrevious = onJumpPrevious
        self.onJumpNextOrBottom = onJumpNextOrBottom
        self.onForceJumpBottom = onForceJumpBottom
    }
    
    public var body: some View {
        VStack(spacing: 8) {
            // 上方圆钮：上一轮提问
            Button(action: {
                guard !isAtTop else { return }
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                onJumpPrevious()
            }) {
                Image(systemName: "chevron.up")
                    .font(.system(size: 16, weight: .bold))
                    .foregroundColor(isAtTop ? Color.secondary.opacity(0.35) : Color.primary)
                    .frame(width: 44, height: 44)
                    .background(.ultraThinMaterial)
                    .clipShape(Circle())
                    .overlay(
                        Circle()
                            .stroke(Color.primary.opacity(0.08), lineWidth: 0.5)
                    )
                    .shadow(color: Color.black.opacity(0.14), radius: 5, x: 0, y: 2.5)
            }
            .buttonStyle(.plain)
            .disabled(isAtTop)
            
            // 下方圆钮：自适应 下一轮提问 / ⏬ 直达最新
            Button(action: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                onJumpNextOrBottom()
            }) {
                Image(systemName: isAtLastTurn ? "chevron.down.2" : "chevron.down")
                    .font(.system(size: isAtLastTurn ? 15 : 16, weight: .bold))
                    .foregroundColor(Color.primary)
                    .frame(width: 44, height: 44)
                    .background(.ultraThinMaterial)
                    .clipShape(Circle())
                    .overlay(
                        Circle()
                            .stroke(Color.primary.opacity(0.08), lineWidth: 0.5)
                    )
                    .shadow(color: Color.black.opacity(0.14), radius: 5, x: 0, y: 2.5)
            }
            .buttonStyle(.plain)
            .simultaneousGesture(
                LongPressGesture(minimumDuration: 0.45)
                    .onEnded { _ in
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        onForceJumpBottom()
                    }
            )
        }
        .accessibilityElement(children: .contain)
    }
}
