package com.antigravity.mobile.data.service

/** Connection indicator state shown in the home quota bar. */
enum class LinkHealthState { UNKNOWN, CONNECTED, ATTENTION, DISCONNECTED }

/** Raw facts about the gateway events socket; [LinkHealth.evaluate] turns them into a state at a point in time. */
data class LinkSnapshot(
    val started: Boolean = false,
    val connected: Boolean = false,
    val hasEverConnected: Boolean = false,
    /** True once a `beat` (or a `hello` carrying `desktop`) arrived, i.e. the gateway supports heartbeats. */
    val supportsBeat: Boolean = false,
    val lastBeatAtMs: Long? = null,
    val startedAtMs: Long? = null,
    /** `ok` / `down` / `unknown` from the gateway, null before the first frame. */
    val desktop: String? = null,
    /** Bumped on every heartbeat so the dot can pulse. */
    val beatCount: Int = 0
)

object LinkHealth {
    /** Heartbeats arrive every 5s: 2 missed beats = unstable, 6 missed = disconnected. */
    const val ATTENTION_AFTER_MS = 12_000L
    const val DISCONNECTED_AFTER_MS = 30_000L
    /** How long a fresh (re)connect attempt shows as "reconnecting" before turning red. */
    const val CONNECTING_GRACE_MS = 10_000L

    fun evaluate(s: LinkSnapshot, nowMs: Long): LinkHealthState {
        if (!s.started) return LinkHealthState.UNKNOWN
        if (!s.connected) {
            val connecting = s.startedAtMs?.let { nowMs - it < CONNECTING_GRACE_MS } ?: false
            if (connecting) return if (s.hasEverConnected) LinkHealthState.ATTENTION else LinkHealthState.UNKNOWN
            return LinkHealthState.DISCONNECTED
        }
        // Old gateways never send `beat`: a live socket is all we can tell, so report plain "connected".
        if (!s.supportsBeat) return LinkHealthState.CONNECTED
        val last = s.lastBeatAtMs ?: return LinkHealthState.ATTENTION
        val age = nowMs - last
        return when {
            age >= DISCONNECTED_AFTER_MS -> LinkHealthState.DISCONNECTED
            age >= ATTENTION_AFTER_MS -> LinkHealthState.ATTENTION
            s.desktop == "down" -> LinkHealthState.ATTENTION
            else -> LinkHealthState.CONNECTED
        }
    }
}
