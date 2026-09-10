import SwiftUI

public struct ConversationListView: View {
    @Environment(\.scenePhase) private var scenePhase
    @State private var viewModel = ConversationListViewModel()
    @State private var showSettings = false
    @State private var showQRScanner = false
    @State private var showNewConversation = false
    @State private var showAccountQuota = false
    @State private var selectedDraftProject: ProjectItem?
    @State private var navigationPath = NavigationPath()
    
    public init() {}
    
    public var body: some View {
        NavigationStack(path: $navigationPath) {
            mainBodyView
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
            .sheet(isPresented: $showAccountQuota) {
                AccountQuotaSheet(
                    quotaResponse: viewModel.quotaResponse,
                    isRefreshing: viewModel.isRefreshingQuota,
                    onRefresh: {
                        Task {
                            await viewModel.triggerQuotaRefresh()
                        }
                    }
                )
            }
            .navigationDestination(for: ConversationItem.self) { item in
                ChatView(conversation: item, isNewConversation: item.stepCount == 0)
                    .onAppear {
                        Task {
                            if let url = AppSettings.shared.gatewayURL {
                                await APIClient.shared.markConversationAsRead(cascadeId: item.id, baseURL: url)
                            } else {
                                CacheManager.shared.markConversationAsRead(cascadeId: item.id)
                            }
                        }
                    }
            }
            .navigationDestination(item: $selectedDraftProject) { project in
                ChatView(draftProject: project)
            }
            .onAppear {
                viewModel.reloadFromCache()
                viewModel.startAutoRefresh()
                Task {
                    await viewModel.fetchConversations(isBackgroundPoll: !viewModel.conversations.isEmpty)
                }
                Task {
                    await ProjectCacheManager.shared.fetchAndCacheProjects()
                }
            }
            .onDisappear {
                viewModel.stopAutoRefresh()
            }
            .onChange(of: scenePhase) { _, newPhase in
                handleScenePhaseChange(newPhase)
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
                handleAppDidBecomeActive()
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didEnterBackgroundNotification)) { _ in
                viewModel.stopAutoRefresh()
            }
            .onReceive(NotificationCenter.default.publisher(for: .deviceTokenRevoked)) { _ in
                handleTokenRevoked()
            }
            .onOpenURL { url in
                handleDeepLink(url)
            }
            .sheet(isPresented: $showQRScanner) {
                QRScannerView { _ in
                    Task {
                        await viewModel.fetchConversations()
                    }
                }
            }
        }
    }
    
    @ViewBuilder
    private var mainBodyView: some View {
        if !AppSettings.shared.isPaired && viewModel.conversations.isEmpty {
            OnboardingGuideView(
                onScanTapped: { showQRScanner = true },
                onManualInputTapped: { showSettings = true }
            )
        } else if viewModel.isLoading && viewModel.conversations.isEmpty {
            loadingView
        } else if let err = viewModel.errorMessage, viewModel.conversations.isEmpty {
            errorView(err)
        } else if viewModel.filteredConversations.isEmpty {
            emptyView
        } else {
            listView
        }
    }
    
    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.2)
            Text("正在连接 Agent...")
                .font(.system(size: 15))
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
    
    private func errorView(_ err: String) -> some View {
        VStack(spacing: 14) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.system(size: 44))
                .foregroundColor(.orange)
            Text("无法连接网关")
                .font(.system(size: 17, weight: .semibold))
            Text(err)
                .font(.system(size: 13))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)
            
            VStack(spacing: 10) {
                HStack(spacing: 12) {
                    if !AppSettings.shared.isPaired {
                        Button("扫码配对") {
                            showQRScanner = true
                        }
                        .buttonStyle(.borderedProminent)
                    } else {
                        Button("智能重测端点") {
                            Task {
                                await ConnectionManager.shared.probeEndpoints()
                                await viewModel.fetchConversations()
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        
                        Button("重试") {
                            Task {
                                await viewModel.fetchConversations()
                            }
                        }
                        .buttonStyle(.bordered)
                    }
                }
                
                Button("打开设置") {
                    showSettings = true
                }
                .font(.system(size: 14))
                .foregroundColor(.secondary)
                .padding(.top, 4)
            }
            .padding(.top, 6)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
    
    private var emptyView: some View {
        ContentUnavailableView {
            Label(
                viewModel.searchQuery.isEmpty ? "暂无会话" : "未找到匹配会话",
                systemImage: viewModel.searchQuery.isEmpty ? "bubble.left.and.bubble.right" : "magnifyingglass"
            )
        } description: {
            Text(viewModel.searchQuery.isEmpty ? "可点击右上角 + 开启新会话，或下拉刷新同步" : "请尝试其他关键词搜索")
        } actions: {
            if viewModel.searchQuery.isEmpty {
                Button("刷新列表") {
                    Task {
                        await viewModel.fetchConversations()
                    }
                }
                .buttonStyle(.bordered)
            }
        }
    }
    
    private var listView: some View {
        List {
            if viewModel.quotaResponse?.currentAccount?.gemini5h != nil {
                QuotaStatusBarView(account: viewModel.quotaResponse?.currentAccount) {
                    showAccountQuota = true
                }
                .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 6, trailing: 16))
                .listRowSeparator(.hidden)
                .listRowBackground(Color.clear)
            }
            
            ForEach(viewModel.filteredConversations) { item in
                NavigationLink(value: item) {
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
    
    private func handleScenePhaseChange(_ newPhase: ScenePhase) {
        if newPhase == .active {
            viewModel.startAutoRefresh()
            Task {
                await viewModel.resumeActive()
                await ProjectCacheManager.shared.fetchAndCacheProjects()
            }
        } else if newPhase == .background {
            viewModel.stopAutoRefresh()
        }
    }
    
    private func handleAppDidBecomeActive() {
        viewModel.startAutoRefresh()
        Task {
            await viewModel.resumeActive()
            await ProjectCacheManager.shared.fetchAndCacheProjects()
        }
    }
    
    private func handleTokenRevoked() {
        AppSettings.shared.unpair()
        showQRScanner = true
    }
    
    private func handleDeepLink(_ url: URL) {
        guard url.scheme == "antigravity" || url.scheme == "agy" else { return }
        
        // Handle agy://pair pairing deep link
        if url.host == "pair" {
            switch PairingService.shared.parsePairingURI(url.absoluteString) {
            case .success(let info):
                Task {
                    do {
                        _ = try await PairingService.shared.pair(with: info)
                        await viewModel.fetchConversations()
                    } catch {
                        // Pairing failed
                    }
                }
            case .failure:
                break
            }
            return
        }
        
        let cascadeId: String
        if url.host == "cascade" {
            cascadeId = url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        } else if let host = url.host, !host.isEmpty {
            cascadeId = host
        } else {
            cascadeId = url.lastPathComponent
        }
        
        guard !cascadeId.isEmpty else { return }
        
        if let existing = viewModel.conversations.first(where: { $0.id == cascadeId }) {
            navigationPath.append(existing)
        } else {
            let placeholder = ConversationItem(
                id: cascadeId,
                title: "会话",
                status: .running,
                stepCount: 0,
                workspaceName: "workspace",
                lastModified: Date(),
                isSubagent: false,
                isUnread: false
            )
            navigationPath.append(placeholder)
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
                } else if item.isUnread {
                    ZStack {
                        Circle()
                            .fill(Color.blue.opacity(0.15))
                            .frame(width: 14, height: 14)
                        Circle()
                            .fill(Color.blue)
                            .frame(width: 6, height: 6)
                    }
                    .frame(width: 16, height: 16)
                    .padding(.top, 2)
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
