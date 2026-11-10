import Foundation

/// Minimal HTTP/1.1 request encoding and response decoding for the raw `NWConnection` paths in
/// `NetworkTransport` (Cloudflare Anycast pinning and cleartext relays), which bypass URLSession and
/// therefore get none of its framing, keep-alive or content decoding for free.
/// Pure Foundation so it can be compiled and tested on its own.
enum HTTP11Codec {
    static func buildRequest(_ request: URLRequest, url: URL, bareHost: String, keepAlive: Bool) -> Data {
        var path = url.path.isEmpty ? "/" : url.path
        if let query = url.query, !query.isEmpty {
            path += "?" + query
        }
        let method = request.httpMethod ?? "GET"
        let defaultPort = (url.scheme?.lowercased() == "https") ? 443 : 80
        let port = url.port ?? defaultPort
        let hostHeader: String
        if bareHost.contains(":") {
            hostHeader = "[\(bareHost)]:\(port)"
        } else if port == defaultPort {
            hostHeader = bareHost
        } else {
            hostHeader = "\(bareHost):\(port)"
        }

        var lines: [String] = [
            "\(method) \(path) HTTP/1.1",
            "Host: \(hostHeader)",
            keepAlive ? "Connection: keep-alive" : "Connection: close"
        ]
        var hasAcceptEncoding = false
        if let headers = request.allHTTPHeaderFields {
            for (key, value) in headers {
                let lower = key.lowercased()
                if lower == "host" || lower == "connection" { continue }
                if lower == "accept-encoding" { hasAcceptEncoding = true }
                lines.append("\(key): \(value)")
            }
        }
        // URLSession adds this implicitly; without it the gateway (and Cloudflare) reply uncompressed.
        if !hasAcceptEncoding {
            lines.append("Accept-Encoding: gzip")
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

    static let maxBodyBytes = 64 * 1024 * 1024

    /// Collects bytes read from a connection and reports when one complete response has arrived.
    /// The header is parsed once; afterwards only newly received bytes are inspected, so large
    /// responses are not re-copied and re-scanned on every chunk.
    struct ResponseAccumulator {
        private enum Framing {
            case length(Int)
            case chunked
            case untilClose
        }

        private enum ChunkPhase {
            case readingSize
            case readingData(remaining: Int)
            case readingTrailers
            case done
        }

        private(set) var buffer = Data()
        private var bodyStart: Int?
        private var framing: Framing?
        private var chunkCursor = 0
        private var chunkPhase: ChunkPhase = .readingSize

        private static let headerTerminator = Data("\r\n\r\n".utf8)
        private static let crlf = Data("\r\n".utf8)

        var isUntilClose: Bool {
            if case .untilClose = framing { return true }
            return false
        }

        /// Appends a received chunk and returns true once the response is complete.
        mutating func append(_ chunk: Data) -> Bool {
            buffer.append(chunk)
            if bodyStart == nil {
                let searchFrom = max(0, buffer.count - chunk.count - 3)
                guard let end = buffer.range(of: Self.headerTerminator, in: searchFrom..<buffer.count) else {
                    return false
                }
                bodyStart = end.upperBound
                framing = Self.framing(forHeader: buffer.subdata(in: 0..<end.lowerBound))
                chunkCursor = end.upperBound
                chunkPhase = .readingSize
            }
            guard let start = bodyStart, let framing else { return false }
            switch framing {
            case .length(let n):
                return buffer.count - start >= max(0, n)
            case .untilClose:
                return false
            case .chunked:
                return advanceChunkedParser()
            }
        }

        private mutating func advanceChunkedParser() -> Bool {
            while true {
                switch chunkPhase {
                case .done:
                    return true
                case .readingSize:
                    guard chunkCursor < buffer.count,
                          let lineEnd = buffer.range(of: Self.crlf, options: [], in: chunkCursor..<buffer.count) else {
                        return false
                    }
                    let lineData = buffer.subdata(in: chunkCursor..<lineEnd.lowerBound)
                    chunkCursor = lineEnd.upperBound
                    guard let lineStr = String(data: lineData, encoding: .ascii) else {
                        chunkPhase = .done
                        return true
                    }
                    let hexPart = lineStr.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: false)
                        .first
                        .map { $0.trimmingCharacters(in: .whitespaces) } ?? ""
                    guard !hexPart.isEmpty, let chunkSize = Int(hexPart, radix: 16), chunkSize >= 0 else {
                        chunkPhase = .done
                        return true
                    }
                    if chunkSize == 0 {
                        chunkPhase = .readingTrailers
                    } else {
                        chunkPhase = .readingData(remaining: chunkSize)
                    }
                case .readingData(let remaining):
                    let (needed, overflow) = remaining.addingReportingOverflow(2)
                    if overflow || remaining > HTTP11Codec.maxBodyBytes {
                        chunkPhase = .done
                        return true
                    }
                    guard buffer.count - chunkCursor >= needed else {
                        return false
                    }
                    let crlfStart = chunkCursor + remaining
                    guard buffer.subdata(in: crlfStart..<(crlfStart + 2)) == Self.crlf else {
                        chunkPhase = .done
                        return true
                    }
                    chunkCursor = crlfStart + 2
                    chunkPhase = .readingSize
                case .readingTrailers:
                    guard buffer.count - chunkCursor >= 2 else {
                        return false
                    }
                    if buffer.subdata(in: chunkCursor..<(chunkCursor + 2)) == Self.crlf {
                        chunkCursor += 2
                        chunkPhase = .done
                        return true
                    }
                    guard let trailerEnd = buffer.range(of: Self.headerTerminator, options: [], in: chunkCursor..<buffer.count) else {
                        return false
                    }
                    chunkCursor = trailerEnd.upperBound
                    chunkPhase = .done
                    return true
                }
            }
        }

        private static func framing(forHeader header: Data) -> Framing {
            guard let text = String(data: header, encoding: .isoLatin1) else { return .untilClose }
            let lines = text.split(separator: "\r\n")
            if let status = lines.first {
                let parts = status.split(separator: " ")
                if parts.count >= 2, let code = Int(parts[1]), code == 204 || code == 304 || (100..<200).contains(code) {
                    return .length(0)
                }
            }
            var isChunked = false
            var contentLength: Int?
            for line in lines.dropFirst() {
                let parts = line.split(separator: ":", maxSplits: 1)
                guard parts.count == 2 else { continue }
                let name = parts[0].trimmingCharacters(in: .whitespaces).lowercased()
                let value = parts[1].trimmingCharacters(in: .whitespaces).lowercased()
                if name == "transfer-encoding" && value.contains("chunked") {
                    isChunked = true
                } else if name == "content-length", let n = Int(value) {
                    contentLength = n
                }
            }
            // Transfer-Encoding wins over Content-Length (RFC 9112 §6.3).
            if isChunked { return .chunked }
            if let contentLength { return .length(contentLength) }
            return .untilClose
        }
    }

    /// Splits a complete raw response into its (de-chunked, decompressed) body and an HTTPURLResponse.
    static func parseResponse(_ data: Data, url: URL) throws -> (Data, URLResponse) {
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
        func fieldKey(_ name: String) -> String? {
            fields.keys.first { $0.lowercased() == name }
        }

        var body = data.subdata(in: headerEnd.upperBound..<data.endIndex)
        if let te = fieldKey("transfer-encoding"), fields[te]?.lowercased().contains("chunked") == true {
            body = try decodeChunkedBody(body)
            fields.removeValue(forKey: te)
        } else if let cl = fieldKey("content-length"), let len = Int(fields[cl] ?? ""), len >= 0, body.count > len {
            body = body.prefix(len)
        }
        if let ce = fieldKey("content-encoding"), fields[ce]?.lowercased() == "gzip" {
            guard let inflated = gunzip(body) else {
                throw URLError(.cannotDecodeContentData)
            }
            body = inflated
            // Describe the body callers actually receive, as URLSession does after decoding.
            fields.removeValue(forKey: ce)
        }
        if let cl = fieldKey("content-length") {
            fields[cl] = String(body.count)
        }
        let response = HTTPURLResponse(url: url, statusCode: code, httpVersion: "HTTP/1.1", headerFields: fields)
            ?? URLResponse(url: url, mimeType: nil, expectedContentLength: body.count, textEncodingName: nil)
        return (body, response)
    }

    static func decodeChunkedBody(_ data: Data) throws -> Data {
        var unchunked = Data()
        var offset = data.startIndex
        let crlf = Data("\r\n".utf8)

        while offset < data.endIndex {
            guard let range = data.range(of: crlf, options: [], in: offset..<data.endIndex) else {
                throw URLError(.cannotParseResponse)
            }
            let sizeData = data.subdata(in: offset..<range.lowerBound)
            guard let sizeStr = String(data: sizeData, encoding: .ascii)?.trimmingCharacters(in: .whitespaces) else {
                throw URLError(.cannotParseResponse)
            }
            if sizeStr.isEmpty {
                throw URLError(.cannotParseResponse)
            }
            let hexStr = sizeStr.split(separator: ";").first.map(String.init) ?? sizeStr
            guard let chunkSize = Int(hexStr.trimmingCharacters(in: .whitespaces), radix: 16), chunkSize >= 0 else {
                throw URLError(.cannotParseResponse)
            }
            if chunkSize == 0 {
                break
            }
            let chunkStart = range.upperBound
            let (chunkEnd, overflow) = chunkStart.addingReportingOverflow(chunkSize)
            guard !overflow, chunkSize <= maxBodyBytes, unchunked.count + chunkSize <= maxBodyBytes, chunkEnd + 2 <= data.endIndex else {
                throw URLError(.cannotParseResponse)
            }
            guard data.subdata(in: chunkEnd..<(chunkEnd + 2)) == crlf else {
                throw URLError(.cannotParseResponse)
            }
            unchunked.append(data.subdata(in: chunkStart..<chunkEnd))
            offset = chunkEnd + 2
        }
        return unchunked
    }

    /// Decodes a gzip (RFC 1952) member. Foundation's `.zlib` algorithm is raw DEFLATE, so the gzip
    /// header and trailer are handled here. Returns nil for malformed input.
    static func gunzip(_ data: Data) -> Data? {
        let bytes = [UInt8](data)
        guard bytes.count >= 18, bytes[0] == 0x1f, bytes[1] == 0x8b, bytes[2] == 8 else { return nil }
        let n = bytes.count
        let expectedSize = UInt32(bytes[n - 4]) | UInt32(bytes[n - 3]) << 8 | UInt32(bytes[n - 2]) << 16 | UInt32(bytes[n - 1]) << 24
        guard Int(expectedSize) <= maxBodyBytes else { return nil }
        let flags = bytes[3]
        var pos = 10
        if flags & 0x04 != 0 { // FEXTRA
            guard pos + 2 <= n - 8 else { return nil }
            let extraLen = Int(bytes[pos]) + (Int(bytes[pos + 1]) << 8)
            pos += 2 + extraLen
        }
        if flags & 0x08 != 0 { // FNAME
            while pos < n - 8 && bytes[pos] != 0 { pos += 1 }
            guard pos < n - 8 else { return nil }
            pos += 1
        }
        if flags & 0x10 != 0 { // FCOMMENT
            while pos < n - 8 && bytes[pos] != 0 { pos += 1 }
            guard pos < n - 8 else { return nil }
            pos += 1
        }
        if flags & 0x02 != 0 { // FHCRC
            pos += 2
        }
        guard pos >= 10, pos <= n - 8 else { return nil }
        let deflated = Data(bytes[pos..<(n - 8)])
        guard let inflated = try? (deflated as NSData).decompressed(using: .zlib) as Data,
              inflated.count <= maxBodyBytes else { return nil }
        guard UInt32(truncatingIfNeeded: inflated.count) == expectedSize else { return nil }
        return inflated
    }
}
