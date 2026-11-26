import Foundation
import Observation
import UIKit

public extension Notification.Name {
    /// Posted on the main actor right after `GatewaySwitcher` atomically commits a gateway switch.
    /// `object` is the newly activated `profileId` (`String`).
    static let didSwitchGatewayProfile = Notification.Name("antigravity.did_switch_gateway_profile")
}

/// Orchestrates atomic switching between decoupled gateway profiles while preventing stale
/// in-flight responses, cache bleeding, or active call/stream leaks across gateways.
@Observable
@MainActor
public final class GatewaySwitcher {
    public static let shared = GatewaySwitcher()

    public private(set) var switchBannerText: String? = nil
    private var bannerDismissTask: Task<Void, Never>? = nil

    private init() {}

    /// Atomically switches the active gateway to `profileId`.
    /// Returns `true` if the active profile changed (or was forced), `false` if already active or profile not found.
    @discardableResult
    public func switchTo(
        profileId: String,
        currentConversations: [ConversationItem]? = nil,
        showToast: Bool = true,
        force: Bool = false
    ) -> Bool {
        let store = GatewayProfileStore.shared
        guard let target = store.profile(for: profileId) else {
            return false
        }
        if !force && store.activeProfileId == target.id && AppSettings.shared.isPaired && !AppSettings.shared.isDemoMode {
            return false
        }

        // 1. Bump epoch FIRST so all in-flight responses / probes / 401s from the old gateway are discarded.
        GatewayRuntimeState.shared.bumpEpoch()

        // 2. Snapshot current active gateway state & flush pending disk writes before swapping namespaces.
        if let convs = currentConversations {
            store.updateActiveSummary(from: convs)
        }
        if store.activeProfileId != nil {
            store.updateActiveProfileFromSettings(AppSettings.shared)
        }
        CacheManager.shared.flushPendingSessionWrites()

        // 3. Tear down active transport / voice / endpoint probing state.
        if VoiceCallManager.shared.isInCall {
            VoiceCallManager.shared.endCall()
        }
        ConnectionManager.shared.resetForGatewaySwitch()

        // 4. Commit active profile & swap Keychain / E2EE / Cache namespaces atomically.
        store.setActiveProfileId(target.id)
        E2EECrypto.shared.resetForGatewaySwitch(requiredHosts: target.e2eeRequiredHosts)
        CacheManager.shared.switchStorageNamespace(target.storageNamespace)
        ProjectCacheManager.shared.switchStorageNamespace(target.storageNamespace)
        DocumentCacheManager.shared.switchStorageNamespace(target.storageNamespace)
        WeChatArchiveCacheManager.shared.switchStorageNamespace(target.storageNamespace)
        DraftFileStore.shared.switchStorageNamespace(target.storageNamespace)
        GatewayCompatStore.shared.update(GatewayCompat())

        // 5. Hydrate AppSettings with target profile endpoints & metadata (without emitting intermediate route storms).
        AppSettings.shared.applyProfile(target)
        AppSettings.shared.syncWidgetConfiguration(reloadWidget: true)

        // 6. Notify views/ViewModels to tear down old ChatViewModels, reload target cache, and reconnect streams.
        NotificationCenter.default.post(name: .didSwitchGatewayProfile, object: target.id)

        // 7. Ensure push token & multi-host label stay synced with the newly active gateway.
        PushNotificationManager.shared.syncAllTokensIfPaired(force: false)

        if showToast {
            let pos = store.activeProfileDisplayPosition
            presentSwitchToast(
                for: target.resolvedDisplayName,
                index: pos?.index,
                total: pos?.total
            )
        }
        return true
    }

    /// Cycles to the next (`step > 0`) or previous (`step < 0`) paired gateway.
    @discardableResult
    public func cycle(
        step: Int,
        currentConversations: [ConversationItem]? = nil,
        showToast: Bool = true
    ) -> Bool {
        guard let next = GatewayProfileStore.shared.adjacentProfile(step: step) else {
            return false
        }
        return switchTo(
            profileId: next.id,
            currentConversations: currentConversations,
            showToast: showToast
        )
    }

    public func presentSwitchToast(for gatewayName: String, index: Int? = nil, total: Int? = nil) {
        bannerDismissTask?.cancel()
        if let index, let total, total >= 3 {
            switchBannerText = String(localized: "已切换 · \(gatewayName) (\(index)/\(total))")
        } else {
            switchBannerText = String(localized: "已切换 · \(gatewayName)")
        }
        bannerDismissTask = Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            guard !Task.isCancelled else { return }
            self?.switchBannerText = nil
        }
    }
}
