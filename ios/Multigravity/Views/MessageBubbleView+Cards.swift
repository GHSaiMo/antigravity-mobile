import SwiftUI
import Photos

extension MessageBubbleView {
    @ViewBuilder
    func activeToolBatchCard(count: Int, tools: [String]) -> some View {
        let resolvedFocus: String = {
            if let focus = message.focusText?.trimmingCharacters(in: .whitespacesAndNewlines), !focus.isEmpty {
                return focus
            }
            return String(localized: "Working")
        }()
        let resolvedFallbackTools = !tools.isEmpty ? tools : (count > 0 ? Array(repeating: "tool_call", count: count) : [])
        ActiveFocusToolRowView(
            focusText: resolvedFocus,
            startedAt: message.startedAt,
            thinking: message.thinking,
            toolDetails: message.toolDetails,
            fallbackTools: resolvedFallbackTools,
            isExpandable: true
        )
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
            return String(localized: "error · 尝试 \(cur)/\(max)")
        } else if let cur = message.attemptCount, cur > 1 {
            return String(localized: "error · 重试 \(cur) 次")
        }
        let info = APIClient.parseAttemptError(message.content)
        if info.isAttempt {
            return String(localized: "error · 尝试 \(info.attempt)/\(info.maxAttempts)")
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
            return String(localized: "执行遇到错误 (已重试 \(count) 次)")
        }
        return String(localized: "执行遇到错误")
    }

    var rescueOverflowCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Circle()
                    .fill(Color.orange.opacity(0.18))
                    .frame(width: 28, height: 28)
                    .overlay(
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.orange)
                    )
                VStack(alignment: .leading, spacing: 2) {
                    Text("上下文窗口已触及上限")
                        .font(.system(size: 14, weight: .bold))
                        .foregroundColor(.orange)
                    Text("建议立即压缩历史对话或开启新会话继续工作")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                }
            }

            if !message.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Text(message.content)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(.secondary)
                    .lineLimit(3)
            }

            HStack(spacing: 8) {
                if let onCompact {
                    Button(action: onCompact) {
                        HStack(spacing: 5) {
                            CompactChevronsIcon(size: 7.5, weight: .bold)
                            Text("一键压缩 /compact")
                                .lineLimit(1)
                                .minimumScaleFactor(0.78)
                        }
                        .font(.system(size: 12, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 7)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.orange)
                }
                if let onStartNewSession {
                    Button(action: onStartNewSession) {
                        Text("开启新会话")
                            .font(.system(size: 12, weight: .semibold))
                            .lineLimit(1)
                            .minimumScaleFactor(0.78)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 7)
                    }
                    .buttonStyle(.bordered)
                }
            }
        }
        .padding(14)
        .background(Color.orange.opacity(0.09))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(Color.orange.opacity(0.35), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}
