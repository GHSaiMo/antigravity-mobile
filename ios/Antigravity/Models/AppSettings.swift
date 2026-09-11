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
    private let customServerURLKey = "antigravity.custom_server_url"
    private let activeServerURLKey = "antigravity.active_server_url"
    private let enableLiveActivityKey = "antigravity.enable_live_activity"
    private let preferCellularNetworkKey = "antigravity.prefer_cellular_network"
    private let activeModelKey = "antigravity.active_model"
    
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
    
    public var preferCellularNetwork: Bool {
        didSet {
            UserDefaults.standard.set(preferCellularNetwork, forKey: preferCellularNetworkKey)
            
            // Automatically switch activeServerURL and rawServerURL to match the selected route strategy
            if preferCellularNetwork {
                if let v6 = ipv6ServerURL, !v6.isEmpty {
                    self.activeServerURL = v6
                    self.rawServerURL = v6
                } else if let custom = customServerURL, !custom.isEmpty {
                    self.activeServerURL = custom
                    self.rawServerURL = custom
                }
            } else {
                // If turning off cellular preference, only revert to LAN if not currently on cellular data
                if !NetworkTransport.shared.isCellular, let lan = lanServerURL, !lan.isEmpty {
                    self.activeServerURL = lan
                    self.rawServerURL = lan
                }
            }
            
            NotificationCenter.default.post(name: .networkRoutingPreferenceChanged, object: nil)
        }
    }
    
    public var activeModel: String {
        didSet {
            UserDefaults.standard.set(activeModel, forKey: activeModelKey)
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
        if let custom = customServerURL, !custom.isEmpty {
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
        
        // 1. Check if host is IPv6 (contains colons and is not link-local fe80 / ULA fc/fd)
        let isIPv6 = clean.contains(":") && !clean.hasPrefix("fe80") && !clean.hasPrefix("fc") && !clean.hasPrefix("fd")
        
        // 2. Check if host is LAN / private
        let isLAN = NetworkTransport.isLocalOrPrivateHost(clean)
        
        // 3. Check if host is Tailscale CGNAT
        let isTailscale = clean.hasPrefix("100.") || clean.contains("ts.net")
        
        if isIPv6 {
            return isCellular ? "蜂窝网络 IPv6" : "Wi-Fi IPv6 直连"
        } else if isLAN {
            return "Wi-Fi 局域网"
        } else if isTailscale {
            return isCellular ? "蜂窝网络 (Tailscale)" : "Wi-Fi (Tailscale)"
        } else {
            return isCellular ? "蜂窝网络 (公网域名)" : "Wi-Fi (公网域名)"
        }
    }
    
    public static func normalize(raw: String) -> URL? {
        var clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if clean.isEmpty { return nil }
        
        // 1. Separate scheme if already provided
        var scheme = ""
        if let range = clean.range(of: "://") {
            scheme = String(clean[..<range.upperBound]).lowercased()
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
        
        // 3. Determine scheme if not present
        if scheme.isEmpty {
            let lower = clean.lowercased()
            if lower.contains(":58900") || lower.hasPrefix("127.0.0.1") || lower.hasPrefix("localhost") ||
               lower.hasPrefix("192.168.") || lower.hasPrefix("10.") || lower.hasPrefix("100.") ||
               lower.hasPrefix("172.") || lower.hasPrefix("[") {
                scheme = "http://"
            } else if lower.contains(":") {
                scheme = "http://"
            } else {
                scheme = "https://"
            }
        }
        
        return URL(string: scheme + clean)
    }
    
    public var serverURL: URL? {
        // 1. Dynamic endpoint selection: If ConnectionManager has established an activeServerURL,
        // use it directly (it has already undergone connectivity and latency probing).
        if let active = activeServerURL, let url = Self.normalize(raw: active) {
            return url
        }
        
        let shouldUseCellularOrRemote = preferCellularNetwork || NetworkTransport.shared.isCellular
        if shouldUseCellularOrRemote {
            // When on cellular or prioritizing cellular (IPv6 direct connection) without an active probe result:
            // 1. If an IPv6 URL is configured, prioritize it
            if let v6 = ipv6ServerURL, let url = Self.normalize(raw: v6) {
                return url
            }
            // 2. If a custom DDNS URL is configured, use it
            if let custom = customServerURL, let url = Self.normalize(raw: custom) {
                return url
            }
            // 3. Fallback to LAN or raw
            if let lan = lanServerURL, let url = Self.normalize(raw: lan) {
                return url
            }
            return Self.normalize(raw: rawServerURL)
        } else {
            // Normal Wi-Fi / local routing mode without an active probe result:
            // 1. If LAN URL is configured, prefer LAN
            if let lan = lanServerURL, let url = Self.normalize(raw: lan) {
                return url
            }
            // 2. If IPv6 URL is configured
            if let v6 = ipv6ServerURL, let url = Self.normalize(raw: v6) {
                return url
            }
            // 3. If custom DDNS URL is configured
            if let custom = customServerURL, let url = Self.normalize(raw: custom) {
                return url
            }
            // 4. Default fallback to rawServerURL
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
    
    public var isPaired: Bool {
        guard let token = deviceToken, !token.isEmpty else { return false }
        return true
    }
    
    public func updateEndpoints(lan: String? = nil, ipv6: String? = nil, custom: String? = nil, active: String? = nil) {
        if let lan = lan, !lan.isEmpty {
            self.lanServerURL = lan
        }
        if let ipv6 = ipv6, !ipv6.isEmpty {
            self.ipv6ServerURL = ipv6
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
        self.lanServerURL = nil
        self.ipv6ServerURL = nil
        self.customServerURL = nil
        self.activeServerURL = nil
    }
    
    public init() {
        // Clean up any legacy Cloudflare credentials from UserDefaults
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_token")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_id")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_secret")
        
        let savedURL = UserDefaults.standard.string(forKey: serverURLKey) ?? "http://127.0.0.1:58900"
        let savedLan = UserDefaults.standard.string(forKey: lanServerURLKey)
        let savedIPv6 = UserDefaults.standard.string(forKey: ipv6ServerURLKey)
        let savedCustom = UserDefaults.standard.string(forKey: customServerURLKey)
        let savedActive = UserDefaults.standard.string(forKey: activeServerURLKey)
        let savedLive = UserDefaults.standard.object(forKey: enableLiveActivityKey) as? Bool ?? false
        
        let migrationKey = "antigravity.prefer_cellular_v2_migrated"
        let isMigrated = UserDefaults.standard.bool(forKey: migrationKey)
        let savedPreferCellular: Bool
        if !isMigrated {
            savedPreferCellular = false
            UserDefaults.standard.set(false, forKey: preferCellularNetworkKey)
            UserDefaults.standard.set(true, forKey: migrationKey)
        } else {
            savedPreferCellular = UserDefaults.standard.object(forKey: preferCellularNetworkKey) as? Bool ?? false
        }
        
        self.rawServerURL = savedURL
        self.lanServerURL = savedLan
        self.ipv6ServerURL = savedIPv6
        self.customServerURL = savedCustom
        self.activeServerURL = savedActive
        self.enableLiveActivities = savedLive
        self.preferCellularNetwork = savedPreferCellular
        
        let savedModel = UserDefaults.standard.string(forKey: activeModelKey) ?? "gemini-3.8-flash-high"
        self.activeModel = savedModel
    }
}
