import Foundation
import os

extension APIClient {
    /// Uploads a local file to the gateway (POST /api/v1/attachments) as a raw streamed body.
    /// `onProgress` receives 0...0.95 as bytes are streamed, leaving the final 100% for the gateway's 200 OK.
    public func uploadAttachment(
        fileURL: URL,
        displayName: String,
        baseURL: URL,
        onProgress: (@Sendable (Double) -> Void)? = nil
    ) async throws -> UploadedAttachment {
        let endpoint = baseURL.appendingPathComponent("api/v1/attachments")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let encodedName = displayName.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? displayName
        request.setValue(encodedName, forHTTPHeaderField: "X-File-Name")
        request.timeoutInterval = 120
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        
        let isDemoEndpoint = DemoGateway.isEnabled
            && endpoint.host == DemoGateway.host
            && endpoint.port == DemoGateway.port
        
        let data: Data
        let response: URLResponse
        if !isDemoEndpoint, E2EECrypto.shared.shouldEncrypt(url: endpoint) {
            do {
                (data, response) = try await transport.uploadOverE2EE(
                    request: request,
                    fileURL: fileURL,
                    onProgress: onProgress,
                    streamUpload: { outerReq, encFileURL, progressHandler in
                        try await Self.uploadViaStreamedURLSessionRaw(
                            request: outerReq,
                            fileURL: encFileURL,
                            onProgress: progressHandler
                        )
                    }
                )
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                throw APIError.networkError(error.localizedDescription)
            }
        } else if NetworkTransport.requiresCleartextATSBypass(endpoint) {
            // App Transport Security blocks cleartext HTTP to Tailscale hosts; stream 32 KB chunks over Network.framework.
            do {
                (data, response) = try await transport.uploadCleartextHTTP(
                    request: request,
                    fileURL: fileURL,
                    onProgress: onProgress
                )
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                throw APIError.networkError(error.localizedDescription)
            }
        } else if !isDemoEndpoint,
                  endpoint.scheme?.lowercased() == "https",
                  CloudflareAnycastAccelerator.isCloudflareTunnelHost(endpoint.host ?? "") {
            // Stream 32 KB chunks through the fastest Asia Anycast edge IP; fall back to streamed URLSession on error.
            do {
                (data, response) = try await transport.uploadCloudflareAcceleratedHTTPS(
                    request: request,
                    fileURL: fileURL,
                    onProgress: onProgress
                )
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                (data, response) = try await Self.uploadViaStreamedURLSession(
                    request: request,
                    fileURL: fileURL,
                    onProgress: onProgress
                )
            }
        } else if isDemoEndpoint {
            (data, response) = try await Self.uploadViaDemoURLSession(
                request: request,
                fileURL: fileURL,
                onProgress: onProgress
            )
        } else {
            (data, response) = try await Self.uploadViaStreamedURLSession(
                request: request,
                fileURL: fileURL,
                onProgress: onProgress
            )
        }
        
        var finalData = data
        var finalResponse = response
        if !isDemoEndpoint,
           let http = finalResponse as? HTTPURLResponse,
           http.statusCode == 426,
           E2EECrypto.shared.isReady,
           let host = endpoint.host,
           !NetworkTransport.isLocalOrPrivateHost(host) {
            E2EECrypto.shared.markHostRequiresE2EE(host)
            do {
                (finalData, finalResponse) = try await transport.uploadOverE2EE(
                    request: request,
                    fileURL: fileURL,
                    onProgress: onProgress,
                    streamUpload: { outerReq, encFileURL, progressHandler in
                        try await Self.uploadViaStreamedURLSessionRaw(
                            request: outerReq,
                            fileURL: encFileURL,
                            onProgress: progressHandler
                        )
                    }
                )
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                throw APIError.networkError(error.localizedDescription)
            }
        }
        
        guard let http = finalResponse as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(http.statusCode) else {
            if http.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            struct ErrBody: Decodable { let message: String? }
            let message = (try? JSONDecoder().decode(ErrBody.self, from: finalData))?.message
                ?? String(data: finalData, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: message)
        }
        do {
            return try JSONDecoder().decode(UploadedAttachment.self, from: finalData)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }
    
    /// Streams a file upload through URLSession using Apple's bound streams (`Stream.getBoundStreams`)
    /// so files under 2 MB emit smooth 32 KB chunk progress instead of CFNetwork's coarse 1–2 MB jumps.
    private static func uploadViaStreamedURLSession(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)?
    ) async throws -> (Data, URLResponse) {
        do {
            return try await uploadViaStreamedURLSessionRaw(
                request: request,
                fileURL: fileURL,
                onProgress: onProgress
            )
        } catch is CancellationError {
            throw CancellationError()
        } catch let apiErr as APIError {
            throw apiErr
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
    }

    /// Raw-error variant of `uploadViaStreamedURLSession` that preserves `URLError` for transport-level retries.
    private static func uploadViaStreamedURLSessionRaw(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)?
    ) async throws -> (Data, URLResponse) {
        let totalBytes = ((try? FileManager.default.attributesOfItem(atPath: fileURL.path))?[.size] as? NSNumber)?.int64Value ?? 0
        guard totalBytes > 0 else {
            return try await uploadViaDemoURLSession(request: request, fileURL: fileURL, onProgress: onProgress)
        }
        
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 120
        config.timeoutIntervalForResource = 600
        config.httpShouldSetCookies = false
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        
        let reporter = UploadProgressReporter(onProgress: onProgress)
        let delegate = UploadProgressDelegate(reporter: reporter)
        let streamer = try BoundedFileUploadStream(
            fileURL: fileURL,
            totalBytes: totalBytes,
            pipeBufferSize: 16 * 1024,
            readChunkSize: 32 * 1024,
            reporter: reporter
        )
        
        var streamedRequest = request
        streamedRequest.httpBody = nil
        streamedRequest.setValue("\(totalBytes)", forHTTPHeaderField: "Content-Length")
        streamedRequest.httpBodyStream = streamer.inputStream
        
        let session = URLSession(configuration: config)
        defer {
            streamer.cancel()
            session.finishTasksAndInvalidate()
        }
        
        streamer.start()
        return try await withTaskCancellationHandler {
            try await session.data(for: streamedRequest, delegate: delegate)
        } onCancel: {
            streamer.cancel()
        }
    }
    
    private static func uploadViaDemoURLSession(
        request: URLRequest,
        fileURL: URL,
        onProgress: (@Sendable (Double) -> Void)?
    ) async throws -> (Data, URLResponse) {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 120
        config.timeoutIntervalForResource = 600
        config.httpShouldSetCookies = false
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        let reporter = UploadProgressReporter(onProgress: onProgress)
        let delegate = UploadProgressDelegate(reporter: reporter)
        let session = URLSession(configuration: config)
        defer { session.finishTasksAndInvalidate() }
        do {
            return try await session.upload(for: request, fromFile: fileURL, delegate: delegate)
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
    }
}

/// Thread-safe monotonic progress reporter that scales streamed bytes into 0...0.95
/// so the UI does not appear stuck at 100% while waiting for the gateway to persist and reply.
nonisolated private final class UploadProgressReporter: Sendable {
    private let onProgress: (@Sendable (Double) -> Void)?
    private let lastPercent = OSAllocatedUnfairLock(initialState: -1)
    
    init(onProgress: (@Sendable (Double) -> Void)?) {
        self.onProgress = onProgress
    }
    
    func report(bytesSent: Int64, totalBytes: Int64) {
        guard totalBytes > 0, bytesSent > 0 else { return }
        let raw = max(0, min(1, Double(bytesSent) / Double(totalBytes)))
        let scaled = min(0.95, raw * 0.95)
        let percent = Int((scaled * 100).rounded(.down))
        let shouldEmit = lastPercent.withLock { last -> Bool in
            if percent > last {
                last = percent
                return true
            }
            return false
        }
        if shouldEmit {
            onProgress?(scaled)
        }
    }
}

nonisolated private final class UploadProgressDelegate: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    private let reporter: UploadProgressReporter
    
    init(reporter: UploadProgressReporter) {
        self.reporter = reporter
    }
    
    func urlSession(_ session: URLSession, task: URLSessionTask, didSendBodyData bytesSent: Int64, totalBytesSent: Int64, totalBytesExpectedToSend: Int64) {
        guard totalBytesExpectedToSend > 0 else { return }
        reporter.report(bytesSent: totalBytesSent, totalBytes: totalBytesExpectedToSend)
    }
}

/// Streams a local file into an `InputStream` backed by `Stream.getBoundStreams`,
/// reporting drained bytes in small chunks while respecting TCP back-pressure from `URLSession`.
nonisolated private final class BoundedFileUploadStream: NSObject, StreamDelegate, @unchecked Sendable {
    let inputStream: InputStream
    private let outputStream: OutputStream
    private let fileHandle: FileHandle
    private let totalBytes: Int64
    private let pipeBufferSize: Int
    private let readChunkSize: Int
    private let reporter: UploadProgressReporter
    private let queue = DispatchQueue(label: "com.antigravity.mobile.attachment-upload-stream", qos: .userInitiated)
    
    private var bytesWritten: Int64 = 0
    private var pendingBuffer = Data()
    private var pendingOffset = 0
    private var eof = false
    private var isFinished = false
    
    init(
        fileURL: URL,
        totalBytes: Int64,
        pipeBufferSize: Int,
        readChunkSize: Int,
        reporter: UploadProgressReporter
    ) throws {
        self.fileHandle = try FileHandle(forReadingFrom: fileURL)
        self.totalBytes = totalBytes
        self.pipeBufferSize = pipeBufferSize
        self.readChunkSize = readChunkSize
        self.reporter = reporter
        var boundInput: InputStream?
        var boundOutput: OutputStream?
        Stream.getBoundStreams(withBufferSize: pipeBufferSize, inputStream: &boundInput, outputStream: &boundOutput)
        guard let input = boundInput, let output = boundOutput else {
            try? fileHandle.close()
            throw URLError(.cannotOpenFile)
        }
        self.inputStream = input
        self.outputStream = output
        super.init()
        self.outputStream.delegate = self
    }
    
    func start() {
        queue.async { [self] in
            guard !isFinished else { return }
            CFWriteStreamSetDispatchQueue(outputStream, queue)
            outputStream.open()
        }
    }
    
    func cancel() {
        queue.async { [self] in
            finish()
        }
    }
    
    private func finish() {
        guard !isFinished else { return }
        isFinished = true
        outputStream.delegate = nil
        outputStream.close()
        CFWriteStreamSetDispatchQueue(outputStream, nil)
        try? fileHandle.close()
    }
    
    func stream(_ aStream: Stream, handle eventCode: Stream.Event) {
        switch eventCode {
        case .hasSpaceAvailable:
            pump()
        case .errorOccurred, .endEncountered:
            finish()
        default:
            break
        }
    }
    
    private func pump() {
        guard !isFinished else { return }
        while outputStream.hasSpaceAvailable {
            if pendingOffset >= pendingBuffer.count {
                if eof {
                    finish()
                    return
                }
                do {
                    if let chunk = try fileHandle.read(upToCount: readChunkSize), !chunk.isEmpty {
                        pendingBuffer = chunk
                        pendingOffset = 0
                    } else {
                        eof = true
                        finish()
                        return
                    }
                } catch {
                    finish()
                    return
                }
            }
            let remaining = pendingBuffer.count - pendingOffset
            let written = pendingBuffer.withUnsafeBytes { raw -> Int in
                guard let base = raw.baseAddress?.assumingMemoryBound(to: UInt8.self) else { return -1 }
                return outputStream.write(base.advanced(by: pendingOffset), maxLength: remaining)
            }
            if written > 0 {
                pendingOffset += written
                bytesWritten += Int64(written)
                // Subtract the initial pipe buffer so progress stays at 0% during TCP/TLS handshake
                // and starts advancing once URLSession actually pulls bytes out of the pipe.
                let drained = bytesWritten - Int64(pipeBufferSize)
                if drained > 0 {
                    reporter.report(bytesSent: drained, totalBytes: totalBytes)
                }
            } else {
                if written < 0 { finish() }
                return
            }
        }
    }
}
