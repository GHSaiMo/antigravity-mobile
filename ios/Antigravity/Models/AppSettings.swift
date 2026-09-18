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
            items.append(ServerEndpointItem(type: "lan", urlString: lan))
        }
        if let v6 = ipv6ServerURL, !v6.isEmpty {
            items.append(ServerEndpointItem(type: "ipv6", urlString: v6))
        }
        if let relay = relayServerURL, !relay.isEmpty {
            items.append(ServerEndpointItem(type: "relay", urlString: relay))
        }
        if let custom = customServerURL, !custom.isEmpty && custom != relayServerURL {
            items.append(ServerEndpointItem(type: "custom", urlString: custom))
        }
        if items.isEmpty && !rawServerURL.isEmpty {
            items.append(ServerEndpointItem(type: "primary", urlString: rawServerURL))
        }
        return items
    }
    
    /// Accurately describes the connection channel based on target endpoint and active interface
    public static func describeEndpoint(url: URL, isCellular: Bool) -> String {
        guard let host = url.host else {
            return isCellular ? "蜂窝网络" : "Wi-Fi"
        }
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        
        let isIPv6 = clean.contains(":") && !clean.hasPrefix("fe80") && !clean.hasPrefix("fc") && !clean.hasPrefix("fd")
        let isLAN = NetworkTransport.isLocalOrPrivateHost(clean)
        let isTailscale = clean.hasPrefix("100.") || clean.contains("ts.net")
        let isRelay = (AppSettings.shared.relayServerURL?.contains(clean) == true) || clean.contains("relay")
        
        if isLAN {
            return "Wi-Fi 局域网"
        } else if isIPv6 {
            return isCellular ? "蜂窝网络 IPv6 直连" : "Wi-Fi IPv6 直连"
        } else if isRelay {
            return isCellular ? "蜂窝网络 (云中继)" : "Wi-Fi (云中继)"
        } else if isTailscale {
            return isCellular ? "蜂窝网络 (Tailscale)" : "Wi-Fi (Tailscale)"
        } else {
            return isCellular ? "蜂窝网络 (公网)" : "Wi-Fi (公网)"
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
        // 1. Dynamic endpoint selection: If ConnectionManager has established an activeServerURL,
        // use it directly (it has already undergone connectivity and latency probing).
        if let active = activeServerURL, let url = Self.normalize(raw: active) {
            return url
        }
        
        // 2. Default fallback priority:
        // When not on cellular, check LAN first
        if !NetworkTransport.shared.isCellular {
            if let lan = lanServerURL, let url = Self.normalize(raw: lan) {
                return url
            }
        }
        // Then public IPv6 direct
        if let v6 = ipv6ServerURL, let url = Self.normalize(raw: v6) {
            return url
        }
        // Then Cloud Relay
        if let relay = relayServerURL, let url = Self.normalize(raw: relay) {
            return url
        }
        // Then Custom DDNS
        if let custom = customServerURL, let url = Self.normalize(raw: custom) {
            return url
        }
        // LAN fallback if not already tried
        if let lan = lanServerURL, let url = Self.normalize(raw: lan) {
            return url
        }
        return Self.normalize(raw: rawServerURL)
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
    
    public func updateEndpoints(lan: String? = nil, ipv6: String? = nil, relay: String? = nil, custom: String? = nil, active: String? = nil) {
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
        }
        if let custom = custom, !custom.isEmpty {
            self.customServerURL = custom
        }
        if let active = active, !active.isEmpty {
            self.activeServerURL = active
            self.rawServerURL = active
        }
    }
    
    public func unpair() {
        KeychainHelper.shared.clearAll()
        self.isPaired = false
        self.rawServerURL = ""
        self.lanServerURL = nil
        self.ipv6ServerURL = nil
        self.relayServerURL = nil
        self.customServerURL = nil
        self.activeServerURL = nil
        UserDefaults.standard.removeObject(forKey: serverURLKey)
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
        let savedLan = UserDefaults.standard.string(forKey: lanServerURLKey)
        let savedIPv6 = UserDefaults.standard.string(forKey: ipv6ServerURLKey)
        let savedRelay = UserDefaults.standard.string(forKey: relayServerURLKey)
        let savedCustom = UserDefaults.standard.string(forKey: customServerURLKey)
        let savedActive = UserDefaults.standard.string(forKey: activeServerURLKey)
        let savedLive = UserDefaults.standard.object(forKey: enableLiveActivityKey) as? Bool ?? true
        
        self.rawServerURL = savedURL
        self.lanServerURL = savedLan
        self.ipv6ServerURL = savedIPv6
        self.relayServerURL = savedRelay
        self.customServerURL = savedCustom
        self.activeServerURL = savedActive
        self.enableLiveActivities = savedLive
        
        let savedModel = UserDefaults.standard.string(forKey: activeModelKey) ?? "gemini-3.8-flash-high"
        self.activeModel = savedModel
        
        let savedAutoApprove = UserDefaults.standard.bool(forKey: autoApprovePermissionsKey)
        self.autoApprovePermissions = savedAutoApprove
    }
}
