import Foundation

/// Connection indicator state shown in the home quota bar.
public enum LinkHealthState: Equatable, Sendable {
    /// Not started yet / first connect still pending.
    case unknown
    /// Gateway heartbeat is fresh and the desktop is reachable.
    case connected
    /// Gateway reachable but desktop not ready, heartbeat late, or reconnecting.
    case attention
    /// No socket or no heartbeat for too long.
    case disconnected

    /// Heartbeats arrive every 5s: 2 missed beats = unstable, 6 missed = disconnected.
    static let attentionAfter: TimeInterval = 12
    static let disconnectedAfter: TimeInterval = 30
    /// How long a fresh (re)connect attempt shows as "reconnecting" before turning red.
    static let connectingGrace: TimeInterval = 10

    static func evaluate(
        isStarted: Bool, isConnected: Bool, hasEverConnected: Bool, supportsBeat: Bool,
        lastBeatAt: Date?, startedAt: Date?, desktopState: String?, now: Date
    ) -> LinkHealthState {
        guard isStarted else { return .unknown }
        if !isConnected {
            let connecting = startedAt.map { now.timeIntervalSince($0) < connectingGrace } ?? false
            if connecting { return hasEverConnected ? .attention : .unknown }
            return .disconnected
        }
        // Old gateways never send `beat`: a live socket is all we can tell, so report plain "connected".
        guard supportsBeat else { return .connected }
        guard let last = lastBeatAt else { return .attention }
        let age = now.timeIntervalSince(last)
        if age >= disconnectedAfter { return .disconnected }
        if age >= attentionAfter { return .attention }
        if desktopState == "down" { return .attention }
        return .connected
    }

    var accessibilityText: String {
        switch self {
        case .unknown: return "正在连接电脑"
        case .connected: return "已连接电脑"
        case .attention: return "连接异常"
        case .disconnected: return "已断开"
        }
    }
}
