import SwiftUI

/// 「本会话改动」：Agent 在当前会话里累计改动了哪些文件，点开可看正向 diff。
public struct CascadeChangesSheet: View {
    @Bindable var viewModel: ChatViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var viewingDiffFile: RevertPreviewFile? = nil

    public init(viewModel: ChatViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        NavigationStack {
            Group {
                if let state = viewModel.changesSheet {
                    content(state)
                } else {
                    Color.clear
                }
            }
            .navigationTitle("本会话改动")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") { dismiss() }
                }
            }
        }
        .sheet(item: $viewingDiffFile) { file in
            DiffViewerSheet(file: file)
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
        }
    }

    @ViewBuilder
    private func content(_ state: ChangesSheetState) -> some View {
        if state.isLoading {
            VStack(spacing: 12) {
                ProgressView()
                Text("正在汇总代码改动...")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let err = state.error {
            notice(err, icon: "exclamationmark.triangle.fill", tint: .orange)
        } else if let data = state.data, data.hasChanges {
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    HStack(spacing: 8) {
                        Text("\(data.files.count) 个文件")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                        Text("+\(data.additions)")
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundColor(.green)
                        Text("-\(data.deletions)")
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundColor(.red)
                    }
                    ForEach(data.files) { file in
                        fileCard(file)
                    }
                }
                .padding(16)
            }
        } else {
            notice("本会话没有产生文件改动", icon: "checkmark.circle.fill", tint: .green)
        }
    }

    private func notice(_ text: String, icon: String, tint: Color) -> some View {
        VStack(spacing: 10) {
            Image(systemName: icon)
                .font(.system(size: 32))
                .foregroundColor(tint)
            Text(text)
                .font(.system(size: 14))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func fileCard(_ file: RevertPreviewFile) -> some View {
        Button {
            viewingDiffFile = file
        } label: {
            HStack(spacing: 10) {
                Image(systemName: "doc.text.fill")
                    .font(.system(size: 16))
                    .foregroundColor(.accentColor)
                VStack(alignment: .leading, spacing: 3) {
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
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.secondary)
            }
            .padding(12)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10))
        }
        .buttonStyle(.plain)
    }

    private func actionBadge(_ action: String) -> some View {
        let (color, text): (Color, String) = {
            switch action.uppercased() {
            case "CREATE": return (.green, "新增")
            case "DELETE": return (.red, "删除")
            default: return (.blue, "修改")
            }
        }()
        return Text(text)
            .font(.system(size: 10, weight: .bold))
            .foregroundColor(color)
            .padding(.horizontal, 5)
            .padding(.vertical, 1.5)
            .background(color.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 3))
    }
}
