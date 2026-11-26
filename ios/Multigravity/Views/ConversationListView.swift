import AVFoundation
import SwiftUI

public struct ConversationListView: View {
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.colorScheme) private var colorScheme
    /// Wide screens (iPad / unfolded iPhone Duo landscape): whether the conversation list column is shown next to the chat.
    @State private var showSidebar = true
    @State private var swipedBranchChildId: String? = nil
    @State private var layoutGeometry = AdaptiveLayoutGeometry()
    @State private var chatSessionStore = ChatSessionStore()
    @State private var settings = AppSettings.shared
    @State private var viewModel = ConversationListViewModel()
    @State private var showSettings = false
    @State private var showQRScanner = false
    @State private var showManualInput = false
    @State private var showNewConversation = false
    @State private var pendingCreatedConversation: ConversationItem? = nil
    @State private var pendingChatLaunchActions: [String: HomeScreenQuickAction.ChatLaunchAction] = [:]
    @State private var showQuickActionUnpairedAlert = false
    @State private var showAccountQuota = false
    @State private var showBackgroundTasksSheet = false
    @State private var navigationPath: [ConversationItem] = []
    @State private var shareInbox = ShareInbox.shared
    @State private var pendingShareItem: ConversationItem? = nil
    
    @State private var conversationToDelete: ConversationItem?
    @State private var showDeleteConfirm = false
    @State private var conversationToPin: ConversationItem?
    @State private var showPinConfirm = false
    @State private var conversationToArchive: ConversationItem?
    @State private var showArchiveConfirm = false
    @State private var conversationToRename: ConversationItem?
    @State private var renameText = ""
    @State private var showRenameAlert = false
    @State private var draftsVersion: Int = 0
    @ObservedObject private var audioRecorder = AudioRecorderManager.shared
    @State private var pendingPairing: PairingInfo?
    @State private var showPairingConfirm = false
    @State private var pairingErrorMessage: String?
    @State private var isPairingInProgress = false
    
    @State private var easterEggTapCount: Int = 0
    @State private var lastEasterEggTapTime: Date = .distantPast
    @State private var showEasterEgg = false
    @State private var firstCardPeekOffset: CGFloat = 0

    @State private var gatewayPickerState = GatewaySlidePickerState()
    @State private var gatewayStore = GatewayProfileStore.shared
    @State private var gatewaySwitcher = GatewaySwitcher.shared
    @State private var isNavTitleCollapsed: Bool = false
    @State private var isCollapsedTitlePressed: Bool = false
    @State private var scrollToTopToken: Int = 0
    @State private var gatewayToRename: GatewayProfile? = nil
    @State private var gatewayRenameText = ""
    @State private var showGatewayRenameAlert = false
    @State private var gatewayToRemove: GatewayProfile? = nil
    @State private var showGatewayRemoveAlert = false
    
    public init() {
        _ = SwipeActionAdjuster.activateOnce
        ShareInbox.shared.cleanupStale()
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
        applyLifecycleAndRoutingModifiers(
            applySheetAndAlertModifiers(rootLayoutContent)
        )
    }

    private var rootLayoutContent: some View {
        ZStack {
            if !settings.isPaired && !isPairingInProgress {
                OnboardingGuideView(
                    onScanTapped: { showQRScanner = true },
                    onManualInputTapped: { showManualInput = true },
                    onDemoTapped: {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        settings.enterDemoMode()
                        // onAppear already ran for the onboarding page; load the demo data now.
                        viewModel.startAutoRefresh()
                        Task {
                            await viewModel.fetchConversations()
                            await ProjectCacheManager.shared.fetchAndCacheProjects()
                            await WorkflowTemplateStore.shared.refresh()
                        }
                    },
                    onEasterEggTap: handleEasterEggTap
                )
                // Keep the welcome page readable on wide screens.
                .frame(maxWidth: 640)
                .frame(maxWidth: .infinity)
            } else if usesSplitLayout {
                splitLayout
            } else {
                NavigationStack(path: $navigationPath) {
                    listWithChrome
                        .navigationDestination(for: ConversationItem.self) { item in
                            chatDestination(for: item)
                        }
                }
            }
            
            GatewaySlidePickerOverlay(
                state: gatewayPickerState,
                onSelectProfile: { profile in
                    _ = GatewaySwitcher.shared.switchTo(
                        profileId: profile.id,
                        currentConversations: viewModel.conversations,
                        showToast: true
                    )
                },
                onAddGateway: {
                    openAddGatewayFlow()
                },
                onRenameProfile: { profile in
                    gatewayToRename = profile
                    gatewayRenameText = profile.resolvedDisplayName
                    showGatewayRenameAlert = true
                },
                onRemoveProfile: { profile in
                    gatewayToRemove = profile
                    showGatewayRemoveAlert = true
                }
            )

            if let bannerText = gatewaySwitcher.switchBannerText {
                VStack {
                    HStack(spacing: 6) {
                        Image(systemName: "arrow.triangle.2.circlepath")
                            .font(.system(size: 11.5, weight: .semibold))
                            .foregroundColor(.indigo)
                        Text(bannerText)
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(.ultraThinMaterial, in: Capsule())
                    .overlay(
                        Capsule()
                            .stroke(Color.primary.opacity(0.12), lineWidth: 0.8)
                    )
                    .shadow(color: Color.black.opacity(0.12), radius: 10, x: 0, y: 4)
                    .padding(.top, 10)
                    Spacer()
                }
                .allowsHitTesting(false)
                .transition(.move(edge: .top).combined(with: .opacity))
                .zIndex(600)
            }

            if showEasterEgg {
                EasterEggModalView(isPresented: $showEasterEgg)
                    .zIndex(999)
            }
        }
        .animation(.spring(response: 0.28, dampingFraction: 0.85), value: gatewaySwitcher.switchBannerText)
        .onGeometryChange(for: AdaptiveLayoutGeometry.self) { proxy in
            AdaptiveLayoutGeometry.measure(from: proxy)
        } action: { newGeometry in
            layoutGeometry = newGeometry
        }
    }

    @ViewBuilder
    private func applySheetAndAlertModifiers<Content: View>(_ content: Content) -> some View {
        content
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
                navigationPath = [item]
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
                tokenHistory: viewModel.tokenHistory,
                isLoadingTokenHistory: viewModel.isLoadingTokenHistory,
                onLoadTokenHistory: { force in
                    await viewModel.fetchTokenHistory(force: force)
                },
                onSwitch: { id in
                    try await viewModel.switchCockpitAccount(id: id)
                },
                onRefresh: {
                    try await viewModel.refreshCockpitQuotas()
                },
                onAppearFetch: {
                    await viewModel.fetchQuotas(force: true)
                },
                enableSwitchButton: true,
                linkMonitor: viewModel.linkMonitor
            )
            .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: $showBackgroundTasksSheet) {
            McpSkillsSheet(
                initialTab: .tasks,
                currentConversationId: navigationPath.last?.id,
                onSelectConversation: { targetId in
                    openConversationById(targetId)
                },
                onTasksChanged: { updated in
                    viewModel.updateGlobalBackgroundTasks(updated)
                }
            )
            .presentationDetents([.large])
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
        .alert("重命名主机", isPresented: $showGatewayRenameAlert) {
            TextField("输入主机名称", text: $gatewayRenameText)
            Button("取消", role: .cancel) {
                gatewayToRename = nil
            }
            Button("保存") {
                if let target = gatewayToRename {
                    GatewayProfileStore.shared.renameProfile(id: target.id, newName: gatewayRenameText)
                }
                gatewayToRename = nil
            }
        }
        .alert("移除主机", isPresented: $showGatewayRemoveAlert) {
            Button("取消", role: .cancel) {
                gatewayToRemove = nil
            }
            Button("移除", role: .destructive) {
                if let target = gatewayToRemove {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    if target.id == GatewayProfileStore.shared.activeProfileId {
                        let hasRemaining = settings.unpairActiveProfile()
                        if !hasRemaining {
                            handleAllGatewaysCleared()
                        }
                    } else {
                        GatewayProfileStore.shared.removeProfile(id: target.id)
                    }
                }
                gatewayToRemove = nil
            }
        } message: {
            if let target = gatewayToRemove {
                Text(String(localized: "确定移除「\(target.resolvedDisplayName)」吗？将通知该网关解绑此设备并清除该主机的本地缓存。"))
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
        .alert("请先配对电脑网关", isPresented: $showQuickActionUnpairedAlert) {
            if AVCaptureDevice.authorizationStatus(for: .video) != .restricted {
                Button("扫码配对") {
                    showQRScanner = true
                }
            }
            Button("粘贴或手动配对") {
                showManualInput = true
            }
            Button("取消", role: .cancel) {}
        } message: {
            Text("配对电脑网关（或进入演示模式）后，即可从桌面长按图标快速发起对话、拍照或扫描文稿。")
        }
    }

    @ViewBuilder
    private func applyLifecycleAndRoutingModifiers<Content: View>(_ content: Content) -> some View {
        content
        .onAppear {
            handleListAppear()
        }
        .onChange(of: navigationPath) { oldPath, newPath in
            let viewingId = newPath.last?.id
            PushNotificationManager.shared.activeViewingCascadeId = viewingId
            if let viewingId, !viewingId.isEmpty {
                PushNotificationManager.shared.clearDeliveredNotifications(for: viewingId)
            } else if !oldPath.isEmpty && !hasBlockingModalPresented {
                // 用户看完刚跑完的会话后心满意足返回列表页：顺势承接未过期的爽点窗口
                AppReviewManager.shared.flushPendingPromptIfReady(delay: 0.9)
            }
        }
        .onChange(of: hasBlockingModalPresented) { _, isBlocked in
            PushNotificationManager.shared.hasBlockingModalPresented = isBlocked
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
        .onReceive(NotificationCenter.default.publisher(for: .deviceTokenRevoked)) { notif in
            handleTokenRevoked(notif)
        }
        .onReceive(NotificationCenter.default.publisher(for: .didSwitchGatewayProfile)) { _ in
            dismissAllModals()
            navigationPath = []
            chatSessionStore.clearAll()
            viewModel.resetForGatewaySwitch()
            scrollToTopToken += 1
            Task {
                async let projects = ProjectCacheManager.shared.fetchAndCacheProjects()
                async let workflows: Void = WorkflowTemplateStore.shared.refresh()
                async let mcp: Void = CacheManager.shared.fetchAndCacheMcpAndSkills()
                _ = await (projects, workflows, mcp)
            }
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
        .onReceive(NotificationCenter.default.publisher(for: .openNotificationRoute)) { notif in
            if let route = notif.object as? PushNotificationManager.NotificationRoute {
                handleNotificationRoute(route)
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .openConversationFromTaskCenter)) { notif in
            if let targetId = notif.object as? String {
                openConversationById(targetId)
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .openConversationById)) { notif in
            if let targetId = notif.object as? String, !targetId.isEmpty {
                openConversationById(targetId)
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .startNewSessionForWorkspace)) { notif in
            dismissAllModals()
            let wsName = (notif.object as? String) ?? ""
            let cachedProjects = ProjectCacheManager.shared.loadProjects()
            let project = cachedProjects.first(where: { !$0.isPureChat && ($0.name == wsName || $0.displayName == wsName) }) ?? ProjectItem.pureChat
            let session = CacheManager.shared.createLocalDraftSession(project: project)
            navigationPath = [session.toConversationItem()]
        }
        .onOpenURL { url in
            if url.isFileURL {
                handleSharedFile(url)
            } else {
                handleDeepLink(url)
            }
        }
        .onChange(of: settings.selectedQuotaBucket) { _, _ in
            viewModel.publishQuotaToWidget()
        }
        .sheet(isPresented: $shareInbox.showSheet, onDismiss: {
            if let item = pendingShareItem {
                pendingShareItem = nil
                navigationPath = [item]
            }
        }) {
            ShareTargetSheet(
                inbox: shareInbox,
                conversations: viewModel.conversations,
                currentConversationId: navigationPath.last?.id,
                onSelectProject: { project in
                    let session = CacheManager.shared.createLocalDraftSession(project: project)
                    let item = session.toConversationItem()
                    pendingShareItem = item
                    shareInbox.deliver(to: item.id)
                },
                onSelectConversation: { item in
                    // Already open: the chat picks the files up itself; otherwise open it after the sheet closes.
                    if navigationPath.last?.id != item.id { pendingShareItem = item }
                    shareInbox.deliver(to: item.id)
                }
            )
        }
        .overlay(alignment: .bottom) {
            if !shareInbox.files.isEmpty && !shareInbox.showSheet && navigationPath.isEmpty && settings.isPaired {
                // Sits above the bottom search capsule (centered), drawn with the native glass effect.
                // When iPhone Duo places the search button in the vertical side bar, sit closer to the bottom edge.
                SharePendingDeliveryPill(count: shareInbox.files.count) {
                    shareInbox.showSheet = true
                }
            }
        }
    }
    
    // MARK: - Layout (compact stack / wide split)
    
    /// Two-column split requires both a regular horizontal size class and enough width (>= 700 pt)
    /// so sidebar + chat never crush on narrower regular canvases (such as iPhone Duo inner portrait 669 pt
    /// or narrow multitasking windows).
    private var usesSplitLayout: Bool {
        horizontalSizeClass == .regular && (layoutGeometry.width == 0 || layoutGeometry.width >= 700)
    }

    private struct SplitColumnMetrics {
        let sidebarWidth: CGFloat
        let hingeGapWidth: CGFloat
    }

    private var splitColumnMetrics: SplitColumnMetrics {
        let width = layoutGeometry.width
        // iPhone Duo Book posture: align the split boundary to the active vertical fold division
        // so the conversation list stays on the leading pane and the chat stays on the trailing pane.
        if let division = layoutGeometry.verticalDivision,
           division.minX >= 280,
           (width - division.maxX) >= 340 {
            return SplitColumnMetrics(
                sidebarWidth: floor(division.minX),
                hingeGapWidth: ceil(division.width)
            )
        }
        guard width > 0 else {
            return SplitColumnMetrics(sidebarWidth: 360, hingeGapWidth: 0)
        }
        // Keep exact 390 pt sidebar on wide iPads (>= 1020 pt); scale gracefully on iPhone Duo
        // unfolded landscape (~951 pt -> 342 pt) and narrower iPad portraits (744–834 pt -> 320 pt).
        let sidebar: CGFloat = width >= 1020
            ? 390
            : min(390, max(320, floor(width * 0.36)))
        return SplitColumnMetrics(sidebarWidth: sidebar, hingeGapWidth: 0)
    }
    
    private var selectedConversationID: String? { navigationPath.last?.id }
    
    /// Opens a conversation: replaces the detail on iPad, pushes on iPhone.
    private func compatBanner(_ text: String) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(.red)
                .padding(.top, 2)
            Text(text)
                .font(.system(size: 13.5))
                .foregroundColor(.primary)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color.red.opacity(0.12), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Color.red.opacity(0.35), lineWidth: 1))
        .padding(.horizontal, 16)
        .padding(.vertical, 6)
    }
    
    private var hasBlockingModalPresented: Bool {
        showSettings
            || showAccountQuota
            || showBackgroundTasksSheet
            || showNewConversation
            || showQRScanner
            || showManualInput
            || shareInbox.showSheet
            || showEasterEgg
    }

    private static func activeRootViewController() -> UIViewController? {
        UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .first(where: { $0.activationState == .foregroundActive })?
            .windows.first(where: \.isKeyWindow)?.rootViewController
        ?? UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first(where: \.isKeyWindow)?.rootViewController
    }

    private static func activePresentedViewController() -> UIViewController? {
        guard let presented = activeRootViewController()?.presentedViewController,
              !presented.isBeingDismissed else { return nil }
        return presented
    }

    private static func dismissRemainingPresentedViewControllerIfNeeded(matching target: UIViewController) {
        let targetID = ObjectIdentifier(target)
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
            guard let root = activeRootViewController(),
                  let current = root.presentedViewController,
                  ObjectIdentifier(current) == targetID,
                  !current.isBeingDismissed else { return }
            root.dismiss(animated: true)
        }
    }

    @discardableResult
    private func dismissAllModals(excludingQuota: Bool = false, excludingNewConversation: Bool = false) -> Bool {
        var hadModal = false
        if showSettings { showSettings = false; hadModal = true }
        if !excludingQuota && showAccountQuota { showAccountQuota = false; hadModal = true }
        if showBackgroundTasksSheet { showBackgroundTasksSheet = false; hadModal = true }
        if !excludingNewConversation && showNewConversation {
            pendingCreatedConversation = nil
            showNewConversation = false
            hadModal = true
        }
        if showQRScanner { showQRScanner = false; hadModal = true }
        if showManualInput { showManualInput = false; hadModal = true }
        if shareInbox.showSheet {
            pendingShareItem = nil
            shareInbox.showSheet = false
            hadModal = true
        }
        if showEasterEgg { showEasterEgg = false; hadModal = true }
        if gatewayPickerState.isPresented { gatewayPickerState.dismiss() }
        showGatewayRenameAlert = false
        showGatewayRemoveAlert = false
        showRenameAlert = false
        showDeleteConfirm = false
        showPinConfirm = false
        showArchiveConfirm = false
        conversationToArchive = nil
        showPairingConfirm = false
        showQuickActionUnpairedAlert = false
        pendingPairing = nil

        NotificationCenter.default.post(name: .dismissAllPresentedModals, object: nil)
        let shouldSkipPresentedDismiss = (excludingQuota && showAccountQuota) || (excludingNewConversation && showNewConversation)
        if !shouldSkipPresentedDismiss {
            if let existingPresented = Self.activePresentedViewController() {
                hadModal = true
                Self.dismissRemainingPresentedViewControllerIfNeeded(matching: existingPresented)
            }
        }
        return hadModal
    }

    private func switchGatewayIfNeeded(gatewayId: String?, fallbackCascadeId: String? = nil) {
        let store = GatewayProfileStore.shared
        var targetProfileId: String? = nil
        if let gw = gatewayId?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty,
           let matched = store.profile(matchingGatewayId: gw) {
            targetProfileId = matched.id
        } else if let cid = fallbackCascadeId, !cid.isEmpty, store.hasMultipleGateways {
            targetProfileId = store.profileId(containingCachedCascade: cid)
        }
        if let targetId = targetProfileId, targetId != store.activeProfileId {
            _ = GatewaySwitcher.shared.switchTo(
                profileId: targetId,
                currentConversations: viewModel.conversations,
                showToast: true
            )
        }
    }

    private func handleNotificationRoute(_ route: PushNotificationManager.NotificationRoute) {
        PushNotificationManager.shared.clearPendingRouteIfMatching(route)
        switch route {
        case .cascade(let targetId, let gatewayId):
            switchGatewayIfNeeded(gatewayId: gatewayId, fallbackCascadeId: targetId)
            openConversationById(targetId)
        case .quota(let gatewayId):
            switchGatewayIfNeeded(gatewayId: gatewayId)
            openAccountQuotaSheet()
        case .deepLink(let url):
            handleDeepLink(url)
        }
    }

    private func openAccountQuotaSheet() {
        guard settings.isPaired else { return }
        PushNotificationManager.shared.clearPendingRouteIfMatching(.quota())
        if showAccountQuota {
            Task {
                await viewModel.fetchQuotas(force: true)
            }
            return
        }
        let hadOtherModal = dismissAllModals(excludingQuota: true)
        if hadOtherModal {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.38) {
                guard settings.isPaired else { return }
                showAccountQuota = true
            }
        } else {
            showAccountQuota = true
        }
        Task {
            await viewModel.fetchQuotas(force: true)
        }
    }

    private func resolveParentConversationItem(for item: ConversationItem) -> ConversationItem? {
        guard item.isSideQuestion,
              let parentId = item.branchParentId?.trimmingCharacters(in: .whitespacesAndNewlines),
              !parentId.isEmpty,
              parentId != item.id else {
            return nil
        }
        if let found = viewModel.conversations.first(where: { $0.id == parentId }) {
            return found
        }
        if let cached = CacheManager.shared.loadConversations().first(where: { $0.id == parentId }) {
            return cached
        }
        return ConversationItem(
            id: parentId,
            title: (item.branchParentTitle?.isEmpty == false ? item.branchParentTitle! : String(localized: "会话详情")),
            status: .idle,
            stepCount: 0,
            workspaceName: item.workspaceName,
            lastModified: item.lastModified
        )
    }

    private func openConversation(_ item: ConversationItem, includeSideQuestionParent: Bool = false) {
        if usesSplitLayout {
            navigationPath = [item]
        } else if includeSideQuestionParent,
                  let parentItem = resolveParentConversationItem(for: item),
                  navigationPath.last?.id != parentItem.id {
            navigationPath = [parentItem, item]
        } else {
            navigationPath.append(item)
        }
    }

    private func openConversationById(_ targetId: String) {
        let trimmedId = targetId.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedId.isEmpty, settings.isPaired else { return }
        PushNotificationManager.shared.clearPendingRouteIfMatching(.cascade(cascadeId: trimmedId))
        dismissAllModals()

        if navigationPath.last?.id == trimmedId {
            return
        }

        let targetItem: ConversationItem
        if let found = viewModel.conversations.first(where: { $0.id == trimmedId }) {
            targetItem = found
        } else if let draft = CacheManager.shared.getLocalDraftSession(id: trimmedId)?.toConversationItem() {
            targetItem = draft
        } else if let cached = CacheManager.shared.loadConversations().first(where: { $0.id == trimmedId }) {
            targetItem = cached
        } else {
            let taskMeta = viewModel.globalBackgroundTasks.first(where: { $0.conversationId == trimmedId })
            targetItem = ConversationItem(
                id: trimmedId,
                title: taskMeta?.conversationTitle ?? String(localized: "会话详情"),
                status: .running,
                stepCount: 1,
                workspaceName: taskMeta?.workspaceName ?? "",
                lastModified: Date()
            )
        }
        if usesSplitLayout {
            navigationPath = [targetItem]
        } else if let existingIdx = navigationPath.firstIndex(where: { $0.id == trimmedId }) {
            navigationPath = Array(navigationPath.prefix(through: existingIdx))
        } else if targetItem.isSideQuestion,
                  let parentId = targetItem.branchParentId,
                  navigationPath.last?.id == parentId {
            navigationPath.append(targetItem)
        } else if let parentItem = resolveParentConversationItem(for: targetItem) {
            navigationPath = [parentItem, targetItem]
        } else {
            navigationPath = [targetItem]
        }
    }

    private var globalBackgroundTasksCapsule: some View {
        let count = viewModel.globalBackgroundTasks.count
        let daemonCount = viewModel.globalBackgroundTasks.filter(\.isDaemon).count
        return Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            showBackgroundTasksSheet = true
        } label: {
            HStack(spacing: 8) {
                Image(systemName: "terminal.fill")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(.indigo)

                Text(daemonCount > 0
                     ? (daemonCount == 1
                        ? String(localized: "\(count) 个后台任务运行中（1 个常驻）")
                        : String(localized: "\(count) 个后台任务运行中（\(daemonCount) 个常驻）"))
                     : (count == 1
                        ? String(localized: "1 个后台任务运行中")
                        : String(localized: "\(count) 个后台任务运行中")))
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)
                    .contentTransition(.numericText())
                    .animation(.spring(response: 0.35, dampingFraction: 0.8), value: count)
                    .animation(.spring(response: 0.35, dampingFraction: 0.8), value: daemonCount)

                Spacer(minLength: 4)

                HStack(spacing: 3) {
                    Text("管理")
                        .font(.system(size: 11.5, weight: .semibold))
                        .lineLimit(1)
                    Image(systemName: "chevron.right")
                        .font(.system(size: 9.5, weight: .bold))
                }
                .fixedSize(horizontal: true, vertical: false)
                .layoutPriority(1)
                .foregroundColor(.indigo)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(Capsule())
            .overlay(
                Capsule()
                    .stroke(Color.indigo.opacity(0.28), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
    }

    private func hiddenPendingApprovalCapsule(count: Int) -> some View {
        Button {
            clearWorkspaceFilter()
        } label: {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.circle.fill")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.orange)

                Text(count == 1
                     ? String(localized: "其他工作区有 1 个待审批")
                     : String(localized: "其他工作区有 \(count) 个待审批"))
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)
                    .contentTransition(.numericText())

                Spacer(minLength: 4)

                HStack(spacing: 2) {
                    Text("查看")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.orange)
                        .lineLimit(1)
                    Image(systemName: "chevron.right")
                        .font(.system(size: 9.5, weight: .bold))
                        .foregroundColor(.orange.opacity(0.85))
                }
                .fixedSize(horizontal: true, vertical: false)
                .layoutPriority(1)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color.orange.opacity(0.10))
            .clipShape(Capsule())
            .overlay(
                Capsule()
                    .stroke(Color.orange.opacity(0.32), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
    }
    
    private func toggleWorkspaceFilter(for item: ConversationItem) {
        let target: ConversationListViewModel.WorkspaceFilter = item.isPureChat ? .chat : .workspace(item.workspaceName)
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
            if viewModel.selectedWorkspaceFilter == target {
                viewModel.selectedWorkspaceFilter = nil
            } else {
                viewModel.selectedWorkspaceFilter = target
            }
        }
    }

    private func selectWorkspaceFilter(_ filter: ConversationListViewModel.WorkspaceFilter) {
        guard viewModel.selectedWorkspaceFilter != filter else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
            viewModel.selectedWorkspaceFilter = filter
        }
    }

    private func clearWorkspaceFilter() {
        guard viewModel.selectedWorkspaceFilter != nil else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        withAnimation(.spring(response: 0.34, dampingFraction: 0.86)) {
            viewModel.selectedWorkspaceFilter = nil
        }
    }

    private func handleNewConversationTapped() {
        guard let filter = viewModel.selectedWorkspaceFilter else {
            showNewConversation = true
            return
        }
        switch filter {
        case .chat:
            let session = CacheManager.shared.createLocalDraftSession(project: .pureChat)
            openConversation(session.toConversationItem())
        case .workspace(let wsName):
            let cachedProjects = ProjectCacheManager.shared.loadProjects()
            if let matched = cachedProjects.first(where: { !$0.isPureChat && ($0.name == wsName || $0.displayName == wsName) }) {
                let session = CacheManager.shared.createLocalDraftSession(project: matched)
                openConversation(session.toConversationItem())
            } else if let draftProj = viewModel.workspaceFilteredConversations.compactMap(\.draftProject).first(where: { !$0.isPureChat }) {
                let session = CacheManager.shared.createLocalDraftSession(project: draftProj)
                openConversation(session.toConversationItem())
            } else {
                showNewConversation = true
            }
        case .archived, .starred:
            showNewConversation = true
        }
    }

    private func workspaceFilterCapsule(for filter: ConversationListViewModel.WorkspaceFilter) -> some View {
        let totalCount = viewModel.workspaceTotalCount
        let options = viewModel.availableWorkspaceFilters
        return HStack(spacing: 8) {
            Menu {
                Button {
                    clearWorkspaceFilter()
                } label: {
                    Label(String(localized: "全部会话 (\(viewModel.totalVisibleCount))"), systemImage: "tray.full")
                }

                if !options.isEmpty {
                    Divider()
                    ForEach(options) { opt in
                        Button {
                            selectWorkspaceFilter(opt.filter)
                        } label: {
                            HStack {
                                Label(
                                    "\(opt.filter.displayName) (\(opt.count))",
                                    systemImage: opt.filter.isArchived ? "archivebox" : (opt.filter.isStarred ? "star" : (opt.filter.isPureChat ? "bubble.left" : "folder"))
                                )
                                if opt.filter == filter {
                                    Image(systemName: "checkmark")
                                }
                            }
                        }
                    }
                }
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: filter.isArchived ? "archivebox.fill" : (filter.isStarred ? "star.fill" : (filter.isPureChat ? "bubble.left.fill" : "folder.fill")))
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(filter.isStarred ? .yellow : .indigo)
                    Text(filter.displayName)
                        .font(.system(size: 12.5, weight: .semibold, design: (filter.isPureChat || filter.isArchived || filter.isStarred) ? .default : .monospaced))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    Image(systemName: "chevron.down")
                        .font(.system(size: 9.5, weight: .bold))
                        .foregroundColor(.secondary)
                }
                .padding(.vertical, 2)
                .contentShape(Rectangle())
            }
            .buttonStyle(.borderless)

            Spacer(minLength: 4)

            Text(String(localized: "\(totalCount) 个会话"))
                .font(.system(size: 11.5, weight: .medium))
                .foregroundColor(.secondary)
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .contentTransition(.numericText())

            Button {
                clearWorkspaceFilter()
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.system(size: 15))
                    .foregroundColor(.secondary)
                    .padding(.leading, 2)
                    .contentShape(Circle())
            }
            .buttonStyle(.borderless)
            .accessibilityLabel(String(localized: "退出项目筛选"))
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(Capsule())
        .overlay(
            Capsule()
                .stroke(Color.indigo.opacity(0.28), lineWidth: 0.8)
        )
    }

    private var shouldUseCollapsedHostHeader: Bool {
        settings.isPaired && !settings.isDemoMode && gatewayStore.hasMultipleGateways
    }

    private var collapsedHostTitleView: some View {
        let hostName = gatewayStore.activeProfile?.resolvedDisplayName ?? "Multigravity"
        let hasPendingAction = gatewayStore.hasInactivePendingAction
        return HStack(spacing: 4) {
            Text(hostName)
                .font(.system(size: 16.5, weight: .semibold))
                .foregroundColor(.primary)
                .lineLimit(1)
                .truncationMode(.tail)
            Image(systemName: "chevron.down")
                .font(.system(size: 10.5, weight: .bold))
                .foregroundColor(.secondary)
                .rotationEffect(.degrees(gatewayPickerState.isPresented ? 180 : 0))
                .animation(.spring(response: 0.25, dampingFraction: 0.82), value: gatewayPickerState.isPresented)
                .fixedSize()
            if hasPendingAction {
                Circle()
                    .fill(Color.orange)
                    .frame(width: 5.5, height: 5.5)
                    .fixedSize()
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .frame(minWidth: 108, minHeight: 36)
        .contentShape(Rectangle())
        .opacity(isNavTitleCollapsed ? (isCollapsedTitlePressed ? 0.55 : 1) : 0)
        .offset(y: isNavTitleCollapsed ? 0 : 4)
        .animation(.easeOut(duration: 0.16), value: isNavTitleCollapsed)
        .animation(.easeOut(duration: 0.10), value: isCollapsedTitlePressed)
        .overlay {
            CollapsedHostTitleGestureOverlay(
                isEnabled: isNavTitleCollapsed,
                onPressChanged: { pressed in
                    if isCollapsedTitlePressed != pressed {
                        isCollapsedTitlePressed = pressed
                    }
                },
                onTap: { anchorRectInWindow, windowSize in
                    if gatewayPickerState.isPresented {
                        gatewayPickerState.dismiss()
                        return
                    }
                    GatewayProfileStore.shared.updateActiveSummary(from: viewModel.conversations)
                    gatewayPickerState.openInteractive(
                        anchorRect: anchorRectInWindow,
                        containerSize: windowSize,
                        centered: true
                    )
                },
                onLongPressBegan: { anchorRectInWindow, windowSize in
                    gatewayPickerState.containerSize = windowSize
                    GatewayProfileStore.shared.updateActiveSummary(from: viewModel.conversations)
                    gatewayPickerState.open(
                        anchorRect: anchorRectInWindow,
                        centered: true
                    )
                },
                onLongPressChanged: { pointInWindow in
                    gatewayPickerState.updateDrag(
                        at: pointInWindow,
                        profileCount: GatewayProfileStore.shared.profiles.count
                    )
                },
                onLongPressEnded: {
                    gatewayPickerState.endDrag(
                        profiles: GatewayProfileStore.shared.profiles,
                        onSelectProfile: { profile in
                            _ = GatewaySwitcher.shared.switchTo(
                                profileId: profile.id,
                                currentConversations: viewModel.conversations,
                                showToast: true
                            )
                        },
                        onAddGateway: {
                            openAddGatewayFlow()
                        }
                    )
                },
                onLongPressCancelled: {
                    gatewayPickerState.dismiss()
                },
                canHorizontalSwipe: {
                    AppSettings.shared.isPaired
                        && !AppSettings.shared.isDemoMode
                        && GatewayProfileStore.shared.hasMultipleGateways
                        && !gatewayPickerState.isPresented
                },
                onHorizontalSwipe: { step in
                    UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
                    _ = GatewaySwitcher.shared.cycle(
                        step: step,
                        currentConversations: viewModel.conversations,
                        showToast: true
                    )
                }
            )
        }
        .allowsHitTesting(isNavTitleCollapsed)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isButton)
        .accessibilityHidden(!isNavTitleCollapsed)
        .accessibilityLabel(
            hasPendingAction
                ? String(localized: "当前主机 \(hostName)，其他主机有待审批")
                : String(localized: "当前主机 \(hostName)")
        )
        .accessibilityHint(String(localized: "轻点展开主机切换面板，左右轻扫快速切换主机"))
    }

    /// The conversation list with its navigation chrome (title, search, toolbar).
    private var listWithChrome: some View {
        pairedContentView
            .safeAreaInset(edge: .top, spacing: 0) {
                // 升级自检：基础能力缺失时在首页顶部提示（任何状态下可见，包括加载失败）
                if GatewayCompatStore.shared.compat.isIncompatible {
                    compatBanner(GatewayCompatStore.shared.compat.bannerText)
                }
            }
            .navigationTitle("Multigravity")
            .modifier(ConversationSearch(
                text: $viewModel.searchQuery,
                usesBottomCapsule: usesSplitLayout,
                filterName: viewModel.selectedWorkspaceFilter?.displayName
            ))
            .onChange(of: viewModel.searchQuery) { _, _ in
                viewModel.scheduleContentSearch()
            }
            .onChange(of: viewModel.selectedWorkspaceFilter) { _, _ in
                if !viewModel.searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    viewModel.scheduleContentSearch()
                }
            }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: { showSettings = true }) {
                        Image(systemName: "gearshape")
                    }
                    .keyboardShortcut(",", modifiers: .command)
                    .help("设置")
                }
                if shouldUseCollapsedHostHeader {
                    ToolbarItem(placement: .principal) {
                        collapsedHostTitleView
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: handleNewConversationTapped) {
                        Image(systemName: "plus")
                            .font(.system(size: 16, weight: .semibold))
                    }
                    .keyboardShortcut("n", modifiers: .command)
                    .help(viewModel.selectedWorkspaceFilter.map { String(localized: "在 \($0.displayName) 新建会话") } ?? String(localized: "新建会话"))
                }
            }
            .background(
                NavigationBarTapHelper(
                    scrollToTopToken: scrollToTopToken,
                    onTap: handleEasterEggTap,
                    canOpenCollapsedDropdown: {
                        AppSettings.shared.isPaired
                            && !AppSettings.shared.isDemoMode
                            && GatewayProfileStore.shared.hasMultipleGateways
                    },
                    onCollapsedTitleTap: { anchorRectInWindow, windowSize in
                        if gatewayPickerState.isPresented {
                            gatewayPickerState.dismiss()
                            return
                        }
                        GatewayProfileStore.shared.updateActiveSummary(from: viewModel.conversations)
                        gatewayPickerState.openInteractive(
                            anchorRect: anchorRectInWindow,
                            containerSize: windowSize,
                            centered: true
                        )
                    },
                    canBeginLongPress: {
                        AppSettings.shared.isPaired && !AppSettings.shared.isDemoMode
                    },
                    onLongPressBegan: { anchorRectInWindow, windowSize, isCollapsedInline in
                        gatewayPickerState.containerSize = windowSize
                        GatewayProfileStore.shared.updateActiveSummary(from: viewModel.conversations)
                        gatewayPickerState.open(
                            anchorRect: anchorRectInWindow,
                            centered: isCollapsedInline
                        )
                    },
                    onLongPressChanged: { pointInWindow in
                        gatewayPickerState.updateDrag(
                            at: pointInWindow,
                            profileCount: GatewayProfileStore.shared.profiles.count
                        )
                    },
                    onLongPressEnded: {
                        gatewayPickerState.endDrag(
                            profiles: GatewayProfileStore.shared.profiles,
                            onSelectProfile: { profile in
                                _ = GatewaySwitcher.shared.switchTo(
                                    profileId: profile.id,
                                    currentConversations: viewModel.conversations,
                                    showToast: true
                                )
                            },
                            onAddGateway: {
                                openAddGatewayFlow()
                            }
                        )
                    },
                    onLongPressCancelled: {
                        gatewayPickerState.dismiss()
                    },
                    canHorizontalSwipe: {
                        AppSettings.shared.isPaired
                            && !AppSettings.shared.isDemoMode
                            && GatewayProfileStore.shared.hasMultipleGateways
                            && !gatewayPickerState.isPresented
                    },
                    onHorizontalSwipe: { step in
                        UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
                        _ = GatewaySwitcher.shared.cycle(
                            step: step,
                            currentConversations: viewModel.conversations,
                            showToast: true
                        )
                    },
                    onTitleCollapsedChanged: { collapsed in
                        if isNavTitleCollapsed != collapsed {
                            isNavTitleCollapsed = collapsed
                        }
                    }
                )
            )
    }
    
    /// Wide screens (iPad / unfolded iPhone Duo landscape): the conversation list column is the
    /// same navigation page as compact mode (large title, native toolbar, search); the chat sits
    /// beside it. On iOS 27.1+ (iPhone Duo), vertical bar mode is disabled in split layout so the
    /// two side-by-side NavigationStacks keep unified horizontal top chrome and `sidebar.left` stays
    /// anchored beside the sidebar rather than migrating to the far trailing edge.
    private var splitLayout: some View {
        let metrics = splitColumnMetrics
        return HStack(spacing: 0) {
            if showSidebar {
                NavigationStack { listWithChrome }
                    .frame(width: metrics.sidebarWidth)
                    .transition(.move(edge: .leading))
                if metrics.hingeGapWidth > 2 {
                    ZStack {
                        Color.clear
                        Divider().ignoresSafeArea()
                    }
                    .frame(width: metrics.hingeGapWidth)
                } else {
                    Divider().ignoresSafeArea()
                }
            }
            NavigationStack {
                Group {
                    if let item = navigationPath.last {
                        chatDestination(for: item)
                            .id(item.id)
                    } else {
                        ContentUnavailableView {
                            Label("选择一个会话", systemImage: "bubble.left.and.text.bubble.right")
                        } description: {
                            Text("从左侧选择会话查看，或新建一个会话")
                        } actions: {
                            Button("新建会话") { showNewConversation = true }
                                .buttonStyle(.borderedProminent)
                        }
                    }
                }
                .toolbar {
                    SidebarToggleToolbarItem(isShown: showSidebar) {
                        withAnimation(.easeInOut(duration: 0.25)) { showSidebar.toggle() }
                    }
                }
            }
        }
        .modifier(DisableVerticalToolbarIfAvailable())
    }
    
    @ViewBuilder
    private func chatDestination(for item: ConversationItem) -> some View {
        ChatView(
            conversation: item,
            isNewConversation: item.isDraft || item.stepCount == 0,
            sessionStore: chatSessionStore,
            initialLaunchAction: pendingChatLaunchActions[item.id],
            onConsumeLaunchAction: {
                pendingChatLaunchActions.removeValue(forKey: item.id)
            }
        )
            .id(item.id)
            .environment(\.isSplitDetail, usesSplitLayout)
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
    
    @ViewBuilder
    private var pairedContentView: some View {
        if isPairingInProgress || (viewModel.isLoading && viewModel.conversations.isEmpty) {
            refreshingSkeletonView
        } else if let err = viewModel.errorMessage, viewModel.conversations.isEmpty {
            errorView(err)
        } else if viewModel.selectedWorkspaceFilter == nil && viewModel.filteredConversations.isEmpty && unmatchedContentHits.isEmpty && !viewModel.isSearchingContent {
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
                Text(isPairingInProgress ? String(localized: "正在连接网关并同步会话列表...") : String(localized: "正在同步会话列表..."))
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
        let hasMultiple = !settings.isDemoMode && gatewayStore.hasMultipleGateways
        let activeHostName = gatewayStore.activeProfile?.resolvedDisplayName ?? ""
        return VStack(spacing: 14) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.system(size: 44))
                .foregroundColor(.orange)
            Text(hasMultiple && !activeHostName.isEmpty
                 ? String(localized: "无法连接「\(activeHostName)」")
                 : String(localized: "无法连接网关"))
                .font(.system(size: 17, weight: .semibold))
                .multilineTextAlignment(.center)
                .padding(.horizontal, 24)
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
                
                HStack(spacing: 20) {
                    if hasMultiple {
                        Button("切换其他主机") {
                            let containerW = layoutGeometry.width > 0 ? layoutGeometry.width : 390
                            let containerH = layoutGeometry.height > 0 ? layoutGeometry.height : 844
                            let anchorCenterX = usesSplitLayout ? (splitColumnMetrics.sidebarWidth / 2) : (containerW / 2)
                            let anchor = CGRect(x: anchorCenterX - 80, y: 104, width: 160, height: 32)
                            gatewayPickerState.openInteractive(
                                anchorRect: anchor,
                                containerSize: CGSize(width: containerW, height: containerH),
                                centered: true
                            )
                        }
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.indigo)
                    }

                    Button("打开设置") {
                        showSettings = true
                    }
                    .font(.system(size: 14))
                    .foregroundColor(.secondary)
                }
                .padding(.top, 4)
            }
            .padding(.top, 6)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
    
    private var emptyView: some View {
        ContentUnavailableView {
            Label(
                viewModel.searchQuery.isEmpty ? String(localized: "暂无会话") : String(localized: "未找到匹配会话"),
                systemImage: viewModel.searchQuery.isEmpty ? "bubble.left.and.bubble.right" : "magnifyingglass"
            )
        } description: {
            Text(viewModel.searchQuery.isEmpty
                 ? String(localized: "可点击右上角 + 开启新会话，或下拉刷新同步")
                 : String(localized: "请尝试其他关键词搜索"))
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
            if viewModel.quotaResponse?.currentAccount?.resolvedQuotaDisplay(preferred: AppSettings.shared.selectedQuotaBucket) != nil {
                QuotaStatusBarView(account: viewModel.quotaResponse?.currentAccount, linkMonitor: viewModel.linkMonitor) {
                    showAccountQuota = true
                }
                .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 6, trailing: 16))
                .listRowSeparator(.hidden)
                .listRowBackground(Color.clear)
            }

            if !viewModel.globalBackgroundTasks.isEmpty {
                globalBackgroundTasksCapsule
                    .listRowInsets(EdgeInsets(top: 2, leading: 16, bottom: 6, trailing: 16))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
                    .transition(.opacity.combined(with: .scale(scale: 0.94, anchor: .top)).combined(with: .move(edge: .top)))
            }

            if let activeFilter = viewModel.selectedWorkspaceFilter {
                workspaceFilterCapsule(for: activeFilter)
                    .listRowInsets(EdgeInsets(top: 2, leading: 16, bottom: 6, trailing: 16))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
                    .transition(.opacity.combined(with: .scale(scale: 0.96, anchor: .top)).combined(with: .move(edge: .top)))

                if viewModel.hiddenPendingApprovalCount > 0 {
                    hiddenPendingApprovalCapsule(count: viewModel.hiddenPendingApprovalCount)
                        .listRowInsets(EdgeInsets(top: 0, leading: 16, bottom: 6, trailing: 16))
                        .listRowSeparator(.hidden)
                        .listRowBackground(Color.clear)
                        .transition(.opacity.combined(with: .scale(scale: 0.96, anchor: .top)).combined(with: .move(edge: .top)))
                }
            }

            if let activeFilter = viewModel.selectedWorkspaceFilter,
               !viewModel.isAutoExitingArchive,
               viewModel.filteredConversations.isEmpty && unmatchedContentHits.isEmpty && !viewModel.isSearchingContent {
                VStack(spacing: 10) {
                    Image(systemName: viewModel.searchQuery.isEmpty ? (activeFilter.isArchived ? "archivebox" : (activeFilter.isStarred ? "star" : (activeFilter.isPureChat ? "bubble.left" : "folder"))) : "magnifyingglass")
                        .font(.system(size: 28))
                        .foregroundColor(.secondary)
                    Text(viewModel.searchQuery.isEmpty
                         ? String(localized: "「\(activeFilter.displayName)」暂无会话")
                         : String(localized: "在「\(activeFilter.displayName)」中未找到匹配会话"))
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.secondary)
                    Button("查看全部会话") {
                        clearWorkspaceFilter()
                    }
                    .font(.system(size: 13.5, weight: .semibold))
                    .buttonStyle(.bordered)
                    .padding(.top, 2)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 44)
                .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                .listRowSeparator(.hidden)
                .listRowBackground(Color.clear)
            }
            
            let displayRows = makeDisplayRows()
            let firstConversationId = viewModel.filteredConversations.first?.id
            ForEach(displayRows) { row in
                switch row {
                case .branchDrawer(let parentId, let branches, let isExpanded, let aggregateStatus):
                    applyConversationConfirmationDialogs(
                        to: branchGroupDrawer(
                            parentId: parentId,
                            branches: branches,
                            isExpanded: isExpanded,
                            aggregateStatus: aggregateStatus
                        ),
                        matches: { id in branches.contains(where: { $0.id == id }) }
                    )
                    .listRowInsets(EdgeInsets(
                        top: 0,
                        leading: 28,
                        bottom: 6,
                        trailing: 28
                    ))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)

                case .conversation(let item, let hasAttachedBranches):
                    applyConversationConfirmationDialogs(
                        to: conversationCard(for: item)
                            .offset(x: item.id == firstConversationId ? firstCardPeekOffset : 0)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                swipedBranchChildId = nil
                                openConversation(item)
                            }
                            .modifier(RowActions(
                                isSplit: usesSplitLayout,
                                item: item,
                                onRename: {
                                    conversationToRename = item
                                    renameText = item.title
                                    showRenameAlert = true
                                },
                                onToggleStar: {
                                    viewModel.toggleStar(item: item)
                                },
                                onArchive: {
                                    conversationToArchive = item
                                    showArchiveConfirm = true
                                },
                                onDelete: {
                                    conversationToDelete = item
                                    showDeleteConfirm = true
                                }
                            )),
                        matches: { id in id == item.id }
                    )
                    .listRowInsets(EdgeInsets(
                        top: 6,
                        leading: 16,
                        bottom: hasAttachedBranches ? 0 : 6,
                        trailing: 16
                    ))
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
                    .swipeActions(edge: .leading, allowsFullSwipe: false) {
                        Button {
                            UserDefaults.standard.set(true, forKey: "has_seen_swipe_gesture_hint")
                            conversationToPin = item
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                                showPinConfirm = true
                            }
                        } label: {
                            Image(uiImage: Self.pinActionImage(isPinned: item.isPinned))
                        }
                        .tint(.clear)
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button {
                            UserDefaults.standard.set(true, forKey: "has_seen_swipe_gesture_hint")
                            conversationToDelete = item
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                                showDeleteConfirm = true
                            }
                        } label: {
                            Image(uiImage: Self.trashActionImage)
                        }
                        .tint(.clear)

                        if !item.isDraft {
                            Button {
                                UserDefaults.standard.set(true, forKey: "has_seen_swipe_gesture_hint")
                                conversationToArchive = item
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                                    showArchiveConfirm = true
                                }
                            } label: {
                                Image(uiImage: Self.archiveActionImage(isArchived: item.isArchived))
                            }
                            .tint(.clear)
                        }
                    }
                }
            }
            .animation(.spring(response: 0.34, dampingFraction: 0.86), value: displayRows.map(\.id))

            if viewModel.selectedWorkspaceFilter == nil,
               viewModel.searchQuery.trimmingCharacters(in: .whitespaces).isEmpty,
               viewModel.archivedCount > 0 {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    selectWorkspaceFilter(.archived)
                } label: {
                    HStack(spacing: 6) {
                        Image(systemName: "archivebox")
                            .font(.system(size: 12, weight: .medium))
                        Text(String(localized: "已归档会话 (\(viewModel.archivedCount))"))
                            .font(.system(size: 12.5, weight: .medium))
                        Image(systemName: "chevron.right")
                            .font(.system(size: 10, weight: .semibold))
                    }
                    .foregroundColor(.secondary)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                }
                .buttonStyle(.plain)
                .listRowInsets(EdgeInsets(top: 4, leading: 16, bottom: 10, trailing: 16))
                .listRowSeparator(.hidden)
                .listRowBackground(Color.clear)
            }

            // 对话内容命中（language_server 全文搜索），与上面的标题/工作区匹配并列
            if !viewModel.searchQuery.trimmingCharacters(in: .whitespaces).isEmpty,
               !unmatchedContentHits.isEmpty || viewModel.isSearchingContent {
                Section {
                    ForEach(unmatchedContentHits) { hit in
                        contentHitRow(hit)
                            .contentShape(Rectangle())
                            .onTapGesture { openConversation(hit.item, includeSideQuestionParent: true) }
                            .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                            .listRowSeparator(.hidden)
                            .listRowBackground(Color.clear)
                    }
                } header: {
                    HStack(spacing: 8) {
                        Text("对话内容匹配")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.secondary)
                        if viewModel.isSearchingContent {
                            ProgressView().scaleEffect(0.6)
                        }
                    }
                }
            }
        }
        .listStyle(.plain)
        .listRowSpacing(0)
        .environment(\.defaultMinListRowHeight, 0)
        .animation(.spring(response: 0.42, dampingFraction: 0.72), value: viewModel.globalBackgroundTasks.isEmpty)
        .animation(.spring(response: 0.34, dampingFraction: 0.86), value: viewModel.selectedWorkspaceFilter)
        .refreshable {
            await viewModel.pullToRefresh()
        }
    }

    /// 内容命中里去掉已经按标题/工作区出现在上面列表中的会话，同时遵守当前项目筛选。
    private var unmatchedContentHits: [ConversationListViewModel.ContentSearchHit] {
        let shown = Set(viewModel.filteredConversations.map(\.id))
        let activeFilter = viewModel.selectedWorkspaceFilter
        return viewModel.contentHits.filter { hit in
            !shown.contains(hit.item.id) && (activeFilter?.matches(hit.item) ?? true)
        }
    }

    private func contentHitRow(_ hit: ConversationListViewModel.ContentSearchHit) -> some View {
        var highlight = AttributeContainer()
        highlight.backgroundColor = Color.orange.opacity(0.28)
        let snippet = SearchSnippetHighlighter.attributed(
            snippet: hit.result.snippet,
            ranges: hit.result.snippetMatchRanges,
            highlight: highlight
        )
        return VStack(alignment: .leading, spacing: 6) {
            Text(hit.item.title.isEmpty ? String(localized: "未命名会话") : hit.item.title)
                .font(.system(size: 15, weight: .semibold))
                .foregroundColor(.primary)
                .lineLimit(1)
            if !hit.result.snippet.isEmpty {
                Text(snippet)
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .lineLimit(3)
            }
            let isSide = hit.result.isSideQuestion == true || hit.item.isSideQuestion
            let displayWs = hit.item.isPureChat ? String(localized: "Chat") : hit.item.workspaceName
            let metaText: String = {
                if isSide && !displayWs.isEmpty {
                    return "↳ /btw · \(displayWs)"
                } else if isSide {
                    return "↳ /btw"
                } else {
                    return displayWs
                }
            }()
            if !metaText.isEmpty {
                Text(metaText)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary.opacity(0.8))
                    .lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private func handleListAppear() {
        viewModel.reloadFromCache()
        let openCount = UserDefaults.standard.integer(forKey: "conversation_list_open_count") + 1
        UserDefaults.standard.set(openCount, forKey: "conversation_list_open_count")
        let hasSeenSwipe = UserDefaults.standard.bool(forKey: "has_seen_swipe_gesture_hint")
        if openCount >= 2 && !hasSeenSwipe {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.7) {
                guard !viewModel.filteredConversations.isEmpty else { return }
                withAnimation(.spring(response: 0.36, dampingFraction: 0.72)) {
                    firstCardPeekOffset = -24
                }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.38) {
                    withAnimation(.spring(response: 0.42, dampingFraction: 0.76)) {
                        firstCardPeekOffset = 0
                    }
                    UserDefaults.standard.set(true, forKey: "has_seen_swipe_gesture_hint")
                }
            }
        }
        if settings.isPaired {
            ConnectionManager.shared.ensureStarted()
            viewModel.startAutoRefresh()
            Task {
                await viewModel.resumeActive()
                PushNotificationManager.shared.syncAllTokensIfPaired()
                async let projects = ProjectCacheManager.shared.fetchAndCacheProjects()
                async let mcp: Void = CacheManager.shared.fetchAndCacheMcpAndSkills()
                _ = await (projects, mcp)
            }
        }
        if let pendingRoute = PushNotificationManager.shared.consumePendingRoute() {
            handleNotificationRoute(pendingRoute)
        }
        PushNotificationManager.shared.hasBlockingModalPresented = hasBlockingModalPresented
    }

    private func handleScenePhaseChange(_ newPhase: ScenePhase) {
        if newPhase == .active {
            if AppSettings.shared.isPaired {
                ConnectionManager.shared.ensureStarted()
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
        ActivityManager.shared.reconcileActiveActivities()
        if AppSettings.shared.isPaired {
            ConnectionManager.shared.ensureStarted()
            viewModel.startAutoRefresh()
            GatewayProfileStore.shared.refreshInactiveSummaries(force: false, minInterval: 45.0)
            Task {
                await viewModel.resumeActive()
                PushNotificationManager.shared.syncAllTokensIfPaired()
                await ProjectCacheManager.shared.fetchAndCacheProjects()
            }
        }
        if let pendingRoute = PushNotificationManager.shared.consumePendingRoute() {
            handleNotificationRoute(pendingRoute)
        }
    }

    private func handleAllGatewaysCleared() {
        isPairingInProgress = false
        viewModel.stopAutoRefresh()
        viewModel.conversations.removeAll()
        viewModel.resetBranchExpansionState()
        viewModel.selectedWorkspaceFilter = nil
        viewModel.errorMessage = nil
        viewModel.isLoading = false
        chatSessionStore.clearAll()
        navigationPath = []
        showSettings = false
        CacheManager.shared.clearCache()
        DocumentCacheManager.shared.clearCache()
    }

    private func handleTokenRevoked(_ notification: Notification) {
        if let notifEpoch = notification.userInfo?["epoch"] as? UInt64,
           notifEpoch != GatewayRuntimeState.shared.epoch {
            return
        }
        let hasRemaining = settings.unpairActiveProfile()
        if !hasRemaining {
            handleAllGatewaysCleared()
        }
    }
    
    private func openAddGatewayFlow() {
        if AVCaptureDevice.authorizationStatus(for: .video) == .restricted {
            showManualInput = true
        } else {
            showQRScanner = true
        }
    }

    private var pendingPairingConfirmText: String {
        guard let info = pendingPairing else {
            return String(localized: "请确认这是你自己的电脑网关，不要配对来历不明的链接。")
        }
        let scheme = info.ssl ? "HTTPS" : "HTTP"
        let gatewayName = info.gatewayDisplayName
        return String(localized: "目标 \(info.host):\(info.port)（\(scheme)）。请确认这是你自己的 \(gatewayName)，不要配对来历不明的链接。")
    }
    
    @MainActor
    private func startPairing(info: PairingInfo) {
        let hadExistingGateway = settings.isPaired && !settings.isDemoMode && GatewayProfileStore.shared.activeProfile != nil
        showQRScanner = false
        pendingPairing = nil
        viewModel.errorMessage = nil
        isPairingInProgress = true
        
        Task {
            do {
                _ = try await PairingService.shared.pair(with: info)
                settings.refreshPairedState()
                AppReviewManager.shared.recordActiveDayIfEligible()
                viewModel.startAutoRefresh()
                await viewModel.fetchConversations()
                await ProjectCacheManager.shared.fetchAndCacheProjects()
                isPairingInProgress = false
            } catch {
                isPairingInProgress = false
                if !hadExistingGateway {
                    settings.unpair()
                }
                pairingErrorMessage = error.localizedDescription
            }
        }
    }
    
    @MainActor
    private func confirmPendingPairing() async {
        guard let info = pendingPairing else { return }
        startPairing(info: info)
    }
    
    /// A document handed over by another app ("用其他应用打开" / share sheet "open in").
    private func handleSharedFile(_ url: URL) {
        guard settings.isPaired else {
            try? FileManager.default.removeItem(at: url)
            pairingErrorMessage = String(localized: "请先完成配对，再分享文件到 Multigravity")
            return
        }
        shareInbox.receive([url])
    }
    
    private func handleDeepLink(_ url: URL) {
        guard url.scheme == "antigravity" || url.scheme == "agy" || url.scheme == "multigravity" else { return }
        let urlComponents = URLComponents(url: url, resolvingAgainstBaseURL: false)
        let gwParam = urlComponents?.queryItems?.first(where: { $0.name == "gw" || $0.name == "gatewayId" })?.value
        
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

        // Handle antigravity://quota deep link (from Lock Screen / Home Screen / CarPlay Quota Widget / Quota Alert)
        if url.host == "quota" {
            switchGatewayIfNeeded(gatewayId: gwParam)
            openAccountQuotaSheet()
            return
        }

        // Handle Home Screen Quick Actions & Shortcuts URLs (new-chat / new-workspace / camera / scan)
        if let host = url.host, let quickAction = HomeScreenQuickAction(urlHost: host) {
            handleQuickAction(quickAction)
            return
        }
        
        // Handle antigravity://conversations or antigravity://home deep link (from Agent Status Widget)
        if url.host == "conversations" || url.host == "home" {
            switchGatewayIfNeeded(gatewayId: gwParam)
            dismissAllModals()
            navigationPath = []
            return
        }

        // Handle recording action deep link (e.g. from Dynamic Island / Lock Screen Live Activity)
        if url.host == "recording" {
            let action = urlComponents?.queryItems?.first(where: { $0.name == "action" })?.value ?? "stop"
            let rawCid = urlComponents?.queryItems?.first(where: { $0.name == "cascadeId" })?.value ?? ""
            let cascadeId = CacheManager.isValidCascadeId(rawCid) ? rawCid : ""
            let sendImmediately = (action == "send")
            
            _ = AudioRecorderManager.shared.finishRecordingFromLiveActivity(
                sendImmediately: sendImmediately,
                targetCascadeId: cascadeId
            )
            
            if !cascadeId.isEmpty {
                switchGatewayIfNeeded(gatewayId: gwParam, fallbackCascadeId: cascadeId)
                openConversationById(cascadeId)
            } else {
                dismissAllModals()
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
        
        guard CacheManager.isValidCascadeId(cascadeId) else { return }
        switchGatewayIfNeeded(gatewayId: gwParam, fallbackCascadeId: cascadeId)
        openConversationById(cascadeId)
    }

    private func handleQuickAction(_ action: HomeScreenQuickAction) {
        guard !isPairingInProgress else { return }
        guard settings.isPaired else {
            if showQRScanner { return }
            let hadOtherModal = dismissAllModals()
            if hadOtherModal {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.38) {
                    guard !settings.isPaired, !isPairingInProgress else { return }
                    showQuickActionUnpairedAlert = true
                }
            } else {
                showQuickActionUnpairedAlert = true
            }
            return
        }

        switch action {
        case .newChat:
            dismissAllModals()
            let session = CacheManager.shared.createLocalDraftSession(project: .pureChat)
            navigationPath = [session.toConversationItem()]

        case .newWorkspace:
            let hadOtherModal = dismissAllModals(excludingNewConversation: true)
            if showNewConversation { return }
            if hadOtherModal {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.38) {
                    guard settings.isPaired else { return }
                    showNewConversation = true
                }
            } else {
                showNewConversation = true
            }

        case .camera, .scan:
            let hadOtherModal = dismissAllModals()
            let launchAction: HomeScreenQuickAction.ChatLaunchAction = (action == .camera) ? .camera : .documentScanner
            let openCaptureChat = {
                guard settings.isPaired else { return }
                let session = CacheManager.shared.createLocalDraftSession(project: .pureChat)
                let item = session.toConversationItem()
                pendingChatLaunchActions[item.id] = launchAction
                navigationPath = [item]
            }
            if hadOtherModal {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.38) {
                    openCaptureChat()
                }
            } else {
                openCaptureChat()
            }
        }
    }
    
    private enum BranchAggregateStatus: Equatable {
        case action
        case error
        case working
        case draft
        case unread
        case none
    }

    private enum ConversationListRow: Identifiable {
        case conversation(ConversationItem, hasAttachedBranches: Bool)
        case branchDrawer(parentId: String, branches: [ConversationItem], isExpanded: Bool, aggregateStatus: BranchAggregateStatus)

        var id: String {
            switch self {
            case .conversation(let item, _):
                return item.id
            case .branchDrawer(let parentId, _, _, _):
                return "btw-drawer-\(parentId)"
            }
        }
    }

    @ViewBuilder
    private func applyConversationConfirmationDialogs<Content: View>(
        to content: Content,
        matches: @escaping (String?) -> Bool
    ) -> some View {
        content
            .confirmationDialog(
                (conversationToPin?.isPinned == true)
                    ? String(localized: "确定取消置顶此会话吗？")
                    : String(localized: "确定置顶此会话吗？\n置顶后该会话将固定显示在会话列表顶部。"),
                isPresented: Binding(
                    get: { showPinConfirm && matches(conversationToPin?.id) },
                    set: { if !$0 {
                        showPinConfirm = false
                        conversationToPin = nil
                    } }
                ),
                titleVisibility: .visible
            ) {
                Button((conversationToPin?.isPinned == true) ? String(localized: "取消置顶") : String(localized: "置顶会话")) {
                    showPinConfirm = false
                    if let target = conversationToPin {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        viewModel.togglePin(item: target)
                    }
                    conversationToPin = nil
                }
                Button("取消", role: .cancel) {
                    showPinConfirm = false
                    conversationToPin = nil
                }
            }
            .confirmationDialog(
                (conversationToArchive?.isArchived == true)
                    ? String(localized: "确定取消归档此会话吗？")
                    : String(localized: "确定归档此会话吗？\n归档后该会话将移入「已归档」，随时可查看或恢复。"),
                isPresented: Binding(
                    get: { showArchiveConfirm && matches(conversationToArchive?.id) },
                    set: { if !$0 {
                        showArchiveConfirm = false
                        conversationToArchive = nil
                    } }
                ),
                titleVisibility: .visible
            ) {
                Button((conversationToArchive?.isArchived == true) ? String(localized: "取消归档") : String(localized: "归档会话")) {
                    showArchiveConfirm = false
                    if let target = conversationToArchive {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        viewModel.toggleArchive(item: target)
                    }
                    conversationToArchive = nil
                }
                Button("取消", role: .cancel) {
                    showArchiveConfirm = false
                    conversationToArchive = nil
                }
            }
            .confirmationDialog(
                (conversationToDelete?.isDraft == true)
                    ? String(localized: "确定删除此草稿会话吗？\n此操作将删除本地临时会话且无法撤销。")
                    : String(localized: "确定删除此会话吗？\n此操作将永久删除会话记录且无法撤销。"),
                isPresented: Binding(
                    get: { showDeleteConfirm && matches(conversationToDelete?.id) },
                    set: { if !$0 {
                        showDeleteConfirm = false
                        conversationToDelete = nil
                    } }
                ),
                titleVisibility: .visible
            ) {
                Button("删除", role: .destructive) {
                    showDeleteConfirm = false
                    if let target = conversationToDelete {
                        conversationToDelete = nil
                        if navigationPath.last?.id == target.id { navigationPath = [] }
                        Task {
                            await viewModel.deleteConversation(item: target)
                        }
                    }
                }
                Button("取消", role: .cancel) {
                    showDeleteConfirm = false
                    conversationToDelete = nil
                }
            }
    }

    private func aggregateBranchStatus(for branches: [ConversationItem]) -> BranchAggregateStatus {
        if branches.contains(where: { $0.status.needsAction }) { return .action }
        if branches.contains(where: { $0.status.isError }) { return .error }
        if branches.contains(where: { $0.isWorking }) { return .working }
        if branches.contains(where: { $0.isDraft || CacheManager.shared.hasDraft(for: $0.id) }) { return .draft }
        if branches.contains(where: { $0.isUnread }) { return .unread }
        return .none
    }

    private func makeDisplayRows() -> [ConversationListRow] {
        let _ = draftsVersion
        let groups = ConversationListViewModel.groupConversationsWithBranches(viewModel.filteredConversations)
        var rows: [ConversationListRow] = []
        rows.reserveCapacity(groups.count * 2)
        for group in groups {
            let hasBranches = !group.branches.isEmpty
            rows.append(.conversation(
                group.root,
                hasAttachedBranches: hasBranches
            ))
            if hasBranches {
                let agg = aggregateBranchStatus(for: group.branches)
                let isExpanded = viewModel.isBranchGroupExpanded(
                    parentId: group.root.id,
                    hasAction: agg == .action
                )
                rows.append(.branchDrawer(
                    parentId: group.root.id,
                    branches: group.branches,
                    isExpanded: isExpanded,
                    aggregateStatus: agg
                ))
            }
        }
        return rows
    }

    private func branchGroupDrawer(
        parentId: String,
        branches: [ConversationItem],
        isExpanded: Bool,
        aggregateStatus: BranchAggregateStatus
    ) -> some View {
        let _ = draftsVersion
        let countLabel = branches.count == 1
            ? String(localized: "1 个旁路")
            : String(localized: "\(branches.count) 个旁路")
        let previewTitle = branches.first?.title.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let drawerShape = UnevenRoundedRectangle(
            topLeadingRadius: 0,
            bottomLeadingRadius: 12,
            bottomTrailingRadius: 12,
            topTrailingRadius: 0,
            style: .continuous
        )
        let trayBackground = colorScheme == .dark
            ? Color(red: 0.095, green: 0.095, blue: 0.112)
            : Color(red: 0.922, green: 0.925, blue: 0.955)
        // Opaque pre-blended connector tone eliminates any alpha double-compositing at T-junctions
        let connectorColor = colorScheme == .dark
            ? Color(red: 0.23, green: 0.23, blue: 0.45)
            : Color(red: 0.72, green: 0.72, blue: 0.90)

        return VStack(spacing: 0) {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                swipedBranchChildId = nil
                withAnimation(.snappy(duration: 0.28, extraBounce: 0)) {
                    viewModel.toggleBranchGroup(parentId: parentId, hasAction: aggregateStatus == .action)
                }
            } label: {
                HStack(alignment: .center, spacing: 5) {
                    Image(systemName: "arrow.triangle.branch")
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(.indigo)
                        .frame(width: 12, height: 12)

                    Text(countLabel)
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.indigo)
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)

                    if !isExpanded && !previewTitle.isEmpty {
                        Text("·")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(.secondary.opacity(0.75))
                        Text(previewTitle)
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .truncationMode(.tail)
                    }

                    Spacer(minLength: 4)

                    ZStack(alignment: .trailing) {
                        Group {
                            switch aggregateStatus {
                            case .action:
                                Text("Action")
                                    .font(.system(size: 10, weight: .bold))
                                    .lineLimit(1)
                                    .fixedSize(horizontal: true, vertical: false)
                                    .foregroundColor(.blue)
                                    .padding(.horizontal, 5.5)
                                    .padding(.vertical, 1.5)
                                    .background(Color.blue.opacity(0.15))
                                    .cornerRadius(5)
                            case .error:
                                Text("Error")
                                    .font(.system(size: 10, weight: .bold))
                                    .lineLimit(1)
                                    .fixedSize(horizontal: true, vertical: false)
                                    .foregroundColor(.red)
                                    .padding(.horizontal, 5.5)
                                    .padding(.vertical, 1.5)
                                    .background(Color.red.opacity(0.15))
                                    .cornerRadius(5)
                            case .working:
                                HStack(spacing: 3.5) {
                                    ProgressView()
                                        .controlSize(.mini)
                                        .tint(.green)
                                    Text("Working")
                                        .lineLimit(1)
                                }
                                .fixedSize(horizontal: true, vertical: false)
                                .font(.system(size: 10, weight: .bold))
                                .foregroundColor(.green)
                                .padding(.horizontal, 5.5)
                                .padding(.vertical, 1.5)
                                .background(Color.green.opacity(0.15))
                                .cornerRadius(5)
                            case .draft:
                                Text("DRAFT")
                                    .font(.system(size: 9.5, weight: .bold))
                                    .lineLimit(1)
                                    .fixedSize(horizontal: true, vertical: false)
                                    .foregroundColor(.yellow)
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 1.5)
                                    .background(Color.yellow.opacity(0.15))
                                    .cornerRadius(5)
                            case .unread:
                                ZStack {
                                    Circle()
                                        .fill(Color.blue.opacity(0.16))
                                        .frame(width: 12, height: 12)
                                    Circle()
                                        .fill(Color.blue)
                                        .frame(width: 5.5, height: 5.5)
                                }
                                .frame(width: 14, height: 14)
                            case .none:
                                EmptyView()
                            }
                        }
                        .opacity(isExpanded ? 0 : 1)
                        .animation(.easeOut(duration: 0.12), value: isExpanded)

                        Text("收起")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                            .opacity(isExpanded ? 1 : 0)
                            .animation(.easeIn(duration: 0.14), value: isExpanded)
                    }

                    Image(systemName: "chevron.down")
                        .font(.system(size: 9.5, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.85))
                        .rotationEffect(.degrees(isExpanded ? 180 : 0))
                        .animation(.snappy(duration: 0.26, extraBounce: 0), value: isExpanded)
                }
                .padding(.horizontal, 10)
                .frame(maxWidth: .infinity)
                .frame(height: 34)
                .overlay(alignment: .bottomLeading) {
                    Rectangle()
                        .fill(connectorColor)
                        .frame(width: 1.5, height: 6.5)
                        .offset(x: 15.25, y: 0)
                        .opacity(isExpanded ? 1 : 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            VerticalRevealLayout(progress: isExpanded ? 1 : 0) {
                VStack(spacing: 0) {
                    ForEach(Array(branches.enumerated()), id: \.element.id) { idx, child in
                        let isLast = idx == branches.count - 1
                        let bottomPad: CGFloat = isLast ? 8 : 6
                        let childHasDraft = child.isDraft || CacheManager.shared.hasDraft(for: child.id)

                        BranchChildSwipeRow(
                            item: child,
                            hasDraft: childHasDraft,
                            isSelected: usesSplitLayout && selectedConversationID == child.id,
                            isSwipedOpen: swipedBranchChildId == child.id,
                            onSwipeStateChanged: { isOpen in
                                if isOpen {
                                    swipedBranchChildId = child.id
                                } else if swipedBranchChildId == child.id {
                                    swipedBranchChildId = nil
                                }
                            },
                            onWillBeginSwipe: {
                                if swipedBranchChildId != nil && swipedBranchChildId != child.id {
                                    swipedBranchChildId = nil
                                }
                            },
                            onTap: {
                                if swipedBranchChildId != nil {
                                    swipedBranchChildId = nil
                                } else {
                                    openConversation(child)
                                }
                            },
                            onPin: {
                                swipedBranchChildId = nil
                                conversationToPin = child
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                    showPinConfirm = true
                                }
                            },
                            onRename: {
                                swipedBranchChildId = nil
                                conversationToRename = child
                                renameText = child.title
                                showRenameAlert = true
                            },
                            onToggleStar: {
                                swipedBranchChildId = nil
                                viewModel.toggleStar(item: child)
                            },
                            onArchive: {
                                swipedBranchChildId = nil
                                conversationToArchive = child
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                    showArchiveConfirm = true
                                }
                            },
                            onDelete: {
                                swipedBranchChildId = nil
                                conversationToDelete = child
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                    showDeleteConfirm = true
                                }
                            },
                            isSplit: usesSplitLayout,
                            statusBadge: {
                                sideQuestionStatusBadge(item: child, hasDraft: childHasDraft)
                            }
                        )
                        .padding(.leading, 28)
                        .padding(.trailing, 8)
                        .padding(.bottom, bottomPad)
                        .background(alignment: .leading) {
                            BranchInternalTreeConnector(
                                continuesBelow: !isLast,
                                bottomPadding: bottomPad,
                                spineX: 16,
                                endX: 27,
                                cornerRadius: 6
                            )
                            .stroke(
                                connectorColor,
                                style: StrokeStyle(lineWidth: 1.5, lineCap: .round, lineJoin: .round)
                            )
                            .frame(width: 28)
                        }
                    }
                }
                .opacity(isExpanded ? 1 : 0)
            }
            .allowsHitTesting(isExpanded)
            .accessibilityHidden(!isExpanded)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(trayBackground)
        .overlay(alignment: .top) {
            VStack(spacing: 0) {
                Rectangle()
                    .fill(colorScheme == .dark ? Color.white.opacity(0.08) : Color.black.opacity(0.07))
                    .frame(height: 0.5)
                LinearGradient(
                    colors: [
                        Color.black.opacity(colorScheme == .dark ? 0.28 : 0.055),
                        Color.clear
                    ],
                    startPoint: .top,
                    endPoint: .bottom
                )
                .frame(height: 3.5)
            }
            .allowsHitTesting(false)
        }
        .overlay {
            DrawerTraySideBottomBorder(bottomRadius: 12)
                .stroke(
                    Color.indigo.opacity(colorScheme == .dark ? 0.24 : 0.17),
                    lineWidth: 0.75
                )
                .allowsHitTesting(false)
        }
        .clipShape(drawerShape)
    }

    @ViewBuilder
    private func conversationCard(for item: ConversationItem) -> some View {
        let _ = draftsVersion
        let hasDraft = item.isDraft || CacheManager.shared.hasDraft(for: item.id)
        let isRecording = audioRecorder.isRecording && audioRecorder.currentCascadeId == item.id

        if item.isSideQuestion {
            sideQuestionStandaloneCard(item: item, hasDraft: hasDraft)
        } else {
            standardConversationCard(for: item, hasDraft: hasDraft, isRecording: isRecording)
        }
    }

    @ViewBuilder
    private func sideQuestionStatusBadge(item: ConversationItem, hasDraft: Bool) -> some View {
        if item.status.needsAction {
            Text("Action")
                .font(.system(size: 10.5, weight: .bold))
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .foregroundColor(.blue)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(Color.blue.opacity(0.15))
                .cornerRadius(5)
        } else if item.status.isError {
            Text("Error")
                .font(.system(size: 10.5, weight: .bold))
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .foregroundColor(.red)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(Color.red.opacity(0.15))
                .cornerRadius(5)
        } else if item.isWorking {
            HStack(spacing: 4) {
                ProgressView()
                    .controlSize(.mini)
                    .tint(.green)
                Text("Working")
                    .lineLimit(1)
            }
            .fixedSize(horizontal: true, vertical: false)
            .font(.system(size: 10.5, weight: .bold))
            .foregroundColor(.green)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(Color.green.opacity(0.15))
            .cornerRadius(5)
        } else if hasDraft {
            Text("DRAFT")
                .font(.system(size: 10, weight: .bold))
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .foregroundColor(.yellow)
                .padding(.horizontal, 5)
                .padding(.vertical, 2)
                .background(Color.yellow.opacity(0.15))
                .cornerRadius(5)
        } else if item.isUnread {
            ZStack {
                Circle()
                    .fill(Color.blue.opacity(0.16))
                    .frame(width: 12, height: 12)
                Circle()
                    .fill(Color.blue)
                    .frame(width: 5.5, height: 5.5)
            }
            .frame(width: 14, height: 14)
        }
    }

    private func sideQuestionStandaloneCard(
        item: ConversationItem,
        hasDraft: Bool
    ) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .center, spacing: 6) {
                Image(systemName: "arrow.triangle.branch")
                    .font(.system(size: 11.5, weight: .semibold))
                    .foregroundColor(.indigo)

                Text("旁路")
                    .font(.system(size: 10, weight: .semibold))
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
                    .foregroundColor(.indigo)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 1.5)
                    .background(Color.indigo.opacity(0.12))
                    .clipShape(Capsule())

                Text(item.title)
                    .font(.system(size: 14, weight: .semibold))
                    .lineLimit(1)
                    .truncationMode(.tail)
                    .foregroundColor(.primary)

                Spacer(minLength: 4)

                sideQuestionStatusBadge(item: item, hasDraft: hasDraft)
            }

            HStack(spacing: 6) {
                if let parentTitle = item.branchParentTitle?.trimmingCharacters(in: .whitespacesAndNewlines), !parentTitle.isEmpty {
                    Text("来自「\(parentTitle)」")
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                } else {
                    Text(item.isPureChat ? String(localized: "Chat") : item.workspaceName)
                        .font(.system(size: 11.5, design: item.isPureChat ? .default : .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }

                Spacer(minLength: 4)

                if !item.relativeTimeString.isEmpty {
                    Text(item.relativeTimeString)
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemBackground).opacity(0.85))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.indigo.opacity(0.20), lineWidth: 0.8)
        )
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay {
            if usesSplitLayout && selectedConversationID == item.id {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(Color.accentColor.opacity(0.10))
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(Color.accentColor.opacity(0.55), lineWidth: 1.5)
            }
        }
        .contentShape(.hoverEffect, RoundedRectangle(cornerRadius: 12, style: .continuous))
        .hoverEffect(.highlight)
    }

    private func standardConversationCard(for item: ConversationItem, hasDraft: Bool, isRecording: Bool) -> some View {
        let indent = CGFloat(min(item.nestingDepth, 3) * 14)
        
        return VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top, spacing: 6) {
                if item.isPinned {
                    Image(systemName: "pin.fill")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.orange)
                        .padding(.top, 3)
                }

                if item.isStarred {
                    Image(systemName: "star.fill")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.yellow)
                        .padding(.top, 3)
                }
                
                if item.nestingDepth > 0 || (item.parentConversationId != nil && !item.parentConversationId!.isEmpty) {
                    Image(systemName: "arrow.turn.down.right")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.indigo)
                        .padding(.top, 2)
                }

                if item.hasSwarm {
                    SwarmPixelBeeSpriteView(
                        name: "coordinator",
                        pose: item.isSwarmWorking ? .working : .landed,
                        width: 22,
                        height: 18
                    )
                    .fixedSize()
                    .padding(.top, 0.5)
                    .accessibilityLabel(String(localized: "蜂群会话"))
                }
                
                Text(item.title)
                    .font(.system(size: 15.5, weight: .semibold))
                    .lineLimit(1)
                    .truncationMode(.tail)
                    .foregroundColor(.primary)
                
                Spacer(minLength: 4)
                
                if item.status.needsAction {
                    Text("Action required")
                        .font(.system(size: 11, weight: .bold))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                        .layoutPriority(1)
                        .foregroundColor(.blue)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.blue.opacity(0.15))
                        .cornerRadius(6)
                } else if item.status.isError {
                    Text("Error")
                        .font(.system(size: 11, weight: .bold))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                        .layoutPriority(1)
                        .foregroundColor(.red)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.red.opacity(0.15))
                        .cornerRadius(6)
                } else if item.isWorking {
                    let isTerminal = item.hasRunningTasks
                    let label = isTerminal ? String(localized: "Running command") : String(localized: "Working")
                    HStack(spacing: 4) {
                        if isTerminal {
                            TerminalRunningIndicator(size: 12, color: .green)
                        } else {
                            ProgressView()
                                .controlSize(.mini)
                                .tint(.green)
                        }
                        Text(label)
                            .lineLimit(1)
                    }
                    .fixedSize(horizontal: true, vertical: false)
                    .layoutPriority(1)
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(.green)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .background(Color.green.opacity(0.15))
                    .cornerRadius(6)
                } else if isRecording {
                    HStack(spacing: 4) {
                        Circle()
                            .fill(Color.red)
                            .frame(width: 6, height: 6)
                        Text("录音中")
                            .lineLimit(1)
                    }
                    .fixedSize(horizontal: true, vertical: false)
                    .layoutPriority(1)
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(.red)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .background(Color.red.opacity(0.15))
                    .cornerRadius(6)
                } else if hasDraft {
                    Text("DRAFT")
                        .font(.system(size: 11, weight: .bold))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                        .layoutPriority(1)
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
                    let displayWs = item.isPureChat ? String(localized: "Chat") : item.workspaceName
                    Button {
                        toggleWorkspaceFilter(for: item)
                    } label: {
                        Image(systemName: item.isPureChat ? "bubble.left" : "folder")
                            .font(.system(size: item.isPureChat ? 10.5 : 11))
                            .frame(width: 15, height: 14)
                            .padding(.vertical, 6)
                            .padding(.horizontal, 5)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.borderless)
                    .padding(.vertical, -6)
                    .padding(.horizontal, -5)
                    .accessibilityLabel(String(localized: "筛选 \(displayWs) 的会话"))

                    Text(displayWs)
                        .font(.system(size: 12, design: item.isPureChat ? .default : .monospaced))
                        .lineLimit(1)
                        .allowsHitTesting(false)

                    if let pr = item.pullRequest {
                        GitHubPRPillBadge(item: pr, compact: true)
                    }
                }
                .foregroundColor(.secondary)
                
                Spacer(minLength: 6)
                
                if item.isDraft || (item.stepCount == 0 && hasDraft) {
                    if !item.relativeTimeString.isEmpty {
                        Text("草稿 • \(item.relativeTimeString)")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    } else {
                        Text("草稿")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                } else {
                    if !item.relativeTimeString.isEmpty {
                        Text("\(item.stepCount) 步骤 • \(item.relativeTimeString)")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    } else {
                        Text("\(item.stepCount) 步骤")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                }
            }
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemBackground))
        .cornerRadius(14)
        .padding(.leading, indent)
        .overlay {
            if usesSplitLayout && selectedConversationID == item.id {
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .fill(Color.accentColor.opacity(0.10))
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color.accentColor.opacity(0.55), lineWidth: 1.5)
            }
        }
        .contentShape(.hoverEffect, RoundedRectangle(cornerRadius: 14, style: .continuous))
        .hoverEffect(.highlight)
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

    private static func pinActionImage(isPinned: Bool) -> UIImage {
        let size: CGFloat = 40
        let circleDiameter: CGFloat = 38
        
        let format = UIGraphicsImageRendererFormat()
        format.opaque = false
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: size, height: size), format: format)
        
        let img = renderer.image { ctx in
            let circleOffset = (size - circleDiameter) / 2
            let circleRect = CGRect(x: circleOffset, y: circleOffset, width: circleDiameter, height: circleDiameter)
            if isPinned {
                UIColor.systemGray.setFill()
            } else {
                UIColor.systemOrange.setFill()
            }
            ctx.cgContext.fillEllipse(in: circleRect)
            
            let iconName = isPinned ? "pin.slash.fill" : "pin.fill"
            let config = UIImage.SymbolConfiguration(pointSize: 15, weight: .semibold)
            if let baseSymbol = UIImage(systemName: iconName, withConfiguration: config) {
                let symbol = baseSymbol.withTintColor(.white, renderingMode: .alwaysOriginal)
                let iconX = (size - symbol.size.width) / 2
                let iconY = (size - symbol.size.height) / 2
                symbol.draw(at: CGPoint(x: iconX, y: iconY))
            }
        }
        return img.withRenderingMode(.alwaysOriginal)
    }

    private static func archiveActionImage(isArchived: Bool) -> UIImage {
        let size: CGFloat = 40
        let circleDiameter: CGFloat = 38

        let format = UIGraphicsImageRendererFormat()
        format.opaque = false
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: size, height: size), format: format)

        let img = renderer.image { ctx in
            let circleOffset = (size - circleDiameter) / 2
            let circleRect = CGRect(x: circleOffset, y: circleOffset, width: circleDiameter, height: circleDiameter)
            (isArchived ? UIColor.systemIndigo : UIColor.systemSlateGrayCompatible).setFill()
            ctx.cgContext.fillEllipse(in: circleRect)

            let iconName = isArchived ? "tray.and.arrow.up.fill" : "archivebox.fill"
            let config = UIImage.SymbolConfiguration(pointSize: 15, weight: .semibold)
            if let baseSymbol = UIImage(systemName: iconName, withConfiguration: config) {
                let symbol = baseSymbol.withTintColor(.white, renderingMode: .alwaysOriginal)
                let iconX = (size - symbol.size.width) / 2
                let iconY = (size - symbol.size.height) / 2
                symbol.draw(at: CGPoint(x: iconX, y: iconY))
            }
        }
        return img.withRenderingMode(.alwaysOriginal)
    }
}

private extension UIColor {
    static var systemSlateGrayCompatible: UIColor {
        UIColor(red: 0.39, green: 0.45, blue: 0.55, alpha: 1.0)
    }
}

private enum SwipeActionAdjuster {
    static let activateOnce: Void = {
        // Center swipe action button in card's vacated area by shifting 16pt (compensating for 16pt card margin)
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
    private func agy_computeSwipeButtonOffset() -> CGFloat {
        // Find ancestor PullView
        var current: UIView? = self.superview
        while let v = current {
            // Layer 1: Check cellEdge property on pull view
            // UIRectEdge.left (2) -> leading (+16), UIRectEdge.right (8) -> trailing (-16)
            if v.responds(to: NSSelectorFromString("cellEdge")) {
                if let edge = v.value(forKey: "cellEdge") as? UInt {
                    if edge == 2 {
                        return 16
                    } else if edge == 8 {
                        return -16
                    }
                }
            }
            
            // Layer 2: Check pull view frame relative to its superview / cell
            let className = String(describing: type(of: v))
            if className.contains("PullView") {
                if let parent = v.superview {
                    let midX = v.frame.midX
                    let parentMidX = parent.bounds.midX
                    if parentMidX > 0 {
                        return midX < parentMidX ? 16 : -16
                    }
                }
            }
            
            current = v.superview
        }
        
        // Layer 3: Check window coordinate
        if let window = self.window {
            let midX = self.convert(self.bounds, to: window).midX
            let winMidX = window.bounds.midX
            if winMidX > 0 {
                return midX < winMidX ? 16 : -16
            }
        }
        
        return -16
    }

    @objc func agy_swipeDynamicButtonViewLayoutSubviews() {
        agy_swipeDynamicButtonViewLayoutSubviews()
        let targetTransform = CGAffineTransform(translationX: agy_computeSwipeButtonOffset(), y: 0)
        if self.transform != targetTransform {
            self.transform = targetTransform
        }
    }
    
    @objc func agy_swipeActionButtonLayoutSubviews() {
        agy_swipeActionButtonLayoutSubviews()
        let targetTransform = CGAffineTransform(translationX: agy_computeSwipeButtonOffset(), y: 0)
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

    var nearestViewController: UIViewController? {
        var responder: UIResponder? = self
        while let next = responder?.next {
            if let vc = next as? UIViewController {
                return vc
            }
            responder = next
        }
        return nil
    }
}

class NavigationBarTapGestureRecognizer: UITapGestureRecognizer {}
class NavigationBarLongPressGestureRecognizer: UILongPressGestureRecognizer {}
class NavigationBarHorizontalPanGestureRecognizer: UIPanGestureRecognizer {}

/// Dedicated touch target mounted directly inside the collapsed centered `[Host ▾]` principal title item.
/// On iOS 26/27 Liquid Glass navigation bars, empty/non-interactive bar areas pass touches through to the
/// underlying scroll view; hosting tap, 0.22s long-press-drag, and horizontal swipe recognizers directly
/// on an interactive UIView guarantees reliable hit-testing without interfering with expanded large titles.
private final class CollapsedHostTitleGestureView: UIView {
    var onPressChanged: ((Bool) -> Void)?

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesBegan(touches, with: event)
        onPressChanged?(true)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesEnded(touches, with: event)
        onPressChanged?(false)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesCancelled(touches, with: event)
        onPressChanged?(false)
    }
}

private struct CollapsedHostTitleGestureOverlay: UIViewRepresentable {
    let isEnabled: Bool
    let onPressChanged: (Bool) -> Void
    let onTap: (CGRect, CGSize) -> Void
    let onLongPressBegan: (CGRect, CGSize) -> Void
    let onLongPressChanged: (CGPoint) -> Void
    let onLongPressEnded: () -> Void
    let onLongPressCancelled: () -> Void
    let canHorizontalSwipe: () -> Bool
    let onHorizontalSwipe: (Int) -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(
            onPressChanged: onPressChanged,
            onTap: onTap,
            onLongPressBegan: onLongPressBegan,
            onLongPressChanged: onLongPressChanged,
            onLongPressEnded: onLongPressEnded,
            onLongPressCancelled: onLongPressCancelled,
            canHorizontalSwipe: canHorizontalSwipe,
            onHorizontalSwipe: onHorizontalSwipe
        )
    }

    func makeUIView(context: Context) -> CollapsedHostTitleGestureView {
        let view = CollapsedHostTitleGestureView()
        view.backgroundColor = .clear
        view.isUserInteractionEnabled = isEnabled
        view.onPressChanged = onPressChanged

        let coordinator = context.coordinator

        let tap = UITapGestureRecognizer(target: coordinator, action: #selector(Coordinator.handleTap(_:)))
        tap.cancelsTouchesInView = false
        tap.delegate = coordinator
        view.addGestureRecognizer(tap)

        let lp = UILongPressGestureRecognizer(target: coordinator, action: #selector(Coordinator.handleLongPress(_:)))
        lp.minimumPressDuration = 0.22
        lp.allowableMovement = 12
        lp.cancelsTouchesInView = true
        lp.delegate = coordinator
        view.addGestureRecognizer(lp)

        let pan = UIPanGestureRecognizer(target: coordinator, action: #selector(Coordinator.handleHorizontalPan(_:)))
        pan.maximumNumberOfTouches = 1
        pan.cancelsTouchesInView = true
        pan.delegate = coordinator
        view.addGestureRecognizer(pan)

        coordinator.tapGesture = tap
        coordinator.longPressGesture = lp
        coordinator.panGesture = pan

        return view
    }

    func updateUIView(_ uiView: CollapsedHostTitleGestureView, context: Context) {
        uiView.isUserInteractionEnabled = isEnabled
        uiView.onPressChanged = onPressChanged
        if !isEnabled {
            onPressChanged(false)
        }
        context.coordinator.onPressChanged = onPressChanged
        context.coordinator.onTap = onTap
        context.coordinator.onLongPressBegan = onLongPressBegan
        context.coordinator.onLongPressChanged = onLongPressChanged
        context.coordinator.onLongPressEnded = onLongPressEnded
        context.coordinator.onLongPressCancelled = onLongPressCancelled
        context.coordinator.canHorizontalSwipe = canHorizontalSwipe
        context.coordinator.onHorizontalSwipe = onHorizontalSwipe
    }

    final class Coordinator: NSObject, UIGestureRecognizerDelegate {
        weak var tapGesture: UITapGestureRecognizer?
        weak var longPressGesture: UILongPressGestureRecognizer?
        weak var panGesture: UIPanGestureRecognizer?

        var onPressChanged: (Bool) -> Void
        var onTap: (CGRect, CGSize) -> Void
        var onLongPressBegan: (CGRect, CGSize) -> Void
        var onLongPressChanged: (CGPoint) -> Void
        var onLongPressEnded: () -> Void
        var onLongPressCancelled: () -> Void
        var canHorizontalSwipe: () -> Bool
        var onHorizontalSwipe: (Int) -> Void

        private var suppressNextTap = false
        private var hasTriggeredHorizontalSwitch = false

        init(
            onPressChanged: @escaping (Bool) -> Void,
            onTap: @escaping (CGRect, CGSize) -> Void,
            onLongPressBegan: @escaping (CGRect, CGSize) -> Void,
            onLongPressChanged: @escaping (CGPoint) -> Void,
            onLongPressEnded: @escaping () -> Void,
            onLongPressCancelled: @escaping () -> Void,
            canHorizontalSwipe: @escaping () -> Bool,
            onHorizontalSwipe: @escaping (Int) -> Void
        ) {
            self.onPressChanged = onPressChanged
            self.onTap = onTap
            self.onLongPressBegan = onLongPressBegan
            self.onLongPressChanged = onLongPressChanged
            self.onLongPressEnded = onLongPressEnded
            self.onLongPressCancelled = onLongPressCancelled
            self.canHorizontalSwipe = canHorizontalSwipe
            self.onHorizontalSwipe = onHorizontalSwipe
        }

        @objc func handleTap(_ gesture: UITapGestureRecognizer) {
            guard gesture.state == .ended else { return }
            onPressChanged(false)
            if suppressNextTap {
                suppressNextTap = false
                return
            }
            guard let view = gesture.view else { return }
            let window = view.window
            let windowSize = window?.bounds.size ?? view.bounds.size
            let anchorInWindow = view.convert(view.bounds, to: window)
            onTap(anchorInWindow, windowSize)
        }

        @objc func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
            guard let view = gesture.view else { return }
            let window = view.window
            let pointInWindow = gesture.location(in: window)
            let windowSize = window?.bounds.size ?? view.bounds.size

            switch gesture.state {
            case .began:
                suppressNextTap = true
                onPressChanged(false)
                let anchorInWindow = view.convert(view.bounds, to: window)
                onLongPressBegan(anchorInWindow, windowSize)
            case .changed:
                onLongPressChanged(pointInWindow)
            case .ended:
                onLongPressEnded()
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                    self?.suppressNextTap = false
                }
            case .cancelled, .failed:
                onPressChanged(false)
                onLongPressCancelled()
                suppressNextTap = false
            default:
                break
            }
        }

        @objc func handleHorizontalPan(_ gesture: UIPanGestureRecognizer) {
            guard let view = gesture.view else { return }
            let tx = gesture.translation(in: view).x
            let vx = gesture.velocity(in: view).x

            switch gesture.state {
            case .began:
                hasTriggeredHorizontalSwitch = false
                suppressNextTap = true
                onPressChanged(false)
            case .changed:
                if !hasTriggeredHorizontalSwitch && abs(tx) >= 36 {
                    hasTriggeredHorizontalSwitch = true
                    onHorizontalSwipe(tx < 0 ? 1 : -1)
                }
            case .ended:
                if !hasTriggeredHorizontalSwitch && abs(tx) >= 18 && abs(vx) >= 300 {
                    hasTriggeredHorizontalSwitch = true
                    onHorizontalSwipe(tx < 0 ? 1 : -1)
                }
                hasTriggeredHorizontalSwitch = false
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                    self?.suppressNextTap = false
                }
            case .cancelled, .failed:
                hasTriggeredHorizontalSwitch = false
                suppressNextTap = false
                onPressChanged(false)
            default:
                break
            }
        }

        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            if gestureRecognizer === panGesture, let pan = gestureRecognizer as? UIPanGestureRecognizer {
                guard canHorizontalSwipe(), let view = pan.view else { return false }
                let vel = pan.velocity(in: view)
                let trans = pan.translation(in: view)
                let absVX = abs(vel.x)
                let absVY = abs(vel.y)
                if absVX > 10 || absVY > 10 {
                    return absVX > absVY * 1.4
                }
                return abs(trans.x) > abs(trans.y) * 1.4 && abs(trans.x) > 4
            }
            return true
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            if otherGestureRecognizer === tapGesture
                || otherGestureRecognizer === longPressGesture
                || otherGestureRecognizer === panGesture {
                return false
            }
            let otherName = String(describing: type(of: otherGestureRecognizer))
            if otherName.contains("ResponderGestureRecognizer") {
                return true
            }
            return false
        }
    }
}

private final class NavigationBarHookView: UIView {
    weak var coordinator: NavigationBarTapHelper.Coordinator?
    private weak var observedNavBar: UINavigationBar?
    private var boundsObservation: NSKeyValueObservation?
    private var lastReportedCollapsed: Bool? = nil
    var lastHandledScrollToTopToken: Int = 0
    
    override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil {
            configure()
        } else {
            boundsObservation?.invalidate()
            boundsObservation = nil
            observedNavBar = nil
        }
    }
    
    override func didMoveToSuperview() {
        super.didMoveToSuperview()
        if superview != nil {
            configure()
        }
    }

    deinit {
        boundsObservation?.invalidate()
    }
    
    func configure() {
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            guard let nav = self.nearestNavigationController else { return }
            let navBar = nav.navigationBar
            let recognizers = navBar.gestureRecognizers ?? []
            guard let coordinator = self.coordinator else { return }
            coordinator.hookView = self
            coordinator.navigationController = nav

            let existingTap = recognizers.first(where: { $0 is NavigationBarTapGestureRecognizer })
            if existingTap?.delegate !== coordinator {
                if let old = existingTap { navBar.removeGestureRecognizer(old) }
                let tap = NavigationBarTapGestureRecognizer(target: coordinator, action: #selector(NavigationBarTapHelper.Coordinator.handleTap))
                tap.cancelsTouchesInView = false
                tap.delegate = coordinator
                navBar.addGestureRecognizer(tap)
            }
            let existingLP = recognizers.first(where: { $0 is NavigationBarLongPressGestureRecognizer })
            if existingLP?.delegate !== coordinator {
                if let old = existingLP { navBar.removeGestureRecognizer(old) }
                let lp = NavigationBarLongPressGestureRecognizer(target: coordinator, action: #selector(NavigationBarTapHelper.Coordinator.handleLongPress))
                lp.minimumPressDuration = 0.22
                lp.allowableMovement = 12
                lp.cancelsTouchesInView = true
                lp.delegate = coordinator
                navBar.addGestureRecognizer(lp)
            }
            let existingPan = recognizers.first(where: { $0 is NavigationBarHorizontalPanGestureRecognizer })
            if existingPan?.delegate !== coordinator {
                if let old = existingPan { navBar.removeGestureRecognizer(old) }
                let pan = NavigationBarHorizontalPanGestureRecognizer(target: coordinator, action: #selector(NavigationBarTapHelper.Coordinator.handleHorizontalPan))
                pan.maximumNumberOfTouches = 1
                pan.cancelsTouchesInView = true
                pan.delegate = coordinator
                navBar.addGestureRecognizer(pan)
            }

            if self.observedNavBar !== navBar {
                self.boundsObservation?.invalidate()
                self.observedNavBar = navBar
                self.boundsObservation = navBar.observe(\.bounds, options: [.initial, .new]) { [weak self, weak nav] bar, _ in
                    DispatchQueue.main.async {
                        guard let self, let nav else { return }
                        self.evaluateCollapsedState(nav: nav, navBar: bar)
                    }
                }
            } else {
                self.evaluateCollapsedState(nav: nav, navBar: navBar)
            }
        }
    }

    func scrollListToTopIfNeeded(token: Int) {
        guard token != lastHandledScrollToTopToken else { return }
        lastHandledScrollToTopToken = token
        DispatchQueue.main.async { [weak self] in
            guard let self, let rootVC = self.nearestViewController else { return }
            guard let scrollView = Self.findPrimaryScrollView(in: rootVC.view) else { return }
            let targetY = -scrollView.adjustedContentInset.top
            if scrollView.contentOffset.y > targetY + 1 {
                scrollView.setContentOffset(CGPoint(x: scrollView.contentOffset.x, y: targetY), animated: false)
            }
        }
    }

    private func evaluateCollapsedState(nav: UINavigationController, navBar: UINavigationBar) {
        guard self.window != nil else { return }
        // Only track large-title collapse when the root list screen is the active top controller.
        let isRootTop = nav.viewControllers.count <= 1 || nav.topViewController === self.nearestViewController
        guard isRootTop else { return }

        let height = navBar.bounds.height
        guard height > 1 else { return }

        // Hysteresis thresholds between inline bar (~44–54pt) and expanded large title (~96pt).
        let nextCollapsed: Bool
        if let prev = lastReportedCollapsed {
            if prev {
                nextCollapsed = height < 64
            } else {
                nextCollapsed = height <= 58
            }
        } else {
            nextCollapsed = height <= 58
        }

        if nextCollapsed != lastReportedCollapsed {
            lastReportedCollapsed = nextCollapsed
            coordinator?.onTitleCollapsedChanged(nextCollapsed)
        }
    }

    private static func findPrimaryScrollView(in view: UIView) -> UIScrollView? {
        if let scroll = view as? UIScrollView,
           scroll.isScrollEnabled,
           scroll.bounds.height > 120 {
            return scroll
        }
        for sub in view.subviews {
            if let found = findPrimaryScrollView(in: sub) {
                return found
            }
        }
        return nil
    }
}

private struct NavigationBarTapHelper: UIViewRepresentable {
    let scrollToTopToken: Int
    let onTap: () -> Void
    let canOpenCollapsedDropdown: () -> Bool
    let onCollapsedTitleTap: (CGRect, CGSize) -> Void
    let canBeginLongPress: () -> Bool
    let onLongPressBegan: (CGRect, CGSize, Bool) -> Void
    let onLongPressChanged: (CGPoint) -> Void
    let onLongPressEnded: () -> Void
    let onLongPressCancelled: () -> Void
    let canHorizontalSwipe: () -> Bool
    let onHorizontalSwipe: (Int) -> Void
    let onTitleCollapsedChanged: (Bool) -> Void
    
    func makeCoordinator() -> Coordinator {
        Coordinator(
            onTap: onTap,
            canOpenCollapsedDropdown: canOpenCollapsedDropdown,
            onCollapsedTitleTap: onCollapsedTitleTap,
            canBeginLongPress: canBeginLongPress,
            onLongPressBegan: onLongPressBegan,
            onLongPressChanged: onLongPressChanged,
            onLongPressEnded: onLongPressEnded,
            onLongPressCancelled: onLongPressCancelled,
            canHorizontalSwipe: canHorizontalSwipe,
            onHorizontalSwipe: onHorizontalSwipe,
            onTitleCollapsedChanged: onTitleCollapsedChanged
        )
    }
    
    func makeUIView(context: Context) -> NavigationBarHookView {
        let view = NavigationBarHookView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        view.coordinator = context.coordinator
        view.lastHandledScrollToTopToken = scrollToTopToken
        return view
    }
    
    func updateUIView(_ uiView: NavigationBarHookView, context: Context) {
        context.coordinator.onTap = onTap
        context.coordinator.canOpenCollapsedDropdown = canOpenCollapsedDropdown
        context.coordinator.onCollapsedTitleTap = onCollapsedTitleTap
        context.coordinator.canBeginLongPress = canBeginLongPress
        context.coordinator.onLongPressBegan = onLongPressBegan
        context.coordinator.onLongPressChanged = onLongPressChanged
        context.coordinator.onLongPressEnded = onLongPressEnded
        context.coordinator.onLongPressCancelled = onLongPressCancelled
        context.coordinator.canHorizontalSwipe = canHorizontalSwipe
        context.coordinator.onHorizontalSwipe = onHorizontalSwipe
        context.coordinator.onTitleCollapsedChanged = onTitleCollapsedChanged
        uiView.coordinator = context.coordinator
        uiView.configure()
        uiView.scrollListToTopIfNeeded(token: scrollToTopToken)
    }
    
    class Coordinator: NSObject, UIGestureRecognizerDelegate {
        weak var hookView: NavigationBarHookView?
        weak var navigationController: UINavigationController?

        var onTap: () -> Void
        var canOpenCollapsedDropdown: () -> Bool
        var onCollapsedTitleTap: (CGRect, CGSize) -> Void
        var canBeginLongPress: () -> Bool
        var onLongPressBegan: (CGRect, CGSize, Bool) -> Void
        var onLongPressChanged: (CGPoint) -> Void
        var onLongPressEnded: () -> Void
        var onLongPressCancelled: () -> Void
        var canHorizontalSwipe: () -> Bool
        var onHorizontalSwipe: (Int) -> Void
        var onTitleCollapsedChanged: (Bool) -> Void

        private var suppressNextTap = false
        private var hasTriggeredHorizontalSwitch = false

        init(
            onTap: @escaping () -> Void,
            canOpenCollapsedDropdown: @escaping () -> Bool,
            onCollapsedTitleTap: @escaping (CGRect, CGSize) -> Void,
            canBeginLongPress: @escaping () -> Bool,
            onLongPressBegan: @escaping (CGRect, CGSize, Bool) -> Void,
            onLongPressChanged: @escaping (CGPoint) -> Void,
            onLongPressEnded: @escaping () -> Void,
            onLongPressCancelled: @escaping () -> Void,
            canHorizontalSwipe: @escaping () -> Bool,
            onHorizontalSwipe: @escaping (Int) -> Void,
            onTitleCollapsedChanged: @escaping (Bool) -> Void
        ) {
            self.onTap = onTap
            self.canOpenCollapsedDropdown = canOpenCollapsedDropdown
            self.onCollapsedTitleTap = onCollapsedTitleTap
            self.canBeginLongPress = canBeginLongPress
            self.onLongPressBegan = onLongPressBegan
            self.onLongPressChanged = onLongPressChanged
            self.onLongPressEnded = onLongPressEnded
            self.onLongPressCancelled = onLongPressCancelled
            self.canHorizontalSwipe = canHorizontalSwipe
            self.onHorizontalSwipe = onHorizontalSwipe
            self.onTitleCollapsedChanged = onTitleCollapsedChanged
        }

        private func isRootHomeVisible() -> Bool {
            guard let hookView, hookView.window != nil,
                  let nav = navigationController ?? hookView.nearestNavigationController else {
                return false
            }
            if nav.viewControllers.count <= 1 {
                return true
            }
            return nav.topViewController === hookView.nearestViewController
        }
        
        @objc func handleTap(_ gesture: UITapGestureRecognizer) {
            guard gesture.state == .ended else { return }
            if suppressNextTap {
                suppressNextTap = false
                return
            }
            guard isRootHomeVisible(), let navBar = gesture.view as? UINavigationBar else { return }
            let point = gesture.location(in: navBar)
            guard let hit = titleHitTarget(for: point, in: navBar) else { return }

            if hit.isCollapsedInline && canOpenCollapsedDropdown() {
                let window = navBar.window
                let windowSize = window?.bounds.size ?? navBar.bounds.size
                let anchorInWindow = navBar.convert(hit.rect, to: window)
                onCollapsedTitleTap(anchorInWindow, windowSize)
            } else {
                onTap()
            }
        }

        @objc func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
            guard let navBar = gesture.view as? UINavigationBar else { return }
            let window = navBar.window
            let pointInWindow = gesture.location(in: window)
            let windowSize = window?.bounds.size ?? navBar.bounds.size

            switch gesture.state {
            case .began:
                let pointInNav = gesture.location(in: navBar)
                guard isRootHomeVisible(),
                      canBeginLongPress(),
                      let hit = titleHitTarget(for: pointInNav, in: navBar) else {
                    gesture.isEnabled = false
                    gesture.isEnabled = true
                    return
                }
                suppressNextTap = true
                let anchorInWindow = navBar.convert(hit.rect, to: window)
                onLongPressBegan(anchorInWindow, windowSize, hit.isCollapsedInline)
            case .changed:
                onLongPressChanged(pointInWindow)
            case .ended:
                onLongPressEnded()
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                    self?.suppressNextTap = false
                }
            case .cancelled, .failed:
                onLongPressCancelled()
                suppressNextTap = false
            default:
                break
            }
        }

        @objc func handleHorizontalPan(_ gesture: UIPanGestureRecognizer) {
            guard let navBar = gesture.view as? UINavigationBar else { return }
            let tx = gesture.translation(in: navBar).x
            let vx = gesture.velocity(in: navBar).x

            switch gesture.state {
            case .began:
                hasTriggeredHorizontalSwitch = false
                suppressNextTap = true
            case .changed:
                if !hasTriggeredHorizontalSwitch && abs(tx) >= 38 {
                    hasTriggeredHorizontalSwitch = true
                    onHorizontalSwipe(tx < 0 ? 1 : -1)
                }
            case .ended:
                if !hasTriggeredHorizontalSwitch && abs(tx) >= 20 && abs(vx) >= 320 {
                    hasTriggeredHorizontalSwitch = true
                    onHorizontalSwipe(tx < 0 ? 1 : -1)
                }
                hasTriggeredHorizontalSwitch = false
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { [weak self] in
                    self?.suppressNextTap = false
                }
            case .cancelled, .failed:
                hasTriggeredHorizontalSwitch = false
                suppressNextTap = false
            default:
                break
            }
        }

        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard isRootHomeVisible() else { return false }
            if let pan = gestureRecognizer as? NavigationBarHorizontalPanGestureRecognizer {
                guard canHorizontalSwipe(), let navBar = pan.view as? UINavigationBar else {
                    return false
                }
                let vel = pan.velocity(in: navBar)
                let trans = pan.translation(in: navBar)
                let absVX = abs(vel.x)
                let absVY = abs(vel.y)
                if absVX > 10 || absVY > 10 {
                    return absVX > absVY * 1.4
                }
                return abs(trans.x) > abs(trans.y) * 1.4 && abs(trans.x) > 4
            }
            return true
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            if gestureRecognizer is NavigationBarLongPressGestureRecognizer
                || otherGestureRecognizer is NavigationBarLongPressGestureRecognizer
                || gestureRecognizer is NavigationBarHorizontalPanGestureRecognizer
                || otherGestureRecognizer is NavigationBarHorizontalPanGestureRecognizer {
                return false
            }
            return true
        }
        
        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
            guard isRootHomeVisible(), let navBar = gestureRecognizer.view as? UINavigationBar else {
                return false
            }
            if gestureRecognizer is NavigationBarLongPressGestureRecognizer, !canBeginLongPress() {
                return false
            }
            if gestureRecognizer is NavigationBarHorizontalPanGestureRecognizer, !canHorizontalSwipe() {
                return false
            }
            
            // Check if touch is on the dedicated collapsed host title overlay or an interactive control (gear, plus, search, etc.)
            var current: UIView? = touch.view
            while let v = current, v !== navBar {
                if v is CollapsedHostTitleGestureView {
                    return false
                }
                let clsName = NSStringFromClass(type(of: v))
                if v is UIControl && !(v is UILabel) && !clsName.contains("NavigationBarTitleControl") {
                    return false
                }
                if clsName.contains("ButtonBarButton") || clsName.contains("Search") {
                    return false
                }
                current = v.superview
            }
            
            let point = touch.location(in: navBar)
            return titleHitTarget(for: point, in: navBar) != nil
        }

        private func titleHitTarget(for point: CGPoint, in navBar: UINavigationBar) -> (rect: CGRect, isCollapsedInline: Bool)? {
            let width = navBar.bounds.width
            let height = navBar.bounds.height
            let isCollapsed = height <= 60

            if !isCollapsed {
                // 1. Expanded large title: search for visible large title UILabel
                let labels = findLargeTitleLabels(in: navBar)
                for label in labels where !label.isHidden && label.alpha > 0.05 {
                    let frameInNavBar = label.convert(label.bounds, to: navBar)
                    guard frameInNavBar.minY >= 24 else { continue }
                    let hitRect = frameInNavBar.insetBy(dx: -20, dy: -12)
                    if hitRect.contains(point) {
                        return (frameInNavBar, false)
                    }
                }
                // 2. Fallback geometry for expanded large title area
                let largeTitleRect = CGRect(x: 16, y: 36, width: min(280, width * 0.76), height: max(32, height - 40))
                if largeTitleRect.insetBy(dx: -8, dy: -6).contains(point) {
                    return (largeTitleRect, false)
                }
                return nil
            }

            // Collapsed inline title bar:
            // 1. Prefer custom principal titleView (multi-gateway centered host header)
            if let titleView = navBar.topItem?.titleView,
               !titleView.isHidden,
               titleView.alpha > 0.05,
               titleView.bounds.width > 8 {
                let frameInNavBar = titleView.convert(titleView.bounds, to: navBar)
                let hitRect = frameInNavBar.insetBy(dx: -20, dy: -12)
                if hitRect.contains(point) {
                    return (frameInNavBar, true)
                }
            }

            // 2. Search for visible inline UILabel (single-gateway "Multigravity" or host text)
            let labels = findVisibleInlineLabels(in: navBar)
            for label in labels {
                let frameInNavBar = label.convert(label.bounds, to: navBar)
                let hitRect = frameInNavBar.insetBy(dx: -20, dy: -12)
                if hitRect.contains(point) {
                    return (frameInNavBar, true)
                }
            }

            // 3. Fallback centered inline zone (strictly clear of leading/trailing bar buttons)
            let inlineTitleRect = CGRect(x: width * 0.22, y: 0, width: width * 0.56, height: height)
            if inlineTitleRect.contains(point) {
                return (inlineTitleRect, true)
            }
            return nil
        }

        private func findLargeTitleLabels(in view: UIView) -> [UILabel] {
            var results: [UILabel] = []
            if let label = view as? UILabel, let text = label.text, text.contains("Multigravity") {
                results.append(label)
            }
            for subview in view.subviews {
                results.append(contentsOf: findLargeTitleLabels(in: subview))
            }
            return results
        }

        private func findVisibleInlineLabels(in navBar: UINavigationBar) -> [UILabel] {
            var results: [UILabel] = []
            let barMidX = navBar.bounds.midX
            let maxOffset = navBar.bounds.width * 0.26
            func walk(_ view: UIView) {
                if let label = view as? UILabel,
                   !label.isHidden,
                   label.alpha > 0.05,
                   let text = label.text,
                   !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    let frame = label.convert(label.bounds, to: navBar)
                    if frame.height > 8 && frame.height < 36 && abs(frame.midX - barMidX) <= maxOffset {
                        results.append(label)
                    }
                }
                for sub in view.subviews {
                    walk(sub)
                }
            }
            walk(navBar)
            return results
        }
    }
}




/// Quantized window geometry and optional active vertical fold division (iPhone Duo Book posture).
private nonisolated struct AdaptiveLayoutGeometry: Equatable, Sendable {
    var width: CGFloat = 0
    var height: CGFloat = 0
    var verticalDivision: CGRect? = nil

    static func measure(from proxy: GeometryProxy) -> AdaptiveLayoutGeometry {
        let w = proxy.size.width.rounded()
        let h = proxy.size.height.rounded()
        let divisionRect: CGRect?
        #if compiler(>=6.4.1)
        if #available(iOS 27.1, *),
           let region = proxy.reservedRegions(kind: .division).first(where: { region in
               region.isActive
                   && region.frame.height > region.frame.width
                   && region.frame.minX >= 280
                   && (proxy.size.width - region.frame.maxX) >= 340
           }) {
            let f = region.frame
            divisionRect = CGRect(
                x: f.origin.x.rounded(),
                y: f.origin.y.rounded(),
                width: f.size.width.rounded(),
                height: f.size.height.rounded()
            )
        } else {
            divisionRect = nil
        }
        #else
        divisionRect = nil
        #endif
        return AdaptiveLayoutGeometry(width: w, height: h, verticalDivision: divisionRect)
    }
}

/// Disables the iOS 27.1+ side vertical toolbar in two-column split layout so both side-by-side
/// NavigationStacks share consistent horizontal top bars.
private struct DisableVerticalToolbarIfAvailable: ViewModifier {
    func body(content: Content) -> some View {
        #if compiler(>=6.4.1)
        if #available(iOS 27.1, *) {
            content.toolbarVerticalBehavior(.disabled)
        } else {
            content
        }
        #else
        content
        #endif
    }
}

/// Floating pill shown when files await delivery from the Share Extension.
/// Adjusts bottom clearance when iPhone Duo moves search into the trailing vertical bar.
private struct SharePendingDeliveryPill: View {
    let count: Int
    let action: () -> Void

    var body: some View {
        #if compiler(>=6.4.1)
        if #available(iOS 27.1, *) {
            SharePendingDeliveryPillV27(count: count, action: action)
        } else {
            pillButton
                .padding(.bottom, 76)
        }
        #else
        pillButton
            .padding(.bottom, 76)
        #endif
    }

    private var pillButton: some View {
        Button(action: action) {
            Text("\(count) 个文件待投递 · 选择去向")
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(.primary)
                .padding(.horizontal, 18)
                .padding(.vertical, 11)
                .glassEffect(.regular.interactive(), in: .capsule)
        }
        .buttonStyle(.plain)
    }
}

#if compiler(>=6.4.1)
@available(iOS 27.1, *)
private struct SharePendingDeliveryPillV27: View {
    @Environment(\.toolbarVerticalEdge) private var toolbarVerticalEdge
    let count: Int
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Text("\(count) 个文件待投递 · 选择去向")
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(.primary)
                .padding(.horizontal, 18)
                .padding(.vertical, 11)
                .glassEffect(.regular.interactive(), in: .capsule)
        }
        .buttonStyle(.plain)
        .padding(.bottom, toolbarVerticalEdge != nil ? 16 : 76)
    }
}
#endif

/// Row interactions: long-press native context menu with Rename, Star, Archive, and Delete.
private struct RowActions: ViewModifier {
    let isSplit: Bool
    let item: ConversationItem
    var onTogglePin: (() -> Void)? = nil
    let onRename: () -> Void
    let onToggleStar: () -> Void
    let onArchive: () -> Void
    let onDelete: () -> Void
    
    func body(content: Content) -> some View {
        content.contextMenu {
            if let onTogglePin {
                Button {
                    onTogglePin()
                } label: {
                    Label(
                        item.isPinned ? String(localized: "取消置顶") : String(localized: "置顶"),
                        systemImage: item.isPinned ? "pin.slash" : "pin"
                    )
                }
            }
            if !item.isDraft {
                Button {
                    onRename()
                } label: {
                    Label("重命名", systemImage: "pencil")
                }
                Button {
                    onToggleStar()
                } label: {
                    Label(
                        item.isStarred ? String(localized: "取消星标") : String(localized: "星标"),
                        systemImage: item.isStarred ? "star.slash" : "star"
                    )
                }
                Button {
                    onArchive()
                } label: {
                    Label(
                        item.isArchived ? String(localized: "取消归档") : String(localized: "归档"),
                        systemImage: item.isArchived ? "tray.and.arrow.up" : "archivebox"
                    )
                }
            }
            Button(role: .destructive) {
                onDelete()
            } label: {
                Label("删除", systemImage: "trash")
            }
        }
    }
}


/// Show/hide button for the conversation list column, placed in the chat column's toolbar.
private struct SidebarToggleToolbarItem: ToolbarContent {
    let isShown: Bool
    let action: () -> Void
    
    var body: some ToolbarContent {
        ToolbarItem(placement: .topBarLeading) {
            Button(action: action) {
                Image(systemName: "sidebar.left")
                    .font(.system(size: 16, weight: .semibold))
            }
            .accessibilityLabel(isShown ? String(localized: "收起侧栏") : String(localized: "展开侧栏"))
        }
    }
}


/// iPhone: the system search field (a Liquid Glass capsule at the bottom). On iPad the system moves it into
/// the top toolbar, so the same bottom capsule is drawn by hand with the native glass effect.
private struct ConversationSearch: ViewModifier {
    @Binding var text: String
    let usesBottomCapsule: Bool
    var filterName: String? = nil

    private var prompt: String {
        if let filterName, !filterName.isEmpty {
            return String(localized: "在「\(filterName)」中搜索...")
        }
        return String(localized: "搜索会话或工作区...")
    }
    
    func body(content: Content) -> some View {
        if usesBottomCapsule {
            content.safeAreaInset(edge: .bottom, spacing: 0) {
                HStack(spacing: 8) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField(prompt, text: $text)
                        .textFieldStyle(.plain)
                        .submitLabel(.search)
                        .autocorrectionDisabled()
                    if !text.isEmpty {
                        Button { text = "" } label: {
                            Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, 16)
                .frame(height: 48)
                .glassEffect(.regular.interactive(), in: .capsule)
                .padding(.horizontal, 16)
                .padding(.bottom, 8)
            }
        } else {
            content.searchable(text: $text, prompt: prompt)
        }
    }
}

/// 原样复刻电脑端 (status-task-running) 终端指令动效指示器
/// - 视口: 16x16
/// - 提示符 >: M3.7 5.2L6.7 8L3.7 10.8 (strokeWidth 1.4, round cap/join)
/// - 光标下划线 _: M8.3 10.8H12.3
/// - 动画: 1.05s 周期, 0~55% 显示, 56~100% 隐藏
struct TerminalRunningIndicator: View {
    var size: CGFloat = 12
    var color: Color = .green
    
    var body: some View {
        TimelineView(.periodic(from: .now, by: 0.1)) { timeline in
            let elapsed = timeline.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 1.05)
            let isCaretVisible = elapsed <= 0.5775 // 55% of 1.05s
            
            Canvas { context, canvasSize in
                let scale = canvasSize.width / 16.0
                let stroke = StrokeStyle(lineWidth: 1.4 * scale, lineCap: .round, lineJoin: .round)
                
                // 1. 终端提示符 >
                var prompt = Path()
                prompt.move(to: CGPoint(x: 3.7 * scale, y: 5.2 * scale))
                prompt.addLine(to: CGPoint(x: 6.7 * scale, y: 8.0 * scale))
                prompt.addLine(to: CGPoint(x: 3.7 * scale, y: 10.8 * scale))
                context.stroke(prompt, with: .color(color), style: stroke)
                
                // 2. 下划线光标 _
                if isCaretVisible {
                    var caret = Path()
                    caret.move(to: CGPoint(x: 8.3 * scale, y: 10.8 * scale))
                    caret.addLine(to: CGPoint(x: 12.3 * scale, y: 10.8 * scale))
                    context.stroke(caret, with: .color(color), style: stroke)
                }
            }
            .frame(width: size, height: size)
        }
    }
}

/// Animatable top-anchored vertical reveal layout: expands strictly downward and collapses strictly upward
/// while keeping its top edge welded to the header above with zero vertical centering drift.
private struct VerticalRevealLayout: Layout, Animatable {
    var progress: CGFloat

    var animatableData: CGFloat {
        get { progress }
        set { progress = newValue }
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard let child = subviews.first else {
            return CGSize(width: proposal.width ?? 0, height: 0)
        }
        let childSize = child.sizeThatFits(ProposedViewSize(width: proposal.width, height: nil))
        let clamped = max(0, min(1, progress))
        return CGSize(width: proposal.width ?? childSize.width, height: childSize.height * clamped)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard let child = subviews.first else { return }
        child.place(
            at: CGPoint(x: bounds.minX, y: bounds.minY),
            anchor: .topLeading,
            proposal: ProposedViewSize(width: bounds.width, height: nil)
        )
    }
}

/// Draws only the left, bottom (with smooth rounded corners), and right borders of the recessed branch drawer tray.
private struct DrawerTraySideBottomBorder: Shape {
    var bottomRadius: CGFloat = 12
    var inset: CGFloat = 0.375

    func path(in rect: CGRect) -> Path {
        let minX = rect.minX + inset
        let maxX = rect.maxX - inset
        let maxY = rect.maxY - inset
        let r = min(bottomRadius, (maxX - minX) / 2, maxY - rect.minY)
        var path = Path()
        path.move(to: CGPoint(x: minX, y: rect.minY))
        path.addLine(to: CGPoint(x: minX, y: maxY - r))
        path.addQuadCurve(
            to: CGPoint(x: minX + r, y: maxY),
            control: CGPoint(x: minX, y: maxY)
        )
        path.addLine(to: CGPoint(x: maxX - r, y: maxY))
        path.addQuadCurve(
            to: CGPoint(x: maxX, y: maxY - r),
            control: CGPoint(x: maxX, y: maxY)
        )
        path.addLine(to: CGPoint(x: maxX, y: rect.minY))
        return path
    }
}

/// Continuous unbroken internal tree connector drawn in the same coordinate space as each embedded child slot.
private struct BranchInternalTreeConnector: Shape {
    let continuesBelow: Bool
    let bottomPadding: CGFloat
    var spineX: CGFloat = 16
    var endX: CGFloat = 27
    var cornerRadius: CGFloat = 6

    func path(in rect: CGRect) -> Path {
        let cardHeight = max(rect.height - bottomPadding, 1)
        let midY = cardHeight / 2
        let r = min(cornerRadius, max(midY - 1, 0), max(endX - spineX, 0))
        var path = Path()

        // Unbroken vertical spine + smooth rounded elbow (no overlapping vertical double-stroke)
        if continuesBelow {
            path.move(to: CGPoint(x: spineX, y: -0.5))
            path.addLine(to: CGPoint(x: spineX, y: rect.height + 0.5))

            path.move(to: CGPoint(x: spineX, y: max(0, midY - r)))
            path.addQuadCurve(
                to: CGPoint(x: spineX + r, y: midY),
                control: CGPoint(x: spineX, y: midY)
            )
            path.addLine(to: CGPoint(x: endX, y: midY))
        } else {
            path.move(to: CGPoint(x: spineX, y: -0.5))
            path.addLine(to: CGPoint(x: spineX, y: max(0, midY - r)))
            path.addQuadCurve(
                to: CGPoint(x: spineX + r, y: midY),
                control: CGPoint(x: spineX, y: midY)
            )
            path.addLine(to: CGPoint(x: endX, y: midY))
        }
        return path
    }
}

/// Direction-gated UIKit pan recognizer for embedded swipe cards inside SwiftUI List so vertical scrolling stays 100% native.
private struct HorizontalSwipePanGesture: UIGestureRecognizerRepresentable {
    var onBegan: () -> Void
    var onChanged: (CGFloat) -> Void
    var onEnded: (CGFloat, CGFloat) -> Void

    func makeCoordinator(converter: CoordinateSpaceConverter) -> Coordinator {
        Coordinator()
    }

    func makeUIGestureRecognizer(context: Context) -> UIPanGestureRecognizer {
        let pan = UIPanGestureRecognizer()
        pan.maximumNumberOfTouches = 1
        pan.cancelsTouchesInView = true
        pan.delegate = context.coordinator
        return pan
    }

    func handleUIGestureRecognizerAction(_ recognizer: UIPanGestureRecognizer, context: Context) {
        let tx = recognizer.translation(in: recognizer.view).x
        let vx = recognizer.velocity(in: recognizer.view).x
        switch recognizer.state {
        case .began:
            onBegan()
        case .changed:
            onChanged(tx)
        case .ended, .cancelled, .failed:
            onEnded(tx, vx)
        default:
            break
        }
    }

    final class Coordinator: NSObject, UIGestureRecognizerDelegate {
        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard let pan = gestureRecognizer as? UIPanGestureRecognizer, let view = pan.view else {
                return false
            }
            let vel = pan.velocity(in: view)
            let trans = pan.translation(in: view)
            let absVX = abs(vel.x)
            let absVY = abs(vel.y)
            if absVX > 10 || absVY > 10 {
                return absVX > absVY * 1.4
            }
            return abs(trans.x) > abs(trans.y) * 1.4 && abs(trans.x) > 4
        }
    }
}

/// Embedded child conversation card inside the recessed branch drawer with full Tap, Long-Press Context Menu, and Horizontal Swipe actions.
private struct BranchChildSwipeRow<StatusBadge: View>: View {
    @Environment(\.colorScheme) private var colorScheme

    let item: ConversationItem
    let hasDraft: Bool
    let isSelected: Bool
    let isSwipedOpen: Bool
    let onSwipeStateChanged: (Bool) -> Void
    let onWillBeginSwipe: () -> Void
    let onTap: () -> Void
    let onPin: () -> Void
    let onRename: () -> Void
    let onToggleStar: () -> Void
    let onArchive: () -> Void
    let onDelete: () -> Void
    let isSplit: Bool
    @ViewBuilder let statusBadge: () -> StatusBadge

    @State private var offsetX: CGFloat = 0
    @State private var dragStartOffset: CGFloat = 0

    private let maxLeadingReveal: CGFloat = 52
    private var maxTrailingReveal: CGFloat { item.isDraft ? 52 : 94 }

    var body: some View {
        let cardShape = RoundedRectangle(cornerRadius: 10, style: .continuous)
        let cardBackground = colorScheme == .dark
            ? Color(uiColor: .tertiarySystemBackground)
            : Color(uiColor: .systemBackground)
        let cardBorder = colorScheme == .dark
            ? Color.white.opacity(0.08)
            : Color.black.opacity(0.065)

        ZStack {
            if offsetX > 0.5 {
                HStack(spacing: 0) {
                    Button {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        closeSwipe()
                        onPin()
                    } label: {
                        Image(systemName: item.isPinned ? "pin.slash.fill" : "pin.fill")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.white)
                            .frame(width: 30, height: 30)
                            .background(item.isPinned ? Color.secondary : Color.orange)
                            .clipShape(Circle())
                    }
                    .buttonStyle(.plain)
                    .frame(width: maxLeadingReveal)
                    Spacer(minLength: 0)
                }
            }

            if offsetX < -0.5 {
                HStack(spacing: 0) {
                    Spacer(minLength: 0)
                    HStack(spacing: 8) {
                        if !item.isDraft {
                            Button {
                                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                                closeSwipe()
                                onArchive()
                            } label: {
                                Image(systemName: item.isArchived ? "tray.and.arrow.up.fill" : "archivebox.fill")
                                    .font(.system(size: 12.5, weight: .semibold))
                                    .foregroundColor(.white)
                                    .frame(width: 30, height: 30)
                                    .background(item.isArchived ? Color.indigo : Color(red: 0.39, green: 0.45, blue: 0.55))
                                    .clipShape(Circle())
                            }
                            .buttonStyle(.plain)
                        }

                        Button {
                            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                            closeSwipe()
                            onDelete()
                        } label: {
                            Image(systemName: "trash.fill")
                                .font(.system(size: 12.5, weight: .semibold))
                                .foregroundColor(.white)
                                .frame(width: 30, height: 30)
                                .background(Color.red)
                                .clipShape(Circle())
                        }
                        .buttonStyle(.plain)
                    }
                    .frame(width: maxTrailingReveal)
                }
            }

            HStack(alignment: .center, spacing: 5) {
                if item.isPinned {
                    Image(systemName: "pin.fill")
                        .font(.system(size: 9.5, weight: .bold))
                        .foregroundColor(.orange)
                }
                if item.isStarred {
                    Image(systemName: "star.fill")
                        .font(.system(size: 9.5, weight: .bold))
                        .foregroundColor(.yellow)
                }

                Text(item.title)
                    .font(.system(size: 13.5, weight: .medium))
                    .lineLimit(1)
                    .truncationMode(.tail)
                    .foregroundColor(.primary)

                Spacer(minLength: 4)

                if !item.relativeTimeString.isEmpty {
                    Text(item.relativeTimeString)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                }

                statusBadge()
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8.5)
            .background(cardBackground)
            .overlay(
                cardShape.stroke(cardBorder, lineWidth: 0.5)
            )
            .overlay {
                if isSelected {
                    cardShape.fill(Color.accentColor.opacity(0.10))
                    cardShape.stroke(Color.accentColor.opacity(0.55), lineWidth: 1.5)
                }
            }
            .clipShape(cardShape)
            .shadow(
                color: colorScheme == .dark ? .clear : Color.black.opacity(0.035),
                radius: 1.5,
                x: 0,
                y: 0.5
            )
            .contentShape(cardShape)
            .offset(x: offsetX)
            .onTapGesture {
                if abs(offsetX) > 1 {
                    closeSwipe()
                } else {
                    onTap()
                }
            }
            .gesture(
                HorizontalSwipePanGesture(
                    onBegan: {
                        onWillBeginSwipe()
                        dragStartOffset = offsetX
                    },
                    onChanged: { tx in
                        let raw = dragStartOffset + tx
                        if raw > maxLeadingReveal {
                            offsetX = maxLeadingReveal + (raw - maxLeadingReveal) * 0.22
                        } else if raw < -maxTrailingReveal {
                            offsetX = -maxTrailingReveal + (raw + maxTrailingReveal) * 0.22
                        } else {
                            offsetX = raw
                        }
                    },
                    onEnded: { _, vx in
                        let target: CGFloat
                        if vx < -420 || offsetX < -maxTrailingReveal * 0.45 {
                            target = -maxTrailingReveal
                        } else if vx > 420 || offsetX > maxLeadingReveal * 0.45 {
                            target = maxLeadingReveal
                        } else {
                            target = 0
                        }
                        if target != 0 && dragStartOffset == 0 {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        }
                        withAnimation(.snappy(duration: 0.24, extraBounce: 0)) {
                            offsetX = target
                        }
                        onSwipeStateChanged(target != 0)
                    }
                )
            )
            .modifier(RowActions(
                isSplit: isSplit,
                item: item,
                onTogglePin: onPin,
                onRename: onRename,
                onToggleStar: onToggleStar,
                onArchive: onArchive,
                onDelete: onDelete
            ))
            .contentShape(.hoverEffect, cardShape)
            .hoverEffect(.highlight)
        }
        .clipShape(cardShape)
        .onChange(of: isSwipedOpen) { _, open in
            if !open && offsetX != 0 {
                withAnimation(.snappy(duration: 0.22, extraBounce: 0)) {
                    offsetX = 0
                }
            }
        }
    }

    private func closeSwipe() {
        withAnimation(.snappy(duration: 0.22, extraBounce: 0)) {
            offsetX = 0
        }
        onSwipeStateChanged(false)
    }
}
