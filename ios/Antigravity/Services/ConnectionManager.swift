import Foundation
import Network
import Observation
import os

public struct EndpointHealthStatus: Identifiable, Sendable {
    public let id: String
    public let urlString: String
    public let isReachable: Bool
    public let latencyMs: Double
    public let errorMessage: String?
    public let platform: String?
    
    public init(
        id: String,
        urlString: String,
        isReachable: Bool,
        latencyMs: Double,
        errorMessage: String? = nil,
        platform: String? = nil
    ) {
        self.id = id
        self.urlString = urlString
        self.isReachable = isReachable
        self.latencyMs = latencyMs
        self.errorMessage = errorMessage
        self.platform = platform
    }
}

/// Thread-safe global network state container accessible from any queue/Sendable context
public final class NetworkStatus: Sendable {
    public static let shared = NetworkStatus()
    private let _isCellular = OSAllocatedUnfairLock(initialState: false)
    private let _isWifi = OSAllocatedUnfairLock(initialState: true)
    private let _isConnected = OSAllocatedUnfairLock(initialState: true)
    
    public var isCellular: Bool {
        get { _isCellular.withLock { $0 } }
        set { _isCellular.withLock { $0 = newValue } }
    }
    
    public var isWifi: Bool {
        get { _isWifi.withLock { $0 } }
        set { _isWifi.withLock { $0 = newValue } }
    }
    
    public var isConnected: Bool {
        get { _isConnected.withLock { $0 } }
        set { _isConnected.withLock { $0 = newValue } }
    }
}

@Observable
@MainActor
public final class ConnectionManager {
    public static let shared = ConnectionManager()
    
    public var isProbing: Bool = false
    public var lastProbeDate: Date? = nil
    public var endpointStatuses: [String: EndpointHealthStatus] = [:]
    public var isCellular: Bool = false
    public var isWifi: Bool = true
    public var isConnectedToNetwork: Bool = true
    
    private let pathMonitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "antigravity.connection_monitor", qos: .utility)
    private var networkLostTask: Task<Void, Never>?
    
    private init() {
        startMonitoring()
        
        NotificationCenter.default.addObserver(
            forName: .networkRoutingPreferenceChanged,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self = self else { return }
                await self.probeEndpoints()
            }
        }
    }
    
    private func startMonitoring() {
        pathMonitor.pathUpdateHandler = { [weak self] path in
            let isSatisfied = (path.status == .satisfied)
            let currentCellular = path.isExpensive || path.usesInterfaceType(.cellular)
            let currentWifi = path.usesInterfaceType(.wifi)
            
            NetworkStatus.shared.isConnected = isSatisfied
            NetworkStatus.shared.isCellular = currentCellular
            NetworkStatus.shared.isWifi = currentWifi
            
            Task { @MainActor [weak self] in
                guard let self = self else { return }
                let wasCellular = self.isCellular
                let wasWifi = self.isWifi
                let wasConnected = self.isConnectedToNetwork
                
                self.isConnectedToNetwork = isSatisfied
                self.isCellular = currentCellular
                self.isWifi = currentWifi
                
                if !isSatisfied {
                    // Interface switching in progress (e.g. Wi-Fi severed, cellular acquiring)
                    self.networkLostTask?.cancel()
                    self.networkLostTask = Task { [weak self] in
                        try? await Task.sleep(nanoseconds: 500_000_000)
                        guard let self, !Task.isCancelled else { return }
                        await self.probeEndpoints()
                    }
                } else {
                    self.networkLostTask?.cancel()
                    self.networkLostTask = nil
                    
                    if !wasConnected || wasCellular != currentCellular || wasWifi != currentWifi {
                        NotificationCenter.default.post(name: .networkRoutingPreferenceChanged, object: nil)
                        Task {
                            await self.probeEndpoints()
                        }
                    }
                }
            }
        }
        pathMonitor.start(queue: monitorQueue)
        
        let initialPath = pathMonitor.currentPath
        let isSatisfied = (initialPath.status == .satisfied)
        let currentCellular = initialPath.isExpensive || initialPath.usesInterfaceType(.cellular)
        let currentWifi = initialPath.usesInterfaceType(.wifi)
        NetworkStatus.shared.isConnected = isSatisfied
        NetworkStatus.shared.isCellular = currentCellular
        if isSatisfied {
            NetworkStatus.shared.isWifi = currentWifi
            self.isWifi = currentWifi
        }
        self.isConnectedToNetwork = isSatisfied
        self.isCellular = currentCellular
    }
    
    /// Concurrently probes candidate endpoints and switches activeServerURL to the optimal one.
    /// Routing Order: LAN (skip if cellular) -> Custom (skip if LAN on cellular) -> Primary Cloud Domain (final fallback).
    @discardableResult
    public func probeEndpoints() async -> String? {
        guard !isProbing else { return AppSettings.shared.activeServerURL }
        
        let settings = AppSettings.shared
        let isCellularNow = self.isCellular || !self.isWifi
        
        var endpointsToTest: [String] = []
        
        let lan = settings.lanServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if !isCellularNow, let lan = lan, !lan.isEmpty {
            endpointsToTest.append(lan)
        }
        
        let custom = settings.customServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let custom = custom, !custom.isEmpty {
            let isLan = NetworkTransport.isLocalOrPrivateHost(custom)
            if !isCellularNow || !isLan {
                endpointsToTest.append(custom)
            }
        }
        
        let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let cloud = cloud, !cloud.isEmpty {
            let isLan = NetworkTransport.isLocalOrPrivateHost(cloud)
            if !isCellularNow || !isLan {
                endpointsToTest.append(cloud)
            }
        }
        
        if endpointsToTest.isEmpty, !settings.rawServerURL.isEmpty {
            let isLan = NetworkTransport.isLocalOrPrivateHost(settings.rawServerURL)
            if !isCellularNow || !isLan {
                endpointsToTest.append(settings.rawServerURL)
            }
        }
        
        guard !endpointsToTest.isEmpty else { return nil }
        
        isProbing = true
        defer {
            isProbing = false
            lastProbeDate = Date()
        }
        
        var results: [EndpointHealthStatus] = []
        await withTaskGroup(of: EndpointHealthStatus.self) { group in
            for ep in endpointsToTest {
                group.addTask {
                    await Self.testSingleEndpoint(urlString: ep)
                }
            }
            
            for await status in group {
                results.append(status)
            }
        }
        
        for res in results {
            endpointStatuses[res.urlString] = res
        }
        
        let reachable = results.filter { $0.isReachable }
        
        // Update gateway platform from reachable endpoints
        if let plat = reachable.first(where: { $0.platform != nil && !$0.platform!.isEmpty })?.platform {
            settings.gatewayPlatform = plat
        }
        
        // Self-heal: If primaryCloudURL is missing, discover it from /api/v1/auth/endpoints on reachable gateway
        if settings.primaryCloudURL == nil {
            for ep in reachable {
                let cleanBase = ep.urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                if let probeURL = URL(string: "\(cleanBase)/api/v1/auth/endpoints") {
                    var req = URLRequest(url: probeURL)
                    req.timeoutInterval = 3.0
                    if let (data, resp) = try? await NetworkTransport.shared.send(request: req),
                       let http = resp as? HTTPURLResponse, http.statusCode == 200,
                       let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                       let eps = json["endpoints"] as? [[String: Any]] {
                        for item in eps {
                            if let type = item["type"] as? String, type.lowercased() == "cloudflare",
                               let epUrl = item["url"] as? String, !NetworkTransport.isLocalOrPrivateHost(epUrl) {
                                settings.primaryCloudURL = epUrl
                                break
                            }
                        }
                    }
                    if settings.primaryCloudURL != nil {
                        break
                    }
                }
            }
        }
        
        var selected: String? = nil
        
        // 1. LAN (only if NOT cellular and reachable)
        if !isCellularNow, let lan = lan, !lan.isEmpty {
            let cleanLan = lan.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            if reachable.contains(where: { $0.urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) == cleanLan }) {
                selected = lan
            }
        }
        
        // 2. Custom (if configured and reachable, and not LAN on cellular)
        if selected == nil, let custom = custom, !custom.isEmpty {
            let isLan = NetworkTransport.isLocalOrPrivateHost(custom)
            let cleanCustom = custom.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            if (!isCellularNow || !isLan) && reachable.contains(where: { $0.urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) == cleanCustom }) {
                selected = custom
            }
        }
        
        // 3. Primary Cloud (Cloudflare tunnel HTTPS - ultimate fallback)
        if selected == nil, let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines), !cloud.isEmpty {
            let isLan = NetworkTransport.isLocalOrPrivateHost(cloud)
            if !isCellularNow || !isLan {
                let cleanCloud = cloud.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                selected = reachable.first(where: { $0.urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) == cleanCloud })?.urlString ?? cloud
            }
        }
        
        if let best = selected {
            let oldActive = settings.activeServerURL
            settings.activeServerURL = best
            if oldActive != best {
                NotificationCenter.default.post(name: .networkRoutingPreferenceChanged, object: nil)
            }
            return best
        }
        
        return settings.activeServerURL
    }
    
    /// Tests a single endpoint's reachability and latency with a 5.0s timeout to allow cellular TLS handshakes.
    public static func testSingleEndpoint(urlString: String) async -> EndpointHealthStatus {
        guard let baseURL = AppSettings.normalize(raw: urlString) else {
            return EndpointHealthStatus(
                id: urlString,
                urlString: urlString,
                isReachable: false,
                latencyMs: 0,
                errorMessage: "无效地址"
            )
        }
        
        var cleanBase = baseURL.absoluteString
        if cleanBase.hasSuffix("/") {
            cleanBase.removeLast()
        }
        guard let probeURL = URL(string: "\(cleanBase)/healthz") else {
            return EndpointHealthStatus(
                id: urlString,
                urlString: urlString,
                isReachable: false,
                latencyMs: 0,
                errorMessage: "无效探测地址"
            )
        }
        
        var request = URLRequest(url: probeURL)
        request.httpMethod = "GET"
        request.timeoutInterval = 5.0
        
        let start = CFAbsoluteTimeGetCurrent()
        do {
            let (data, response) = try await NetworkTransport.shared.send(request: request)
            let elapsedMs = (CFAbsoluteTimeGetCurrent() - start) * 1000.0
            
            guard let httpResponse = response as? HTTPURLResponse else {
                return EndpointHealthStatus(
                    id: urlString,
                    urlString: urlString,
                    isReachable: false,
                    latencyMs: elapsedMs,
                    errorMessage: "无效网络响应"
                )
            }
            
            // /healthz returns 200 when the gateway process is up.
            if httpResponse.statusCode == 200 {
                var platform: String? = nil
                if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    platform = (json["platform"] as? String) ?? (json["os"] as? String)
                }
                return EndpointHealthStatus(
                    id: urlString,
                    urlString: urlString,
                    isReachable: true,
                    latencyMs: elapsedMs,
                    errorMessage: nil,
                    platform: platform
                )
            } else {
                return EndpointHealthStatus(
                    id: urlString,
                    urlString: urlString,
                    isReachable: false,
                    latencyMs: elapsedMs,
                    errorMessage: "HTTP \(httpResponse.statusCode)"
                )
            }
        } catch {
            let elapsedMs = (CFAbsoluteTimeGetCurrent() - start) * 1000.0
            return EndpointHealthStatus(
                id: urlString,
                urlString: urlString,
                isReachable: false,
                latencyMs: elapsedMs,
                errorMessage: error.localizedDescription
            )
        }
    }
}
