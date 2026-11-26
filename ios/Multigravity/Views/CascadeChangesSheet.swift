import SwiftUI

/// 「本会话改动」：Agent 在当前会话里累计改动了哪些文件，点开可看正向 diff。
public struct CascadeChangesSheet: View {
    private enum SheetTab: String, CaseIterable {
        case changes = "本会话改动"
        case code = "代码"
        case files = "文件"
    }

    @Bindable var viewModel: ChatViewModel
    @State private var selectedTab: SheetTab = .changes
    @State private var viewingDiffFile: RevertPreviewFile? = nil

    public init(viewModel: ChatViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                Picker("视图", selection: $selectedTab) {
                    Text("本会话改动").tag(SheetTab.changes)
                    Text("搜索代码").tag(SheetTab.code)
                    Text("搜索文件").tag(SheetTab.files)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 8)
                .padding(.bottom, 6)

                Group {
                    switch selectedTab {
                    case .changes:
                        if let state = viewModel.changesSheet {
                            content(state)
                        } else {
                            Color.clear
                        }
                    case .code:
                        WorkspaceSearchTabView(cascadeId: viewModel.cascadeId, mode: .code)
                    case .files:
                        WorkspaceSearchTabView(cascadeId: viewModel.cascadeId, mode: .files)
                    }
                }
            }
            .navigationTitle("工作区改动与搜索")
            .navigationBarTitleDisplayMode(.inline)
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
            notice(String(localized: "本会话没有产生文件改动"), icon: "checkmark.circle.fill", tint: .green)
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
            case "CREATE": return (.green, String(localized: "新增"))
            case "DELETE": return (.red, String(localized: "删除"))
            default: return (.blue, String(localized: "修改"))
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
