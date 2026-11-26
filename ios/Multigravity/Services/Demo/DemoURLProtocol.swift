import Foundation

/// Answers requests aimed at `DemoGateway.baseURL` locally (demo mode). Any other request is
/// declined in `canInit` so it flows through the normal network stack untouched.
final class DemoURLProtocol: URLProtocol, @unchecked Sendable {
    override class func canInit(with request: URLRequest) -> Bool {
        guard DemoGateway.isEnabled, let url = request.url else { return false }
        return url.host == DemoGateway.host && url.port == DemoGateway.port
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let req = request
        let body = Self.bodyData(for: req)
        let result = DemoRouter.handle(request: req, body: body)
        guard let url = req.url else {
            client?.urlProtocolDidFinishLoading(self)
            return
        }
        let response = HTTPURLResponse(
            url: url,
            statusCode: result.status,
            httpVersion: "HTTP/1.1",
            headerFields: [
                "Content-Type": result.contentType,
                "Content-Length": "\(result.body.count)"
            ]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: result.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    /// URLSession drops `httpBody` on requests passed into `URLProtocol` when `httpBodyStream` is used.
    private static func bodyData(for request: URLRequest) -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var data = Data()
        let bufSize = 16 * 1024
        let buf = UnsafeMutablePointer<UInt8>.allocate(capacity: bufSize)
        defer { buf.deallocate() }
        while stream.hasBytesAvailable {
            let n = stream.read(buf, maxLength: bufSize)
            if n <= 0 { break }
            data.append(buf, count: n)
        }
        return data
    }
}

private enum DemoRouter {
    struct Result {
        var status: Int = 200
        var contentType: String = "application/json"
        var body: Data
    }

    static func json(_ object: Any, status: Int = 200) -> Result {
        let data = (try? JSONSerialization.data(withJSONObject: object, options: [])) ?? Data("{}".utf8)
        return Result(status: status, contentType: "application/json", body: data)
    }

    static func rawJSON(_ data: Data, status: Int = 200) -> Result {
        Result(status: status, contentType: "application/json", body: data)
    }

    static func bodyJSON(_ body: Data) -> [String: Any] {
        (try? JSONSerialization.jsonObject(with: body) as? [String: Any]) ?? [:]
    }

    static func handle(request: URLRequest, body: Data) -> Result {
        guard let url = request.url else { return json(["error": "invalid url"], status: 400) }
        let path = url.path
        let method = (request.httpMethod ?? "GET").uppercased()
        let query = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let param: (String) -> String? = { name in query.first(where: { $0.name == name })?.value }
        let gw = DemoGateway.shared

        // LanguageServer ConnectRPC endpoints
        let rpcPrefixes = [
            "/api/exa.language_server_pb.LanguageServerService/",
            "/exa.language_server_pb.LanguageServerService/"
        ]
        for rpcPrefix in rpcPrefixes where path.hasPrefix(rpcPrefix) {
            let rpc = String(path.dropFirst(rpcPrefix.count))
            return handleRPC(rpc: rpc, request: request, body: body, gw: gw)
        }

        // REST Gateway & Cockpit endpoints
        switch (method, path) {
        case ("GET", "/gateway/status"):
            return json(gw.statusPayload())
        case ("GET", "/gateway/models"):
            return rawJSON(gw.modelsPayload())
        case ("POST", "/gateway/slash-commands"):
            return json(gw.slashCommandsPayload())
        case ("GET", "/gateway/projects"):
            return json(gw.projectsPayload())
        case ("POST", "/gateway/projects/alias"):
            let obj = bodyJSON(body)
            let p = (obj["path"] as? String) ?? ""
            let alias = (obj["alias"] as? String) ?? ""
            gw.updateProjectAlias(path: p, alias: alias)
            return json(["ok": true])
        case ("POST", "/gateway/projects/create"):
            let obj = bodyJSON(body)
            let p = (obj["path"] as? String) ?? ""
            let name = obj["name"] as? String
            return json(gw.createProject(path: p, name: name))
        case ("GET", "/gateway/fs/dirs"):
            let p = param("path")
            return json(gw.listLocalDirs(path: p))
        case ("POST", "/gateway/fs/mkdir"):
            let obj = bodyJSON(body)
            return json(gw.createLocalDir(
                parentPath: obj["parentPath"] as? String,
                name: obj["name"] as? String,
                path: obj["path"] as? String
            ))
        case ("POST", "/gateway/projects/clone"):
            let obj = bodyJSON(body)
            let slug = (obj["repoSlug"] as? String) ?? (obj["repoUrl"] as? String) ?? ""
            return json(gw.cloneGitHubRepo(
                repoSlug: slug,
                parentDir: obj["parentDir"] as? String,
                folderName: obj["folderName"] as? String
            ))
        case ("POST", "/gateway/projects/clone/cancel"):
            return json(["ok": true, "status": "cancelled"])

        case ("POST", "/gateway/cascade/interaction"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? (obj["trajectoryId"] as? String) ?? ""
            let step = (obj["stepIndex"] as? Int) ?? 0
            let allow = obj["allow"] as? Bool
            let optionId = (obj["optionId"] as? String) ?? (obj["selectedOptionId"] as? String)
            let writeIn = obj["writeInResponse"] as? String
            let skipped = (obj["skipped"] as? Bool) ?? false
            let answers = (obj["questionResponses"] as? [[String: Any]]) ?? (obj["answers"] as? [[String: Any]])
            gw.respond(
                cascadeId: id,
                stepIndex: step,
                allow: allow,
                optionId: optionId,
                writeInResponse: writeIn,
                skipped: skipped,
                answers: answers
            )
            return json(["ok": true])

        case ("POST", "/gateway/cascade/task/stop"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let taskId = (obj["taskId"] as? String) ?? ""
            gw.stopTask(cascadeId: id, taskId: taskId)
            return json(["ok": true])

        case ("POST", "/gateway/subagent/stop"):
            let obj = bodyJSON(body)
            let convId = (obj["conversationId"] as? String) ?? ""
            gw.stopSubagent(conversationId: convId)
            return json(["ok": true])

        case ("POST", "/gateway/cascade/new"):
            let obj = bodyJSON(body)
            let workspace = (obj["workspace"] as? String) ?? (obj["workspaceUri"] as? String)
            let model = obj["model"] as? String
            let prompt = (obj["prompt"] as? String) ?? ""
            let cmid = request.value(forHTTPHeaderField: "X-Client-Message-Id")
            let id = gw.createConversation(workspace: workspace, model: model)
            if !prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                gw.send(cascadeId: id, text: prompt, clientMessageId: cmid)
            }
            return json(["cascadeId": id])

        case ("GET", "/gateway/cascade/messages"):
            let id = param("cascadeId") ?? ""
            let limit = Int(param("limit") ?? "") ?? 20
            let offset = param("offset").flatMap(Int.init)
            if let payload = gw.messagesPayload(cascadeId: id, limit: limit, offset: offset) {
                return json(payload)
            }
            return json(["error": "not found"], status: 404)

        case ("POST", "/gateway/cascade/changes"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            return json(gw.changesPayload(cascadeId: id))

        case ("POST", "/gateway/cascade/chapters"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let ranges = (obj["ranges"] as? [[String: Any]]) ?? []
            return json(gw.chaptersPayload(cascadeId: id, ranges: ranges))

        case ("POST", "/gateway/cascade/revert/preview"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let stepIndex = (obj["stepIndex"] as? Int) ?? 0
            return json(gw.revertPreviewPayload(cascadeId: id, stepIndex: stepIndex))

        case ("POST", "/gateway/cascade/revert/execute"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let stepIndex = (obj["stepIndex"] as? Int) ?? 0
            let conversationOnly = (obj["conversationOnly"] as? Bool) ?? false
            return json(gw.revertExecute(cascadeId: id, stepIndex: stepIndex, conversationOnly: conversationOnly))

        case ("POST", "/gateway/git/status"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            if let st = gw.gitStatusPayload(cascadeId: id) {
                return json(st)
            }
            return json(["error": "not a git workspace"], status: 404)

        case ("POST", "/gateway/git/commit"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let msg = (obj["message"] as? String) ?? "chore: demo commit"
            let paths = (obj["paths"] as? [String]) ?? []
            let push = (obj["push"] as? Bool) ?? false
            return json(gw.gitCommit(cascadeId: id, message: msg, paths: paths, push: push))

        case ("POST", "/gateway/git/push"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            return json(gw.gitPush(cascadeId: id))

        case ("GET", "/gateway/swarm"):
            let id = param("cascadeId") ?? ""
            return json(gw.swarmSnapshotPayload(cascadeId: id))

        case ("POST", "/gateway/swarm/feedback"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let feedback = (obj["feedback"] as? String) ?? ""
            return json(gw.sendSwarmFeedback(cascadeId: id, feedback: feedback))

        case ("POST", "/gateway/swarm/nudge"):
            let obj = bodyJSON(body)
            let id = (obj["cascadeId"] as? String) ?? ""
            let workerName = (obj["workerName"] as? String) ?? ""
            return json(gw.nudgeSwarmMember(cascadeId: id, workerName: workerName))

        case ("GET", "/gateway/plugins"):
            return json(gw.pluginsPayload())

        case ("POST", "/gateway/plugins/install"):
            let obj = bodyJSON(body)
            let pluginId = (obj["pluginId"] as? String) ?? ""
            return json(gw.installPlugin(pluginId: pluginId))

        case ("GET", "/gateway/automations"):
            return json(gw.automationsPayload())

        case ("POST", "/gateway/automations/manage"):
            let obj = bodyJSON(body)
            let sidecarId = (obj["sidecarId"] as? String) ?? ""
            let action = (obj["action"] as? String) ?? ""
            return json(gw.manageAutomation(sidecarId: sidecarId, action: action))

        case ("POST", "/gateway/automations/update"):
            let obj = bodyJSON(body)
            let sidecarId = (obj["sidecarId"] as? String) ?? ""
            return json(["ok": true, "sidecarId": sidecarId])

        case ("GET", "/gateway/automations/logs"):
            let sid = URLComponents(url: url, resolvingAgainstBaseURL: false)?
                .queryItems?
                .first(where: { $0.name == "sidecarId" })?
                .value ?? "demo-schedule"
            return json([
                "sidecarId": sid,
                "filename": "sidecar.log",
                "files": ["sidecar.log"],
                "logs": "=== sidecar \"\(sid)\" started ===\n[schedule] Scheduler active\n[schedule] Next execution scheduled",
                "truncated": false
            ])

        case ("GET", "/gateway/workflow-templates"):
            return rawJSON(gw.workflowTemplatesPayload())

        case ("PUT", "/gateway/workflow-templates"):
            return rawJSON(gw.saveWorkflowTemplates(data: body))

        case ("DELETE", "/gateway/workflow-templates"):
            return rawJSON(gw.resetWorkflowTemplates())

        case ("POST", "/gateway/tasks/background"):
            return json(gw.globalBackgroundTasksPayload())

        case ("POST", "/gateway/tasks/background/log"):
            let obj = bodyJSON(body)
            let convId = (obj["conversationId"] as? String) ?? ""
            let taskId = (obj["taskId"] as? String) ?? ""
            return json(gw.backgroundTaskLogPayload(conversationId: convId, taskId: taskId))

        case ("GET", "/api/v1/cockpit/quotas"):
            return json(gw.quotasPayload())

        case ("POST", "/api/v1/cockpit/refresh"):
            gw.refreshQuotas()
            return json(gw.quotasPayload())

        case ("POST", "/api/v1/cockpit/switch"):
            let obj = bodyJSON(body)
            let target = (obj["account_id"] as? String) ?? (obj["email"] as? String) ?? ""
            gw.switchAccount(to: target)
            return json(["ok": true])

        case ("GET", "/api/v1/cockpit/tokens/history"):
            return rawJSON(gw.tokenHistoryPayload())

        case ("GET", "/api/v1/cockpit/auto-switch"), ("POST", "/api/v1/cockpit/auto-switch"):
            return json([
                "config": [
                    "enabled": false,
                    "target_bucket": "gemini_5h",
                    "threshold_percent": 10,
                    "wait_until_idle": true,
                    "strategy": "highest_quota",
                    "candidates": [],
                    "debounce_sec": 15,
                    "cooldown_sec": 180
                ],
                "status": ["state": "disabled"],
                "cockpit_installed": true
            ])

        case ("GET", "/api/v1/version"):
            return json(["version": "2.35.0-demo", "gateway": "multigravity-demo"])

        case ("POST", "/api/v1/attachments"):
            let headerName = request.value(forHTTPHeaderField: "X-File-Name")?.removingPercentEncoding
            if let headerName, !headerName.isEmpty {
                let (status, payload) = gw.storeUpload(name: headerName, data: body)
                return json(payload, status: status)
            }
            let (mpName, fileBytes) = extractMultipartFile(contentType: request.value(forHTTPHeaderField: "Content-Type"), body: body)
            let savedName = mpName ?? "upload-\(UUID().uuidString.prefix(6)).bin"
            let (status, payload) = gw.storeUpload(name: savedName, data: fileBytes)
            return json(payload, status: status)

        case ("GET", "/api/v1/files/raw"), ("GET", "/api/v1/files/content"):
            let p = param("uri") ?? param("path") ?? ""
            guard let file = gw.fileData(forPath: p) else { return json(["error": "file not found"], status: 404) }
            if path == "/api/v1/files/content" {
                let text = String(data: file.data, encoding: .utf8) ?? ""
                let isPlan = p.contains("implementation_plan.md") || p.hasSuffix("plan.md")
                return json(["uri": p, "filename": file.name, "content": text, "user_facing": true, "request_feedback": isPlan])
            }
            return Result(contentType: DemoGateway.mime(for: file.name), body: file.data)

        default:
            return json([:])
        }
    }

    private static func handleRPC(rpc: String, request: URLRequest, body: Data, gw: DemoGateway) -> Result {
        let obj = bodyJSON(body)
        switch rpc {
        case "GetAllCascadeTrajectories":
            return json(gw.trajectoriesPayload())

        case "SendUserCascadeMessage":
            let id = (obj["cascadeId"] as? String) ?? ""
            let cmid = request.value(forHTTPHeaderField: "X-Client-Message-Id")
            let items = obj["items"] as? [[String: Any]] ?? []
            let artifactComments = obj["artifactComments"] as? [[String: Any]] ?? []
            var slashCommands: [[String: Any]] = []
            var textParts: [String] = []
            for item in items {
                if let inner = item["item"] as? [String: Any],
                   let sc = inner["slashCommand"] as? [String: Any],
                   let info = sc["info"] as? [String: Any],
                   let name = (info["name"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines),
                   !name.isEmpty {
                    let isSystem = SlashCommandIconCatalog.defaultSystemIcons[name] != nil
                    slashCommands.append([
                        "name": name,
                        "icon": SlashCommandIconCatalog.defaultSystemIcons[name] ?? "code_blocks",
                        "kind": isSystem ? "system" : "skill"
                    ])
                    textParts.append("/\(name)")
                } else if let slashName = (item["slashCommand"] as? String) ?? (item["commandName"] as? String), !slashName.isEmpty {
                    slashCommands.append([
                        "name": slashName,
                        "icon": SlashCommandIconCatalog.defaultSystemIcons[slashName] ?? "code_blocks",
                        "kind": SlashCommandIconCatalog.defaultSystemIcons[slashName] != nil ? "system" : "skill"
                    ])
                    textParts.append("/\(slashName)")
                } else if let cmdDict = item["slashCommand"] as? [String: Any], let name = cmdDict["name"] as? String {
                    slashCommands.append(["name": name, "icon": cmdDict["icon"] ?? "code_blocks", "kind": cmdDict["kind"] ?? "system"])
                    textParts.append("/\(name)")
                }
                if let t = item["text"] as? String {
                    textParts.append(t)
                }
            }
            let text = textParts.joined(separator: "")
            var media = (obj["media"] as? [[String: Any]])?.compactMap { $0["inlineData"] as? String } ?? []
            if media.isEmpty, let imgs = obj["images"] as? [[String: Any]] {
                media = imgs.compactMap { $0["base64Data"] as? String }
            }
            let attachmentIds = (obj["attachments"] as? [[String: Any]])?.compactMap { $0["id"] as? String } ?? []
            let deliveryStrategy = obj["deliveryStrategy"] as? Int
            let isArtifactApproval = text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !artifactComments.isEmpty
            if let err = gw.send(
                cascadeId: id,
                text: text,
                clientMessageId: cmid,
                media: media.isEmpty ? nil : media,
                attachmentIds: attachmentIds,
                deliveryStrategy: deliveryStrategy,
                slashCommands: slashCommands.isEmpty ? nil : slashCommands,
                isArtifactApproval: isArtifactApproval
            ) {
                return json(["error": err, "message": err], status: 400)
            }
            return json([:])

        case "UpdateConversationAnnotations":
            let id = (obj["cascadeId"] as? String) ?? ""
            let ann = obj["annotations"] as? [String: Any]
            gw.updateAnnotations(
                cascadeId: id,
                title: ann?["title"] as? String,
                markedAsUnread: ann?["markedAsUnread"] as? Bool,
                pinned: ann?["pinned"] as? Bool
            )
            return json([:])

        case "DeleteAgentMessage":
            let id = (obj["recipient"] as? String) ?? ""
            let msgId = (obj["messageId"] as? String) ?? ""
            gw.deleteQueuedMessage(cascadeId: id, messageId: msgId)
            return json([:])

        case "DeleteCascadeTrajectory":
            let id = (obj["cascadeId"] as? String) ?? ""
            gw.deleteConversation(cascadeId: id)
            return json([:])

        case "CancelCascadeInvocation":
            let id = (obj["cascadeId"] as? String) ?? ""
            let killBg = (obj["killBackgroundTasks"] as? Bool) ?? false
            gw.stop(cascadeId: id, killBackgroundTasks: killBg)
            return json([:])

        case "JetboxWriteState":
            let cid = request.value(forHTTPHeaderField: "X-Cascade-Id")
            let appState = obj["appState"] as? [String: Any]
            let modelEnum = (appState?["lastSelectedAgentModel"] as? String) ?? ""
            if !modelEnum.isEmpty {
                gw.switchModel(cascadeId: cid, modelEnum: modelEnum)
            }
            return json([:])

        case "SearchConversations":
            let query = (obj["query"] as? String) ?? ""
            return json(gw.searchConversationsPayload(query: query))

        case "ConvertTrajectoryToMarkdown":
            let id = (obj["conversationId"] as? String) ?? (obj["cascadeId"] as? String) ?? ""
            return json(gw.exportMarkdownPayload(cascadeId: id))

        case "GetMcpServerStates":
            return rawJSON(gw.mcpStatesPayload())

        case "GetAllSkills":
            return rawJSON(gw.skillsPayload())

        default:
            return json([:])
        }
    }

    private static func extractMultipartFile(contentType: String?, body: Data) -> (String?, Data) {
        guard let ct = contentType,
              let range = ct.range(of: "boundary="),
              let sep = "\r\n\r\n".data(using: .utf8) else {
            return (nil, body)
        }
        let boundary = "--" + ct[range.upperBound...].trimmingCharacters(in: CharacterSet(charactersIn: "\" ;"))
        guard let bData = boundary.data(using: .utf8),
              let firstBoundary = body.range(of: bData),
              let headerEnd = body.range(of: sep, in: firstBoundary.upperBound..<body.endIndex) else {
            return (nil, body)
        }
        let headerStr = String(data: body[firstBoundary.upperBound..<headerEnd.lowerBound], encoding: .utf8) ?? ""
        var filename: String? = nil
        if let fRange = headerStr.range(of: "filename=\"") {
            let tail = headerStr[fRange.upperBound...]
            if let endQuote = tail.firstIndex(of: "\"") {
                filename = String(tail[..<endQuote])
            }
        }
        let nextBoundary = body.range(of: bData, in: headerEnd.upperBound..<body.endIndex)?.lowerBound ?? body.endIndex
        let fileEnd = max(headerEnd.upperBound, nextBoundary - 2)
        return (filename, Data(body[headerEnd.upperBound..<fileEnd]))
    }
}
