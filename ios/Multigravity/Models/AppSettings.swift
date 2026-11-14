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
    private let activeModelKey = "antigravity.active_model"
    private let defaultGeminiModelKey = "antigravity.default_gemini_model"
    private let defaultClaudeModelKey = "antigravity.default_claude_model"
    private let defaultOrchestratorModelKey = "antigravity.default_orchestrator_model"
    private let defaultNewSessionProviderKey = "antigravity.default_new_session_provider"
    private let autoApprovePermissionsKey = "antigravity.auto_approve_permissions"
    private let enableVoiceModeKey = "antigravity.enable_voice_mode"
    private let weeklyPaceCursorKey = "antigravity.weekly_pace_cursor"
    private let themeModeKey = "antigravity.theme_mode"
    private let fontSizeScaleKey = "antigravity.font_size_scale"
    private let knowledgeModeKey = "antigravity.knowledge_mode_enabled"
    private let gatewayPlatformKey = "antigravity.gateway_platform"
    private let demoModeKey = "antigravity.demo_mode"

    public enum FontSizeScale: String, CaseIterable, Identifiable, Sendable {
        case compact = "compact"
        case standard = "standard"
        case comfortable = "comfortable"

        public var id: String { rawValue }

        public var displayName: String {
            switch self {
            case .compact: return "紧凑 (90%)"
            case .standard: return "标准 (100%)"
            case .comfortable: return "舒适 (115%)"
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
            return "Windows 网关"
        case "darwin", "macos", "mac":
            return "Mac 网关"
        case "linux":
            return "Linux 网关"
        default:
            return "电脑网关"
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
    
    public var activeModel: String {
        didSet {
            UserDefaults.standard.set(activeModel, forKey: activeModelKey)
        }
    }
    
    public var autoApprovePermissions: Bool {
        didSet {
            UserDefaults.standard.set(autoApprovePermissions, forKey: autoApprovePermissionsKey)
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
        }
    }

    /// 外观主题模式："system"（跟随系统）、"light"（浅色）、"dark"（深色）
    public var themeMode: String {
        didSet {
            UserDefaults.standard.set(themeMode, forKey: themeModeKey)
            applyAppearance()
        }
    }

    /// 字体缩放比例：紧凑、标准、舒适
    public var fontSizeScale: FontSizeScale {
        didSet {
            UserDefaults.standard.set(fontSizeScale.rawValue, forKey: fontSizeScaleKey)
        }
    }

    /// 知识工作流模式（精简界面，突出阅读与文档生成）
    public var isKnowledgeModeEnabled: Bool {
        didSet {
            UserDefaults.standard.set(isKnowledgeModeEnabled, forKey: knowledgeModeKey)
        }
    }

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
                    var currentVC = window.rootViewController
                    while let vc = currentVC {
                        if !(vc is SingleImagePreviewController) && !(vc is FullScreenGalleryViewController) {
                            vc.overrideUserInterfaceStyle = style
                        }
                        currentVC = vc.presentedViewController
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
    
    /// 「默认模型」：输入框上方 Gemini / Claude 胶囊切换时使用的具体模型。
    public var defaultGeminiModel: String {
        didSet { UserDefaults.standard.set(defaultGeminiModel, forKey: defaultGeminiModelKey) }
    }

    public var defaultClaudeModel: String {
        didSet { UserDefaults.standard.set(defaultClaudeModel, forKey: defaultClaudeModelKey) }
    }

    public var defaultOrchestratorModel: String {
        didSet { UserDefaults.standard.set(defaultOrchestratorModel, forKey: defaultOrchestratorModelKey) }
    }

    /// 新建会话默认使用的模型厂商（"gemini" | "claude" | "orchestrator"）
    public var defaultNewSessionProvider: String {
        didSet { UserDefaults.standard.set(defaultNewSessionProvider, forKey: defaultNewSessionProviderKey) }
    }

    /// 新建会话默认选用的具体模型（跟随对应厂商的默认设置）
    public var defaultNewSessionModel: String {
        switch defaultNewSessionProvider {
        case "claude": return defaultClaudeModel
        case "orchestrator": return defaultOrchestratorModel
        default: return defaultGeminiModel
        }
    }

    public var activeModelEnum: String {
        if let known = ModelCatalogStore.shared.modelEnum(forID: activeModel) { return known }
        switch activeModelProviderKind {
        case .claude: return "MODEL_PLACEHOLDER_M26"
        case .orchestrator: return "MODEL_PLACEHOLDER_M667"
        case .gemini, .other: return "MODEL_PLACEHOLDER_M318"
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
        activeModel = ModelDefaultsLogic.toggleTarget(
            current: activeModel,
            geminiDefault: defaultGeminiModel,
            claudeDefault: defaultClaudeModel,
            orchestratorDefault: defaultOrchestratorModel,
            hasOrchestrator: ModelCatalogStore.shared.hasOrchestratorModels
        )
    }

    public func syncModel(from raw: String?) {
        let target = ModelDefaultsLogic.resolveActive(
            raw: raw,
            current: activeModel,
            geminiDefault: defaultGeminiModel,
            claudeDefault: defaultClaudeModel,
            orchestratorDefault: defaultOrchestratorModel
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
            return isCellular ? "蜂窝网络" : "Wi-Fi"
        }
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        
        let isLAN = NetworkTransport.isLanOnlyHost(clean)
        let isTailscale = NetworkTransport.isTailscaleHost(clean)
        
        if isLAN {
            return "Wi-Fi 局域网直连"
        } else if isTailscale {
            return isCellular ? "蜂窝网络 (Tailscale)" : "Wi-Fi (Tailscale)"
        } else if url.scheme == "https" {
            return isCellular ? "蜂窝网络 (专属公网 HTTPS)" : "Wi-Fi (专属公网 HTTPS)"
        } else {
            return isCellular ? "蜂窝网络 (公网直连)" : "Wi-Fi (公网直连)"
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
    }
    
    public func refreshPairedState() {
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = isDemoMode || (token != nil && !token!.isEmpty)
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
            let cleanURL = savedURL.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            
            if cleanCustom == cleanLan ||
               cleanCustom == cleanCloud ||
               (!cleanURL.isEmpty && cleanCustom == cleanURL && !NetworkTransport.isLocalOrPrivateHost(savedURL)) {
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
        
        let savedModel = UserDefaults.standard.string(forKey: activeModelKey) ?? "gemini-3.8-flash-high"
        self.activeModel = savedModel
        self.defaultGeminiModel = UserDefaults.standard.string(forKey: defaultGeminiModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackGemini
        self.defaultClaudeModel = UserDefaults.standard.string(forKey: defaultClaudeModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackClaude
        self.defaultOrchestratorModel = UserDefaults.standard.string(forKey: defaultOrchestratorModelKey).flatMap { $0.isEmpty ? nil : $0 } ?? ModelDefaultsLogic.fallbackOrchestrator
        self.defaultNewSessionProvider = UserDefaults.standard.string(forKey: defaultNewSessionProviderKey).flatMap { $0.isEmpty ? nil : $0 } ?? "gemini"
        
        let savedAutoApprove = UserDefaults.standard.bool(forKey: autoApprovePermissionsKey)
        self.autoApprovePermissions = savedAutoApprove
        self.enableVoiceMode = UserDefaults.standard.bool(forKey: enableVoiceModeKey)
        if UserDefaults.standard.object(forKey: weeklyPaceCursorKey) == nil {
            self.weeklyPaceCursorEnabled = true
        } else {
            self.weeklyPaceCursorEnabled = UserDefaults.standard.bool(forKey: weeklyPaceCursorKey)
        }
        self.themeMode = UserDefaults.standard.string(forKey: themeModeKey) ?? "system"
        let savedScale = UserDefaults.standard.string(forKey: fontSizeScaleKey) ?? "standard"
        self.fontSizeScale = FontSizeScale(rawValue: savedScale) ?? .standard
        self.isKnowledgeModeEnabled = UserDefaults.standard.bool(forKey: knowledgeModeKey)
        self.gatewayPlatform = UserDefaults.standard.string(forKey: gatewayPlatformKey)
    }
}
