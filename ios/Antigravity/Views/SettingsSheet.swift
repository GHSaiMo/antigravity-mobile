import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var showClearCacheAlert: Bool = false
    @State private var showUnpairAlert: Bool = false
    
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
                
                // MARK: - 2. 网络与多通道路由 (二级菜单入口)
                Section(
                    header: Text("网络"),
                    footer: Text("配置局域网、外网 IPv6 及云端中继等路由通道。")
                ) {
                    NavigationLink(destination: NetworkSettingsView()) {
                        HStack {
                            Text("网络设置")
                            Spacer()
                            Text(currentNetworkSummary)
                                .foregroundColor(.secondary)
                        }
                    }
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
                        Text("1.0.0")
                            .foregroundColor(.secondary)
                    }
                }
            }
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDragIndicator(.visible)
        .alert("确定解除设备配对？", isPresented: $showUnpairAlert) {
            Button("取消", role: .cancel) {}
            Button("解除配对", role: .destructive) {
                Task {
                    await APIClient.shared.unpair()
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
    
    private var currentNetworkSummary: String {
        if let active = settings.activeServerURL, let url = URL(string: active) {
            let isCellular = NetworkTransport.shared.isCellular
            let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
            if let latency = activeLatency {
                return "\(desc) (\(Int(latency))ms)"
            }
            return desc
        }
        let count = settings.candidateEndpoints.count
        if count > 0 {
            return "已配置 \(count) 个通道"
        }
        return "未设置"
    }
    
    private var activeLatency: Double? {
        guard let active = settings.activeServerURL else { return nil }
        return connectionManager.endpointStatuses[active]?.latencyMs
    }
}
