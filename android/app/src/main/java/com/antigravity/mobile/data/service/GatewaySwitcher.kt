package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.GatewayCompat
import com.antigravity.mobile.data.model.GatewayCompatStore
import com.antigravity.mobile.data.model.GatewayRuntimeState
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class GatewaySwitchEvent(
    val profileId: String,
    val preserveNavigation: Boolean = false
)

/**
 * Orchestrates atomic switching between decoupled Android gateway profiles while preventing stale
 * in-flight responses, cache bleeding, or active socket leaks across gateways.
 */
object GatewaySwitcher {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private val _switchBannerText = MutableStateFlow<String?>(null)
    val switchBannerText: StateFlow<String?> = _switchBannerText.asStateFlow()

    private val _switchEvents = MutableSharedFlow<GatewaySwitchEvent>(extraBufferCapacity = 8)
    val switchEvents: SharedFlow<GatewaySwitchEvent> = _switchEvents.asSharedFlow()

    private var bannerJob: Job? = null

    fun switchTo(
        profileId: String,
        currentConversations: List<ConversationItem>? = null,
        showToast: Boolean = true,
        force: Boolean = false,
        preserveNavigation: Boolean = false
    ): Boolean {
        val store = GatewayProfileStore.sharedOrNull() ?: return false
        val target = store.profile(profileId) ?: return false
        val prefs = store.runtimePrefs()

        if (!force && store.activeProfileId == target.id && prefs.isPaired()) {
            return false
        }

        // 1. Bump epoch FIRST so all in-flight responses / probes / failovers from the old gateway are discarded.
        GatewayRuntimeState.bumpEpoch()

        // 2. Snapshot current active gateway state & flush pending disk writes before swapping namespaces.
        if (currentConversations != null) {
            store.updateActiveSummary(currentConversations)
        }
        if (store.activeProfileId != null) {
            store.updateActiveProfileFromPrefs(prefs)
        }
        store.runtimeCacheManager()?.flushPendingSessionWritesSync()

        // 3. Tear down active transport / WebSocket / endpoint probing state.
        store.runtimeWsClient()?.disconnect(intentional = true)
        store.runtimeConnectionManager()?.resetForGatewaySwitch()

        // 4. Commit active profile & swap E2EE / Cache / Prefs namespaces atomically.
        store.setActiveProfileId(target.id)
        E2eeCrypto.resetForGatewaySwitch(target.e2eeRequiredHosts)
        store.runtimeCacheManager()?.switchStorageNamespace(target.storageNamespace)
        store.runtimeDocumentCacheManager()?.switchStorageNamespace(target.storageNamespace)
        GatewayCompatStore.update(GatewayCompat())

        val devToken = prefs.readProfileDeviceToken(target.id)
        val devId = prefs.readProfileDeviceId(target.id)
        val e2eeKey = prefs.readProfileE2eeKeyB64(target.id)
        prefs.applyProfile(
            profile = target,
            profileDeviceToken = devToken,
            profileDeviceId = devId,
            profileE2eeKeyB64 = e2eeKey
        )

        // 5. Notify UI/ViewModels to tear down active chat state, reload target cache, and reconnect streams.
        _switchEvents.tryEmit(GatewaySwitchEvent(profileId = target.id, preserveNavigation = preserveNavigation))

        // 6. Keep FCM tokens & multi-gateway subtitles synchronized across all paired gateways.
        val fcmToken = prefs.fcmPushToken
        val client = store.runtimeApiClient()
        if (!fcmToken.isNullOrBlank() && client != null) {
            CoroutineScope(Dispatchers.IO).launch {
                client.syncPushTokenToAllGateways(fcmToken)
            }
        }

        if (showToast) {
            val pos = store.activeProfileDisplayPosition
            presentSwitchToast(
                gatewayName = target.resolvedDisplayName,
                index = pos?.first,
                total = pos?.second
            )
        }
        return true
    }

    /** Cycles to the next (`step > 0`) or previous (`step < 0`) paired gateway. */
    fun cycle(
        step: Int,
        currentConversations: List<ConversationItem>? = null,
        showToast: Boolean = true
    ): Boolean {
        val store = GatewayProfileStore.sharedOrNull() ?: return false
        val next = store.adjacentProfile(step) ?: return false
        return switchTo(
            profileId = next.id,
            currentConversations = currentConversations,
            showToast = showToast
        )
    }

    fun presentSwitchToast(gatewayName: String, index: Int? = null, total: Int? = null) {
        bannerJob?.cancel()
        _switchBannerText.value = if (index != null && total != null && total >= 3) {
            "已切换 · %s (%d/%d)".loc(gatewayName, index, total)
        } else {
            "已切换 · %s".loc(gatewayName)
        }
        bannerJob = scope.launch {
            delay(1500L)
            _switchBannerText.value = null
        }
    }
}
