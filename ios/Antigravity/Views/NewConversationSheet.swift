import SwiftUI

public struct NewConversationSheet: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onSelectProject: ((ProjectItem) -> Void)?
    public var onCreated: ((String, ConversationItem) -> Void)?
    
    @State private var projects: [ProjectItem]
    @State private var isLoading: Bool
    @State private var errorMessage: String? = nil
    
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
        VStack(spacing: 0) {
            // Floating grab handle hinting pull-down dismissal
            Capsule()
                .fill(Color(uiColor: .tertiaryLabel))
                .frame(width: 38, height: 5)
                .padding(.top, 10)
                .padding(.bottom, 8)
            
            // Header title
            Text("新建会话")
                .font(.system(size: 18, weight: .bold))
                .foregroundColor(.primary)
                .frame(maxWidth: .infinity)
                .padding(.bottom, 12)
            
            Divider()
            
            Group {
                if isLoading && projects.isEmpty {
                    VStack(spacing: 14) {
                        Spacer()
                        ProgressView()
                            .scaleEffect(1.1)
                        Text("正在拉取上游项目列表...")
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                        Spacer()
                    }
                } else if let err = errorMessage, projects.isEmpty {
                    VStack(spacing: 14) {
                        Spacer()
                        Image(systemName: "exclamationmark.triangle")
                            .font(.system(size: 36))
                            .foregroundColor(.orange)
                        Text(err)
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 24)
                        Button("重新拉取") {
                            Task { await refreshProjectsInBackground() }
                        }
                        .buttonStyle(.bordered)
                        Spacer()
                    }
                } else {
                    VStack(spacing: 0) {
                        // Header description
                        HStack {
                            Text("选择项目发起新会话")
                                .font(.system(size: 13, weight: .medium))
                                .foregroundColor(.secondary)
                            Spacer()
                            Text("\(projects.count) 个项目")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
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
                        
                        // Vertical project cards list
                        ScrollView(.vertical, showsIndicators: true) {
                            LazyVStack(spacing: 9) {
                                ForEach(projects) { project in
                                    projectCard(for: project)
                                }
                            }
                            .padding(.horizontal, 16)
                            .padding(.top, 4)
                            .padding(.bottom, 24)
                        }
                    }
                }
            }
        }
        .presentationDragIndicator(.hidden)
        .task {
            await refreshProjectsInBackground()
        }
    }
    
    private func projectCard(for project: ProjectItem) -> some View {
        Button {
            selectProject(for: project)
        } label: {
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
                        Text(project.name)
                            .font(.system(size: 15.5, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        
                        if project.sessionCount > 0 {
                            Text("\(project.sessionCount) 会话")
                                .font(.system(size: 10.5, weight: .semibold))
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.secondary.opacity(0.12))
                                .clipShape(Capsule())
                        }
                    }
                    
                    Text(project.path)
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
        }
        .buttonStyle(.plain)
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
