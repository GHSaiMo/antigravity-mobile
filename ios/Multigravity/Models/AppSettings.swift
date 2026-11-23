import Foundation
import Observation
import SwiftUI
import UIKit

public struct ServerEndpointItem: Identifiable, Equatable, Sendable {
    public let id: String
    public let type: String // "lan", "ddns", "primary", "custom"
    public let urlString: String
    
    public init(type: String, urlString: String) {
        self.id = "\(type):\(urlString)"
        self.type = type
        self.urlString = urlString
    }
}

public struct NotificationSoundOption: Identifiable, Hashable, Sendable {
    public let id: String
    public let displayName: String
    public let subtitle: String

    public static let completeOptions: [NotificationSoundOption] = [
        NotificationSoundOption(id: "glass", displayName: String(localized: "玻璃清脆"), subtitle: String(localized: "默认 · 清脆叮叮声")),
        NotificationSoundOption(id: "chime", displayName: String(localized: "清脆风铃"), subtitle: String(localized: "柔和双音风铃")),
        NotificationSoundOption(id: "bell", displayName: String(localized: "经典铃铛"), subtitle: String(localized: "明亮单音提醒")),
        NotificationSoundOption(id: "paymentsuccess", displayName: String(localized: "清脆双响"), subtitle: String(localized: "轻快确认音效")),
        NotificationSoundOption(id: "fanfare", displayName: String(localized: "欢快号角"), subtitle: String(localized: "阶段性里程碑")),
        NotificationSoundOption(id: "default", displayName: String(localized: "系统默认"), subtitle: String(localized: "跟随 iOS 系统提示音")),
        NotificationSoundOption(id: "silence", displayName: String(localized: "静音"), subtitle: String(localized: "仅横幅提醒不发声")),
    ]

    public static let actionOptions: [NotificationSoundOption] = [
        NotificationSoundOption(id: "alarm", displayName: String(localized: "急促滴嘟"), subtitle: String(localized: "默认 · 强提醒警报")),
        NotificationSoundOption(id: "electronic", displayName: String(localized: "电子警示"), subtitle: String(localized: "科技感高频双音")),
        NotificationSoundOption(id: "anticipate", displayName: String(localized: "温和期待"), subtitle: String(localized: "不刺耳的渐进提示")),
        NotificationSoundOption(id: "horn", displayName: String(localized: "号角提醒"), subtitle: String(localized: "穿透力强")),
        NotificationSoundOption(id: "default", displayName: String(localized: "系统默认"), subtitle: String(localized: "跟随 iOS 系统提示音")),
        NotificationSoundOption(id: "silence", displayName: String(localized: "静音"), subtitle: String(localized: "仅横幅提醒不发声")),
    ]

    public static func displayName(for id: String, in options: [NotificationSoundOption]) -> String {
        options.first(where: { $0.id == id })?.displayName ?? id
    }
}

extension Notification.Name {
    public static let networkRoutingPreferenceChanged = Notification.Name("antigravity.network_routing_preference_changed")
}

@Observable
public final class AppSettings {
    public static let shared = AppSettings()
    
    private let serverURLKey = "antigravity.server_url"
    private let primaryCloudURLKey = "antigravity.primary_cloud_url"
    private let lanServerURLKey = "antigravity.lan_server_url"
    private let relayServerURLKey = "antigravity.relay_server_url"
    private let customServerURLKey = "antigravity.custom_server_url"
    private let activeServerURLKey = "antigravity.active_server_url"
    private let enableLiveActivityKey = "antigravity.enable_live_activity"
    private let enablePushNotificationsKey = "antigravity.enable_push_notifications"
    private let notificationCompleteSoundKey = "antigravity.notification_complete_sound"
    private let notificationActionSoundKey = "antigravity.notification_action_sound"
    private let activeModelKey = "antigravity.active_model"
    private let favoriteModel1Key = "antigravity.favorite_model_1"
    private let favoriteModel2Key = "antigravity.favorite_model_2"
    private let defaultGeminiModelKey = "antigravity.default_gemini_model"
    private let defaultClaudeModelKey = "antigravity.default_claude_model"
    private let defaultOrchestratorModelKey = "antigravity.default_orchestrator_model"
    private let defaultNewSessionProviderKey = "antigravity.default_new_session_provider"
    private let enableVoiceModeKey = "antigravity.enable_voice_mode"
    private let weeklyPaceCursorKey = "antigravity.weekly_pace_cursor"
    private let selectedQuotaBucketKey = "antigravity.selected_quota_bucket"
    private let themeModeKey = "antigravity.theme_mode"
    private let shareCardSkinKey = "antigravity.share_card_skin_id"
    private let tokenHistoryChartKey = "antigravity.token_history_chart_type"
    private let fontSizeScaleKey = "antigravity.font_size_scale"
    private let knowledgeModeKey = "antigravity.knowledge_mode_enabled"
    private let compactCollapseToolsKey = "antigravity.compact_collapse_tools"
    private let compactCollapseTasksKey = "antigravity.compact_collapse_tasks"
    private let compactHideHealthyContextKey = "antigravity.compact_hide_healthy_context"
    private let gatewayPlatformKey = "antigravity.gateway_platform"
    private let demoModeKey = "antigravity.demo_mode"
    private let workspaceNotesKey = "antigravity.workspace_notes"
    private let cachedPermissionPresetKey = "antigravity.cached_permission_preset"

    public var workspaceNotes: [String: String] {
        didSet { UserDefaults.standard.set(workspaceNotes, forKey: workspaceNotesKey) }
    }

    public func workspaceNote(for pathOrUri: String) -> String {
        workspaceNotes[pathOrUri] ?? ""
    }

    public func setWorkspaceNote(_ note: String, for pathOrUri: String) {
        let trimmed = note.trimmingCharacters(in: .whitespacesAndNewlines)
        var next = workspaceNotes
        if trimmed.isEmpty {
            next.removeValue(forKey: pathOrUri)
        } else {
            next[pathOrUri] = trimmed
        }
        workspaceNotes = next
    }

    public enum FontSizeScale: String, CaseIterable, Identifiable, Sendable {
        case compact = "compact"
        case standard = "standard"
        case comfortable = "comfortable"

        public var id: String { rawValue }

        public var displayName: String {
            switch self {
            case .compact: return String(localized: "紧凑 (90%)")
            case .standard: return String(localized: "标准 (100%)")
            case .comfortable: return String(localized: "舒适 (115%)")
            }
        }

        public var bodyScale: CGFloat {
            switch self {
            case .compact: return 0.90
            case .standard: return 1.00
            case .comfortable: return 1.15
            }
        }

        public var codeScale: CGFloat {
            switch self {
            case .compact: return 0.88
            case .standard: return 1.00
            case .comfortable: return 1.12
            }
        }
    }
    
    public var gatewayPlatform: String? {
        didSet {
            if let val = gatewayPlatform?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
                let normalized: String
                switch val {
                case "darwin", "macos", "mac": normalized = "darwin"
                case "windows", "win": normalized = "windows"
                case "linux": normalized = "linux"
                default: normalized = val
                }
                UserDefaults.standard.set(normalized, forKey: gatewayPlatformKey)
            } else {
                UserDefaults.standard.removeObject(forKey: gatewayPlatformKey)
            }
        }
    }

    public var gatewayPlatformDisplayName: String {
        switch gatewayPlatform?.lowercased() {
        case "windows", "win":
            return String(localized: "Windows 网关")
        case "darwin", "macos", "mac":
            return String(localized: "Mac 网关")
        case "linux":
            return String(localized: "Linux 网关")
        default:
            return String(localized: "电脑网关")
        }
    }
    
    public var rawServerURL: String {
        didSet {
            UserDefaults.standard.set(rawServerURL, forKey: serverURLKey)
            if activeServerURL != rawServerURL {
                activeServerURL = rawServerURL
            }
        }
    }
    
    public var primaryCloudURL: String? {
        didSet {
            if let val = primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines), !val.isEmpty {
                if !NetworkTransport.isLocalOrPrivateHost(val) {
                    UserDefaults.standard.set(val, forKey: primaryCloudURLKey)
                } else {
                    UserDefaults.standard.removeObject(forKey: primaryCloudURLKey)
                }
            } else {
                UserDefaults.standard.removeObject(forKey: primaryCloudURLKey)
            }
        }
    }
    
    public var lanServerURL: String? {
        didSet {
            if let val = lanServerURL {
                UserDefaults.standard.set(val, forKey: lanServerURLKey)
            } else {
                UserDefaults.standard.removeObject(forKey: lanServerURLKey)
            }
        }
    }

    public var relayServerURL: String? {
        didSet {
            if let val = relayServerURL {
                UserDefaults.standard.set(val, forKey: relayServerURLKey)
            } else {
                UserDefaults.standard.removeObject(forKey: relayServerURLKey)
            }
        }
    }
    
    public var customServerURL: String? {
        didSet {
            if let val = customServerURL {
                UserDefaults.standard.set(val, forKey: customServerURLKey)
            } else {
                UserDefaults.standard.removeObject(forKey: customServerURLKey)
            }
        }
    }
    
    public var activeServerURL: String? {
        didSet {
            if let val = activeServerURL {
                UserDefaults.standard.set(val, forKey: activeServerURLKey)
            } else {
                UserDefaults.standard.removeObject(forKey: activeServerURLKey)
            }
            if activeServerURL != oldValue {
                DispatchQueue.main.async {
                    NotificationCenter.default.post(name: .networkRoutingPreferenceChanged, object: nil)
                }
            }
        }
    }
    
    public var enableLiveActivities: Bool {
        didSet {
            UserDefaults.standard.set(enableLiveActivities, forKey: enableLiveActivityKey)
        }
    }

    public var enablePushNotifications: Bool {
        didSet {
            UserDefaults.standard.set(enablePushNotifications, forKey: enablePushNotificationsKey)
        }
    }

    public var notificationCompleteSound: String {
        didSet {
            UserDefaults.standard.set(notificationCompleteSound, forKey: notificationCompleteSoundKey)
        }
    }

    public var notificationActionSound: String {
        didSet {
            UserDefaults.standard.set(notificationActionSound, forKey: notificationActionSoundKey)
        }
    }
    
    public var activeModel: String {
        didSet {
            UserDefaults.standard.set(activeModel, forKey: activeModelKey)
        }
    }
    
    /// 是否在输入框右侧显示「实时语音对话」入口按钮（默认关闭，关闭时保持传统的 ↑ 发送按钮）
    public var enableVoiceMode: Bool {
        didSet {
            UserDefaults.standard.set(enableVoiceMode, forKey: enableVoiceModeKey)
        }
    }

    /// 是否在 Weekly 额度条上显示时间匀速流逝刻度游标（默认开启）
    public var weeklyPaceCursorEnabled: Bool {
        didSet {
            UserDefaults.standard.set(weeklyPaceCursorEnabled, forKey: weeklyPaceCursorKey)
            syncWidgetConfiguration()
        }
    }

    /// 首页额度条默认对齐显示的配额框（默认 Gemini · 5h）
    public var selectedQuotaBucket: QuotaBucketKind {
        didSet {
            UserDefaults.standard.set(selectedQuotaBucket.rawValue, forKey: selectedQuotaBucketKey)
            syncWidgetConfiguration()
        }
    }

    /// 外观主题模式："system"（跟随系统）、"light"（浅色）、"dark"（深色）
    public var themeMode: String {
        didSet {
            if themeMode == "nord" {
                themeMode = "dark"
                return
            }
            UserDefaults.standard.set(themeMode, forKey: themeModeKey)
            applyAppearance()
        }
    }

    /// 分享长图卡片（对话分享 & Token 历史分享共用）皮肤 ID；"auto" 表示跟随当前系统明暗
    public var shareCardSkinID: String {
        didSet {
            UserDefaults.standard.set(shareCardSkinID, forKey: shareCardSkinKey)
        }
    }

    /// Token 历史页用户手动选择过的图表类型（空字符串表示自动按活跃天数决定）
    public var tokenHistoryChartType: String {
        didSet {
            UserDefaults.standard.set(tokenHistoryChartType, forKey: tokenHistoryChartKey)
        }
    }

    /// 字体缩放比例：紧凑、标准、舒适
    public var fontSizeScale: FontSizeScale {
        didSet {
            UserDefaults.standard.set(fontSizeScale.rawValue, forKey: fontSizeScaleKey)
        }
    }

    /// 精简模式主开关（开启后可按需微调对话内降噪子选项）
    public var isKnowledgeModeEnabled: Bool {
        didSet {
            UserDefaults.standard.set(isKnowledgeModeEnabled, forKey: knowledgeModeKey)
        }
    }

    /// 精简模式子选项：折叠工具执行过程（默认开启）
    public var compactCollapseTools: Bool {
        didSet {
            UserDefaults.standard.set(compactCollapseTools, forKey: compactCollapseToolsKey)
        }
    }

    /// 精简模式子选项：收起底部运行任务辅助栏（默认开启）
    public var compactCollapseTasks: Bool {
        didSet {
            UserDefaults.standard.set(compactCollapseTasks, forKey: compactCollapseTasksKey)
        }
    }

    /// 精简模式子选项：隐藏常规实时上下文胶囊（默认关闭；仅 < 70% 健康区隐藏，≥ 70% 或溢出自动穿透预警）
    public var compactHideHealthyContext: Bool {
        didSet {
            UserDefaults.standard.set(compactHideHealthyContext, forKey: compactHideHealthyContextKey)
        }
    }

    /// 电脑端同步缓存的 Agent 权限预设模式（"strict" / "sandbox" / "auto"），避免每次打开设置跳变刷新
    public var cachedPermissionPreset: String {
        didSet {
            UserDefaults.standard.set(cachedPermissionPreset, forKey: cachedPermissionPresetKey)
        }
    }

    public var shouldCollapseTools: Bool {
        isKnowledgeModeEnabled && compactCollapseTools
    }

    public var shouldCollapseRunningTasks: Bool {
        isKnowledgeModeEnabled && compactCollapseTasks
    }

    public func shouldShowContextMeter(
        _ meter: ContextMeterItem?,
        isRunning: Bool = false,
        hasPendingInteraction: Bool = false
    ) -> Bool {
        guard let meter else { return false }
        guard isKnowledgeModeEnabled && compactHideHealthyContext else { return true }
        return meter.isOverflow == true
            || meter.percentage >= 70
            || meter.hasCacheWarning(isRunning: isRunning, hasPendingInteraction: hasPendingInteraction)
    }

    public func resolvedShareCardSkin(for systemScheme: ColorScheme) -> ThemePreset {
        let fallbackDark: Bool
        switch themeMode {
        case "light": fallbackDark = false
        case "dark": fallbackDark = true
        default: fallbackDark = (systemScheme == .dark)
        }
        if shareCardSkinID == "auto" || shareCardSkinID.isEmpty {
            return fallbackDark ? ThemePresetCatalog.defaultDark : ThemePresetCatalog.defaultLight
        }
        return ThemePresetCatalog.findByThemeKey(shareCardSkinID, fallbackIsDark: fallbackDark)
    }

    public var isNordTheme: Bool { false }

    /// 供需要显式读取目标 ColorScheme 的组件使用。
    /// 注意：不要在根视图上挂载 `.preferredColorScheme(settings.colorScheme)`，
    /// SwiftUI 在 `.dark/.light -> nil` 切换时不会重置已弹出 `.sheet` 的外观，
    /// 全应用外观统一由 `applyAppearance()` 通过 `UIWindow.overrideUserInterfaceStyle` 驱动。
    public var colorScheme: ColorScheme? {
        switch themeMode {
        case "light": return .light
        case "dark": return .dark
        default: return nil
        }
    }

    public var userInterfaceStyle: UIUserInterfaceStyle {
        switch themeMode {
        case "light": return .light
        case "dark": return .dark
        default: return .unspecified
        }
    }

    public func applyAppearance() {
        let update = {
            let style = self.userInterfaceStyle
            let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            for scene in scenes {
                for window in scene.windows {
                    window.overrideUserInterfaceStyle = style
                    if let rootVC = window.rootViewController {
                        Self.clearViewControllerAppearanceOverrides(from: rootVC)
                    }
                }
            }
        }
        if Thread.isMainThread {
            update()
        } else {
            DispatchQueue.main.async(execute: update)
        }
    }

    /// 让所有非强制深色画廊的视图控制器保持 `.unspecified`，统一从所属 `UIWindow` 实时继承外观，
    /// 避免已弹出的 `.sheet` 或 `NavigationStack` 子控制器残留旧的 `overrideUserInterfaceStyle`。
    private static func clearViewControllerAppearanceOverrides(from vc: UIViewController) {
        if vc is SingleImagePreviewController || vc is FullScreenGalleryViewController {
            return
        }
        if vc.overrideUserInterfaceStyle != .unspecified {
            vc.overrideUserInterfaceStyle = .unspecified
        }
        for child in vc.children {
            clearViewControllerAppearanceOverrides(from: child)
        }
        if let presented = vc.presentedViewController {
            clearViewControllerAppearanceOverrides(from: presented)
        }
    }
    
    /// 「常用模型」：设置页配置的两个常用模型（全模型全思考强度可选）。
    /// 常用模型 1 为进入新会话的首选模型；单击输入框上方模型胶囊在常用模型 1 与常用模型 2 间快速切换。
    public var favoriteModel1: String {
        didSet { UserDefaults.standard.set(favoriteModel1, forKey: favoriteModel1Key) }
    }

    public var favoriteModel2: String {
        didSet { UserDefaults.standard.set(favoriteModel2, forKey: favoriteModel2Key) }
    }

    /// 新建会话默认选用的具体模型（首选常用模型 1）
    public var defaultNewSessionModel: String { favoriteModel1 }

    public var activeModelEnum: String {
        if let known = ModelCatalogStore.shared.modelEnum(forID: activeModel) { return known }
        switch activeModelProviderKind {
        case .claude: return "MODEL_PLACEHOLDER_M26"
        case .orchestrator: return "MODEL_PLACEHOLDER_M537"
        case .gemini, .other: return "MODEL_PLACEHOLDER_M532"
        }
    }

    public var activeModelDisplayName: String {
        ModelDefaultsLogic.compactDisplayName(forID: activeModel)
    }

    public var activeModelProviderKind: ModelProviderKind {
        ModelDefaultsLogic.providerKind(forID: activeModel)
    }

    public var isClaudeActive: Bool {
        ModelDefaultsLogic.isClaude(activeModel)
    }

    public func toggleActiveModel() {
        activeModel = ModelDefaultsLogic.toggleFavoriteTarget(
            current: activeModel,
            favorite1: favoriteModel1,
            favorite2: favoriteModel2
        )
    }

    public func syncModel(from raw: String?) {
        let target = ModelDefaultsLogic.resolveActive(
            raw: raw,
            current: activeModel,
            geminiDefault: favoriteModel1,
            claudeDefault: favoriteModel2
        )
        if activeModel != target {
            activeModel = target
        }
    }
    
    public var candidateEndpoints: [ServerEndpointItem] {
        var items: [ServerEndpointItem] = []
        if let lan = lanServerURL, !lan.isEmpty {
            let normalized = Self.normalize(raw: lan)?.absoluteString ?? lan
            items.append(ServerEndpointItem(type: "lan", urlString: normalized))
        }
        if let custom = customServerURL, !custom.isEmpty {
            let normalized = Self.normalize(raw: custom)?.absoluteString ?? custom
            items.append(ServerEndpointItem(type: "custom", urlString: normalized))
        }
        if let cloud = primaryCloudURL, !cloud.isEmpty {
            let normalized = Self.normalize(raw: cloud)?.absoluteString ?? cloud
            items.append(ServerEndpointItem(type: "cloudflare", urlString: normalized))
        }
        if items.isEmpty && !rawServerURL.isEmpty {
            let normalized = Self.normalize(raw: rawServerURL)?.absoluteString ?? rawServerURL
            items.append(ServerEndpointItem(type: "primary", urlString: normalized))
        }
        return items
    }
    
    /// Accurately describes the connection channel based on target endpoint and active interface
    public static func describeEndpoint(url: URL, isCellular: Bool) -> String {
        guard let host = url.host else {
            return isCellular ? String(localized: "蜂窝网络") : String(localized: "Wi-Fi")
        }
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        
        let isLAN = NetworkTransport.isLanOnlyHost(clean)
        let isTailscale = NetworkTransport.isTailscaleHost(clean)
        
        if isLAN {
            return String(localized: "Wi-Fi 局域网直连")
        } else if isTailscale {
            return isCellular ? String(localized: "蜂窝网络 (Tailscale)") : String(localized: "Wi-Fi (Tailscale)")
        } else if url.scheme == "https" {
            return isCellular ? String(localized: "蜂窝网络 (专属公网 HTTPS)") : String(localized: "Wi-Fi (专属公网 HTTPS)")
        } else {
            return isCellular ? String(localized: "蜂窝网络 (公网直连)") : String(localized: "Wi-Fi (公网直连)")
        }
    }
    
    /// HTTP is allowed only for loopback, RFC1918, Tailscale CGNAT (100.x), and .local.
    public static func allowsCleartextHTTP(_ hostPort: String) -> Bool {
        NetworkTransport.isLocalOrPrivateHost(hostPort)
    }
    
    public static func normalize(raw: String) -> URL? {
        var clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if clean.isEmpty { return nil }
        
        // 1. Separate scheme if already provided
        var scheme = ""
        if let range = clean.range(of: "://") {
            let parsedScheme = String(clean[..<range.lowerBound]).lowercased()
            // M-2: Strictly enforce http and https schemes
            guard parsedScheme == "http" || parsedScheme == "https" else {
                return nil
            }
            scheme = parsedScheme + "://"
            clean = String(clean[range.upperBound...])
        }
        
        // 2. Determine scheme if not present: HTTP only for loopback / RFC1918 / Tailscale / .local.
        if scheme.isEmpty {
            scheme = allowsCleartextHTTP(clean) ? "http://" : "https://"
        }
        
        return URL(string: scheme + clean)
    }
    
    public var serverURL: URL? {
        if isDemoMode { return DemoGateway.baseURL }
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        
        if isCell {
            // Cellular mode: Strictly avoid LAN-only addresses (192.168.x.x, 10.x.x.x, etc.) to prevent timeouts.
            // Tailscale virtual IPs (100.64/10, *.ts.net) are allowed: the WireGuard tunnel works on cellular.
            // Priority 1: Dynamic activeServerURL (if established and not LAN-only)
            if let active = activeServerURL, let url = Self.normalize(raw: active) {
                if !NetworkTransport.isLanOnlyHost(url.host ?? "") {
                    return url
                }
            }
            
            // Priority 2: Custom (if configured and not LAN-only)
            if let custom = customServerURL, let url = Self.normalize(raw: custom) {
                if !NetworkTransport.isLanOnlyHost(url.host ?? "") {
                    return url
                }
            }
            
            // Priority 3: Primary Cloudflare HTTPS Domain (default fallback on cellular)
            if let cloud = primaryCloudURL, let url = Self.normalize(raw: cloud) {
                if !NetworkTransport.isLanOnlyHost(url.host ?? "") {
                    return url
                }
            }
            
            // Priority 4: rawServerURL only if not LAN-only
            if let raw = Self.normalize(raw: rawServerURL) {
                if !NetworkTransport.isLanOnlyHost(raw.host ?? "") {
                    return raw
                }
            }
            
            return nil
        } else {
            // Wi-Fi mode:
            // Priority 1: Dynamic activeServerURL (if elected and reachable)
            if let active = activeServerURL, let url = Self.normalize(raw: active) {
                return url
            }
            
            // Priority 2: LAN (local direct connection for lowest latency)
            if let lan = lanServerURL, let url = Self.normalize(raw: lan) {
                return url
            }
            
            // Priority 3: Custom
            if let custom = customServerURL, let url = Self.normalize(raw: custom) {
                return url
            }
            
            // Priority 4: Primary Cloudflare HTTPS Domain
            if let cloud = primaryCloudURL, let url = Self.normalize(raw: cloud) {
                return url
            }
            
            return Self.normalize(raw: rawServerURL)
        }
    }
    
    public var gatewayURL: URL? {
        serverURL
    }
    
    public var deviceToken: String? {
        KeychainHelper.shared.read(key: .deviceToken)
    }
    
    public var deviceID: String? {
        KeychainHelper.shared.read(key: .deviceID)
    }
    
    public private(set) var isPaired: Bool
    
    /// Demo mode: the app talks to the built-in `DemoGateway` instead of a real gateway
    /// (used for App Review and for trying the app without a desktop).
    public private(set) var isDemoMode: Bool = false
    
    public func enterDemoMode() {
        DemoGateway.shared.reset()
        DemoGateway.isEnabled = true
        URLProtocol.registerClass(DemoURLProtocol.self)
        DemoGateway.seedDrafts()
        UserDefaults.standard.set(true, forKey: demoModeKey)
        self.isDemoMode = true
        self.isPaired = true
        syncWidgetConfiguration()
    }
    
    public func refreshPairedState() {
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = isDemoMode || (token != nil && !token!.isEmpty)
        syncWidgetConfiguration()
    }
    
    public func updateEndpoints(
        lan: String? = nil,
        relay: String? = nil,
        custom: String? = nil,
        active: String? = nil,
        primaryCloud: String? = nil
    ) {
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = isDemoMode || (token != nil && !token!.isEmpty)
        if let lan = lan, !lan.isEmpty {
            self.lanServerURL = lan
        }
        if let relay = relay, !relay.isEmpty {
            self.relayServerURL = relay
            if self.primaryCloudURL == nil || self.primaryCloudURL!.isEmpty {
                self.primaryCloudURL = relay
            }
        }
        if let cloud = primaryCloud, !cloud.isEmpty, !NetworkTransport.isLocalOrPrivateHost(cloud) {
            self.primaryCloudURL = cloud
        }
        if let custom = custom, !custom.isEmpty {
            let cleanCustom = custom.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            let cleanLan = self.lanServerURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            let cleanCloud = self.primaryCloudURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            if cleanCustom != cleanLan && cleanCustom != cleanCloud {
                self.customServerURL = custom
            }
        }
        if let active = active, !active.isEmpty {
            self.activeServerURL = active
            self.rawServerURL = active
            if !NetworkTransport.isLocalOrPrivateHost(active) {
                self.primaryCloudURL = active
            }
        }
        syncWidgetConfiguration()
    }
    
    public func unpair() {
        KeychainHelper.shared.clearAll()
        DemoGateway.isEnabled = false
        UserDefaults.standard.removeObject(forKey: demoModeKey)
        self.isDemoMode = false
        self.isPaired = false
        self.rawServerURL = ""
        self.primaryCloudURL = nil
        self.lanServerURL = nil
        self.relayServerURL = nil
        self.customServerURL = nil
        self.activeServerURL = nil
        UserDefaults.standard.removeObject(forKey: serverURLKey)
        UserDefaults.standard.removeObject(forKey: primaryCloudURLKey)
        UserDefaults.standard.removeObject(forKey: lanServerURLKey)
        UserDefaults.standard.removeObject(forKey: "antigravity.ipv6_server_url")
        UserDefaults.standard.removeObject(forKey: relayServerURLKey)
        UserDefaults.standard.removeObject(forKey: customServerURLKey)
        UserDefaults.standard.removeObject(forKey: activeServerURLKey)
        UserDefaults.standard.removeObject(forKey: gatewayPlatformKey)
        self.gatewayPlatform = nil
        QuotaWidgetStore.shared.clearAll()
        QuotaWidgetStore.shared.reloadWidgetTimelines()
    }

    /// Mirrors gateway endpoints, auth token, and preferred provider into the shared App Group container
    /// so `QuotaStatusWidget` can refresh in the background while the phone is locked.
    public func syncWidgetConfiguration(reloadWidget: Bool = true) {
        var urls: [String] = []
        var seen = Set<String>()
        let rawCandidates: [String?] = [
            serverURL?.absoluteString,
            activeServerURL,
            primaryCloudURL,
            lanServerURL,
            customServerURL,
            relayServerURL,
            rawServerURL,
        ]
        for raw in rawCandidates {
            guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines),
                  !trimmed.isEmpty,
                  trimmed.hasPrefix("http://") || trimmed.hasPrefix("https://") else {
                continue
            }
            let normalized = trimmed.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            let key = normalized.lowercased()
            if seen.insert(key).inserted {
                urls.append(normalized)
            }
        }

        let preferClaude = (selectedQuotaBucket == .claude5h || selectedQuotaBucket == .claudeWeekly)
        let isEmailMasked = UserDefaults.standard.bool(forKey: "cockpit_email_masked")
        let config = QuotaWidgetEndpointConfig(
            candidateBaseURLs: urls,
            deviceToken: isDemoMode ? nil : deviceToken,
            deviceID: isDemoMode ? nil : E2EECrypto.shared.currentDeviceID,
            e2eeKeyB64: isDemoMode ? nil : E2EECrypto.shared.currentDeviceKeyB64,
            preferClaude: preferClaude,
            selectedBucketRaw: selectedQuotaBucket.rawValue,
            paceCursorEnabled: weeklyPaceCursorEnabled,
            isEmailMasked: isEmailMasked,
            isDemoMode: isDemoMode,
            isPaired: isPaired
        )
        QuotaWidgetStore.shared.saveConfig(config)
        if isDemoMode {
            let now = Date()
            QuotaWidgetStore.shared.saveSnapshot(QuotaWidgetSnapshot.demoPlaceholder(now: now))
            QuotaWidgetStore.shared.saveConversationsSnapshot(AgentConversationsWidgetSnapshot.demoPlaceholder(now: now))
        }
        if reloadWidget {
            QuotaWidgetStore.shared.reloadWidgetTimelines()
        }
    }
    
    public init() {
        // Clean up legacy keys
        UserDefaults.standard.removeObject(forKey: "antigravity.prefer_cellular_network")
        UserDefaults.standard.removeObject(forKey: "antigravity.prefer_cellular_v2_migrated")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_token")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_id")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_secret")
        UserDefaults.standard.removeObject(forKey: "antigravity.ipv6_server_url")
        
        let demo = UserDefaults.standard.bool(forKey: demoModeKey)
        self.isDemoMode = demo
        DemoGateway.isEnabled = demo
        if demo { URLProtocol.registerClass(DemoURLProtocol.self) }
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = demo || (token != nil && !token!.isEmpty)
        
        let savedURL = UserDefaults.standard.string(forKey: serverURLKey) ?? "http://127.0.0.1:58900"
        let savedCloud = UserDefaults.standard.string(forKey: primaryCloudURLKey)
        let savedLan = UserDefaults.standard.string(forKey: lanServerURLKey)
        let savedRelay = UserDefaults.standard.string(forKey: relayServerURLKey)
        let savedCustom = UserDefaults.standard.string(forKey: customServerURLKey)
        let savedActive = UserDefaults.standard.string(forKey: activeServerURLKey)
        let savedLive = UserDefaults.standard.object(forKey: enableLiveActivityKey) as? Bool ?? true
        
        self.rawServerURL = savedURL
        var effectiveCloud = savedCloud
        if let s = effectiveCloud, NetworkTransport.isLocalOrPrivateHost(s) {
            effectiveCloud = nil
            UserDefaults.standard.removeObject(forKey: primaryCloudURLKey)
        }
        if (effectiveCloud == nil || effectiveCloud!.isEmpty) {
            if let savedActive = savedActive, !savedActive.isEmpty,
               !NetworkTransport.isLocalOrPrivateHost(savedActive) {
                effectiveCloud = savedActive
            } else if !savedURL.isEmpty, !NetworkTransport.isLocalOrPrivateHost(savedURL) {
                effectiveCloud = savedURL
            } else if let savedCustom = savedCustom, !savedCustom.isEmpty,
                      !NetworkTransport.isLocalOrPrivateHost(savedCustom) {
                effectiveCloud = savedCustom
            } else if let savedRelay = savedRelay, !savedRelay.isEmpty,
                      !NetworkTransport.isLocalOrPrivateHost(savedRelay) {
                effectiveCloud = savedRelay
            }
        }
        self.primaryCloudURL = effectiveCloud
        self.lanServerURL = savedLan
        self.relayServerURL = savedRelay
        if let savedCustom = savedCustom {
            let cleanCustom = savedCustom.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            let cleanLan = savedLan?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            let cleanCloud = savedCloud?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            
            if cleanCustom == cleanLan ||
               cleanCustom == cleanCloud ||
               cleanCustom.hasSuffix(".mgy.jiuge.space") {
                self.customServerURL = nil
                UserDefaults.standard.removeObject(forKey: customServerURLKey)
            } else {
                self.customServerURL = savedCustom
            }
        } else {
            self.customServerURL = nil
        }
        self.activeServerURL = savedActive
        self.enableLiveActivities = savedLive
        let savedPush = UserDefaults.standard.object(forKey: enablePushNotificationsKey) as? Bool ?? true
        self.enablePushNotifications = savedPush
        let savedCompleteSound = UserDefaults.standard.string(forKey: notificationCompleteSoundKey).flatMap { $0.isEmpty ? nil : $0 } ?? "glass"
        self.notificationCompleteSound = NotificationSoundOption.completeOptions.contains(where: { $0.id == savedCompleteSound }) ? savedCompleteSound : "glass"
        let savedActionSound = UserDefaults.standard.string(forKey: notificationActionSoundKey).flatMap { $0.isEmpty ? nil : $0 } ?? "alarm"
        self.notificationActionSound = NotificationSoundOption.actionOptions.contains(where: { $0.id == savedActionSound }) ? savedActionSound : "alarm"
        
        let legacyGemini = UserDefaults.standard.string(forKey: defaultGeminiModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackGemini
        let legacyClaude = UserDefaults.standard.string(forKey: defaultClaudeModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackClaude
        let legacyOrch = UserDefaults.standard.string(forKey: defaultOrchestratorModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackOrchestrator
        let legacyProvider = UserDefaults.standard.string(forKey: defaultNewSessionProviderKey).flatMap { $0.isEmpty ? nil : $0 } ?? "gemini"
        let migratedFav1: String = {
            switch legacyProvider {
            case "claude": return legacyClaude
            case "orchestrator": return legacyOrch
            default: return legacyGemini
            }
        }()
        let migratedFav2 = migratedFav1.caseInsensitiveCompare(legacyClaude) == .orderedSame ? legacyGemini : legacyClaude
        let resolvedFav1 = UserDefaults.standard.string(forKey: favoriteModel1Key).flatMap { $0.isEmpty ? nil : $0 } ?? migratedFav1
        let resolvedFav2 = UserDefaults.standard.string(forKey: favoriteModel2Key).flatMap { $0.isEmpty ? nil : $0 } ?? migratedFav2
        self.favoriteModel1 = resolvedFav1
        self.favoriteModel2 = resolvedFav2
        let savedModel = UserDefaults.standard.string(forKey: activeModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? resolvedFav1
        self.activeModel = savedModel
        
        self.enableVoiceMode = UserDefaults.standard.bool(forKey: enableVoiceModeKey)
        if UserDefaults.standard.object(forKey: weeklyPaceCursorKey) == nil {
            self.weeklyPaceCursorEnabled = true
        } else {
            self.weeklyPaceCursorEnabled = UserDefaults.standard.bool(forKey: weeklyPaceCursorKey)
        }
        let savedQuotaBucket = UserDefaults.standard.string(forKey: selectedQuotaBucketKey) ?? QuotaBucketKind.gemini5h.rawValue
        self.selectedQuotaBucket = QuotaBucketKind(rawValue: savedQuotaBucket) ?? .gemini5h
        let rawThemeMode = UserDefaults.standard.string(forKey: themeModeKey) ?? "system"
        if rawThemeMode == "nord" {
            self.themeMode = "dark"
            UserDefaults.standard.set("dark", forKey: themeModeKey)
        } else {
            self.themeMode = rawThemeMode
        }
        self.shareCardSkinID = UserDefaults.standard.string(forKey: shareCardSkinKey) ?? "auto"
        self.tokenHistoryChartType = UserDefaults.standard.string(forKey: tokenHistoryChartKey) ?? ""
        let savedScale = UserDefaults.standard.string(forKey: fontSizeScaleKey) ?? "standard"
        self.fontSizeScale = FontSizeScale(rawValue: savedScale) ?? .standard
        self.isKnowledgeModeEnabled = UserDefaults.standard.bool(forKey: knowledgeModeKey)
        if UserDefaults.standard.object(forKey: compactCollapseToolsKey) == nil {
            self.compactCollapseTools = true
        } else {
            self.compactCollapseTools = UserDefaults.standard.bool(forKey: compactCollapseToolsKey)
        }
        if UserDefaults.standard.object(forKey: compactCollapseTasksKey) == nil {
            self.compactCollapseTasks = true
        } else {
            self.compactCollapseTasks = UserDefaults.standard.bool(forKey: compactCollapseTasksKey)
        }
        self.compactHideHealthyContext = UserDefaults.standard.bool(forKey: compactHideHealthyContextKey)
        self.gatewayPlatform = UserDefaults.standard.string(forKey: gatewayPlatformKey)
        self.workspaceNotes = (UserDefaults.standard.dictionary(forKey: workspaceNotesKey) as? [String: String]) ?? [:]
        self.cachedPermissionPreset = UserDefaults.standard.string(forKey: cachedPermissionPresetKey) ?? "strict"
        syncWidgetConfiguration(reloadWidget: false)
    }
}
