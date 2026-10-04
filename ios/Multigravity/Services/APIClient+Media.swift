import Foundation

extension APIClient {
    /// Resolves a raw media/image URI into an authenticated, loadable HTTP URL for the mobile client.
    public func resolveMediaURL(_ raw: String, baseURL: URL) -> String {
        var clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if clean.hasPrefix("MEDIA:") {
            clean = String(clean.dropFirst(6)).trimmingCharacters(in: .whitespaces)
        }
        clean = clean.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
        
        let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken ?? ""
        
        if clean.hasPrefix("http://") || clean.hasPrefix("https://") {
            if let remote = URL(string: clean),
               Self.isSameGatewayHost(remote, gateway: baseURL),
               remote.path.contains("/api/v1/files/raw"),
               !clean.contains("auth_token="),
               !clean.contains("token="),
               !token.isEmpty {
                let separator = clean.contains("?") ? "&" : "?"
                return "\(clean)\(separator)auth_token=\(token)"
            }
            return clean
        }
        
        if clean.hasPrefix("/static/") {
            let base = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            return "\(base)\(clean)"
        }
        
        if clean.hasPrefix("file://") {
            clean = String(clean.dropFirst(7))
        }
        
        var comps = URLComponents(url: baseURL.appendingPathComponent("api/v1/files/raw"), resolvingAgainstBaseURL: false)
        var qItems = [URLQueryItem(name: "uri", value: clean)]
        if !token.isEmpty {
            qItems.append(URLQueryItem(name: "auth_token", value: token))
        }
        comps?.queryItems = qItems
        return comps?.url?.absoluteString ?? clean
    }

    /// Extracts potential image URLs or media paths from text if not already populated.
    public func extractImageURLs(from text: String) -> [String] {
        guard !text.isEmpty else { return [] }
        var urls: [String] = []
        
        let patterns = [
            #"\[!\[.*?\]\((?:[^\s\)]+)\)\]\((https?://[^\s\)]+|/static/[^\s\)]+|file://[^\s\)]+|/[^\s\)]+)\)"#,
            #"!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+|file://[^\s\)]+|/[^\s\)]+)\)"#,
            #"(?:^|\s)MEDIA:([^\s\)\<\>\"\'\`]+)"#,
            #"\[.*?\]\((https?://[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico)|file://[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico)|/[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico))\)"#
        ]
        
        for pat in patterns {
            if let regex = try? NSRegularExpression(pattern: pat, options: [.caseInsensitive]) {
                let nsRange = NSRange(text.startIndex..<text.endIndex, in: text)
                let matches = regex.matches(in: text, range: nsRange)
                for match in matches {
                    if match.numberOfRanges > 1, let range = Range(match.range(at: 1), in: text) {
                        let candidate = String(text[range])
                            .trimmingCharacters(in: .whitespacesAndNewlines)
                            .trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
                        if !candidate.isEmpty && !urls.contains(candidate) {
                            urls.append(candidate)
                        }
                    }
                }
            }
        }
        return urls
    }

    // Fetch file or artifact content from Gateway
    public func fetchFileContent(uri: String, cascadeId: String? = nil, baseURL: URL) async throws -> FileContentResponse {
        var components = URLComponents(url: baseURL.appendingPathComponent("api/v1/files/content"), resolvingAgainstBaseURL: false)
        var queryItems: [URLQueryItem] = [URLQueryItem(name: "uri", value: uri)]
        if let cascadeId = cascadeId, !cascadeId.isEmpty {
            queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
        }
        components?.queryItems = queryItems
        guard let endpoint = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 15.0
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        do {
            return try JSONDecoder().decode(FileContentResponse.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }
    
    // Download raw file or document (e.g. PPTX, PDF, DOCX, HTML, PNG, JPG) from Gateway with progress reporting and local caching
    public func downloadFile(
        uri: String,
        cascadeId: String? = nil,
        baseURL: URL,
        onProgress: (@Sendable (Double, Int64, Int64) -> Void)? = nil
    ) async throws -> (localURL: URL, fileName: String) {
        // Fast-path: Check persistent local cache first
        let initialFileName: String = {
            if let comps = URLComponents(string: uri),
               let paramUri = comps.queryItems?.first(where: { $0.name == "uri" })?.value, !paramUri.isEmpty {
                let fn = (paramUri as NSString).lastPathComponent.removingPercentEncoding ?? (paramUri as NSString).lastPathComponent
                if !fn.isEmpty && fn != "raw" { return fn }
            }
            let last = (uri as NSString).lastPathComponent.removingPercentEncoding ?? (uri as NSString).lastPathComponent
            return (last.isEmpty || last == "raw") ? "document" : last
        }()
        
        if !initialFileName.isEmpty && initialFileName != "document",
           let cached = DocumentCacheManager.shared.getCachedFile(for: uri, fileName: initialFileName) {
            return (cached, initialFileName)
        }
        
        let endpoint: URL
        let isDirectHttp = uri.hasPrefix("http://") || uri.hasPrefix("https://")
        if isDirectHttp, let directURL = URL(string: uri) {
            endpoint = directURL
        } else {
            var cleanPath = uri.trimmingCharacters(in: .whitespacesAndNewlines)
            if cleanPath.hasPrefix("MEDIA:") {
                cleanPath = String(cleanPath.dropFirst(6)).trimmingCharacters(in: .whitespaces)
            }
            if cleanPath.hasPrefix("file://") {
                cleanPath = String(cleanPath.dropFirst(7))
            }
            cleanPath = cleanPath.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
            
            var components = URLComponents(url: baseURL.appendingPathComponent("api/v1/files/raw"), resolvingAgainstBaseURL: false)
            var queryItems: [URLQueryItem] = [URLQueryItem(name: "uri", value: cleanPath)]
            if let cascadeId = cascadeId, !cascadeId.isEmpty {
                queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
            }
            components?.queryItems = queryItems
            guard let builtUrl = components?.url else {
                throw APIError.invalidURL
            }
            endpoint = builtUrl
        }
        
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 60.0
        
        let attachCredentials = !isDirectHttp || Self.isSameGatewayHost(endpoint, gateway: baseURL)
        if attachCredentials,
           let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        
        let delegate = FileDownloadProgressDelegate(onProgress: onProgress)
        let config = URLSessionConfiguration.background(withIdentifier: "com.antigravity.mobile.download.\(UUID().uuidString)")
        let session = URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
        
        let (tempDownloadedURL, httpResp) = try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<(URL, HTTPURLResponse), Error>) in
            delegate.continuation = continuation
            let task = session.downloadTask(with: request)
            task.resume()
        }
        session.finishTasksAndInvalidate()
        
        guard (200...299).contains(httpResp.statusCode) else {
            try? FileManager.default.removeItem(at: tempDownloadedURL)
            if httpResp.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let errData = (try? Data(contentsOf: tempDownloadedURL)) ?? Data()
            let msg = String(data: errData, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        
        // Extract filename from Content-Disposition header if available
        var filename: String? = nil
        if let disp = httpResp.value(forHTTPHeaderField: "Content-Disposition") {
            if let idx = disp.range(of: "filename*=UTF-8''") {
                let encoded = String(disp[idx.upperBound...]).trimmingCharacters(in: CharacterSet(charactersIn: "\"; "))
                filename = encoded.removingPercentEncoding
            } else if let idx = disp.range(of: "filename=") {
                let raw = String(disp[idx.upperBound...]).trimmingCharacters(in: CharacterSet(charactersIn: "\"; "))
                filename = raw.removingPercentEncoding ?? raw
            }
        }
        if filename == nil || filename?.isEmpty == true {
            filename = initialFileName
        }
        let resolvedFileName = filename?.isEmpty == false ? filename! : "document"
        
        // Save persistently to DocumentCacheManager
        let cachedURL = try DocumentCacheManager.shared.saveToCache(from: tempDownloadedURL, for: uri, fileName: resolvedFileName)
        
        let attributes = try? FileManager.default.attributesOfItem(atPath: cachedURL.path)
        let fileSize = (attributes?[.size] as? Int64) ?? 0
        if fileSize == 0 {
            throw APIError.serverError(statusCode: 500, message: "下载的文件为空")
        }
        
        return (cachedURL, resolvedFileName)
    }
    
}

// MARK: - File Download Progress Delegate

private final class FileDownloadProgressDelegate: NSObject, URLSessionDownloadDelegate, @unchecked Sendable {
    let onProgress: (@Sendable (Double, Int64, Int64) -> Void)?
    var continuation: CheckedContinuation<(URL, HTTPURLResponse), Error>?
    
    init(onProgress: (@Sendable (Double, Int64, Int64) -> Void)?) {
        self.onProgress = onProgress
    }
    
    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64, totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        let progress = totalBytesExpectedToWrite > 0 ? max(0, min(1.0, Double(totalBytesWritten) / Double(totalBytesExpectedToWrite))) : 0.0
        onProgress?(progress, totalBytesWritten, totalBytesExpectedToWrite)
    }
    
    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let httpResp = downloadTask.response as? HTTPURLResponse else {
            continuation?.resume(throwing: APIError.networkError("Invalid response type"))
            return
        }
        let tempFile = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        do {
            try FileManager.default.moveItem(at: location, to: tempFile)
            continuation?.resume(returning: (tempFile, httpResp))
        } catch {
            continuation?.resume(throwing: error)
        }
    }
    
    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error = error {
            continuation?.resume(throwing: error)
        }
    }
}
