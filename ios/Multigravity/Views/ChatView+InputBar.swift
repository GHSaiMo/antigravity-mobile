import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    var inputBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Quick action chips at top of input box (➕ and Model Switch placed in front of Commit and Push)
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
                    
                    // 2. Gemini / Claude Model Switch Button
                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            viewModel.toggleModel()
                        }
                    }) {
                        Text(viewModel.isClaudeActive ? "Claude" : "Gemini")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(viewModel.isClaudeActive ? .orange : .blue)
                            .padding(.horizontal, 11)
                            .frame(height: 32)
                            .background(
                                viewModel.isClaudeActive ? Color.orange.opacity(0.12) : Color.blue.opacity(0.12)
                            )
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(
                                        viewModel.isClaudeActive ? Color.orange.opacity(0.35) : Color.blue.opacity(0.35),
                                        lineWidth: 1
                                    )
                            )
                    }
                    .buttonStyle(.plain)
                    
                    // 3. Commit：打开 Git 提交浮窗（直接调用网关，不再消耗一轮 Agent 对话）
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        isInputFocused = false
                        viewModel.openGitSheet()
                    } label: {
                        Text("Commit")
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
                // The chips' 1pt borders are drawn half outside their frame; without vertical
                // padding the horizontal ScrollView clips the bottom (and top) edge.
                .padding(.vertical, 2)
                .padding(.top, 6)
            }
            .animation(.easeInOut(duration: 0.2), value: viewModel.isLatestMessageError)
            .animation(.easeInOut(duration: 0.2), value: viewModel.canProceed)
            
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
            
            // Input field and send/stop button
            HStack(alignment: .bottom, spacing: 10) {
                TextField("", text: $viewModel.inputText, prompt: Text(viewModel.isActivelyRunning ? "向队列添加指令..." : "发送对 Agent 的指令..."), axis: .vertical)
                    .font(.system(size: 16))
                    .padding(.horizontal, 16)
                    .padding(.vertical, 11)
                    .frame(minHeight: 44)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                    .lineLimit(1...5)
                    .focused($isInputFocused)
                
                if viewModel.isActivelyRunning && viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
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
                    .keyboardShortcut(.return, modifiers: .command)
                    .transition(.scale.combined(with: .opacity))
                }
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            .animation(.easeInOut(duration: 0.2), value: viewModel.isActivelyRunning)
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
    
    var isSendDisabled: Bool {
        viewModel.isSending
            || viewModel.selectedFiles.contains(where: { !$0.isUploaded })
            || (viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                && viewModel.selectedImageData.isEmpty
                && viewModel.selectedFiles.isEmpty)
    }
    
}
