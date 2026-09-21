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
    
    /// Concurrently probes candidate endpoints and switches activeServerURL to the optimal one.
    /// Routing Order: LAN (skip if cellular) -> Custom (skip if not configured) -> Primary Cloud Domain (final fallback).
    @discardableResult
    public func probeEndpoints() async -> String? {
        guard !isProbing else { return AppSettings.shared.activeServerURL }
        
        let settings = AppSettings.shared
        let isCellularNow = NetworkTransport.shared.isCellular
        
        // 1. Filter endpoints to test based on network interface and configuration
        var endpointsToTest: [String] = []
        
        let lan = settings.lanServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if !isCellularNow, let lan = lan, !lan.isEmpty {
            endpointsToTest.append(lan)
        }
        
        let custom = settings.customServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let custom = custom, !custom.isEmpty {
            endpointsToTest.append(custom)
        }
        
        let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let cloud = cloud, !cloud.isEmpty {
            endpointsToTest.append(cloud)
        }
        
        if endpointsToTest.isEmpty, !settings.rawServerURL.isEmpty {
            endpointsToTest.append(settings.rawServerURL)
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
        
        // Update statuses map
        for res in results {
            endpointStatuses[res.urlString] = res
        }
        
        let reachable = results.filter { $0.isReachable }
        
        // Smart Routing Selection:
        // 1. LAN: if NOT cellular and reachable -> select LAN
        // 2. Custom: if configured and reachable -> select Custom
        // 3. Primary Cloud: final fallback -> select Cloud
        var selected: String? = nil
        
        // 1. 局域网（非蜂窝网络且在线）
        if !isCellularNow, let lan = lan, !lan.isEmpty {
            let lanNorm = AppSettings.normalize(raw: lan)?.absoluteString
            if let ep = reachable.first(where: {
                let epNorm = AppSettings.normalize(raw: $0.urlString)?.absoluteString
                return epNorm == lanNorm || $0.urlString == lan
            }) {
                selected = ep.urlString
            }
        }
        
        // 2. 自定义（已配置且在线）
        if selected == nil, let custom = custom, !custom.isEmpty {
            let customNorm = AppSettings.normalize(raw: custom)?.absoluteString
            if let ep = reachable.first(where: {
                let epNorm = AppSettings.normalize(raw: $0.urlString)?.absoluteString
                return epNorm == customNorm || $0.urlString == custom
            }) {
                selected = ep.urlString
            }
        }
        
        // 3. 主域名（最终兜底）
        if selected == nil, let cloud = cloud, !cloud.isEmpty {
            let cloudNorm = AppSettings.normalize(raw: cloud)?.absoluteString
            if let ep = reachable.first(where: {
                let epNorm = AppSettings.normalize(raw: $0.urlString)?.absoluteString
                return epNorm == cloudNorm || $0.urlString == cloud
            }) {
                selected = ep.urlString
            } else {
                selected = cloudNorm ?? cloud
            }
        }
        
        if let best = selected {
            settings.activeServerURL = best
            settings.rawServerURL = best
            return best
        }
        
        return settings.activeServerURL
    }
    
    /// Tests a single endpoint's reachability and latency.
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
