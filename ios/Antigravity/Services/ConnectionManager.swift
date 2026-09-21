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
        //    LAN offers ~1ms latency, 0 data consumption, and avoids public internet routing.
        // 2. Remote / Out-of-Home:
        //    - If IPv6 is reachable (cellular 5G or Wi-Fi with IPv6), prefer IPv6 direct (~20ms).
        //    - Otherwise pick the reachable endpoint with lowest latency (e.g. Cloud Relay ~40ms).
        let reachable = results.filter { $0.isReachable }
        
        // Election policy:
        // Priority 1: Primary Cloudflare HTTPS Domain (default unified routing)
        // Priority 2: Custom / LAN endpoint fallback
        let cloudEp = reachable.first(where: { $0.urlString == settings.primaryCloudURL })
        let customEp = reachable.first(where: { $0.urlString == settings.customServerURL || $0.urlString == settings.lanServerURL })
        let selected = cloudEp ?? customEp ?? reachable.min(by: { $0.latencyMs < $1.latencyMs })
        
        if let best = selected {
            settings.activeServerURL = best.urlString
            settings.rawServerURL = best.urlString
            return best.urlString
        }
        
        return settings.activeServerURL
    }
    
    /// Tests a single endpoint's reachability and latency.
    public static func testSingleEndpoint(urlString: String) async -> EndpointHealthStatus {
        guard let baseURL = AppSettings.normalize(raw: urlString),
              let probeURL = URL(string: "\(baseURL.absoluteString)/healthz") else {
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
        request.timeoutInterval = 2.5
        
        let start = CFAbsoluteTimeGetCurrent()
        do {
            let (_, response) = try await NetworkTransport.shared.send(request: request)
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
            
            // /healthz returns 200 when the gateway process is up (no secrets).
            if httpResponse.statusCode == 200 {
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
