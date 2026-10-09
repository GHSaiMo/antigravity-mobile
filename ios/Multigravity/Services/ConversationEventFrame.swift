import Foundation

/// One frame of the gateway's `/gateway/events` channel. Frames carry a revision, never list data.
struct ConversationEventFrame: Equatable {
    let type: String
    let rev: Int64

    /// `hello` (sent on every connect) and `changed` both mean "refetch the conversation list".
    var requestsRefresh: Bool { type == "hello" || type == "changed" }

    /// Returns nil for malformed JSON or frames without a `type`; unknown extra fields are ignored.
    static func parse(_ data: Data) -> ConversationEventFrame? {
        guard let obj = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
              let type = obj["type"] as? String else { return nil }
        let rev = (obj["rev"] as? NSNumber)?.int64Value ?? 0
        return ConversationEventFrame(type: type, rev: rev)
    }
}
