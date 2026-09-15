import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var testStatus: String? = nil
    @State private var isTesting: Bool = false
    @State private var showQRScanner: Bool = false
    
    public init() {}
    
    public var body: some View {
        VStack(spacing: 0) {
            // Floating grab handle hinting pull-down dismissal
            Capsule()
                .fill(Color(uiColor: .tertiaryLabel))
                .frame(width: 38, height: 5)
                .padding(.top, 10)
                .padding(.bottom, 20)
            
            // Header title
            Text("设置")
                .font(.system(size: 18, weight: .bold))
                .foregroundColor(.primary)
                .frame(maxWidth: .infinity)
                .padding(.bottom, 12)
            
            Divider()
            
            Form {
                Section(header: Text("设备扫码配对与鉴权"), footer: Text("扫描 Mac 网关终端显示的配对二维码，自动绑定设备专属凭证并完成长效免密直连。")) {
                    if settings.isPaired {
                        HStack {
                            Image(systemName: "checkmark.seal.fill")
                                .foregroundColor(.green)
                            VStack(alignment: .leading, spacing: 2) {
                                Text("已完成设备配对鉴权")
                                    .font(.system(size: 15, weight: .medium))
                                if let id = settings.deviceID {
                                    Text("设备 ID: \(id)")
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                }
                            }
                        }
                        
                        Button(action: { showQRScanner = true }) {
                            HStack {
                                Image(systemName: "qrcode.viewfinder")
                                Text("重新扫描配对二维码")
                            }
                        }
                        
                        Button(role: .destructive, action: {
                            settings.unpair()
                            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        }) {
                            HStack {
                                Image(systemName: "xmark.circle")
                                Text("解除此设备配对 (清除凭据)")
                            }
                        }
                    } else {
                        HStack {
                            Image(systemName: "exclamationmark.shield")
                                .foregroundColor(.orange)
                            Text("未配对设备（受保护接口将被拦截）")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                        }
                        
                        Button(action: { showQRScanner = true }) {
                            HStack {
                                Image(systemName: "qrcode.viewfinder")
                                Text("扫描二维码完成配对")
                            }
                            .foregroundColor(.indigo)
                        }
                    }
                }

                Section(
                    header: Text("已绑定的网络端点 (多通道智能路由)"),
                    footer: Text("扫码后自动同步局域网 Wi-Fi、外网 IPv6 与云服务器中继三层通道。在家优先走局域网（~1ms）；外出蜂窝网络默认走 IPv6 直连；连接公共 Wi-Fi 时自动走云服务器中继。")
                ) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("局域网 Wi-Fi 地址 (LAN IPv4)")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                        TextField("未设置 (如 http://192.168.1.50:58900)", text: Binding(
                            get: { settings.lanServerURL ?? "" },
                            set: { settings.lanServerURL = $0.isEmpty ? nil : $0 }
                        ))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    }
                    .padding(.vertical, 2)
                    
                    VStack(alignment: .leading, spacing: 4) {
                        Text("外网直连地址 (Public IPv6)")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                        TextField("未设置 (如 http://[2001:db8::1]:58900)", text: Binding(
                            get: { settings.ipv6ServerURL ?? "" },
                            set: { settings.ipv6ServerURL = $0.isEmpty ? nil : $0 }
                        ))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    }
                    .padding(.vertical, 2)
                    
                    VStack(alignment: .leading, spacing: 4) {
                        Text("云服务器中继地址 (Cloud Relay IPv4)")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                        TextField("未设置 (如 http://relay.example.com:58900)", text: Binding(
                            get: { settings.relayServerURL ?? "" },
                            set: { settings.relayServerURL = $0.isEmpty ? nil : $0 }
                        ))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    }
                    .padding(.vertical, 2)
                    
                    VStack(alignment: .leading, spacing: 4) {
                        Text("自定义域名 / DDNS / Tailscale")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                        TextField("未设置 (如 https://mac.yourdomain.com)", text: Binding(
                            get: { settings.customServerURL ?? "" },
                            set: { settings.customServerURL = $0.isEmpty ? nil : $0 }
                        ))
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    }
                    .padding(.vertical, 2)
                    
                    if let active = settings.activeServerURL, !active.isEmpty {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("当前活动通道")
                                    .font(.system(size: 13))
                                if let activeURL = URL(string: active) {
                                    let isCellular = NetworkTransport.shared.isCellular
                                    Text(AppSettings.describeEndpoint(url: activeURL, isCellular: isCellular))
                                        .font(.system(size: 11, weight: .medium))
                                        .foregroundColor(.blue)
                                }
                            }
                            Spacer()
                            Text(active)
                                .font(.system(size: 12, design: .monospaced))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                    
                    Button(action: {
                        Task {
                            isTesting = true
                            testStatus = nil
                            _ = await ConnectionManager.shared.probeEndpoints()
                            testConnection()
                        }
                    }) {
                        HStack {
                            Image(systemName: "bolt.horizontal.circle")
                            Text("智能探活与测速")
                            Spacer()
                            if isTesting {
                                ProgressView()
                            } else if let status = testStatus {
                                Text(status)
                                    .font(.system(size: 12))
                                    .foregroundColor(status.contains("成功") ? .green : .red)
                            }
                        }
                    }
                    .disabled(isTesting)
                }
                
                Section(
                    header: Text("权限与自动化"),
                    footer: Text("开启后，当 Agent 请求外部 URL、网页抓取或文件路径读写权限时，将自动选择「Yes, and always allow」并提交，免去手动确认，适合无人值守连续执行。")
                ) {
                    Toggle("自动批准权限 (Always Allow)", isOn: $settings.autoApprovePermissions)
                }
                
                Section(header: Text("本地缓存"), footer: Text("已开启离线缓存与秒开机制。会话历史、方案 Markdown、PPTX 及 HTML 文档均自动保存到本地，二次进入 0 延迟秒开。")) {
                    Button(role: .destructive, action: {
                        CacheManager.shared.clearCache()
                        DocumentCacheManager.shared.clearCache()
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    }) {
                        HStack {
                            Image(systemName: "trash")
                            Text("清空本地会话与文档缓存")
                        }
                    }
                }
                
                Section(header: Text("关于")) {
                    HStack {
                        Text("应用名称")
                        Spacer()
                        Text("Antigravity")
                            .foregroundColor(.secondary)
                    }
                    HStack {
                        Text("版本")
                        Spacer()
                        Text("1.0.0 (Native)")
                            .foregroundColor(.secondary)
                    }
                }
            }
        }
        .background(Color(uiColor: .systemGroupedBackground))
        .presentationDragIndicator(.hidden)
        .sheet(isPresented: $showQRScanner) {
            QRScannerView()
        }
    }
    
    private func testConnection() {
        guard let url = settings.serverURL else {
            testStatus = "地址格式无效，请检查输入"
            return
        }
        
        isTesting = true
        testStatus = nil
        
        Task { @MainActor in
            do {
                let status = try await APIClient.shared.testConnection(baseURL: url)
                let isCellular = (status.usedInterface == "cellular") || (NetworkTransport.shared.isCellular && !NetworkTransport.shared.isWifi)
                let ifaceDesc = status.connectionDescription ?? AppSettings.describeEndpoint(url: url, isCellular: isCellular)
                
                // Fetch latency from recent probe if available
                var latencyPart = ""
                if let epStatus = ConnectionManager.shared.endpointStatuses[url.absoluteString] ??
                                  ConnectionManager.shared.endpointStatuses[settings.activeServerURL ?? ""],
                   epStatus.isReachable {
                    latencyPart = " · \(Int(epStatus.latencyMs))ms"
                }
                
                if status.status == "connected", let upstream = status.upstream {
                    testStatus = "连接成功 (PID \(upstream.pid ?? 0)\(latencyPart) · \(ifaceDesc))"
                } else {
                    testStatus = "网关在线，上游未就绪 (\(ifaceDesc))"
                }
            } catch {
                testStatus = "失败: \(error.localizedDescription)"
            }
            isTesting = false
        }
    }
}
