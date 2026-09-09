import SwiftUI

public struct NewConversationSheet: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onCreated: ((String, ConversationItem) -> Void)?
    
    @State private var projects: [ProjectItem] = []
    @State private var selectedProject: ProjectItem?
    @State private var promptText: String = ""
    @State private var searchQuery: String = ""
    @State private var isLoadingProjects = true
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    
    public init(onCreated: ((String, ConversationItem) -> Void)? = nil) {
        self.onCreated = onCreated
    }
    
    private var filteredProjects: [ProjectItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty {
            return projects
        }
        return projects.filter {
            $0.name.lowercased().contains(q) || $0.path.lowercased().contains(q)
        }
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                if isLoadingProjects {
                    VStack(spacing: 12) {
                        Spacer()
                        ProgressView()
                        Text("正在嗅探上游项目列表...")
                            .font(.system(size: 14))
                            .foregroundColor(.secondary)
                        Spacer()
                    }
                } else if let err = errorMessage, projects.isEmpty {
                    VStack(spacing: 12) {
                        Spacer()
                        Image(systemName: "exclamationmark.triangle")
                            .font(.system(size: 36))
                            .foregroundColor(.orange)
                        Text(err)
                            .font(.system(size: 14))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal)
                        Button("重新探测") {
                            Task { await loadProjects() }
                        }
                        .buttonStyle(.bordered)
                        Spacer()
                    }
                } else {
                    VStack(alignment: .leading, spacing: 14) {
                        // Section Header: Project Selection
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Text("选择目标项目")
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Text("已发现 \(projects.count) 个项目")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                            
                            // Search bar for projects
                            HStack {
                                Image(systemName: "magnifyingglass")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                                TextField("搜索项目名称或路径...", text: $searchQuery)
                                    .font(.system(size: 13.5))
                            }
                            .padding(.horizontal, 10)
                            .padding(.vertical, 7)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .cornerRadius(10)
                        }
                        .padding(.horizontal, 16)
                        .padding(.top, 8)
                        
                        // Horizontal scroll or list of project chips
                        ScrollView(.vertical, showsIndicators: true) {
                            LazyVStack(spacing: 8) {
                                ForEach(filteredProjects) { project in
                                    projectRow(project)
                                }
                            }
                            .padding(.horizontal, 16)
                            .padding(.bottom, 8)
                        }
                        .frame(maxHeight: 240)
                        
                        Divider()
                            .padding(.horizontal, 16)
                        
                        // Section: Initial Prompt Input
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Text("首条指令 / 任务目标")
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundColor(.secondary)
                                Spacer()
                                if let selected = selectedProject {
                                    Text("将在 \(selected.name) 中运行")
                                        .font(.system(size: 11, design: .monospaced))
                                        .foregroundColor(.accentColor)
                                        .lineLimit(1)
                                }
                            }
                            
                            ZStack(alignment: .topLeading) {
                                TextEditor(text: $promptText)
                                    .font(.system(size: 15))
                                    .padding(8)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .cornerRadius(12)
                                    .frame(minHeight: 110, maxHeight: 160)
                                
                                if promptText.isEmpty {
                                    Text("例如：检查当前 git 状态，并分析最新的架构重构建议...")
                                        .font(.system(size: 15))
                                        .foregroundColor(Color(uiColor: .placeholderText))
                                        .padding(.horizontal, 13)
                                        .padding(.vertical, 16)
                                        .allowsHitTesting(false)
                                }
                            }
                        }
                        .padding(.horizontal, 16)
                        
                        Spacer()
                    }
                }
            }
            .navigationTitle("新建会话")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") {
                        dismiss()
                    }
                    .disabled(isSubmitting)
                }
                
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        Task { await startNewConversation() }
                    } label: {
                        if isSubmitting {
                            ProgressView()
                        } else {
                            Text("开始执行")
                                .fontWeight(.semibold)
                        }
                    }
                    .disabled(isSubmitting || selectedProject == nil || promptText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .task {
                await loadProjects()
            }
        }
    }
    
    private func projectRow(_ project: ProjectItem) -> some View {
        let isSelected = selectedProject?.uri == project.uri
        return Button {
            selectedProject = project
        } label: {
            HStack(spacing: 12) {
                ZStack {
                    Circle()
                        .fill(isSelected ? Color.accentColor.opacity(0.2) : Color.secondary.opacity(0.12))
                        .frame(width: 36, height: 36)
                    Image(systemName: project.isWorkspace ? "briefcase.fill" : "folder.fill")
                        .font(.system(size: 16))
                        .foregroundColor(isSelected ? .accentColor : .secondary)
                }
                
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text(project.name)
                            .font(.system(size: 15, weight: isSelected ? .bold : .medium))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        
                        if project.sessionCount > 0 {
                            Text("\(project.sessionCount) 个历史会话")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.secondary.opacity(0.12))
                                .cornerRadius(4)
                        }
                    }
                    
                    Text(project.path)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                
                Spacer()
                
                if isSelected {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundColor(.accentColor)
                        .font(.system(size: 20))
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(isSelected ? Color.accentColor.opacity(0.08) : Color(uiColor: .secondarySystemBackground))
            .cornerRadius(12)
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .stroke(isSelected ? Color.accentColor : Color.clear, lineWidth: 1.5)
            )
        }
        .buttonStyle(.plain)
    }
    
    private func loadProjects() async {
        guard let url = AppSettings.shared.gatewayURL else {
            errorMessage = "请先在设置中配置有效网关地址"
            isLoadingProjects = false
            return
        }
        
        isLoadingProjects = true
        errorMessage = nil
        do {
            let fetched = try await APIClient.shared.fetchProjects(baseURL: url)
            projects = fetched
            if selectedProject == nil, let first = fetched.first {
                selectedProject = first
            }
        } catch {
            errorMessage = "嗅探项目列表失败: \(error.localizedDescription)"
        }
        isLoadingProjects = false
    }
    
    private func startNewConversation() async {
        guard let project = selectedProject else { return }
        let prompt = promptText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return }
        guard let url = AppSettings.shared.gatewayURL else { return }
        
        isSubmitting = true
        do {
            let cascadeId = try await APIClient.shared.createCascade(
                workspaceUri: project.uri,
                prompt: prompt,
                baseURL: url
            )
            
            // Construct a lightweight ConversationItem for instant UI transition
            let summary = TrajectorySummary(
                summary: prompt,
                stepCount: 1,
                lastModifiedTime: ISO8601DateFormatter().string(from: Date()),
                trajectoryId: cascadeId,
                status: "CASCADE_RUN_STATUS_RUNNING",
                workspaces: [WorkspaceItem(workspaceFolderAbsoluteUri: project.uri)],
                annotations: Annotations(title: prompt, lastUserViewTime: nil),
                trajectoryMetadata: TrajectoryMetadata(workspaceUris: [project.uri], projectId: nil, createdAt: ISO8601DateFormatter().string(from: Date()))
            )
            let convItem = ConversationItem(id: cascadeId, summary: summary)
            
            dismiss()
            onCreated?(cascadeId, convItem)
        } catch {
            errorMessage = "创建会话失败: \(error.localizedDescription)"
        }
        isSubmitting = false
    }
}
