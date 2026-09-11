import Foundation
import Network
import Observation

public struct EndpointHealthStatus: Identifiable, Sendable {
    public let id: String
    public let urlString: String
    public let isReachable: Bool
    public let latencyMs: Double
    public let errorMessage: String?
}

@Observable
@MainActor
public final class ConnectionManager {
    public static let shared = ConnectionManager()
    
    public var isProbing: Bool = false
    public var lastProbeDate: Date? = nil
    public var endpointStatuses: [String: EndpointHealthStatus] = [:]
    public var isCellular: Bool = false
    public var isConnectedToNetwork: Bool = true
    
    private let pathMonitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "antigravity.connection_monitor", qos: .utility)
    
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
            Task { @MainActor [weak self] in
                guard let self = self else { return }
                let wasCellular = self.isCellular
                self.isConnectedToNetwork = (path.status == .satisfied)
                self.isCellular = path.isExpensive || path.usesInterfaceType(.cellular)
                
                // If network interface changed, automatically trigger background endpoint probe
                if wasCellular != self.isCellular && self.isConnectedToNetwork {
                    Task {
                        await self.probeEndpoints()
                    }
                }
            }
        }
        pathMonitor.start(queue: monitorQueue)
    }
    
    /// Concurrently probes all candidate endpoints and switches activeServerURL to the optimal one.
    @discardableResult
    public func probeEndpoints() async -> String? {
        guard !isProbing else { return AppSettings.shared.activeServerURL }
        
        let settings = AppSettings.shared
        let candidates = settings.candidateEndpoints
        guard !candidates.isEmpty else { return nil }
        
        isProbing = true
        defer {
            isProbing = false
            lastProbeDate = Date()
        }
        
        var results: [EndpointHealthStatus] = []
        
        await withTaskGroup(of: EndpointHealthStatus.self) { group in
            for ep in candidates {
                group.addTask {
                    await Self.testSingleEndpoint(urlString: ep.urlString)
                }
            }
            
            for await status in group {
                results.append(status)
            }
        }
        
        // Update statuses map
        for res in results {
            endpointStatuses[res.urlString] = res
        }
        
        // Election policy:
        // 1. LAN IPv4 First: If on Wi-Fi and LAN is reachable (e.g. 192.168.x.x), ALWAYS prefer LAN.
        //    LAN offers ~1ms latency, 0 cellular data, and works locally without going through the public internet.
        // 2. Remote / Out-of-Home: If LAN is unreachable (e.g. on external Wi-Fi, or cellular 5G),
        //    prioritize IPv6 or custom public DDNS when on cellular or preferring cellular direct mode.
        // 3. Otherwise pick the reachable endpoint with the lowest latency.
        let reachable = results.filter { $0.isReachable }
        
        var selected: EndpointHealthStatus? = nil
        let lanEp = reachable.first(where: { ep in
            let clean = ep.urlString.lowercased()
            return clean.contains("192.168.") || clean.contains("10.") || clean.contains("172.")
        })
        
        if !isCellular, let lan = lanEp {
            // If on Wi-Fi and home LAN is reachable, ALWAYS select LAN (fastest ~1ms)
            selected = lan
        } else if isCellular || settings.preferCellularNetwork {
            // If on Cellular, or on external Wi-Fi (LAN failed) with cellular preference, prefer IPv6
            if let v6Ep = reachable.first(where: { ep in
                let clean = ep.urlString.lowercased()
                return clean.contains("[") || clean.contains("::") || (!clean.contains("192.168.") && !clean.contains("10.") && !clean.contains("127."))
            }) {
                selected = v6Ep
            }
        }
        
        if selected == nil {
            selected = reachable.min(by: { $0.latencyMs < $1.latencyMs })
        }
        
        if let best = selected {
            settings.activeServerURL = best.urlString
            settings.rawServerURL = best.urlString
            return best.urlString
        } else if (isCellular || settings.preferCellularNetwork), let v6 = settings.ipv6ServerURL, !v6.isEmpty {
            // If on cellular or preferring cellular direct connection, ensure IPv6 remains active endpoint
            settings.activeServerURL = v6
            settings.rawServerURL = v6
            return v6
        }
        
        return settings.activeServerURL
    }
    
    /// Tests a single endpoint's reachability and latency.
    public static func testSingleEndpoint(urlString: String) async -> EndpointHealthStatus {
        guard let baseURL = AppSettings.normalize(raw: urlString),
              let probeURL = URL(string: "\(baseURL.absoluteString)/gateway/status") else {
            return EndpointHealthStatus(
                id: urlString,
                urlString: urlString,
                isReachable: false,
                latencyMs: 0,
                errorMessage: "无效地址"
            )
        }
        
        var request = URLRequest(url: probeURL)
        request.httpMethod = "GET"
        request.timeoutInterval = 2.5 // Bound probe timeout so dead routes fail quickly
        
        let start = CFAbsoluteTimeGetCurrent()
        do {
            // When probing, test candidate reachability over current system route without artificial interface penalties
            let (_, response) = try await NetworkTransport.shared.send(
                request: request,
                preferCellular: false
            )
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
            
            // Status 200 or 401 (auth required) both confirm the gateway is alive
            if httpResponse.statusCode == 200 || httpResponse.statusCode == 401 {
                return EndpointHealthStatus(
                    id: urlString,
                    urlString: urlString,
                    isReachable: true,
                    latencyMs: elapsedMs,
                    errorMessage: nil
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
