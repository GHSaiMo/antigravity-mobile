import Foundation
import Observation

public struct ServerEndpointItem: Identifiable, Equatable, Sendable {
    public let id: String
    public let type: String // "lan", "ipv6", "ddns", "primary", "custom"
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
    private let ipv6ServerURLKey = "antigravity.ipv6_server_url"
    private let relayServerURLKey = "antigravity.relay_server_url"
    private let customServerURLKey = "antigravity.custom_server_url"
    private let activeServerURLKey = "antigravity.active_server_url"
    private let enableLiveActivityKey = "antigravity.enable_live_activity"
    private let activeModelKey = "antigravity.active_model"
    private let autoApprovePermissionsKey = "antigravity.auto_approve_permissions"
    
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
    
    public var ipv6ServerURL: String? {
        didSet {
            if let val = ipv6ServerURL {
                UserDefaults.standard.set(val, forKey: ipv6ServerURLKey)
            } else {
                UserDefaults.standard.removeObject(forKey: ipv6ServerURLKey)
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
    
    public var activeModelEnum: String {
        activeModel == "claude-opus-4-6-thinking" ? "MODEL_PLACEHOLDER_M26" : "MODEL_PLACEHOLDER_M318"
    }
    
    public var activeModelDisplayName: String {
        activeModel == "claude-opus-4-6-thinking" ? "Claude" : "Gemini"
    }
    
    public var isClaudeActive: Bool {
        activeModel == "claude-opus-4-6-thinking"
    }
    
    public func toggleActiveModel() {
        if activeModel == "gemini-3.8-flash-high" {
            activeModel = "claude-opus-4-6-thinking"
        } else {
            activeModel = "gemini-3.8-flash-high"
        }
    }
    
    public func syncModel(from raw: String?) {
        guard let raw = raw, !raw.isEmpty else { return }
        let lower = raw.lowercased()
        let target = (lower.contains("claude") || lower.contains("m26")) ? "claude-opus-4-6-thinking" : "gemini-3.8-flash-high"
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
        
        let isLAN = NetworkTransport.isLocalOrPrivateHost(clean)
        let isTailscale = clean.hasPrefix("100.") || clean.contains("ts.net")
        
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
    
    /// HTTP is allowed only for loopback, RFC1918, Tailscale CGNAT (100.x), .local, and IPv6 ULA/link-local.
    public static func allowsCleartextHTTP(_ hostPort: String) -> Bool {
        var host = hostPort.lowercased()
        if host.hasPrefix("[") {
            if let end = host.firstIndex(of: "]") {
                host = String(host[host.index(after: host.startIndex)..<end])
            }
        } else if let colon = host.lastIndex(of: ":"),
                  host[colon...].dropFirst().allSatisfy({ $0.isNumber }) {
            host = String(host[..<colon])
        }
        if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0:0:0:0:0:0:0:1" {
            return true
        }
        if host.hasSuffix(".local") {
            return true
        }
        if host.hasPrefix("192.168.") || host.hasPrefix("10.") || host.hasPrefix("100.") {
            return true
        }
        if host.hasPrefix("172.") {
            let parts = host.split(separator: ".")
            if parts.count >= 2, let second = Int(parts[1]), (16...31).contains(second) {
                return true
            }
        }
        // IPv6 literals (including global unicast used for cellular pairing).
        if host.contains(":") {
            return true
        }
        return false
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
        
        // 2. Normalize unbracketed IPv6 address
        if !clean.hasPrefix("[") && clean.filter({ $0 == ":" }).count >= 2 {
            if let lastColon = clean.lastIndex(of: ":") {
                let possiblePort = String(clean[clean.index(after: lastColon)...])
                if let port = Int(possiblePort), port > 0 && port <= 65535 {
                    let ipPart = String(clean[..<lastColon])
                    clean = "[\(ipPart)]:\(port)"
                } else {
                    clean = "[\(clean)]"
                }
            } else {
                clean = "[\(clean)]"
            }
        }
        
        // 3. Determine scheme if not present: HTTP only for loopback / RFC1918 / Tailscale / .local / ULA.
        if scheme.isEmpty {
            scheme = allowsCleartextHTTP(clean) ? "http://" : "https://"
        }
        
        return URL(string: scheme + clean)
    }
    
    public var serverURL: URL? {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        
        if isCell {
            // Cellular mode: Strictly avoid LAN addresses (192.168.x.x, 10.x.x.x, etc.) to prevent timeouts
            // Priority 1: Dynamic activeServerURL (if established and not LAN)
            if let active = activeServerURL, let url = Self.normalize(raw: active) {
                if !NetworkTransport.isLocalOrPrivateHost(url.host ?? "") {
                    return url
                }
            }
            
            // Priority 2: Custom (if configured and not LAN)
            if let custom = customServerURL, let url = Self.normalize(raw: custom) {
                if !NetworkTransport.isLocalOrPrivateHost(url.host ?? "") {
                    return url
                }
            }
            
            // Priority 3: Primary Cloudflare HTTPS Domain (default fallback on cellular)
            if let cloud = primaryCloudURL, let url = Self.normalize(raw: cloud) {
                if !NetworkTransport.isLocalOrPrivateHost(url.absoluteString) {
                    return url
                }
            }
            
            // Priority 4: rawServerURL only if not LAN
            if let raw = Self.normalize(raw: rawServerURL) {
                if !NetworkTransport.isLocalOrPrivateHost(raw.host ?? "") {
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
    
    public func refreshPairedState() {
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = (token != nil && !token!.isEmpty)
    }
    
    public func updateEndpoints(
        lan: String? = nil,
        ipv6: String? = nil,
        relay: String? = nil,
        custom: String? = nil,
        active: String? = nil,
        primaryCloud: String? = nil
    ) {
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = (token != nil && !token!.isEmpty)
        if let lan = lan, !lan.isEmpty {
            self.lanServerURL = lan
        }
        if let ipv6 = ipv6, !ipv6.isEmpty {
            self.ipv6ServerURL = ipv6
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
        self.isPaired = false
        self.rawServerURL = ""
        self.primaryCloudURL = nil
        self.lanServerURL = nil
        self.ipv6ServerURL = nil
        self.relayServerURL = nil
        self.customServerURL = nil
        self.activeServerURL = nil
        UserDefaults.standard.removeObject(forKey: serverURLKey)
        UserDefaults.standard.removeObject(forKey: primaryCloudURLKey)
        UserDefaults.standard.removeObject(forKey: lanServerURLKey)
        UserDefaults.standard.removeObject(forKey: ipv6ServerURLKey)
        UserDefaults.standard.removeObject(forKey: relayServerURLKey)
        UserDefaults.standard.removeObject(forKey: customServerURLKey)
        UserDefaults.standard.removeObject(forKey: activeServerURLKey)
    }
    
    public init() {
        // Clean up legacy keys
        UserDefaults.standard.removeObject(forKey: "antigravity.prefer_cellular_network")
        UserDefaults.standard.removeObject(forKey: "antigravity.prefer_cellular_v2_migrated")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_token")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_id")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_secret")
        
        let token = KeychainHelper.shared.read(key: .deviceToken)
        self.isPaired = (token != nil && !token!.isEmpty)
        
        let savedURL = UserDefaults.standard.string(forKey: serverURLKey) ?? "http://127.0.0.1:58900"
        let savedCloud = UserDefaults.standard.string(forKey: primaryCloudURLKey)
        let savedLan = UserDefaults.standard.string(forKey: lanServerURLKey)
        let savedIPv6 = UserDefaults.standard.string(forKey: ipv6ServerURLKey)
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
        self.ipv6ServerURL = savedIPv6
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
        
        let savedAutoApprove = UserDefaults.standard.bool(forKey: autoApprovePermissionsKey)
        self.autoApprovePermissions = savedAutoApprove
    }
}
