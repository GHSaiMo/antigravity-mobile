import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var showClearCacheAlert: Bool = false
    @State private var showUnpairAlert: Bool = false
    @State private var lanAddress: String = ""
    @State private var customAddress: String = ""
    @State private var modelCatalog = ModelCatalogStore.shared
    @State private var showMcpSkillsSheet: Bool = false
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Form {
                // MARK: - 1. 设备配对与鉴权
                Section(
                    header: Text("设备配对"),
                    footer: Text(settings.isDemoMode ? "当前为演示模式，数据均为模拟，不会连接任何电脑。" : "管理当前设备与 \(settings.gatewayPlatformDisplayName)的配对状态。")
                ) {
                    HStack {
                        Text("配对状态")
                        Spacer()
                        if settings.isPaired {
                            HStack(spacing: 4) {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundColor(.green)
                                    .font(.system(size: 14))
                                Text(settings.isDemoMode ? "演示模式" : "已配对")
                                    .foregroundColor(.secondary)
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
                        Text(settings.isDemoMode ? "退出演示模式" : "解除设备配对")
                    }
                }
                
                // MARK: - 2. 网络 (局域网与自定义平铺，主域名后台智能路由)
                Section(header: Text("网络")) {
                    // 局域网 (扫码配对默认填写)
                    VStack(alignment: .leading, spacing: 6) {
                        Text("局域网")
                        
                        TextField("如 http://192.168.1.50:58900", text: $lanAddress)
                            .font(.system(size: 13, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .keyboardType(.URL)
                            .onChange(of: lanAddress) { _, newValue in
                                let trimmed = newValue.trimmingCharacters(in: .whitespacesAndNewlines)
                                settings.lanServerURL = trimmed.isEmpty ? nil : trimmed
                                Task {
                                    await connectionManager.probeEndpoints()
                                }
                            }
                    }
                    .padding(.vertical, 2)
                    
                    // 自定义 (留给用户配置，如 Tailscale)
                    VStack(alignment: .leading, spacing: 6) {
                        Text("自定义")
                        
                        TextField("如 http://100.x.x.x:58900", text: $customAddress)
                            .font(.system(size: 13, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .keyboardType(.URL)
                            .onChange(of: customAddress) { _, newValue in
                                let trimmed = newValue.trimmingCharacters(in: .whitespacesAndNewlines)
                                settings.customServerURL = trimmed.isEmpty ? nil : trimmed
                                Task {
                                    await connectionManager.probeEndpoints()
                                }
                            }
                    }
                    .padding(.vertical, 2)
                }
                
                // MARK: - 3. 权限与自动化
                Section(
                    header: Text("权限"),
                    footer: Text("自动批准 Agent 工具调用与执行权限，无需每次手动确认。")
                ) {
                    Toggle("自动批准操作权限", isOn: $settings.autoApprovePermissions)
                        .onChange(of: settings.autoApprovePermissions) { _, enabled in
                            if enabled {
                                UINotificationFeedbackGenerator().notificationOccurred(.warning)
                            }
                        }
                }
                
                // MARK: - 4. 灵动岛与实时活动
                Section(
                    header: Text("实时活动"),
                    footer: Text("在灵动岛和锁屏上显示任务进展及后台命令。")
                ) {
                    Toggle("灵动岛与实时活动", isOn: $settings.enableLiveActivities)
                        .onChange(of: settings.enableLiveActivities) { _, _ in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        }
                }
                
                // MARK: - 4a. 实时语音对话
                Section(
                    header: Text("语音"),
                    footer: Text("开启后，输入框为空且空闲时右侧按钮将变为实时语音对话入口（单击开始双工通话，长按离线录音）；关闭时保持原有的发送按钮。")
                ) {
                    Toggle("实时语音对话", isOn: $settings.enableVoiceMode)
                        .onChange(of: settings.enableVoiceMode) { _, enabled in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            if !enabled && VoiceCallManager.shared.isInCall {
                                VoiceCallManager.shared.endCall()
                            }
                        }
                }
                
                // MARK: - 4b. 默认模型
                Section(
                    header: Text("默认模型"),
                    footer: Text(defaultModelFooter)
                ) {
                    defaultModelPicker(title: "Gemini", provider: "gemini", selection: $settings.defaultGeminiModel)
                    defaultModelPicker(title: "Claude", provider: "claude", selection: $settings.defaultClaudeModel)
                    if !modelCatalog.options(for: "orchestrator").isEmpty {
                        defaultModelPicker(title: "Orchestrator", provider: "orchestrator", selection: $settings.defaultOrchestratorModel)
                    }
                }
                
                // MARK: - 4c. MCP 工具与技能看板
                Section(
                    header: Text("扩展与能力"),
                    footer: Text("查看当前已连接的 MCP 工具服务、运行健康状态与可调用的扩展技能。")
                ) {
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
                }
                
                // MARK: - 5. 外观与主题
                Section(
                    header: Text("外观与主题"),
                    footer: Text("选择浅色、深色模式与界面字号缩放；开启精简模式可收起工具执行细节、专注阅读正文；并可控制额度条上的时间均速刻度游标。")
                ) {
                    Picker("外观模式", selection: $settings.themeMode) {
                        Text("跟随系统").tag("system")
                        Text("浅色").tag("light")
                        Text("深色").tag("dark")
                    }
                    .pickerStyle(.menu)
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

                    Toggle("精简模式", isOn: $settings.isKnowledgeModeEnabled)
                        .onChange(of: settings.isKnowledgeModeEnabled) { _, _ in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        }

                    Toggle("额度匀速指标", isOn: $settings.weeklyPaceCursorEnabled)
                        .onChange(of: settings.weeklyPaceCursorEnabled) { _, _ in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        }
                }

                // MARK: - 6. 存储 (整体下移一行，放在关于的上面)
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
                
                // MARK: - 7. 关于
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
                        Text(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.1.4")
                            .foregroundColor(.secondary)
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
        }
        .task {
            await connectionManager.probeEndpoints()
        }
        .task {
            if let url = settings.serverURL, !settings.isDemoMode {
                await modelCatalog.refresh(settings: settings, baseURL: url, force: true)
            }
        }
        .alert(settings.isDemoMode ? "确定退出演示模式？" : "确定解除设备配对？", isPresented: $showUnpairAlert) {
            Button("取消", role: .cancel) {}
            Button(settings.isDemoMode ? "退出演示" : "解除配对", role: .destructive) {
                let currentURL = settings.serverURL
                let currentCandidates = settings.candidateEndpoints.compactMap { URL(string: $0.urlString) }
                let currentToken = settings.deviceToken
                let currentDeviceID = settings.deviceID
                
                Task {
                    await APIClient.shared.unpair(
                        baseURL: currentURL,
                        candidates: currentCandidates,
                        token: currentToken,
                        deviceID: currentDeviceID
                    )
                }
                settings.unpair()
                CacheManager.shared.clearCache()
                DocumentCacheManager.shared.clearCache()
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
                dismiss()
            }
        } message: {
            Text(settings.isDemoMode
                 ? "将退出演示模式并回到欢迎页，演示数据会被清除。"
                 : "解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。")
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
    }
}


// MARK: - 默认模型

extension SettingsSheet {
    fileprivate var defaultModelFooter: String {
        if modelCatalog.isLoading && modelCatalog.catalog == nil { return "正在读取可用模型..." }
        if modelCatalog.loadFailed && modelCatalog.catalog == nil { return "暂时无法读取可用模型列表，仍会使用当前已保存的默认模型。" }
        if modelCatalog.hasOrchestratorModels {
            return "输入框上方短按在 Gemini / Claude / Orchestrator 间切换，长按上滑并右滑选择模型及思考强度；已有会话自动跟随桌面当前模型，更改后随消息发送生效。"
        }
        return "输入框上方短按切换 Gemini 与 Claude，长按上滑并右滑选择模型及思考强度；已有会话自动跟随桌面当前模型，更改后随消息发送生效。"
    }

    /// 列表来自网关实时取到的可用模型；已保存的模型不在列表里时显示统一可读名称，避免露出小写裸 ID。
    @ViewBuilder
    fileprivate func defaultModelPicker(title: String, provider: String, selection: Binding<String>) -> some View {
        let options = modelCatalog.options(for: provider)
        Picker(title, selection: selection) {
            if !options.contains(where: { $0.id == selection.wrappedValue }) {
                Text(modelCatalog.resolvedDisplayName(forID: selection.wrappedValue)).tag(selection.wrappedValue)
            }
            ForEach(options) { option in
                let label = ModelDefaultsLogic.formatPickerLabel(name: option.name, tag: option.tag)
                Text(label).tag(option.id)
            }
        }
        .pickerStyle(.menu)
        .disabled(options.isEmpty)
    }
}
