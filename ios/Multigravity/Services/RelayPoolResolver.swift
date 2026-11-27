import Foundation
import os

/// Stateless mobile auto-discovery for multi-VPS relay pools.
/// When a paired gateway fails over or migrates to a newly added VPS node in `RELAY_POOL`,
/// the client queries the public read-only pool manifest (`GET /api/relay/pool`), expands
/// the user's deterministic subdomain ID (`r-{sid}.*`), and concurrently probes `/healthz`
/// to re-lock onto the active VPS with zero app updates or user intervention.
public final class RelayPoolResolver: Sendable {
    public static let shared = RelayPoolResolver()

    public static let defaultPoolURL = URL(string: "https://dispatcher.multigravity.app/api/relay/pool")!
    public static let legacyPoolURL = URL(string: "https://dispatcher.jiuge.space/api/relay/pool")!

    public struct PoolNode: Decodable, Sendable {
        public let id: String
        public let region: String
        public let priority: Int?
        public let host_template: String
    }

    private struct PoolManifest: Decodable, Sendable {
        let pool_version: Int?
        let nodes: [PoolNode]?
    }

    private struct CacheState {
        var nodes: [PoolNode] = []
        var fetchedAt: Date = .distantPast
        var trustedSuffixes: Set<String> = []
    }

    private let cacheLock = OSAllocatedUnfairLock(initialState: CacheState())
    private let inFlightResolveLock = OSAllocatedUnfairLock<[String: Task<String?, Never>]>(initialState: [:])

    private init() {}

    /// Extracts the deterministic 8-hex machine subdomain ID (`sid`) from a cloud URL,
    /// supporting both relay (`https://r-a1b2c3d4.jiuge.space`) and legacy CF (`https://a1b2c3d4.jiuge.space`).
    public static func extractSID(from cloudURL: String?) -> String? {
        guard let raw = cloudURL?.trimmingCharacters(in: .whitespacesAndNewlines),
              !raw.isEmpty,
              let url = URL(string: raw),
              let host = url.host?.lowercased(),
              let firstLabel = host.split(separator: ".").first.map(String.init) else {
            return nil
        }
        let candidate = firstLabel.hasPrefix("r-") ? String(firstLabel.dropFirst(2)) : firstLabel
        guard candidate.count >= 6, candidate.count <= 16,
              candidate.allSatisfy({ $0.isHexDigit }) else {
            return nil
        }
        return candidate
    }

    /// Returns true if `host` belongs to a domain suffix published by the official relay pool manifest.
    public func isPoolDomainHost(_ host: String) -> Bool {
        let clean = NetworkTransport.extractHost(from: host)
        guard !clean.isEmpty else { return false }
        let suffixes = cacheLock.withLock { $0.trustedSuffixes }
        return suffixes.contains { clean.hasSuffix($0) }
    }

    /// Returns true if `hostOrURL` belongs to the official managed relay pool (`.multigravity.app`,
    /// `.jiuge.space`, or a suffix advertised by the official pool manifest). Custom self-hosted
    /// BYO-Relay domains return false so clients never phone home to the official dispatcher.
    public func isOfficialOrPoolHost(_ hostOrURL: String) -> Bool {
        let clean = NetworkTransport.extractHost(from: hostOrURL)
        guard !clean.isEmpty else { return false }
        if clean.hasSuffix(".multigravity.app") || clean == "multigravity.app" ||
           clean.hasSuffix(".jiuge.space") || clean == "jiuge.space" {
            return true
        }
        return isPoolDomainHost(clean)
    }

    /// Fetches (or returns 60s-cached) pool node templates from the dispatcher Worker,
    /// falling back to the legacy dispatcher domain if the primary domain is unreachable.
    public func fetchPoolNodes(forceRefresh: Bool = false) async -> [PoolNode] {
        let cached = cacheLock.withLock { state -> [PoolNode]? in
            if !forceRefresh && !state.nodes.isEmpty && Date().timeIntervalSince(state.fetchedAt) < 60 {
                return state.nodes
            }
            return nil
        }
        if let cached { return cached }

        for endpoint in [Self.defaultPoolURL, Self.legacyPoolURL] {
            var req = URLRequest(url: endpoint)
            req.httpMethod = "GET"
            req.timeoutInterval = 4.0

            do {
                let (data, resp) = try await NetworkTransport.shared.send(request: req)
                guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else {
                    continue
                }
                let manifest = try JSONDecoder().decode(PoolManifest.self, from: data)
                let nodes = manifest.nodes ?? []
                guard !nodes.isEmpty else { continue }
                var suffixes = Set<String>()
                for node in nodes {
                    if let dotIdx = node.host_template.firstIndex(of: ".") {
                        let suffix = String(node.host_template[dotIdx...]).lowercased()
                        if suffix.count > 2 {
                            suffixes.insert(suffix)
                        }
                    }
                }
                cacheLock.withLock { state in
                    state.nodes = nodes
                    state.fetchedAt = Date()
                    state.trustedSuffixes.formUnion(suffixes)
                }
                return nodes
            } catch {
                continue
            }
        }
        return cacheLock.withLock { $0.nodes }
    }

    /// Discovers which VPS node in `RELAY_POOL` currently serves this paired gateway.
    /// Concurrent callers for the same failed URL share one in-flight probe (single-flight).
    /// Returns the lowest-latency reachable HTTPS base URL, or nil if no pool node is online.
    public func resolveReachableCloudURL(
        currentCloudURL: String?,
        expectedEpoch: UInt64? = nil,
        expectedGatewayId: String? = nil
    ) async -> String? {
        let reqEpoch = expectedEpoch ?? GatewayRuntimeState.shared.epoch
        let targetGwId = expectedGatewayId ?? GatewayRuntimeState.shared.activeGatewayId
        guard E2EECrypto.shared.isReady,
              GatewayRuntimeState.shared.epoch == reqEpoch,
              let raw = currentCloudURL,
              isOfficialOrPoolHost(raw),
              Self.extractSID(from: currentCloudURL) != nil else {
            return nil
        }
        let key = (currentCloudURL ?? "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased() + "|e:\(reqEpoch)"

        let task: Task<String?, Never> = inFlightResolveLock.withLock { map in
            if let existing = map[key] {
                return existing
            }
            let created = Task<String?, Never> { [self] in
                await self.performResolveReachableCloudURL(
                    currentCloudURL: currentCloudURL,
                    expectedEpoch: reqEpoch,
                    expectedGatewayId: targetGwId
                )
            }
            map[key] = created
            return created
        }

        let result = await task.value
        inFlightResolveLock.withLock { map in
            if map[key] == task {
                map.removeValue(forKey: key)
            }
        }
        return result
    }

    /// Discovers an active peer VPS relay node when `failedURL` returns HTTP 502/503 (`gateway_offline`),
    /// updates `AppSettings` active/primary cloud URLs, and returns the new base URL if migration succeeded.
    public func migrateRelayHostIfNeeded(
        failedURL: URL,
        expectedEpoch: UInt64? = nil
    ) async -> URL? {
        let reqEpoch = expectedEpoch ?? GatewayRuntimeState.shared.epoch
        guard GatewayRuntimeState.shared.epoch == reqEpoch else {
            return nil
        }
        guard let scheme = failedURL.scheme?.lowercased(), scheme == "https",
              let failedHost = failedURL.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased(),
              !failedHost.isEmpty,
              !NetworkTransport.isLocalOrPrivateHost(failedHost) else {
            return nil
        }
        let portSuffix: String
        if let port = failedURL.port, port != 443 {
            portSuffix = ":\(port)"
        } else {
            portSuffix = ""
        }
        let failedOrigin = "https://\(failedHost)\(portSuffix)"
        guard let migratedStr = await resolveReachableCloudURL(
                currentCloudURL: failedOrigin,
                expectedEpoch: reqEpoch,
                expectedGatewayId: GatewayRuntimeState.shared.activeGatewayId
              ),
              GatewayRuntimeState.shared.epoch == reqEpoch,
              let migratedURL = URL(string: migratedStr),
              let migratedHost = migratedURL.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased(),
              !migratedHost.isEmpty,
              migratedHost != failedHost else {
            return nil
        }

        E2EECrypto.shared.markHostRequiresE2EE(migratedHost)
        let applied: Bool = await MainActor.run {
            guard GatewayRuntimeState.shared.epoch == reqEpoch else { return false }
            let settings = AppSettings.shared
            let cleanMigrated = migratedStr.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            settings.primaryCloudURL = cleanMigrated
            settings.activeServerURL = cleanMigrated
            settings.syncWidgetConfiguration(reloadWidget: false)
            return true
        }
        return applied ? migratedURL : nil
    }

    private func performResolveReachableCloudURL(
        currentCloudURL: String?,
        expectedEpoch: UInt64,
        expectedGatewayId: String?
    ) async -> String? {
        guard E2EECrypto.shared.isReady,
              GatewayRuntimeState.shared.epoch == expectedEpoch,
              let raw = currentCloudURL,
              isOfficialOrPoolHost(raw),
              let sid = Self.extractSID(from: currentCloudURL) else {
            return nil
        }
        let currentClean = currentCloudURL?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased()

        let nodes = await fetchPoolNodes(forceRefresh: true)
        guard !nodes.isEmpty, GatewayRuntimeState.shared.epoch == expectedEpoch else { return nil }

        var candidateURLs: [String] = []
        var seen = Set<String>()
        for node in nodes {
            let host = node.host_template
                .replacingOccurrences(of: "{sid}", with: sid)
                .trimmingCharacters(in: .whitespacesAndNewlines)
                .lowercased()
            guard !host.isEmpty, host.contains(".") else { continue }
            let urlStr = "https://\(host)"
            if urlStr == currentClean { continue }
            if seen.insert(urlStr).inserted {
                candidateURLs.append(urlStr)
            }
        }
        guard !candidateURLs.isEmpty else { return nil }

        var statuses: [EndpointHealthStatus] = []
        await withTaskGroup(of: EndpointHealthStatus.self) { group in
            for cand in candidateURLs {
                group.addTask {
                    await ConnectionManager.testSingleEndpoint(
                        urlString: cand,
                        expectedGatewayId: expectedGatewayId
                    )
                }
            }
            for await status in group {
                statuses.append(status)
            }
        }

        guard GatewayRuntimeState.shared.epoch == expectedEpoch else { return nil }
        let best = statuses
            .filter { $0.isReachable }
            .min(by: { $0.latencyMs < $1.latencyMs })
        if let best {
            if let host = URL(string: best.urlString)?.host {
                E2EECrypto.shared.markHostRequiresE2EE(host)
            }
            return best.urlString
        }
        return nil
    }
}
