import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    /// 悬浮在输入栏顶部横线上方、覆盖在会话界面底部的斜杠命令/技能选择气泡弹窗
    /// 独立为子 View 以隔离 viewModel.inputText 的 @Observable 订阅，避免每次按键重绘整个 ChatView.body 与历史消息列表
    @ViewBuilder
    var floatingSlashCommandPopup: some View {
        ChatFloatingSlashPopupHost(
            viewModel: viewModel,
            isSlashPickerDismissed: $isSlashPickerDismissed,
            currentViewportHeight: currentViewportHeight,
            onSelectWorkflow: {
                showWorkflowTemplateSheet = true
            }
        )
    }

    /// 悬浮在输入栏上方、覆盖在会话气泡底部的 @ 上下文引用选择浮窗
    @ViewBuilder
    var floatingMentionPopup: some View {
        ChatFloatingMentionPopupHost(
            viewModel: viewModel,
            currentViewportHeight: currentViewportHeight
        )
    }

    var inputBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Quick action chips at top of input box (➕ and Model Switch placed in front of Slash Command, Commit and Push)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    // 1. Add ➕ Button (opens the attachment panel: camera, recent photos, add file; long-press opens @ mentions)
                    Button {
                        isInputFocused = false
                        showAttachmentSheet = true
                    } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.indigo)
                            .frame(width: 34, height: 32)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                            )
                    }
                    .buttonStyle(.plain)
                    .simultaneousGesture(
                        LongPressGesture(minimumDuration: 0.45).onEnded { _ in
                            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                            viewModel.openMentionPicker()
                        }
                    )
                    
                    // 2. Model Switch Button (Tap to cycle Gemini/Claude/Orchestrator, Long-press + slide up/right for custom menu)
                    ModelSwitchChipButton(
                        displayName: viewModel.activeModelDisplayName,
                        foregroundColor: modelChipForegroundColor,
                        backgroundColor: modelChipBackgroundColor,
                        borderColor: modelChipBorderColor,
                        providerKind: viewModel.activeModelProviderKind,
                        activeModelID: viewModel.activeModel,
                        pickerState: modelPickerState,
                        onToggle: {
                            withAnimation(.easeInOut(duration: 0.15)) {
                                viewModel.toggleModel()
                            }
                        },
                        onSelectModel: { modelId in
                            withAnimation(.easeInOut(duration: 0.15)) {
                                viewModel.selectModel(modelId)
                            }
                        }
                    )

                    // 2b. Workflow: 单击打开满屏模板页，长按上滑快捷输入（纯文字无 logo，视觉与 Commit/Changes 对齐）
                    WorkflowChipButton(
                        pickerState: workflowPickerState,
                        onOpenSheet: {
                            isInputFocused = false
                            workflowSheetStartInEditMode = false
                            showWorkflowTemplateSheet = true
                        },
                        onSelectTemplate: { templatePrompt in
                            viewModel.inputText = templatePrompt
                            viewModel.pendingSelectFirstPlaceholder = true
                            isInputFocused = true
                        },
                        onOpenSetup: {
                            isInputFocused = false
                            workflowSheetStartInEditMode = true
                            showWorkflowTemplateSheet = true
                        },
                        onWillOpen: {
                            if modelPickerState.isPresented {
                                modelPickerState.dismiss()
                            }
                        }
                    )

                    // 2c. 已选中的斜杠命令按钮（紧跟右侧，样式同高对齐，右上角带小叉）
                    if let command = viewModel.selectedSlashCommand {
                        SlashCommandChipView(command: command) {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.15)) {
                                viewModel.clearSlashCommand()
                            }
                        }
                        .transition(.scale(scale: 0.9).combined(with: .opacity))
                    }
                    
                    // 3. Commit：打开 Git 提交浮窗（会话没有 Git 工作区或没有文件变动/PR 时不显示）
                    if viewModel.hasGitChanges {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            isInputFocused = false
                            viewModel.openGitSheet()
                        } label: {
                            Text(viewModel.gitChipLabel)
                                .font(.system(size: 13, weight: .medium))
                                .foregroundColor(.primary)
                                .padding(.horizontal, 14)
                                .frame(height: 32)
                                .background(Color(uiColor: .secondarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                                )
                        }
                        .buttonStyle(.plain)
                    }

                    // 3b. Changes：本会话累计改动的 diff（当前 Antigravity 缺少所需接口时隐藏）
                    if viewModel.hasSessionChanges {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            isInputFocused = false
                            viewModel.openChangesSheet()
                        } label: {
                            Text("Changes")
                                .font(.system(size: 13, weight: .medium))
                                .foregroundColor(.primary)
                                .padding(.horizontal, 14)
                                .frame(height: 32)
                                .background(Color(uiColor: .secondarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                                )
                        }
                        .buttonStyle(.plain)
                    }

                    // 4. Continue Button (Shown when Agent's latest message is an error)
                    if viewModel.isLatestMessageError {
                        Button(action: handleContinue) {
                            HStack(spacing: 6) {
                                Image(systemName: "play.fill")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.white)
                                Text("Continue")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.white)
                            }
                            .padding(.horizontal, 14)
                            .frame(height: 32)
                            .background(Color.blue)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .shadow(color: Color.blue.opacity(0.35), radius: 4, x: 0, y: 2)
                        }
                        .buttonStyle(.plain)
                        .transition(.scale(scale: 0.9).combined(with: .opacity))
                        .contextMenu {
                            Button {
                                insertContinue()
                            } label: {
                                Label("填入输入框", systemImage: "square.and.pencil")
                            }
                        }
                    }
                    
                    // 5. Proceed Button (Plan is opened directly via implementation_plan.md button in chat)
                    if viewModel.canProceed {
                        Button(action: handleProceed) {
                            HStack(spacing: 6) {
                                Image(systemName: "play.fill")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.white)
                                Text("Proceed")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.white)
                            }
                            .padding(.horizontal, 14)
                            .frame(height: 32)
                            .background(Color.blue)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .shadow(color: Color.blue.opacity(0.35), radius: 4, x: 0, y: 2)
                        }
                        .buttonStyle(.plain)
                        .transition(.scale(scale: 0.9).combined(with: .opacity))
                    }
                }
                .padding(.horizontal, 16)
                // The chips' 1pt borders and the slash chip's top-right badge need vertical breathing
                // room inside the ScrollView so they are never clipped and never stick to the top divider.
                .padding(.top, 10)
                .padding(.bottom, 2)
            }
            .animation(.easeInOut(duration: 0.2), value: viewModel.isLatestMessageError)
            .animation(.easeInOut(duration: 0.2), value: viewModel.canProceed)
            .animation(.easeInOut(duration: 0.15), value: viewModel.selectedSlashCommand)
            
            // Image previews strip
            if !viewModel.selectedImageData.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        ForEach(Array(viewModel.selectedImageData.enumerated()), id: \.offset) { index, data in
                            if let uiImage = UIImage(data: data) {
                                ZStack(alignment: .topTrailing) {
                                    Button(action: {
                                        isInputFocused = false
                                        let draftImages = viewModel.selectedImageData.compactMap { UIImage(data: $0) }.map { IdentifiableImage(image: $0) }
                                        previewDraftGallery = ImageGalleryData(items: draftImages, initialIndex: index)
                                    }) {
                                        Image(uiImage: uiImage)
                                            .resizable()
                                            .scaledToFill()
                                            .frame(width: 52, height: 52)
                                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                            .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                            .overlay(
                                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                                            )
                                    }
                                    .buttonStyle(.plain)
                                    
                                    Button(action: {
                                        removeImage(at: index)
                                    }) {
                                        Image(systemName: "xmark.circle.fill")
                                            .font(.system(size: 16))
                                            .foregroundColor(.white)
                                            .background(Circle().fill(Color.black.opacity(0.65)))
                                    }
                                    .buttonStyle(.plain)
                                    .offset(x: 4, y: -4)
                                }
                                .padding(.top, 4)
                                .padding(.trailing, 4)
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 2)
                }
                .transition(.opacity.combined(with: .move(edge: .bottom)))
            }
            
            // File chips strip (documents / archives / source files)
            if !viewModel.selectedFiles.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        ForEach(viewModel.selectedFiles) { file in
                            DraftFileChipView(
                                file: file,
                                onRemove: { viewModel.removeFile(file.id) },
                                onRetry: { viewModel.retryFileUpload(file.id) }
                            )
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 2)
                }
                .transition(.opacity.combined(with: .move(edge: .bottom)))
            }
            
            if narrationPlayer.isPlaying {
                NarrationPlayerBar(player: narrationPlayer)
            }

            if dictationManager.isDictating {
                DictationActiveBar {
                    dictationManager.stop()
                }
            }

            if !viewModel.suggestedResponses.isEmpty && !isInputFocused && !viewModel.isActivelyRunning && !viewModel.showMentionPicker {
                SuggestedResponsesBar(suggestions: viewModel.suggestedResponses) { chosen in
                    Task {
                        viewModel.suggestedResponses = []
                        _ = await viewModel.sendMessage(text: chosen)
                    }
                }
            }

            if !viewModel.selectedContextItems.isEmpty {
                ContextMentionsChipBar(
                    items: viewModel.selectedContextItems,
                    onRemove: { id in viewModel.removeContextItem(id: id) }
                )
            }

            // Input field and send/stop button (isolated into child View so typing never invalidates ChatView.body)
            ChatInputTextAndSendRow(
                viewModel: viewModel,
                voiceCall: voiceCall,
                dictationManager: dictationManager,
                isInputFocused: $isInputFocused,
                isSlashPickerDismissed: $isSlashPickerDismissed,
                onSend: handleSend,
                onCancel: handleCancel,
                onStartVoiceCall: handleStartVoiceCall,
                onAudioRecord: handleAudioRecordAction,
                onStartDictation: {
                    if let gwUrl = AppSettings.shared.gatewayURL {
                        dictationManager.start(
                            cascadeId: viewModel.cascadeId,
                            baseURL: gwUrl,
                            onTranscriptUpdate: { transcript in
                                let existing = viewModel.inputText
                                if existing.isEmpty || existing.hasSuffix(" ") || existing.hasSuffix("\n") {
                                    viewModel.inputText = existing + transcript
                                } else {
                                    viewModel.inputText = transcript
                                }
                            },
                            onError: { err in
                                viewModel.errorMessage = err
                            }
                        )
                    }
                }
            )
        }
        .readableChatWidth(horizontalSizeClass == .regular)
        .background(Color(uiColor: .systemBackground))
        .overlay(
            Divider(), alignment: .top
        )
        .fullScreenCover(item: $previewDraftGallery) { gallery in
            ImageViewerSheet(gallery: gallery)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
    }
    // hasSendableDraft / isSendDisabled 仅在 ChatInputTextAndSendRow 内部计算：
    // 不要在 ChatView 上重新声明并在 body 中读取，否则会把 inputText 订阅带回 ChatView.body，打字时整页重绘。

    private var modelChipForegroundColor: Color {
        viewModel.activeModelProviderKind.accentColor
    }

    private var modelChipBackgroundColor: Color {
        modelChipForegroundColor.opacity(0.12)
    }

    private var modelChipBorderColor: Color {
        modelChipForegroundColor.opacity(0.35)
    }
}

private struct ChatFloatingSlashPopupHost: View {
    @Bindable var viewModel: ChatViewModel
    @Binding var isSlashPickerDismissed: Bool
    let currentViewportHeight: CGFloat
    var onSelectWorkflow: (() -> Void)? = nil

    var body: some View {
        if GatewayCompatStore.shared.isAvailable(GatewayFeature.slash),
           !viewModel.showMentionPicker,
           !isSlashPickerDismissed,
           let slashQuery = SlashCommandFilter.query(of: viewModel.inputText) {
            let maxTotal = currentViewportHeight > 0 ? min(max(currentViewportHeight - 16, 200), 360) : 300
            let maxList = max(maxTotal - 46, 154)
            SlashCommandPickerView(
                commands: SlashCommandFilter.filter(viewModel.slashCommands, query: slashQuery),
                isLoading: viewModel.isLoadingSlashCommands,
                maxListHeight: maxList,
                onSelect: { cmd in
                    isSlashPickerDismissed = false
                    if cmd.name == "workflow" {
                        onSelectWorkflow?()
                        viewModel.selectSlashCommand(cmd)
                    } else {
                        viewModel.selectSlashCommand(cmd)
                    }
                },
                onDismiss: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.easeOut(duration: 0.18)) {
                        isSlashPickerDismissed = true
                    }
                }
            )
            .padding(.horizontal, 12)
            .padding(.bottom, 8)
            .zIndex(100)
            .transition(
                .asymmetric(
                    insertion: .opacity.combined(with: .scale(scale: 0.96, anchor: .bottom)).combined(with: .offset(y: 6)),
                    removal: .opacity.combined(with: .scale(scale: 0.96, anchor: .bottom))
                )
            )
            .onAppear { viewModel.ensureSlashCommands() }
        }
    }
}

private struct ChatInputTextAndSendRow: View {
    @Bindable var viewModel: ChatViewModel
    @ObservedObject var voiceCall: VoiceCallManager
    let dictationManager: AudioDictationManager
    @Binding var isInputFocused: Bool
    @Binding var isSlashPickerDismissed: Bool
    let onSend: () -> Void
    let onCancel: () -> Void
    let onStartVoiceCall: () -> Void
    let onAudioRecord: () -> Void
    let onStartDictation: () -> Void

    private var hasSendableDraft: Bool {
        !viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || !viewModel.selectedImageData.isEmpty
            || !viewModel.selectedFiles.isEmpty
            || viewModel.selectedSlashCommand != nil
            || !viewModel.selectedContextItems.isEmpty
    }

    private var isSendDisabled: Bool {
        viewModel.isSending
            || viewModel.selectedFiles.contains(where: { !$0.isUploaded })
            || !hasSendableDraft
    }

    var body: some View {
        HStack(alignment: .bottom, spacing: 10) {
            BracketPlaceholderTextInput(
                text: $viewModel.inputText,
                placeholder: viewModel.isActivelyRunning ? String(localized: "向队列添加指令...") : String(localized: "发送对 Agent 的指令..."),
                isFocused: $isInputFocused,
                pendingSelectFirstPlaceholder: $viewModel.pendingSelectFirstPlaceholder
            )
            .frame(minHeight: 44)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
            
            if voiceCall.isInCall && !hasSendableDraft {
                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    voiceCall.endCall()
                } label: {
                    ZStack {
                        Circle()
                            .fill(Color.red)
                            .frame(width: 44, height: 44)
                            .shadow(color: Color.red.opacity(0.28), radius: 4, x: 0, y: 2)

                        Image(systemName: "phone.down.fill")
                            .font(.system(size: 16, weight: .bold))
                            .foregroundColor(.white)
                    }
                }
                .buttonStyle(.plain)
                .accessibilityLabel("挂断实时语音通话")
                .transition(.scale.combined(with: .opacity))
            } else if hasSendableDraft || (!viewModel.isActivelyRunning && !AppSettings.shared.enableVoiceMode) {
                Button(action: onSend) {
                    ZStack {
                        Circle()
                            .fill(isSendDisabled ? Color(uiColor: .tertiarySystemFill) : Color.indigo)
                            .frame(width: 44, height: 44)
                            .overlay(
                                Circle()
                                    .stroke(Color.secondary.opacity(isSendDisabled ? 0.22 : 0), lineWidth: 0.8)
                            )
                        
                        Image(systemName: "arrow.up")
                            .font(.system(size: 18, weight: .semibold))
                            .foregroundColor(isSendDisabled ? Color(uiColor: .tertiaryLabel) : .white)
                    }
                }
                .disabled(isSendDisabled)
                .buttonStyle(.plain)
                .keyboardShortcut(.return, modifiers: .command)
                .transition(.scale.combined(with: .opacity))
            } else if viewModel.isActivelyRunning {
                Button(action: onCancel) {
                    ZStack {
                        Circle()
                            .fill(Color(uiColor: .secondarySystemBackground))
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
                // Voice Mode idle button: 1:1 desktop Antigravity parity
                ZStack {
                    Circle()
                        .fill(Color(uiColor: UIColor { trait in
                            if trait.userInterfaceStyle == .dark {
                                return UIColor(red: 71/255.0, green: 85/255.0, blue: 88/255.0, alpha: 1.0)
                            } else {
                                return UIColor(red: 67/255.0, green: 81/255.0, blue: 85/255.0, alpha: 1.0)
                            }
                        }))
                        .frame(width: 44, height: 44)
                        .overlay(
                            Circle()
                                .stroke(Color.white.opacity(0.14), lineWidth: 0.8)
                        )
                        .shadow(color: Color.black.opacity(0.16), radius: 3, x: 0, y: 1.5)

                    AntigravityVoiceShape()
                        .fill(Color.white)
                        .frame(width: 22, height: 22)
                }
                .contentShape(Circle())
                .onTapGesture {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    if dictationManager.isDictating {
                        dictationManager.stop()
                    } else {
                        onStartVoiceCall()
                    }
                }
                .onLongPressGesture(minimumDuration: 0.45) {
                    UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
                    if dictationManager.isDictating {
                        dictationManager.stop()
                    } else {
                        onStartDictation()
                    }
                }
                .accessibilityLabel("实时语音对话，长按语音听写或录制语音消息")
                .transition(.scale.combined(with: .opacity))
            }
        }
        .padding(.horizontal, 16)
        .padding(.bottom, 12)
        .animation(.easeInOut(duration: 0.2), value: viewModel.isActivelyRunning)
        .animation(.easeInOut(duration: 0.2), value: voiceCall.isInCall)
        .animation(.easeInOut(duration: 0.18), value: hasSendableDraft)
        .onChange(of: viewModel.inputText) { oldText, newText in
            if SlashCommandFilter.query(of: newText) == nil {
                isSlashPickerDismissed = false
            } else if newText.hasSuffix("/") && !oldText.hasSuffix("/") {
                isSlashPickerDismissed = false
            }
            detectAtMentionTyping(oldText: oldText, newText: newText)
        }
    }

    private func trailingAtMentionQuery(in text: String) -> String? {
        guard let regex = try? NSRegularExpression(pattern: "(?:^|\\s)@([^\\s@]*)$", options: []) else {
            return nil
        }
        let ns = text as NSString
        let range = NSRange(location: 0, length: ns.length)
        guard let match = regex.firstMatch(in: text, options: [], range: range), match.numberOfRanges > 1 else {
            return nil
        }
        return ns.substring(with: match.range(at: 1))
    }

    private func detectAtMentionTyping(oldText: String, newText: String) {
        if let query = trailingAtMentionQuery(in: newText) {
            if !viewModel.showMentionPicker {
                viewModel.openMentionPicker(query: query)
            } else {
                viewModel.scheduleMentionSearch(query: query)
            }
            return
        }
        if viewModel.showMentionPicker && (trailingAtMentionQuery(in: oldText) != nil || newText.hasSuffix(" ")) {
            viewModel.closeMentionPicker()
        }
    }
}

private struct NarrationPlayerBar: View {
    let player: NarrationAudioPlayer

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "speaker.wave.2.fill")
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(.indigo)
            Text(player.statusLabel.isEmpty ? "正在语音播报..." : player.statusLabel)
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(.primary)
            Spacer()
            Button("停止") {
                player.stop()
            }
            .font(.system(size: 11.5, weight: .semibold))
            .foregroundColor(.red)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(Color.indigo.opacity(0.1))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .padding(.horizontal, 16)
    }
}

private struct DictationActiveBar: View {
    let onStop: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(Color.red)
                .frame(width: 8, height: 8)
            Text("正在原生流式听写...")
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(.primary)
            Spacer()
            Button("完成") {
                onStop()
            }
            .font(.system(size: 11.5, weight: .semibold))
            .foregroundColor(.indigo)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(Color.red.opacity(0.1))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .padding(.horizontal, 16)
    }
}

private struct SuggestedResponsesBar: View {
    let suggestions: [String]
    let onSelect: (String) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(suggestions, id: \.self) { text in
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSelect(text)
                    } label: {
                        Text(text)
                            .font(.system(size: 12.5, weight: .medium))
                            .foregroundColor(.indigo)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 6)
                            .background(Color.indigo.opacity(0.1))
                            .clipShape(Capsule())
                            .overlay(
                                Capsule().stroke(Color.indigo.opacity(0.25), lineWidth: 0.8)
                            )
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 16)
        }
        .transition(.opacity.combined(with: .move(edge: .bottom)))
    }
}

private struct ContextMentionsChipBar: View {
    let items: [ContextMentionItem]
    let onRemove: (String) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(items) { item in
                    HStack(spacing: 4) {
                        Image(systemName: item.type == "directory" ? "folder.fill" : (item.type == "conversation" ? "bubble.left.fill" : "doc.text.fill"))
                            .font(.system(size: 10))
                            .foregroundColor(.indigo)
                        Text("@\(item.label)")
                            .font(.system(size: 11.5, weight: .medium, design: .monospaced))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        Button {
                            onRemove(item.id)
                        } label: {
                            Image(systemName: "xmark.circle.fill")
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                        }
                        .buttonStyle(.plain)
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(Color.indigo.opacity(0.1))
                    .clipShape(Capsule())
                }
            }
            .padding(.horizontal, 16)
        }
    }
}

private struct ChatFloatingMentionPopupHost: View {
    @Bindable var viewModel: ChatViewModel
    let currentViewportHeight: CGFloat

    var body: some View {
        if viewModel.showMentionPicker {
            let maxTotal: CGFloat = currentViewportHeight > 0
                ? min(max(currentViewportHeight - 12, 240), 500)
                : 380
            let maxList = max(maxTotal - 96, 160)
            MentionPickerView(
                viewModel: viewModel,
                maxListHeight: maxList
            )
            .padding(.horizontal, 10)
            .padding(.bottom, 8)
            .zIndex(101)
            .transition(
                .asymmetric(
                    insertion: .opacity.combined(with: .scale(scale: 0.96, anchor: .bottom)).combined(with: .offset(y: 8)),
                    removal: .opacity.combined(with: .scale(scale: 0.96, anchor: .bottom))
                )
            )
        }
    }
}

private struct MentionPickerView: View {
    @Bindable var viewModel: ChatViewModel
    var maxListHeight: CGFloat = 320

    @State private var contentHeight: CGFloat = 0

    private var effectiveListHeight: CGFloat {
        let clampedMax = max(maxListHeight, 160)
        let minComfortable = min(200, clampedMax)
        let fallback = CGFloat(max(viewModel.mentionCandidates.count, 1)) * 68
        let measured = contentHeight > 0 ? contentHeight : fallback
        return min(max(measured, minComfortable), clampedMax)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // 顶部标题栏 + 数量徽标 + 关闭按钮
            HStack(spacing: 8) {
                Image(systemName: "at.circle.fill")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.indigo)

                Text("引用文件或关联会话")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)

                if !viewModel.mentionCandidates.isEmpty {
                    Text("\(viewModel.mentionCandidates.count)")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.indigo)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1.5)
                        .background(Color.indigo.opacity(0.14))
                        .clipShape(Capsule())
                }

                Spacer(minLength: 8)

                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    viewModel.closeMentionPicker()
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 11.5, weight: .bold))
                        .foregroundColor(.secondary)
                        .frame(width: 26, height: 26)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Circle())
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("关闭引用选择浮窗")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            // 搜索输入框
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.secondary)
                TextField("搜索文件、目录或关联会话...", text: Binding(
                    get: { viewModel.mentionQuery },
                    set: { viewModel.scheduleMentionSearch(query: $0) }
                ))
                .font(.system(size: 14))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                if viewModel.isLoadingMentions {
                    ProgressView().scaleEffect(0.8)
                } else if !viewModel.mentionQuery.isEmpty {
                    Button {
                        viewModel.scheduleMentionSearch(query: "")
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .font(.system(size: 14))
                            .foregroundColor(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .tertiarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .padding(.horizontal, 12)
            .padding(.bottom, 10)

            Divider()
                .opacity(0.6)

            if viewModel.mentionCandidates.isEmpty {
                VStack(spacing: 10) {
                    if viewModel.isLoadingMentions {
                        ProgressView()
                            .scaleEffect(0.95)
                    } else {
                        Image(systemName: "doc.text.magnifyingglass")
                            .font(.system(size: 24))
                            .foregroundColor(.secondary.opacity(0.7))
                    }
                    Text(viewModel.isLoadingMentions ? "正在检索工作区上下文..." : "暂无匹配的文件、目录或关联会话")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity)
                .frame(height: min(200, max(maxListHeight, 160)))
            } else {
                ScrollView(.vertical, showsIndicators: true) {
                    VStack(spacing: 0) {
                        ForEach(Array(viewModel.mentionCandidates.enumerated()), id: \.element.id) { index, item in
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                viewModel.selectMentionItem(item)
                            } label: {
                                mentionRow(item)
                            }
                            .buttonStyle(.plain)

                            if index < viewModel.mentionCandidates.count - 1 {
                                Divider()
                                    .padding(.leading, 60)
                                    .opacity(0.45)
                            }
                        }
                    }
                    .onGeometryChange(for: CGFloat.self) { proxy in
                        proxy.size.height
                    } action: { newHeight in
                        if newHeight > 0 && abs(contentHeight - newHeight) > 0.5 {
                            contentHeight = newHeight
                        }
                    }
                }
                .scrollBounceBehavior(.basedOnSize)
                .frame(height: effectiveListHeight)
            }
        }
        .background(
            ZStack {
                Color(uiColor: .secondarySystemGroupedBackground)
                Rectangle().fill(.regularMaterial).opacity(0.55)
            }
        )
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 0.8)
        )
        .shadow(color: Color.black.opacity(0.32), radius: 18, x: 0, y: 6)
    }

    private func mentionRow(_ item: ContextMentionItem) -> some View {
        let style = mentionStyle(for: item.type)
        let isAlreadySelected = viewModel.selectedContextItems.contains(where: { $0.id == item.id && $0.label == item.label })

        return HStack(alignment: .top, spacing: 12) {
            ZStack {
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .fill(style.color.opacity(0.13))
                    .frame(width: 34, height: 34)
                Image(systemName: style.icon)
                    .font(.system(size: 15, weight: .medium))
                    .foregroundColor(style.color)
            }
            .padding(.top, 1)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(item.label)
                        .font(.system(size: 14.5, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)

                    Text(style.tag)
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(style.color)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1.5)
                        .background(style.color.opacity(0.12))
                        .clipShape(Capsule())
                }

                if let detail = item.detail ?? item.path, !detail.isEmpty {
                    Text(detail)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                        .truncationMode(.middle)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            Spacer(minLength: 4)

            if isAlreadySelected {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 16))
                    .foregroundColor(.indigo)
                    .padding(.top, 2)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .contentShape(Rectangle())
    }

    private func mentionStyle(for type: String) -> (icon: String, color: Color, tag: String) {
        switch type {
        case "directory":
            return ("folder.fill", .blue, "目录")
        case "conversation":
            return ("bubble.left.and.bubble.right.fill", .teal, "会话")
        default:
            return ("doc.text.fill", .indigo, "文件")
        }
    }
}

/// 1:1 对齐 Antigravity 桌面端（main.js tuc() 组件）的 3 波形条 + 4 芒星矢量图标
struct AntigravityVoiceShape: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        let w = rect.width
        let h = rect.height
        let ox = rect.minX
        let oy = rect.minY
        
        // Bar 1: x: 167.8, y: -575.2, width: 82, height: 350, rx: 41
        let bar1 = CGRect(x: ox + w * 0.21021, y: oy + h * 0.35083, width: w * 0.08542, height: h * 0.36458)
        let r1 = w * 0.04271
        path.addRoundedRect(in: bar1, cornerSize: CGSize(width: r1, height: r1))
        
        // Bar 2: x: 359.8, y: -662.7, width: 82, height: 525, rx: 41
        let bar2 = CGRect(x: ox + w * 0.41021, y: oy + h * 0.25969, width: w * 0.08542, height: h * 0.54688)
        let r2 = w * 0.04271
        path.addRoundedRect(in: bar2, cornerSize: CGSize(width: r2, height: r2))
        
        // Bar 3: x: 551.8, y: -512.7, width: 82, height: 225, rx: 41
        let bar3 = CGRect(x: ox + w * 0.61021, y: oy + h * 0.41594, width: w * 0.08542, height: h * 0.23438)
        let r3 = w * 0.04271
        path.addRoundedRect(in: bar3, cornerSize: CGSize(width: r3, height: r3))
        
        // Sparkle 4-point star (upper-right)
        path.move(to: CGPoint(x: ox + w * 0.78958, y: oy + h * 0.37400))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.78158, y: oy + h * 0.36800), control: CGPoint(x: ox + w * 0.78358, y: oy + h * 0.37400))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.72416, y: oy + h * 0.26750), control: CGPoint(x: ox + w * 0.76635, y: oy + h * 0.30969))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.62366, y: oy + h * 0.21008), control: CGPoint(x: ox + w * 0.68197, y: oy + h * 0.22531))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.61766, y: oy + h * 0.20208), control: CGPoint(x: ox + w * 0.61766, y: oy + h * 0.20808))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.62366, y: oy + h * 0.19408), control: CGPoint(x: ox + w * 0.61766, y: oy + h * 0.19508))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.72416, y: oy + h * 0.13666), control: CGPoint(x: ox + w * 0.68197, y: oy + h * 0.17885))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.78158, y: oy + h * 0.03616), control: CGPoint(x: ox + w * 0.76635, y: oy + h * 0.09447))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.78958, y: oy + h * 0.03016), control: CGPoint(x: ox + w * 0.78358, y: oy + h * 0.03016))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.79758, y: oy + h * 0.03616), control: CGPoint(x: ox + w * 0.79658, y: oy + h * 0.03016))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.85538, y: oy + h * 0.13669), control: CGPoint(x: ox + w * 0.81343, y: oy + h * 0.09447))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.95550, y: oy + h * 0.19409), control: CGPoint(x: ox + w * 0.89733, y: oy + h * 0.17891))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.96150, y: oy + h * 0.20209), control: CGPoint(x: ox + w * 0.96150, y: oy + h * 0.19509))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.95550, y: oy + h * 0.21009), control: CGPoint(x: ox + w * 0.96150, y: oy + h * 0.20809))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.85500, y: oy + h * 0.26751), control: CGPoint(x: ox + w * 0.89719, y: oy + h * 0.22532))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.79758, y: oy + h * 0.36801), control: CGPoint(x: ox + w * 0.81281, y: oy + h * 0.30970))
        path.addQuadCurve(to: CGPoint(x: ox + w * 0.78958, y: oy + h * 0.37401), control: CGPoint(x: ox + w * 0.79658, y: oy + h * 0.37401))
        path.closeSubpath()
        
        return path
    }
}

private struct BracketPlaceholderTextInput: View {
    @Binding var text: String
    let placeholder: String
    @Binding var isFocused: Bool
    @Binding var pendingSelectFirstPlaceholder: Bool

    var body: some View {
        ZStack(alignment: .topLeading) {
            BracketPlaceholderUITextView(
                text: $text,
                isFocused: $isFocused,
                pendingSelectFirstPlaceholder: $pendingSelectFirstPlaceholder
            )
            if text.isEmpty {
                Text(placeholder)
                    .font(.system(size: 16))
                    .foregroundColor(Color(uiColor: .placeholderText))
                    .padding(.horizontal, 16)
                    .padding(.vertical, 12)
                    .allowsHitTesting(false)
            }
        }
    }
}

private struct BracketPlaceholderUITextView: UIViewRepresentable {
    @Binding var text: String
    @Binding var isFocused: Bool
    @Binding var pendingSelectFirstPlaceholder: Bool

    private static let placeholderRegex = try? NSRegularExpression(pattern: "\\[[^\\[\\]\\n]+\\]")

    static func bracketPlaceholderRanges(in text: String) -> [NSRange] {
        guard let regex = placeholderRegex, !text.isEmpty else { return [] }
        let fullRange = NSRange(location: 0, length: (text as NSString).length)
        return regex.matches(in: text, options: [], range: fullRange).map(\.range)
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(parent: self)
    }

    func makeUIView(context: Context) -> UITextView {
        let textView = UITextView()
        textView.delegate = context.coordinator
        textView.font = .systemFont(ofSize: 16)
        textView.textColor = .label
        textView.backgroundColor = .clear
        textView.textContainerInset = UIEdgeInsets(top: 12, left: 16, bottom: 12, right: 16)
        textView.textContainer.lineFragmentPadding = 0
        textView.isScrollEnabled = false
        textView.showsVerticalScrollIndicator = true
        textView.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let tapGesture = UITapGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.handleTap(_:)))
        tapGesture.numberOfTapsRequired = 1
        tapGesture.cancelsTouchesInView = false
        tapGesture.delegate = context.coordinator
        textView.addGestureRecognizer(tapGesture)
        return textView
    }

    func updateUIView(_ uiView: UITextView, context: Context) {
        context.coordinator.parent = self

        if uiView.markedTextRange == nil && uiView.text != text {
            uiView.text = text
            uiView.invalidateIntrinsicContentSize()
        }

        let parentFocused = isFocused
        let focusChangedFromParent = (parentFocused != context.coordinator.lastParentFocused)
        context.coordinator.lastParentFocused = parentFocused

        if pendingSelectFirstPlaceholder {
            let currentText = uiView.text ?? text
            let firstRange = Self.bracketPlaceholderRanges(in: currentText).first
            DispatchQueue.main.async {
                self.pendingSelectFirstPlaceholder = false
            }
            if let firstRange {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                    if !uiView.isFirstResponder {
                        uiView.becomeFirstResponder()
                    }
                    uiView.selectedRange = firstRange
                    uiView.scrollRangeToVisible(firstRange)
                }
                return
            }
        }

        if focusChangedFromParent {
            if parentFocused && !uiView.isFirstResponder {
                DispatchQueue.main.async {
                    if self.isFocused && !uiView.isFirstResponder {
                        uiView.becomeFirstResponder()
                    }
                }
            } else if !parentFocused && uiView.isFirstResponder {
                DispatchQueue.main.async {
                    if !self.isFocused && uiView.isFirstResponder {
                        uiView.resignFirstResponder()
                    }
                }
            }
        }
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView: UITextView, context: Context) -> CGSize? {
        let targetWidth = proposal.width ?? uiView.bounds.width
        guard targetWidth > 0 else { return nil }
        let lineHeight = uiView.font?.lineHeight ?? 20
        let verticalInset = uiView.textContainerInset.top + uiView.textContainerInset.bottom
        let minHeight = max(44, ceil(lineHeight) + verticalInset)
        let maxHeight = ceil(lineHeight * 5) + verticalInset
        let fittingSize = uiView.sizeThatFits(CGSize(width: targetWidth, height: .greatestFiniteMagnitude))
        let clampedHeight = min(max(minHeight, ceil(fittingSize.height)), maxHeight)
        let needsScroll = ceil(fittingSize.height) > maxHeight
        if uiView.isScrollEnabled != needsScroll {
            uiView.isScrollEnabled = needsScroll
        }
        return CGSize(width: targetWidth, height: clampedHeight)
    }

    final class Coordinator: NSObject, UITextViewDelegate, UIGestureRecognizerDelegate {
        var parent: BracketPlaceholderUITextView
        var lastParentFocused: Bool = false

        init(parent: BracketPlaceholderUITextView) {
            self.parent = parent
        }

        func textViewDidChange(_ textView: UITextView) {
            let newText = textView.text ?? ""
            if parent.text != newText {
                parent.text = newText
            }
            textView.invalidateIntrinsicContentSize()
        }

        func textViewDidBeginEditing(_ textView: UITextView) {
            DispatchQueue.main.async {
                self.lastParentFocused = true
                if !self.parent.isFocused {
                    self.parent.isFocused = true
                }
            }
        }

        func textViewDidEndEditing(_ textView: UITextView) {
            DispatchQueue.main.async {
                self.lastParentFocused = false
                if self.parent.isFocused {
                    self.parent.isFocused = false
                }
            }
        }

        func gestureRecognizer(
            _ gestureRecognizer: UIGestureRecognizer,
            shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer
        ) -> Bool {
            true
        }

        @objc func handleTap(_ recognizer: UITapGestureRecognizer) {
            guard let textView = recognizer.view as? UITextView,
                  let text = textView.text, !text.isEmpty else { return }
            let ranges = BracketPlaceholderUITextView.bracketPlaceholderRanges(in: text)
            guard !ranges.isEmpty else { return }

            var location = recognizer.location(in: textView)
            location.x -= textView.textContainerInset.left
            location.y -= textView.textContainerInset.top

            let layoutManager = textView.layoutManager
            let textContainer = textView.textContainer
            var fraction: CGFloat = 0
            let charIndex = layoutManager.characterIndex(
                for: location,
                in: textContainer,
                fractionOfDistanceBetweenInsertionPoints: &fraction
            )

            let matchedRange = ranges.first { range in
                if NSLocationInRange(charIndex, range) {
                    return true
                }
                if charIndex > 0 && fraction < 0.2 && NSLocationInRange(charIndex - 1, range) {
                    return true
                }
                return false
            }
            guard let targetRange = matchedRange else { return }

            let glyphRange = layoutManager.glyphRange(forCharacterRange: targetRange, actualCharacterRange: nil)
            var boundingRect = layoutManager.boundingRect(forGlyphRange: glyphRange, in: textContainer)
            boundingRect = boundingRect.insetBy(dx: -6, dy: -6)
            guard boundingRect.contains(location) else { return }

            DispatchQueue.main.async {
                if !textView.isFirstResponder {
                    textView.becomeFirstResponder()
                }
                textView.selectedRange = targetRange
            }
        }
    }
}
