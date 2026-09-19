import SwiftUI

public struct ConfirmUndoSheet: View {
    @Bindable var viewModel: ChatViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var viewingDiffFile: RevertPreviewFile? = nil
    
    public init(viewModel: ChatViewModel) {
        self.viewModel = viewModel
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 16) {
                        // Warning & Explanation Banner
                        HStack(alignment: .top, spacing: 12) {
                            Image(systemName: "arrow.uturn.backward.circle.fill")
                                .font(.system(size: 24))
                                .foregroundColor(.orange)
                            
                            VStack(alignment: .leading, spacing: 4) {
                                Text("确定要撤回到该消息吗？")
                                    .font(.system(size: 16, weight: .bold))
                                    .foregroundColor(.primary)
                                
                                Text("撤回将删除此消息及后续的所有对话历史与代码修改。该句话将被放回输入框，方便您重新编辑。")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        }
                        .padding(14)
                        .background(Color.orange.opacity(0.1))
                        .clipShape(RoundedRectangle(cornerRadius: 14))
                        
                        // Undone message preview
                        if let msg = viewModel.activeUndoMessage {
                            VStack(alignment: .leading, spacing: 6) {
                                Text("撤回的内容：")
                                    .font(.system(size: 12, weight: .semibold))
                                    .foregroundColor(.secondary)
                                
                                Text(msg.content.isEmpty ? "（图片消息）" : msg.content)
                                    .font(.system(size: 14))
                                    .foregroundColor(.primary)
                                    .padding(12)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 10))
                            }
                        }
                        
                        // Code rollback preview
                        if viewModel.isLoadingRevertPreview {
                            HStack(spacing: 10) {
                                ProgressView()
                                    .scaleEffect(0.8)
                                Text("正在计算代码变更差异...")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity, alignment: .center)
                            .padding(.vertical, 16)
                        } else if let preview = viewModel.revertPreview {
                            if preview.hasCodeChanges {
                                VStack(alignment: .leading, spacing: 10) {
                                    HStack {
                                        Text("代码回退变更")
                                            .font(.system(size: 14, weight: .bold))
                                        
                                        Spacer()
                                        
                                        Text("\(preview.files.count) 个文件")
                                            .font(.system(size: 12, weight: .medium))
                                            .foregroundColor(.secondary)
                                    }
                                    
                                    ForEach(preview.files) { file in
                                        fileChangeCard(file)
                                    }
                                }
                            } else {
                                HStack(spacing: 8) {
                                    Image(systemName: "checkmark.circle")
                                        .foregroundColor(.green)
                                    Text("无工作区代码变更（仅回退会话与指令记录）")
                                        .font(.system(size: 13))
                                        .foregroundColor(.secondary)
                                }
                                .padding(12)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color(uiColor: .secondarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 10))
                            }
                        }
                    }
                    .padding(16)
                }
                
                Divider()
                
                // Bottom Action Buttons (Native Material Capsule Style from Image 4)
                HStack(spacing: 12) {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        viewModel.showConfirmUndoSheet = false
                        viewModel.activeUndoMessage = nil
                        viewModel.revertPreview = nil
                    }) {
                        Text("取消")
                            .font(.system(size: 16, weight: .medium))
                            .foregroundColor(.primary)
                            .frame(maxWidth: .infinity)
                            .frame(height: 50)
                    }
                    .buttonStyle(NativeMaterialCapsuleButtonStyle(isDestructive: false))
                    .disabled(viewModel.isReverting)
                    
                    Button(action: {
                        viewModel.confirmUndo()
                    }) {
                        HStack(spacing: 6) {
                            if viewModel.isReverting {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .red))
                                    .scaleEffect(0.8)
                            }
                            Text(viewModel.isReverting ? "正在撤回..." : "确认撤回")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundColor(.red)
                        }
                        .frame(maxWidth: .infinity)
                        .frame(height: 50)
                    }
                    .buttonStyle(NativeMaterialCapsuleButtonStyle(isDestructive: true))
                    .disabled(viewModel.isReverting || viewModel.isLoadingRevertPreview)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 14)
                .background(.ultraThinMaterial)
            }
            .navigationTitle("撤回确认")
            .navigationBarTitleDisplayMode(.inline)
            .sheet(item: $viewingDiffFile) { file in
                DiffViewerSheet(file: file)
            }
        }
    }
    
    @ViewBuilder
    private func fileChangeCard(_ file: RevertPreviewFile) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "doc.text.fill")
                .font(.system(size: 16))
                .foregroundColor(.accentColor)
            
            VStack(alignment: .leading, spacing: 2) {
                Text(file.fileName)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                
                HStack(spacing: 6) {
                    actionBadge(file.actionType)
                    
                    if file.additions > 0 {
                        Text("+\(file.additions)")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(.green)
                    }
                    if file.deletions > 0 {
                        Text("-\(file.deletions)")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(.red)
                    }
                }
            }
            
            Spacer()
            
            Button("查看 Diff") {
                viewingDiffFile = file
            }
            .font(.system(size: 12, weight: .medium))
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(Color.blue.opacity(0.1))
            .foregroundColor(.blue)
            .clipShape(RoundedRectangle(cornerRadius: 6))
        }
        .padding(12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
    
    @ViewBuilder
    private func actionBadge(_ action: String) -> some View {
        let (color, text): (Color, String) = {
            switch action.uppercased() {
            case "CREATE":
                return (.green, "新增")
            case "DELETE":
                return (.red, "删除")
            default:
                return (.blue, "修改")
            }
        }()
        
        Text(text)
            .font(.system(size: 10, weight: .bold))
            .foregroundColor(color)
            .padding(.horizontal, 5)
            .padding(.vertical, 1.5)
            .background(color.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 3))
    }
}

// MARK: - Apple Native Material Capsule Button Style (Matching Image 4)

public struct NativeMaterialCapsuleButtonStyle: ButtonStyle {
    public var isDestructive: Bool
    
    public init(isDestructive: Bool = false) {
        self.isDestructive = isDestructive
    }
    
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(
                ZStack {
                    Capsule()
                        .fill(.regularMaterial)
                    if isDestructive {
                        Capsule()
                            .fill(Color.red.opacity(configuration.isPressed ? 0.12 : 0.06))
                    }
                }
            )
            .overlay(
                Capsule()
                    .stroke(
                        isDestructive ? Color.red.opacity(configuration.isPressed ? 0.35 : 0.22) : Color.primary.opacity(0.08),
                        lineWidth: 0.5
                    )
            )
            .shadow(
                color: isDestructive ? Color.red.opacity(configuration.isPressed ? 0.05 : 0.12) : Color.black.opacity(configuration.isPressed ? 0.02 : 0.07),
                radius: configuration.isPressed ? 3 : 8,
                x: 0,
                y: configuration.isPressed ? 1 : 2.5
            )
            .scaleEffect(configuration.isPressed ? 0.98 : 1.0)
            .opacity(configuration.isPressed ? 0.88 : 1.0)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}

