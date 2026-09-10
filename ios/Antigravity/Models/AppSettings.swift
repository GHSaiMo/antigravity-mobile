import Foundation
import Observation

@Observable
public final class AppSettings {
    public static let shared = AppSettings()
    
    private let serverURLKey = "antigravity.server_url"
    private let enableLiveActivityKey = "antigravity.enable_live_activity"
    private let preferCellularNetworkKey = "antigravity.prefer_cellular_network"
    
    public var rawServerURL: String {
        didSet {
            UserDefaults.standard.set(rawServerURL, forKey: serverURLKey)
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
        }
    }
    
    public var serverURL: URL? {
        var clean = rawServerURL.trimmingCharacters(in: .whitespacesAndNewlines)
        if clean.isEmpty { return nil }
        
        // 1. Separate scheme if already provided
        var scheme = ""
        if let range = clean.range(of: "://") {
            scheme = String(clean[..<range.upperBound]).lowercased()
            clean = String(clean[range.upperBound...])
        }
        
        // 2. Normalize unbracketed IPv6 address
        // Check if clean has multiple colons and isn't already bracketed
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
            // If it connects to default gateway port 58900, or is an IP address, default to http://
            if lower.contains(":58900") || lower.hasPrefix("127.0.0.1") || lower.hasPrefix("localhost") ||
               lower.hasPrefix("192.168.") || lower.hasPrefix("10.") || lower.hasPrefix("100.") ||
               lower.hasPrefix("172.") || lower.hasPrefix("[") {
                scheme = "http://"
            } else if lower.contains(":") {
                // If specifying a custom port, default to http://
                scheme = "http://"
            } else {
                // For standard domain names without port (e.g. agy.mycorp.com), default to https://
                scheme = "https://"
            }
        }
        
        return URL(string: scheme + clean)
    }
    
    public var gatewayURL: URL? {
        serverURL
    }
    
    public init() {
        // Clean up any legacy Cloudflare credentials from UserDefaults
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_token")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_id")
        UserDefaults.standard.removeObject(forKey: "antigravity.cf_access_client_secret")
        
        let savedURL = UserDefaults.standard.string(forKey: serverURLKey) ?? "http://127.0.0.1:58900"
        let savedLive = UserDefaults.standard.object(forKey: enableLiveActivityKey) as? Bool ?? false
        
        // One-time migration: reset preferCellularNetwork to false (default) so that users
        // on local Wi-Fi don't have their traffic forcibly redirected to cellular.
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
        self.enableLiveActivities = savedLive
        self.preferCellularNetwork = savedPreferCellular
    }
}
