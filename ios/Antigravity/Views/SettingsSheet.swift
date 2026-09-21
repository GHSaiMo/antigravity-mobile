import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var showClearCacheAlert: Bool = false
    @State private var showUnpairAlert: Bool = false
    @State private var lanAddress: String = ""
    @State private var customAddress: String = ""
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Form {
                // MARK: - 1. 设备配对与鉴权
                Section(
                    header: Text("设备配对"),
                    footer: Text("管理当前设备与 Mac 网关的配对状态。")
                ) {
                    HStack {
                        Text("配对状态")
                        Spacer()
                        if settings.isPaired {
                            HStack(spacing: 4) {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundColor(.green)
                                    .font(.system(size: 14))
                                Text("已配对")
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
                        Text("解除设备配对")
                    }
                }
                
                // MARK: - 2. 网络 (局域网与自定义平铺，主域名后台兜底)
                Section(header: Text("网络")) {
                    // 局域网 (扫码配对默认填写)
                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Text("局域网")
                            Spacer()
                            if isLanActive {
                                Text("生效中")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                            }
                        }
                        
                        HStack {
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
                            
                            if !lanAddress.isEmpty {
                                Button {
                                    lanAddress = ""
                                    settings.lanServerURL = nil
                                    Task {
                                        await connectionManager.probeEndpoints()
                                    }
                                } label: {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundColor(.secondary.opacity(0.6))
                                        .font(.system(size: 14))
                                }
                                .buttonStyle(.borderless)
                            }
                        }
                    }
                    .padding(.vertical, 2)
                    
                    // 自定义 (留给用户配置，如 Tailscale)
                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Text("自定义")
                            Spacer()
                            if isCustomActive {
                                Text("生效中")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                            }
                        }
                        
                        HStack {
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
                            
                            if !customAddress.isEmpty {
                                Button {
                                    customAddress = ""
                                    settings.customServerURL = nil
                                    Task {
                                        await connectionManager.probeEndpoints()
                                    }
                                } label: {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundColor(.secondary.opacity(0.6))
                                        .font(.system(size: 14))
                                }
                                .buttonStyle(.borderless)
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
                
                // MARK: - 5. 本地缓存
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
                
                // MARK: - 6. 关于
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
                        Text("1.0.1")
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
        .alert("确定解除设备配对？", isPresented: $showUnpairAlert) {
            Button("取消", role: .cancel) {}
            Button("解除配对", role: .destructive) {
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
            Text("解除配对将通知网关清理此设备绑定，并清除本地访问令牌与网关配置。")
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
    }
    
    // MARK: - 辅助计算属性
    
    private func isEndpointActive(_ urlString: String?) -> Bool {
        guard let target = urlString, !target.isEmpty else { return false }
        let active = settings.activeServerURL ?? settings.serverURL?.absoluteString
        guard let active = active, !active.isEmpty else { return false }
        let targetClean = target.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
        let activeClean = active.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
        return targetClean == activeClean
    }
    
    private var isLanActive: Bool {
        isEndpointActive(settings.lanServerURL)
    }
    
    private var isCustomActive: Bool {
        isEndpointActive(settings.customServerURL)
    }
}
