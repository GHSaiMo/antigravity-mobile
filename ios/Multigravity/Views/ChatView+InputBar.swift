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
            currentViewportHeight: currentViewportHeight
        )
    }

    var inputBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Quick action chips at top of input box (➕ and Model Switch placed in front of Slash Command, Commit and Push)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    // 1. Add ➕ Button (opens the attachment panel: camera, recent photos, add file)
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
                    
                    // 2. Model Switch Button (Tap to toggle Gemini/Claude, Long press for current provider's family & thinking menu)
                    Menu {
                        let currentProvider = viewModel.activeModelProviderKind.rawValue
                        let families = ModelCatalogStore.shared.modelFamilies(for: currentProvider)
                        ForEach(families) { family in
                            if family.variants.count == 1, let single = family.variants.first {
                                Button {
                                    withAnimation(.easeInOut(duration: 0.15)) {
                                        viewModel.selectModel(single.id)
                                    }
                                } label: {
                                    if viewModel.activeModel == single.id {
                                        Label(family.name, systemImage: "checkmark")
                                    } else {
                                        Text(family.name)
                                    }
                                }
                            } else {
                                Menu(family.name) {
                                    ForEach(family.variants) { variant in
                                        Button {
                                            withAnimation(.easeInOut(duration: 0.15)) {
                                                viewModel.selectModel(variant.id)
                                            }
                                        } label: {
                                            if viewModel.activeModel == variant.id {
                                                Label(variant.label, systemImage: "checkmark")
                                            } else {
                                                Text(variant.label)
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    } label: {
                        Text(viewModel.activeModelDisplayName)
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(modelChipForegroundColor)
                            .padding(.horizontal, 11)
                            .frame(height: 32)
                            .background(modelChipBackgroundColor)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(modelChipBorderColor, lineWidth: 1)
                            )
                    } primaryAction: {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            viewModel.toggleModel()
                        }
                    }
                    .buttonStyle(.plain)

                    // 2b. 已选中的斜杠命令按钮（紧跟 Gemini 右侧，样式同高对齐，右上角带小叉）
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
            
            // Input field and send/stop button (isolated into child View so typing never invalidates ChatView.body)
            ChatInputTextAndSendRow(
                viewModel: viewModel,
                voiceCall: voiceCall,
                isInputFocused: $isInputFocused,
                isSlashPickerDismissed: $isSlashPickerDismissed,
                onSend: handleSend,
                onCancel: handleCancel,
                onStartVoiceCall: handleStartVoiceCall,
                onAudioRecord: handleAudioRecordAction
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
        switch viewModel.activeModelProviderKind {
        case .claude: return .orange
        case .orchestrator: return .purple
        case .other: return .teal
        case .gemini: return .blue
        }
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

    var body: some View {
        if GatewayCompatStore.shared.isAvailable(GatewayFeature.slash),
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
                    viewModel.selectSlashCommand(cmd)
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
    var isInputFocused: FocusState<Bool>.Binding
    @Binding var isSlashPickerDismissed: Bool
    let onSend: () -> Void
    let onCancel: () -> Void
    let onStartVoiceCall: () -> Void
    let onAudioRecord: () -> Void

    private var hasSendableDraft: Bool {
        !viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || !viewModel.selectedImageData.isEmpty
            || !viewModel.selectedFiles.isEmpty
            || viewModel.selectedSlashCommand != nil
    }

    private var isSendDisabled: Bool {
        viewModel.isSending
            || viewModel.selectedFiles.contains(where: { !$0.isUploaded })
            || !hasSendableDraft
    }

    var body: some View {
        HStack(alignment: .bottom, spacing: 10) {
            TextField("", text: $viewModel.inputText, prompt: Text(viewModel.isActivelyRunning ? "向队列添加指令..." : "发送对 Agent 的指令..."), axis: .vertical)
                .font(.system(size: 16))
                .padding(.horizontal, 16)
                .padding(.vertical, 11)
                .frame(minHeight: 44)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                .lineLimit(1...5)
                .focused(isInputFocused)
            
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
                    onStartVoiceCall()
                }
                .onLongPressGesture(minimumDuration: 0.45) {
                    UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
                    onAudioRecord()
                }
                .accessibilityLabel("实时语音对话，长按录制语音消息")
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
