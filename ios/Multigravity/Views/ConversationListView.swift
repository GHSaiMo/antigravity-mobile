import SwiftUI

public struct ConversationListView: View {
    @Environment(\.scenePhase) private var scenePhase
    @State private var settings = AppSettings.shared
    @State private var viewModel = ConversationListViewModel()
    @State private var showSettings = false
    @State private var showQRScanner = false
    @State private var showManualInput = false
    @State private var showNewConversation = false
    @State private var pendingCreatedConversation: ConversationItem? = nil
    @State private var showAccountQuota = false
    @State private var navigationPath: [ConversationItem] = []
    
    @State private var conversationToDelete: ConversationItem?
    @State private var showDeleteConfirm = false
    @State private var conversationToRename: ConversationItem?
    @State private var renameText = ""
    @State private var showRenameAlert = false
    @State private var draftsVersion: Int = 0
    @State private var pendingPairing: PairingInfo?
    @State private var showPairingConfirm = false
    @State private var pairingErrorMessage: String?
    @State private var isPairingInProgress = false
    
    @State private var easterEggTapCount: Int = 0
    @State private var lastEasterEggTapTime: Date = .distantPast
    @State private var showEasterEgg = false
    
    public init() {
        _ = SwipeActionAdjuster.activateOnce
    }
    
    private func handleEasterEggTap() {
        guard !showEasterEgg else { return }
        let now = Date()
        if now.timeIntervalSince(lastEasterEggTapTime) > 2.0 {
            easterEggTapCount = 1
        } else {
            easterEggTapCount += 1
        }
        lastEasterEggTapTime = now
        
        let impact = UIImpactFeedbackGenerator(style: .light)
        impact.impactOccurred()
        
        if easterEggTapCount >= 10 {
            easterEggTapCount = 0
            let notification = UINotificationFeedbackGenerator()
            notification.notificationOccurred(.success)
            withAnimation(.spring(response: 0.4, dampingFraction: 0.8)) {
                showEasterEgg = true
            }
        }
    }
    
    public var body: some View {
        ZStack {
            if !settings.isPaired && !isPairingInProgress {
                OnboardingGuideView(
                    onScanTapped: { showQRScanner = true },
                    onManualInputTapped: { showManualInput = true },
                    onEasterEggTap: handleEasterEggTap
                )
            } else {
                NavigationStack(path: $navigationPath) {
                    pairedContentView
                        .navigationTitle("Multigravity")
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
                        .background(NavigationBarTapHelper(onTap: handleEasterEggTap))
                        .navigationDestination(for: ConversationItem.self) { item in
                            ChatView(conversation: item, isNewConversation: item.isDraft || item.stepCount == 0)
                                .id(item.id)
                                .onAppear {
                                    guard !item.isDraft else { return }
                                    if let url = AppSettings.shared.gatewayURL {
                                        APIClient.shared.notifySessionFocus(cascadeId: item.id, baseURL: url)
                                    }
                                    Task {
                                        if let url = AppSettings.shared.gatewayURL {
                                            await APIClient.shared.markConversationAsRead(cascadeId: item.id, baseURL: url)
                                        } else {
                                            CacheManager.shared.markConversationAsRead(cascadeId: item.id)
                                        }
                                    }
                                }
                                .onDisappear {
                                    draftsVersion += 1
                                    viewModel.reloadFromCache()
                                }
                        }
                }
            }
            
            if showEasterEgg {
                EasterEggModalView(isPresented: $showEasterEgg)
                    .zIndex(999)
            }
        }
        .sheet(isPresented: $showManualInput) {
            ManualPairingSheet { info in
                startPairing(info: info)
            }
        }
        .sheet(isPresented: $showQRScanner) {
            QRScannerView { info in
                startPairing(info: info)
            }
        }
        .sheet(isPresented: $showSettings) {
            SettingsSheet()
                .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: $showNewConversation, onDismiss: {
            if let item = pendingCreatedConversation {
                pendingCreatedConversation = nil
                navigationPath.append(item)
            }
        }) {
            NewConversationSheet(onSelectProject: { project in
                let session = CacheManager.shared.createLocalDraftSession(project: project)
                pendingCreatedConversation = session.toConversationItem()
                showNewConversation = false
            })
            .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: $showAccountQuota) {
            AccountQuotaSheet(
                quotaResponse: $viewModel.quotaResponse,
                onSwitch: { id in
                    try await viewModel.switchCockpitAccount(id: id)
                },
                onRefresh: {
                    try await viewModel.refreshCockpitQuotas()
                },
                onAppearFetch: {
                    await viewModel.fetchQuotas(force: true)
                },
                enableSwitchButton: true
            )
            .presentationDragIndicator(.visible)
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
        .confirmationDialog(
            "确认配对网关",
            isPresented: $showPairingConfirm,
            titleVisibility: .visible
        ) {
            Button("配对") {
                Task { await confirmPendingPairing() }
            }
            Button("取消", role: .cancel) {
                pendingPairing = nil
            }
        } message: {
            Text(pendingPairingConfirmText)
        }
        .alert("配对失败", isPresented: Binding(
            get: { pairingErrorMessage != nil },
            set: { if !$0 { pairingErrorMessage = nil } }
        )) {
            Button("确定") { pairingErrorMessage = nil }
        } message: {
            if let msg = pairingErrorMessage {
                Text(msg)
            }
        }
        .onAppear {
            viewModel.reloadFromCache()
            if settings.isPaired {
                viewModel.startAutoRefresh()
                Task {
                    await viewModel.fetchConversations(isBackgroundPoll: !viewModel.conversations.isEmpty)
                }
                Task {
                    await ProjectCacheManager.shared.fetchAndCacheProjects()
                }
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
        .onReceive(NotificationCenter.default.publisher(for: .conversationDraftChanged)) { _ in
            draftsVersion += 1
        }
        .onReceive(NotificationCenter.default.publisher(for: .conversationDraftDeleted)) { notif in
            draftsVersion += 1
            if let draftId = notif.object as? String {
                viewModel.conversations.removeAll(where: { $0.id == draftId })
            }
            viewModel.reloadFromCache()
        }
        .onOpenURL { url in
            handleDeepLink(url)
        }
    }
    
    @ViewBuilder
    private var pairedContentView: some View {
        if isPairingInProgress || (viewModel.isLoading && viewModel.conversations.isEmpty) {
            refreshingSkeletonView
        } else if let err = viewModel.errorMessage, viewModel.conversations.isEmpty {
            errorView(err)
        } else if viewModel.filteredConversations.isEmpty {
            emptyView
        } else {
            listView
        }
    }
    
    private var refreshingSkeletonView: some View {
        List {
            // Status bar showing refreshing indicator
            HStack(spacing: 10) {
                ProgressView()
                    .scaleEffect(0.9)
                Text(isPairingInProgress ? "正在连接网关并同步会话列表..." : "正在同步会话列表...")
                    .font(.system(size: 13.5, weight: .medium))
                    .foregroundColor(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .center)
            .padding(.vertical, 8)
            .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 4, trailing: 16))
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
            
            // Skeleton placeholder cards
            ForEach(0..<4, id: \.self) { idx in
                skeletonConversationCard(opacity: 1.0 - Double(idx) * 0.18)
                    .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
            }
        }
        .listStyle(.plain)
    }
    
    private func skeletonConversationCard(opacity: Double) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                RoundedRectangle(cornerRadius: 6)
                    .fill(Color.primary.opacity(0.08 * opacity))
                    .frame(width: 140, height: 16)
                Spacer()
                RoundedRectangle(cornerRadius: 6)
                    .fill(Color.primary.opacity(0.06 * opacity))
                    .frame(width: 48, height: 16)
            }
            HStack {
                RoundedRectangle(cornerRadius: 4)
                    .fill(Color.primary.opacity(0.06 * opacity))
                    .frame(width: 80, height: 12)
                Spacer()
                RoundedRectangle(cornerRadius: 4)
                    .fill(Color.primary.opacity(0.05 * opacity))
                    .frame(width: 100, height: 12)
            }
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemBackground).opacity(opacity))
        .cornerRadius(14)
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
                        guard !item.isDraft else { return }
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
                        (conversationToDelete?.isDraft == true)
                            ? "确定删除此草稿会话吗？\n此操作将删除本地临时会话且无法撤销。"
                            : "确定删除此会话吗？\n此操作将永久删除会话记录且无法撤销。",
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
            if AppSettings.shared.isPaired {
                viewModel.startAutoRefresh()
                Task {
                    await viewModel.resumeActive()
                    await ProjectCacheManager.shared.fetchAndCacheProjects()
                }
            }
        } else if newPhase == .background {
            viewModel.stopAutoRefresh()
        }
    }
    
    private func handleAppDidBecomeActive() {
        if AppSettings.shared.isPaired {
            viewModel.startAutoRefresh()
            Task {
                await viewModel.resumeActive()
                await ProjectCacheManager.shared.fetchAndCacheProjects()
            }
        }
    }
    
    private func handleTokenRevoked() {
        isPairingInProgress = false
        settings.unpair()
        viewModel.stopAutoRefresh()
        viewModel.conversations.removeAll()
        viewModel.errorMessage = nil
        viewModel.isLoading = false
        navigationPath = []
        showSettings = false
        CacheManager.shared.clearCache()
        DocumentCacheManager.shared.clearCache()
    }
    
    private var pendingPairingConfirmText: String {
        guard let info = pendingPairing else {
            return "请确认这是你自己的电脑网关，不要配对来历不明的链接。"
        }
        let scheme = info.ssl ? "HTTPS" : "HTTP"
        let gatewayName = info.gatewayDisplayName
        return "目标 \(info.host):\(info.port)（\(scheme)）。请确认这是你自己的 \(gatewayName)，不要配对来历不明的链接。"
    }
    
    @MainActor
    private func startPairing(info: PairingInfo) {
        showQRScanner = false
        pendingPairing = nil
        viewModel.errorMessage = nil
        isPairingInProgress = true
        
        Task {
            do {
                _ = try await PairingService.shared.pair(with: info)
                settings.refreshPairedState()
                viewModel.startAutoRefresh()
                await viewModel.fetchConversations()
                await ProjectCacheManager.shared.fetchAndCacheProjects()
                isPairingInProgress = false
            } catch {
                isPairingInProgress = false
                settings.unpair()
                pairingErrorMessage = error.localizedDescription
            }
        }
    }
    
    @MainActor
    private func confirmPendingPairing() async {
        guard let info = pendingPairing else { return }
        startPairing(info: info)
    }
    
    private func handleDeepLink(_ url: URL) {
        guard url.scheme == "antigravity" || url.scheme == "agy" else { return }
        
        // Handle agy://pair pairing deep link
        if url.host == "pair" {
            switch PairingService.shared.parsePairingURI(url.absoluteString) {
            case .success(let info):
                pendingPairing = info
                showPairingConfirm = true
            case .failure(let err):
                pairingErrorMessage = err.localizedDescription
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
        
        // Dismiss modal sheets that could cover the target conversation view
        showSettings = false
        showNewConversation = false
        showAccountQuota = false
        showQRScanner = false
        showEasterEgg = false
        
        // If already looking at this exact conversation, avoid duplicate pushing
        if navigationPath.last?.id == cascadeId {
            return
        }
        
        let targetItem: ConversationItem
        if let existing = viewModel.conversations.first(where: { $0.id == cascadeId }) {
            targetItem = existing
        } else if let cached = CacheManager.shared.loadConversations().first(where: { $0.id == cascadeId }) {
            targetItem = cached
        } else {
            targetItem = ConversationItem(
                id: cascadeId,
                title: "会话",
                status: .running,
                stepCount: 0,
                workspaceName: "Chat",
                lastModified: Date(),
                isSubagent: false,
                isUnread: false
            )
        }
        
        navigationPath = [targetItem]
    }
    
    private func conversationCard(for item: ConversationItem) -> some View {
        let _ = draftsVersion
        let hasDraft = item.isDraft || CacheManager.shared.hasDraft(for: item.id)
        
        return VStack(alignment: .leading, spacing: 8) {
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
                } else if item.status.isError {
                    Text("ERROR")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.red)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.red.opacity(0.15))
                        .cornerRadius(6)
                } else if item.status.isRunning {
                    Text(item.status.rawValue)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.green)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.green.opacity(0.15))
                        .cornerRadius(6)
                } else if hasDraft {
                    Text("DRAFT")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.yellow)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.yellow.opacity(0.15))
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
                    Image(systemName: item.isPureChat ? "bubble.left" : "folder")
                        .font(.system(size: item.isPureChat ? 10.5 : 11))
                        .frame(width: 15, height: 14)
                    Text(item.workspaceName)
                        .font(.system(size: 12, design: .monospaced))
                }
                .foregroundColor(.secondary)
                
                Spacer()
                
                if item.isDraft || (item.stepCount == 0 && hasDraft) {
                    if !item.relativeTimeString.isEmpty {
                        Text("草稿 • \(item.relativeTimeString)")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                    } else {
                        Text("草稿")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                    }
                } else {
                    if !item.relativeTimeString.isEmpty {
                        Text("\(item.stepCount) 步骤 • \(item.relativeTimeString)")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                    } else {
                        Text("\(item.stepCount) 步骤")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                    }
                }
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
    
    var nearestNavigationController: UINavigationController? {
        var responder: UIResponder? = self
        while let next = responder?.next {
            if let nav = next as? UINavigationController {
                return nav
            }
            if let vc = next as? UIViewController, let nav = vc.navigationController {
                return nav
            }
            responder = next
        }
        return nil
    }
}

class NavigationBarTapGestureRecognizer: UITapGestureRecognizer {}

private final class NavigationBarHookView: UIView {
    weak var coordinator: NavigationBarTapHelper.Coordinator?
    
    override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil {
            configure()
        }
    }
    
    override func didMoveToSuperview() {
        super.didMoveToSuperview()
        if superview != nil {
            configure()
        }
    }
    
    func configure() {
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            guard let nav = self.nearestNavigationController else { return }
            let navBar = nav.navigationBar
            let recognizers = navBar.gestureRecognizers ?? []
            if !recognizers.contains(where: { $0 is NavigationBarTapGestureRecognizer }) {
                guard let coordinator = self.coordinator else { return }
                let tap = NavigationBarTapGestureRecognizer(target: coordinator, action: #selector(NavigationBarTapHelper.Coordinator.handleTap))
                tap.cancelsTouchesInView = false
                tap.delegate = coordinator
                navBar.addGestureRecognizer(tap)
            }
        }
    }
}

private struct NavigationBarTapHelper: UIViewRepresentable {
    let onTap: () -> Void
    
    func makeCoordinator() -> Coordinator {
        Coordinator(onTap: onTap)
    }
    
    func makeUIView(context: Context) -> NavigationBarHookView {
        let view = NavigationBarHookView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        view.coordinator = context.coordinator
        return view
    }
    
    func updateUIView(_ uiView: NavigationBarHookView, context: Context) {
        uiView.coordinator = context.coordinator
        uiView.configure()
    }
    
    class Coordinator: NSObject, UIGestureRecognizerDelegate {
        let onTap: () -> Void
        init(onTap: @escaping () -> Void) { self.onTap = onTap }
        
        @objc func handleTap(_ gesture: UITapGestureRecognizer) {
            guard gesture.state == .ended else { return }
            guard let navBar = gesture.view as? UINavigationBar else { return }
            let point = gesture.location(in: navBar)
            if isTouchOnTitle(point: point, in: navBar) {
                onTap()
            }
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            return true
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
            guard let navBar = gestureRecognizer.view as? UINavigationBar else { return false }
            // Only active when the top item is the root Multigravity view
            if let topTitle = navBar.topItem?.title, !topTitle.contains("Multigravity") {
                return false
            }
            
            // Check if touch is on an interactive control (gear, plus, search, etc.)
            var current: UIView? = touch.view
            while let v = current, v !== navBar {
                if v is UIControl && !(v is UILabel) {
                    return false
                }
                let clsName = NSStringFromClass(type(of: v))
                if clsName.contains("ButtonBarButton") || clsName.contains("Search") {
                    return false
                }
                current = v.superview
            }
            
            let point = touch.location(in: navBar)
            return isTouchOnTitle(point: point, in: navBar)
        }
        
        private func isTouchOnTitle(point: CGPoint, in navBar: UINavigationBar) -> Bool {
            // 1. Search for any label containing "Multigravity"
            let labels = findTitleLabels(in: navBar)
            for label in labels where !label.isHidden && label.alpha > 0.05 {
                let frameInNavBar = label.convert(label.bounds, to: navBar)
                let hitRect = frameInNavBar.insetBy(dx: -20, dy: -12)
                if hitRect.contains(point) {
                    return true
                }
            }
            
            // 2. Fallback geometry heuristic
            let width = navBar.bounds.width
            let height = navBar.bounds.height
            if height > 54 {
                // Large title expanded: located in leading area below top bar (y: 36..height, x: 12..width * 0.75)
                let largeTitleRect = CGRect(x: 12, y: 36, width: min(260, width * 0.75), height: height - 36)
                if largeTitleRect.contains(point) {
                    return true
                }
            } else {
                // Inline title collapsed: centered in top bar (y: 0..44, x: center 50%)
                let inlineTitleRect = CGRect(x: width * 0.22, y: 0, width: width * 0.56, height: 44)
                if inlineTitleRect.contains(point) {
                    return true
                }
            }
            return false
        }
        
        private func findTitleLabels(in view: UIView) -> [UILabel] {
            var results: [UILabel] = []
            if let label = view as? UILabel, let text = label.text, text.contains("Multigravity") {
                results.append(label)
            }
            for subview in view.subviews {
                results.append(contentsOf: findTitleLabels(in: subview))
            }
            return results
        }
    }
}


