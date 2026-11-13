import Foundation

/// One frame of the gateway's `/gateway/events` channel. Frames carry a revision, never list data.
struct ConversationEventFrame: Equatable {
    let type: String
    let rev: Int64
    /// Server unix milliseconds (`hello` / `beat` on gateways that support heartbeats).
    var ts: Int64? = nil
    /// Desktop language_server state reported by the gateway: `ok`, `down` or `unknown`.
    var desktop: String? = nil

    /// True for frames that prove the gateway is alive (every frame type does).
    var isHeartbeat: Bool { type == "hello" || type == "beat" || type == "changed" }

    /// `hello` (sent on every connect) and `changed` both mean "refetch the conversation list".
    var requestsRefresh: Bool { type == "hello" || type == "changed" }

    /// Returns nil for malformed JSON or frames without a `type`; unknown extra fields are ignored.
    static func parse(_ data: Data) -> ConversationEventFrame? {
        guard let obj = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
              let type = obj["type"] as? String else { return nil }
        let rev = (obj["rev"] as? NSNumber)?.int64Value ?? 0
        let ts = (obj["ts"] as? NSNumber)?.int64Value
        let desktop = obj["desktop"] as? String
        return ConversationEventFrame(type: type, rev: rev, ts: ts, desktop: desktop)
    }
}
