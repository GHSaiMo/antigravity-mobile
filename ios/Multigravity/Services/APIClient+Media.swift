import Foundation

public enum GatewayImagePreviewMode: String {
    /// Chat bubble tier (~40-90KB, <=720px normal / <=480px short-side tall).
    case thumb = "thumb"
    /// Fullscreen & share preview tier (~150-450KB, <=1600px normal / <=1080px short-side tall).
    case full = "1"
}

extension APIClient {
    /// Returns a gateway preview URL for `/api/v1/files/raw` endpoints; non-gateway URLs pass through unchanged.
    public func previewMediaURL(_ url: URL, mode: GatewayImagePreviewMode) -> URL {
        guard !url.isFileURL,
              let scheme = url.scheme?.lowercased(), (scheme == "http" || scheme == "https"),
              var comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
              comps.path.hasSuffix("/api/v1/files/raw") else {
            return url
        }
        var items = (comps.queryItems ?? []).filter { $0.name.lowercased() != "preview" }
        items.append(URLQueryItem(name: "preview", value: mode.rawValue))
        comps.queryItems = items
        return comps.url ?? url
    }

    /// Strips any `preview` query parameter from a `/api/v1/files/raw` URL so original file downloads stay untouched.
    public func originalMediaURL(_ url: URL) -> URL {
        guard !url.isFileURL,
              var comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
              comps.path.hasSuffix("/api/v1/files/raw"),
              let items = comps.queryItems,
              items.contains(where: { $0.name.lowercased() == "preview" }) else {
            return url
        }
        let filtered = items.filter { $0.name.lowercased() != "preview" }
        comps.queryItems = filtered.isEmpty ? nil : filtered
        return comps.url ?? url
    }

    /// Resolves a raw media/image URI into a loadable HTTP URL for the mobile client.
    /// Authentication is injected via HTTP `Authorization: Bearer` header in `NetworkTransport` (never in query URLs).
    public func resolveMediaURL(_ raw: String, baseURL: URL) -> String {
        var clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if clean.hasPrefix("MEDIA:") {
            clean = String(clean.dropFirst(6)).trimmingCharacters(in: .whitespaces)
        }
        clean = clean.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
        if clean.hasPrefix("data:image/") { return clean }
        
        if clean.hasPrefix("http://") || clean.hasPrefix("https://") {
            if var comps = URLComponents(string: clean), let items = comps.queryItems, !items.isEmpty {
                let filtered = items.filter { item in
                    let n = item.name.lowercased()
                    return n != "auth_token" && n != "device_token"
                }
                if filtered.count != items.count {
                    comps.queryItems = filtered.isEmpty ? nil : filtered
                    return comps.url?.absoluteString ?? clean
                }
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
        comps?.queryItems = [URLQueryItem(name: "uri", value: clean)]
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
        // Demo mode: serve files from the built-in demo gateway (background sessions bypass URLProtocol).
        if DemoGateway.isEnabled {
            var path = uri
            if let comps = URLComponents(string: uri),
               let p = comps.queryItems?.first(where: { $0.name == "uri" || $0.name == "path" })?.value {
                path = p
            }
            guard let file = DemoGateway.shared.fileData(forPath: path) else {
                throw APIError.serverError(statusCode: 404, message: "file not found")
            }
            let tmp = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            try file.data.write(to: tmp)
            onProgress?(1, Int64(file.data.count), Int64(file.data.count))
            let cached = try DocumentCacheManager.shared.saveToCache(from: tmp, for: uri, fileName: file.name)
            return (cached, file.name)
        }
        
        // Fast-path: Check persistent local cache first
        let initialFileName: String = {
            let stripSuffix: (String) -> String = { s in
                let noHash = s.split(separator: "#", maxSplits: 1).first.map(String.init) ?? s
                return noHash.split(separator: "?", maxSplits: 1).first.map(String.init) ?? noHash
            }
            if let comps = URLComponents(string: uri),
               let paramUri = comps.queryItems?.first(where: { $0.name == "uri" })?.value, !paramUri.isEmpty {
                let cleanParam = stripSuffix(paramUri)
                let fn = (cleanParam as NSString).lastPathComponent.removingPercentEncoding ?? (cleanParam as NSString).lastPathComponent
                if !fn.isEmpty && fn != "raw" { return fn }
            }
            let cleanUri = stripSuffix(uri)
            let last = (cleanUri as NSString).lastPathComponent.removingPercentEncoding ?? (cleanUri as NSString).lastPathComponent
            return (last.isEmpty || last == "raw") ? "document" : last
        }()
        
        if !initialFileName.isEmpty && initialFileName != "document",
           let cached = DocumentCacheManager.shared.getCachedFile(for: uri, fileName: initialFileName) {
            return (cached, initialFileName)
        }
        
        let endpoint: URL
        let isDirectHttp = uri.hasPrefix("http://") || uri.hasPrefix("https://")
        if isDirectHttp, let directURL = URL(string: uri) {
            // Document/image download sheets (QuickLookPreviewSheet) must always fetch the original untouched file.
            endpoint = originalMediaURL(directURL)
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
        
        let (tempDownloadedURL, httpResp) = try await transport.download(
            request: request,
            allowE2EE: attachCredentials || NetworkTransport.isConfiguredGatewayOrigin(endpoint),
            onProgress: onProgress
        )
        
        guard (200...299).contains(httpResp.statusCode) else {
            let errData = (try? Data(contentsOf: tempDownloadedURL)) ?? Data()
            try? FileManager.default.removeItem(at: tempDownloadedURL)
            if httpResp.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let rawMsg = String(data: errData, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            let msg = rawMsg.isEmpty ? "HTTP \(httpResp.statusCode)" : rawMsg
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
            throw APIError.serverError(statusCode: 500, message: String(localized: "下载的文件为空"))
        }
        
        return (cachedURL, resolvedFileName)
    }

    public func fetchWeChatChatArchive(uri: String, cascadeId: String? = nil, baseURL: URL) async throws -> WeChatChatArchive {
        var components = URLComponents(url: baseURL.appendingPathComponent("api/v1/files/wechat-chat"), resolvingAgainstBaseURL: false)
        var queryItems: [URLQueryItem] = [URLQueryItem(name: "uri", value: normalizeFileURIForGateway(uri))]
        if let cascadeId = cascadeId, !cascadeId.isEmpty {
            queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
        }
        components?.queryItems = queryItems
        guard let endpoint = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 30.0

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
            return try JSONDecoder().decode(WeChatChatArchive.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }

    public func zipEntryURL(uri: String, entry: String, cascadeId: String? = nil, baseURL: URL, thumb: Bool = false) -> URL? {
        var components = URLComponents(url: baseURL.appendingPathComponent("api/v1/files/zip-entry"), resolvingAgainstBaseURL: false)
        var queryItems: [URLQueryItem] = [
            URLQueryItem(name: "uri", value: normalizeFileURIForGateway(uri)),
            URLQueryItem(name: "entry", value: entry)
        ]
        if let cascadeId = cascadeId, !cascadeId.isEmpty {
            queryItems.append(URLQueryItem(name: "cascade_id", value: cascadeId))
        }
        if thumb {
            queryItems.append(URLQueryItem(name: "thumb", value: "1"))
        }
        components?.queryItems = queryItems
        return components?.url
    }

    public func fetchZipEntryData(
        uri: String,
        entry: String,
        cascadeId: String? = nil,
        baseURL: URL,
        thumb: Bool = false
    ) async throws -> Data {
        guard let endpoint = zipEntryURL(
            uri: uri,
            entry: entry,
            cascadeId: cascadeId,
            baseURL: baseURL,
            thumb: thumb
        ) else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 25.0

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
        return data
    }

    private func normalizeFileURIForGateway(_ uri: String) -> String {
        var cleanPath = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        if cleanPath.hasPrefix("MEDIA:") {
            cleanPath = String(cleanPath.dropFirst(6)).trimmingCharacters(in: .whitespaces)
        }
        if cleanPath.hasPrefix("file://") {
            cleanPath = String(cleanPath.dropFirst(7))
        }
        return cleanPath.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
    }
    
}
