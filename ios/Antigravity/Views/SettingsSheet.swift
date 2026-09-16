import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var showQRScanner: Bool = false
    @State private var showClearCacheAlert: Bool = false
    @State private var showUnpairAlert: Bool = false
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Form {
                // MARK: - 1. 设备扫码配对与鉴权
                Section(
                    header: Text("设备配对与鉴权"),
                    footer: Text("扫描 Mac 终端显示的配对二维码，自动绑定专属凭据并开启长效免密通道。")
                ) {
                    if settings.isPaired {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(Color.green.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: "checkmark.seal.fill")
                                    .font(.system(size: 18))
                                    .foregroundColor(.green)
                            }
                            
                            VStack(alignment: .leading, spacing: 2) {
                                Text("已完成设备配对鉴权")
                                    .font(.system(size: 15, weight: .medium))
                                    .foregroundColor(.primary)
                                if let id = settings.deviceID {
                                    Text("设备 ID: \(id)")
                                        .font(.system(size: 12, design: .monospaced))
                                        .foregroundColor(.secondary)
                                        .lineLimit(1)
                                        .truncationMode(.middle)
                                }
                            }
                        }
                        .padding(.vertical, 2)
                        
                        Button(action: { showQRScanner = true }) {
                            HStack(spacing: 8) {
                                Image(systemName: "qrcode.viewfinder")
                                    .font(.system(size: 15))
                                    .foregroundColor(.blue)
                                Text("重新扫描配对二维码")
                                    .font(.system(size: 15))
                                    .foregroundColor(.blue)
                            }
                        }
                        
                        Button(role: .destructive, action: {
                            showUnpairAlert = true
                        }) {
                            HStack(spacing: 8) {
                                Image(systemName: "xmark.circle")
                                    .font(.system(size: 15))
                                Text("解除此设备配对 (清除凭据)")
                                    .font(.system(size: 15))
                            }
                        }
                    } else {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(Color.orange.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: "exclamationmark.shield.fill")
                                    .font(.system(size: 18))
                                    .foregroundColor(.orange)
                            }
                            
                            VStack(alignment: .leading, spacing: 2) {
                                Text("设备未完成配对")
                                    .font(.system(size: 15, weight: .medium))
                                    .foregroundColor(.primary)
                                Text("未配对设备将无法连接网关受保护接口")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                        }
                        .padding(.vertical, 2)
                        
                        Button(action: { showQRScanner = true }) {
                            HStack(spacing: 8) {
                                Image(systemName: "qrcode.viewfinder")
                                    .font(.system(size: 15, weight: .medium))
                                Text("扫描二维码完成配对")
                                    .font(.system(size: 15, weight: .semibold))
                            }
                            .foregroundColor(.indigo)
                        }
                    }
                }
                
                // MARK: - 2. 网络与多通道路由 (二级菜单入口)
                Section(
                    header: Text("网络与连接"),
                    footer: Text("支持局域网 Wi-Fi 直连、外网 IPv6 直连、云服务器中继与自定义域名等多通道路由与智能探活。")
                ) {
                    NavigationLink(destination: NetworkSettingsView()) {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(networkStatusColor.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: networkStatusIcon)
                                    .font(.system(size: 17, weight: .medium))
                                    .foregroundColor(networkStatusColor)
                            }
                            
                            VStack(alignment: .leading, spacing: 3) {
                                HStack(spacing: 6) {
                                    Text("网络与多通道设置")
                                        .font(.system(size: 15, weight: .medium))
                                        .foregroundColor(.primary)
                                    
                                    if settings.activeServerURL != nil {
                                        Circle()
                                            .fill(Color.green)
                                            .frame(width: 6, height: 6)
                                    }
                                }
                                
                                Text(networkStatusSubtitle)
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                                    .lineLimit(1)
                                    .truncationMode(.tail)
                            }
                            
                            Spacer()
                            
                            if let latency = activeLatency {
                                Text("\(Int(latency))ms")
                                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                                    .foregroundColor(latency <= 30 ? .green : (latency <= 100 ? .blue : .orange))
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(Color(uiColor: .tertiarySystemGroupedBackground))
                                    .clipShape(Capsule())
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
                
                // MARK: - 3. 权限与自动化
                Section(
                    header: Text("权限与自动化"),
                    footer: Text("默认关闭。开启后，Agent 申请的文件读写与外连权限会被自动批准为「始终允许」，等同于把本机文件与网络交给该会话。仅在你完全信任当前任务时打开。")
                ) {
                    Toggle(isOn: $settings.autoApprovePermissions) {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(Color.blue.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: "shield.lefthalf.filled")
                                    .font(.system(size: 17))
                                    .foregroundColor(.blue)
                            }
                            
                            VStack(alignment: .leading, spacing: 2) {
                                Text("自动批准权限 (Always Allow)")
                                    .font(.system(size: 15, weight: .medium))
                                Text("自动批准 Agent 工具调用与执行授权")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                        }
                    }
                    .onChange(of: settings.autoApprovePermissions) { _, enabled in
                        if enabled {
                            UINotificationFeedbackGenerator().notificationOccurred(.warning)
                        }
                    }
                }
                
                // MARK: - 4. 灵动岛与实时活动
                Section(
                    header: Text("灵动岛与实时活动"),
                    footer: Text("开启后，当 Agent 执行推理或后台有终端任务运行时，在 iPhone 灵动岛与锁屏实时显示步骤、任务数与命令详情。轻触灵动岛可直接回到对应会话。")
                ) {
                    Toggle(isOn: $settings.enableLiveActivities) {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(Color.indigo.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: "dot.radiowaves.up.forward")
                                    .font(.system(size: 17))
                                    .foregroundColor(.indigo)
                            }
                            
                            VStack(alignment: .leading, spacing: 2) {
                                Text("灵动岛与锁屏实时活动")
                                    .font(.system(size: 15, weight: .medium))
                                Text("展示推理进展、后台任务与命令状态")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                        }
                    }
                    .onChange(of: settings.enableLiveActivities) { _, _ in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    }
                }
                
                // MARK: - 5. 本地缓存
                Section(
                    header: Text("本地缓存"),
                    footer: Text("已开启离线缓存与秒开机制。会话历史、方案 Markdown、PPTX 及 HTML 文档均自动保存到本地，二次进入 0 延迟秒开。")
                ) {
                    Button(role: .destructive, action: {
                        showClearCacheAlert = true
                    }) {
                        HStack(spacing: 12) {
                            ZStack {
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(Color.red.opacity(0.12))
                                    .frame(width: 36, height: 36)
                                Image(systemName: "trash.fill")
                                    .font(.system(size: 16))
                                    .foregroundColor(.red)
                            }
                            
                            Text("清空本地会话与文档缓存")
                                .font(.system(size: 15, weight: .medium))
                        }
                    }
                }
                
                // MARK: - 6. 关于
                Section(header: Text("关于")) {
                    HStack {
                        Text("应用名称")
                            .font(.system(size: 15))
                        Spacer()
                        Text("Antigravity Mobile")
                            .font(.system(size: 15))
                            .foregroundColor(.secondary)
                    }
                    HStack {
                        Text("版本")
                            .font(.system(size: 15))
                        Spacer()
                        Text("1.0.0 (Native Swift)")
                            .font(.system(size: 15))
                            .foregroundColor(.secondary)
                    }
                }
            }
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                    .font(.system(size: 16, weight: .semibold))
                }
            }
        }
        .sheet(isPresented: $showQRScanner) {
            QRScannerView()
        }
        .alert("确定解除设备配对？", isPresented: $showUnpairAlert) {
            Button("取消", role: .cancel) {}
            Button("解除配对", role: .destructive) {
                settings.unpair()
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            }
        } message: {
            Text("解除配对将清除此设备的访问令牌与已保存的网关地址。")
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
    
    private var networkStatusIcon: String {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return "network"
        }
        let isCellular = NetworkTransport.shared.isCellular
        let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
        if desc.contains("局域网") { return "wifi" }
        if desc.contains("IPv6") { return "globe.asia.australia.fill" }
        if desc.contains("中继") { return "icloud.fill" }
        return "link"
    }
    
    private var networkStatusColor: Color {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return .blue
        }
        let isCellular = NetworkTransport.shared.isCellular
        let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
        if desc.contains("局域网") { return .green }
        if desc.contains("IPv6") { return .blue }
        if desc.contains("中继") { return .purple }
        return .orange
    }
    
    private var networkStatusSubtitle: String {
        if let active = settings.activeServerURL, let url = URL(string: active) {
            let isCellular = NetworkTransport.shared.isCellular
            let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
            return "\(desc) · 点击管理端点与测速"
        }
        let count = settings.candidateEndpoints.count
        if count > 0 {
            return "已配置 \(count) 个候选通道 · 点击管理"
        }
        return "尚未配置端点 · 点击设置"
    }
    
    private var activeLatency: Double? {
        guard let active = settings.activeServerURL else { return nil }
        return connectionManager.endpointStatuses[active]?.latencyMs
    }
}
