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

    /// Collects bytes read from a connection and reports when one complete response has arrived.
    /// The header is parsed once; afterwards only newly received bytes are inspected, so large
    /// responses are not re-copied and re-scanned on every chunk.
    struct ResponseAccumulator {
        private enum Framing {
            case length(Int)
            case chunked
            case untilClose
        }

        private(set) var buffer = Data()
        private var bodyStart: Int?
        private var framing: Framing?
        private var chunkScanFrom = 0

        private static let headerTerminator = Data("\r\n\r\n".utf8)
        private static let lastChunk = Data("\r\n0\r\n\r\n".utf8)
        private static let leadingLastChunk = Data("0\r\n\r\n".utf8)

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
                chunkScanFrom = end.upperBound
            }
            guard let start = bodyStart, let framing else { return false }
            switch framing {
            case .length(let n):
                return buffer.count - start >= n
            case .untilClose:
                return false
            case .chunked:
                if buffer.count - start >= Self.leadingLastChunk.count,
                   buffer.subdata(in: start..<(start + Self.leadingLastChunk.count)) == Self.leadingLastChunk {
                    return true
                }
                let from = max(start, chunkScanFrom - (Self.lastChunk.count - 1))
                chunkScanFrom = buffer.count
                return buffer.range(of: Self.lastChunk, in: from..<buffer.count) != nil
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
            guard chunkEnd <= data.endIndex else {
                throw URLError(.cannotParseResponse)
            }
            unchunked.append(data.subdata(in: chunkStart..<chunkEnd))
            offset = chunkEnd
            if offset + 2 <= data.endIndex && data.subdata(in: offset..<offset + 2) == crlf {
                offset += 2
            }
        }
        return unchunked
    }

    /// Decodes a gzip (RFC 1952) member. Foundation's `.zlib` algorithm is raw DEFLATE, so the gzip
    /// header and trailer are handled here. Returns nil for malformed input.
    static func gunzip(_ data: Data) -> Data? {
        let bytes = [UInt8](data)
        guard bytes.count >= 18, bytes[0] == 0x1f, bytes[1] == 0x8b, bytes[2] == 8 else { return nil }
        let flags = bytes[3]
        var pos = 10
        if flags & 0x04 != 0 { // FEXTRA
            guard pos + 2 <= bytes.count else { return nil }
            pos += 2 + Int(bytes[pos]) + Int(bytes[pos + 1]) << 8
        }
        if flags & 0x08 != 0 { // FNAME
            while pos < bytes.count && bytes[pos] != 0 { pos += 1 }
            pos += 1
        }
        if flags & 0x10 != 0 { // FCOMMENT
            while pos < bytes.count && bytes[pos] != 0 { pos += 1 }
            pos += 1
        }
        if flags & 0x02 != 0 { // FHCRC
            pos += 2
        }
        guard pos <= bytes.count - 8 else { return nil }
        let deflated = Data(bytes[pos..<(bytes.count - 8)])
        guard let inflated = try? (deflated as NSData).decompressed(using: .zlib) as Data else { return nil }
        let n = bytes.count
        let expectedSize = UInt32(bytes[n - 4]) | UInt32(bytes[n - 3]) << 8 | UInt32(bytes[n - 2]) << 16 | UInt32(bytes[n - 1]) << 24
        guard UInt32(truncatingIfNeeded: inflated.count) == expectedSize else { return nil }
        return inflated
    }
}
