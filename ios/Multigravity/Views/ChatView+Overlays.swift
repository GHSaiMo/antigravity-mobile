import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    @ViewBuilder
    var floatingCards: some View {
        if !viewModel.runningTasks.isEmpty {
            RunningTasksCardView(
                cascadeId: viewModel.cascadeId,
                items: viewModel.runningTasks,
                onStop: { task in
                    Task {
                        await viewModel.stopTask(task)
                    }
                },
                onSendToBackground: { task in
                    Task {
                        await viewModel.sendTaskToBackground(stepIndices: [task.stepIndex])
                    }
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
        
        // 底部只放仍在运行的子代理（方便随时关停）；已结束的留在消息流里的内联卡片中，不占底部位置
        let runningSubagents = viewModel.subagents.filter(\.isRunning)
        if !runningSubagents.isEmpty {
            SubagentsCardView(
                items: runningSubagents,
                canStop: GatewayCompatStore.shared.isAvailable(GatewayFeature.subagents),
                onStop: { item in
                    Task { await viewModel.stopSubagent(item) }
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }

        if !viewModel.queuedMessages.isEmpty {
            let activeSubagents = viewModel.subagents.filter { $0.isRunning }
            let blockingTasks = viewModel.runningTasks.filter { $0.isDaemon != true }
            let effectiveTasks = blockingTasks.isEmpty ? viewModel.runningTasks : blockingTasks
            let hasActiveBgWork = viewModel.isActivelyRunning || !effectiveTasks.isEmpty || !activeSubagents.isEmpty
            let blockingNames: [String] = {
                var names: [String] = []
                for sa in activeSubagents {
                    let label = (sa.role?.isEmpty == false ? sa.role : sa.typeName) ?? ""
                    let clean = label.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !clean.isEmpty { names.append(clean) }
                }
                for task in effectiveTasks {
                    let raw = (task.toolSummary?.isEmpty == false ? task.toolSummary : nil)
                        ?? (task.commandLine.isEmpty ? nil : task.commandLine)
                        ?? task.toolName
                        ?? ""
                    let clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !clean.isEmpty {
                        names.append(clean.count > 18 ? String(clean.prefix(18)) + "…" : clean)
                    }
                }
                return names
            }()
            QueuedMessagesCardView(
                cascadeId: viewModel.cascadeId,
                items: viewModel.queuedMessages,
                hasActiveBackgroundWork: hasActiveBgWork,
                blockingWorkNames: blockingNames,
                onSendNow: { item in
                    if hasActiveBgWork {
                        queuedItemToSendNow = item
                        showSendQueuedNowConfirmAlert = true
                    } else {
                        Task {
                            await viewModel.sendQueuedMessageNow(item: item)
                        }
                    }
                },
                onEdit: { item in
                    viewModel.editQueuedMessage(item: item)
                    isInputFocused = true
                },
                onDelete: { item in
                    viewModel.deleteQueuedMessage(item: item)
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
        
        if showVoiceTroubleshootBubble && !voiceCall.isInCall {
            VoiceTroubleshootBubbleView(
                onRetry: {
                    withAnimation { showVoiceTroubleshootBubble = false }
                },
                onTroubleshoot: {
                    withAnimation { showVoiceTroubleshootBubble = false }
                    Task {
                        await viewModel.sendMessage(text: VoiceCallManager.troubleshootPrompt)
                    }
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }

        if voiceCall.isInCall {
            VoiceCallCardView(
                voiceCall: voiceCall,
                isAgentRunning: viewModel.isActivelyRunning,
                onStopAgentTask: {
                    handleCancel()
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }

        if audioRecorder.isRecording && audioRecorder.currentCascadeId == viewModel.draftKey {
            AudioRecordingCardView(
                recorder: audioRecorder,
                onCancel: {
                    audioRecorder.cancelRecording()
                    viewModel.saveCurrentDraft()
                },
                onDone: {
                    if let fileURL = audioRecorder.stopRecording() {
                        viewModel.addFiles(from: [fileURL])
                        viewModel.saveCurrentDraft()
                    }
                },
                onSend: {
                    if let fileURL = audioRecorder.stopRecording() {
                        viewModel.addFiles(from: [fileURL])
                        viewModel.saveCurrentDraft()
                        Task {
                            for _ in 0..<100 {
                                if viewModel.selectedFiles.allSatisfy({ $0.isUploaded }) { break }
                                try? await Task.sleep(nanoseconds: 100_000_000)
                            }
                            handleSend()
                        }
                    }
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
        
        if let interaction = viewModel.pendingInteraction {
            let bannerReserve: CGFloat = (viewModel.swarmBrief?.active == true) ? 52 : 0
            let availableInteractionHeight: CGFloat? = chatAreaHeight > 0
                ? max(180, chatAreaHeight - bannerReserve)
                : nil
            InteractionCardView(
                interaction: interaction,
                isSubmitting: viewModel.isSubmittingInteraction,
                maxAvailableHeight: availableInteractionHeight,
                onSubmit: { optionId, writeInText, target, questionResponses in
                    Task {
                        await viewModel.submitInteraction(optionId: optionId, writeInText: writeInText, target: target, questionResponses: questionResponses)
                    }
                },
                onSkip: { questionResponses in
                    Task {
                        await viewModel.skipInteraction(questionResponses: questionResponses)
                    }
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .id(interaction.id)
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }
    
    @ViewBuilder
    var errorBanner: some View {
        if let err = viewModel.errorMessage, (!viewModel.messages.isEmpty || viewModel.isNewConversation) {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundColor(.orange)
                    .font(.system(size: 14))
                Text(err)
                    .font(.system(size: 12.5, weight: .medium))
                    .foregroundColor(.primary)
                    .lineLimit(2)
                Spacer()
                Button(action: {
                    viewModel.errorMessage = nil
                }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundColor(.secondary)
                        .font(.system(size: 14))
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .padding(.horizontal, 16)
            .padding(.bottom, 6)
        }
    }
    
    func handleURLTap(_ url: URL) -> OpenURLAction.Result {
        let clean = url.absoluteString.trimmingCharacters(in: .whitespacesAndNewlines)
        let unescaped = clean.removingPercentEncoding ?? clean
        let lower = unescaped.lowercased()
        let pathLower = url.path.lowercased()
        let fileTargetURI: String = {
            if url.scheme?.lowercased() == "file",
               var comps = URLComponents(url: url, resolvingAgainstBaseURL: false) {
                comps.fragment = nil
                comps.query = nil
                if let stripped = comps.url?.absoluteString, !stripped.isEmpty {
                    return stripped
                }
            }
            return unescaped
        }()

        // 0. Native Sidecar UI links (sidecar://<id>/...)
        if url.scheme?.lowercased() == "sidecar" {
            let sidecarId = (url.host ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if ["dashboard", "automations", "sidecars", "schedule", "schedules"].contains(sidecarId.lowercased()) {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                mcpSkillsInitialTabOverride = .automations
                showMcpSkillsSheet = true
                return .handled
            }
            if !sidecarId.isEmpty, let gwURL = AppSettings.shared.gatewayURL {
                let relPath = url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                if sidecarId.lowercased() == "swarm" || relPath.lowercased().contains("swarm_board") {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    showSwarmSheet = true
                    return .handled
                }
                var target = gwURL.appendingPathComponent("gateway/sidecar-ui/\(sidecarId)")
                if !relPath.isEmpty {
                    target = target.appendingPathComponent(relPath)
                }
                if let query = url.query, !query.isEmpty, var comps = URLComponents(url: target, resolvingAgainstBaseURL: false) {
                    comps.percentEncodedQuery = query
                    if let full = comps.url { target = full }
                }
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                Task { @MainActor in
                    var finalTarget = target
                    if let ticket = try? await APIClient.shared.fetchWSTicket(baseURL: gwURL),
                       var comps = URLComponents(url: target, resolvingAgainstBaseURL: false) {
                        var items = comps.queryItems ?? []
                        items.removeAll { $0.name == "ticket" }
                        items.append(URLQueryItem(name: "ticket", value: ticket))
                        comps.queryItems = items
                        if let authedURL = comps.url {
                            finalTarget = authedURL
                        }
                    }
                    viewModel.openInAppBrowser(url: finalTarget)
                }
                return .handled
            }
        }
        
        let uriParam = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first(where: { $0.name == "uri" })?.value
        let decodedFileName: String = {
            if let param = uriParam, !param.isEmpty {
                let fn = (param as NSString).lastPathComponent.removingPercentEncoding ?? (param as NSString).lastPathComponent
                if !fn.isEmpty && fn != "raw" { return fn }
            }
            if let last = url.lastPathComponent.removingPercentEncoding, !last.isEmpty, last != "raw" {
                return last
            }
            let fn = (unescaped as NSString).lastPathComponent
            return fn.isEmpty ? String(localized: "文档详情") : fn
        }()
        
        let uriLower = (uriParam ?? "").lowercased()
        let uriPathLower = uriLower.split(separator: "#", maxSplits: 1).first
            .flatMap { $0.split(separator: "?", maxSplits: 1).first }
            .map(String.init) ?? uriLower
        let isZipFile = lower.hasSuffix(".zip") || pathLower.hasSuffix(".zip") || uriLower.hasSuffix(".zip") ||
            lower.contains(".zip?") || lower.contains(".zip#")
        if isZipFile {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if FileIconResolver.isWeChatChatArchive(decodedFileName) || FileIconResolver.isWeChatChatArchive(fileTargetURI) {
                viewModel.openWeChatArchive(uri: fileTargetURI, fileName: decodedFileName)
            } else {
                viewModel.downloadAndPreviewDocument(uri: fileTargetURI, fileName: decodedFileName, isHTML: false)
            }
            return .handled
        }

        // 1. Images, Presentations, Videos, Audio & Office documents (PNG, JPG, MP4, MOV, M4A, MP3, PPTX, PPT, KEY, DOCX, XLSX, PDF, HTML, etc.)
        let isHTML = lower.hasSuffix(".html") || lower.hasSuffix(".htm") ||
            pathLower.hasSuffix(".html") || pathLower.hasSuffix(".htm") ||
            uriLower.hasSuffix(".html") || uriLower.hasSuffix(".htm") ||
            uriPathLower.hasSuffix(".html") || uriPathLower.hasSuffix(".htm") ||
            lower.contains(".html?") || lower.contains(".htm?") ||
            lower.contains(".html#") || lower.contains(".htm#")
        let documentExtensions = [".pptx", ".ppt", ".key", ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".numbers", ".pages", ".ipynb", ".html", ".htm", ".txt", ".csv", ".json", ".log", ".r", ".rmd", ".rmarkdown"]
        let imageExtensions = [".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic", ".heif", ".bmp", ".svg", ".ico", ".tiff", ".tif"]
        let videoExtensions = [".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv", ".3gp", ".ts", ".flv"]
        let audioExtensions = [".m4a", ".mp3", ".wav", ".aac", ".caf", ".flac", ".ogg", ".oga", ".opus", ".amr", ".wma"]
        let allPreviewExtensions = documentExtensions + imageExtensions + videoExtensions + audioExtensions
        
        let isPreviewableFile = allPreviewExtensions.contains { ext in
            lower.hasSuffix(ext) || pathLower.hasSuffix(ext) || uriPathLower.hasSuffix(ext) || lower.contains(ext + "?") || lower.contains(ext + "#")
        }
        
        if isPreviewableFile {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            viewModel.downloadAndPreviewDocument(uri: fileTargetURI, fileName: decodedFileName, isHTML: isHTML)
            return .handled
        }
        
        // 2. Markdown & Plan Artifacts
        let isMarkdownArtifact = lower.hasSuffix(".md") || lower.hasSuffix(".markdown") ||
            pathLower.hasSuffix(".md") || pathLower.hasSuffix(".markdown") ||
            uriPathLower.hasSuffix(".md") || uriPathLower.hasSuffix(".markdown") ||
            lower.contains(".md#") || lower.contains(".md?") ||
            lower.contains(".markdown#") || lower.contains(".markdown?") ||
            lower.contains("implementation_plan") || lower.contains("walkthrough") ||
            lower.contains("/brain/") || lower.contains("/static/artifacts/")
        if isMarkdownArtifact {
            let docTitle: String = {
                if lower.contains("walkthrough") {
                    return "Walkthrough"
                } else if lower.contains("implementation_plan") {
                    return "Implementation Plan"
                }
                return decodedFileName
            }()
            viewModel.openMarkdownViewer(uri: fileTargetURI, title: docTitle)
            return .handled
        }
        
        // 3. GitHub Pull Request links (live status + details before opening Safari)
        if (url.scheme == "http" || url.scheme == "https"),
           GitHubPRLinkExtractor.parse(url: url) != nil {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            selectedGitHubPRItem = IdentifiableURL(url: url)
            return .handled
        }

        // 4. External Web links (Open in immersive in-app Safari)
        if url.scheme == "http" || url.scheme == "https" {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            viewModel.openInAppBrowser(url: url)
            return .handled
        }
        
        return .handled
    }
    
    @ViewBuilder
    func renderMarkdownViewer(data: MarkdownFileViewerData) -> some View {
        MarkdownViewerSheet(
            data: data,
            cascadeId: viewModel.cascadeId,
            onDismiss: {
                viewModel.closeMarkdownViewer()
            },
            onProceed: {
                viewModel.proceedFromViewer()
            },
            onRetry: {
                viewModel.openMarkdownViewer(uri: data.uri, title: data.title, forceRefresh: true)
            },
            onRefresh: {
                viewModel.openMarkdownViewer(uri: data.uri, title: data.title, forceRefresh: true)
            }
        )
    }
    
}
