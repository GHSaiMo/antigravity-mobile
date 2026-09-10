import Foundation
import Network

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
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
        config.httpShouldSetCookies = true
        config.httpCookieAcceptPolicy = .always
        config.httpCookieStorage = HTTPCookieStorage.shared
        self.fallbackSession = URLSession(configuration: config)
    }
    
    /// Sends a request prioritizing the cellular interface (IPv6 direct) if requested and available,
    nonisolated public static func isLocalOrPrivateHost(_ host: String) -> Bool {
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
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
        if clean.hasPrefix("100.") {
            return true
        }
        // IPv6 Link-Local (fe80::/10) and Unique Local Address ULA (fc00::/7, fd00::/8)
        if clean.hasPrefix("fe80:") || clean.hasPrefix("fc") || clean.hasPrefix("fd") {
            return true
        }
        return false
     }

    /// Sends a request prioritizing the cellular interface (IPv6 direct) if requested and available,
    /// otherwise seamlessly falls back to standard URLSession routing.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        var req = request
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }
        
        guard preferCellular, let url = req.url, let host = url.host else {
            return try await fallbackSession.data(for: req)
        }
        
        if Self.isLocalOrPrivateHost(host) {
            return try await fallbackSession.data(for: req)
        }
        
        do {
            return try await executeViaCellular(request: req, url: url, host: host)
        } catch let NetworkTransportError.requestAlreadyDispatched(underlying) {
            let method = (req.httpMethod ?? "GET").uppercased()
            let path = req.url?.path ?? ""
            // In ConnectRPC, all RPC methods (including read queries like GetAllCascadeTrajectories and GetCascadeTrajectory)
            // use HTTP POST. Only state-mutating requests like SendUserCascadeMessage create new messages and should suppress retry.
            // All queries and idempotent operations MUST seamlessly fall back to standard URLSession routing (Wi-Fi).
            let isNonIdempotentMutation = method == "POST" && path.contains("SendUserCascadeMessage")
            if !isNonIdempotentMutation {
                print("[NetworkTransport] Cellular direct response read failed after send, retrying idempotent request (\(path)) via standard interface: \(underlying.localizedDescription)")
                return try await fallbackSession.data(for: req)
            } else {
                print("[NetworkTransport] Cellular direct request was already sent to server, but response failed (\(underlying.localizedDescription)). Suppressing fallback retry for non-idempotent \(path) to prevent duplicate execution.")
                throw APIError.networkError("指令已成功送达服务器，但等待响应超时 (\(underlying.localizedDescription))")
            }
        } catch {
            print("[NetworkTransport] Cellular direct request failed before send (\(error.localizedDescription)), falling back to standard interface")
            return try await fallbackSession.data(for: req)
        }
    }
    
    private func executeViaCellular(request: URLRequest, url: URL, host: String) async throws -> (Data, HTTPURLResponse) {
        let portNumber = url.port ?? (url.scheme?.lowercased() == "https" ? 443 : 80)
        guard let port = NWEndpoint.Port(rawValue: UInt16(portNumber)) else {
            throw URLError(.badURL)
        }
        
        let cleanHost = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        let endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(cleanHost), port: port)
        let parameters: NWParameters
        if url.scheme?.lowercased() == "https" {
            let tlsOptions = NWProtocolTLS.Options()
            let hostStr = cleanHost.lowercased()
            let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"
            sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { (metadata, trust, completion) in
                if isLoopback {
                    // Trust self-signed certificates only for local gateway connections
                    completion(true)
                } else {
                    // Standard X.509 trust evaluation for all remote hosts
                    let secTrust = sec_trust_copy_ref(trust).takeRetainedValue()
                    SecTrustEvaluateAsyncWithError(secTrust, DispatchQueue.global()) { _, result, _ in
                        completion(result)
                    }
                }
            }, .global())
            parameters = NWParameters(tls: tlsOptions)
        } else {
            parameters = NWParameters.tcp
        }
        
        // Pin to cellular interface to use mobile carrier's native IPv6
        parameters.requiredInterfaceType = .cellular
        
        let connection = NWConnection(to: endpoint, using: parameters)
        let method = request.httpMethod ?? "GET"
        let body = request.httpBody
        // Cap cellular timeout to 8.0s so that network blocks/drops don't cause prolonged UI freezing before fallback
        let timeoutInterval: TimeInterval = min(request.timeoutInterval > 0 ? request.timeoutInterval : 8.0, 8.0)
        
        return try await withCheckedThrowingContinuation { continuation in
            final class SyncState: @unchecked Sendable {
                var isCompleted = false
                var requestDispatched = false
                var timeoutWork: DispatchWorkItem?
                var waitingTimerWork: DispatchWorkItem?
                var connection: NWConnection?
                var continuation: CheckedContinuation<(Data, HTTPURLResponse), any Error>?
                
                func finish(result: Result<(Data, HTTPURLResponse), any Error>) {
                    objc_sync_enter(self)
                    defer { objc_sync_exit(self) }
                    guard !isCompleted else { return }
                    isCompleted = true
                    timeoutWork?.cancel()
                    waitingTimerWork?.cancel()
                    connection?.cancel()
                    switch result {
                    case .success(let res):
                        continuation?.resume(returning: res)
                    case .failure(let err):
                        if requestDispatched {
                            continuation?.resume(throwing: NetworkTransportError.requestAlreadyDispatched(err))
                        } else {
                            continuation?.resume(throwing: err)
                        }
                    }
                }
            }
            let state = SyncState()
            state.continuation = continuation
            state.connection = connection
            
            let timeoutWork = DispatchWorkItem {
                state.finish(result: .failure(URLError(.timedOut)))
            }
            state.timeoutWork = timeoutWork
            DispatchQueue.global().asyncAfter(deadline: .now() + timeoutInterval, execute: timeoutWork)
            
            connection.stateUpdateHandler = { connState in
                switch connState {
                case .ready:
                    state.waitingTimerWork?.cancel()
                    state.waitingTimerWork = nil
                    
                    var pathAndQuery = url.path.isEmpty ? "/" : url.path
                    if let query = url.query {
                        pathAndQuery += "?\(query)"
                    }
                    
                    let formattedHost = cleanHost.contains(":") ? "[\(cleanHost)]" : cleanHost
                    var reqStr = "\(method) \(pathAndQuery) HTTP/1.1\r\n"
                    reqStr += "Host: \(formattedHost):\(portNumber)\r\n"
                    
                    if let headers = request.allHTTPHeaderFields {
                        for (k, v) in headers {
                            let lower = k.lowercased()
                            if lower != "host" && lower != "content-length" && lower != "connection" {
                                reqStr += "\(k): \(v)\r\n"
                            }
                        }
                    }
                    reqStr += "Connection: close\r\n"
                    if let body = body {
                        reqStr += "Content-Length: \(body.count)\r\n\r\n"
                    } else {
                        reqStr += "\r\n"
                    }
                    
                    var reqData = reqStr.data(using: .utf8) ?? Data()
                    if let body = body {
                        reqData.append(body)
                    }
                    
                    connection.send(content: reqData, completion: .contentProcessed { err in
                        if let err = err {
                            state.finish(result: .failure(err))
                            return
                        }
                        objc_sync_enter(state)
                        state.requestDispatched = true
                        objc_sync_exit(state)
                        
                        final class DataBuffer: @unchecked Sendable {
                            var buffer = Data()
                            func readNext(connection: NWConnection, state: SyncState, url: URL) {
                                connection.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, isComplete, err in
                                    guard let self = self else { return }
                                    if let data = data, !data.isEmpty {
                                        self.buffer.append(data)
                                    }
                                    
                                    // Complete immediately if Content-Length bytes have all been received
                                    if let (bodyPart, response) = NetworkTransport.parseHTTPResponse(data: self.buffer, url: url) {
                                        if let clStr = response.allHeaderFields["Content-Length"] as? String ?? (response.allHeaderFields["content-length"] as? String),
                                           let cl = Int(clStr), bodyPart.count >= cl {
                                            state.finish(result: .success((bodyPart, response)))
                                            return
                                        }
                                    }
                                    
                                    if isComplete || err != nil {
                                        if let (bodyPart, response) = NetworkTransport.parseHTTPResponse(data: self.buffer, url: url) {
                                            state.finish(result: .success((bodyPart, response)))
                                        } else if let err = err {
                                            state.finish(result: .failure(err))
                                        } else {
                                            state.finish(result: .failure(URLError(.badServerResponse)))
                                        }
                                        return
                                    }
                                    self.readNext(connection: connection, state: state, url: url)
                                }
                            }
                        }
                        let dataBuffer = DataBuffer()
                        dataBuffer.readNext(connection: connection, state: state, url: url)
                    })
                case .waiting(let err):
                    // When requiredInterfaceType = .cellular, .waiting indicates cellular interface
                    // is bringing up its radio carrier or waiting for DNS resolution.
                    // Allow up to 6.0s for carrier radio warmup before timing out.
                    // Stored in state.waitingTimerWork so it gets canceled once .ready is reached.
                    if state.waitingTimerWork == nil {
                        let work = DispatchWorkItem { [weak state] in
                            state?.finish(result: .failure(err))
                        }
                        state.waitingTimerWork = work
                        DispatchQueue.global().asyncAfter(deadline: .now() + 6.0, execute: work)
                    }
                case .failed(let err):
                    state.finish(result: .failure(err))
                default:
                    break
                }
            }
            
            connection.start(queue: .global())
        }
    }
    
    nonisolated private static func parseHTTPResponse(data: Data, url: URL) -> (Data, HTTPURLResponse)? {
        guard let separatorRange = data.range(of: Data("\r\n\r\n".utf8)) else {
            return nil
        }
        let headerData = data.subdata(in: 0..<separatorRange.lowerBound)
        var bodyData = data.subdata(in: separatorRange.upperBound..<data.count)
        
        guard let headerString = String(data: headerData, encoding: .utf8) else {
            return nil
        }
        
        let lines = headerString.components(separatedBy: "\r\n")
        guard let statusLine = lines.first else { return nil }
        let statusParts = statusLine.components(separatedBy: " ")
        guard statusParts.count >= 2, let statusCode = Int(statusParts[1]) else {
            return nil
        }
        
        var headers: [String: String] = [:]
        for line in lines.dropFirst() {
            if let colonIndex = line.firstIndex(of: ":") {
                let key = String(line[..<colonIndex]).trimmingCharacters(in: .whitespaces)
                let value = String(line[line.index(after: colonIndex)...]).trimmingCharacters(in: .whitespaces)
                headers[key] = value
            }
        }
        
        if headers["Transfer-Encoding"]?.lowercased().contains("chunked") == true {
            bodyData = dechunk(data: bodyData)
        }
        
        headers["X-Antigravity-Interface"] = "cellular"
        
        guard let response = HTTPURLResponse(
            url: url,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: headers
        ) else {
            return nil
        }
        return (bodyData, response)
    }
    
    nonisolated private static func dechunk(data: Data) -> Data {
        var result = Data()
        var offset = data.startIndex
        while offset < data.endIndex {
            guard let crlfRange = data.range(of: Data("\r\n".utf8), in: offset..<data.endIndex) else {
                break
            }
            let lengthData = data.subdata(in: offset..<crlfRange.lowerBound)
            guard let lengthStr = String(data: lengthData, encoding: .utf8)?.trimmingCharacters(in: .whitespaces),
                  let chunkSize = Int(lengthStr, radix: 16) else {
                break
            }
            if chunkSize == 0 { break }
            let chunkStart = crlfRange.upperBound
            let chunkEnd = chunkStart + chunkSize
            if chunkEnd <= data.endIndex {
                result.append(data.subdata(in: chunkStart..<chunkEnd))
                offset = chunkEnd
                if offset + 2 <= data.endIndex && data.subdata(in: offset..<(offset+2)) == Data("\r\n".utf8) {
                    offset += 2
                }
            } else {
                break
            }
        }
        return result
    }
}
