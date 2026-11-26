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
    public let gatewayId: String?
    
    public init(
        id: String,
        urlString: String,
        isReachable: Bool,
        latencyMs: Double,
        errorMessage: String? = nil,
        platform: String? = nil,
        gatewayId: String? = nil
    ) {
        self.id = id
        self.urlString = urlString
        self.isReachable = isReachable
        self.latencyMs = latencyMs
        self.errorMessage = errorMessage
        self.platform = platform
        self.gatewayId = gatewayId
    }
}

/// Thread-safe global network state container accessible from any queue/Sendable context
public final nonisolated class NetworkStatus: Sendable {
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
    private var inFlightProbeTask: Task<String?, Never>?
    private var inFlightLanProbeTask: Task<Bool, Never>?
    private var lastFailedLanProbeDate: Date? = nil
    private var lastGatewaySignature: String? = nil
    private var hasReceivedInitialPath: Bool = false

    private static let lanFastProbeFailureCooldown: TimeInterval = 30.0
    private static let lanProbeTimeout: TimeInterval = 1.2
    private static let publicProbeTimeout: TimeInterval = 5.0
    
    private init() {
        startMonitoring()
    }

    /// Explicitly initializes the singleton and ensures `NWPathMonitor` is running from app launch.
    public func ensureStarted() {
        // Accessing `ConnectionManager.shared` starts `pathMonitor` in `init()`.
    }

    /// Clears recent LAN probe failure cooldown when returning to foreground so walking back onto
    /// home Wi-Fi while suspended immediately re-evaluates LAN reachability.
    public func noteForegroundResume() {
        lastFailedLanProbeDate = nil
    }

    /// Cancels in-flight probes and clears cached endpoint health when switching between decoupled gateways.
    public func resetForGatewaySwitch() {
        inFlightProbeTask?.cancel()
        inFlightProbeTask = nil
        inFlightLanProbeTask?.cancel()
        inFlightLanProbeTask = nil
        endpointStatuses.removeAll()
        lastFailedLanProbeDate = nil
        isProbing = false
    }

    private static func normalizedEndpointKey(_ raw: String?) -> String? {
        guard let trimmed = raw?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased(),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private func startMonitoring() {
        pathMonitor.pathUpdateHandler = { [weak self] path in
            let isSatisfied = (path.status == .satisfied)
            let currentCellular = path.isExpensive || path.usesInterfaceType(.cellular)
            let currentWifi = path.usesInterfaceType(.wifi)
            let gatewaySig = path.gateways.map(\.debugDescription).sorted().joined(separator: ",")
            
            NetworkStatus.shared.isConnected = isSatisfied
            NetworkStatus.shared.isCellular = currentCellular
            NetworkStatus.shared.isWifi = currentWifi
            
            Task { @MainActor [weak self] in
                guard let self = self else { return }
                let wasCellular = self.isCellular
                let wasWifi = self.isWifi
                let wasConnected = self.isConnectedToNetwork
                let wasInitial = !self.hasReceivedInitialPath
                let gatewayChanged = (self.lastGatewaySignature != nil && self.lastGatewaySignature != gatewaySig)
                
                self.hasReceivedInitialPath = true
                self.isConnectedToNetwork = isSatisfied
                self.isCellular = currentCellular
                self.isWifi = currentWifi
                if isSatisfied {
                    self.lastGatewaySignature = gatewaySig
                }
                
                if !isSatisfied {
                    self.lastFailedLanProbeDate = nil
                    // Interface switching in progress (e.g. Wi-Fi severed, cellular acquiring)
                    self.networkLostTask?.cancel()
                    self.networkLostTask = Task { [weak self] in
                        try? await Task.sleep(nanoseconds: 500_000_000)
                        guard let self, !Task.isCancelled else { return }
                        guard AppSettings.shared.isPaired, !AppSettings.shared.isDemoMode else { return }
                        await self.probeEndpoints()
                    }
                } else {
                    self.networkLostTask?.cancel()
                    self.networkLostTask = nil
                    
                    let routeChanged = wasInitial || !wasConnected || wasCellular != currentCellular || wasWifi != currentWifi || gatewayChanged
                    if routeChanged {
                        self.lastFailedLanProbeDate = nil
                        let settings = AppSettings.shared
                        guard settings.isPaired, !settings.isDemoMode else { return }
                        // When leaving Wi-Fi for cellular while pinned to a LAN-only host, immediately
                        // fail over activeServerURL to a non-LAN route so live streams don't wait on probe.
                        if (currentCellular || !currentWifi),
                           let active = settings.activeServerURL,
                           NetworkTransport.isLanOnlyHost(active) {
                            let nonLanCustom = settings.customServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
                            if let custom = nonLanCustom, !custom.isEmpty, !NetworkTransport.isLanOnlyHost(custom) {
                                settings.activeServerURL = custom
                            } else if let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines),
                                      !cloud.isEmpty, !NetworkTransport.isLanOnlyHost(cloud) {
                                settings.activeServerURL = cloud
                            }
                        }
                        Task {
                            await self.probeEndpoints()
                        }
                    }
                }
            }
        }
        pathMonitor.start(queue: monitorQueue)
        
        let initialPath = pathMonitor.currentPath
        if initialPath.status == .satisfied {
            let currentCellular = initialPath.isExpensive || initialPath.usesInterfaceType(.cellular)
            let currentWifi = initialPath.usesInterfaceType(.wifi)
            NetworkStatus.shared.isConnected = true
            NetworkStatus.shared.isCellular = currentCellular
            NetworkStatus.shared.isWifi = currentWifi
            self.isConnectedToNetwork = true
            self.isCellular = currentCellular
            self.isWifi = currentWifi
        }
    }

    /// Ultra-fast LAN-only pre-probe on Wi-Fi when `activeServerURL` is not yet the LAN endpoint.
    /// Returns `true` if LAN answered HTTP 200 and `activeServerURL` was switched to LAN.
    @discardableResult
    public func probeLanFastIfNeeded(force: Bool = false, timeout: TimeInterval = 0.65) async -> Bool {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode else { return false }
        let isCellularNow = self.isCellular || !self.isWifi
        guard !isCellularNow else { return false }
        guard let lan = settings.lanServerURL?.trimmingCharacters(in: .whitespacesAndNewlines),
              !lan.isEmpty,
              let cleanLan = Self.normalizedEndpointKey(lan) else {
            return false
        }
        if Self.normalizedEndpointKey(settings.activeServerURL) == cleanLan {
            return false
        }
        if !force,
           let lastFail = lastFailedLanProbeDate,
           Date().timeIntervalSince(lastFail) < Self.lanFastProbeFailureCooldown {
            return false
        }
        if let existing = inFlightLanProbeTask {
            return await existing.value
        }

        let startEpoch = GatewayRuntimeState.shared.epoch
        let expectedGwId = GatewayRuntimeState.shared.activeGatewayId
        let task = Task<Bool, Never> { @MainActor [weak self] in
            guard let self else { return false }
            let status = await Self.testSingleEndpoint(urlString: lan, timeout: timeout, expectedGatewayId: expectedGwId)
            guard !Task.isCancelled, GatewayRuntimeState.shared.epoch == startEpoch else { return false }
            self.endpointStatuses[status.urlString] = status
            if status.isReachable {
                self.lastFailedLanProbeDate = nil
                self.lastProbeDate = Date()
                if let plat = status.platform, !plat.isEmpty {
                    settings.gatewayPlatform = plat
                }
                if settings.isPaired && !E2EECrypto.shared.isReady {
                    Task {
                        await E2EECrypto.shared.silentUpgradeIfNeeded(reachableBaseURL: lan, expectedEpoch: startEpoch)
                    }
                }
                let changed = Self.normalizedEndpointKey(settings.activeServerURL) != cleanLan
                settings.activeServerURL = lan
                return changed
            } else {
                self.lastFailedLanProbeDate = Date()
                return false
            }
        }
        inFlightLanProbeTask = task
        let switched = await task.value
        if inFlightLanProbeTask == task {
            inFlightLanProbeTask = nil
        }
        return switched
    }
    
    /// Concurrently probes candidate endpoints (single-flight) and switches activeServerURL to the optimal one.
    /// Routing Order: LAN (skip if cellular; short-circuits immediately on HTTP 200) -> Custom -> Primary Cloud Domain.
    @discardableResult
    public func probeEndpoints() async -> String? {
        let settings = AppSettings.shared
        if settings.isDemoMode {
            return DemoGateway.baseURL.absoluteString
        }
        if let existing = inFlightProbeTask {
            return await existing.value
        }
        let task = Task<String?, Never> { @MainActor [weak self] in
            guard let self else { return AppSettings.shared.activeServerURL }
            return await self.performProbeEndpoints()
        }
        inFlightProbeTask = task
        let result = await task.value
        if inFlightProbeTask == task {
            inFlightProbeTask = nil
        }
        return result
    }

    private func performProbeEndpoints() async -> String? {
        let startEpoch = GatewayRuntimeState.shared.epoch
        let expectedGwId = GatewayRuntimeState.shared.activeGatewayId
        let settings = AppSettings.shared
        let isCellularNow = self.isCellular || !self.isWifi
        
        var endpointsToTest: [String] = []
        
        let lan = settings.lanServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanLan = (!isCellularNow ? Self.normalizedEndpointKey(lan) : nil)
        if !isCellularNow, let lan = lan, !lan.isEmpty {
            endpointsToTest.append(lan)
        }
        
        let custom = settings.customServerURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let custom = custom, !custom.isEmpty {
            let isLan = NetworkTransport.isLanOnlyHost(custom)
            if !isCellularNow || !isLan {
                endpointsToTest.append(custom)
            }
        }
        
        let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let cloud = cloud, !cloud.isEmpty {
            let isLan = NetworkTransport.isLanOnlyHost(cloud)
            if !isCellularNow || !isLan {
                endpointsToTest.append(cloud)
            }
        }

        if endpointsToTest.isEmpty, !settings.rawServerURL.isEmpty {
            let isLan = NetworkTransport.isLanOnlyHost(settings.rawServerURL)
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
                    await Self.testSingleEndpoint(urlString: ep, expectedGatewayId: expectedGwId)
                }
            }
            
            for await status in group {
                results.append(status)
                // LAN short-circuit: LAN always wins on Wi-Fi, so elect immediately instead of
                // blocking up to 5s waiting for a slow or dead cloud endpoint to finish.
                if let cleanLan,
                   status.isReachable,
                   Self.normalizedEndpointKey(status.urlString) == cleanLan {
                    group.cancelAll()
                    break
                }
            }
        }
        
        for res in results {
            guard GatewayRuntimeState.shared.epoch == startEpoch else { return settings.activeServerURL }
            endpointStatuses[res.urlString] = res
        }
        guard !Task.isCancelled, GatewayRuntimeState.shared.epoch == startEpoch else {
            return settings.activeServerURL
        }

        if let cleanLan,
           let lanResult = results.first(where: { Self.normalizedEndpointKey($0.urlString) == cleanLan }) {
            lastFailedLanProbeDate = lanResult.isReachable ? nil : Date()
        }
        
        let reachable = results.filter { $0.isReachable }
        
        // Update gateway platform from reachable endpoints
        if let plat = reachable.first(where: { $0.platform != nil && !$0.platform!.isEmpty })?.platform {
            settings.gatewayPlatform = plat
        }

        // Silent legacy-to-E2EE key negotiation for pre-1.2.5 paired devices (zero user interaction)
        if settings.isPaired && !E2EECrypto.shared.isReady, let reachableBase = reachable.first?.urlString {
            Task {
                await E2EECrypto.shared.silentUpgradeIfNeeded(reachableBaseURL: reachableBase, expectedEpoch: startEpoch)
            }
        }

        // Stateless multi-VPS pool auto-discovery: if all known endpoints are unreachable (e.g. gateway failed over
        // or migrated to a newly added VPS in RELAY_POOL), probe /api/relay/pool and re-lock automatically.
        if reachable.isEmpty && settings.isPaired && E2EECrypto.shared.isReady,
           let currentCloud = settings.primaryCloudURL, !currentCloud.isEmpty {
            if let migratedURL = await RelayPoolResolver.shared.resolveReachableCloudURL(
                currentCloudURL: currentCloud,
                expectedEpoch: startEpoch,
                expectedGatewayId: expectedGwId
            ) {
                guard !Task.isCancelled, GatewayRuntimeState.shared.epoch == startEpoch else {
                    return settings.activeServerURL
                }
                settings.primaryCloudURL = migratedURL
                settings.syncWidgetConfiguration(reloadWidget: false)
                settings.activeServerURL = migratedURL
                return migratedURL
            }
        }
        
        // Self-heal: If primaryCloudURL is missing, discover it from /api/v1/auth/endpoints on reachable gateway
        if settings.primaryCloudURL == nil {
            for ep in reachable {
                guard !Task.isCancelled, GatewayRuntimeState.shared.epoch == startEpoch else {
                    return settings.activeServerURL
                }
                let cleanBase = ep.urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                if let probeURL = URL(string: "\(cleanBase)/api/v1/auth/endpoints") {
                    var req = URLRequest(url: probeURL)
                    req.timeoutInterval = 3.0
                    if let token = settings.deviceToken?.trimmingCharacters(in: .whitespacesAndNewlines), !token.isEmpty {
                        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                    }
                    if let (data, resp) = try? await NetworkTransport.shared.send(request: req),
                       GatewayRuntimeState.shared.epoch == startEpoch,
                       let http = resp as? HTTPURLResponse, http.statusCode == 200,
                       let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                       let eps = json["endpoints"] as? [[String: Any]] {
                        for item in eps {
                            if let type = item["type"] as? String,
                               (type.lowercased() == "cloudflare" || type.lowercased() == "relay"),
                               let rawUrl = item["url"] as? String {
                                let epUrl = rawUrl.trimmingCharacters(in: .whitespacesAndNewlines)
                                    .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                                if NetworkTransport.isValidAdvertisedCloudURL(epUrl) {
                                    settings.primaryCloudURL = epUrl
                                    settings.syncWidgetConfiguration(reloadWidget: false)
                                    break
                                }
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
        if !isCellularNow, let lan = lan, !lan.isEmpty, let cleanLan {
            if reachable.contains(where: { Self.normalizedEndpointKey($0.urlString) == cleanLan }) {
                selected = lan
            }
        }
        
        // 2. Custom (if configured and reachable; LAN-only hosts skipped on cellular, Tailscale allowed)
        if selected == nil, let custom = custom, !custom.isEmpty, let cleanCustom = Self.normalizedEndpointKey(custom) {
            let isLan = NetworkTransport.isLanOnlyHost(custom)
            if (!isCellularNow || !isLan) && reachable.contains(where: { Self.normalizedEndpointKey($0.urlString) == cleanCustom }) {
                selected = custom
            }
        }
        
        // 3. Primary Cloud (Cloudflare tunnel HTTPS - ultimate fallback)
        if selected == nil, let cloud = settings.primaryCloudURL?.trimmingCharacters(in: .whitespacesAndNewlines), !cloud.isEmpty {
            let isLan = NetworkTransport.isLanOnlyHost(cloud)
            if !isCellularNow || !isLan, let cleanCloud = Self.normalizedEndpointKey(cloud) {
                selected = reachable.first(where: { Self.normalizedEndpointKey($0.urlString) == cleanCloud })?.urlString ?? cloud
            }
        }
        
        if let best = selected {
            guard !Task.isCancelled, GatewayRuntimeState.shared.epoch == startEpoch else {
                return settings.activeServerURL
            }
            settings.activeServerURL = best
            return best
        }
        
        return settings.activeServerURL
    }
    
    /// Tests a single endpoint's reachability and latency.
    /// LAN-only endpoints default to a tight 1.2s timeout so off-home Wi-Fi fails fast;
    /// public/Tailscale endpoints keep a 5.0s timeout to allow cellular TLS handshakes.
    /// When `expectedGatewayId` is provided, rejects endpoints advertising a different gateway identity.
    public static func testSingleEndpoint(
        urlString: String,
        timeout: TimeInterval? = nil,
        expectedGatewayId: String? = nil
    ) async -> EndpointHealthStatus {
        guard let baseURL = AppSettings.normalize(raw: urlString) else {
            return EndpointHealthStatus(
                id: urlString,
                urlString: urlString,
                isReachable: false,
                latencyMs: 0,
                errorMessage: String(localized: "无效地址")
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
                errorMessage: String(localized: "无效探测地址")
            )
        }
        
        let effectiveTimeout = timeout ?? (NetworkTransport.isLanOnlyHost(urlString) ? lanProbeTimeout : publicProbeTimeout)
        var request = URLRequest(url: probeURL)
        request.httpMethod = "GET"
        request.cachePolicy = .reloadIgnoringLocalCacheData
        request.timeoutInterval = effectiveTimeout
        
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
                    errorMessage: String(localized: "无效网络响应")
                )
            }
            
            // /healthz returns 200 when the gateway process is up.
            if httpResponse.statusCode == 200 {
                var platform: String? = nil
                var gatewayId: String? = nil
                if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    platform = (json["platform"] as? String) ?? (json["os"] as? String)
                    let explicitGwId = (json["gateway_id"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines)
                    if let explicitGwId, !explicitGwId.isEmpty {
                        gatewayId = explicitGwId
                    } else if let pub = json["e2ee_pub"] as? String {
                        gatewayId = GatewayProfileStore.deriveGatewayId(fromE2EEPubKeyB64: pub)
                    }
                }
                if let expected = expectedGatewayId?.trimmingCharacters(in: .whitespacesAndNewlines),
                   !expected.isEmpty,
                   let actual = gatewayId,
                   !actual.isEmpty,
                   actual != expected {
                    return EndpointHealthStatus(
                        id: urlString,
                        urlString: urlString,
                        isReachable: false,
                        latencyMs: elapsedMs,
                        errorMessage: String(localized: "网关身份不匹配"),
                        platform: platform,
                        gatewayId: actual
                    )
                }
                return EndpointHealthStatus(
                    id: urlString,
                    urlString: urlString,
                    isReachable: true,
                    latencyMs: elapsedMs,
                    errorMessage: nil,
                    platform: platform,
                    gatewayId: gatewayId
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
