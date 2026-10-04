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
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Header description
                HStack {
                    Text("选择模式或工作区发起新会话")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.secondary)
                    Spacer()
                    if !projects.isEmpty {
                        Text("\(projects.count) 个工作区")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
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
                        
                        if isLoading && projects.isEmpty {
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
                            ForEach(projects) { project in
                                projectCard(for: project)
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
                    
                    Text("新对话 · 不关联任何工作区")
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
            .onLongPressGesture(minimumDuration: 0.5) {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                aliasInputText = project.alias ?? ""
                editingAliasProject = project
            }
    }
    
    private func saveAlias(_ text: String, for project: ProjectItem) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let newAlias: String? = trimmed.isEmpty ? nil : trimmed
        let targetPath = project.path.isEmpty ? project.uri : project.path
        
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
                errorMessage = "同步工作区备注失败: \(error.localizedDescription)"
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
                errorMessage = "请先在设置中配置有效网关地址"
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
                errorMessage = "拉取项目列表失败: \(error.localizedDescription)"
            }
            isLoading = false
        }
    }
}
