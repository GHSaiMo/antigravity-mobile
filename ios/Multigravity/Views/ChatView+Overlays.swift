import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    @ViewBuilder
    var floatingCards: some View {
        if !viewModel.runningTasks.isEmpty {
            RunningTasksCardView(
                items: viewModel.runningTasks,
                onStop: { task in
                    Task {
                        await viewModel.stopTask(task)
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
            QueuedMessagesCardView(
                items: viewModel.queuedMessages,
                onSendNow: { item in
                    Task {
                        await viewModel.sendQueuedMessageNow(item: item)
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
        
        if let interaction = viewModel.pendingInteraction {
            InteractionCardView(
                interaction: interaction,
                isSubmitting: viewModel.isSubmittingInteraction,
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
            return fn.isEmpty ? "文档详情" : fn
        }()
        
        // 1. Markdown & Plan Artifacts
        if lower.hasSuffix(".md") || lower.hasSuffix(".markdown") ||
           lower.contains("/brain/") || lower.contains("/static/artifacts/") ||
           lower.contains("implementation_plan") || lower.contains("walkthrough") {
            let docTitle: String = {
                if lower.contains("walkthrough") {
                    return "Walkthrough"
                } else if lower.contains("implementation_plan") {
                    return "Implementation Plan"
                }
                return decodedFileName
            }()
            viewModel.openMarkdownViewer(uri: unescaped, title: docTitle)
            return .handled
        }
        
        // 2. Images, Presentations & Office documents (PNG, JPG, WEBP, GIF, PPTX, PPT, KEY, DOCX, XLSX, PDF, HTML, etc.)
        let isHTML = lower.hasSuffix(".html") || lower.hasSuffix(".htm") || pathLower.hasSuffix(".html") || pathLower.hasSuffix(".htm")
        let documentExtensions = [".pptx", ".ppt", ".key", ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".numbers", ".pages", ".html", ".htm", ".txt", ".csv", ".json", ".log"]
        let imageExtensions = [".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic", ".heif", ".bmp", ".svg", ".ico", ".tiff", ".tif"]
        let allPreviewExtensions = documentExtensions + imageExtensions
        
        let uriLower = (uriParam ?? "").lowercased()
        let isPreviewableFile = allPreviewExtensions.contains { ext in
            lower.hasSuffix(ext) || pathLower.hasSuffix(ext) || uriLower.hasSuffix(ext) || lower.contains(ext + "?") || lower.contains(ext + "#")
        }
        
        if isPreviewableFile {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            viewModel.downloadAndPreviewDocument(uri: unescaped, fileName: decodedFileName, isHTML: isHTML)
            return .handled
        }
        
        // 3. External Web links
        if url.scheme == "http" || url.scheme == "https" {
            return .systemAction
        }
        
        return .handled
    }
    
    @ViewBuilder
    func renderMarkdownViewer(data: MarkdownFileViewerData) -> some View {
        MarkdownViewerSheet(
            data: data,
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
