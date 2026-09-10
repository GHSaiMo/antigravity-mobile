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
    
    @State private var conversationToDelete: ConversationItem?
    @State private var showDeleteConfirm = false
    @State private var conversationToRename: ConversationItem?
    @State private var renameText = ""
    @State private var showRenameAlert = false
    
    public init() {
        _ = SwipeActionAdjuster.activateOnce
    }
    
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
                    quotaResponse: $viewModel.quotaResponse,
                    onSwitch: { id in
                        try await viewModel.switchCockpitAccount(id: id)
                    },
                    onRefresh: {
                        try await viewModel.refreshCockpitQuotas()
                    }
                )
            }
            .alert("重命名会话", isPresented: $showRenameAlert) {
                TextField("输入新标题", text: $renameText)
                Button("取消", role: .cancel) {}
                Button("保存") {
                    if let item = conversationToRename {
                        Task {
                            await viewModel.renameConversation(item: item, newTitle: renameText)
                        }
                    }
                }
            }
            .alert("提示", isPresented: Binding(
                get: { viewModel.errorMessage != nil && !viewModel.conversations.isEmpty },
                set: { if !$0 { viewModel.errorMessage = nil } }
            )) {
                Button("确定") { viewModel.errorMessage = nil }
            } message: {
                if let msg = viewModel.errorMessage {
                    Text(msg)
                }
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
                conversationCard(for: item)
                    .contentShape(Rectangle())
                    .onTapGesture {
                        navigationPath.append(item)
                    }
                    .onLongPressGesture(minimumDuration: 0.45) {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        conversationToRename = item
                        renameText = item.title
                        showRenameAlert = true
                    }
                    .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button {
                            conversationToDelete = item
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                                showDeleteConfirm = true
                            }
                        } label: {
                            Image(uiImage: Self.trashActionImage)
                        }
                        .tint(.clear)
                    }

                    .confirmationDialog(
                        "确定删除此会话吗？\n此操作将永久删除会话记录且无法撤销。",
                        isPresented: Binding(
                            get: { showDeleteConfirm && conversationToDelete?.id == item.id },
                            set: { if !$0 { 
                                showDeleteConfirm = false 
                                conversationToDelete = nil
                            } }
                        ),
                        titleVisibility: .visible
                    ) {
                        Button("删除", role: .destructive) {
                            showDeleteConfirm = false
                            let target = conversationToDelete ?? item
                            conversationToDelete = nil
                            Task {
                                await viewModel.deleteConversation(item: target)
                            }
                        }
                        Button("取消", role: .cancel) {
                            showDeleteConfirm = false
                            conversationToDelete = nil
                        }
                    }
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
    
    private static let trashActionImage: UIImage = {
        let size: CGFloat = 40
        let circleDiameter: CGFloat = 38
        
        let format = UIGraphicsImageRendererFormat()
        format.opaque = false
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: size, height: size), format: format)
        
        let img = renderer.image { ctx in
            // 1. Draw solid red circle centered with 1pt inset for anti-aliasing
            let circleOffset = (size - circleDiameter) / 2
            let circleRect = CGRect(x: circleOffset, y: circleOffset, width: circleDiameter, height: circleDiameter)
            UIColor.systemRed.setFill()
            ctx.cgContext.fillEllipse(in: circleRect)
            
            // 2. Draw white trash icon centered inside the circle
            let config = UIImage.SymbolConfiguration(pointSize: 16, weight: .semibold)
            if let baseSymbol = UIImage(systemName: "trash", withConfiguration: config) {
                let symbol = baseSymbol.withTintColor(.white, renderingMode: .alwaysOriginal)
                let trashX = (size - symbol.size.width) / 2
                let trashY = (size - symbol.size.height) / 2
                symbol.draw(at: CGPoint(x: trashX, y: trashY))
            }
        }
        return img.withRenderingMode(.alwaysOriginal)
    }()
}

private enum SwipeActionAdjuster {
    static let activateOnce: Void = {
        // Center swipe action button in card's vacated area by shifting 16pt left (compensating for 16pt card margin)
        if let dynamicCls = NSClassFromString("_UISwipeActionDynamicButtonView") {
            let orig = #selector(UIView.layoutSubviews)
            let swiz = #selector(UIView.agy_swipeDynamicButtonViewLayoutSubviews)
            if let m1 = class_getInstanceMethod(dynamicCls, orig),
               let m2 = class_getInstanceMethod(UIView.self, swiz) {
                method_exchangeImplementations(m1, m2)
            }
        } else if let actionBtnCls = NSClassFromString("UISwipeActionButton") {
            let orig = #selector(UIView.layoutSubviews)
            let swiz = #selector(UIView.agy_swipeActionButtonLayoutSubviews)
            if let m1 = class_getInstanceMethod(actionBtnCls, orig),
               let m2 = class_getInstanceMethod(UIView.self, swiz) {
                method_exchangeImplementations(m1, m2)
            }
        }
    }()
}

private extension UIView {
    @objc func agy_swipeDynamicButtonViewLayoutSubviews() {
        agy_swipeDynamicButtonViewLayoutSubviews()
        let targetTransform = CGAffineTransform(translationX: -16, y: 0)
        if self.transform != targetTransform {
            self.transform = targetTransform
        }
    }
    
    @objc func agy_swipeActionButtonLayoutSubviews() {
        agy_swipeActionButtonLayoutSubviews()
        let targetTransform = CGAffineTransform(translationX: -16, y: 0)
        if self.transform != targetTransform {
            self.transform = targetTransform
        }
    }
}

