import SwiftUI

public struct NewConversationSheet: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onSelectProject: ((ProjectItem) -> Void)?
    public var onCreated: ((String, ConversationItem) -> Void)?
    
    @State private var projects: [ProjectItem]
    @State private var isLoading: Bool
    @State private var errorMessage: String? = nil
    @State private var editingAliasProject: ProjectItem? = nil
    @State private var aliasInputText: String = ""
    @State private var showClonePage: Bool = false
    @State private var showAddWorkspacePage: Bool = false
    @State private var isCreatingWorktree: Bool = false
    
    public init(
        onSelectProject: ((ProjectItem) -> Void)? = nil,
        onCreated: ((String, ConversationItem) -> Void)? = nil
    ) {
        self.onSelectProject = onSelectProject
        self.onCreated = onCreated
        let cached = ProjectCacheManager.shared.loadProjects()
        _projects = State(initialValue: cached)
        _isLoading = State(initialValue: cached.isEmpty)
    }
    
    private var workspaceProjects: [ProjectItem] {
        projects.filter { !$0.isPureChat }
    }

    private var pureChatSessionCount: Int {
        projects.first(where: { $0.isPureChat })?.sessionCount ?? 0
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Header description
                HStack(spacing: 8) {
                    Text("选择模式或工作区发起新会话")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.85)
                    Spacer(minLength: 4)
                    if !workspaceProjects.isEmpty {
                        Text("\(workspaceProjects.count) 个工作区")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.top, 14)
                .padding(.bottom, 8)
                
                if let err = errorMessage {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundColor(.orange)
                            .font(.system(size: 13))
                        Text(err)
                            .font(.system(size: 12.5))
                            .foregroundColor(.primary)
                            .lineLimit(2)
                        Spacer()
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Color.orange.opacity(0.12))
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .padding(.horizontal, 16)
                    .padding(.bottom, 6)
                }
                
                // Vertical cards list
                ScrollView(.vertical, showsIndicators: true) {
                    LazyVStack(spacing: 9) {
                        chatCard
                        
                        if isLoading && workspaceProjects.isEmpty {
                            VStack(spacing: 12) {
                                ProgressView()
                                    .scaleEffect(1.0)
                                Text("正在拉取工作区列表...")
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 28)
                        } else {
                            ForEach(workspaceProjects) { project in
                                projectCard(for: project)
                            }
                        }

                        RemoteSSHSectionView {
                            Task {
                                await refreshProjectsInBackground()
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 4)
                    .padding(.bottom, 24)
                }
            }
            .navigationTitle("新建会话")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        showClonePage = true
                    } label: {
                        Image("file_icon_github")
                            .renderingMode(.template)
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(width: 25, height: 25)
                    }
                    .disabled(isCreatingWorktree)
                    .accessibilityLabel("克隆 GitHub 仓库")
                }

                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showAddWorkspacePage = true
                    } label: {
                        if isCreatingWorktree {
                            ProgressView()
                                .scaleEffect(0.8)
                        } else {
                            Image(systemName: "plus")
                                .font(.system(size: 16, weight: .semibold))
                        }
                    }
                    .disabled(isCreatingWorktree)
                    .accessibilityLabel("添加本地工作区")
                }
            }
            .navigationDestination(isPresented: $showClonePage) {
                CloneGitHubRepoView(
                    onProjectReady: { created in
                        handleNewProjectRegistered(created)
                    },
                    onStartConversation: { project in
                        selectProject(for: project)
                    }
                )
            }
            .navigationDestination(isPresented: $showAddWorkspacePage) {
                AddLocalWorkspaceView(
                    onProjectReady: { created in
                        handleNewProjectRegistered(created)
                    },
                    onStartConversation: { project in
                        selectProject(for: project)
                    }
                )
            }
            .presentationDragIndicator(.visible)
        }
        .presentationDragIndicator(.visible)
        .task {
            await refreshProjectsInBackground()
        }
        .alert("设置工作区备注", isPresented: Binding(
            get: { editingAliasProject != nil },
            set: { if !$0 { editingAliasProject = nil } }
        )) {
            TextField("输入中文备注（留空恢复默认）", text: $aliasInputText)
            Button("取消", role: .cancel) { editingAliasProject = nil }
            Button("保存") {
                if let project = editingAliasProject {
                    saveAlias(aliasInputText, for: project)
                }
                editingAliasProject = nil
            }
        } message: {
            Text("原文件夹：\(editingAliasProject?.name ?? "")")
        }
    }
    
    private var chatCard: some View {
        Button {
            selectProject(for: .pureChat)
        } label: {
            HStack(spacing: 14) {
                ZStack {
                    Circle()
                        .fill(Color.indigo.opacity(0.14))
                        .frame(width: 42, height: 42)
                    Image(systemName: "bubble.left.and.bubble.right.fill")
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(.indigo)
                }
                
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text("Chat")
                            .font(.system(size: 15.5, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        
                        if pureChatSessionCount > 0 {
                            Text("\(pureChatSessionCount) 会话")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(.indigo)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.indigo.opacity(0.12))
                                .clipShape(Capsule())
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                        } else {
                            Text("新对话")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(.indigo)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.indigo.opacity(0.12))
                                .clipShape(Capsule())
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                        }
                    }
                    
                    Text("不关联任何工作区")
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                
                Spacer()
                
                Image(systemName: "chevron.right")
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(Color(uiColor: .tertiaryLabel))
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 12)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color.indigo.opacity(0.2), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
    }
    
    private func projectCard(for project: ProjectItem) -> some View {
        HStack(spacing: 14) {
                ZStack {
                    Circle()
                        .fill(Color.accentColor.opacity(0.12))
                        .frame(width: 42, height: 42)
                    Image(systemName: project.isWorkspace ? "briefcase.fill" : "folder.fill")
                        .font(.system(size: 17))
                        .foregroundColor(.accentColor)
                }
                
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text(project.displayName)
                            .font(.system(size: 15.5, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                            .truncationMode(.tail)
                        
                        if project.sessionCount > 0 {
                            Text("\(project.sessionCount) 会话")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.secondary.opacity(0.12))
                                .clipShape(Capsule())
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                        }
                    }
                    
                    Text(project.hasCustomAlias ? "\(project.name) · \(project.path)" : project.path)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                
                Spacer()
                
                Image(systemName: "chevron.right")
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(Color(uiColor: .tertiaryLabel))
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 12)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .contentShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .onTapGesture {
                selectProject(for: project)
            }
            .contextMenu {
                Button {
                    startWorktreeSession(for: project)
                } label: {
                    Label("在独立 Worktree 开启会话", systemImage: "arrow.triangle.branch")
                }

                Button {
                    aliasInputText = project.alias ?? AppSettings.shared.workspaceNote(for: project.path.isEmpty ? project.uri : project.path)
                    editingAliasProject = project
                } label: {
                    Label("设置工作区备注", systemImage: "pencil")
                }
            }
    }

    private func startWorktreeSession(for project: ProjectItem) {
        guard let url = AppSettings.shared.gatewayURL, !isCreatingWorktree else { return }
        let wsPath = project.path.isEmpty ? project.uri : project.path
        let repoUri = project.uri.isEmpty ? (wsPath.hasPrefix("/") ? "file://\(wsPath)" : wsPath) : project.uri
        isCreatingWorktree = true
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { isCreatingWorktree = false }
            do {
                let resp = try await APIClient.shared.createWorktreeSession(
                    repoUri: repoUri,
                    workspacePath: wsPath,
                    startSession: true,
                    baseURL: url
                )
                if let cid = resp.cascadeId, !cid.isEmpty {
                    dismiss()
                    NotificationCenter.default.post(name: .openConversationById, object: cid)
                } else if let err = resp.error, !err.isEmpty {
                    errorMessage = err
                } else {
                    await refreshProjectsInBackground()
                }
            } catch {
                errorMessage = String(localized: "创建 Worktree 失败: \(error.localizedDescription)")
            }
        }
    }

    private func handleNewProjectRegistered(_ created: ProjectItem) {
        withAnimation(.easeInOut(duration: 0.2)) {
            projects.removeAll {
                $0.id == created.id ||
                $0.uri == created.uri ||
                (!created.path.isEmpty && $0.path == created.path)
            }
            projects.insert(created, at: 0)
        }
        ProjectCacheManager.shared.saveProjects(projects)
        Task {
            await refreshProjectsInBackground()
        }
    }
    
    private func saveAlias(_ text: String, for project: ProjectItem) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let newAlias: String? = trimmed.isEmpty ? nil : trimmed
        let targetPath = project.path.isEmpty ? project.uri : project.path
        AppSettings.shared.setWorkspaceNote(trimmed, for: targetPath)
        
        // Optimistic local update + cache
        projects = projects.map { item in
            var copy = item
            let itemPath = item.path.isEmpty ? item.uri : item.path
            if itemPath == targetPath || item.id == project.id {
                copy.alias = newAlias
            }
            return copy
        }
        ProjectCacheManager.shared.saveProjects(projects)
        
        guard let url = AppSettings.shared.gatewayURL else { return }
        Task {
            do {
                try await APIClient.shared.updateProjectAlias(path: targetPath, alias: trimmed, baseURL: url)
            } catch {
                errorMessage = String(localized: "同步工作区备注失败: \(error.localizedDescription)")
            }
        }
    }
    
    private func selectProject(for project: ProjectItem) {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        dismiss()
        onSelectProject?(project)
    }
    
    /// Asynchronously refreshes the project list in background without blocking the UI
    private func refreshProjectsInBackground() async {
        guard let url = AppSettings.shared.gatewayURL else {
            if projects.isEmpty {
                errorMessage = String(localized: "请先在设置中配置有效网关地址")
                isLoading = false
            }
            return
        }
        
        do {
            let fetched = try await APIClient.shared.fetchProjects(baseURL: url)
            if !fetched.isEmpty {
                if fetched != projects {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        self.projects = fetched
                    }
                }
                ProjectCacheManager.shared.saveProjects(fetched)
            }
            isLoading = false
            errorMessage = nil
        } catch {
            if projects.isEmpty {
                errorMessage = String(localized: "拉取项目列表失败: \(error.localizedDescription)")
            }
            isLoading = false
        }
    }
}
