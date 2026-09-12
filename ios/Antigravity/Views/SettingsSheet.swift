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
                    header: Text("已绑定的网络端点 (多网址自适应)"),
                    footer: Text("扫码后会自动同步局域网 Wi-Fi 与公网 IPv6 双网址。在家同一 Wi-Fi 下优先走局域网（极速秒连），出门在外自动秒切 IPv6 直连。")
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
                        TextField("未设置 (如 http://[240e:...]:58900)", text: Binding(
                            get: { settings.ipv6ServerURL ?? "" },
                            set: { settings.ipv6ServerURL = $0.isEmpty ? nil : $0 }
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
                                    let isCellular = settings.preferCellularNetwork || NetworkTransport.shared.isCellular
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
                
                Section(header: Text("网络直连策略"), footer: Text("同局域网下系统始终极速走局域网（~1ms）；外出连接无 IPv6 的外部公共 Wi-Fi 时，开启此项将在局域网不通时优先走蜂窝 IPv6 直连。提示：若外部 Wi-Fi 限制双网并发，在 iOS 控制中心临时断开 Wi-Fi 即可秒切 5G 极速直连。")) {
                    Toggle("蜂窝网络优先", isOn: $settings.preferCellularNetwork)
                    
                    if settings.preferCellularNetwork {
                        if let v6 = settings.ipv6ServerURL, !v6.isEmpty {
                            HStack {
                                Image(systemName: "antenna.radiowaves.left.and.right")
                                    .foregroundColor(.blue)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("已绑定 IPv6 直连通道")
                                        .font(.system(size: 13, weight: .medium))
                                    Text(v6)
                                        .font(.system(size: 11, design: .monospaced))
                                        .foregroundColor(.secondary)
                                        .lineLimit(1)
                                }
                            }
                        } else {
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: "exclamationmark.triangle.fill")
                                    .foregroundColor(.orange)
                                Text("未检测到已保存的 IPv6 地址。请在上方「外网直连地址」中填入 Mac 终端显示的 IPv6 地址，或重新扫码。")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                            }
                        }
                    }
                }

                
                Section(header: Text("本地缓存"), footer: Text("已开启离线缓存与秒开机制。会话列表与对话历史自动保存到本地，二次进入 0 延迟秒开。")) {
                    Button(role: .destructive, action: {
                        CacheManager.shared.clearCache()
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    }) {
                        HStack {
                            Image(systemName: "trash")
                            Text("清空本地会话与历史缓存")
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
                let isCellular = (status.usedInterface == "cellular") || (settings.preferCellularNetwork && !NetworkTransport.isLocalOrPrivateHost(url.host ?? "")) || (NetworkTransport.shared.isCellular && !NetworkTransport.shared.isWifi)
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
