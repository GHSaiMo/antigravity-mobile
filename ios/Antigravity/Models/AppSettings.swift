import Foundation
import Observation

@Observable
public final class AppSettings {
    public static let shared = AppSettings()
    
    private let serverURLKey = "antigravity.server_url"
    private let enableLiveActivityKey = "antigravity.enable_live_activity"
    private let cfClientIdKey = "antigravity.cf_client_id"
    private let cfClientSecretKey = "antigravity.cf_client_secret"
    
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
    
    public var cfAccessClientId: String {
        didSet {
            UserDefaults.standard.set(cfAccessClientId, forKey: cfClientIdKey)
        }
    }
    
    public var cfAccessClientSecret: String {
        didSet {
            UserDefaults.standard.set(cfAccessClientSecret, forKey: cfClientSecretKey)
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
        let savedClientId = UserDefaults.standard.string(forKey: cfClientIdKey) ?? ""
        let savedClientSecret = UserDefaults.standard.string(forKey: cfClientSecretKey) ?? ""
        
        self.rawServerURL = savedURL
        self.enableLiveActivities = savedLive
        self.cfAccessClientId = savedClientId
        self.cfAccessClientSecret = savedClientSecret
    }
}
