import SwiftUI

public struct ConversationListView: View {
    @State private var viewModel = ConversationListViewModel()
    @State private var showSettings = false
    @State private var showNewConversation = false
    @State private var selectedDraftProject: ProjectItem?
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Group {
                if viewModel.isLoading && viewModel.conversations.isEmpty {
                    ProgressView("正在连接 Agent...")
                } else if let err = viewModel.errorMessage, viewModel.conversations.isEmpty {
                    VStack(spacing: 12) {
                        Image(systemName: "exclamationmark.triangle")
                            .font(.system(size: 40))
                            .foregroundColor(.orange)
                        Text(err)
                            .font(.system(size: 14))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal)
                        if err.contains("Cloudflare") {
                            Button("点击完成邮箱验证") {
                                showSettings = true
                            }
                            .buttonStyle(.borderedProminent)
                        } else {
                            Button("打开设置") {
                                showSettings = true
                            }
                            .buttonStyle(.borderedProminent)
                        }
                    }
                } else {
                    List {
                        ForEach(viewModel.filteredConversations) { item in
                            NavigationLink(destination: ChatView(conversation: item, isNewConversation: item.stepCount == 0)) {
                                conversationCard(for: item)
                            }
                            .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                            .listRowSeparator(.hidden)
                        }
                    }
                    .listStyle(.plain)
                    .refreshable {
                        await viewModel.fetchConversations()
                    }
                }
            }
            .navigationTitle("Antigravity")
            .searchable(text: $viewModel.searchQuery, prompt: "搜索会话或工作区...")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: { showSettings = true }) {
                        Image(systemName: "gearshape")
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: { showNewConversation = true }) {
                        Image(systemName: "plus")
                            .font(.system(size: 16, weight: .semibold))
                    }
                }
            }
            .sheet(isPresented: $showSettings) {
                SettingsSheet()
            }
            .sheet(isPresented: $showNewConversation) {
                NewConversationSheet(onSelectProject: { project in
                    Task { @MainActor in
                        try? await Task.sleep(nanoseconds: 200_000_000)
                        selectedDraftProject = project
                    }
                })
            }
            .navigationDestination(item: $selectedDraftProject) { project in
                ChatView(draftProject: project)
            }
            .onAppear {
                Task {
                    await viewModel.fetchConversations()
                }
            }
            .task {
                await viewModel.fetchConversations()
            }
            .task {
                await ProjectCacheManager.shared.fetchAndCacheProjects()
            }
        }
    }
    
    private func conversationCard(for item: ConversationItem) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                Text(item.title)
                    .font(.system(size: 15.5, weight: .semibold))
                    .lineLimit(2)
                    .foregroundColor(.primary)
                
                Spacer()
                
                if item.status.needsAction {
                    Text(item.status.rawValue)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.blue)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.blue.opacity(0.15))
                        .cornerRadius(6)
                } else if item.status.isRunning {
                    Text(item.status.rawValue)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.green)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.green.opacity(0.15))
                        .cornerRadius(6)
                }
            }
            
            HStack {
                HStack(spacing: 4) {
                    Image(systemName: "folder")
                        .font(.system(size: 11))
                    Text(item.workspaceName)
                        .font(.system(size: 12, design: .monospaced))
                }
                .foregroundColor(.secondary)
                
                Spacer()
                
                Text("\(item.stepCount) 步骤 • \(item.relativeTimeString)")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
            }
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemBackground))
        .cornerRadius(14)
    }
}
