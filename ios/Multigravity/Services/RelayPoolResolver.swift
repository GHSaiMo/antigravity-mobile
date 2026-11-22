import Foundation
import os

/// Stateless mobile auto-discovery for multi-VPS relay pools.
/// When a paired gateway fails over or migrates to a newly added VPS node in `RELAY_POOL`,
/// the client queries the public read-only pool manifest (`GET /api/relay/pool`), expands
/// the user's deterministic subdomain ID (`r-{sid}.*`), and concurrently probes `/healthz`
/// to re-lock onto the active VPS with zero app updates or user intervention.
public final class RelayPoolResolver: Sendable {
    public static let shared = RelayPoolResolver()

    public static let defaultPoolURL = URL(string: "https://dispatcher.jiuge.space/api/relay/pool")!

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

    /// Fetches (or returns 60s-cached) pool node templates from the dispatcher Worker.
    public func fetchPoolNodes(forceRefresh: Bool = false) async -> [PoolNode] {
        let cached = cacheLock.withLock { state -> [PoolNode]? in
            if !forceRefresh && !state.nodes.isEmpty && Date().timeIntervalSince(state.fetchedAt) < 60 {
                return state.nodes
            }
            return nil
        }
        if let cached { return cached }

        var req = URLRequest(url: Self.defaultPoolURL)
        req.httpMethod = "GET"
        req.timeoutInterval = 4.0

        do {
            let (data, resp) = try await NetworkTransport.shared.send(request: req)
            guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else {
                return cacheLock.withLock { $0.nodes }
            }
            let manifest = try JSONDecoder().decode(PoolManifest.self, from: data)
            let nodes = manifest.nodes ?? []
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
            return cacheLock.withLock { $0.nodes }
        }
    }

    /// Discovers which VPS node in `RELAY_POOL` currently serves this paired gateway.
    /// Returns the lowest-latency reachable HTTPS base URL, or nil if no pool node is online.
    public func resolveReachableCloudURL(currentCloudURL: String?) async -> String? {
        guard E2EECrypto.shared.isReady,
              let sid = Self.extractSID(from: currentCloudURL) else {
            return nil
        }
        let currentClean = currentCloudURL?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased()

        let nodes = await fetchPoolNodes(forceRefresh: true)
        guard !nodes.isEmpty else { return nil }

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
                    await ConnectionManager.testSingleEndpoint(urlString: cand)
                }
            }
            for await status in group {
                statuses.append(status)
            }
        }

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
