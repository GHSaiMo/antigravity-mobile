import Foundation
import Compression

/// Minimal HTTP/1.1 request encoding and response decoding for the raw `NWConnection` paths in
/// `NetworkTransport` (Cloudflare Anycast pinning and cleartext relays), which bypass URLSession and
/// therefore get none of its framing, keep-alive or content decoding for free.
/// Pure Foundation so it can be compiled and tested on its own.
enum HTTP11Codec {
    static func buildRequestHead(_ request: URLRequest, url: URL, bareHost: String, keepAlive: Bool, contentLength: Int64) -> Data {
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
        var hasUserAgent = false
        if let headers = request.allHTTPHeaderFields {
            for (key, value) in headers {
                let lower = key.lowercased()
                if lower == "host" || lower == "connection" { continue }
                if lower == "accept-encoding" { hasAcceptEncoding = true }
                if lower == "user-agent" { hasUserAgent = true }
                lines.append("\(key): \(value)")
            }
        }
        if !hasUserAgent {
            lines.append("User-Agent: Multigravity/1.2.5 (iOS)")
        }
        // URLSession adds this implicitly; without it the gateway (and Cloudflare) reply uncompressed.
        if !hasAcceptEncoding {
            lines.append("Accept-Encoding: gzip")
        }
        if request.value(forHTTPHeaderField: "Content-Length") == nil {
            lines.append("Content-Length: \(contentLength)")
        }
        var data = Data(lines.joined(separator: "\r\n").utf8)
        data.append(Data("\r\n\r\n".utf8))
        return data
    }

    static func buildRequest(_ request: URLRequest, url: URL, bareHost: String, keepAlive: Bool) -> Data {
        let body = request.httpBody ?? Data()
        var data = buildRequestHead(request, url: url, bareHost: bareHost, keepAlive: keepAlive, contentLength: Int64(body.count))
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
        let parsed = try parseResponseHeader(data.subdata(in: data.startIndex..<headerEnd.lowerBound))
        var fields = parsed.fields
        func fieldKey(_ name: String) -> String? {
            fields.keys.first { $0.lowercased() == name }
        }

        var body = data.subdata(in: headerEnd.upperBound..<data.endIndex)
        if case .chunked = parsed.framing {
            body = try decodeChunkedBody(body)
            if let te = fieldKey("transfer-encoding") { fields.removeValue(forKey: te) }
        } else if case .length(let len) = parsed.framing, len >= 0, Int64(body.count) > len {
            body = body.prefix(Int(len))
        }
        if parsed.isGzip {
            guard let inflated = gunzip(body) else {
                throw URLError(.cannotDecodeContentData)
            }
            body = inflated
            // Describe the body callers actually receive, as URLSession does after decoding.
            if let ce = fieldKey("content-encoding") { fields.removeValue(forKey: ce) }
        }
        if let cl = fieldKey("content-length") {
            fields[cl] = String(body.count)
        }
        let response = HTTPURLResponse(url: url, statusCode: parsed.statusCode, httpVersion: "HTTP/1.1", headerFields: fields)
            ?? URLResponse(url: url, mimeType: nil, expectedContentLength: body.count, textEncodingName: nil)
        return (body, response)
    }

    enum BodyFraming {
        case length(Int64)
        case chunked
        case untilClose
    }

    struct ParsedResponseHeader {
        let statusCode: Int
        let fields: [String: String]
        let framing: BodyFraming
        let isGzip: Bool
    }

    static func parseResponseHeader(_ headerData: Data) throws -> ParsedResponseHeader {
        guard let headerText = String(data: headerData, encoding: .isoLatin1) else {
            throw URLError(.cannotParseResponse)
        }
        var lines = headerText.split(separator: "\r\n", omittingEmptySubsequences: false).map(String.init)
        guard let statusLine = lines.first else {
            throw URLError(.cannotParseResponse)
        }
        lines.removeFirst()
        let statusParts = statusLine.split(separator: " ")
        let code = statusParts.count >= 2 ? (Int(statusParts[1]) ?? 500) : 500

        var fields: [String: String] = [:]
        var isChunked = false
        var contentLength: Int64?
        var isGzip = false

        for line in lines {
            guard let colon = line.firstIndex(of: ":") else { continue }
            let key = String(line[..<colon])
            let value = line[line.index(after: colon)...].trimmingCharacters(in: .whitespaces)
            fields[key] = value

            let lowerKey = key.trimmingCharacters(in: .whitespaces).lowercased()
            let lowerVal = value.lowercased()
            if lowerKey == "transfer-encoding" && lowerVal.contains("chunked") {
                isChunked = true
            } else if lowerKey == "content-length", let n = Int64(lowerVal), n >= 0 {
                contentLength = n
            } else if lowerKey == "content-encoding" && lowerVal == "gzip" {
                isGzip = true
            }
        }

        let framing: BodyFraming
        if code == 204 || code == 304 || (100..<200).contains(code) {
            framing = .length(0)
        } else if isChunked {
            framing = .chunked
        } else if let contentLength {
            framing = .length(contentLength)
        } else {
            framing = .untilClose
        }

        return ParsedResponseHeader(
            statusCode: code,
            fields: fields,
            framing: framing,
            isGzip: isGzip
        )
    }

    static func makeHTTPResponse(url: URL, header: ParsedResponseHeader, decodedContentLength: Int64?) -> HTTPURLResponse? {
        var fields = header.fields
        func fieldKey(_ name: String) -> String? {
            fields.keys.first { $0.lowercased() == name }
        }
        if case .chunked = header.framing, let te = fieldKey("transfer-encoding") {
            fields.removeValue(forKey: te)
        }
        if header.isGzip, let ce = fieldKey("content-encoding") {
            fields.removeValue(forKey: ce)
        }
        if let decodedContentLength {
            if let cl = fieldKey("content-length") {
                fields[cl] = String(decodedContentLength)
            } else {
                fields["Content-Length"] = String(decodedContentLength)
            }
        }
        return HTTPURLResponse(url: url, statusCode: header.statusCode, httpVersion: "HTTP/1.1", headerFields: fields)
    }

    /// Incremental HTTP/1.1 chunked transfer-encoding decoder for streaming file downloads to disk
    /// without buffering the entire payload in memory.
    struct ChunkedStreamDecoder {
        private enum Phase {
            case readingSize
            case readingData(remaining: Int)
            case readingDataCRLF
            case readingTrailers
            case done
        }

        private var phase: Phase = .readingSize
        private var pending = Data()
        private static let crlf = Data("\r\n".utf8)
        private static let headerTerminator = Data("\r\n\r\n".utf8)

        var isDone: Bool {
            if case .done = phase { return true }
            return false
        }

        mutating func feed(_ bytes: Data, write: (Data) throws -> Void) throws {
            if !bytes.isEmpty {
                pending.append(bytes)
            }
            while true {
                switch phase {
                case .done:
                    return
                case .readingSize:
                    guard let lineEnd = pending.range(of: Self.crlf) else {
                        if pending.count > 4096 {
                            throw URLError(.cannotParseResponse)
                        }
                        return
                    }
                    let lineData = pending.subdata(in: pending.startIndex..<lineEnd.lowerBound)
                    pending = Data(pending[lineEnd.upperBound...])
                    guard let lineStr = String(data: lineData, encoding: .ascii) else {
                        throw URLError(.cannotParseResponse)
                    }
                    let hexPart = lineStr.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: false)
                        .first
                        .map { $0.trimmingCharacters(in: .whitespaces) } ?? ""
                    guard !hexPart.isEmpty, let chunkSize = Int(hexPart, radix: 16), chunkSize >= 0 else {
                        throw URLError(.cannotParseResponse)
                    }
                    if chunkSize == 0 {
                        phase = .readingTrailers
                    } else {
                        phase = .readingData(remaining: chunkSize)
                    }
                case .readingData(let remaining):
                    guard !pending.isEmpty else { return }
                    let take = min(remaining, pending.count)
                    try write(Data(pending.prefix(take)))
                    pending = Data(pending.dropFirst(take))
                    let left = remaining - take
                    phase = (left == 0) ? .readingDataCRLF : .readingData(remaining: left)
                case .readingDataCRLF:
                    guard pending.count >= 2 else { return }
                    guard pending.prefix(2) == Self.crlf else {
                        throw URLError(.cannotParseResponse)
                    }
                    pending = Data(pending.dropFirst(2))
                    phase = .readingSize
                case .readingTrailers:
                    guard pending.count >= 2 else { return }
                    if pending.prefix(2) == Self.crlf {
                        pending = Data(pending.dropFirst(2))
                        phase = .done
                        return
                    }
                    guard let trailerEnd = pending.range(of: Self.headerTerminator) else {
                        if pending.count > 65_536 {
                            throw URLError(.cannotParseResponse)
                        }
                        return
                    }
                    pending = Data(pending[trailerEnd.upperBound...])
                    phase = .done
                    return
                }
            }
        }
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

    /// Decodes a gzip (RFC 1952) member with bounded streaming decompression to prevent gzip-bomb OOMs.
    /// Foundation's `COMPRESSION_ZLIB` algorithm is raw DEFLATE, so the gzip header and trailer are handled here.
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

        let compSlice = Array(bytes[pos..<(n - 8)])
        if compSlice.isEmpty {
            return expectedSize == 0 ? Data() : nil
        }

        var dummySrc = UInt8(0)
        var dummyDst = UInt8(0)
        var stream = compression_stream(
            dst_ptr: &dummyDst,
            dst_size: 0,
            src_ptr: &dummySrc,
            src_size: 0,
            state: nil
        )
        guard compression_stream_init(&stream, COMPRESSION_STREAM_DECODE, COMPRESSION_ZLIB) == COMPRESSION_STATUS_OK else {
            return nil
        }
        defer { compression_stream_destroy(&stream) }

        let chunkSize = 64 * 1024
        var dstBuffer = [UInt8](repeating: 0, count: chunkSize)
        var output = Data()
        output.reserveCapacity(min(Int(expectedSize), 256 * 1024))

        let decodeOK: Bool = compSlice.withUnsafeBufferPointer { srcBuf in
            guard let srcBase = srcBuf.baseAddress else { return false }
            stream.src_ptr = srcBase
            stream.src_size = srcBuf.count

            while true {
                let status: compression_status = dstBuffer.withUnsafeMutableBufferPointer { dstBuf in
                    stream.dst_ptr = dstBuf.baseAddress!
                    stream.dst_size = dstBuf.count
                    return compression_stream_process(&stream, Int32(COMPRESSION_STREAM_FINALIZE.rawValue))
                }
                let produced = chunkSize - stream.dst_size
                if produced > 0 {
                    if output.count + produced > maxBodyBytes {
                        return false
                    }
                    output.append(dstBuffer, count: produced)
                }
                switch status {
                case COMPRESSION_STATUS_END:
                    return true
                case COMPRESSION_STATUS_OK:
                    if produced == 0 && stream.src_size == 0 {
                        return false
                    }
                default:
                    return false
                }
            }
        }

        guard decodeOK, UInt32(truncatingIfNeeded: output.count) == expectedSize else {
            return nil
        }
        return output
    }
}
