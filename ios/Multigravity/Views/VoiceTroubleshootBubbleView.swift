import SwiftUI

/// 语音连接失败后的提示气泡：给出三条自查建议，并允许一键让智能体排查。
public struct VoiceTroubleshootBubbleView: View {
    let onRetry: () -> Void
    let onTroubleshoot: () -> Void

    public init(onRetry: @escaping () -> Void, onTroubleshoot: @escaping () -> Void) {
        self.onRetry = onRetry
        self.onTroubleshoot = onTroubleshoot
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundColor(.orange)
                    .font(.system(size: 17, weight: .semibold))
                Text("当前无法连接语音")
                    .font(.system(size: 16.5, weight: .bold))
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("请先在电脑上依次检查：")
                    .font(.system(size: 15, weight: .medium))
                    .foregroundColor(.secondary)

                VStack(alignment: .leading, spacing: 8) {
                    HStack(alignment: .top, spacing: 8) {
                        Text("1.")
                            .font(.system(size: 15, weight: .bold, design: .monospaced))
                            .foregroundColor(.orange)
                        Text("**重新登录**：退出 Antigravity 登录，完全关闭后重新打开并登录，授权页勾选全部权限")
                            .font(.system(size: 15))
                            .lineSpacing(3)
                    }
                    HStack(alignment: .top, spacing: 8) {
                        Text("2.")
                            .font(.system(size: 15, weight: .bold, design: .monospaced))
                            .foregroundColor(.orange)
                        Text("**开启代理 TUN 模式**：确保 `*.googleapis.com` 长连接（HTTP/2、gRPC）正常转发")
                            .font(.system(size: 15))
                            .lineSpacing(3)
                    }
                    HStack(alignment: .top, spacing: 8) {
                        Text("3.")
                            .font(.system(size: 15, weight: .bold, design: .monospaced))
                            .foregroundColor(.orange)
                        Text("**检查电脑麦克风**：系统麦克风权限已开启，默认录音设备正常且未被占用")
                            .font(.system(size: 15))
                            .lineSpacing(3)
                    }
                }
            }
            .fixedSize(horizontal: false, vertical: true)

            Text("如果都不行，是否让智能体自动排查？")
                .font(.system(size: 15, weight: .semibold))
                .padding(.top, 4)

            HStack(spacing: 12) {
                Spacer()
                Button(action: onRetry) {
                    Text("我再试试")
                        .font(.system(size: 14, weight: .medium))
                        .padding(.horizontal, 16)
                        .padding(.vertical, 8)
                        .background(Capsule().fill(Color.secondary.opacity(0.15)))
                }
                .buttonStyle(.plain)

                Button(action: onTroubleshoot) {
                    Text("一键排查")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.white)
                        .padding(.horizontal, 18)
                        .padding(.vertical, 8)
                        .background(Capsule().fill(Color.indigo))
                }
                .buttonStyle(.plain)
            }
        }
        .padding(16)
        .background(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .fill(.ultraThinMaterial)
                .overlay(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(Color.orange.opacity(0.4), lineWidth: 1)
                )
                .shadow(color: Color.black.opacity(0.08), radius: 8, x: 0, y: 3)
        )
        .padding(.horizontal, 12)
        .padding(.bottom, 6)
    }
}
