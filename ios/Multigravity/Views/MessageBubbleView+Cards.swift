import SwiftUI
import Photos

extension MessageBubbleView {
    func activeToolBatchCard(count: Int, tools: [String]) -> some View {
        HStack(alignment: .top, spacing: 10) {
            AgentAvatarView()
            
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Text("Agent 正在思考与执行")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.primary)
                    
                    AgentActivityDotsView()
                }
                
                HStack(spacing: 6) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 10))
                        .foregroundColor(.orange)
                    
                    Text("已执行 \(count) 项操作")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.primary.opacity(0.85))
                    
                    if !tools.isEmpty {
                        let localizedPrefix = tools.prefix(3).map { ToolLocalization.localizedName(for: $0) }
                        Text("(\(localizedPrefix.joined(separator: ", "))\(tools.count > 3 ? "..." : ""))")
                            .font(.system(size: 10.5))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 20)
        }
    }
    
    var errorCard: some View {
        HStack(alignment: .top, spacing: 10) {
            Circle()
                .fill(Color.red.opacity(0.15))
                .frame(width: 28, height: 28)
                .overlay(
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.red)
                )
            
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Text(errorBadgeText)
                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                        .textCase(.uppercase)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.red.opacity(0.18))
                        .foregroundColor(.red)
                        .clipShape(Capsule())
                    
                    Text(errorTitleText)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.red)
                }
                
                Text(message.content)
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundColor(.primary.opacity(0.9))
                    .lineSpacing(3)
                    .textSelection(.enabled)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color.red.opacity(0.08))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color.red.opacity(0.25), lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 20)
        }
    }
    
    private var errorBadgeText: String {
        if let cur = message.attemptCount, let max = message.maxAttempts, max > 0 {
            return "error · 尝试 \(cur)/\(max)"
        } else if let cur = message.attemptCount, cur > 1 {
            return "error · 重试 \(cur) 次"
        }
        let info = APIClient.parseAttemptError(message.content)
        if info.isAttempt {
            return "error · 尝试 \(info.attempt)/\(info.maxAttempts)"
        }
        return "error"
    }

    private var errorTitleText: String {
        let count: Int = {
            if let c = message.attemptCount, c > 0 { return c }
            let info = APIClient.parseAttemptError(message.content)
            return info.isAttempt ? info.attempt : 1
        }()
        if count > 1 {
            return "执行遇到错误 (已重试 \(count) 次)"
        }
        return "执行遇到错误"
    }
}
