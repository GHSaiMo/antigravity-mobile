import Foundation

extension APIClient {
    /// Uploads a local file to the gateway (POST /api/v1/attachments) as a raw streamed body.
    /// `onProgress` receives 0...1 as bytes are sent.
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
        
        let data: Data
        let response: URLResponse
        if NetworkTransport.requiresCleartextATSBypass(endpoint) {
            // App Transport Security blocks cleartext HTTP to public hosts; the shared transport
            // routes those through Network.framework (no progress, body held in memory).
            request.httpBody = try Data(contentsOf: fileURL, options: .mappedIfSafe)
            (data, response) = try await transport.send(request: request)
            onProgress?(1)
        } else {
            let config = URLSessionConfiguration.default
            config.timeoutIntervalForRequest = 120
            config.timeoutIntervalForResource = 600
            config.httpShouldSetCookies = false
            let delegate = UploadProgressDelegate(onProgress: onProgress)
            let session = URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
            defer { session.finishTasksAndInvalidate() }
            do {
                (data, response) = try await session.upload(for: request, fromFile: fileURL)
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                throw APIError.networkError(error.localizedDescription)
            }
        }
        
        guard let http = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(http.statusCode) else {
            if http.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            struct ErrBody: Decodable { let message: String? }
            let message = (try? JSONDecoder().decode(ErrBody.self, from: data))?.message
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: message)
        }
        do {
            return try JSONDecoder().decode(UploadedAttachment.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }
}

private final class UploadProgressDelegate: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    let onProgress: (@Sendable (Double) -> Void)?
    
    init(onProgress: (@Sendable (Double) -> Void)?) {
        self.onProgress = onProgress
    }
    
    func urlSession(_ session: URLSession, task: URLSessionTask, didSendBodyData bytesSent: Int64, totalBytesSent: Int64, totalBytesExpectedToSend: Int64) {
        guard totalBytesExpectedToSend > 0 else { return }
        onProgress?(max(0, min(1, Double(totalBytesSent) / Double(totalBytesExpectedToSend))))
    }
}
