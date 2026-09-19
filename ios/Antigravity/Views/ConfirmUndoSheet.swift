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
                
                // Bottom Action Buttons
                HStack(spacing: 12) {
                    Button(action: {
                        viewModel.showConfirmUndoSheet = false
                        viewModel.activeUndoMessage = nil
                        viewModel.revertPreview = nil
                    }) {
                        Text("取消")
                            .font(.system(size: 16, weight: .medium))
                            .foregroundColor(.primary)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 14)
                            .background(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .fill(.ultraThinMaterial)
                            )
                            .overlay(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .stroke(Color.primary.opacity(0.1), lineWidth: 0.5)
                            )
                    }
                    .buttonStyle(.plain)
                    .disabled(viewModel.isReverting)
                    
                    Button(action: {
                        viewModel.confirmUndo()
                    }) {
                        HStack(spacing: 6) {
                            if viewModel.isReverting {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .scaleEffect(0.8)
                            }
                            Text(viewModel.isReverting ? "正在撤回..." : "确认撤回")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundColor(.white)
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                        .background(
                            ZStack {
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .fill(.ultraThinMaterial)
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .fill(Color.red.opacity(viewModel.isReverting ? 0.5 : 0.82))
                            }
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .stroke(Color.white.opacity(0.22), lineWidth: 0.5)
                        )
                        .shadow(color: Color.red.opacity(0.2), radius: 6, x: 0, y: 3)
                    }
                    .buttonStyle(.plain)
                    .disabled(viewModel.isReverting || viewModel.isLoadingRevertPreview)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
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
