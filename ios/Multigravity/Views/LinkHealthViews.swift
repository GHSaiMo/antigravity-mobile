import SwiftUI

extension LinkHealthState {
    var dotColor: Color {
        switch self {
        case .unknown: return Color.gray
        case .connected: return Color.green
        case .attention: return Color.orange
        case .disconnected: return Color.red
        }
    }

    var title: String {
        switch self {
        case .unknown: return "连接中"
        case .connected: return "已连接电脑"
        case .attention: return "连接不稳或电脑端未就绪"
        case .disconnected: return "未连接"
        }
    }
}

/// Small heartbeat dot for the right edge of the home quota bar. Pulses on every gateway heartbeat.
struct LinkHealthDot: View {
    let monitor: ConversationEventsClient
    @State private var pulse = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let state = monitor.linkState(at: context.date)
            ZStack {
                Circle()
                    .fill(state.dotColor.opacity(0.28))
                    .frame(width: 14, height: 14)
                    .scaleEffect(pulse && state == .connected ? 1.0 : 0.55)
                    .opacity(pulse && state == .connected ? 0.0 : 1.0)
                Circle()
                    .fill(state.dotColor)
                    .frame(width: 7, height: 7)
            }
            .frame(width: 14, height: 14)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(state.accessibilityText)
        }
        .onChange(of: monitor.beatCount) { _, _ in
            pulse = false
            withAnimation(.easeOut(duration: 0.8)) { pulse = true }
        }
    }
}

/// Connection detail card shown at the top of the quota sheet.
struct LinkHealthDetailCard: View {
    let monitor: ConversationEventsClient

    private func ageText(_ date: Date?, now: Date) -> String {
        guard let date else { return "—" }
        let s = max(0, Int(now.timeIntervalSince(date)))
        return s < 2 ? "刚刚" : "\(s) 秒前"
    }

    private var desktopText: String {
        switch monitor.desktopState {
        case "ok": return "已就绪"
        case "down": return "未就绪(请确认电脑端 Antigravity 已打开)"
        default: return "未知"
        }
    }

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let state = monitor.linkState(at: context.date)
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Circle().fill(state.dotColor).frame(width: 9, height: 9)
                    Text(state.title)
                        .font(.system(size: 14, weight: .semibold))
                    Spacer()
                    Button {
                        monitor.reconnect()
                    } label: {
                        Label("立即重连", systemImage: "arrow.clockwise")
                            .font(.system(size: 12, weight: .medium))
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
                }
                Text("最近心跳 \(ageText(monitor.lastBeatAt, now: context.date)) · 电脑端 \(desktopText)")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(14)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14))
        }
    }
}
