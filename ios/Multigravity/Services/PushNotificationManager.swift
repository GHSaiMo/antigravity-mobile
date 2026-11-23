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
    public var quotaLow: Bool
    public var quotaBonusReset: Bool
    public var quotaAnomaly: Bool
    public var stallMinutes: Int

    public static let totalCount = 6

    public static let defaultValue = GatewayAlertPreferences(
        taskCompletion: true,
        taskStall: true,
        cacheMiss: true,
        quotaLow: true,
        quotaBonusReset: true,
        quotaAnomaly: true,
        stallMinutes: 8
    )

    public var enabledCount: Int {
        [taskCompletion, taskStall, cacheMiss, quotaLow, quotaBonusReset, quotaAnomaly].filter { $0 }.count
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
        case quotaLow = "quota_low"
        case quotaBonusReset = "quota_bonus_reset"
        case quotaAnomaly = "quota_anomaly"
        case stallMinutes = "stall_minutes"
    }

    public init(
        taskCompletion: Bool,
        taskStall: Bool,
        cacheMiss: Bool,
        quotaLow: Bool,
        quotaBonusReset: Bool,
        quotaAnomaly: Bool,
        stallMinutes: Int = 8
    ) {
        self.taskCompletion = taskCompletion
        self.taskStall = taskStall
        self.cacheMiss = cacheMiss
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
        PushNotificationManager.shared.configureDelegate()
        ActivityManager.shared.startRemoteTokenObservers()
        return true
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
        case cascade(String)
        case quota
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
        if count == GatewayAlertPreferences.totalCount {
            return String(localized: "全部开启")
        }
        if count == 0 {
            return String(localized: "全部关闭")
        }
        return String(localized: "已开 \(count)/\(GatewayAlertPreferences.totalCount)")
    }

    /// Cascade currently visible on screen; foreground notifications for this exact cascade are suppressed
    /// unless a blocking modal sheet (e.g. Account Quota or Settings) is covering the screen.
    public var activeViewingCascadeId: String?
    public var hasBlockingModalPresented: Bool = false

    /// Buffers a tapped notification's destination during cold launch until ConversationListView mounts.
    private var pendingRoute: NotificationRoute?
    private var activeLiveActivityTokens: [String: String] = [:] // cascadeId -> tokenHex
    private var lastSyncedAlertSignature: String?
    private var lastSyncedStartSignature: String?
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

    public func configureDelegate() {
        UNUserNotificationCenter.current().delegate = self
        Task {
            await refreshAuthorizationStatus()
        }
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
    /// Does not trigger permission popups unexpectedly; only refreshes and syncs already-held tokens.
    public func syncAllTokensIfPaired(force: Bool = false) {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }

        Task {
            await refreshAuthorizationStatus()
            if let alertTok = deviceTokenHex {
                await uploadToken(
                    baseURL: baseURL,
                    tokenType: "alert",
                    token: alertTok,
                    cascadeId: nil,
                    force: force
                )
            }
            if let startTok = liveActivityStartTokenHex {
                await uploadToken(
                    baseURL: baseURL,
                    tokenType: "live_activity_start",
                    token: startTok,
                    cascadeId: nil,
                    force: force
                )
            }
            for (cid, actTok) in activeLiveActivityTokens {
                await uploadToken(
                    baseURL: baseURL,
                    tokenType: "live_activity_update",
                    token: actTok,
                    cascadeId: cid,
                    force: force
                )
            }
        }
    }

    public func syncPreferencesWithGateway() {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        Task {
            await sendPushTokenRequest(
                baseURL: baseURL,
                body: [
                    "platform": "ios",
                    "token_type": "preferences",
                    "notifications_enabled": settings.enablePushNotifications,
                    "live_activities_enabled": settings.enableLiveActivities,
                    "sound_complete": settings.notificationCompleteSound,
                    "sound_action": settings.notificationActionSound,
                    "device_id": settings.deviceID ?? ""
                ]
            )
        }
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
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        Task {
            await uploadToken(baseURL: baseURL, tokenType: "alert", token: clean, cascadeId: nil, force: false)
        }
    }

    public func updateLiveActivityStartToken(_ hex: String) {
        let clean = hex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        self.liveActivityStartTokenHex = clean

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        Task {
            await uploadToken(baseURL: baseURL, tokenType: "live_activity_start", token: clean, cascadeId: nil, force: false)
        }
    }

    public func updateLiveActivityToken(cascadeId: String, tokenHex: String) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        let tok = tokenHex.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty, !tok.isEmpty else { return }
        activeLiveActivityTokens[cid] = tok

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        Task {
            await uploadToken(baseURL: baseURL, tokenType: "live_activity_update", token: tok, cascadeId: cid, force: false)
        }
    }

    public func dropCachedLiveActivityToken(cascadeId: String) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        activeLiveActivityTokens.removeValue(forKey: cid)
        lastSyncedActivitySignatures.removeValue(forKey: cid)
    }

    public func clearLiveActivityToken(cascadeId: String) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        activeLiveActivityTokens.removeValue(forKey: cid)
        lastSyncedActivitySignatures.removeValue(forKey: cid)

        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return }
        let authToken = KeychainHelper.shared.read(key: .deviceToken) ?? settings.deviceToken
        Task {
            await sendDeletePushToken(baseURL: baseURL, tokenType: "live_activity_update", cascadeId: cid, authToken: authToken, deviceId: settings.deviceID)
        }
    }

    public func unregisterFromGateway(baseURL: URL?, authToken: String?, deviceId: String?) async {
        lastSyncedAlertSignature = nil
        lastSyncedStartSignature = nil
        lastSyncedActivitySignatures.removeAll()
        activeLiveActivityTokens.removeAll()
        guard let baseURL else { return }
        await sendDeletePushToken(baseURL: baseURL, tokenType: nil, cascadeId: nil, authToken: authToken, deviceId: deviceId)
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
        guard case .cascade(let cid) = pendingRoute else { return nil }
        pendingRoute = nil
        return cid
    }

    public func route(_ route: NotificationRoute) {
        pendingRoute = route
        NotificationCenter.default.post(name: .openNotificationRoute, object: route)
    }

    public func routeToCascade(_ cascadeId: String) {
        let cid = cascadeId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cid.isEmpty else { return }
        route(.cascade(cid))
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
                guard lower == "antigravity_action" || lower == "antigravity_proceed" else {
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

    private func uploadToken(
        baseURL: URL,
        tokenType: String,
        token: String,
        cascadeId: String?,
        force: Bool
    ) async {
        let settings = AppSettings.shared
        let signature = "\(baseURL.absoluteString)|\(tokenType)|\(token)|\(Self.apnsEnvironment)|\(settings.enablePushNotifications)|\(settings.enableLiveActivities)|\(settings.notificationCompleteSound)|\(settings.notificationActionSound)"

        if !force {
            switch tokenType {
            case "alert":
                if lastSyncedAlertSignature == signature { return }
            case "live_activity_start":
                if lastSyncedStartSignature == signature { return }
            case "live_activity_update":
                if let cid = cascadeId, lastSyncedActivitySignatures[cid] == signature { return }
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
            "live_activities_enabled": settings.enableLiveActivities
        ]
        if tokenType == "alert" {
            body["sound_complete"] = settings.notificationCompleteSound
            body["sound_action"] = settings.notificationActionSound
        }
        if let cid = cascadeId, !cid.isEmpty {
            body["cascade_id"] = cid
        }
        if let did = settings.deviceID, !did.isEmpty {
            body["device_id"] = did
        }

        let ok = await sendPushTokenRequest(baseURL: baseURL, body: body)
        if ok {
            switch tokenType {
            case "alert":
                lastSyncedAlertSignature = signature
            case "live_activity_start":
                lastSyncedStartSignature = signature
            case "live_activity_update":
                if let cid = cascadeId {
                    lastSyncedActivitySignatures[cid] = signature
                }
            default:
                break
            }
        }
    }

    @discardableResult
    private func sendPushTokenRequest(baseURL: URL, body: [String: Any]) async -> Bool {
        guard let data = try? JSONSerialization.data(withJSONObject: body) else { return false }
        let endpoint = baseURL.appendingPathComponent("api/v1/device/push-token")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 10.0
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let authTok = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !authTok.isEmpty {
            request.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
            request.setValue(authTok, forHTTPHeaderField: "x-device-token")
        }
        request.httpBody = data

        do {
            let (_, response) = try await NetworkTransport.shared.send(request: request)
            if let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) {
                return true
            }
        } catch {
            // Silent retry on next foreground / reconnect
        }
        return false
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

    nonisolated private static func extractNotificationRoute(
        from userInfo: [AnyHashable: Any],
        categoryIdentifier: String
    ) -> NotificationRoute? {
        if let cid = extractCascadeId(from: userInfo) {
            return .cascade(cid)
        }
        let rawCategory = !categoryIdentifier.isEmpty
            ? categoryIdentifier
            : ((userInfo["category"] as? String)
                ?? ((userInfo["aps"] as? [AnyHashable: Any])?["category"] as? String)
                ?? "")
        if rawCategory.lowercased() == "quota_alert" {
            return .quota
        }
        if let urlStr = userInfo["url"] as? String,
           let url = URL(string: urlStr.trimmingCharacters(in: .whitespacesAndNewlines)) {
            if url.host?.lowercased() == "quota" {
                return .quota
            }
            return .deepLink(url)
        }
        return nil
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

        DispatchQueue.main.async {
            let isViewingSame = (
                cid != nil
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
