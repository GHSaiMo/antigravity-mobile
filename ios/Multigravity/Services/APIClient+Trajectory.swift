import Foundation

extension APIClient {
    // Fast lightweight paginated messages endpoint served by Go gateway
    public func fetchMessages(
        cascadeId: String,
        limit: Int = 10,
        offset: Int? = nil,
        baseURL: URL
    ) async throws -> FetchMessagesResult {
        var comps = URLComponents(url: baseURL.appendingPathComponent("gateway/cascade/messages"), resolvingAgainstBaseURL: true)
        var queryItems = [
            URLQueryItem(name: "cascadeId", value: cascadeId),
            URLQueryItem(name: "limit", value: "\(limit)")
        ]
        if let o = offset {
            queryItems.append(URLQueryItem(name: "offset", value: "\(o)"))
        }
        comps?.queryItems = queryItems
        
        guard let url = comps?.url else { throw APIError.invalidURL }
        
        var request = URLRequest(url: url)
        request.timeoutInterval = 15
        
        do {
            let (data, response) = try await transport.send(request: request)
            if let httpResp = response as? HTTPURLResponse, httpResp.statusCode == 200 {
                let decoded = try JSONDecoder().decode(PaginatedMessagesResponse.self, from: data)
                let chatMessages = decoded.messages.map { item -> ChatMessage in
                    let sender: ChatMessage.MessageSender = {
                        switch item.type {
                        case "user": return .user
                        case "agent": return .agent
                        case "error": return .error
                        case "subagent": return .subagent
                        default: return .toolBatch(count: item.toolCount ?? 1, tools: item.toolNames ?? [])
                        }
                    }()
                    
                    let imgDataList = (item.media ?? []).compactMap { Data(base64Encoded: $0) }
                    
                    let resolvedImageUrls: [String] = {
                        let rawList = item.imageUrls ?? []
                        return rawList.map { self.resolveMediaURL($0, baseURL: baseURL) }
                    }()
                    
                    return ChatMessage(
                        id: item.id,
                        sender: sender,
                        content: item.text,
                        toolCount: item.toolCount ?? 0,
                        toolNames: item.toolNames ?? [],
                        imageDataList: imgDataList,
                        imageUrls: resolvedImageUrls,
                        artifacts: item.artifacts ?? [],
                        stepIndex: item.stepIndex,
                        attemptCount: item.attemptCount,
                        maxAttempts: item.maxAttempts,
                        model: item.model,
                        modelName: item.modelName,
                        subagent: item.subagent
                    )
                }
                
                let lastUserIdx = chatMessages.lastIndex(where: { $0.isUser }) ?? -1
                let latestTurnMessages = lastUserIdx >= 0 ? chatMessages.suffix(from: lastUserIdx + 1) : chatMessages[...]
                let latestTurnHasErr = latestTurnMessages.contains(where: { $0.isError }) && !(latestTurnMessages.last?.isAgent == true)
                let isErr = decoded.hasError ?? (decoded.status == "CASCADE_RUN_STATUS_ERROR" || latestTurnHasErr)
                let errMsg = decoded.errorMessage ?? latestTurnMessages.last(where: { $0.isError })?.content
                
                return FetchMessagesResult(
                    status: isErr ? "CASCADE_RUN_STATUS_ERROR" : decoded.status,
                    messages: chatMessages,
                    totalSteps: decoded.totalSteps,
                    totalTools: decoded.totalTools,
                    duration: decoded.duration,
                    hasMore: decoded.hasMore,
                    nextOffset: decoded.nextOffset,
                    cascadeConfigRaw: decoded.cascadeConfigRaw,
                    title: decoded.title,
                    canProceed: decoded.canProceed ?? false,
                    proceedArtifactUri: decoded.proceedArtifactUri,
                    pendingInteraction: decoded.pendingInteraction,
                    queuedMessages: decoded.queuedMessages ?? [],
                    runningTasks: decoded.runningTasks ?? [],
                    activeModel: decoded.activeModel,
                    activeModelName: decoded.activeModelName,
                    modelDisplayName: decoded.modelDisplayName,
                    startedAt: decoded.startedAt,
                    hasError: isErr,
                    errorMessage: errMsg
                )
            }
        } catch {
            // Fallback to full trajectory fetch if gateway custom endpoint fails
        }
        
        let (status, msgs, steps, tools, dur, title, hasErr, errMsg) = try await fetchTrajectory(cascadeId: cascadeId, baseURL: baseURL)
        return FetchMessagesResult(
            status: status,
            messages: msgs,
            totalSteps: steps,
            totalTools: tools,
            duration: dur,
            hasMore: false,
            nextOffset: 0,
            cascadeConfigRaw: nil,
            title: title,
            canProceed: false,
            proceedArtifactUri: nil,
            pendingInteraction: nil,
            queuedMessages: [],
            runningTasks: [],
            activeModel: nil,
            activeModelName: nil,
            modelDisplayName: nil,
            startedAt: nil,
            hasError: hasErr,
            errorMessage: errMsg
        )
    }
    
    // Fetch trajectory steps and parse into streamlined ChatMessage array
    public func fetchTrajectory(cascadeId: String, baseURL: URL) async throws -> (status: String, messages: [ChatMessage], totalSteps: Int, totalTools: Int, duration: String, title: String?, hasError: Bool, errorMessage: String?) {
        let req = GetCascadeTrajectoryRequest(cascadeId: cascadeId)
        let resp: GetCascadeTrajectoryResponse = try await rpc(
            method: "GetCascadeTrajectory",
            body: req,
            baseURL: baseURL
        )
        
        guard let traj = resp.trajectory, let steps = traj.steps else {
            return ("UNKNOWN", [], 0, 0, "0秒", nil, false, nil)
        }
        
        var messages: [ChatMessage] = []
        var pendingTools: [String] = []
        var totalToolsCount = 0
        
        func flushTools() {
            guard !pendingTools.isEmpty else { return }
            let count = pendingTools.count
            totalToolsCount += count
            let allTools = pendingTools
            let toolId = "tools-\(messages.count)"
            messages.append(ChatMessage(
                id: toolId,
                sender: .toolBatch(count: count, tools: allTools),
                content: "已执行 \(count) 次工具调用",
                toolCount: count,
                toolNames: allTools
            ))
            pendingTools.removeAll()
        }
        
        for (idx, step) in steps.enumerated() {
            let type = step.type ?? ""
            
            if type == "CORTEX_STEP_TYPE_USER_INPUT" {
                flushTools()
                let text = step.userInput?.userResponse ?? step.userInput?.items?.first?.text ?? ""
                let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
                let isSystemApproval = trimmed.hasPrefix("Comments on artifact URI:") || trimmed.contains("The user has approved this document")
                
                var images: [Data] = []
                var imageUrls: [String] = []
                if let mediaList = step.userInput?.media {
                    for media in mediaList {
                        if let u = media.uri, !u.isEmpty {
                            imageUrls.append(resolveMediaURL(u, baseURL: baseURL))
                        }
                        if let thumb = media.thumbnail, !thumb.isEmpty, let data = Data(base64Encoded: thumb) {
                            images.append(data)
                        } else if let inline = media.inlineData, !inline.isEmpty, let data = Data(base64Encoded: inline) {
                            images.append(data)
                        }
                    }
                }
                if let imgList = step.userInput?.images {
                    for img in imgList {
                        if let b64 = img.base64Data, !b64.isEmpty, let data = Data(base64Encoded: b64) {
                            images.append(data)
                        }
                    }
                }
                
                for u in extractImageURLs(from: text) {
                    let resolved = resolveMediaURL(u, baseURL: baseURL)
                    if !imageUrls.contains(resolved) {
                        imageUrls.append(resolved)
                    }
                }
                
                if (!trimmed.isEmpty && !isSystemApproval) || !images.isEmpty || !imageUrls.isEmpty {
                    messages.append(ChatMessage(
                        id: "step-\(idx)",
                        sender: .user,
                        content: text,
                        imageDataList: images,
                        imageUrls: imageUrls,
                        stepIndex: idx
                    ))
                }
            } else if type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
                let response = step.plannerResponse?.response?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                let thinking = step.plannerResponse?.thinking?.trimmingCharacters(in: .whitespacesAndNewlines)
                
                if !response.isEmpty || (thinking != nil && !thinking!.isEmpty) {
                    flushTools()
                    
                    let extracted = extractImageURLs(from: response)
                    let resolved = extracted.map { resolveMediaURL($0, baseURL: baseURL) }
                    
                    messages.append(ChatMessage(
                        id: "step-\(idx)",
                        sender: .agent,
                        content: response.isEmpty ? "（已完成思考，准备下发指令）" : response,
                        thinking: thinking?.isEmpty == false ? thinking : nil,
                        imageUrls: resolved,
                        stepIndex: idx
                    ))
                }
            } else if type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
                let isUserVisible: Bool = {
                    if let em = step.errorMessage {
                        if em.shouldShowUser == true { return true }
                        if em.shouldShowModel == true { return false }
                        let short = (em.shortError ?? "").lowercased()
                        let userMsg = (em.userErrorMessage ?? "").lowercased()
                        if short.contains("stream was interrupted") || userMsg.contains("stream was interrupted") ||
                            short.contains("model produced invalid output") || userMsg.contains("model produced invalid output") {
                            return false
                        }
                        return true
                    }
                    if let err = step.error {
                        let short = (err.message ?? err.detail ?? "").lowercased()
                        if short.contains("stream was interrupted") || short.contains("model produced invalid output") {
                            return false
                        }
                        return true
                    }
                    return false
                }()
                if !isUserVisible {
                    continue
                }
                flushTools()
                let errText = step.errorMessage?.userErrorMessage
                    ?? step.errorMessage?.shortError
                    ?? step.errorMessage?.message
                    ?? step.error?.message
                    ?? "执行遇到错误"
                let trimmed = errText.trimmingCharacters(in: .whitespacesAndNewlines)
                
                let parsed = Self.parseAttemptError(trimmed)
                if let last = messages.last, last.isError {
                    let prevParsed = Self.parseAttemptError(last.content)
                    if parsed.isAttempt && prevParsed.isAttempt && Self.normalizeErrKey(prevParsed.baseError) == Self.normalizeErrKey(parsed.baseError) {
                        let newAttempt = max(parsed.attempt, (last.attemptCount ?? prevParsed.attempt) + 1)
                        let maxAttempts = parsed.maxAttempts > 0 ? parsed.maxAttempts : (last.maxAttempts ?? 9)
                        let formatted = "\(parsed.prefix) (attempt \(newAttempt)/\(maxAttempts) · 已重试 \(newAttempt) 次): \(parsed.baseError)"
                        messages[messages.count - 1] = ChatMessage(
                            id: last.id,
                            sender: .error,
                            content: formatted,
                            stepIndex: last.stepIndex,
                            attemptCount: newAttempt,
                            maxAttempts: maxAttempts
                        )
                        continue
                    } else if !parsed.isAttempt && !prevParsed.isAttempt && Self.normalizeErrKey(last.content) == Self.normalizeErrKey(trimmed) {
                        let count = (last.attemptCount ?? 1) + 1
                        let formatted = "\(trimmed) (已重试 \(count) 次)"
                        messages[messages.count - 1] = ChatMessage(
                            id: last.id,
                            sender: .error,
                            content: formatted,
                            stepIndex: last.stepIndex,
                            attemptCount: count,
                            maxAttempts: nil
                        )
                        continue
                    }
                }
                
                let formatted = parsed.isAttempt ? "\(parsed.prefix) (attempt \(parsed.attempt)/\(parsed.maxAttempts)): \(parsed.baseError)" : trimmed
                messages.append(ChatMessage(
                    id: "step-\(idx)",
                    sender: .error,
                    content: formatted,
                    stepIndex: idx,
                    attemptCount: parsed.isAttempt ? parsed.attempt : nil,
                    maxAttempts: parsed.isAttempt ? parsed.maxAttempts : nil
                ))
            } else if type.hasPrefix("CORTEX_STEP_TYPE_") && type != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" {
                let toolName: String = {
                    if let name = step.metadata?.toolCall?.name, !name.isEmpty {
                        return name
                    }
                    if let name = step.toolCall?.name, !name.isEmpty {
                        return name
                    }
                    if let action = step.metadata?.toolAction, !action.isEmpty {
                        return action
                    }
                    return type
                        .replacingOccurrences(of: "CORTEX_STEP_TYPE_", with: "")
                        .lowercased()
                }()
                pendingTools.append(toolName)
            }
        }
        
        flushTools()
        
        // Determine whether the latest turn has an unrecovered error
        var lastUserInputIndex = -1
        for (i, step) in steps.enumerated().reversed() {
            if step.type == "CORTEX_STEP_TYPE_USER_INPUT" {
                lastUserInputIndex = i
                break
            }
        }
        
        var latestTurnHasError = traj.hasError ?? false
        var latestTurnErrorMessage = traj.errorMessage
        
        var foundErrorInTurn = false
        var lastErrInTurn: String?
        for i in (lastUserInputIndex + 1)..<steps.count {
            let step = steps[i]
            if step.type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
                let isUserVisible: Bool = {
                    if let em = step.errorMessage {
                        if em.shouldShowUser == true { return true }
                        if em.shouldShowModel == true { return false }
                        let short = (em.shortError ?? "").lowercased()
                        let userMsg = (em.userErrorMessage ?? "").lowercased()
                        if short.contains("stream was interrupted") || userMsg.contains("stream was interrupted") ||
                            short.contains("model produced invalid output") || userMsg.contains("model produced invalid output") {
                            return false
                        }
                        return true
                    }
                    if let err = step.error {
                        let short = (err.message ?? err.detail ?? "").lowercased()
                        if short.contains("stream was interrupted") || short.contains("model produced invalid output") {
                            return false
                        }
                        return true
                    }
                    return false
                }()
                if isUserVisible {
                    foundErrorInTurn = true
                    let errText = step.errorMessage?.userErrorMessage
                        ?? step.errorMessage?.shortError
                        ?? step.errorMessage?.message
                        ?? step.error?.message
                        ?? "执行遇到错误"
                    lastErrInTurn = errText.trimmingCharacters(in: .whitespacesAndNewlines)
                }
            } else if step.type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
                let resp = step.plannerResponse?.response?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                if !resp.isEmpty {
                    foundErrorInTurn = false
                    lastErrInTurn = nil
                }
            }
        }
        
        if foundErrorInTurn {
            latestTurnHasError = true
            if latestTurnErrorMessage == nil {
                latestTurnErrorMessage = lastErrInTurn
            }
        } else if traj.hasError != true {
            latestTurnHasError = false
            latestTurnErrorMessage = nil
        }
        
        // Calculate total duration
        var durationString = "0秒"
        let firstCreated = steps.first(where: { $0.metadata?.createdAt != nil })?.metadata?.createdAt
        let lastCreated = steps.last(where: { $0.metadata?.createdAt != nil })?.metadata?.createdAt
        
        if let firstStr = firstCreated, let lastStr = lastCreated {
            let fmtWithFraction = ISO8601DateFormatter()
            fmtWithFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let fmtStandard = ISO8601DateFormatter()
            
            let startDate = fmtWithFraction.date(from: firstStr) ?? fmtStandard.date(from: firstStr)
            let endDate = fmtWithFraction.date(from: lastStr) ?? fmtStandard.date(from: lastStr)
            
            if let start = startDate, let end = endDate {
                let diff = max(0, Int(end.timeIntervalSince(start)))
                let hours = diff / 3600
                let minutes = (diff % 3600) / 60
                let secs = diff % 60
                if hours > 0 {
                    durationString = "\(hours)小时\(minutes)分"
                } else if minutes > 0 {
                    durationString = "\(minutes)分\(secs)秒"
                } else {
                    durationString = "\(secs)秒"
                }
            }
        }
        
        var runStatus = resp.status ?? "DONE"
        if latestTurnHasError || runStatus == "CASCADE_RUN_STATUS_ERROR" {
            latestTurnHasError = true
            runStatus = "CASCADE_RUN_STATUS_ERROR"
        }
        let title = traj.annotations?.title ?? traj.summary
        return (runStatus, messages, steps.count, totalToolsCount, durationString, title, latestTurnHasError, latestTurnErrorMessage)
    }
    
    static func parseAttemptError(_ text: String) -> (isAttempt: Bool, prefix: String, attempt: Int, maxAttempts: Int, baseError: String) {
        let pattern = #"(?i)^(.*?)\s*\(attempt\s+(\d+)(?:\s*(?:of|/)\s*(\d+))?(?:\s*[·,]\s*[^)]*)?\)\s*[:：\-]?\s*(.*)$"#
        guard let regex = try? NSRegularExpression(pattern: pattern, options: .dotMatchesLineSeparators) else {
            return (false, "API error", 0, 0, text)
        }
        let ns = text as NSString
        guard let m = regex.firstMatch(in: text, options: [], range: NSRange(location: 0, length: ns.length)) else {
            return (false, "API error", 0, 0, text)
        }
        var prefix = ns.substring(with: m.range(at: 1)).trimmingCharacters(in: .whitespacesAndNewlines)
        if prefix.isEmpty { prefix = "API error" }
        let attempt = Int(ns.substring(with: m.range(at: 2))) ?? 1
        var maxAttempts = 0
        if m.numberOfRanges > 3 && m.range(at: 3).location != NSNotFound {
            maxAttempts = Int(ns.substring(with: m.range(at: 3))) ?? 0
        }
        if maxAttempts == 0 {
            maxAttempts = attempt <= 9 ? 9 : attempt
        }
        let baseError = ns.substring(with: m.range(at: 4)).trimmingCharacters(in: .whitespacesAndNewlines)
        return (true, prefix, attempt, maxAttempts, baseError)
    }

    static func normalizeErrKey(_ text: String) -> String {
        var s = text.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        while s.hasSuffix(".") || s.hasSuffix(":") || s.hasSuffix(";") || s.hasSuffix(",") {
            s.removeLast()
        }
        return s.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
