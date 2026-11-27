import ActivityKit
import SwiftUI
import UIKit
import UserNotifications

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var pushManager = PushNotificationManager.shared
    @State private var aiAssist = AIAssistManager.shared
    @State private var showClearCacheAlert: Bool = false
    @State private var showUnpairAlert: Bool = false
    @State private var lanAddress: String = ""
    @State private var customAddress: String = ""
    @State private var modelCatalog = ModelCatalogStore.shared
    @State private var showMcpSkillsSheet: Bool = false
    @State private var webSheetURL: IdentifiableURL? = nil
    @State private var showPrivacyPolicySheet: Bool = false
    @State private var showLicensesSheet: Bool = false
    @State private var desktopUpdateStatus: DesktopUpdateStatus? = nil
    @State private var gatewayUpdateStatus: GatewaySelfUpdateStatus? = nil
    @State private var permissionPreset: String = AppSettings.shared.cachedPermissionPreset
    @State private var confirmedPermissionPreset: String = AppSettings.shared.cachedPermissionPreset
    @State private var showAutoPermissionConfirm: Bool = false
    @State private var permissionErrorMessage: String? = nil
    @State private var probeTask: Task<Void, Never>? = nil
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Form {
                // MARK: - 1. 设备配对与鉴权
                let gwStore = GatewayProfileStore.shared
                let showHostInSettings = settings.isPaired && !settings.isDemoMode && gwStore.hasMultipleGateways
                let activeHostLabel = gwStore.activeProfile?.resolvedDisplayName ?? settings.gatewayPlatformDisplayName
                Section(
                    header: Text("设备配对"),
                    footer: Text(settings.isDemoMode
                                 ? String(localized: "当前为演示模式，数据均为模拟，不会连接任何电脑。")
                                 : (showHostInSettings
                                    ? String(localized: "管理当前设备与「\(activeHostLabel)」的配对状态。")
                                    : String(localized: "管理当前设备与 \(settings.gatewayPlatformDisplayName) 的配对状态。")))
                ) {
                    HStack {
                        Text("配对状态")
                        Spacer()
                        if settings.isPaired {
                            HStack(spacing: 4) {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundColor(.green)
                                    .font(.system(size: 14))
                                Text(settings.isDemoMode
                                     ? String(localized: "演示模式")
                                     : (showHostInSettings
                                        ? String(localized: "已配对 · \(activeHostLabel)")
                                        : String(localized: "已配对")))
                                    .foregroundColor(.secondary)
                                    .lineLimit(1)
                                    .truncationMode(.tail)
                            }
                        } else {
                            Text("未配对")
                                .foregroundColor(.secondary)
                        }
                    }
                    
                    if let id = settings.deviceID {
                        HStack {
                            Text("设备 ID")
                            Spacer()
                            Text(id)
                                .font(.system(size: 13, design: .monospaced))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                    
                    Button(role: .destructive, action: {
                        showUnpairAlert = true
                    }) {
                        Text(settings.isDemoMode ? String(localized: "退出演示模式") : String(localized: "解除设备配对"))
                    }
                }
                
                // MARK: - 2. 网络 (局域网与自定义平铺，主域名后台智能路由)
                Section(
                    header: Text("网络"),
                    footer: Text("配置局域网或自定义节点直连地址（如 Tailscale/VPN/DDNS）。点击「保存」即可保存并立即测速探活。")
                ) {
                    // 局域网 (扫码配对默认填写)
                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Text("局域网")
                            Spacer()
                            let isLanDirty = normalizeURLString(lanAddress) != (settings.lanServerURL ?? "")
                            Button(action: saveLanAddress) {
                                if !isLanDirty && settings.lanServerURL != nil {
                                    HStack(spacing: 3) {
                                        Image(systemName: "checkmark")
                                            .font(.system(size: 10, weight: .bold))
                                        Text("已保存")
                                            .font(.system(size: 12, weight: .medium))
                                    }
                                    .foregroundColor(.secondary)
                                } else {
                                    Text("保存")
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundColor(isLanDirty ? .white : Color.accentColor)
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 3)
                                        .background(
                                            Capsule()
                                                .fill(isLanDirty ? Color.accentColor : Color.accentColor.opacity(0.12))
                                        )
                                }
                            }
                            .buttonStyle(.plain)
                        }
                        
                        TextField("如 http://192.168.1.50:58900", text: $lanAddress)
                            .font(.system(size: 13, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .keyboardType(.URL)
                            .onSubmit {
                                saveLanAddress()
                            }
                    }
                    .padding(.vertical, 2)
                    
                    // 自定义 (留给用户配置，如 Tailscale)
                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Text("自定义")
                            Spacer()
                            let isCustomDirty = normalizeURLString(customAddress) != (settings.customServerURL ?? "")
                            Button(action: saveCustomAddress) {
                                if !isCustomDirty && settings.customServerURL != nil {
                                    HStack(spacing: 3) {
                                        Image(systemName: "checkmark")
                                            .font(.system(size: 10, weight: .bold))
                                        Text("已保存")
                                            .font(.system(size: 12, weight: .medium))
                                    }
                                    .foregroundColor(.secondary)
                                } else {
                                    Text("保存")
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundColor(isCustomDirty ? .white : Color.accentColor)
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 3)
                                        .background(
                                            Capsule()
                                                .fill(isCustomDirty ? Color.accentColor : Color.accentColor.opacity(0.12))
                                        )
                                }
                            }
                            .buttonStyle(.plain)
                        }
                        
                        TextField("如 http://100.x.x.x:58900", text: $customAddress)
                            .font(.system(size: 13, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .keyboardType(.URL)
                            .onSubmit {
                                saveCustomAddress()
                            }
                    }
                    .padding(.vertical, 2)
                }
                
                // MARK: - 3. 模型
                Section(
                    header: Text("模型"),
                    footer: Text(favoriteModelFooter)
                ) {
                    favoriteModelPicker(
                        title: String(localized: "首选模型"),
                        selection: $settings.favoriteModel1
                    )
                    favoriteModelPicker(
                        title: String(localized: "次选模型"),
                        selection: $settings.favoriteModel2
                    )
                }
                
                // MARK: - 4. 扩展与能力
                Section(
                    header: Text("扩展与能力"),
                    footer: Text("在此配置电脑端全局默认权限并查看 MCP 工具与技能；进入会话后长按右上角工具按钮也可随时切换该全局权限。")
                ) {
                    Picker("Agent 权限模式", selection: $permissionPreset) {
                        Text("严格审批").tag("strict")
                        Text("默认沙箱").tag("default")
                        Text("智能自动 (沙箱)").tag("auto")
                        Text("全自动 (无沙箱)").tag("turbo")
                    }
                    .onChange(of: permissionPreset) { _, next in
                        guard next != confirmedPermissionPreset else { return }
                        if next == "turbo" {
                            showAutoPermissionConfirm = true
                        } else {
                            applyPermissionPreset(next)
                        }
                    }
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        showMcpSkillsSheet = true
                    }) {
                        HStack {
                            Text("MCP 工具与技能看板")
                                .foregroundColor(.primary)
                            Spacer()
                            Image(systemName: "chevron.right")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.secondary)
                        }
                    }

                    NavigationLink {
                        AIAssistSettingsView()
                    } label: {
                        HStack {
                            Text("智能辅助")
                            Spacer()
                            Text(aiAssist.summaryText)
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                    }
                }
                .task {
                    await aiAssist.fetchStatus()
                }
                
                // MARK: - 4a. 实时语音对话
                Section(
                    header: Text("语音"),
                    footer: Text("开启后，输入框为空且空闲时右侧按钮支持单击实时双工语音通话、长按原生流式语音听写。")
                ) {
                    Toggle("实时语音对话", isOn: $settings.enableVoiceMode)
                        .onChange(of: settings.enableVoiceMode) { _, enabled in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            if !enabled && VoiceCallManager.shared.isInCall {
                                VoiceCallManager.shared.endCall()
                            }
                        }
                }
                
                // MARK: - 4b. 通知与实时活动
                Section(
                    header: Text("通知与实时活动"),
                    footer: Text("在锁屏、通知中心与灵动岛实时接收审批请求、任务进展与完成提醒。")
                ) {
                    if settings.isDemoMode {
                        HStack {
                            Text("推送通知")
                            Spacer()
                            Text("配对网关后可用")
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                        }
                    } else if pushManager.authorizationStatus == .denied {
                        Button {
                            if let url = URL(string: UIApplication.openSettingsURLString) {
                                UIApplication.shared.open(url)
                            }
                        } label: {
                            HStack {
                                Text("推送通知")
                                    .foregroundColor(.primary)
                                Spacer()
                                Text("前往系统设置开启")
                                    .font(.subheadline)
                                    .foregroundColor(.indigo)
                            }
                        }
                    } else if pushManager.authorizationStatus == .notDetermined {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task {
                                _ = await pushManager.requestAuthorizationAndRegister()
                            }
                        } label: {
                            HStack {
                                Text("开启推送通知")
                                Spacer()
                                Text("未开启")
                                    .font(.subheadline)
                                    .foregroundColor(.secondary)
                            }
                        }
                    } else {
                        Toggle("推送通知", isOn: $settings.enablePushNotifications)
                            .onChange(of: settings.enablePushNotifications) { _, enabled in
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                if enabled {
                                    pushManager.syncAllTokensIfPaired(force: true)
                                } else {
                                    pushManager.syncPreferencesWithGateway()
                                }
                            }
                    }

                    Toggle(isOn: $settings.enableLiveActivities) {
                        Text("灵动岛与实时活动")
                            .lineLimit(1)
                            .minimumScaleFactor(0.84)
                    }
                        .onChange(of: settings.enableLiveActivities) { _, enabled in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            if !enabled {
                                ActivityManager.shared.endAllActivities(finalStatus: "CANCELLED", dismissalPolicy: .immediate)
                            }
                            pushManager.syncPreferencesWithGateway()
                        }

                    NavigationLink {
                        NotificationAlertsSettingsView()
                    } label: {
                        HStack {
                            Text("提醒内容")
                            Spacer()
                            Text(pushManager.alertPreferencesSummary)
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                    }

                    NavigationLink {
                        NotificationSoundSettingsView()
                    } label: {
                        HStack {
                            Text("通知提示音")
                                .layoutPriority(1)
                            Spacer(minLength: 8)
                            Text(notificationSoundSummary)
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                                .minimumScaleFactor(0.8)
                        }
                    }
                }
                .task {
                    await pushManager.fetchAlertPreferences()
                }
                
                // MARK: - 5. 外观与主题
                Section(
                    header: Text("外观与主题"),
                    footer: Text("选择浅色、深色模式与界面字号缩放；点进「额度显示」与「精简模式」可自定义配额指示和对话降噪选项。")
                ) {
                    Picker("外观模式", selection: $settings.themeMode) {
                        Text("跟随系统").tag("system")
                        Text("浅色").tag("light")
                        Text("深色").tag("dark")
                    }
                    .pickerStyle(.segmented)
                    .onChange(of: settings.themeMode) { _, _ in
                        settings.applyAppearance()
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }

                    Picker("字号缩放", selection: $settings.fontSizeScale) {
                        ForEach(AppSettings.FontSizeScale.allCases) { scale in
                            Text(scale.displayName).tag(scale)
                        }
                    }
                    .pickerStyle(.menu)
                    .onChange(of: settings.fontSizeScale) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }

                    NavigationLink {
                        QuotaDisplaySettingsView()
                    } label: {
                        HStack {
                            Text("额度显示")
                            Spacer()
                            Text(quotaDisplaySummary)
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                        }
                    }

                    NavigationLink {
                        CompactModeSettingsView()
                    } label: {
                        HStack {
                            Text("精简模式")
                            Spacer()
                            Text(compactModeSummary)
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                        }
                    }

                    Toggle("新建对话启动台", isOn: $settings.newChatLaunchpadEnabled)
                        .onChange(of: settings.newChatLaunchpadEnabled) { _, _ in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        }
                }

                // MARK: - 6. 手势与使用技巧
                Section(
                    header: Text("手势与使用技巧"),
                    footer: Text("掌握常用快捷手势与界面隐蔽功能，大幅提升移动端操作效率。")
                ) {
                    NavigationLink {
                        TipsAndGesturesGuideView()
                    } label: {
                        Label {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("功能速查与手势指南")
                                    .font(.body)
                                Text("会话手势、模型轮换、文件夹过滤、工具看板等")
                                    .font(.caption)
                                    .foregroundColor(.secondary)
                            }
                        } icon: {
                            Image(systemName: "lightbulb.fill")
                                .foregroundColor(.orange)
                        }
                    }
                }

                // MARK: - 7. 存储 (整体下移一行，放在关于的上面)
                Section(
                    header: Text("存储"),
                    footer: Text("清除本地缓存的会话与文档数据，下次访问时将从网关重新拉取。")
                ) {
                    Button(role: .destructive, action: {
                        showClearCacheAlert = true
                    }) {
                        Text("清空本地缓存")
                    }
                }
                
                // MARK: - 8. 关于
                Section(header: Text("关于")) {
                    HStack {
                        Text("应用名称")
                        Spacer()
                        Text("Multigravity")
                            .foregroundColor(.secondary)
                    }
                    HStack {
                        Text("版本")
                        Spacer()
                        Text(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.2.5")
                            .foregroundColor(.secondary)
                    }
                    if settings.isPaired && !settings.isDemoMode {
                        let anyHostUpdate = (desktopUpdateStatus?.hasUpdate == true) || (gatewayUpdateStatus?.hasUpdate == true)
                        let hasHostStatus = (desktopUpdateStatus != nil) || (gatewayUpdateStatus != nil)
                        NavigationLink {
                            DesktopUpdateSheet(
                                initialStatus: desktopUpdateStatus,
                                initialGatewayStatus: gatewayUpdateStatus,
                                onStatusUpdated: { dStatus, gStatus in
                                    if let dStatus { desktopUpdateStatus = dStatus }
                                    if let gStatus { gatewayUpdateStatus = gStatus }
                                }
                            )
                        } label: {
                            HStack(spacing: 6) {
                                Text("宿主机更新")
                                Spacer()
                                if anyHostUpdate {
                                    Text("有新版")
                                        .font(.system(size: 10.5, weight: .semibold))
                                        .foregroundColor(.white)
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(Color.indigo)
                                        .clipShape(Capsule())
                                } else if hasHostStatus {
                                    Text("已是最新")
                                        .font(.subheadline)
                                        .foregroundColor(.secondary)
                                }
                            }
                        }
                    }
                    if let shareURL = appShareURL {
                        ShareLink(
                            item: shareURL,
                            subject: Text("Multigravity"),
                            message: Text(appShareMessage),
                            preview: SharePreview("Multigravity")
                        ) {
                            HStack {
                                Text("分享此应用")
                                    .foregroundColor(.primary)
                                Spacer()
                                Image(systemName: "square.and.arrow.up")
                                    .font(.caption)
                                    .foregroundColor(.secondary)
                            }
                        }
                    }
                    Button {
                        showPrivacyPolicySheet = true
                    } label: {
                        HStack {
                            Text("隐私政策")
                                .foregroundColor(.primary)
                            Spacer()
                            Image(systemName: "chevron.right")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    Button {
                        if let url = URL(string: "https://github.com/GHSaiMo/antigravity-mobile") {
                            webSheetURL = IdentifiableURL(url: url)
                        }
                    } label: {
                        HStack {
                            Text("支持与源代码")
                                .foregroundColor(.primary)
                            Spacer()
                            Text("GitHub")
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                            Image(systemName: "arrow.up.right.square")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    Button {
                        showLicensesSheet = true
                    } label: {
                        HStack {
                            Text("开源许可")
                                .foregroundColor(.primary)
                            Spacer()
                            Text("MIT")
                                .font(.subheadline)
                                .foregroundColor(.secondary)
                            Image(systemName: "chevron.right")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                }
            }
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.inline)
            .presentationDragIndicator(.visible)
        }
        .presentationDragIndicator(.visible)
        .onAppear {
            lanAddress = settings.lanServerURL ?? ""
            customAddress = settings.customServerURL ?? ""
            let normCached: String = {
                switch settings.cachedPermissionPreset.lowercased() {
                case "sandbox", "default": return "default"
                case "smart_auto": return "auto"
                case "auto", "turbo": return "turbo"
                default: return "strict"
                }
            }()
            permissionPreset = normCached
            confirmedPermissionPreset = normCached
        }
        .onDisappear {
            probeTask?.cancel()
            let normCustom = normalizeURLString(customAddress)
            if normCustom != (settings.customServerURL ?? "") {
                settings.customServerURL = normCustom.isEmpty ? nil : normCustom
            }
            let normLan = normalizeURLString(lanAddress)
            if normLan != (settings.lanServerURL ?? "") {
                settings.lanServerURL = normLan.isEmpty ? nil : normLan
            }
        }
        .task {
            await pushManager.refreshAuthorizationStatus()
            await connectionManager.probeEndpoints()
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
            Task {
                await pushManager.refreshAuthorizationStatus()
            }
        }
        .task {
            if let url = settings.serverURL {
                await modelCatalog.refresh(settings: settings, baseURL: url, force: true)
            }
            if let gwUrl = settings.gatewayURL {
                if let state = try? await APIClient.shared.fetchPermissionSettings(baseURL: gwUrl) {
                    settings.cachedPermissionPreset = state.effectiveMode
                    confirmedPermissionPreset = state.effectiveMode
                    permissionPreset = state.effectiveMode
                }
                if !settings.isDemoMode {
                    async let desktopUpd = try? APIClient.shared.fetchDesktopUpdateStatus(baseURL: gwUrl)
                    async let gwUpd = try? APIClient.shared.fetchGatewaySelfUpdateStatus(baseURL: gwUrl, refresh: false)
                    if let upd = await desktopUpd {
                        desktopUpdateStatus = upd
                    }
                    if let gUpd = await gwUpd {
                        gatewayUpdateStatus = gUpd
                    }
                }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .permissionPresetDidChange)) { notif in
            if let state = notif.object as? PermissionStateResponse {
                settings.cachedPermissionPreset = state.effectiveMode
                confirmedPermissionPreset = state.effectiveMode
                permissionPreset = state.effectiveMode
            }
        }
        .alert("开启全自动 (无沙箱)？", isPresented: $showAutoPermissionConfirm) {
            Button("取消", role: .cancel) {
                permissionPreset = confirmedPermissionPreset
            }
            Button("确认开启", role: .destructive) {
                applyPermissionPreset("turbo")
            }
        } message: {
            Text("开启后 Agent 将关闭终端沙箱并自动执行命令与工作区外写入（同步作用于桌面端全部会话）。日常离席推荐改用「智能自动 (沙箱)」。")
        }
        .alert("权限设置更新失败", isPresented: Binding(
            get: { permissionErrorMessage != nil },
            set: { if !$0 { permissionErrorMessage = nil } }
        )) {
            Button("确定", role: .cancel) { permissionErrorMessage = nil }
        } message: {
            Text(permissionErrorMessage ?? "")
        }
        .alert(settings.isDemoMode ? String(localized: "确定退出演示模式？") : String(localized: "确定解除设备配对？"), isPresented: $showUnpairAlert) {
            Button("取消", role: .cancel) {}
            Button(settings.isDemoMode ? String(localized: "退出演示") : String(localized: "解除配对"), role: .destructive) {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                if settings.isDemoMode {
                    settings.unpair()
                    CacheManager.shared.clearCache()
                    DocumentCacheManager.shared.clearCache()
                    NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil, userInfo: ["epoch": GatewayRuntimeState.shared.epoch])
                } else {
                    let hasRemaining = settings.unpairActiveProfile()
                    if !hasRemaining {
                        CacheManager.shared.clearCache()
                        DocumentCacheManager.shared.clearCache()
                        NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil, userInfo: ["epoch": GatewayRuntimeState.shared.epoch])
                    }
                }
                dismiss()
            }
        } message: {
            if settings.isDemoMode {
                Text(String(localized: "将退出演示模式并回到欢迎页，演示数据会被清除。"))
            } else if GatewayProfileStore.shared.hasMultipleGateways,
                      let hostName = GatewayProfileStore.shared.activeProfile?.resolvedDisplayName {
                Text(String(localized: "解除与「\(hostName)」的配对将通知该网关清理此设备绑定，并自动切换至其他已配对主机。"))
            } else {
                Text(String(localized: "解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。"))
            }
        }
        .alert("确定清空本地缓存？", isPresented: $showClearCacheAlert) {
            Button("取消", role: .cancel) {}
            Button("清空", role: .destructive) {
                CacheManager.shared.clearCache()
                DocumentCacheManager.shared.clearCache()
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            }
        } message: {
            Text("本地缓存的会话消息、方案及离线文档将被清理，下次访问时将从网关重新拉取。")
        }
        .sheet(isPresented: $showMcpSkillsSheet) {
            McpSkillsSheet()
        }
        .sheet(item: $webSheetURL) { item in
            SafariWebViewSheet(url: item.url)
        }
        .sheet(isPresented: $showPrivacyPolicySheet) {
            PrivacyPolicySheet(onOpenWeb: { url in
                showPrivacyPolicySheet = false
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) {
                    webSheetURL = IdentifiableURL(url: url)
                }
            })
        }
        .sheet(isPresented: $showLicensesSheet) {
            OpenSourceLicensesSheet()
        }
    }

    private func normalizeURLString(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines).trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard !trimmed.isEmpty else { return "" }
        if !trimmed.lowercased().hasPrefix("http://") && !trimmed.lowercased().hasPrefix("https://") {
            return "http://" + trimmed
        }
        return trimmed
    }

    private func saveLanAddress() {
        let normalized = normalizeURLString(lanAddress)
        lanAddress = normalized
        settings.lanServerURL = normalized.isEmpty ? nil : normalized
        scheduleProbeEndpoints()
    }

    private func saveCustomAddress() {
        let normalized = normalizeURLString(customAddress)
        customAddress = normalized
        settings.customServerURL = normalized.isEmpty ? nil : normalized
        scheduleProbeEndpoints()
    }

    private func scheduleProbeEndpoints() {
        probeTask?.cancel()
        probeTask = Task {
            try? await Task.sleep(nanoseconds: 500_000_000)
            guard !Task.isCancelled else { return }
            await connectionManager.probeEndpoints()
        }
    }

    private func applyPermissionPreset(_ preset: String) {
        guard let url = settings.gatewayURL else {
            permissionPreset = confirmedPermissionPreset
            return
        }
        Task {
            do {
                let state = try await APIClient.shared.updatePermissionPreset(preset: preset, baseURL: url)
                settings.cachedPermissionPreset = state.effectiveMode
                confirmedPermissionPreset = state.effectiveMode
                permissionPreset = state.effectiveMode
                NotificationCenter.default.post(name: .permissionPresetDidChange, object: state)
            } catch {
                permissionPreset = confirmedPermissionPreset
                permissionErrorMessage = error.localizedDescription
            }
        }
    }

    private var quotaDisplaySummary: String {
        let pace = settings.weeklyPaceCursorEnabled
        let picker = settings.modelPickerQuotaEnabled
        let window = settings.modelPickerQuotaWeekly ? String(localized: "每周") : "5h"
        switch (pace, picker) {
        case (true, true):
            return "\(String(localized: "匀速")) · \(window)"
        case (true, false):
            return String(localized: "匀速游标")
        case (false, true):
            return "\(String(localized: "余量")) · \(window)"
        case (false, false):
            return String(localized: "已关闭")
        }
    }

    private var compactModeSummary: String {
        guard settings.isKnowledgeModeEnabled else { return String(localized: "已关闭") }
        let count = [
            settings.compactCollapseTools,
            settings.compactCollapseTasks,
            settings.compactHideHealthyContext
        ].filter { $0 }.count
        return String(localized: "已开启 (\(count)/3)")
    }

    private var notificationSoundSummary: String {
        let completeName = NotificationSoundOption.displayName(
            for: settings.notificationCompleteSound,
            in: NotificationSoundOption.completeOptions
        )
        let actionName = NotificationSoundOption.displayName(
            for: settings.notificationActionSound,
            in: NotificationSoundOption.actionOptions
        )
        return "\(completeName) · \(actionName)"
    }

    private var appShareURL: URL? {
        let lang = (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "en").lowercased()
        if lang.hasPrefix("zh") {
            return URL(string: "https://multigravity.app/zh.html")
        } else if lang.hasPrefix("ja") {
            return URL(string: "https://multigravity.app/ja.html")
        } else {
            return URL(string: "https://multigravity.app/")
        }
    }

    private var appShareMessage: String {
        String(localized: "推荐 Multigravity —— Google Antigravity 原生移动客户端，支持实时审批、任务进度与额度监控")
    }
}

// MARK: - 提醒内容二级页

private struct NotificationAlertsSettingsView: View {
    @State private var pushManager = PushNotificationManager.shared
    @State private var settings = AppSettings.shared

    private var stallThresholdMinutes: Int {
        let m = pushManager.alertPreferences.stallMinutes
        return m > 0 ? m : 8
    }

    var body: some View {
        Form {
            Section(
                header: Text("任务与会话提醒"),
                footer: Text("审批请求与 Agent 提问在推送开启时始终提醒，确保离席不漏批。")
            ) {
                alertToggleRow(
                    title: String(localized: "任务完成与失败"),
                    subtitle: String(localized: "任务执行完毕或异常终止时提醒"),
                    key: "task_completion",
                    isOn: pushManager.alertPreferences.taskCompletion
                )
                alertToggleRow(
                    title: String(localized: "长任务疑似卡死"),
                    subtitle: String(localized: "连续 \(stallThresholdMinutes) 分钟无新输出或步骤推进时提醒"),
                    key: "task_stall",
                    isOn: pushManager.alertPreferences.taskStall
                )
                if pushManager.alertPreferences.isCacheMissLive {
                    alertToggleRow(
                        title: String(localized: "缓存连续未命中"),
                        subtitle: String(localized: "长任务连续 3 步大上下文未命中缓存时提醒"),
                        key: "cache_miss",
                        isOn: pushManager.alertPreferences.cacheMiss
                    )
                }
            }

            Section(
                header: Text("额度与配额提醒"),
                footer: Text(
                    pushManager.alertPreferencesSyncError
                        ?? (settings.isDemoMode
                            ? String(localized: "演示模式下仅预览开关状态，配对网关后实时生效。")
                            : String(localized: "开关实时同步至本机网关并跨设备生效。"))
                )
            ) {
                alertToggleRow(
                    title: String(localized: "低额度预警"),
                    subtitle: String(localized: "任一额度池降至 20% 或 5% 以下时提醒"),
                    key: "quota_low",
                    isOn: pushManager.alertPreferences.quotaLow
                )
                alertToggleRow(
                    title: String(localized: "额度提前重置提醒"),
                    subtitle: String(localized: "检测到官方非周期提前回满额度时提醒"),
                    key: "quota_bonus_reset",
                    isOn: pushManager.alertPreferences.quotaBonusReset
                )
                alertToggleRow(
                    title: String(localized: "异常骤降与静默扣额"),
                    subtitle: String(localized: "短时骤降或无活跃会话却发生扣额时提醒"),
                    key: "quota_anomaly",
                    isOn: pushManager.alertPreferences.quotaAnomaly
                )
            }
        }
        .navigationTitle("提醒内容")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await pushManager.fetchAlertPreferences()
        }
    }

    private func alertToggleRow(title: String, subtitle: String, key: String, isOn: Bool) -> some View {
        Toggle(isOn: Binding(
            get: { isOn },
            set: { newValue in
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                Task {
                    await pushManager.updateAlertPreference(key: key, value: newValue)
                }
            }
        )) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                Text(subtitle)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
    }
}

// MARK: - 通知提示音二级页

private struct NotificationSoundSettingsView: View {
    @State private var settings = AppSettings.shared
    @State private var pushManager = PushNotificationManager.shared

    var body: some View {
        Form {
            Section(
                header: Text("任务完成"),
                footer: Text("AI 完成全部步骤时播放。点击任意选项可立即试听并自动同步至网关。")
            ) {
                ForEach(NotificationSoundOption.completeOptions) { option in
                    soundRow(
                        option: option,
                        isSelected: settings.notificationCompleteSound == option.id
                    ) {
                        selectCompleteSound(option.id)
                    }
                }
            }

            Section(
                header: Text("需要确认"),
                footer: Text("命令审批、文件权限或计划待确认时播放，建议使用辨识度较高的提醒音。")
            ) {
                ForEach(NotificationSoundOption.actionOptions) { option in
                    soundRow(
                        option: option,
                        isSelected: settings.notificationActionSound == option.id
                    ) {
                        selectActionSound(option.id)
                    }
                }
            }

            if settings.notificationCompleteSound != "glass" || settings.notificationActionSound != "alarm" {
                Section {
                    Button {
                        settings.notificationActionSound = "alarm"
                        selectCompleteSound("glass")
                    } label: {
                        HStack {
                            Spacer()
                            Text("恢复默认提示音（叮叮 / 滴嘟）")
                                .foregroundColor(.indigo)
                            Spacer()
                        }
                    }
                }
            }
        }
        .navigationTitle("通知提示音")
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder
    private func soundRow(
        option: NotificationSoundOption,
        isSelected: Bool,
        onTap: @escaping () -> Void
    ) -> some View {
        Button(action: onTap) {
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(option.displayName)
                        .font(.system(size: 15))
                        .foregroundColor(.primary)
                    Text(option.subtitle)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                }

                Spacer()

                if option.id != "silence" {
                    Image(systemName: isSelected ? "speaker.wave.2.fill" : "speaker.wave.2")
                        .font(.system(size: 13))
                        .foregroundColor(isSelected ? .indigo : .secondary.opacity(0.65))
                }

                Image(systemName: "checkmark")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.indigo)
                    .opacity(isSelected ? 1 : 0)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func selectCompleteSound(_ soundId: String) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        settings.notificationCompleteSound = soundId
        pushManager.previewNotificationSound(soundId)
        pushManager.syncPreferencesWithGateway()
    }

    private func selectActionSound(_ soundId: String) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        settings.notificationActionSound = soundId
        pushManager.previewNotificationSound(soundId)
        pushManager.syncPreferencesWithGateway()
    }
}

// MARK: - 额度显示二级页

private struct QuotaDisplaySettingsView: View {
    @State private var settings = AppSettings.shared

    var body: some View {
        Form {
            Section(
                header: Text("配额条"),
                footer: Text("在首页与配额条上显示时间均速基准刻度针。")
            ) {
                Toggle("额度匀速指标", isOn: $settings.weeklyPaceCursorEnabled)
                    .onChange(of: settings.weeklyPaceCursorEnabled) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }
            }

            Section(
                header: Text("模型选择器"),
                footer: Text("长按模型胶囊上滑时，在每个模型行右侧显示所属额度池（Gemini / Claude）的剩余百分比，低于 20% 标红。")
            ) {
                Toggle("显示余量", isOn: $settings.modelPickerQuotaEnabled)
                    .onChange(of: settings.modelPickerQuotaEnabled) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }

                HStack {
                    Text("余量窗口")
                    Spacer()
                    Picker("余量窗口", selection: $settings.modelPickerQuotaWeekly) {
                        Text("5h").tag(false)
                        Text("每周").tag(true)
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    .frame(width: 140)
                    .onChange(of: settings.modelPickerQuotaWeekly) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }
                }
                .disabled(!settings.modelPickerQuotaEnabled)
                .opacity(settings.modelPickerQuotaEnabled ? 1.0 : 0.48)
            }
        }
        .navigationTitle("额度显示")
        .navigationBarTitleDisplayMode(.inline)
    }
}

// MARK: - 精简模式二级页

private struct CompactModeSettingsView: View {
    @State private var settings = AppSettings.shared

    var body: some View {
        Form {
            Section(
                footer: Text("开启后可折叠对话中的工具执行过程与底部任务辅助栏，让长对话阅读更聚焦。")
            ) {
                Toggle("开启精简模式", isOn: $settings.isKnowledgeModeEnabled)
                    .onChange(of: settings.isKnowledgeModeEnabled) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }
            }

            Section("降噪选项") {
                compactSubToggle(
                    title: "折叠工具执行过程",
                    subtitle: "将检索与命令步骤收起为单行摘要",
                    isOn: $settings.compactCollapseTools
                )
                compactSubToggle(
                    title: "收起任务辅助栏",
                    subtitle: "底部运行任务默认单行折叠，保留终止按钮",
                    isOn: $settings.compactCollapseTasks
                )
                compactSubToggle(
                    title: "隐藏常规上下文",
                    subtitle: "占用 < 70% 时隐藏顶栏胶囊，≥ 70% 自动恢复预警",
                    isOn: $settings.compactHideHealthyContext
                )
            }
            .disabled(!settings.isKnowledgeModeEnabled)
            .opacity(settings.isKnowledgeModeEnabled ? 1.0 : 0.48)
        }
        .navigationTitle("精简模式")
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder
    private func compactSubToggle(
        title: LocalizedStringKey,
        subtitle: LocalizedStringKey,
        isOn: Binding<Bool>
    ) -> some View {
        Toggle(isOn: isOn) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.system(size: 14.5))
                Text(subtitle)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
            }
        }
        .onChange(of: isOn.wrappedValue) { _, _ in
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        }
    }
}


// MARK: - 模型

extension SettingsSheet {
    fileprivate var favoriteModelFooter: String {
        if modelCatalog.isLoading && modelCatalog.catalog == nil { return String(localized: "正在读取可用模型...") }
        if modelCatalog.loadFailed && modelCatalog.catalog == nil {
            return String(localized: "暂时无法读取实时模型列表，可先从内置列表中选择模型；新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。")
        }
        return String(localized: "新会话默认使用首选模型，已有会话自动同步电脑端最新模型；短按模型胶囊切换首选与次选，长按可任选模型。")
    }

    private func providerSectionTitle(_ key: String) -> String {
        switch key {
        case "gemini": return "Gemini"
        case "claude": return "Claude"
        case "orchestrator": return "Orchestrator"
        default: return String(localized: "其他模型")
        }
    }

    /// 列表来自网关实时取到的全部可用模型（离线回退到内置列表），按厂商分组并保留全部思考强度。
    @ViewBuilder
    fileprivate func favoriteModelPicker(
        title: String,
        subtitle: String? = nil,
        selection: Binding<String>
    ) -> some View {
        let all = modelCatalog.allOptions
        let groups: [(key: String, items: [ModelOption])] = ["gemini", "claude", "orchestrator", "other"].compactMap { key in
            let items = all.filter { opt in
                let kind = ModelDefaultsLogic.providerKind(forID: opt.id).rawValue
                let resolved = (kind == "orchestrator" || opt.provider.isEmpty) ? kind : opt.provider.lowercased()
                if key == "other" {
                    return !["gemini", "claude", "orchestrator"].contains(resolved)
                }
                return resolved == key
            }
            return items.isEmpty ? nil : (key, items)
        }
        let selectedDisplayLabel: String = {
            if let opt = all.first(where: { $0.id == selection.wrappedValue }) {
                return ModelDefaultsLogic.formatPickerLabel(name: opt.name, tag: opt.tag)
            }
            return modelCatalog.resolvedDisplayName(forID: selection.wrappedValue)
        }()

        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
                if let subtitle {
                    Text(subtitle)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            }
            .layoutPriority(1)

            Spacer(minLength: 8)

            Menu {
                if !all.contains(where: { $0.id == selection.wrappedValue }) {
                    Button {
                        selection.wrappedValue = selection.wrappedValue
                    } label: {
                        Label(modelCatalog.resolvedDisplayName(forID: selection.wrappedValue), systemImage: "checkmark")
                    }
                }
                ForEach(groups, id: \.key) { group in
                    Section(providerSectionTitle(group.key)) {
                        ForEach(group.items) { option in
                            let label = ModelDefaultsLogic.formatPickerLabel(name: option.name, tag: option.tag)
                            Button {
                                selection.wrappedValue = option.id
                            } label: {
                                if selection.wrappedValue == option.id {
                                    Label(label, systemImage: "checkmark")
                                } else {
                                    Text(label)
                                }
                            }
                        }
                    }
                }
            } label: {
                HStack(spacing: 4) {
                    Text(selectedDisplayLabel)
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.75))
                }
                .contentShape(Rectangle())
            }
            .disabled(all.isEmpty)
        }
    }
}

// MARK: - 隐私政策与开源许可

private struct PrivacyPolicySheet: View {
    let onOpenWeb: (URL) -> Void

    var body: some View {
        NavigationStack {
            List {
                Section(header: Text("数据收集与连接方式")) {
                    Text("Multigravity 不包含任何第三方广告 SDK、行为追踪或开发者云端遥测服务。所有会话消息、项目文件与终端输出仅在您的 iOS 设备与您手动配对的自托管网关之间直接传输。")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .padding(.vertical, 4)
                }

                Section(header: Text("本地存储")) {
                    Text("设备仅在本地保存网关主机地址、TLS 选项、配对鉴权凭证以及会话/UI 偏好缓存。您可以随时在设置中清空缓存或解除配对。")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .padding(.vertical, 4)
                }

                Section(header: Text("系统权限用途")) {
                    Text("相机仅用于扫描配对二维码；本地网络仅用于发现与连接局域网/VPN 网关；麦克风与语音识别仅用于语音输入；相册仅用于选取附件或保存图片。")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .padding(.vertical, 4)
                }

                Section(header: Text("演示模式")) {
                    Text("开启演示模式时，所有示例工程与会话均在设备本地离线生成，不会发起网络请求。")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .padding(.vertical, 4)
                }

                Section {
                    Button {
                        if let url = URL(string: "https://github.com/GHSaiMo/antigravity-mobile/blob/main/PRIVACY.md") {
                            onOpenWeb(url)
                        }
                    } label: {
                        HStack {
                            Text("在浏览器中查看完整英文/中文隐私政策")
                            Spacer()
                            Image(systemName: "arrow.up.right.square")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                }
            }
            .navigationTitle("隐私政策")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDragIndicator(.visible)
    }
}

private struct OpenSourceLicensesSheet: View {
    private let items: [(name: String, license: String, summary: String)] = [
        ("Multigravity", "MIT License", "Copyright (c) 2026 Multigravity Contributors"),
        ("ZLPhotoBrowser", "MIT License", "Lightweight iOS photo picker framework used for attachment selection."),
        ("Mermaid", "MIT License", "Diagram and flowchart rendering engine bundled for offline preview."),
        ("highlight.js", "BSD-3-Clause License", "Syntax highlighting library used in code and file preview."),
        ("Bark Sounds", "MIT License", "Notification alert sound effects by Finb/Bark.")
    ]

    var body: some View {
        NavigationStack {
            List {
                ForEach(items, id: \.name) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text(item.name)
                                .font(.headline)
                            Spacer()
                            Text(item.license)
                                .font(.caption)
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 2)
                                .background(Color.secondary.opacity(0.12))
                                .clipShape(Capsule())
                        }
                        Text(item.summary)
                            .font(.subheadline)
                            .foregroundColor(.secondary)
                    }
                    .padding(.vertical, 4)
                }
            }
            .navigationTitle("开源许可")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDragIndicator(.visible)
    }
}
