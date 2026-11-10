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
            return "指令已成功送达服务器，但等待响应超时 (\(err.localizedDescription))"
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
        config.timeoutIntervalForResource = 30
        config.httpShouldSetCookies = false
        config.httpCookieAcceptPolicy = .never
        config.httpCookieStorage = nil
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        self.fallbackSession = URLSession(configuration: config, delegate: nil, delegateQueue: nil)
    }
    
    /// Decorates HTTPURLResponse with X-Antigravity-Interface header to indicate actual interface used
    private func decorateResponse(_ response: URLResponse, url: URL?, forcedCellular: Bool = false) -> URLResponse {
        guard let http = response as? HTTPURLResponse, let targetURL = url ?? http.url else {
            return response
        }
        // Self-heal primary Cloud URL only when advertised by an authenticated, configured/local gateway over HTTPS
        if let cloudHeader = http.value(forHTTPHeaderField: "X-Antigravity-Cloud-URL")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !cloudHeader.isEmpty,
           Self.isConfiguredOrLocalGatewayHost(targetURL.host ?? ""),
           Self.isValidAdvertisedCloudURL(cloudHeader) {
            Task { @MainActor in
                if AppSettings.shared.primaryCloudURL != cloudHeader {
                    AppSettings.shared.primaryCloudURL = cloudHeader
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
        if let colonIdx = clean.lastIndex(of: ":") {
            let port = clean[clean.index(after: colonIdx)...]
            if !port.isEmpty && port.allSatisfy({ $0.isNumber }) {
                clean = String(clean[..<colonIdx])
            }
        }
        return clean.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
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
        // Tailscale CGNAT range (100.64.0.0/10) or any Tailscale virtual node (*.ts.net)
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
        if clean.hasSuffix(".ts.net") { return true }
        // Tailscale CGNAT range 100.64.0.0/10
        let parts = clean.split(separator: ".")
        if parts.count == 4,
           let first = Int(parts[0]), first == 100,
           let second = Int(parts[1]), second >= 64, second <= 127,
           let third = Int(parts[2]), third >= 0, third <= 255,
           let fourth = Int(parts[3]), fourth >= 0, fourth <= 255 {
            return true
        }
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

    nonisolated public static func isConfiguredOrLocalGatewayHost(_ host: String) -> Bool {
        let clean = extractHost(from: host)
        if clean.isEmpty { return false }
        if isLocalOrPrivateHost(clean) || clean == "demo.antigravity.local" {
            return true
        }
        return configuredGatewayHosts().contains(clean)
    }

    nonisolated public static func isValidAdvertisedCloudURL(_ raw: String) -> Bool {
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
        if host.hasSuffix(".trycloudflare.com") || host.hasSuffix(".jiuge.space") {
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

    /// Sends a request using URLSession, except cleartext HTTP to Tailscale private tunnel nodes
    /// which App Transport Security blocks. Cleartext HTTP to public internet hosts is rejected.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        var req = request
        if let url = req.url, url.scheme?.lowercased() == "http" {
            let host = url.host ?? ""
            guard Self.isLocalOrPrivateHost(host) else {
                throw URLError(.appTransportSecurityRequiresSecureConnection)
            }
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           let targetHost = req.url?.host, Self.isConfiguredOrLocalGatewayHost(targetHost) {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }
        
        if Self.requiresCleartextATSBypass(req.url) {
            let (data, response) = try await sendCleartextHTTP(req)
            return (data, decorateResponse(response, url: req.url, forcedCellular: preferCellular || isCellular))
        }
        
        if let url = req.url, url.scheme?.lowercased() == "https", CloudflareAnycastAccelerator.isCloudflareTunnelHost(url.host ?? "") {
            do {
                let (data, response) = try await sendCloudflareAcceleratedHTTPS(req)
                return (data, decorateResponse(response, url: req.url))
            } catch {
                // If Anycast acceleration fails, smoothly fall back to system URLSession
            }
        }
        
        let (data, response) = try await fallbackSession.data(for: req)
        return (data, decorateResponse(response, url: req.url))
    }
    
    /// ATS blocks cleartext HTTP to Tailscale private hosts (NSAllowsLocalNetworking only exempts RFC1918/loopback/.local).
    /// Only Tailscale WireGuard private hosts (100.64.0.0/10, *.ts.net) may use NWConnection cleartext transport; public cleartext is rejected above.
    nonisolated public static func requiresCleartextATSBypass(_ url: URL?) -> Bool {
        guard let url, url.scheme?.lowercased() == "http" else { return false }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        return isTailscaleHost(host)
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
    }
    
    private func sendCloudflareAcceleratedHTTPS(_ request: URLRequest) async throws -> (Data, URLResponse) {
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
        
        let anycastIP = CloudflareAnycastAccelerator.primaryIP
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
    }
    
    private static func sendAll(_ connection: NWConnection, _ data: Data) async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            connection.send(content: data, completion: .contentProcessed { error in
                if let error {
                    cont.resume(throwing: error)
                } else {
                    cont.resume()
                }
            })
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
