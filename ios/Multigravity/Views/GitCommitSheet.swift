import SwiftUI

/// 默认提交信息：只列文件名，让用户在此基础上改写；真正有语义的信息交给用户或 Agent。
func suggestCommitMessage(for files: [GitFileStatus]) -> String {
    guard !files.isEmpty else { return "" }
    let names = files.map { ($0.path as NSString).lastPathComponent }
    return names.count <= 3 ? "update " + names.joined(separator: ", ") : "update \(names.count) files"
}

/// 直接调用网关提交 / 推送，不再消耗一轮 Agent 对话。
public struct GitCommitSheet: View {
    @Bindable var viewModel: ChatViewModel
    /// 「让 Agent 写提交信息并提交」：保留原有行为，由 ChatView 把指令放进输入框。
    let onDelegateToAgent: () -> Void
    @Environment(\.dismiss) private var dismiss

    @State private var unchecked: Set<String> = []
    @State private var message: String = ""
    @State private var messageEdited = false
    @State private var detent: PresentationDetent = .medium

    public init(viewModel: ChatViewModel, onDelegateToAgent: @escaping () -> Void) {
        self.viewModel = viewModel
        self.onDelegateToAgent = onDelegateToAgent
    }

    public var body: some View {
        NavigationStack {
            Group {
                if let state = viewModel.gitSheet {
                    content(state)
                } else {
                    Color.clear
                }
            }
            .navigationTitle("提交改动")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium, .large], selection: $detent)
        .presentationDragIndicator(.visible)
    }

    @ViewBuilder
    private func content(_ state: GitSheetState) -> some View {
        if let result = state.result, result.committed {
            committedView(result, state: state)
        } else if state.isLoading && state.status == nil {
            VStack(spacing: 12) {
                ProgressView()
                Text("正在读取 Git 状态...").font(.system(size: 13)).foregroundColor(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let err = state.error, state.status == nil {
            VStack(spacing: 14) {
                notice(err, icon: "exclamationmark.triangle.fill", tint: .orange)
                Button("改为让 Agent 提交") { onDelegateToAgent() }
                    .buttonStyle(.glass)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let status = state.status {
            statusView(status, state: state)
        }
    }

    // MARK: - 变更列表 + 提交

    private func statusView(_ status: GitStatusResponse, state: GitSheetState) -> some View {
        let selected = status.files.map(\.path).filter { !unchecked.contains($0) }
        // 全选时传空数组 = 提交全部（含之后新出现的），部分选择时才传明确路径
        let pathsArg = selected.count == status.files.count ? [] : selected
        let canCommit = !state.isWorking && !message.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !selected.isEmpty

        return List {
            Section {
                branchHeader(status)
            }
            if status.clean {
                Section {
                    notice("工作区没有可提交的改动", icon: "checkmark.circle.fill", tint: .green)
                    if status.ahead > 0, status.upstream != nil {
                        Button {
                            viewModel.retryGitPush()
                        } label: {
                            Text(state.isWorking ? "推送中..." : "推送 \(status.ahead) 个未推送的提交")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .disabled(state.isWorking)
                    }
                    if let err = state.actionError {
                        Text(err).font(.system(size: 12)).foregroundColor(.red)
                    }
                }
            } else {
                Section("变更文件") {
                    ForEach(status.files) { file in
                        fileRow(file)
                    }
                }
                Section {
                    Button { onDelegateToAgent() } label: {
                        Text("让 Agent 提交").font(.system(size: 16, weight: .semibold)).frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.glassProminent)
                    .controlSize(.large)
                    .disabled(state.isWorking)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets(top: 4, leading: 0, bottom: 4, trailing: 0))
                }
                Section("提交信息") {
                    // 只有用户输入才走这个 Binding 的 set，程序刷新建议文案不会把它标记成「已手改」
                    TextField("提交信息", text: Binding(
                        get: { message },
                        set: { message = $0; messageEdited = true }
                    ), axis: .vertical)
                        .lineLimit(2...5)
                    if let err = state.actionError {
                        Text(err).font(.system(size: 12)).foregroundColor(.red)
                    }
                }
                Section {
                    HStack(spacing: 10) {
                        Button {
                            viewModel.commitGit(message: message, paths: pathsArg, push: false)
                        } label: {
                            Text("提交").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .disabled(!canCommit)

                        Button {
                            viewModel.commitGit(message: message, paths: pathsArg, push: true)
                        } label: {
                            Text(state.isWorking ? "处理中..." : "提交并推送").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.glass)
                        .tint(.indigo)
                        .disabled(!canCommit)
                    }
                    .controlSize(.large)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets(top: 4, leading: 0, bottom: 4, trailing: 0))
                }
            }
        }
        .onChange(of: status.files.count, initial: true) { _, count in
            // 文件多到半屏放不下时默认全屏（只升不降，避免用户手动拉到全屏后又被缩回）
            if count > 3 { detent = .large }
        }
        .onAppear { refreshSuggestion(status) }
        .onChange(of: status.files) { _, _ in refreshSuggestion(status) }
        .onChange(of: unchecked) { _, _ in refreshSuggestion(status) }
    }

    private func refreshSuggestion(_ status: GitStatusResponse) {
        guard !messageEdited else { return }
        message = suggestCommitMessage(for: status.files.filter { !unchecked.contains($0.path) })
    }

    private func branchHeader(_ status: GitStatusResponse) -> some View {
        HStack(spacing: 8) {
            Text(status.repoName)
                .font(.system(size: 13))
                .foregroundColor(.secondary)
                .lineLimit(1)
            Text(status.branch)
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(.indigo)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .background(Color.indigo.opacity(0.14))
                .clipShape(RoundedRectangle(cornerRadius: 6))
            if status.ahead > 0 {
                Text("↑\(status.ahead)").font(.system(size: 12, design: .monospaced)).foregroundColor(.secondary)
            }
            if status.behind > 0 {
                Text("↓\(status.behind)").font(.system(size: 12, design: .monospaced)).foregroundColor(.orange)
            }
            if status.upstream == nil && !status.detached {
                Text("未设置上游").font(.system(size: 11)).foregroundColor(.secondary)
            }
            Spacer()
        }
    }

    private func fileRow(_ file: GitFileStatus) -> some View {
        let isOn = !unchecked.contains(file.path)
        let (label, tint): (String, Color) = {
            switch file.status {
            case "ADDED": return ("A", .green)
            case "UNTRACKED": return ("U", .green)
            case "DELETED": return ("D", .red)
            case "RENAMED": return ("R", .blue)
            case "CONFLICT": return ("!", .red)
            default: return ("M", .orange)
            }
        }()
        let dir = (file.path as NSString).deletingLastPathComponent
        return Button {
            if isOn { unchecked.insert(file.path) } else { unchecked.remove(file.path) }
        } label: {
            HStack(spacing: 10) {
                Image(systemName: isOn ? "checkmark.circle.fill" : "circle")
                    .foregroundColor(isOn ? .accentColor : .secondary)
                Text(label)
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundColor(tint)
                    .frame(width: 16)
                VStack(alignment: .leading, spacing: 1) {
                    Text((file.path as NSString).lastPathComponent)
                        .font(.system(size: 14))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    if !dir.isEmpty {
                        Text(dir)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .truncationMode(.head)
                    }
                }
                Spacer()
            }
        }
        .buttonStyle(.plain)
    }

    // MARK: - 提交完成

    private func committedView(_ result: GitCommitResponse, state: GitSheetState) -> some View {
        let line = committedSummary(result)
        return VStack(spacing: 14) {
            notice(line, icon: "checkmark.circle.fill", tint: .green)
            if let err = result.pushError ?? state.actionError {
                notice("推送失败：\(err)", icon: "exclamationmark.triangle.fill", tint: .orange)
                Button {
                    viewModel.retryGitPush()
                } label: {
                    Text(state.isWorking ? "推送中..." : "重试推送")
                }
                .buttonStyle(.glass)
                .disabled(state.isWorking)
            }
            Button("完成") { dismiss() }
                .buttonStyle(.glass)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func committedSummary(_ result: GitCommitResponse) -> String {
        var line = "已提交"
        if let id = result.commitId, !id.isEmpty { line += " \(id)" }
        if result.pushed { line += "，已推送" }
        return line
    }

    private func notice(_ text: String, icon: String, tint: Color) -> some View {
        HStack(spacing: 8) {
            Image(systemName: icon).foregroundColor(tint)
            Text(text).font(.system(size: 14)).foregroundColor(.secondary)
        }
        .padding(.vertical, 4)
    }
}
