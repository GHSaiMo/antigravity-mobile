import Foundation
import Network
import os
import Security

extension Notification.Name {
    public static let deviceTokenRevoked = Notification.Name("antigravity.device_token_revoked")
}

public enum NetworkTransportError: Error, LocalizedError, Sendable {
    case requestAlreadyDispatched(any Error)
    
    public var errorDescription: String? {
        switch self {
        case .requestAlreadyDispatched(let err):
            return String(localized: "指令已成功送达服务器，但等待响应超时 (\(err.localizedDescription))")
        }
    }
}

public final class NetworkTransport: Sendable {
    public static let shared = NetworkTransport()
    
    private let fallbackSession: URLSession
    
    public var isCellular: Bool {
        NetworkStatus.shared.isCellular
    }
    
    public var isWifi: Bool {
        NetworkStatus.shared.isWifi
    }
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 180
        config.httpShouldSetCookies = false
        config.httpCookieAcceptPolicy = .never
        config.httpCookieStorage = nil
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        self.fallbackSession = URLSession(configuration: config, delegate: nil, delegateQueue: nil)
    }
    
    /// Decorates HTTPURLResponse with X-Antigravity-Interface header to indicate actual interface used
    private func decorateResponse(
        _ response: URLResponse,
        url: URL?,
        forcedCellular: Bool = false,
        trustedByE2EE: Bool = false,
        requestEpoch: UInt64? = nil
    ) -> URLResponse {
        guard let http = response as? HTTPURLResponse, let targetURL = url ?? http.url else {
            return response
        }
        let expectedEpoch = requestEpoch ?? GatewayRuntimeState.shared.epoch
        let epochMatches = (GatewayRuntimeState.shared.epoch == expectedEpoch)
        let isStaleInactiveOrigin = GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(targetURL)
        if epochMatches && !isStaleInactiveOrigin {
            E2EECrypto.shared.observeServerCapabilities(from: http, url: targetURL)
        }
        // Self-heal primary Cloud URL only when advertised by an authenticated, configured/local gateway over HTTPS
        // AND the active gateway epoch has not changed since this request started AND neither origin belongs to an inactive profile.
        if epochMatches,
           !isStaleInactiveOrigin,
           let rawCloudHeader = http.value(forHTTPHeaderField: "X-Antigravity-Cloud-URL")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !rawCloudHeader.isEmpty {
            let cloudHeader = rawCloudHeader.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            if !cloudHeader.isEmpty,
               !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(URL(string: cloudHeader)),
               (trustedByE2EE || Self.isConfiguredOrLocalGatewayHost(targetURL.host ?? "")),
               Self.isValidAdvertisedCloudURL(cloudHeader, trustedByE2EE: trustedByE2EE) {
                if let host = URL(string: cloudHeader)?.host, host.lowercased().hasPrefix("r-") {
                    E2EECrypto.shared.markHostRequiresE2EE(host)
                }
                Task { @MainActor in
                    guard GatewayRuntimeState.shared.epoch == expectedEpoch else { return }
                    guard !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(targetURL),
                          !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(URL(string: cloudHeader)) else {
                        return
                    }
                    let current = AppSettings.shared.primaryCloudURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                    if current != cloudHeader {
                        AppSettings.shared.primaryCloudURL = cloudHeader
                        AppSettings.shared.syncWidgetConfiguration(reloadWidget: false)
                    }
                }
            }
        }
        
        var fields: [String: String] = [:]
        for (k, v) in http.allHeaderFields {
            fields["\(k)"] = "\(v)"
        }
        // If low-level NWConnection pinned cellular was used, or phone is on cellular network without Wi-Fi
        let usedCellular = forcedCellular || (isCellular && !isWifi)
        fields["X-Antigravity-Interface"] = usedCellular ? "cellular" : "wifi"
        return HTTPURLResponse(
            url: targetURL,
            statusCode: http.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: fields
        ) ?? response
    }
    
    nonisolated public static func extractHost(from raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return "" }
        if let url = URL(string: trimmed.contains("://") ? trimmed : "http://\(trimmed)"),
           let host = url.host {
            return host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        }
        var clean = trimmed
        if let schemeRange = clean.range(of: "://") {
            clean = String(clean[schemeRange.upperBound...])
        }
        if let slashIdx = clean.firstIndex(of: "/") {
            clean = String(clean[..<slashIdx])
        }
        if clean.hasPrefix("["), let end = clean.firstIndex(of: "]") {
            return String(clean[clean.index(after: clean.startIndex)..<end]).lowercased()
        }
        if clean.filter({ $0 == ":" }).count == 1, let colonIdx = clean.lastIndex(of: ":") {
            let port = clean[clean.index(after: colonIdx)...]
            if !port.isEmpty && port.allSatisfy({ $0.isNumber }) {
                clean = String(clean[..<colonIdx])
            }
        }
        return clean.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
    }
    
    nonisolated private static func isValidIPv6Chars(_ value: String) -> Bool {
        guard !value.isEmpty else { return false }
        return value.unicodeScalars.allSatisfy { s in
            (s.value >= 0x30 && s.value <= 0x39) || // 0-9
            (s.value >= 0x61 && s.value <= 0x66) || // a-f
            s.value == 0x3A || s.value == 0x2E      // : .
        }
    }

    nonisolated private static func isTailscaleIPv6(_ clean: String) -> Bool {
        let noZone = clean.split(separator: "%", maxSplits: 1).first.map(String.init) ?? clean
        return noZone.hasPrefix("fd7a:115c:a1e0:") && isValidIPv6Chars(noZone)
    }

    nonisolated private static func isPrivateLanIPv6(_ clean: String) -> Bool {
        let noZone = clean.split(separator: "%", maxSplits: 1).first.map(String.init) ?? clean
        guard noZone.contains(":"), !isTailscaleIPv6(noZone), isValidIPv6Chars(noZone) else { return false }
        guard let firstGroup = noZone.split(separator: ":", maxSplits: 1, omittingEmptySubsequences: false).first,
              !firstGroup.isEmpty, firstGroup.count <= 4,
              let firstHextet = UInt16(firstGroup, radix: 16) else { return false }
        // ULA fc00::/7 or Link-Local fe80::/10
        return (firstHextet & 0xFE00) == 0xFC00 || (firstHextet & 0xFFC0) == 0xFE80
    }

    nonisolated public static func isLocalOrPrivateHost(_ host: String) -> Bool {
        let clean = extractHost(from: host)
        if clean.isEmpty { return false }
        if clean == "127.0.0.1" || clean == "localhost" || clean == "::1" || clean.hasSuffix(".local") {
            return true
        }
        // IPv4 private ranges (10.0.0.0/8, 127.0.0.0/8, 192.168.0.0/16)
        if clean.hasPrefix("192.168.") || clean.hasPrefix("10.") || clean.hasPrefix("127.") {
            return true
        }
        // Class B private range (172.16.0.0/12)
        if clean.hasPrefix("172.") {
            let parts = clean.split(separator: ".")
            if parts.count >= 2, let second = Int(parts[1]), second >= 16 && second <= 31 {
                return true
            }
        }
        // IPv6 ULA (fc00::/7) and Link-Local (fe80::/10)
        if isPrivateLanIPv6(clean) {
            return true
        }
        // Tailscale CGNAT range (100.64.0.0/10), Tailscale IPv6 (fd7a:115c:a1e0::/48), or MagicDNS (*.ts.net)
        if isTailscaleHost(clean) {
            return true
        }
        return false
    }

    /// Tailscale virtual IPs / MagicDNS hosts: reachable from ANY network (Wi-Fi or cellular)
    /// because the WireGuard tunnel stays up across interface changes.
    nonisolated public static func isTailscaleHost(_ host: String) -> Bool {
        let clean = extractHost(from: host)
        if clean.isEmpty { return false }
        if clean == "ts.net" || clean.hasSuffix(".ts.net") { return true }
        // Tailscale CGNAT range 100.64.0.0/10
        let parts = clean.split(separator: ".")
        if parts.count == 4,
           let first = Int(parts[0]), first == 100,
           let second = Int(parts[1]), second >= 64, second <= 127,
           let third = Int(parts[2]), third >= 0, third <= 255,
           let fourth = Int(parts[3]), fourth >= 0, fourth <= 255 {
            return true
        }
        if isTailscaleIPv6(clean) { return true }
        return false
    }

    /// Hosts reachable ONLY on the same physical LAN (RFC1918 / loopback / .local),
    /// excluding Tailscale virtual addresses which work on cellular too.
    nonisolated public static func isLanOnlyHost(_ host: String) -> Bool {
        isLocalOrPrivateHost(host) && !isTailscaleHost(host)
    }

    /// Determines whether a request mutates server state and must never be silently retried on timeout/failure
    nonisolated public static func isNonIdempotentRequest(method: String, path: String) -> Bool {
        let m = method.uppercased()
        guard m == "POST" || m == "PUT" || m == "DELETE" || m == "PATCH" else {
            return false
        }
        if path.contains("SendUserCascadeMessage") ||
           path.contains("gateway/cascade/new") ||
           path.contains("gateway/interaction/submit") {
            return true
        }
        return false
    }

    nonisolated public static func configuredGatewayHosts() -> Set<String> {
        let keys = [
            "antigravity.server_url",
            "antigravity.primary_cloud_url",
            "antigravity.lan_server_url",
            "antigravity.relay_server_url",
            "antigravity.custom_server_url",
            "antigravity.active_server_url"
        ]
        var hosts = Set<String>()
        let defaults = UserDefaults.standard
        for key in keys {
            if let raw = defaults.string(forKey: key) {
                let h = extractHost(from: raw)
                if !h.isEmpty {
                    hosts.insert(h)
                }
            }
        }
        return hosts
    }

    nonisolated public static func configuredGatewayOrigins() -> Set<String> {
        let keys = [
            "antigravity.server_url",
            "antigravity.primary_cloud_url",
            "antigravity.lan_server_url",
            "antigravity.relay_server_url",
            "antigravity.custom_server_url",
            "antigravity.active_server_url"
        ]
        var origins = Set<String>()
        let defaults = UserDefaults.standard
        for key in keys {
            guard let raw = defaults.string(forKey: key)?.trimmingCharacters(in: .whitespacesAndNewlines),
                  !raw.isEmpty,
                  let parsed = URL(string: raw),
                  let scheme = parsed.scheme?.lowercased(),
                  (scheme == "http" || scheme == "https") else {
                continue
            }
            let host = (parsed.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
            guard !host.isEmpty else { continue }
            let port = parsed.port ?? (scheme == "https" ? 443 : 80)
            origins.insert("\(scheme)://\(host):\(port)")
        }
        return origins
    }

    nonisolated public static func isConfiguredGatewayOrigin(_ url: URL?) -> Bool {
        guard let url,
              let scheme = url.scheme?.lowercased(),
              (scheme == "http" || scheme == "https") else {
            return false
        }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        guard !host.isEmpty else { return false }
        if host == "demo.antigravity.local" {
            return true
        }
        let port = url.port ?? (scheme == "https" ? 443 : 80)
        return configuredGatewayOrigins().contains("\(scheme)://\(host):\(port)")
    }

    nonisolated public static func isConfiguredOrLocalGatewayHost(_ host: String) -> Bool {
        let clean = extractHost(from: host)
        if clean.isEmpty { return false }
        if clean == "demo.antigravity.local" {
            return true
        }
        return configuredGatewayHosts().contains(clean)
    }

    nonisolated public static func isValidAdvertisedCloudURL(_ raw: String, trustedByE2EE: Bool = false) -> Bool {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: trimmed),
              url.scheme?.lowercased() == "https",
              url.user == nil,
              url.password == nil,
              let host = url.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased(),
              !host.isEmpty,
              !isLocalOrPrivateHost(host) else {
            return false
        }
        // Authenticated inside our own gateway's AES-256-GCM E2EE envelope or listed in dispatcher pool manifest:
        // safely allows future VPS pool nodes on any HTTPS domain without requiring an app update.
        if trustedByE2EE || RelayPoolResolver.shared.isPoolDomainHost(host) {
            return true
        }
        if host.hasSuffix(".trycloudflare.com") || host.hasSuffix(".multigravity.app") || host.hasSuffix(".jiuge.space") {
            return true
        }
        let configured = configuredGatewayHosts()
        if configured.contains(host) {
            return true
        }
        // Allow sibling subdomains under an already-paired custom domain (e.g. a.example.com -> b.example.com)
        let parts = host.split(separator: ".")
        if parts.count >= 3 {
            let parentSuffix = "." + parts.dropFirst().joined(separator: ".")
            if configured.contains(where: { $0.hasSuffix(parentSuffix) }) {
                return true
            }
        }
        return false
    }

    private func sendRaw(_ req: URLRequest, preferCellular: Bool) async throws -> (Data, URLResponse, Bool) {
        if Self.requiresCleartextATSBypass(req.url) {
            let (data, response) = try await sendCleartextHTTP(req)
            return (data, response, preferCellular || isCellular)
        }
        if let url = req.url, url.scheme?.lowercased() == "https", CloudflareAnycastAccelerator.isCloudflareTunnelHost(url.host ?? "") {
            CloudflareAnycastAccelerator.shared.triggerProbeIfNeeded(force: false)
            if let anycastIP = CloudflareAnycastAccelerator.verifiedPrimaryIP {
                do {
                    let (data, response) = try await sendCloudflareAcceleratedHTTPS(req, anycastIP: anycastIP)
                    if let http = response as? HTTPURLResponse,
                       http.statusCode == 403,
                       CloudflareAnycastAccelerator.isError1034Body(data) {
                        CloudflareAnycastAccelerator.shared.reportEdgeRestricted(ip: anycastIP)
                        await AcceleratedConnectionPool.shared.evict(ip: anycastIP)
                        // Fall through to system URLSession
                    } else {
                        return (data, response, false)
                    }
                } catch {
                    // If Anycast acceleration fails, smoothly fall back to system URLSession
                }
            }
        }
        let (data, response) = try await fallbackSession.data(for: req)
        return (data, response, false)
    }

    private static func isIdempotentForTransientRetry(_ req: URLRequest) -> Bool {
        let method = (req.httpMethod ?? "GET").uppercased()
        if method == "GET" || method == "HEAD" || method == "OPTIONS" {
            return true
        }
        // Requests carrying explicit server-deduplicated idempotency IDs (e.g. SendUserCascadeMessage
        // and createCascade via X-Client-Message-Id) can be retried safely without duplicating messages.
        if let cmid = req.value(forHTTPHeaderField: "X-Client-Message-Id")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !cmid.isEmpty {
            return true
        }
        if let idem = req.value(forHTTPHeaderField: "Idempotency-Key")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !idem.isEmpty {
            return true
        }
        guard let path = req.url?.path else { return false }
        // Attachment uploads are SHA-256 content-addressed and deduplicated on the gateway, so retry is safe.
        if path.hasSuffix("/api/v1/attachments") {
            return true
        }
        let safeReadSuffixes = [
            "/GetAllCascadeTrajectories",
            "/GetCascadeTrajectory",
            "/GetCascadeTrajectorySteps",
            "/GetAuthStatus",
            "/GetCascadeNuxes",
            "/GetUserStatus",
            "/SearchConversations",
            "/GetModelConfigs",
            "/GetAvailableCascadePlugins",
            "/GetMcpServerStates"
        ]
        return safeReadSuffixes.contains(where: { path.hasSuffix($0) })
    }

    private static func isRelayTunnelBusy(statusCode: Int, data: Data) -> Bool {
        guard statusCode == 429 else { return false }
        let snippet = String(data: data.prefix(256), encoding: .utf8)?.lowercased() ?? ""
        return snippet.contains("tunnel_busy")
    }

    private static func isGatewayUpstreamUnavailableResponse(statusCode: Int, headers: [AnyHashable: Any]? = nil, data: Data) -> Bool {
        guard statusCode == 503 else { return false }
        if let headers,
           let mgyErr = headers.first(where: { String(describing: $0.key).caseInsensitiveCompare("X-Mgy-Error") == .orderedSame })?.value,
           String(describing: mgyErr).trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "upstream_not_connected" {
            return true
        }
        let snippet = String(data: data.prefix(320), encoding: .utf8)?.lowercased() ?? ""
        return snippet.contains("upstream_not_connected") ||
            snippet.contains("antigravity upstream not connected") ||
            snippet.contains("language_server is not connected") ||
            snippet.contains("language_server not connected")
    }

    private static func isOuterRelayRetryableStatus(_ statusCode: Int, data: Data) -> (retryable: Bool, preGateway: Bool) {
        switch statusCode {
        case 503:
            if isGatewayUpstreamUnavailableResponse(statusCode: statusCode, data: data) {
                return (false, false)
            }
            // Relay `gateway_offline` / `relay_capacity_full` (or edge 503): never reached mgy gateway.
            return (true, true)
        case 429:
            return isRelayTunnelBusy(statusCode: statusCode, data: data) ? (true, true) : (false, false)
        case 502, 520, 521, 522, 523, 524:
            // Relay `gateway_unreachable` (e.g. single dead yamux slot) or Cloudflare edge blip.
            return (true, false)
        default:
            return (false, false)
        }
    }

    private static func isRetryableTransportError(_ error: Error) -> Bool {
        guard let urlErr = error as? URLError else { return false }
        switch urlErr.code {
        case .networkConnectionLost, .cannotConnectToHost, .cannotFindHost, .dnsLookupFailed, .secureConnectionFailed:
            return true
        default:
            return false
        }
    }

    private static func rewrittenRequest(_ req: URLRequest, toBaseURL base: URL) -> URLRequest? {
        guard let orig = req.url,
              var comps = URLComponents(url: orig, resolvingAgainstBaseURL: false),
              let scheme = base.scheme,
              let host = base.host else {
            return nil
        }
        comps.scheme = scheme
        comps.host = host
        comps.port = base.port
        guard let newURL = comps.url else { return nil }
        var copy = req
        copy.url = newURL
        return copy
    }

    private static func isInvalidInnerRequestError(statusCode: Int, data: Data) -> Bool {
        guard statusCode == 400 else { return false }
        let snippet = String(data: data.prefix(256), encoding: .utf8)?.lowercased() ?? ""
        return snippet.contains("invalid_inner_request")
    }

    private func sendOverE2EEOnce(
        _ req: URLRequest,
        preferCellular: Bool,
        reqEpoch: UInt64,
        forceLegacyJSON: Bool = false,
        retriedClockSkew: Bool = false
    ) async throws -> (data: Data, response: URLResponse, outerStatus: Int?) {
        guard let originalURL = req.url else {
            throw URLError(.badURL)
        }
        guard GatewayRuntimeState.shared.epoch == reqEpoch else {
            throw CancellationError()
        }
        let wrapped = try E2EECrypto.shared.wrapHTTPRequestDetailed(req, forceLegacyJSON: forceLegacyJSON)
        let (outerData, outerResp, usedCellular) = try await sendRaw(wrapped.outerRequest, preferCellular: preferCellular)
        guard let outerHTTP = outerResp as? HTTPURLResponse else {
            return (outerData, decorateResponse(outerResp, url: originalURL, forcedCellular: usedCellular, requestEpoch: reqEpoch), nil)
        }
        if outerHTTP.statusCode != 200 {
            if !retriedClockSkew,
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let serverTs = E2EECrypto.parseClockSkewServerTimestamp(statusCode: outerHTTP.statusCode, data: outerData) {
                E2EECrypto.shared.recordClockSkew(host: originalURL.host ?? "", serverTsMs: serverTs)
                return try await sendOverE2EEOnce(
                    req,
                    preferCellular: preferCellular,
                    reqEpoch: reqEpoch,
                    forceLegacyJSON: forceLegacyJSON,
                    retriedClockSkew: true
                )
            }
            if wrapped.usedBinaryFrame && !forceLegacyJSON && GatewayRuntimeState.shared.epoch == reqEpoch && Self.isInvalidInnerRequestError(statusCode: outerHTTP.statusCode, data: outerData) {
                if let host = originalURL.host {
                    E2EECrypto.shared.clearHostBinaryRequestSupport(host)
                }
                return try await sendOverE2EEOnce(
                    req,
                    preferCellular: preferCellular,
                    reqEpoch: reqEpoch,
                    forceLegacyJSON: true,
                    retriedClockSkew: retriedClockSkew
                )
            }
            return (outerData, decorateResponse(outerHTTP, url: originalURL, forcedCellular: usedCellular, requestEpoch: reqEpoch), outerHTTP.statusCode)
        }
        if GatewayRuntimeState.shared.epoch == reqEpoch {
            E2EECrypto.shared.observeServerCapabilities(from: outerHTTP, url: originalURL)
        }
        let (innerData, innerResp) = try E2EECrypto.shared.unwrapHTTPResponse(
            encData: outerData,
            originalURL: originalURL,
            deviceID: wrapped.deviceID,
            key: wrapped.key
        )
        return (innerData, decorateResponse(innerResp, url: originalURL, forcedCellular: usedCellular, trustedByE2EE: true, requestEpoch: reqEpoch), 200)
    }

    /// When on Wi-Fi and a public relay fails or returns 502/503, prefer switching to local LAN
    /// directly instead of spending seconds migrating across remote VPS pool nodes.
    private func tryLanFastFallback(for req: URLRequest, reqEpoch: UInt64) async -> (Data, URLResponse)? {
        guard !isCellular && isWifi, !Task.isCancelled, GatewayRuntimeState.shared.epoch == reqEpoch else { return nil }
        guard let origHost = req.url?.host, !Self.isLocalOrPrivateHost(origHost) else { return nil }
        let switched = await ConnectionManager.shared.probeLanFastIfNeeded(force: true, timeout: 0.65)
        guard GatewayRuntimeState.shared.epoch == reqEpoch else { return nil }
        let lanBase: URL? = await MainActor.run {
            guard GatewayRuntimeState.shared.epoch == reqEpoch else { return nil }
            guard let lanStr = AppSettings.shared.lanServerURL?.trimmingCharacters(in: .whitespacesAndNewlines),
                  !lanStr.isEmpty,
                  let normalized = AppSettings.normalize(raw: lanStr),
                  Self.isLanOnlyHost(normalized.host ?? "") else {
                return nil
            }
            if switched { return normalized }
            // Also use LAN if another concurrent caller just elected LAN a moment ago.
            if let active = AppSettings.shared.activeServerURL,
               let activeURL = AppSettings.normalize(raw: active),
               Self.isLanOnlyHost(activeURL.host ?? "") {
                return activeURL
            }
            return nil
        }
        guard GatewayRuntimeState.shared.epoch == reqEpoch,
              let lanBase,
              let lanReq = Self.rewrittenRequest(req, toBaseURL: lanBase) else {
            return nil
        }
        do {
            let (data, response, usedCellular) = try await sendRaw(lanReq, preferCellular: false)
            return (data, decorateResponse(response, url: lanReq.url, forcedCellular: usedCellular, requestEpoch: reqEpoch))
        } catch {
            return nil
        }
    }

    /// Sends an E2EE request with transparent recovery for relay-layer failures (`503 gateway_offline`,
    /// `502 gateway_unreachable`, `429 tunnel_busy`, or transient pre-response transport drops) so
    /// recoverable glitches heal silently without popping spurious errors in the UI.
    private func sendOverE2EE(_ req: URLRequest, preferCellular: Bool, reqEpoch: UInt64) async throws -> (Data, URLResponse) {
        guard let originalURL = req.url else {
            throw URLError(.badURL)
        }

        let firstResult: (data: Data, response: URLResponse, outerStatus: Int?)
        do {
            firstResult = try await sendOverE2EEOnce(req, preferCellular: preferCellular, reqEpoch: reqEpoch)
        } catch {
            guard !Task.isCancelled,
                  GatewayRuntimeState.shared.epoch == reqEpoch,
                  Self.isRetryableTransportError(error),
                  Self.isIdempotentForTransientRetry(req) else {
                throw error
            }
            if let lanRes = await tryLanFastFallback(for: req, reqEpoch: reqEpoch) {
                return lanRes
            }
            if let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: originalURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(req, toBaseURL: migratedBase) {
                let retried = try await sendOverE2EEOnce(migratedReq, preferCellular: preferCellular, reqEpoch: reqEpoch)
                return (retried.data, retried.response)
            }
            try await Task.sleep(nanoseconds: 250_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                throw CancellationError()
            }
            let retried = try await sendOverE2EEOnce(req, preferCellular: preferCellular, reqEpoch: reqEpoch)
            return (retried.data, retried.response)
        }

        guard let outerStatus = firstResult.outerStatus, outerStatus != 200 else {
            return (firstResult.data, firstResult.response)
        }

        let classification = Self.isOuterRelayRetryableStatus(outerStatus, data: firstResult.data)
        let canRetry = classification.retryable && (classification.preGateway || Self.isIdempotentForTransientRetry(req))
        guard canRetry, !Task.isCancelled, GatewayRuntimeState.shared.epoch == reqEpoch else {
            return (firstResult.data, firstResult.response)
        }

        // On Wi-Fi, if the cloud relay returned 502/503, immediately prefer local LAN over remote relay retry.
        if (outerStatus == 503 || outerStatus == 502), let lanRes = await tryLanFastFallback(for: req, reqEpoch: reqEpoch) {
            return lanRes
        }

        var latest = firstResult
        var latestStatus = outerStatus

        // 1. Fast same-node retry for 502/429/edge blips (immediately rotates onto a healthy yamux slot).
        if latestStatus != 503 {
            try await Task.sleep(nanoseconds: 250_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                return (latest.data, latest.response)
            }
            if let attempt = try? await sendOverE2EEOnce(req, preferCellular: preferCellular, reqEpoch: reqEpoch) {
                latest = attempt
                if attempt.outerStatus == 200 {
                    return (attempt.data, attempt.response)
                }
                if let nextStatus = attempt.outerStatus {
                    latestStatus = nextStatus
                }
            }
        }

        // 2. Stale-relay migration via /api/relay/pool (e.g. gateway moved from lax-cf to jp1-direct).
        if (latestStatus == 503 || latestStatus == 502) && GatewayRuntimeState.shared.epoch == reqEpoch {
            if let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: originalURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(req, toBaseURL: migratedBase),
               let migrated = try? await sendOverE2EEOnce(migratedReq, preferCellular: preferCellular, reqEpoch: reqEpoch) {
                if migrated.outerStatus == 200 {
                    return (migrated.data, migrated.response)
                }
                latest = migrated
            } else if latestStatus == 503 && GatewayRuntimeState.shared.epoch == reqEpoch {
                // Gateway is still assigned to this relay node (brief tunnel reconnect window); retry once.
                try await Task.sleep(nanoseconds: 800_000_000)
                if GatewayRuntimeState.shared.epoch == reqEpoch,
                   let healed = try? await sendOverE2EEOnce(req, preferCellular: preferCellular, reqEpoch: reqEpoch) {
                    latest = healed
                }
            }
        }

        return (latest.data, latest.response)
    }

    /// Streams an E2EE-encrypted file upload (`POST /api/v1/e2ee/rpc`) through a dedicated streaming
    /// uploader with long resource timeouts, real chunk-level progress (0...0.95), automatic `req-bin2`
    /// fallback for older gateways, and transparent retry/failover on transient drops or relay 502/503/429.
    public func uploadOverE2EE(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)?,
        streamUpload: @escaping @Sendable (URLRequest, URL, (@Sendable (Double) -> Void)?) async throws -> (Data, URLResponse)
    ) async throws -> (Data, URLResponse) {
        let reqEpoch = GatewayRuntimeState.shared.epoch
        return try await uploadOverE2EEInternal(
            request: request,
            fileURL: fileURL,
            reqEpoch: reqEpoch,
            forceLegacyJSON: false,
            onProgress: onProgress,
            streamUpload: streamUpload
        )
    }

    private func uploadOverE2EEInternal(
        request: URLRequest,
        fileURL: URL,
        reqEpoch: UInt64,
        forceLegacyJSON: Bool,
        retriedClockSkew: Bool = false,
        retriedFreshEnvelope: Bool = false,
        onProgress: (@Sendable (Double) -> Void)?,
        streamUpload: @escaping @Sendable (URLRequest, URL, (@Sendable (Double) -> Void)?) async throws -> (Data, URLResponse)
    ) async throws -> (Data, URLResponse) {
        guard let originalURL = request.url else {
            throw URLError(.badURL)
        }
        guard GatewayRuntimeState.shared.epoch == reqEpoch,
              !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(originalURL) else {
            throw CancellationError()
        }

        let wrapped = try E2EECrypto.shared.wrapFileUploadToTempFile(
            request,
            fileURL: fileURL,
            forceLegacyJSON: forceLegacyJSON
        )
        defer { try? FileManager.default.removeItem(at: wrapped.tempFileURL) }

        func attempt(_ outerReq: URLRequest) async throws -> (data: Data, response: URLResponse, outerStatus: Int?) {
            let (outerData, outerResp) = try await streamUpload(outerReq, wrapped.tempFileURL, onProgress)
            guard let outerHTTP = outerResp as? HTTPURLResponse else {
                return (outerData, decorateResponse(outerResp, url: originalURL, requestEpoch: reqEpoch), nil)
            }
            if outerHTTP.statusCode != 200 {
                return (outerData, decorateResponse(outerHTTP, url: originalURL, requestEpoch: reqEpoch), outerHTTP.statusCode)
            }
            if GatewayRuntimeState.shared.epoch == reqEpoch {
                E2EECrypto.shared.observeServerCapabilities(from: outerHTTP, url: outerReq.url ?? originalURL)
            }
            let (innerData, innerResp) = try E2EECrypto.shared.unwrapHTTPResponse(
                encData: outerData,
                originalURL: originalURL,
                deviceID: wrapped.deviceID,
                key: wrapped.key
            )
            onProgress?(0.95)
            return (innerData, decorateResponse(innerResp, url: originalURL, trustedByE2EE: true, requestEpoch: reqEpoch), 200)
        }

        let firstResult: (data: Data, response: URLResponse, outerStatus: Int?)
        do {
            firstResult = try await attempt(wrapped.outerRequest)
        } catch {
            guard !Task.isCancelled, !retriedFreshEnvelope, GatewayRuntimeState.shared.epoch == reqEpoch, Self.isRetryableTransportError(error) else {
                throw error
            }
            if let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: originalURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(request, toBaseURL: migratedBase) {
                return try await uploadOverE2EEInternal(
                    request: migratedReq,
                    fileURL: fileURL,
                    reqEpoch: reqEpoch,
                    forceLegacyJSON: forceLegacyJSON,
                    retriedClockSkew: retriedClockSkew,
                    retriedFreshEnvelope: true,
                    onProgress: onProgress,
                    streamUpload: streamUpload
                )
            }
            try await Task.sleep(nanoseconds: 300_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                throw CancellationError()
            }
            return try await uploadOverE2EEInternal(
                request: request,
                fileURL: fileURL,
                reqEpoch: reqEpoch,
                forceLegacyJSON: forceLegacyJSON,
                retriedClockSkew: retriedClockSkew,
                retriedFreshEnvelope: true,
                onProgress: onProgress,
                streamUpload: streamUpload
            )
        }

        guard let outerStatus = firstResult.outerStatus, outerStatus != 200 else {
            return (firstResult.data, firstResult.response)
        }

        if !retriedClockSkew,
           GatewayRuntimeState.shared.epoch == reqEpoch,
           let serverTs = E2EECrypto.parseClockSkewServerTimestamp(statusCode: outerStatus, data: firstResult.data) {
            E2EECrypto.shared.recordClockSkew(host: originalURL.host ?? "", serverTsMs: serverTs)
            return try await uploadOverE2EEInternal(
                request: request,
                fileURL: fileURL,
                reqEpoch: reqEpoch,
                forceLegacyJSON: forceLegacyJSON,
                retriedClockSkew: true,
                retriedFreshEnvelope: retriedFreshEnvelope,
                onProgress: onProgress,
                streamUpload: streamUpload
            )
        }

        if !retriedFreshEnvelope && GatewayRuntimeState.shared.epoch == reqEpoch && E2EECrypto.isReplayDetectedError(statusCode: outerStatus, data: firstResult.data) {
            return try await uploadOverE2EEInternal(
                request: request,
                fileURL: fileURL,
                reqEpoch: reqEpoch,
                forceLegacyJSON: forceLegacyJSON,
                retriedClockSkew: retriedClockSkew,
                retriedFreshEnvelope: true,
                onProgress: onProgress,
                streamUpload: streamUpload
            )
        }

        if wrapped.usedBinaryFrame && !forceLegacyJSON && GatewayRuntimeState.shared.epoch == reqEpoch && Self.isInvalidInnerRequestError(statusCode: outerStatus, data: firstResult.data) {
            if let host = originalURL.host {
                E2EECrypto.shared.clearHostBinaryRequestSupport(host)
            }
            return try await uploadOverE2EEInternal(
                request: request,
                fileURL: fileURL,
                reqEpoch: reqEpoch,
                forceLegacyJSON: true,
                retriedClockSkew: retriedClockSkew,
                retriedFreshEnvelope: retriedFreshEnvelope,
                onProgress: onProgress,
                streamUpload: streamUpload
            )
        }

        let classification = Self.isOuterRelayRetryableStatus(outerStatus, data: firstResult.data)
        guard classification.retryable, !Task.isCancelled, GatewayRuntimeState.shared.epoch == reqEpoch else {
            return (firstResult.data, firstResult.response)
        }

        // For non-preGateway blips (e.g. 502 where gateway might have already recorded req_id), re-wrap with a fresh req_id.
        if !classification.preGateway && !retriedFreshEnvelope {
            try await Task.sleep(nanoseconds: 250_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                return (firstResult.data, firstResult.response)
            }
            return try await uploadOverE2EEInternal(
                request: request,
                fileURL: fileURL,
                reqEpoch: reqEpoch,
                forceLegacyJSON: forceLegacyJSON,
                retriedClockSkew: retriedClockSkew,
                retriedFreshEnvelope: true,
                onProgress: onProgress,
                streamUpload: streamUpload
            )
        }

        var latest = firstResult
        var latestStatus = outerStatus

        // 1. Fast same-node retry for 429 tunnel_busy (preGateway; never reached mgy replay cache).
        if latestStatus != 503 {
            try await Task.sleep(nanoseconds: 250_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                return (latest.data, latest.response)
            }
            if let retried = try? await attempt(wrapped.outerRequest) {
                latest = retried
                if retried.outerStatus == 200 {
                    return (retried.data, retried.response)
                }
                if let nextStatus = retried.outerStatus {
                    latestStatus = nextStatus
                }
            }
        }

        // 2. Stale-relay migration via /api/relay/pool or brief 503 reconnect retry.
        if (latestStatus == 503 || latestStatus == 502) && GatewayRuntimeState.shared.epoch == reqEpoch {
            if let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: originalURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(wrapped.outerRequest, toBaseURL: migratedBase),
               let migrated = try? await attempt(migratedReq) {
                if migrated.outerStatus == 200 {
                    return (migrated.data, migrated.response)
                }
                latest = migrated
            } else if latestStatus == 503 && GatewayRuntimeState.shared.epoch == reqEpoch {
                try await Task.sleep(nanoseconds: 800_000_000)
                if GatewayRuntimeState.shared.epoch == reqEpoch,
                   let healed = try? await attempt(wrapped.outerRequest) {
                    latest = healed
                }
            }
        }

        return (latest.data, latest.response)
    }

    /// Sends a request using URLSession, except cleartext HTTP to Tailscale private tunnel nodes
    /// which App Transport Security blocks. Cleartext HTTP to public internet hosts is rejected.
    /// Requests targeting E2EE relay hosts are transparently wrapped inside AES-256-GCM envelopes.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        let reqEpoch = GatewayRuntimeState.shared.epoch
        var req = request
        if let url = req.url {
            if url.scheme?.lowercased() == "http" {
                let host = url.host ?? ""
                guard Self.isLocalOrPrivateHost(host) else {
                    throw URLError(.appTransportSecurityRequiresSecureConnection)
                }
            }
            let path = url.path
            let isUnscopedUtilityPath = (path == "/healthz" || path.hasPrefix("/api/v1/auth/pair") || path.hasPrefix("/api/v1/e2ee/pair"))
            if !isUnscopedUtilityPath && GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(url) {
                throw CancellationError()
            }
        }
        let isConfiguredOrigin = Self.isConfiguredGatewayOrigin(req.url)
        if isConfiguredOrigin && req.value(forHTTPHeaderField: "Accept-Language") == nil {
            req.setValue(DeviceTitleTargetLanguage.current().bcp47Tag, forHTTPHeaderField: "Accept-Language")
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           let reqURL = req.url,
           reqURL.path != "/healthz",
           isConfiguredOrigin {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }

        if isConfiguredOrigin && E2EECrypto.shared.shouldEncrypt(url: req.url) {
            return try await sendOverE2EE(req, preferCellular: preferCellular, reqEpoch: reqEpoch)
        }

        let (data, response, usedCellular) = try await sendRaw(req, preferCellular: preferCellular)
        // Auto-recover if a custom relay domain enforces E2EE with HTTP 426 Upgrade Required
        if let http = response as? HTTPURLResponse,
           http.statusCode == 426,
           isConfiguredOrigin,
           E2EECrypto.shared.isReady,
           let host = req.url?.host,
           !Self.isLocalOrPrivateHost(host),
           req.url?.path.hasPrefix("/api/v1/e2ee/") != true {
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                return (data, decorateResponse(response, url: req.url, forcedCellular: usedCellular, requestEpoch: reqEpoch))
            }
            E2EECrypto.shared.markHostRequiresE2EE(host)
            return try await sendOverE2EE(req, preferCellular: preferCellular, reqEpoch: reqEpoch)
        }
        // Auto-migrate if a legacy cloud/tunnel endpoint returns 502/503 while local LAN or an E2EE relay pool node is live
        // (never treat a gateway-generated 503 upstream_not_connected as an offline relay node).
        if let http = response as? HTTPURLResponse,
           (http.statusCode == 503 || (http.statusCode == 502 && Self.isIdempotentForTransientRetry(req))),
           !Self.isGatewayUpstreamUnavailableResponse(statusCode: http.statusCode, headers: http.allHeaderFields, data: data),
           GatewayRuntimeState.shared.epoch == reqEpoch,
           let reqURL = req.url,
           reqURL.path != "/healthz",
           !reqURL.path.hasPrefix("/api/v1/e2ee/"),
           isConfiguredOrigin,
           let host = reqURL.host,
           !Self.isLocalOrPrivateHost(host) {
            if let lanRes = await tryLanFastFallback(for: req, reqEpoch: reqEpoch) {
                return lanRes
            }
            if E2EECrypto.shared.isReady,
               let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: reqURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(req, toBaseURL: migratedBase) {
                return try await sendOverE2EE(migratedReq, preferCellular: preferCellular, reqEpoch: reqEpoch)
            }
        }
        return (data, decorateResponse(response, url: req.url, forcedCellular: usedCellular, requestEpoch: reqEpoch))
    }

    /// Sends a side-effect-free request using an explicit `GatewayRequestContext` (for background multi-gateway
    /// summary probes, cross-gateway push token sync, and lock-screen quick approvals).
    /// Never reads or mutates global active gateway credentials, active URLs, or 401 revocation broadcasts.
    public func send(
        request: URLRequest,
        context: GatewayRequestContext,
        preferCellular: Bool = false
    ) async throws -> (Data, URLResponse) {
        var req = request
        guard let originalURL = req.url else {
            throw URLError(.badURL)
        }
        if originalURL.scheme?.lowercased() == "http" {
            let host = originalURL.host ?? ""
            guard Self.isLocalOrPrivateHost(host) else {
                throw URLError(.appTransportSecurityRequiresSecureConnection)
            }
        }
        if req.value(forHTTPHeaderField: "Accept-Language") == nil {
            req.setValue(DeviceTitleTargetLanguage.current().bcp47Tag, forHTTPHeaderField: "Accept-Language")
        }
        if let token = context.deviceToken, !token.isEmpty, originalURL.path != "/healthz" {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }

        func sendContextE2EEOnce(forceLegacyJSON: Bool, retriedClockSkew: Bool = false) async throws -> (Data, URLResponse) {
            let wrapped = try E2EECrypto.shared.wrapHTTPRequestDetailed(
                req,
                context: context,
                forceLegacyJSON: forceLegacyJSON
            )
            let (outerData, outerResp, _) = try await sendRaw(wrapped.outerRequest, preferCellular: preferCellular)
            guard let outerHTTP = outerResp as? HTTPURLResponse else {
                return (outerData, outerResp)
            }
            if outerHTTP.statusCode != 200 {
                if !retriedClockSkew,
                   let serverTs = E2EECrypto.parseClockSkewServerTimestamp(statusCode: outerHTTP.statusCode, data: outerData) {
                    E2EECrypto.shared.recordClockSkew(host: originalURL.host ?? "", serverTsMs: serverTs)
                    return try await sendContextE2EEOnce(forceLegacyJSON: forceLegacyJSON, retriedClockSkew: true)
                }
                if wrapped.usedBinaryFrame && !forceLegacyJSON && Self.isInvalidInnerRequestError(statusCode: outerHTTP.statusCode, data: outerData) {
                    if let host = originalURL.host {
                        E2EECrypto.shared.clearHostBinaryRequestSupport(host)
                    }
                    return try await sendContextE2EEOnce(forceLegacyJSON: true, retriedClockSkew: retriedClockSkew)
                }
                return (outerData, outerHTTP)
            }
            E2EECrypto.shared.observeServerCapabilities(from: outerHTTP, url: originalURL)
            let (innerData, innerResp) = try E2EECrypto.shared.unwrapHTTPResponse(
                encData: outerData,
                originalURL: originalURL,
                deviceID: wrapped.deviceID,
                key: wrapped.key
            )
            if let rawCloud = innerResp.value(forHTTPHeaderField: "X-Antigravity-Cloud-URL")?.trimmingCharacters(in: .whitespacesAndNewlines),
               !rawCloud.isEmpty,
               Self.isValidAdvertisedCloudURL(rawCloud, trustedByE2EE: true) {
                let profileId = context.profileId
                Task { @MainActor in
                    GatewayProfileStore.shared.updateCloudURL(profileId: profileId, cloudURL: rawCloud)
                }
            }
            return (innerData, innerResp)
        }

        if context.shouldEncrypt(url: originalURL) {
            return try await sendContextE2EEOnce(forceLegacyJSON: false)
        }

        let (data, response, _) = try await sendRaw(req, preferCellular: preferCellular)
        if let http = response as? HTTPURLResponse,
           http.statusCode == 426,
           context.isE2EEReady,
           let host = originalURL.host,
           !Self.isLocalOrPrivateHost(host),
           !originalURL.path.hasPrefix("/api/v1/e2ee/") {
            let profileId = context.profileId
            Task { @MainActor in
                GatewayProfileStore.shared.markE2EEHost(profileId: profileId, host: host)
            }
            return try await sendContextE2EEOnce(forceLegacyJSON: false)
        }
        return (data, response)
    }

    /// Downloads a file to a temporary URL, transparently handling E2EE relay encryption
    /// (`POST /api/v1/e2ee/rpc`) with `HTTP 426 e2ee_required` auto-recovery, Tailscale
    /// cleartext ATS bypass, streaming progress updates, and Swift Task cancellation.
    public func download(
        request: URLRequest,
        allowE2EE: Bool? = nil,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)? = nil
    ) async throws -> (URL, HTTPURLResponse) {
        let reqEpoch = GatewayRuntimeState.shared.epoch
        var req = request
        if let url = req.url {
            if url.scheme?.lowercased() == "http" {
                let host = url.host ?? ""
                guard Self.isLocalOrPrivateHost(host) else {
                    throw URLError(.appTransportSecurityRequiresSecureConnection)
                }
            }
            if GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(url) {
                throw CancellationError()
            }
        }
        let isConfiguredOrigin = Self.isConfiguredGatewayOrigin(req.url)
        if isConfiguredOrigin && req.value(forHTTPHeaderField: "Accept-Language") == nil {
            req.setValue(DeviceTitleTargetLanguage.current().bcp47Tag, forHTTPHeaderField: "Accept-Language")
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           let reqURL = req.url,
           reqURL.path != "/healthz",
           isConfiguredOrigin,
           req.value(forHTTPHeaderField: "Authorization") == nil {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let useE2EE = (allowE2EE ?? isConfiguredOrigin) && isConfiguredOrigin
        if useE2EE && E2EECrypto.shared.shouldEncrypt(url: req.url) {
            return try await downloadOverE2EE(req, reqEpoch: reqEpoch, onProgress: onProgress)
        }

        if Self.requiresCleartextATSBypass(req.url) {
            return try await downloadCleartextHTTP(request: req, reqEpoch: reqEpoch, onProgress: onProgress)
        }

        let (tempURL, httpResp) = try await downloadViaURLSession(req, onProgress: onProgress)
        // Auto-recover if a custom relay domain enforces E2EE with HTTP 426 Upgrade Required
        if useE2EE,
           httpResp.statusCode == 426,
           GatewayRuntimeState.shared.epoch == reqEpoch,
           E2EECrypto.shared.isReady,
           let host = req.url?.host,
           !Self.isLocalOrPrivateHost(host),
           req.url?.path.hasPrefix("/api/v1/e2ee/") != true {
            try? FileManager.default.removeItem(at: tempURL)
            E2EECrypto.shared.markHostRequiresE2EE(host)
            return try await downloadOverE2EE(req, reqEpoch: reqEpoch, onProgress: onProgress)
        }
        let decorated = (decorateResponse(httpResp, url: req.url, requestEpoch: reqEpoch) as? HTTPURLResponse) ?? httpResp
        return (tempURL, decorated)
    }

    private func downloadOverE2EEOnce(
        _ req: URLRequest,
        reqEpoch: UInt64,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)?,
        retriedClockSkew: Bool = false
    ) async throws -> (fileURL: URL, response: HTTPURLResponse, outerStatus: Int) {
        guard let originalURL = req.url else {
            throw URLError(.badURL)
        }
        guard GatewayRuntimeState.shared.epoch == reqEpoch else {
            throw CancellationError()
        }
        let (outerReq, deviceID, key) = try E2EECrypto.shared.wrapHTTPRequest(req)
        let (tempEncURL, outerHTTP) = try await downloadViaURLSession(outerReq, onProgress: onProgress)
        if outerHTTP.statusCode != 200 {
            if outerHTTP.statusCode == 401 && !retriedClockSkew && GatewayRuntimeState.shared.epoch == reqEpoch,
               let errData = try? Data(contentsOf: tempEncURL),
               let serverTs = E2EECrypto.parseClockSkewServerTimestamp(statusCode: 401, data: errData) {
                try? FileManager.default.removeItem(at: tempEncURL)
                E2EECrypto.shared.recordClockSkew(host: originalURL.host ?? "", serverTsMs: serverTs)
                return try await downloadOverE2EEOnce(req, reqEpoch: reqEpoch, onProgress: onProgress, retriedClockSkew: true)
            }
            let decorated = (decorateResponse(outerHTTP, url: originalURL, requestEpoch: reqEpoch) as? HTTPURLResponse) ?? outerHTTP
            return (tempEncURL, decorated, outerHTTP.statusCode)
        }
        if GatewayRuntimeState.shared.epoch == reqEpoch {
            E2EECrypto.shared.observeServerCapabilities(from: outerHTTP, url: originalURL)
        }

        defer { try? FileManager.default.removeItem(at: tempEncURL) }
        try Task.checkCancellation()
        let encData = try Data(contentsOf: tempEncURL, options: .mappedIfSafe)
        let (innerData, innerResp) = try E2EECrypto.shared.unwrapHTTPResponse(
            encData: encData,
            originalURL: originalURL,
            deviceID: deviceID,
            key: key
        )
        try Task.checkCancellation()

        let plainTempURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try innerData.write(to: plainTempURL, options: .atomic)
        let decorated = (decorateResponse(innerResp, url: originalURL, trustedByE2EE: true, requestEpoch: reqEpoch) as? HTTPURLResponse) ?? innerResp
        if (200...299).contains(decorated.statusCode), !innerData.isEmpty {
            let size = Int64(innerData.count)
            onProgress?(1.0, size, size)
        }
        return (plainTempURL, decorated, 200)
    }

    private func downloadOverE2EE(
        _ req: URLRequest,
        reqEpoch: UInt64,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)?
    ) async throws -> (URL, HTTPURLResponse) {
        guard let originalURL = req.url else {
            throw URLError(.badURL)
        }
        var latest = try await downloadOverE2EEOnce(req, reqEpoch: reqEpoch, onProgress: onProgress)
        guard latest.outerStatus != 200 else {
            return (latest.fileURL, latest.response)
        }

        let errSnippet = (try? Data(contentsOf: latest.fileURL)) ?? Data()
        let classification = Self.isOuterRelayRetryableStatus(latest.outerStatus, data: errSnippet)
        guard classification.retryable, !Task.isCancelled, GatewayRuntimeState.shared.epoch == reqEpoch else {
            return (latest.fileURL, latest.response)
        }

        if latest.outerStatus != 503 {
            try await Task.sleep(nanoseconds: 250_000_000)
            guard GatewayRuntimeState.shared.epoch == reqEpoch else {
                return (latest.fileURL, latest.response)
            }
            if let attempt = try? await downloadOverE2EEOnce(req, reqEpoch: reqEpoch, onProgress: onProgress) {
                try? FileManager.default.removeItem(at: latest.fileURL)
                latest = attempt
                if latest.outerStatus == 200 {
                    return (latest.fileURL, latest.response)
                }
            }
        }

        if (latest.outerStatus == 503 || latest.outerStatus == 502) && GatewayRuntimeState.shared.epoch == reqEpoch {
            if let migratedBase = await RelayPoolResolver.shared.migrateRelayHostIfNeeded(failedURL: originalURL, expectedEpoch: reqEpoch),
               GatewayRuntimeState.shared.epoch == reqEpoch,
               let migratedReq = Self.rewrittenRequest(req, toBaseURL: migratedBase),
               let migrated = try? await downloadOverE2EEOnce(migratedReq, reqEpoch: reqEpoch, onProgress: onProgress) {
                try? FileManager.default.removeItem(at: latest.fileURL)
                latest = migrated
            } else if latest.outerStatus == 503 && GatewayRuntimeState.shared.epoch == reqEpoch {
                try await Task.sleep(nanoseconds: 800_000_000)
                if GatewayRuntimeState.shared.epoch == reqEpoch,
                   let healed = try? await downloadOverE2EEOnce(req, reqEpoch: reqEpoch, onProgress: onProgress) {
                    try? FileManager.default.removeItem(at: latest.fileURL)
                    latest = healed
                }
            }
        }

        return (latest.fileURL, latest.response)
    }

    private func downloadViaURLSession(
        _ request: URLRequest,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)?
    ) async throws -> (URL, HTTPURLResponse) {
        try Task.checkCancellation()
        let delegate = FileDownloadProgressDelegate(onProgress: onProgress)
        // A plain session: background sessions are scheduled by nsurlsessiond out of process, which adds
        // start-up latency to the small, user-awaited preview downloads this is used for.
        let config = URLSessionConfiguration.default
        let reqTimeout = request.timeoutInterval > 0 ? request.timeoutInterval : 60
        config.timeoutIntervalForRequest = reqTimeout
        config.timeoutIntervalForResource = max(reqTimeout * 5, 300)
        config.httpShouldSetCookies = false
        config.httpCookieAcceptPolicy = .never
        config.httpCookieStorage = nil
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        let session = URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }

        let taskHolder = OSAllocatedUnfairLock<URLSessionDownloadTask?>(initialState: nil)
        let cancelledFlag = OSAllocatedUnfairLock(initialState: false)
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<(URL, HTTPURLResponse), Error>) in
                delegate.setContinuation(continuation)
                if cancelledFlag.withLock({ $0 }) {
                    delegate.cancel()
                    return
                }
                let task = session.downloadTask(with: request)
                taskHolder.withLock { $0 = task }
                if cancelledFlag.withLock({ $0 }) {
                    task.cancel()
                    delegate.cancel()
                    return
                }
                task.resume()
            }
        } onCancel: {
            cancelledFlag.withLock { $0 = true }
            taskHolder.withLock { $0 }?.cancel()
            delegate.cancel()
        }
    }
    
    /// ATS blocks cleartext HTTP to Tailscale private hosts (NSAllowsLocalNetworking only exempts RFC1918/loopback/.local).
    /// Only Tailscale WireGuard private hosts (100.64.0.0/10, *.ts.net) may use NWConnection cleartext transport; public cleartext is rejected above.
    nonisolated public static func requiresCleartextATSBypass(_ url: URL?) -> Bool {
        guard let url, url.scheme?.lowercased() == "http" else { return false }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        return isTailscaleHost(host)
    }

    /// ATS also blocks cleartext `ws://` WebSocket connections over `URLSession` to Tailscale private hosts.
    nonisolated public static func requiresCleartextWebSocketATSBypass(_ url: URL?) -> Bool {
        guard let url, let scheme = url.scheme?.lowercased(), scheme == "ws" || scheme == "http" else { return false }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        return isTailscaleHost(host)
    }

    /// Fetches data with `URLCache.shared` support while routing Tailscale cleartext HTTP and
    /// Cloudflare Anycast HTTPS through `NetworkTransport` so image/media loaders never fail ATS.
    public func fetchCachedData(for request: URLRequest) async throws -> (Data, URLResponse) {
        if request.cachePolicy == .returnCacheDataElseLoad || request.cachePolicy == .returnCacheDataDontLoad {
            if let cached = URLCache.shared.cachedResponse(for: request) {
                return (cached.data, cached.response)
            }
            if request.cachePolicy == .returnCacheDataDontLoad {
                throw URLError(.resourceUnavailable)
            }
        }
        let bypassedURLSession = Self.requiresCleartextATSBypass(request.url) ||
            (request.url?.scheme?.lowercased() == "https" &&
             CloudflareAnycastAccelerator.isCloudflareTunnelHost(request.url?.host ?? "") &&
             CloudflareAnycastAccelerator.verifiedPrimaryIP != nil)
        let (data, response) = try await send(request: request)
        if bypassedURLSession,
           request.cachePolicy != .reloadIgnoringLocalCacheData,
           let http = response as? HTTPURLResponse,
           (200..<300).contains(http.statusCode) {
            URLCache.shared.storeCachedResponse(CachedURLResponse(response: response, data: data), for: request)
        }
        return (data, response)
    }
    
    // MARK: - Accelerated Connection Pool
    
    private actor AcceleratedConnectionPool {
        static let shared = AcceleratedConnectionPool()
        
        struct Entry {
            let connection: NWConnection
            let host: String
            let lastUsed: Date
        }
        
        /// Idle keep-alive connections per "\(ip):\(port)", most recently used last. HTTP/1.1 carries one
        /// request at a time per connection, so concurrent requests (list, quotas, status and messages on
        /// launch) each need their own; keeping several avoids a fresh TCP+TLS handshake for each.
        private var pool: [String: [Entry]] = [:]
        private static let maxIdlePerKey = 4
        private static let idleTimeout: TimeInterval = 15.0

        func acquire(ip: String, host: String, port: NWEndpoint.Port, parameters: NWParameters) -> (NWConnection, Bool) {
            let key = "\(ip):\(port.rawValue)"
            var idle = pool[key] ?? []
            var reusable: NWConnection?
            while reusable == nil, let entry = idle.popLast() {
                if entry.connection.state == .ready &&
                   entry.host == host &&
                   Date().timeIntervalSince(entry.lastUsed) < Self.idleTimeout {
                    reusable = entry.connection
                } else {
                    entry.connection.cancel()
                }
            }
            pool[key] = idle.isEmpty ? nil : idle
            if let reusable {
                return (reusable, true)
            }
            let conn = NWConnection(host: NWEndpoint.Host(ip), port: port, using: parameters)
            return (conn, false)
        }

        func release(connection: NWConnection, ip: String, host: String, port: NWEndpoint.Port, canReuse: Bool) {
            let key = "\(ip):\(port.rawValue)"
            guard canReuse && connection.state == .ready else {
                connection.cancel()
                return
            }
            var idle = pool[key] ?? []
            idle.append(Entry(connection: connection, host: host, lastUsed: Date()))
            while idle.count > Self.maxIdlePerKey {
                idle.removeFirst().connection.cancel()
            }
            pool[key] = idle
        }

        func evict(ip: String) {
            let prefix = "\(ip):"
            for key in Array(pool.keys) where key.hasPrefix(prefix) {
                if let entries = pool.removeValue(forKey: key) {
                    for entry in entries {
                        entry.connection.cancel()
                    }
                }
            }
        }
    }
    
    private func sendCloudflareAcceleratedHTTPS(_ request: URLRequest, anycastIP: String) async throws -> (Data, URLResponse) {
        guard let url = request.url else {
            throw URLError(.badURL)
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty else {
            throw URLError(.badURL)
        }
        
        let tlsOptions = NWProtocolTLS.Options()
        sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, bareHost)
        let parameters = NWParameters(tls: tlsOptions)
        
        let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 443)) ?? .https
        
        for attempt in 0..<2 {
            let (connection, isReused) = await AcceleratedConnectionPool.shared.acquire(
                ip: anycastIP, host: bareHost, port: port, parameters: parameters
            )
            
            if !isReused {
                do {
                    try await Self.waitUntilReady(connection, timeout: request.timeoutInterval > 0 ? min(request.timeoutInterval, 5) : 5)
                } catch {
                    connection.cancel()
                    throw error
                }
            }
            
            let payload = HTTP11Codec.buildRequest(request, url: url, bareHost: bareHost, keepAlive: true)
            var canReuse = false
            do {
                try await Self.sendAll(connection, payload)
                let (raw, isUntilClose) = try await Self.receiveHTTPMessage(connection, timeout: request.timeoutInterval > 0 ? max(request.timeoutInterval, 30) : 30)
                let (data, response) = try HTTP11Codec.parseResponse(raw, url: url)
                
                if isUntilClose {
                    canReuse = false
                } else if let http = response as? HTTPURLResponse,
                   let connHeader = http.value(forHTTPHeaderField: "Connection"),
                   connHeader.lowercased().contains("close") {
                    canReuse = false
                } else {
                    canReuse = true
                }
                
                await AcceleratedConnectionPool.shared.release(
                    connection: connection, ip: anycastIP, host: bareHost, port: port, canReuse: canReuse
                )
                return (data, response)
            } catch {
                connection.cancel()
                if isReused && attempt == 0 {
                    continue
                }
                throw error
            }
        }
        
        throw URLError(.cannotConnectToHost)
    }

    /// Streams a file upload over cleartext HTTP via `NWConnection` (for Tailscale ATS bypass),
    /// reporting chunk-level upload progress (0...0.95) without buffering the entire file in memory.
    public func uploadCleartextHTTP(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)? = nil
    ) async throws -> (Data, URLResponse) {
        let reqEpoch = GatewayRuntimeState.shared.epoch
        var req = request
        guard let url = req.url else {
            throw URLError(.badURL)
        }
        guard !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(url) else {
            throw CancellationError()
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty, Self.isLocalOrPrivateHost(bareHost), let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 80)) else {
            throw URLError(.appTransportSecurityRequiresSecureConnection)
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           Self.isConfiguredOrLocalGatewayHost(bareHost),
           req.value(forHTTPHeaderField: "Authorization") == nil {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let connection = NWConnection(host: NWEndpoint.Host(bareHost), port: port, using: .tcp)
        try await Self.waitUntilReady(connection, timeout: req.timeoutInterval > 0 ? min(req.timeoutInterval, 15) : 15)
        defer { connection.cancel() }

        let (data, response) = try await Self.streamFileUpload(
            connection: connection,
            request: req,
            url: url,
            bareHost: bareHost,
            fileURL: fileURL,
            onProgress: onProgress
        )
        return (data, decorateResponse(response, url: req.url, forcedCellular: isCellular, requestEpoch: reqEpoch))
    }

    /// Streams a file upload over HTTPS via Cloudflare Anycast acceleration (`NWConnection` + TLS + SNI),
    /// reporting 32 KB chunk-level upload progress (0...0.95) without URLSession's 1–2 MB chunk buffering.
    public func uploadCloudflareAcceleratedHTTPS(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)? = nil
    ) async throws -> (Data, URLResponse) {
        let reqEpoch = GatewayRuntimeState.shared.epoch
        var req = request
        guard let url = req.url, url.scheme?.lowercased() == "https" else {
            throw URLError(.badURL)
        }
        guard !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(url) else {
            throw CancellationError()
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty else {
            throw URLError(.badURL)
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           Self.isConfiguredOrLocalGatewayHost(bareHost),
           req.value(forHTTPHeaderField: "Authorization") == nil {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let tlsOptions = NWProtocolTLS.Options()
        sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, bareHost)
        let parameters = NWParameters(tls: tlsOptions)

        guard let anycastIP = CloudflareAnycastAccelerator.verifiedPrimaryIP else {
            throw URLError(.cannotFindHost)
        }
        let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 443)) ?? .https

        let connection = NWConnection(host: NWEndpoint.Host(anycastIP), port: port, using: parameters)
        try await Self.waitUntilReady(connection, timeout: req.timeoutInterval > 0 ? min(req.timeoutInterval, 10) : 10)
        defer { connection.cancel() }

        let (data, response) = try await Self.streamFileUpload(
            connection: connection,
            request: req,
            url: url,
            bareHost: bareHost,
            fileURL: fileURL,
            onProgress: onProgress
        )
        if let http = response as? HTTPURLResponse,
           http.statusCode == 403,
           CloudflareAnycastAccelerator.isError1034Body(data) {
            CloudflareAnycastAccelerator.shared.reportEdgeRestricted(ip: anycastIP)
            throw URLError(.cannotConnectToHost)
        }
        return (data, decorateResponse(response, url: req.url, requestEpoch: reqEpoch))
    }

    private static func streamFileUpload(
        connection: NWConnection,
        request: URLRequest,
        url: URL,
        bareHost: String,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)?
    ) async throws -> (Data, URLResponse) {
        let handle = try FileHandle(forReadingFrom: fileURL)
        defer { try? handle.close() }
        let totalBytes = Int64(try handle.seekToEnd())
        try handle.seek(toOffset: 0)

        let head = HTTP11Codec.buildRequestHead(request, url: url, bareHost: bareHost, keepAlive: false, contentLength: totalBytes)
        try await sendAll(connection, head)

        let chunkSize = 32 * 1024
        var totalSent: Int64 = 0
        var lastPercent = -1
        while true {
            try Task.checkCancellation()
            guard let chunk = try handle.read(upToCount: chunkSize), !chunk.isEmpty else {
                break
            }
            try await sendAll(connection, chunk)
            totalSent += Int64(chunk.count)
            if totalBytes > 0 {
                // Cap streaming progress at 0.95 until the gateway finishes persisting and returns 200 OK.
                let rawProgress = max(0, min(1, Double(totalSent) / Double(totalBytes)))
                let scaled = min(0.95, rawProgress * 0.95)
                let percent = Int((scaled * 100).rounded(.down))
                if percent > lastPercent {
                    lastPercent = percent
                    onProgress?(scaled)
                }
            }
        }

        let (raw, _) = try await receiveHTTPMessage(connection, timeout: request.timeoutInterval > 0 ? max(request.timeoutInterval, 60) : 60)
        return try HTTP11Codec.parseResponse(raw, url: url)
    }

    /// Streams a file download over cleartext HTTP via `NWConnection` (for Tailscale ATS bypass),
    /// writing incoming chunks directly to a temporary file on disk and reporting throttled progress
    /// without buffering large files in memory.
    public func downloadCleartextHTTP(
        request: URLRequest,
        reqEpoch: UInt64? = nil,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)? = nil
    ) async throws -> (URL, HTTPURLResponse) {
        let effectiveEpoch = reqEpoch ?? GatewayRuntimeState.shared.epoch
        var req = request
        guard let url = req.url else {
            throw URLError(.badURL)
        }
        guard !GatewayRuntimeState.shared.isOriginOwnedByInactiveProfile(url) else {
            throw CancellationError()
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty, Self.isLocalOrPrivateHost(bareHost), let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 80)) else {
            throw URLError(.appTransportSecurityRequiresSecureConnection)
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           Self.isConfiguredOrLocalGatewayHost(bareHost),
           req.value(forHTTPHeaderField: "Authorization") == nil {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        // Request uncompressed bytes so `http.ServeContent` preserves exact `Content-Length` and streams directly to disk.
        if req.value(forHTTPHeaderField: "Accept-Encoding") == nil {
            req.setValue("identity", forHTTPHeaderField: "Accept-Encoding")
        }

        let connection = NWConnection(host: NWEndpoint.Host(bareHost), port: port, using: .tcp)
        try await Self.waitUntilReady(connection, timeout: req.timeoutInterval > 0 ? min(req.timeoutInterval, 15) : 15)
        defer { connection.cancel() }

        let payload = HTTP11Codec.buildRequest(req, url: url, bareHost: bareHost, keepAlive: false)
        try await Self.sendAll(connection, payload)

        let headerTerminator = Data("\r\n\r\n".utf8)
        let chunkIdleTimeout: TimeInterval = req.timeoutInterval > 0 ? min(max(req.timeoutInterval, 15), 60) : 30
        var headerBuffer = Data()
        let headerEnd: Range<Data.Index>
        while true {
            try Task.checkCancellation()
            let chunk = try await Self.receiveChunk(connection, timeout: chunkIdleTimeout)
            if chunk.isEmpty {
                throw URLError(.networkConnectionLost)
            }
            let searchStart = max(0, headerBuffer.count - 3)
            headerBuffer.append(chunk)
            if let found = headerBuffer.range(of: headerTerminator, in: searchStart..<headerBuffer.count) {
                headerEnd = found
                break
            }
            if headerBuffer.count > 65_536 {
                throw URLError(.cannotParseResponse)
            }
        }

        let parsedHeader = try HTTP11Codec.parseResponseHeader(headerBuffer.subdata(in: 0..<headerEnd.lowerBound))
        let initialBody = Data(headerBuffer[headerEnd.upperBound...])

        let tempFile = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        guard FileManager.default.createFile(atPath: tempFile.path, contents: nil) else {
            throw URLError(.cannotCreateFile)
        }
        var succeeded = false
        defer {
            if !succeeded {
                try? FileManager.default.removeItem(at: tempFile)
            }
        }

        let handle = try FileHandle(forWritingTo: tempFile)
        var handleClosed = false
        defer {
            if !handleClosed {
                try? handle.close()
            }
        }

        let expectedTotal: Int64 = {
            if case .length(let len) = parsedHeader.framing, !parsedHeader.isGzip {
                return len
            }
            return -1
        }()

        var written: Int64 = 0
        var lastReportedBytes: Int64 = 0
        var lastReportedTime: CFAbsoluteTime = 0

        let writeSlice: (Data) throws -> Void = { slice in
            guard !slice.isEmpty else { return }
            try handle.write(contentsOf: slice)
            written += Int64(slice.count)
            guard let onProgress else { return }
            let now = CFAbsoluteTimeGetCurrent()
            let isCompleted = expectedTotal > 0 && written >= expectedTotal
            let shouldReport = isCompleted
                || lastReportedBytes == 0
                || (now - lastReportedTime) >= 0.05
                || (written - lastReportedBytes) >= 262_144
            if shouldReport {
                lastReportedBytes = written
                lastReportedTime = now
                let progress = expectedTotal > 0 ? max(0, min(1.0, Double(written) / Double(expectedTotal))) : 0.0
                onProgress(progress, written, expectedTotal)
            }
        }

        switch parsedHeader.framing {
        case .length(let contentLength):
            if contentLength > 0 && !initialBody.isEmpty {
                let take = Int(min(contentLength, Int64(initialBody.count)))
                try writeSlice(Data(initialBody.prefix(take)))
            }
            while written < contentLength {
                try Task.checkCancellation()
                let chunk = try await Self.receiveChunk(connection, timeout: chunkIdleTimeout)
                if chunk.isEmpty {
                    throw URLError(.networkConnectionLost)
                }
                let remaining = contentLength - written
                let take = Int(min(remaining, Int64(chunk.count)))
                try writeSlice(Data(chunk.prefix(take)))
            }
        case .chunked:
            var decoder = HTTP11Codec.ChunkedStreamDecoder()
            if !initialBody.isEmpty {
                try decoder.feed(initialBody, write: writeSlice)
            }
            while !decoder.isDone {
                try Task.checkCancellation()
                let chunk = try await Self.receiveChunk(connection, timeout: chunkIdleTimeout)
                if chunk.isEmpty {
                    throw URLError(.networkConnectionLost)
                }
                try decoder.feed(chunk, write: writeSlice)
            }
        case .untilClose:
            if !initialBody.isEmpty {
                try writeSlice(initialBody)
            }
            while true {
                try Task.checkCancellation()
                let chunk = try await Self.receiveChunk(connection, timeout: chunkIdleTimeout)
                if chunk.isEmpty {
                    break
                }
                try writeSlice(chunk)
            }
        }

        try handle.close()
        handleClosed = true

        if parsedHeader.isGzip && written > 0 {
            let raw = try Data(contentsOf: tempFile)
            guard let inflated = HTTP11Codec.gunzip(raw) else {
                throw URLError(.cannotDecodeContentData)
            }
            try inflated.write(to: tempFile, options: .atomic)
            written = Int64(inflated.count)
        }

        if expectedTotal > 0 {
            onProgress?(1.0, expectedTotal, expectedTotal)
        } else if written > 0 {
            onProgress?(1.0, written, written)
        }

        guard let rawResp = HTTP11Codec.makeHTTPResponse(url: url, header: parsedHeader, decodedContentLength: written),
              let httpResp = decorateResponse(rawResp, url: req.url, forcedCellular: isCellular, requestEpoch: effectiveEpoch) as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }

        succeeded = true
        return (tempFile, httpResp)
    }

    private func sendCleartextHTTP(_ request: URLRequest) async throws -> (Data, URLResponse) {
        guard let url = request.url else {
            throw URLError(.badURL)
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty, Self.isLocalOrPrivateHost(bareHost), let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 80)) else {
            throw URLError(.appTransportSecurityRequiresSecureConnection)
        }
        
        let connection = NWConnection(host: NWEndpoint.Host(bareHost), port: port, using: .tcp)
        try await Self.waitUntilReady(connection, timeout: request.timeoutInterval > 0 ? request.timeoutInterval : 8)
        defer { connection.cancel() }
        
        let payload = HTTP11Codec.buildRequest(request, url: url, bareHost: bareHost, keepAlive: false)
        try await Self.sendAll(connection, payload)
        let (raw, _) = try await Self.receiveHTTPMessage(connection, timeout: request.timeoutInterval > 0 ? max(request.timeoutInterval, 30) : 30)
        return try HTTP11Codec.parseResponse(raw, url: url)
    }
    
    private static func waitUntilReady(_ connection: NWConnection, timeout: TimeInterval) async throws {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
                let finished = OSAllocatedUnfairLock(initialState: false)
                @Sendable func complete(_ result: Result<Void, Error>) {
                    let go = finished.withLock { flag -> Bool in
                        if flag { return false }
                        flag = true
                        return true
                    }
                    guard go else { return }
                    cont.resume(with: result)
                }
                
                connection.stateUpdateHandler = { state in
                    switch state {
                    case .ready:
                        complete(.success(()))
                    case .failed(let err):
                        complete(.failure(err))
                    case .cancelled:
                        complete(.failure(URLError(.cancelled)))
                    default:
                        break
                    }
                }
                connection.start(queue: DispatchQueue.global(qos: .userInitiated))
                
                DispatchQueue.global(qos: .userInitiated).asyncAfter(deadline: .now() + timeout) {
                    let timedOut = finished.withLock { flag -> Bool in
                        if flag { return false }
                        flag = true
                        return true
                    }
                    if timedOut {
                        connection.cancel()
                        cont.resume(throwing: URLError(.timedOut))
                    }
                }
            }
        } onCancel: {
            connection.cancel()
        }
    }
    
    private static func sendAll(_ connection: NWConnection, _ data: Data) async throws {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
                connection.send(content: data, completion: .contentProcessed { error in
                    if let error {
                        cont.resume(throwing: error)
                    } else {
                        cont.resume()
                    }
                })
            }
        } onCancel: {
            connection.cancel()
        }
    }
    
    private static let maxRawResponseBytes = 64 * 1024 * 1024

    private static func receiveChunk(_ connection: NWConnection, timeout: TimeInterval) async throws -> Data {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Data, Error>) in
                let finished = OSAllocatedUnfairLock(initialState: false)
                @Sendable func complete(_ result: Result<Data, Error>) {
                    let go = finished.withLock { flag -> Bool in
                        if flag { return false }
                        flag = true
                        return true
                    }
                    guard go else { return }
                    cont.resume(with: result)
                }

                connection.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { content, _, _, error in
                    if let error {
                        complete(.failure(error))
                    } else {
                        complete(.success(content ?? Data()))
                    }
                }

                DispatchQueue.global(qos: .userInitiated).asyncAfter(deadline: .now() + timeout) {
                    let timedOut = finished.withLock { flag -> Bool in
                        if flag { return false }
                        flag = true
                        return true
                    }
                    if timedOut {
                        connection.cancel()
                        cont.resume(throwing: URLError(.timedOut))
                    }
                }
            }
        } onCancel: {
            connection.cancel()
        }
    }

    private static func receiveHTTPMessage(_ connection: NWConnection, timeout: TimeInterval = 30) async throws -> (Data, Bool) {
        var accumulator = HTTP11Codec.ResponseAccumulator()
        let deadline = Date().addingTimeInterval(timeout)
        while true {
            let remaining = deadline.timeIntervalSinceNow
            if remaining <= 0 {
                connection.cancel()
                throw URLError(.timedOut)
            }
            let chunk = try await receiveChunk(connection, timeout: min(remaining, 30))
            if accumulator.buffer.count + chunk.count > maxRawResponseBytes {
                connection.cancel()
                throw URLError(.dataLengthExceedsMaximum)
            }
            if accumulator.append(chunk) {
                return (accumulator.buffer, accumulator.isUntilClose)
            }
            if chunk.isEmpty {
                if accumulator.buffer.isEmpty {
                    throw URLError(.networkConnectionLost)
                }
                return (accumulator.buffer, true)
            }
        }
    }
}

// MARK: - File Download Progress Delegate

private final class FileDownloadProgressDelegate: NSObject, URLSessionDownloadDelegate, @unchecked Sendable {
    let onProgress: (@Sendable (Double, Int64, Int64) -> Void)?
    private let lock = NSLock()
    private var continuation: CheckedContinuation<(URL, HTTPURLResponse), Error>?
    private var pendingResult: Result<(URL, HTTPURLResponse), Error>?
    private var lastReportedBytes: Int64 = 0
    private var lastReportedTime: CFAbsoluteTime = 0

    init(onProgress: (@Sendable (Double, Int64, Int64) -> Void)?) {
        self.onProgress = onProgress
    }

    func setContinuation(_ cont: CheckedContinuation<(URL, HTTPURLResponse), Error>) {
        let immediate: Result<(URL, HTTPURLResponse), Error>?
        lock.lock()
        if let pending = pendingResult {
            pendingResult = nil
            immediate = pending
        } else {
            continuation = cont
            immediate = nil
        }
        lock.unlock()
        if let immediate {
            cont.resume(with: immediate)
        }
    }

    func cancel() {
        finish(with: .failure(CancellationError()))
    }

    @discardableResult
    private func finish(with result: Result<(URL, HTTPURLResponse), Error>) -> Bool {
        let cont: CheckedContinuation<(URL, HTTPURLResponse), Error>?
        var accepted = false
        lock.lock()
        if let c = continuation {
            continuation = nil
            cont = c
            accepted = true
        } else if pendingResult == nil {
            pendingResult = result
            cont = nil
            accepted = true
        } else {
            cont = nil
        }
        lock.unlock()
        cont?.resume(with: result)
        return accepted
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64, totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        guard let onProgress = onProgress else { return }
        if let httpResp = downloadTask.response as? HTTPURLResponse,
           !(200...299).contains(httpResp.statusCode) {
            return
        }
        let now = CFAbsoluteTimeGetCurrent()

        lock.lock()
        // 严格保证单调递增，丢弃网络乱序或重置导致的倒退包
        if totalBytesWritten < lastReportedBytes && totalBytesWritten > 0 {
            lock.unlock()
            return
        }

        let isCompleted = totalBytesExpectedToWrite > 0 && totalBytesWritten >= totalBytesExpectedToWrite
        let timeElapsed = now - lastReportedTime
        let bytesDelta = totalBytesWritten - lastReportedBytes

        // 节流策略：首包、尾包(完成)、间隔超50ms、或增量超256KB时上报，避免主线程状态风暴
        let shouldReport = isCompleted || lastReportedBytes == 0 || timeElapsed >= 0.05 || bytesDelta >= 262_144
        if shouldReport {
            lastReportedBytes = totalBytesWritten
            lastReportedTime = now
            lock.unlock()

            let progress = totalBytesExpectedToWrite > 0 ? max(0, min(1.0, Double(totalBytesWritten) / Double(totalBytesExpectedToWrite))) : 0.0
            onProgress(progress, totalBytesWritten, totalBytesExpectedToWrite)
        } else {
            lock.unlock()
        }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let httpResp = downloadTask.response as? HTTPURLResponse else {
            finish(with: .failure(URLError(.badServerResponse)))
            return
        }
        if (200...299).contains(httpResp.statusCode) {
            let total = downloadTask.countOfBytesExpectedToReceive
            let written = downloadTask.countOfBytesReceived
            if total > 0 {
                onProgress?(1.0, total, total)
            } else if written > 0 {
                onProgress?(1.0, written, written)
            }
        }
        let tempFile = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        do {
            try FileManager.default.moveItem(at: location, to: tempFile)
            if !finish(with: .success((tempFile, httpResp))) {
                try? FileManager.default.removeItem(at: tempFile)
            }
        } catch {
            finish(with: .failure(error))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error = error {
            finish(with: .failure(error))
        } else {
            finish(with: .failure(URLError(.unknown)))
        }
    }
}
