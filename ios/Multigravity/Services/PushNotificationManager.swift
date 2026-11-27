import AudioToolbox
import AVFoundation
import Foundation
import Observation
import UIKit
import UserNotifications

public struct GatewayAlertPreferences: Codable, Equatable, Sendable {
    public var taskCompletion: Bool
    public var taskStall: Bool
    public var cacheMiss: Bool
    public var cacheMissMode: String
    public var quotaLow: Bool
    public var quotaBonusReset: Bool
    public var quotaAnomaly: Bool
    public var stallMinutes: Int

    public static let defaultValue = GatewayAlertPreferences(
        taskCompletion: true,
        taskStall: true,
        cacheMiss: true,
        cacheMissMode: "shadow",
        quotaLow: true,
        quotaBonusReset: true,
        quotaAnomaly: true,
        stallMinutes: 8
    )

    public var isCacheMissLive: Bool {
        cacheMissMode.lowercased() == "live"
    }

    public var visibleTotalCount: Int {
        isCacheMissLive ? 6 : 5
    }

    public var enabledCount: Int {
        var flags = [taskCompletion, taskStall, quotaLow, quotaBonusReset, quotaAnomaly]
        if isCacheMissLive {
            flags.append(cacheMiss)
        }
        return flags.filter { $0 }.count
    }

    public mutating func apply(key: String, value: Bool) {
        switch key {
        case "task_completion": taskCompletion = value
        case "task_stall": taskStall = value
        case "cache_miss": cacheMiss = value
        case "quota_low": quotaLow = value
        case "quota_bonus_reset": quotaBonusReset = value
        case "quota_anomaly": quotaAnomaly = value
        default: break
        }
    }

    enum CodingKeys: String, CodingKey {
        case taskCompletion = "task_completion"
        case taskStall = "task_stall"
        case cacheMiss = "cache_miss"
        case cacheMissMode = "cache_miss_mode"
        case quotaLow = "quota_low"
        case quotaBonusReset = "quota_bonus_reset"
        case quotaAnomaly = "quota_anomaly"
        case stallMinutes = "stall_minutes"
    }

    public init(
        taskCompletion: Bool,
        taskStall: Bool,
        cacheMiss: Bool,
        cacheMissMode: String = "shadow",
        quotaLow: Bool,
        quotaBonusReset: Bool,
        quotaAnomaly: Bool,
        stallMinutes: Int = 8
    ) {
        self.taskCompletion = taskCompletion
        self.taskStall = taskStall
        self.cacheMiss = cacheMiss
        self.cacheMissMode = cacheMissMode
        self.quotaLow = quotaLow
        self.quotaBonusReset = quotaBonusReset
        self.quotaAnomaly = quotaAnomaly
        self.stallMinutes = stallMinutes
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.taskCompletion = try c.decodeIfPresent(Bool.self, forKey: .taskCompletion) ?? true
        self.taskStall = try c.decodeIfPresent(Bool.self, forKey: .taskStall) ?? true
        self.cacheMiss = try c.decodeIfPresent(Bool.self, forKey: .cacheMiss) ?? true
        self.cacheMissMode = try c.decodeIfPresent(String.self, forKey: .cacheMissMode) ?? "shadow"
        self.quotaLow = try c.decodeIfPresent(Bool.self, forKey: .quotaLow) ?? true
        self.quotaBonusReset = try c.decodeIfPresent(Bool.self, forKey: .quotaBonusReset) ?? true
        self.quotaAnomaly = try c.decodeIfPresent(Bool.self, forKey: .quotaAnomaly) ?? true
        self.stallMinutes = try c.decodeIfPresent(Int.self, forKey: .stallMinutes) ?? 8
    }
}

/// UIApplicationDelegate bridge for receiving APNs device token callbacks in SwiftUI.
public final class MultigravityAppDelegate: NSObject, UIApplicationDelegate {
    public func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        ConnectionManager.shared.ensureStarted()
        PushNotificationManager.shared.configureDelegate()
        ActivityManager.shared.startRemoteTokenObservers()
        return true
    }

    public func application(
        _ application: UIApplication,
        configurationForConnecting connectingSceneSession: UISceneSession,
        options: UIScene.ConnectionOptions
    ) -> UISceneConfiguration {
        let config = UISceneConfiguration(name: nil, sessionRole: connectingSceneSession.role)
        config.delegateClass = MultigravitySceneDelegate.self
        return config
    }

    public func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        let hex = deviceToken.map { String(format: "%02x", $0) }.joined()
        Task { @MainActor in
            PushNotificationManager.shared.handleDidRegisterRemoteToken(hex)
        }
    }

    public func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        print("[APNs] Failed to register for remote notifications: \(error.localizedDescription)")
    }

    public func application(
        _ application: UIApplication,
        didReceiveRemoteNotification userInfo: [AnyHashable: Any],
        fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void
    ) {
        Task { @MainActor in
            let result = await PushNotificationManager.shared.handleSilentRemoteNotification(userInfo)
            completionHandler(result)
        }
    }
}

/// Manages native iOS APNs notification authorization, token synchronization with the paired gateway,
/// Live Activity push token syncing, and notification tap routing.
@Observable
@MainActor
public final class PushNotificationManager: NSObject {
    public static let shared = PushNotificationManager()

    public enum NotificationRoute: Equatable, Sendable {
        case cascade(cascadeId: String, gatewayId: String? = nil)
        case quota(gatewayId: String? = nil)
        case deepLink(URL)
    }

    public private(set) var authorizationStatus: UNAuthorizationStatus = .notDetermined
    public private(set) var deviceTokenHex: String?
    public private(set) var liveActivityStartTokenHex: String?
    public private(set) var alertPreferences: GatewayAlertPreferences = .defaultValue
    public private(set) var isLoadingAlertPreferences: Bool = false
    public private(set) var alertPreferencesSyncError: String? = nil

    public var alertPreferencesSummary: String {
        let count = alertPreferences.enabledCount
        let total = alertPreferences.visibleTotalCount
        if count == total {
            return String(localized: "全部开启")
        }
        if count == 0 {
            return String(localized: "全部关闭")
        }
        return String(localized: "已开 \(count)/\(total)")
    }

    /// Cascade currently visible on screen; foreground notifications for this exact cascade are suppressed
    /// unless a blocking modal sheet (e.g. Account Quota or Settings) is covering the screen.
    public var activeViewingCascadeId: String?
    public var hasBlockingModalPresented: Bool = false

    /// Buffers a tapped notification's destination during cold launch until ConversationListView mounts.
    private var pendingRoute: NotificationRoute?
    private var activeLiveActivityTokens: [String: (token: String, gatewayId: String?)] = [:] // cascadeId -> (tokenHex, gatewayId)
    private var lastSyncedAlertSignaturesByProfile: [String: String] = [:]
    private var lastSyncedStartSignaturesByProfile: [String: String] = [:]
    private var lastSyncedActivitySignatures: [String: String] = [:]
    private var previewPlayer: AVAudioPlayer?

    private static var apnsEnvironment: String {
        #if DEBUG
        return "sandbox"
        #else
        return "production"
        #endif
    }

    private override init() {
        super.init()
    }

    public static let categoryAction = "antigravity_action"
    public static let categoryActionHigh = "antigravity_action_high"
    public static let categoryProceed = "antigravity_proceed"
    public static let actionApproveId = "MULTIGRAVITY_APPROVE"
    public static let actionDenyId = "MULTIGRAVITY_DENY"
    public static let actionReviewId = "MULTIGRAVITY_REVIEW"

    public func configureDelegate() {
        UNUserNotificationCenter.current().delegate = self
        registerNotificationCategories()
        Task {
            await refreshAuthorizationStatus()
        }
    }

    public func registerNotificationCategories() {
        let approveAction = UNNotificationAction(
            identifier: Self.actionApproveId,
            title: String(localized: "允许"),
            options: [.authenticationRequired]
        )
        let denyAction = UNNotificationAction(
            identifier: Self.actionDenyId,
            title: String(localized: "拒绝"),
            options: [.destructive, .authenticationRequired]
        )
        let reviewAction = UNNotificationAction(
            identifier: Self.actionReviewId,
            title: String(localized: "打开核验"),
            options: [.foreground]
        )
        let proceedAction = UNNotificationAction(
            identifier: Self.actionApproveId,
            title: String(localized: "继续执行"),
            options: [.authenticationRequired]
        )
        let viewPlanAction = UNNotificationAction(
            identifier: Self.actionReviewId,
            title: String(localized: "查看方案"),
            options: [.foreground]
        )

        let standardApprovalCategory = UNNotificationCategory(
            identifier: Self.categoryAction,
            actions: [approveAction, denyAction],
            intentIdentifiers: [],
            options: []
        )
        // High-risk approvals intentionally omit a blind background Allow action on the lock screen.
        let highRiskApprovalCategory = UNNotificationCategory(
            identifier: Self.categoryActionHigh,
            actions: [reviewAction, denyAction],
            intentIdentifiers: [],
            options: []
        )
        let proceedCategory = UNNotificationCategory(
            identifier: Self.categoryProceed,
            actions: [proceedAction, viewPlanAction],
            intentIdentifiers: [],
            options: []
        )

        UNUserNotificationCenter.current().setNotificationCategories([
            standardApprovalCategory,
            highRiskApprovalCategory,
            proceedCategory,
        ])
    }

    public func refreshAuthorizationStatus() async {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        self.authorizationStatus = settings.authorizationStatus
        // If already authorized and paired (outside Demo Mode), ensure APNs token is fresh.
        if (settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional),
           AppSettings.shared.isPaired,
           !AppSettings.shared.isDemoMode {
            UIApplication.shared.registerForRemoteNotifications()
        }
    }

    /// Requests iOS notification authorization on explicit user action (or right after live pairing).
    /// Strictly no-ops in Demo Mode to keep App Review offline flow clean.
    public func requestAuthorizationAndRegister() async -> Bool {
        guard !AppSettings.shared.isDemoMode else { return false }
        do {
            let granted = try await UNUserNotificationCenter.current().requestAuthorization(
                options: [.alert, .sound, .badge]
            )
            await refreshAuthorizationStatus()
            if granted {
                AppSettings.shared.enablePushNotifications = true
                UIApplication.shared.registerForRemoteNotifications()
            }
            return granted
        } catch {
            print("[APNs] requestAuthorization failed: \(error.localizedDescription)")
            await refreshAuthorizationStatus()
            return false
        }
    }

    /// Called after pairing completes or when returning to foreground.
    /// Does not trigger permission popups unexpectedly; syncs tokens & multi-host labels to ALL paired gateways.
    public func syncAllTokensIfPaired(force: Bool = false) {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }

        Task {
            await refreshAuthorizationStatus()
            let contexts = GatewayProfileStore.shared.allRequestContexts()
            let showLabel = contexts.count > 1

            for ctx in contexts {
                if let alertTok = deviceTokenHex {
                    await uploadToken(
                        context: ctx,
                        tokenType: "alert",
                        token: alertTok,
                        cascadeId: nil,
                        showGatewayLabel: showLabel,
                        force: force
                    )
                }
                if let startTok = liveActivityStartTokenHex {
                    await uploadToken(
                        context: ctx,
                        tokenType: "live_activity_start",
                        token: startTok,
                        cascadeId: nil,
                        showGatewayLabel: showLabel,
                        force: force
                    )
                }
            }
            for (cid, entry) in activeLiveActivityTokens {
                let targetCtx = resolveContext(forGatewayId: entry.gatewayId) ?? GatewayProfileStore.shared.activeProfileId.flatMap { GatewayProfileStore.shared.requestContext(for: $0) }
                if let targetCtx {
                    await uploadToken(
                        context: targetCtx,
                        tokenType: "live_activity_update",
                        token: entry.token,
                        cascadeId: cid,
                        showGatewayLabel: showLabel,
                        force: force
                    )
                }
            }
        }
    }

    public func syncPreferencesWithGateway() {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }
        let contexts = GatewayProfileStore.shared.allRequestContexts()
        let showLabel = contexts.count > 1
        Task {
            for ctx in contexts {
                await sendPushTokenRequest(
                    context: ctx,
                    body: [
                        "platform": "ios",
                        "token_type": "preferences",
                        "notifications_enabled": settings.enablePushNotifications,
                        "live_activities_enabled": settings.enableLiveActivities,
                        "sound_complete": settings.notificationCompleteSound,
                        "sound_action": settings.notificationActionSound,
                        "lang": Self.currentLanguageCode,
                        "device_id": ctx.deviceID ?? "",
                        "show_gateway_label": showLabel,
                        "gateway_label": ctx.displayName
                    ]
                )
            }
        }
    }

    /// Syncs multi-gateway display label preferences for a single gateway profile (e.g. after rename or removing a sibling host).
    public func syncGatewayLabel(for context: GatewayRequestContext, showLabel: Bool, label: String) async {
        let settings = AppSettings.shared
        guard !settings.isDemoMode else { return }
        _ = await sendPushTokenRequest(
            context: context,
            body: [
                "platform": "ios",
                "token_type": "preferences",
                "notifications_enabled": settings.enablePushNotifications,
                "live_activities_enabled": settings.enableLiveActivities,
                "lang": Self.currentLanguageCode,
                "device_id": context.deviceID ?? "",
                "show_gateway_label": showLabel,
                "gateway_label": label
            ]
        )
    }

    /// Plays a short audio preview for the selected notification sound in Settings.
    public func previewNotificationSound(_ soundId: String) {
        let clean = soundId.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        previewPlayer?.stop()
        previewPlayer = nil

        if clean == "silence" || clean == "none" || clean.isEmpty {
            return
        }
        if clean == "default" {
            AudioServicesPlaySystemSound(1007)
            return
        }

        let fileURL = Bundle.main.url(forResource: clean, withExtension: "caf")
            ?? Bundle.main.url(forResource: clean, withExtension: "caf", subdirectory: "Sounds")
        guard let url = fileURL else {
            AudioServicesPlaySystemSound(1007)
            return
        }

        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.ambient, options: [.mixWithOthers])
            try session.setActive(true)
            let player = try AVAudioPlayer(contentsOf: url)
            player.prepareToPlay()
            player.play()
            self.previewPlayer = player
        } catch {
            print("[APNs] Failed to preview sound \(clean): \(error.localizedDescription)")
        }
    }

    public func handleDidRegisterRemoteToken(_ hex: String) {
        let clean = hex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        self.deviceTokenHex = clean

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }
        let contexts = GatewayProfileStore.shared.allRequestContexts()
        let showLabel = contexts.count > 1
        Task {
            for ctx in contexts {
                await uploadToken(context: ctx, tokenType: "alert", token: clean, cascadeId: nil, showGatewayLabel: showLabel, force: false)
            }
        }
    }

    public func updateLiveActivityStartToken(_ hex: String) {
        let clean = hex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        self.liveActivityStartTokenHex = clean

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }
        let contexts = GatewayProfileStore.shared.allRequestContexts()
        let showLabel = contexts.count > 1
        Task {
            for ctx in contexts {
                await uploadToken(context: ctx, tokenType: "live_activity_start", token: clean, cascadeId: nil, showGatewayLabel: showLabel, force: false)
            }
        }
    }

    public func updateLiveActivityToken(cascadeId: String, tokenHex: String, gatewayId: String? = nil) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        let tok = tokenHex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty, !tok.isEmpty else { return }
        activeLiveActivityTokens[cid] = (token: tok, gatewayId: gatewayId)

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }
        let targetCtx = resolveContext(forGatewayId: gatewayId)
            ?? GatewayProfileStore.shared.activeProfileId.flatMap { GatewayProfileStore.shared.requestContext(for: $0) }
        guard let targetCtx else { return }
        let showLabel = GatewayProfileStore.shared.hasMultipleGateways
        Task {
            await uploadToken(context: targetCtx, tokenType: "live_activity_update", token: tok, cascadeId: cid, showGatewayLabel: showLabel, force: false)
        }
    }

    public func dropCachedLiveActivityToken(cascadeId: String) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        activeLiveActivityTokens.removeValue(forKey: cid)
        lastSyncedActivitySignatures.removeValue(forKey: cid)
    }

    public func clearLiveActivityToken(cascadeId: String, gatewayId: String? = nil) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        let prevGw = gatewayId ?? activeLiveActivityTokens[cid]?.gatewayId
        activeLiveActivityTokens.removeValue(forKey: cid)
        lastSyncedActivitySignatures.removeValue(forKey: cid)

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return }
        let targetCtx = resolveContext(forGatewayId: prevGw)
            ?? GatewayProfileStore.shared.activeProfileId.flatMap { GatewayProfileStore.shared.requestContext(for: $0) }
        guard let targetCtx else { return }
        Task {
            await sendDeletePushToken(context: targetCtx, tokenType: "live_activity_update", cascadeId: cid)
        }
    }

    public func unregisterFromGateway(context: GatewayRequestContext) async {
        lastSyncedAlertSignaturesByProfile.removeValue(forKey: context.profileId)
        lastSyncedStartSignaturesByProfile.removeValue(forKey: context.profileId)
        await sendDeletePushToken(context: context, tokenType: nil, cascadeId: nil)
    }

    public func unregisterFromGateway(baseURL: URL?, authToken: String?, deviceId: String?) async {
        lastSyncedAlertSignaturesByProfile.removeAll()
        lastSyncedStartSignaturesByProfile.removeAll()
        lastSyncedActivitySignatures.removeAll()
        activeLiveActivityTokens.removeAll()
        guard let baseURL else { return }
        await sendDeletePushToken(baseURL: baseURL, tokenType: nil, cascadeId: nil, authToken: authToken, deviceId: deviceId)
    }

    private func resolveContext(forGatewayId rawGwId: String?, cascadeId: String? = nil) -> GatewayRequestContext? {
        let store = GatewayProfileStore.shared
        if let profile = store.profile(matchingGatewayId: rawGwId) {
            return store.requestContext(for: profile.id)
        }
        if let cid = cascadeId, !cid.isEmpty, let pid = store.profileId(containingCachedCascade: cid) {
            return store.requestContext(for: pid)
        }
        return nil
    }

    public func consumePendingRoute() -> NotificationRoute? {
        defer { pendingRoute = nil }
        return pendingRoute
    }

    public func clearPendingRouteIfMatching(_ route: NotificationRoute) {
        if pendingRoute == route {
            pendingRoute = nil
        }
    }

    public func consumePendingCascadeId() -> String? {
        guard case .cascade(let cid, _) = pendingRoute else { return nil }
        pendingRoute = nil
        return cid
    }

    public func route(_ route: NotificationRoute) {
        pendingRoute = route
        NotificationCenter.default.post(name: .openNotificationRoute, object: route)
    }

    public func routeToCascade(_ cascadeId: String, gatewayId: String? = nil) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        route(.cascade(cascadeId: cid, gatewayId: gatewayId))
    }

    /// Removes delivered notifications for a given cascade from iOS Notification Center.
    /// When `approvalOnly` is true, only stale approval/proceed banners are cleared.
    public func clearDeliveredNotifications(for cascadeId: String, approvalOnly: Bool = false) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        Task {
            await clearDeliveredNotificationsAsync(for: cid, approvalOnly: approvalOnly)
        }
    }

    @discardableResult
    public func clearDeliveredNotificationsAsync(for cascadeId: String, approvalOnly: Bool = false) async -> Int {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return 0 }
        let center = UNUserNotificationCenter.current()
        let delivered = await center.deliveredNotifications()
        let matchingIDs = delivered.compactMap { notif -> String? in
            let content = notif.request.content
            guard Self.extractCascadeId(from: content.userInfo) == cid else { return nil }
            if approvalOnly {
                let rawCat = !content.categoryIdentifier.isEmpty
                    ? content.categoryIdentifier
                    : ((content.userInfo["category"] as? String)
                        ?? ((content.userInfo["aps"] as? [AnyHashable: Any])?["category"] as? String)
                        ?? "")
                let lower = rawCat.lowercased()
                guard lower == Self.categoryAction
                    || lower == Self.categoryActionHigh
                    || lower == Self.categoryProceed else {
                    return nil
                }
            }
            return notif.request.identifier
        }
        if !matchingIDs.isEmpty {
            center.removeDeliveredNotifications(withIdentifiers: matchingIDs)
        }
        return matchingIDs.count
    }

    /// Handles APNs silent background pushes (`content-available: 1`), e.g. clearing stale approval
    /// banners after an approval was resolved on desktop or another device.
    public func handleSilentRemoteNotification(_ userInfo: [AnyHashable: Any]) async -> UIBackgroundFetchResult {
        let action = ((userInfo["action"] as? String)
            ?? (userInfo["category"] as? String)
            ?? "").trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard action == "dismiss_approval" else {
            return .noData
        }
        guard let cid = Self.extractCascadeId(from: userInfo), !cid.isEmpty else {
            return .noData
        }
        let removed = await clearDeliveredNotificationsAsync(for: cid, approvalOnly: true)
        return removed > 0 ? .newData : .noData
    }

    // MARK: - Networking Helpers

    private static var currentLanguageCode: String {
        DeviceTitleTargetLanguage.current().bcp47Tag
    }

    private func uploadToken(
        context: GatewayRequestContext,
        tokenType: String,
        token: String,
        cascadeId: String?,
        showGatewayLabel: Bool,
        force: Bool
    ) async {
        let settings = AppSettings.shared
        let langCode = Self.currentLanguageCode
        let signature = "\(context.profileId)|\(tokenType)|\(token)|\(Self.apnsEnvironment)|\(settings.enablePushNotifications)|\(settings.enableLiveActivities)|\(settings.notificationCompleteSound)|\(settings.notificationActionSound)|\(langCode)|\(showGatewayLabel)|\(context.displayName)"

        if !force {
            switch tokenType {
            case "alert":
                if lastSyncedAlertSignaturesByProfile[context.profileId] == signature { return }
            case "live_activity_start":
                if lastSyncedStartSignaturesByProfile[context.profileId] == signature { return }
            case "live_activity_update":
                if let cid = cascadeId, lastSyncedActivitySignatures["\(context.profileId):\(cid)"] == signature { return }
            default:
                break
            }
        }

        var body: [String: Any] = [
            "platform": "ios",
            "token_type": tokenType,
            "token": token,
            "apns_env": Self.apnsEnvironment,
            "notifications_enabled": settings.enablePushNotifications,
            "live_activities_enabled": settings.enableLiveActivities,
            "lang": langCode,
            "show_gateway_label": showGatewayLabel,
            "gateway_label": context.displayName
        ]
        if tokenType == "alert" {
            body["sound_complete"] = settings.notificationCompleteSound
            body["sound_action"] = settings.notificationActionSound
        }
        if let cid = cascadeId, !cid.isEmpty {
            body["cascade_id"] = cid
        }
        if let did = context.deviceID, !did.isEmpty {
            body["device_id"] = did
        }

        let ok = await sendPushTokenRequest(context: context, body: body)
        if ok {
            switch tokenType {
            case "alert":
                lastSyncedAlertSignaturesByProfile[context.profileId] = signature
            case "live_activity_start":
                lastSyncedStartSignaturesByProfile[context.profileId] = signature
            case "live_activity_update":
                if let cid = cascadeId {
                    lastSyncedActivitySignatures["\(context.profileId):\(cid)"] = signature
                }
            default:
                break
            }
        }
    }

    @discardableResult
    private func sendPushTokenRequest(context: GatewayRequestContext, body: [String: Any]) async -> Bool {
        guard let data = try? JSONSerialization.data(withJSONObject: body) else { return false }
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        for baseURL in context.candidateURLs(isCellular: isCell) {
            let endpoint = baseURL.appendingPathComponent("api/v1/device/push-token")
            var request = URLRequest(url: endpoint)
            request.httpMethod = "POST"
            request.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.5 : 10.0
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            if let authTok = context.deviceToken, !authTok.isEmpty {
                request.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
                request.setValue(authTok, forHTTPHeaderField: "x-device-token")
            }
            request.httpBody = data
            do {
                let (_, response) = try await NetworkTransport.shared.send(request: request, context: context)
                if let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) {
                    return true
                }
            } catch {
                continue
            }
        }
        return false
    }

    private func sendDeletePushToken(
        context: GatewayRequestContext,
        tokenType: String?,
        cascadeId: String?
    ) async {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        for baseURL in context.candidateURLs(isCellular: isCell) {
            var comps = URLComponents(url: baseURL.appendingPathComponent("api/v1/device/push-token"), resolvingAgainstBaseURL: false)
            var queryItems: [URLQueryItem] = []
            if let tokenType, !tokenType.isEmpty {
                queryItems.append(URLQueryItem(name: "token_type", value: tokenType))
            }
            if let cascadeId, !cascadeId.isEmpty {
                queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
            }
            if let did = context.deviceID, !did.isEmpty {
                queryItems.append(URLQueryItem(name: "device_id", value: did))
            }
            if !queryItems.isEmpty {
                comps?.queryItems = queryItems
            }
            guard let url = comps?.url else { continue }
            var request = URLRequest(url: url)
            request.httpMethod = "DELETE"
            request.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.0 : 8.0
            if let authTok = context.deviceToken, !authTok.isEmpty {
                request.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
                request.setValue(authTok, forHTTPHeaderField: "x-device-token")
            }
            if let (_, resp) = try? await NetworkTransport.shared.send(request: request, context: context),
               let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode) {
                return
            }
        }
    }

    // MARK: - Gateway Alert Category Preferences

    public func fetchAlertPreferences() async {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        isLoadingAlertPreferences = true
        alertPreferencesSyncError = nil
        defer { isLoadingAlertPreferences = false }

        var request = URLRequest(url: baseURL.appendingPathComponent("api/v1/push/preferences"))
        request.httpMethod = "GET"
        request.timeoutInterval = 8.0
        if let authTok = KeychainHelper.shared.read(key: .deviceToken) ?? settings.deviceToken, !authTok.isEmpty {
            request.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
            request.setValue(authTok, forHTTPHeaderField: "x-device-token")
        }

        do {
            let (data, response) = try await NetworkTransport.shared.send(request: request)
            if let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) {
                let decoded = try JSONDecoder().decode(GatewayAlertPreferences.self, from: data)
                self.alertPreferences = decoded
            }
        } catch {
            // Older gateways without /api/v1/push/preferences keep local defaults silently.
        }
    }

    public func updateAlertPreference(key: String, value: Bool) async {
        let previous = self.alertPreferences
        var optimistic = previous
        optimistic.apply(key: key, value: value)
        self.alertPreferences = optimistic
        self.alertPreferencesSyncError = nil

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }

        guard let data = try? JSONSerialization.data(withJSONObject: [key: value]) else { return }
        var request = URLRequest(url: baseURL.appendingPathComponent("api/v1/push/preferences"))
        request.httpMethod = "POST"
        request.timeoutInterval = 8.0
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let authTok = KeychainHelper.shared.read(key: .deviceToken) ?? settings.deviceToken, !authTok.isEmpty {
            request.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
            request.setValue(authTok, forHTTPHeaderField: "x-device-token")
        }
        request.httpBody = data

        do {
            let (respData, response) = try await NetworkTransport.shared.send(request: request)
            if let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) {
                if let decoded = try? JSONDecoder().decode(GatewayAlertPreferences.self, from: respData) {
                    self.alertPreferences = decoded
                }
            } else {
                self.alertPreferences = previous
                self.alertPreferencesSyncError = String(localized: "同步失败，请稍后重试")
            }
        } catch {
            self.alertPreferences = previous
            self.alertPreferencesSyncError = String(localized: "同步失败，请检查网关连接")
        }
    }

    private func sendDeletePushToken(
        baseURL: URL,
        tokenType: String?,
        cascadeId: String?,
        authToken: String?,
        deviceId: String?
    ) async {
        var comps = URLComponents(url: baseURL.appendingPathComponent("api/v1/device/push-token"), resolvingAgainstBaseURL: false)
        var queryItems: [URLQueryItem] = []
        if let tokenType, !tokenType.isEmpty {
            queryItems.append(URLQueryItem(name: "token_type", value: tokenType))
        }
        if let cascadeId, !cascadeId.isEmpty {
            queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
        }
        if let deviceId, !deviceId.isEmpty {
            queryItems.append(URLQueryItem(name: "device_id", value: deviceId))
        }
        if !queryItems.isEmpty {
            comps?.queryItems = queryItems
        }
        guard let url = comps?.url else { return }

        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.timeoutInterval = 8.0
        if let authToken, !authToken.isEmpty {
            request.setValue("Bearer \(authToken)", forHTTPHeaderField: "Authorization")
            request.setValue(authToken, forHTTPHeaderField: "x-device-token")
        }
        _ = try? await NetworkTransport.shared.send(request: request)
    }

    nonisolated private static func extractCascadeId(from userInfo: [AnyHashable: Any]) -> String? {
        if let cid = userInfo["cascadeId"] as? String, !cid.isEmpty {
            return cid
        }
        if let cid = userInfo["cascade_id"] as? String, !cid.isEmpty {
            return cid
        }
        if let urlStr = userInfo["url"] as? String,
           let url = URL(string: urlStr),
           url.host?.lowercased() == "cascade" {
            let trimmed = url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            if !trimmed.isEmpty { return trimmed }
        }
        return nil
    }

    nonisolated public static func extractGatewayId(from userInfo: [AnyHashable: Any]) -> String? {
        if let gw = (userInfo["gatewayId"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty {
            return gw
        }
        if let gw = (userInfo["gateway_id"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty {
            return gw
        }
        if let urlStr = userInfo["url"] as? String,
           let url = URL(string: urlStr.trimmingCharacters(in: .whitespacesAndNewlines)),
           let comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
           let gw = comps.queryItems?.first(where: { $0.name == "gw" || $0.name == "gatewayId" })?.value?.trimmingCharacters(in: .whitespacesAndNewlines),
           !gw.isEmpty {
            return gw
        }
        return nil
    }

    nonisolated private static func extractNotificationRoute(
        from userInfo: [AnyHashable: Any],
        categoryIdentifier: String
    ) -> NotificationRoute? {
        let gwId = extractGatewayId(from: userInfo)
        if let cid = extractCascadeId(from: userInfo) {
            return .cascade(cascadeId: cid, gatewayId: gwId)
        }
        let rawCategory = !categoryIdentifier.isEmpty
            ? categoryIdentifier
            : ((userInfo["category"] as? String)
                ?? ((userInfo["aps"] as? [AnyHashable: Any])?["category"] as? String)
                ?? "")
        if rawCategory.lowercased() == "quota_alert" {
            return .quota(gatewayId: gwId)
        }
        if let urlStr = userInfo["url"] as? String,
           let url = URL(string: urlStr.trimmingCharacters(in: .whitespacesAndNewlines)) {
            if url.host?.lowercased() == "quota" {
                return .quota(gatewayId: gwId)
            }
            return .deepLink(url)
        }
        return nil
    }
    @MainActor
    fileprivate func handleQuickAction(actionIdentifier: String, cascadeId: String, gatewayId: String? = nil) async {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        guard !AppSettings.shared.isDemoMode else {
            route(.cascade(cascadeId: cid, gatewayId: gatewayId))
            return
        }
        let targetCtx = resolveContext(forGatewayId: gatewayId, cascadeId: cid)
            ?? GatewayProfileStore.shared.activeProfileId.flatMap { GatewayProfileStore.shared.requestContext(for: $0) }
        guard let targetCtx else {
            route(.cascade(cascadeId: cid, gatewayId: gatewayId))
            return
        }

        let app = UIApplication.shared
        var bgTaskId: UIBackgroundTaskIdentifier = .invalid
        bgTaskId = app.beginBackgroundTask(withName: "multigravity.quickApproval") {
            if bgTaskId != .invalid {
                app.endBackgroundTask(bgTaskId)
                bgTaskId = .invalid
            }
        }
        defer {
            if bgTaskId != .invalid {
                app.endBackgroundTask(bgTaskId)
                bgTaskId = .invalid
            }
        }

        let api = APIClient()
        do {
            let snap = try await api.fetchMessages(cascadeId: cid, limit: 5, context: targetCtx)
            if actionIdentifier == Self.actionApproveId {
                if let interaction = snap.pendingInteraction {
                    let kind = interaction.type.lowercased()
                    let isHighRisk = interaction.approvalPreview?.riskLevel?.lowercased() == "high"
                    let requiresInAppReview = isHighRisk
                        || kind == "ask_question"
                        || kind == "workflow_approval"
                        || kind == "elicitation"
                        || !(interaction.questions ?? []).isEmpty
                    if requiresInAppReview {
                        await postLocalQuickActionNotice(
                            cascadeId: cid,
                            gatewayId: targetCtx.gatewayId ?? targetCtx.profileId,
                            title: isHighRisk ? String(localized: "🚨 高危操作审批") : String(localized: "⚠️ Antigravity 需要审批"),
                            body: isHighRisk ? String(localized: "高危操作，请打开 App 核验后再允许") : String(localized: "请打开 App 查看完整详情并确认"),
                            categoryIdentifier: isHighRisk ? Self.categoryActionHigh : Self.categoryAction
                        )
                        return
                    }
                    let approveOpt = interaction.options.first(where: {
                        $0.isDeny != true && $0.id != "5" && $0.id != "__write_in__"
                    }) ?? interaction.options.first
                    let optId = approveOpt?.id ?? interaction.defaultOptionId ?? "1"
                    let scopeVal = approveOpt?.scope ?? 1
                    try await api.submitInteraction(
                        cascadeId: cid,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: optId,
                        scope: scopeVal,
                        allow: true,
                        writeInResponse: "",
                        skipped: false,
                        target: interaction.target,
                        questionResponses: nil,
                        context: targetCtx
                    )
                    await clearDeliveredNotificationsAsync(for: cid, approvalOnly: true)
                } else if snap.canProceed {
                    let uri = (snap.proceedArtifactUri?.isEmpty == false) ? snap.proceedArtifactUri! : "implementation_plan.md"
                    try await api.proceedArtifact(
                        cascadeId: cid,
                        artifactUri: uri,
                        model: snap.activeModel,
                        context: targetCtx
                    )
                    await clearDeliveredNotificationsAsync(for: cid, approvalOnly: true)
                } else {
                    await clearDeliveredNotificationsAsync(for: cid, approvalOnly: true)
                }
            } else if actionIdentifier == Self.actionDenyId {
                if let interaction = snap.pendingInteraction {
                    let denyOpt = interaction.options.first(where: { $0.isDeny == true || $0.id == "5" })
                    try await api.submitInteraction(
                        cascadeId: cid,
                        trajectoryId: interaction.trajectoryId,
                        stepIndex: interaction.stepIndex,
                        type: interaction.type,
                        optionId: denyOpt?.id ?? "",
                        scope: denyOpt?.scope ?? 1,
                        allow: false,
                        writeInResponse: "",
                        skipped: denyOpt == nil,
                        target: interaction.target,
                        questionResponses: nil,
                        context: targetCtx
                    )
                }
                await clearDeliveredNotificationsAsync(for: cid, approvalOnly: true)
            }
        } catch {
            print("[APNs] Quick approval action \(actionIdentifier) failed for \(cid): \(error.localizedDescription)")
            await postLocalQuickActionNotice(
                cascadeId: cid,
                gatewayId: targetCtx.gatewayId ?? targetCtx.profileId,
                title: String(localized: "锁屏审批失败"),
                body: String(localized: "请打开 App 确认当前操作"),
                categoryIdentifier: Self.categoryActionHigh
            )
        }
    }

    private func postLocalQuickActionNotice(
        cascadeId: String,
        gatewayId: String? = nil,
        title: String,
        body: String,
        categoryIdentifier: String
    ) async {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        content.categoryIdentifier = categoryIdentifier
        var targetURL = "antigravity://cascade/\(cascadeId)?action=review"
        var info: [String: Any] = [
            "cascadeId": cascadeId,
            "category": categoryIdentifier,
        ]
        if let gw = gatewayId, !gw.isEmpty {
            targetURL += "&gw=\(gw)"
            info["gatewayId"] = gw
        }
        info["url"] = targetURL
        content.userInfo = info
        let req = UNNotificationRequest(
            identifier: "mgy.quick.feedback.\(cascadeId)",
            content: content,
            trigger: nil
        )
        try? await UNUserNotificationCenter.current().add(req)
    }
}

extension Notification.Name {
    public static let openNotificationRoute = Notification.Name("multigravity.openNotificationRoute")
    public static let dismissAllPresentedModals = Notification.Name("multigravity.dismissAllPresentedModals")
}

extension PushNotificationManager: UNUserNotificationCenterDelegate {
    nonisolated public func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        let userInfo = notification.request.content.userInfo
        let cid = Self.extractCascadeId(from: userInfo)
        let gwId = Self.extractGatewayId(from: userInfo)

        DispatchQueue.main.async {
            let activeProfile = GatewayProfileStore.shared.activeProfile
            let sameGateway: Bool
            if let gwId, !gwId.isEmpty {
                sameGateway = (activeProfile?.gatewayId == gwId || activeProfile?.id == gwId)
            } else {
                sameGateway = true
            }
            let isViewingSame = (
                sameGateway
                && cid != nil
                && cid == PushNotificationManager.shared.activeViewingCascadeId
                && !PushNotificationManager.shared.hasBlockingModalPresented
            )
            if isViewingSame {
                completionHandler([])
            } else {
                completionHandler([.banner, .list, .sound])
            }
        }
    }

    nonisolated public func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let content = response.notification.request.content
        let actionId = response.actionIdentifier
        let gwId = Self.extractGatewayId(from: content.userInfo)
        if (actionId == Self.actionApproveId || actionId == Self.actionDenyId),
           let cid = Self.extractCascadeId(from: content.userInfo) {
            DispatchQueue.main.async {
                Task { @MainActor in
                    await PushNotificationManager.shared.handleQuickAction(
                        actionIdentifier: actionId,
                        cascadeId: cid,
                        gatewayId: gwId
                    )
                    completionHandler()
                }
            }
            return
        }

        let route = Self.extractNotificationRoute(
            from: content.userInfo,
            categoryIdentifier: content.categoryIdentifier
        )

        DispatchQueue.main.async {
            if let route {
                PushNotificationManager.shared.route(route)
            }
            completionHandler()
        }
    }
}
