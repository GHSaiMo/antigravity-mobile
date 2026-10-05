import Foundation

/// Answers requests aimed at `DemoGateway.baseURL` locally (demo mode). Any other request is
/// left to the network stack.
final class DemoURLProtocol: URLProtocol, @unchecked Sendable {
    override class func canInit(with request: URLRequest) -> Bool {
        guard DemoGateway.isEnabled, let url = request.url else { return false }
        return url.host == DemoGateway.host && url.port == DemoGateway.port
    }
    
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    
    override func stopLoading() {}
    
    override func startLoading() {
        let request = self.request
        // Uploads are delivered as a body stream; give them a short delay so progress UI is visible.
        let path = request.url?.path ?? ""
        let delay: TimeInterval
        switch path {
        case "/api/v1/attachments": delay = 0.8
        case "/api/v1/cockpit/refresh": delay = 1.0
        case "/api/v1/cockpit/switch": delay = 1.5
        default: delay = 0.05
        }
        let body = Self.readBody(request)
        DispatchQueue.global().asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self else { return }
            let result = DemoRouter.handle(request: request, body: body)
            self.finish(url: request.url!, result: result)
        }
    }
    
    private func finish(url: URL, result: DemoRouter.Result) {
        var headers = ["Content-Type": result.contentType, "Content-Length": "\(result.body.count)"]
        for (k, v) in result.headers { headers[k] = v }
        let response = HTTPURLResponse(url: url, statusCode: result.status, httpVersion: "HTTP/1.1", headerFields: headers)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: result.body)
        client?.urlProtocolDidFinishLoading(self)
    }
    
    private static func readBody(_ request: URLRequest) -> Data {
        if let b = request.httpBody { return b }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 64 * 1024)
        while stream.hasBytesAvailable {
            let n = stream.read(&buffer, maxLength: buffer.count)
            if n <= 0 { break }
            data.append(buffer, count: n)
        }
        return data
    }
}

enum DemoRouter {
    struct Result {
        var status = 200
        var contentType = "application/json"
        var body: Data
        var headers: [String: String] = [:]
    }
    
    private static func json(_ obj: Any, status: Int = 200) -> Result {
        let data = (try? JSONSerialization.data(withJSONObject: obj)) ?? Data("{}".utf8)
        return Result(status: status, body: data)
    }
    
    private static func parse(_ body: Data) -> [String: Any] {
        (try? JSONSerialization.jsonObject(with: body)) as? [String: Any] ?? [:]
    }
    
    static func handle(request: URLRequest, body: Data) -> Result {
        let gw = DemoGateway.shared
        let url = request.url!
        let path = url.path
        let query = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        func q(_ name: String) -> String? { query.first(where: { $0.name == name })?.value }
        
        // ConnectRPC: /api/exa.language_server_pb.LanguageServerService/<Method>
        if let range = path.range(of: "LanguageServerService/") {
            let method = String(path[range.upperBound...])
            let req = parse(body)
            switch method {
            case "GetAllCascadeTrajectories":
                return json(["trajectorySummaries": gw.trajectorySummaries()])
            case "SendUserCascadeMessage":
                if let err = gw.send(body: req) {
                    return json(["error": "invalid_attachments", "message": err], status: 400)
                }
                return json([:])
            case "UpdateConversationAnnotations":
                let ids = req["cascadeIds"] as? [String] ?? []
                let ann = req["annotations"] as? [String: Any] ?? [:]
                if let title = ann["title"] as? String { gw.rename(ids: ids, title: title) }
                if ann["markedAsUnread"] as? Bool == false { gw.markRead(ids: ids) }
                return json([:])
            case "DeleteAgentMessage":
                if let cid = req["recipient"] as? String, let mid = req["messageId"] as? String {
                    gw.deleteQueued(cascadeId: cid, messageId: mid)
                }
                return json([:])
            case "DeleteCascadeTrajectory":
                if let id = req["cascadeId"] as? String { gw.delete(id: id) }
                return json([:])
            case "CancelCascadeInvocation":
                if let id = req["cascadeId"] as? String { gw.cancel(id: id) }
                return json([:])
            default:
                return json([:])
            }
        }
        
        switch path {
        case "/gateway/status":
            return json(["status": "connected", "os": "darwin", "platform": "darwin"])
        case "/gateway/projects":
            return json(gw.projectsPayload())
        case "/gateway/cascade/interaction":
            let req = parse(body)
            gw.submitInteraction(
                cascadeId: req["cascadeId"] as? String ?? "",
                optionId: req["optionId"] as? String ?? "",
                answers: req["questionResponses"] as? [[String: Any]] ?? []
            )
            return json([:])
        case "/gateway/cascade/task/stop":
            gw.stopTask(cascadeId: parse(body)["cascadeId"] as? String ?? "")
            return json([:])
        case "/gateway/cascade/new":
            let req = parse(body)
            let project = gw.project(forURI: req["workspaceUri"] as? String ?? "", id: req["projectId"] as? String)
            return json(["cascadeId": gw.createConversation(project: project, prompt: req["prompt"] as? String ?? "")])
        case "/gateway/cascade/messages":
            guard let id = q("cascadeId"), let payload = gw.messagesPayload(cascadeId: id) else {
                return json(["error": "not_found"], status: 404)
            }
            return json(payload)
        case "/api/v1/cockpit/quotas":
            return json(gw.quotasPayload())
        case "/api/v1/cockpit/refresh":
            gw.refreshQuotas()
            return json(gw.quotasPayload())
        case "/api/v1/cockpit/switch":
            if let id = parse(body)["account_id"] as? String { gw.switchQuotaAccount(id: id) }
            return json(["status": "ok"])
        case "/api/v1/version":
            return json(["app_name": "Multigravity Demo", "os": "darwin", "version": "demo"])
        case "/api/v1/attachments":
            let rawName = request.value(forHTTPHeaderField: "X-File-Name") ?? "file"
            let name = rawName.removingPercentEncoding ?? rawName
            let res = gw.storeUpload(name: (name as NSString).lastPathComponent, data: body)
            return json(res.json, status: res.status)
        case "/api/v1/files/raw", "/api/v1/files/content":
            guard let uri = q("uri"), let file = gw.fileData(forPath: uri) else {
                return Result(status: 404, contentType: "text/plain", body: Data("file not found".utf8))
            }
            if path.hasSuffix("/content") {
                return json(["uri": "file://" + uri, "filename": file.name, "content": String(decoding: file.data, as: UTF8.self)])
            }
            return Result(contentType: DemoGateway.mime(for: file.name), body: file.data)
        default:
            // focus, task stop, interaction, revert, auth/*, ... : acknowledge
            return json([:])
        }
    }
}
