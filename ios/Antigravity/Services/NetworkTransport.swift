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
        self.fallbackSession = URLSession(configuration: config, delegate: nil, delegateQueue: nil)
    }
    
    /// Decorates HTTPURLResponse with X-Antigravity-Interface header to indicate actual interface used
    private func decorateResponse(_ response: URLResponse, url: URL?, forcedCellular: Bool = false) -> URLResponse {
        guard let http = response as? HTTPURLResponse, let targetURL = url ?? http.url else {
            return response
        }
        // Self-heal primary Cloud URL if advertised by gateway
        if let cloudHeader = http.value(forHTTPHeaderField: "X-Antigravity-Cloud-URL")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !cloudHeader.isEmpty {
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
        // Tailscale CGNAT range (100.64.0.0/10) or any Tailscale virtual node
        if clean.hasPrefix("100.") || clean.hasSuffix(".ts.net") {
            return true
        }
        // IPv6 Link-Local (fe80::/10) and Unique Local Address ULA (fc00::/7, fd00::/8)
        if clean.hasPrefix("fe80:") || clean.hasPrefix("fc") || clean.hasPrefix("fd") {
            return true
        }
        return false
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

    /// Sends a request using URLSession, except cleartext HTTP to IPv6 literals
    /// which App Transport Security blocks. Those go through Network.framework.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        var req = request
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }
        
        if Self.requiresCleartextATSBypass(req.url) {
            let (data, response) = try await sendCleartextHTTP(req)
            return (data, decorateResponse(response, url: req.url, forcedCellular: preferCellular || isCellular))
        }
        
        let (data, response) = try await fallbackSession.data(for: req)
        return (data, decorateResponse(response, url: req.url))
    }
    
    /// ATS blocks cleartext HTTP to public hosts (NSAllowsArbitraryLoads is false).
    /// IPv6 literals and the cloud FRP relay must still work, so those go through Network.framework.
    nonisolated public static func requiresCleartextATSBypass(_ url: URL?) -> Bool {
        guard let url, url.scheme?.lowercased() == "http" else { return false }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if host.contains(":") {
            return true
        }
        return !isLocalOrPrivateHost(host)
    }
    
    private func sendCleartextHTTP(_ request: URLRequest) async throws -> (Data, URLResponse) {
        guard let url = request.url else {
            throw URLError(.badURL)
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty, let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 80)) else {
            throw URLError(.badURL)
        }
        
        let connection = NWConnection(host: NWEndpoint.Host(bareHost), port: port, using: .tcp)
        try await Self.waitUntilReady(connection, timeout: request.timeoutInterval > 0 ? request.timeoutInterval : 8)
        defer { connection.cancel() }
        
        let payload = Self.buildHTTP11Request(request, url: url, bareHost: bareHost)
        try await Self.sendAll(connection, payload)
        let raw = try await Self.receiveHTTPMessage(connection)
        return try Self.parseHTTP11Response(raw, url: url)
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
    
    private static func buildHTTP11Request(_ request: URLRequest, url: URL, bareHost: String) -> Data {
        var path = url.path.isEmpty ? "/" : url.path
        if let query = url.query, !query.isEmpty {
            path += "?" + query
        }
        let method = request.httpMethod ?? "GET"
        let port = url.port ?? 80
        let hostHeader: String
        if bareHost.contains(":") {
            hostHeader = "[\(bareHost)]:\(port)"
        } else if port == 80 {
            hostHeader = bareHost
        } else {
            hostHeader = "\(bareHost):\(port)"
        }
        
        var lines: [String] = [
            "\(method) \(path) HTTP/1.1",
            "Host: \(hostHeader)",
            "Connection: close"
        ]
        if let headers = request.allHTTPHeaderFields {
            for (key, value) in headers {
                if key.lowercased() == "host" || key.lowercased() == "connection" { continue }
                lines.append("\(key): \(value)")
            }
        }
        let body = request.httpBody ?? Data()
        if request.value(forHTTPHeaderField: "Content-Length") == nil {
            lines.append("Content-Length: \(body.count)")
        }
        var data = Data(lines.joined(separator: "\r\n").utf8)
        data.append(Data("\r\n\r\n".utf8))
        data.append(body)
        return data
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
    
    private static func receiveHTTPMessage(_ connection: NWConnection) async throws -> Data {
        var buffer = Data()
        while true {
            let chunk: Data = try await withCheckedThrowingContinuation { cont in
                connection.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { content, _, _, error in
                    if let error {
                        cont.resume(throwing: error)
                        return
                    }
                    cont.resume(returning: content ?? Data())
                }
            }
            buffer.append(chunk)
            if httpMessageComplete(buffer) {
                return buffer
            }
            if chunk.isEmpty {
                if buffer.isEmpty {
                    throw URLError(.networkConnectionLost)
                }
                return buffer
            }
        }
    }
    
    private static func httpMessageComplete(_ data: Data) -> Bool {
        guard let headerEnd = data.range(of: Data("\r\n\r\n".utf8)) else { return false }
        let headerData = data.subdata(in: data.startIndex..<headerEnd.lowerBound)
        guard let headerText = String(data: headerData, encoding: .isoLatin1) else { return false }
        let body = data.subdata(in: headerEnd.upperBound..<data.endIndex)
        let lines = headerText.split(separator: "\r\n")
        var isChunked = false
        for line in lines {
            let parts = line.split(separator: ":", maxSplits: 1)
            if parts.count == 2 {
                let name = parts[0].trimmingCharacters(in: .whitespaces).lowercased()
                let value = parts[1].trimmingCharacters(in: .whitespaces).lowercased()
                if name == "content-length" {
                    let n = Int(value) ?? 0
                    return body.count >= n
                }
                if name == "transfer-encoding" && value.contains("chunked") {
                    isChunked = true
                }
            }
        }
        if isChunked {
            if body.range(of: Data("\r\n0\r\n\r\n".utf8)) != nil || body.starts(with: Data("0\r\n\r\n".utf8)) {
                return true
            }
        }
        return false
    }
    
    private static func decodeChunkedBody(_ data: Data) throws -> Data {
        var unchunked = Data()
        var offset = 0
        let crlf = Data("\r\n".utf8)
        
        while offset < data.count {
            guard let range = data.range(of: crlf, options: [], in: offset..<data.count) else {
                break
            }
            let sizeData = data.subdata(in: offset..<range.lowerBound)
            guard let sizeStr = String(data: sizeData, encoding: .ascii)?.trimmingCharacters(in: .whitespaces) else {
                throw URLError(.cannotParseResponse)
            }
            if sizeStr.isEmpty {
                offset = range.upperBound
                continue
            }
            let hexStr = sizeStr.split(separator: ";").first.map(String.init) ?? sizeStr
            guard let chunkSize = Int(hexStr.trimmingCharacters(in: .whitespaces), radix: 16) else {
                throw URLError(.cannotParseResponse)
            }
            if chunkSize == 0 {
                break
            }
            let chunkStart = range.upperBound
            let chunkEnd = chunkStart + chunkSize
            guard chunkEnd <= data.count else {
                throw URLError(.cannotParseResponse)
            }
            unchunked.append(data.subdata(in: chunkStart..<chunkEnd))
            offset = chunkEnd
            if offset + 2 <= data.count && data.subdata(in: offset..<offset + 2) == crlf {
                offset += 2
            }
        }
        return unchunked
    }
    
    private static func parseHTTP11Response(_ data: Data, url: URL) throws -> (Data, URLResponse) {
        guard let headerEnd = data.range(of: Data("\r\n\r\n".utf8)) else {
            throw URLError(.cannotParseResponse)
        }
        let headerText = String(data: data.subdata(in: data.startIndex..<headerEnd.lowerBound), encoding: .isoLatin1) ?? ""
        var lines = headerText.split(separator: "\r\n", omittingEmptySubsequences: false).map(String.init)
        guard let statusLine = lines.first else {
            throw URLError(.cannotParseResponse)
        }
        lines.removeFirst()
        let statusParts = statusLine.split(separator: " ")
        let code = statusParts.count >= 2 ? (Int(statusParts[1]) ?? 500) : 500
        
        var fields: [String: String] = [:]
        for line in lines {
            guard let colon = line.firstIndex(of: ":") else { continue }
            let key = String(line[..<colon])
            let value = line[line.index(after: colon)...].trimmingCharacters(in: .whitespaces)
            fields[key] = value
        }
        var body = data.subdata(in: headerEnd.upperBound..<data.endIndex)
        let isChunked = fields.contains { $0.key.lowercased() == "transfer-encoding" && $0.value.lowercased().contains("chunked") }
        if isChunked {
            body = try decodeChunkedBody(body)
        } else if let lenStr = fields.first(where: { $0.key.lowercased() == "content-length" })?.value,
           let len = Int(lenStr), len >= 0, body.count > len {
            body = body.prefix(len)
        }
        let response = HTTPURLResponse(url: url, statusCode: code, httpVersion: "HTTP/1.1", headerFields: fields)
            ?? URLResponse(url: url, mimeType: nil, expectedContentLength: body.count, textEncodingName: nil)
        return (body, response)
    }
}
