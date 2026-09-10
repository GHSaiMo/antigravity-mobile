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
        if !clean.lowercased().hasPrefix("http://") && !clean.lowercased().hasPrefix("https://") {
            clean = "https://" + clean
        }
        return URL(string: clean)
    }
    
    public var gatewayURL: URL? {
        serverURL
    }
    
    public init() {
        let savedURL = UserDefaults.standard.string(forKey: serverURLKey) ?? "http://127.0.0.1:58900"
        let savedLive = UserDefaults.standard.object(forKey: enableLiveActivityKey) as? Bool ?? false
        let savedPreferCellular = UserDefaults.standard.object(forKey: preferCellularNetworkKey) as? Bool ?? true
        
        self.rawServerURL = savedURL
        self.enableLiveActivities = savedLive
        self.preferCellularNetwork = savedPreferCellular
    }
}
