import Foundation
import Network

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
    /// otherwise seamlessly falls back to standard URLSession routing.
    public func send(request: URLRequest, preferCellular: Bool = true) async throws -> (Data, URLResponse) {
        guard preferCellular, let url = request.url, let host = url.host else {
            return try await fallbackSession.data(for: request)
        }
        
        do {
            return try await executeViaCellular(request: request, url: url, host: host)
        } catch {
            // Cellular connection failed or interface unavailable; graceful fallback to default interface
            return try await fallbackSession.data(for: request)
        }
    }
    
    private func executeViaCellular(request: URLRequest, url: URL, host: String) async throws -> (Data, HTTPURLResponse) {
        let portNumber = url.port ?? (url.scheme?.lowercased() == "https" ? 443 : 80)
        guard let port = NWEndpoint.Port(rawValue: UInt16(portNumber)) else {
            throw URLError(.badURL)
        }
        
        let endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(host), port: port)
        let parameters: NWParameters
        if url.scheme?.lowercased() == "https" {
            let tlsOptions = NWProtocolTLS.Options()
            sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { (_, _, completion) in
                completion(true)
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
        let timeoutInterval = request.timeoutInterval > 0 ? request.timeoutInterval : 10.0
        
        return try await withCheckedThrowingContinuation { continuation in
            final class SyncState: @unchecked Sendable {
                var isCompleted = false
                var timeoutWork: DispatchWorkItem?
            }
            let state = SyncState()
            
            func finish(result: Result<(Data, HTTPURLResponse), Error>) {
                objc_sync_enter(state)
                defer { objc_sync_exit(state) }
                guard !state.isCompleted else { return }
                state.isCompleted = true
                state.timeoutWork?.cancel()
                connection.cancel()
                switch result {
                case .success(let res):
                    continuation.resume(returning: res)
                case .failure(let err):
                    continuation.resume(throwing: err)
                }
            }
            
            let timeoutWork = DispatchWorkItem {
                finish(result: .failure(URLError(.timedOut)))
            }
            state.timeoutWork = timeoutWork
            DispatchQueue.global().asyncAfter(deadline: .now() + timeoutInterval, execute: timeoutWork)
            
            connection.stateUpdateHandler = { connState in
                switch connState {
                case .ready:
                    var pathAndQuery = url.path.isEmpty ? "/" : url.path
                    if let query = url.query {
                        pathAndQuery += "?\(query)"
                    }
                    
                    var reqStr = "\(method) \(pathAndQuery) HTTP/1.1\r\n"
                    reqStr += "Host: \(host):\(portNumber)\r\n"
                    
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
                            finish(result: .failure(err))
                            return
                        }
                        
                        final class DataBuffer: @unchecked Sendable {
                            var buffer = Data()
                        }
                        let dataBuffer = DataBuffer()
                        
                        func readNext() {
                            connection.receive(minimumIncompleteLength: 1, maximumLength: 65536) { data, _, isComplete, err in
                                if let data = data, !data.isEmpty {
                                    dataBuffer.buffer.append(data)
                                }
                                
                                if isComplete || err != nil {
                                    if let (bodyPart, response) = Self.parseHTTPResponse(data: dataBuffer.buffer, url: url) {
                                        finish(result: .success((bodyPart, response)))
                                    } else if let err = err {
                                        finish(result: .failure(err))
                                    } else {
                                        finish(result: .failure(URLError(.badServerResponse)))
                                    }
                                    return
                                }
                                readNext()
                            }
                        }
                        readNext()
                    })
                case .failed(let err):
                    finish(result: .failure(err))
                default:
                    break
                }
            }
            
            connection.start(queue: .global())
        }
    }
    
    private static func parseHTTPResponse(data: Data, url: URL) -> (Data, HTTPURLResponse)? {
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
    
    private static func dechunk(data: Data) -> Data {
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
