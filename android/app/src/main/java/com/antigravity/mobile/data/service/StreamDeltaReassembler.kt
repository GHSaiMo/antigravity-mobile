package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.data.model.StreamUpdatePayload

/**
 * Rebuilds complete stream payloads from the gateway's `delta=1` frames.
 *
 * A delta frame carries only new/changed messages plus [StreamUpdatePayload.messageIds], the full
 * ordered ID list of the current window. The baseline is whatever this connection last received,
 * so call [reset] on every (re)connect: the server always opens with a complete `init` frame.
 * Rebuilding before the payload is published keeps downstream consumers (and StateFlow
 * conflation) working on complete windows only.
 *
 * Not thread-safe; used from the single OkHttp WebSocket callback thread.
 */
class StreamDeltaReassembler {
    // The server's per-connection baseline starts empty, so does ours: its placeholder "init" for an
    // unreadable trajectory carries no messages and is followed by a delta against that empty baseline.
    private var baseline: List<GatewayMessageItem> = emptyList()

    fun reset() {
        baseline = emptyList()
    }

    /** Returns the complete payload, or null when the delta does not line up with the baseline (caller should resync). */
    fun apply(payload: StreamUpdatePayload): StreamUpdatePayload? {
        if (!payload.delta) {
            baseline = payload.messages ?: emptyList()
            return payload
        }

        val ids = payload.messageIds ?: return payload
        val prev = baseline
        val byId = HashMap<String, GatewayMessageItem>(prev.size + 4)
        prev.forEach { byId[it.id] = it }
        payload.messages?.forEach { byId[it.id] = it }

        val rebuilt = ArrayList<GatewayMessageItem>(ids.size)
        for (id in ids) {
            rebuilt.add(byId[id] ?: return null)
        }
        baseline = rebuilt
        return payload.copy(messages = rebuilt, delta = false, messageIds = null)
    }
}
