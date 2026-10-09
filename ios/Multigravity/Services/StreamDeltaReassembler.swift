import Foundation

/// Rebuilds complete message windows from the gateway's `delta=1` stream frames.
///
/// A delta frame carries only new/changed messages plus `messageIds`, the full ordered ID list of
/// the current window. The baseline is whatever this connection last received, so call `reset()` on
/// every (re)connect: the server always opens with a complete `init` frame. The baseline starts
/// empty because the server's per-connection baseline does too (its placeholder `init` for an
/// unreadable trajectory carries no messages and is followed by a delta against that empty baseline).
struct StreamDeltaReassembler<Item> {
    enum Outcome {
        /// Use these messages for the payload (nil leaves the payload's messages untouched).
        case messages([Item]?)
        /// The delta does not line up with the baseline; drop the connection and reconnect.
        case resync
    }

    private let idOf: (Item) -> String
    private var baseline: [Item] = []

    init(idOf: @escaping (Item) -> String) {
        self.idOf = idOf
    }

    mutating func reset() {
        baseline = []
    }

    mutating func apply(isDelta: Bool, messageIds: [String]?, messages: [Item]?) -> Outcome {
        guard isDelta, let ids = messageIds else {
            baseline = messages ?? []
            return .messages(messages)
        }

        var byId: [String: Item] = [:]
        byId.reserveCapacity(baseline.count + (messages?.count ?? 0))
        for item in baseline { byId[idOf(item)] = item }
        for item in messages ?? [] { byId[idOf(item)] = item }

        var rebuilt: [Item] = []
        rebuilt.reserveCapacity(ids.count)
        for id in ids {
            guard let item = byId[id] else { return .resync }
            rebuilt.append(item)
        }
        baseline = rebuilt
        return .messages(rebuilt)
    }
}
