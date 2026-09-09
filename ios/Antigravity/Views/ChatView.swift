import SwiftUI

public struct ChatView: View {
    @State private var viewModel: ChatViewModel
    @FocusState private var isInputFocused: Bool
    @State private var hasInitiallyAligned = false
    
    public init(conversation: ConversationItem) {
        _viewModel = State(initialValue: ChatViewModel(
            cascadeId: conversation.id,
            initialTitle: conversation.title
        ))
    }
    
    public var body: some View {
        VStack(spacing: 0) {
            // Content area
            if viewModel.isLoading && viewModel.messages.isEmpty {
                VStack(spacing: 14) {
                    ProgressView()
                        .scaleEffect(1.2)
                    Text("正在同步会话历史与步骤...")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty {
                VStack(spacing: 14) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.system(size: 36))
                        .foregroundColor(.orange)
                    Text(err)
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 24)
                    Button("点击重试") {
                        Task { await viewModel.loadMessages() }
                    }
                    .buttonStyle(.borderedProminent)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                // Message stream
                ScrollViewReader { proxy in
                    ScrollView {
                        VStack(spacing: 8) {
                            // Load older messages button
                            if viewModel.hasMore {
                                Button(action: {
                                    Task { await viewModel.loadOlderMessages() }
                                }) {
                                    if viewModel.isLoadingOlder {
                                        ProgressView()
                                            .scaleEffect(0.9)
                                            .frame(maxWidth: .infinity)
                                            .padding(.vertical, 8)
                                    } else {
                                        HStack(spacing: 6) {
                                            Image(systemName: "arrow.up.circle.fill")
                                                .font(.system(size: 13))
                                            Text("查看更早的消息")
                                                .font(.system(size: 12.5, weight: .medium))
                                        }
                                        .foregroundColor(.secondary)
                                        .padding(.horizontal, 14)
                                        .padding(.vertical, 7)
                                        .background(Color(uiColor: .secondarySystemBackground).opacity(0.8))
                                        .clipShape(Capsule())
                                        .frame(maxWidth: .infinity)
                                        .padding(.vertical, 4)
                                    }
                                }
                                .buttonStyle(.plain)
                            }
                            
                            ForEach(Array(viewModel.messages.enumerated()), id: \.element.id) { index, message in
                                let isLast = (index == viewModel.messages.count - 1)
                                let isActive = isLast && (viewModel.isRunning || viewModel.isAwaitingResponse)
                                MessageBubbleView(message: message, isActiveToolBatch: isActive)
                                    .id(message.id)
                            }
                            
                            // Agent thinking & executing indicator animation (shown while awaiting before tools/response arrive)
                            if (viewModel.isAwaitingResponse || viewModel.isRunning) && viewModel.messages.last?.sender == .user {
                                AgentThinkingBubbleView()
                                    .id("THINKING_INDICATOR")
                                    .transition(.opacity.combined(with: .scale(scale: 0.95, anchor: .topLeading)))
                            }
                            
                            Color.clear
                                .frame(height: 1)
                                .id("BOTTOM_ANCHOR")
                        }
                        .padding(.vertical, 12)
                        .frame(maxWidth: .infinity)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(Color(uiColor: .systemBackground))
                    .contentShape(Rectangle())
                    .simultaneousGesture(
                        TapGesture().onEnded {
                            if isInputFocused {
                                isInputFocused = false
                                UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
                            }
                        }
                    )
                    .scrollDismissesKeyboard(.interactively)
                    .refreshable {
                        await viewModel.loadMessages()
                    }
                    .onAppear {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                            initialAlignmentIfNeeded(proxy: proxy)
                        }
                    }
                    .onChange(of: viewModel.isLoading) { _, loading in
                        if !loading {
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
                                initialAlignmentIfNeeded(proxy: proxy)
                            }
                        }
                    }
                    .onChange(of: viewModel.scrollToTurnStartTrigger) { _, _ in
                        scrollToTurnStart(proxy: proxy, animated: true)
                    }
                    .onChange(of: viewModel.messages.last?.id) { _, lastId in
                        guard lastId != nil else { return }
                        if !hasInitiallyAligned {
                            initialAlignmentIfNeeded(proxy: proxy)
                            return
                        }
                        if viewModel.messages.last?.sender == .user {
                            scrollToBottom(proxy: proxy, animated: true)
                        } else if viewModel.isAwaitingResponse || viewModel.isRunning {
                            scrollToBottom(proxy: proxy, animated: true)
                        }
                    }
                    .onChange(of: isInputFocused) { _, focused in
                        if focused {
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                                scrollToBottom(proxy: proxy, animated: true)
                            }
                        }
                    }
                }
            }
            
            // Bottom input bar
            inputBar
        }
        .navigationTitle(viewModel.initialTitle)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await viewModel.loadMessages()
        }
        .onDisappear {
            viewModel.stopPolling()
        }
    }
    
    private var inputBar: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Quick action chip at top-left of input box
            HStack {
                Button(action: insertCommitAndPush) {
                    HStack(spacing: 6) {
                        Image(systemName: "arrow.triangle.branch")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(.indigo)
                        Text("Commit and Push")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.primary)
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 10, style: .continuous)
                            .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                    )
                }
                .buttonStyle(.plain)
                
                Spacer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            
            // Input field and send/stop button
            HStack(alignment: .bottom, spacing: 10) {
                TextField("发送对 Agent 的指令...", text: $viewModel.inputText, axis: .vertical)
                    .font(.system(size: 16))
                    .padding(.horizontal, 16)
                    .padding(.vertical, 11)
                    .frame(minHeight: 44)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                    .lineLimit(1...5)
                    .focused($isInputFocused)
                
                if viewModel.isRunning || viewModel.isAwaitingResponse {
                    Button(action: handleCancel) {
                        ZStack {
                            Circle()
                                .fill(Color(uiColor: .systemGray5))
                                .frame(width: 44, height: 44)
                                .overlay(
                                    Circle()
                                        .stroke(Color.secondary.opacity(0.25), lineWidth: 0.8)
                                )
                            
                            Image(systemName: "stop.fill")
                                .font(.system(size: 15, weight: .bold))
                                .foregroundColor(.red)
                        }
                    }
                    .buttonStyle(.plain)
                    .transition(.scale.combined(with: .opacity))
                } else {
                    Button(action: handleSend) {
                        ZStack {
                            Circle()
                                .fill(isSendDisabled ? Color(uiColor: .systemGray5) : Color.indigo)
                                .frame(width: 44, height: 44)
                                .overlay(
                                    Circle()
                                        .stroke(Color.secondary.opacity(isSendDisabled ? 0.25 : 0), lineWidth: 0.8)
                                )
                            
                            Image(systemName: "arrow.up")
                                .font(.system(size: 18, weight: .semibold))
                                .foregroundColor(isSendDisabled ? .secondary : .white)
                        }
                    }
                    .disabled(isSendDisabled)
                    .buttonStyle(.plain)
                    .transition(.scale.combined(with: .opacity))
                }
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            .animation(.easeInOut(duration: 0.2), value: viewModel.isRunning || viewModel.isAwaitingResponse)
        }
        .background(Color(uiColor: .systemBackground))
        .overlay(
            Divider(), alignment: .top
        )
    }
    
    private var isSendDisabled: Bool {
        viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
    
    private func handleCancel() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            await viewModel.cancelTask()
        }
    }
    
    private func handleSend() {
        let text = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        viewModel.inputText = ""
        Task {
            await viewModel.sendMessage(text: text)
        }
    }
    
    private func insertCommitAndPush() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let toAppend = "Commit and Push"
        if viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            viewModel.inputText = toAppend
        } else {
            viewModel.inputText += "\n" + toAppend
        }
        isInputFocused = true
    }
    
    private func initialAlignmentIfNeeded(proxy: ScrollViewProxy) {
        guard !hasInitiallyAligned, !viewModel.messages.isEmpty else { return }
        hasInitiallyAligned = true
        smartScroll(proxy: proxy, animated: false)
    }
    
    private func smartScroll(proxy: ScrollViewProxy, animated: Bool = false) {
        if (viewModel.isAwaitingResponse || viewModel.isRunning) && viewModel.messages.last?.sender == .user {
            scrollToBottom(proxy: proxy, animated: animated)
        } else if viewModel.messages.last?.sender == .agent {
            scrollToTurnStart(proxy: proxy, animated: animated)
        } else {
            scrollToBottom(proxy: proxy, animated: animated)
        }
    }
    
    private func scrollToTurnStart(proxy: ScrollViewProxy, animated: Bool = true) {
        guard let targetId = viewModel.latestTurnStartMessageId else {
            scrollToBottom(proxy: proxy, animated: animated)
            return
        }
        
        if animated {
            withAnimation(.easeOut(duration: 0.28)) {
                proxy.scrollTo(targetId, anchor: .top)
            }
        } else {
            proxy.scrollTo(targetId, anchor: .top)
        }
    }
    
    private func scrollToBottom(proxy: ScrollViewProxy, animated: Bool = true) {
        let isThinkingActive = (viewModel.isAwaitingResponse || viewModel.isRunning) && viewModel.messages.last?.sender == .user
        let target = isThinkingActive ? "THINKING_INDICATOR" : "BOTTOM_ANCHOR"
        if animated {
            withAnimation(.easeOut(duration: 0.25)) {
                proxy.scrollTo(target, anchor: .bottom)
            }
        } else {
            proxy.scrollTo(target, anchor: .bottom)
        }
    }
}

// Animated thinking & executing bubble displayed while waiting for Agent response
public struct AgentThinkingBubbleView: View {
    public init() {}
    
    public var body: some View {
        HStack(alignment: .top, spacing: 10) {
            AgentAvatarView()
            
            HStack(spacing: 8) {
                Text("Agent 正在思考与执行")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                
                AgentActivityDotsView()
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 40)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 2)
    }
}
