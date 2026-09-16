import SwiftUI

public struct NetworkSettingsView: View {
    @State private var settings = AppSettings.shared
    @State private var connectionManager = ConnectionManager.shared
    @State private var isTesting: Bool = false
    @State private var testStatus: String? = nil
    @State private var testSuccess: Bool? = nil
    @State private var showCopiedAlert: Bool = false
    
    public init() {}
    
    public var body: some View {
        Form {
            // MARK: - 1. 当前活动通道高光卡片
            Section(header: Text("当前活动链路")) {
                VStack(alignment: .leading, spacing: 10) {
                    HStack(alignment: .center, spacing: 12) {
                        // 动态链路图标
                        ZStack {
                            Circle()
                                .fill(activeChannelColor.opacity(0.15))
                                .frame(width: 44, height: 44)
                            Image(systemName: activeChannelIcon)
                                .font(.system(size: 20, weight: .semibold))
                                .foregroundColor(activeChannelColor)
                        }
                        
                        VStack(alignment: .leading, spacing: 3) {
                            HStack(spacing: 6) {
                                Text(activeChannelTitle)
                                    .font(.system(size: 16, weight: .semibold))
                                    .foregroundColor(.primary)
                                
                                if settings.activeServerURL != nil {
                                    HStack(spacing: 4) {
                                        Circle()
                                            .fill(Color.green)
                                            .frame(width: 6, height: 6)
                                        Text("已连接")
                                            .font(.system(size: 11, weight: .medium))
                                            .foregroundColor(.green)
                                    }
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(Color.green.opacity(0.12))
                                    .clipShape(Capsule())
                                }
                            }
                            
                            Text(activeChannelSubtitle)
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }
                        
                        Spacer()
                        
                        // 延迟显示
                        if let latency = activeChannelLatency {
                            HStack(spacing: 2) {
                                Image(systemName: "bolt.fill")
                                    .font(.system(size: 10))
                                Text("\(Int(latency))ms")
                                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                            }
                            .foregroundColor(latencyColor(latency))
                            .padding(.horizontal, 7)
                            .padding(.vertical, 4)
                            .background(latencyColor(latency).opacity(0.12))
                            .clipShape(Capsule())
                        }
                    }
                    
                    if let active = settings.activeServerURL, !active.isEmpty {
                        HStack {
                            Text(active)
                                .font(.system(size: 12, weight: .regular, design: .monospaced))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                            
                            Spacer()
                            
                            Button(action: {
                                UIPasteboard.general.string = active
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                showCopiedAlert = true
                            }) {
                                Image(systemName: "doc.on.doc")
                                    .font(.system(size: 13))
                                    .foregroundColor(.blue)
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(8)
                        .background(Color(uiColor: .tertiarySystemGroupedBackground))
                        .cornerRadius(8)
                    }
                }
                .padding(.vertical, 4)
            }
            
            // MARK: - 2. 智能并发探活与自动测速
            Section(
                header: Text("智能选路与测速"),
                footer: Text("自动并发探测所有已配置端点，智能根据当前 Wi-Fi / 蜂窝网络环境优选延迟最低的可达通道并无缝切换。")
            ) {
                Button(action: {
                    Task {
                        await runSmartProbe()
                    }
                }) {
                    HStack {
                        Image(systemName: "bolt.horizontal.circle.fill")
                            .font(.system(size: 16))
                            .foregroundColor(.blue)
                        Text("一键多通道并发探活与测速")
                            .font(.system(size: 15, weight: .medium))
                            .foregroundColor(.primary)
                        Spacer()
                        if isTesting {
                            ProgressView()
                                .scaleEffect(0.8)
                        } else if let success = testSuccess {
                            Image(systemName: success ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                                .foregroundColor(success ? .green : .orange)
                        }
                    }
                }
                .disabled(isTesting)
                
                if let status = testStatus {
                    HStack(alignment: .top, spacing: 8) {
                        Image(systemName: (testSuccess ?? false) ? "antenna.radiowaves.left.and.right" : "info.circle")
                            .font(.system(size: 13))
                            .foregroundColor((testSuccess ?? false) ? .green : .orange)
                            .padding(.top, 2)
                        
                        Text(status)
                            .font(.system(size: 12))
                            .foregroundColor((testSuccess ?? false) ? .primary : .secondary)
                    }
                    .padding(.vertical, 2)
                }
            }
            
            // MARK: - 3. 多通道候选端点配置
            Section(
                header: Text("多通道路由端点配置"),
                footer: Text("扫码配对后会自动填入所有可用通道。系统在不同网络环境下（Wi-Fi / 蜂窝 / 公用网络）自动优选并秒级切换。")
            ) {
                // 局域网 Wi-Fi
                endpointInputRow(
                    title: "局域网 Wi-Fi (LAN IPv4)",
                    badge: "优先 ~1ms",
                    badgeColor: .green,
                    icon: "wifi",
                    iconColor: .green,
                    hint: "在家/办公室同一 Wi-Fi 直连，极低延迟免公网流量",
                    placeholder: "未设置 (如 http://192.168.1.50:58900)",
                    text: Binding(
                        get: { settings.lanServerURL ?? "" },
                        set: { settings.lanServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.lanServerURL
                )
                
                // 外网直连 IPv6
                endpointInputRow(
                    title: "外网直连地址 (Public IPv6)",
                    badge: "蜂窝直连",
                    badgeColor: .blue,
                    icon: "globe.asia.australia.fill",
                    iconColor: .blue,
                    hint: "外出蜂窝网络 5G/4G 端到端直连，延迟约 20-30ms",
                    placeholder: "未设置 (如 http://[2001:db8::1]:58900)",
                    text: Binding(
                        get: { settings.ipv6ServerURL ?? "" },
                        set: { settings.ipv6ServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.ipv6ServerURL
                )
                
                // 云服务器中继
                endpointInputRow(
                    title: "云服务器中继 (Cloud Relay IPv4)",
                    badge: "全网兜底",
                    badgeColor: .purple,
                    icon: "icloud.fill",
                    iconColor: .purple,
                    hint: "公共 Wi-Fi 或无 IPv6 隔离环境下的高稳定备选",
                    placeholder: "未设置 (如 http://relay.example.com:58900)",
                    text: Binding(
                        get: { settings.relayServerURL ?? "" },
                        set: { settings.relayServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.relayServerURL
                )
                
                // 自定义域名 / DDNS / Tailscale
                endpointInputRow(
                    title: "自定义域名 / DDNS / Tailscale",
                    badge: "专属穿透",
                    badgeColor: .orange,
                    icon: "link.circle.fill",
                    iconColor: .orange,
                    hint: "反向代理域名、动态域名或 Tailscale 虚拟网",
                    placeholder: "未设置 (如 https://mac.yourdomain.com)",
                    text: Binding(
                        get: { settings.customServerURL ?? "" },
                        set: { settings.customServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.customServerURL
                )
            }
            
            // MARK: - 4. 路由策略指南
            Section(header: Text("智能多通道路由策略")) {
                VStack(alignment: .leading, spacing: 12) {
                    policyItem(
                        icon: "house.fill",
                        color: .green,
                        title: "1. 局域网优先 (LAN First)",
                        desc: "手机与电脑连接同一 Wi-Fi 时，优先直连内网，延迟仅 1ms，传输完全不消耗蜂窝流量。"
                    )
                    
                    Divider()
                    
                    policyItem(
                        icon: "antenna.radiowaves.left.and.right",
                        color: .blue,
                        title: "2. 蜂窝网络 IPv6 直连 (Cellular IPv6)",
                        desc: "外出使用蜂窝网络时，优先走运营商公网 IPv6 端到端直连，无需中间云服务器转发。"
                    )
                    
                    Divider()
                    
                    policyItem(
                        icon: "shield.lefthalf.filled",
                        color: .purple,
                        title: "3. 云中继稳定兜底 (Cloud Relay Fallback)",
                        desc: "连接受限公共 Wi-Fi 或缺乏 IPv6 环境时，自动无缝切换到云服务器中继保障随时在线。"
                    )
                }
                .padding(.vertical, 6)
            }
        }
        .navigationTitle("网络设置")
        .navigationBarTitleDisplayMode(.inline)
        .overlay(alignment: .bottom) {
            if showCopiedAlert {
                Text("地址已复制到剪贴板")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.white)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 8)
                    .background(Color.black.opacity(0.75))
                    .clipShape(Capsule())
                    .padding(.bottom, 20)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .onAppear {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                            withAnimation { showCopiedAlert = false }
                        }
                    }
            }
        }
    }
    
    // MARK: - 子组件与辅助方法
    
    @ViewBuilder
    private func endpointInputRow(
        title: String,
        badge: String,
        badgeColor: Color,
        icon: String,
        iconColor: Color,
        hint: String,
        placeholder: String,
        text: Binding<String>,
        urlString: String?
    ) -> some View {
        let isActive = (urlString != nil && !urlString!.isEmpty && settings.activeServerURL == urlString)
        let probeStatus = (urlString != nil) ? connectionManager.endpointStatuses[urlString!] : nil
        
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Image(systemName: icon)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(iconColor)
                    .frame(width: 20)
                
                Text(title)
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.primary)
                
                Spacer()
                
                // 生效标记或类型标签
                if isActive {
                    HStack(spacing: 3) {
                        Circle()
                            .fill(Color.green)
                            .frame(width: 5, height: 5)
                        Text("当前生效")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(.green)
                    }
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color.green.opacity(0.12))
                    .clipShape(Capsule())
                } else {
                    Text(badge)
                        .font(.system(size: 10, weight: .medium))
                        .foregroundColor(badgeColor)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1.5)
                        .background(badgeColor.opacity(0.1))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
            }
            
            Text(hint)
                .font(.system(size: 11))
                .foregroundColor(.secondary)
            
            HStack {
                TextField(placeholder, text: text)
                    .font(.system(size: 13, design: .monospaced))
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                
                if !text.wrappedValue.isEmpty {
                    Button(action: {
                        text.wrappedValue = ""
                    }) {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundColor(.secondary.opacity(0.6))
                            .font(.system(size: 14))
                    }
                    .buttonStyle(.plain)
                }
                
                // 测速结果微标签
                if let status = probeStatus {
                    if status.isReachable {
                        Text("\(Int(status.latencyMs))ms")
                            .font(.system(size: 11, weight: .semibold, design: .monospaced))
                            .foregroundColor(latencyColor(status.latencyMs))
                    } else {
                        Text("不可达")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.red)
                    }
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(Color(uiColor: .tertiarySystemGroupedBackground))
            .cornerRadius(8)
        }
        .padding(.vertical, 4)
    }
    
    @ViewBuilder
    private func policyItem(icon: String, color: Color, title: String, desc: String) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: icon)
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(color)
                .frame(width: 22, height: 22)
                .background(color.opacity(0.12))
                .clipShape(RoundedRectangle(cornerRadius: 6))
            
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                Text(desc)
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .lineSpacing(2)
            }
        }
    }
    
    // MARK: - 动态计算属性
    
    private var activeChannelIcon: String {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return "network.slash"
        }
        let isCellular = NetworkTransport.shared.isCellular
        let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
        if desc.contains("局域网") {
            return "wifi"
        } else if desc.contains("IPv6") {
            return "globe.asia.australia.fill"
        } else if desc.contains("中继") {
            return "icloud.fill"
        } else {
            return "link"
        }
    }
    
    private var activeChannelColor: Color {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return .gray
        }
        let isCellular = NetworkTransport.shared.isCellular
        let desc = AppSettings.describeEndpoint(url: url, isCellular: isCellular)
        if desc.contains("局域网") {
            return .green
        } else if desc.contains("IPv6") {
            return .blue
        } else if desc.contains("中继") {
            return .purple
        } else {
            return .orange
        }
    }
    
    private var activeChannelTitle: String {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return "未配置或未选定通道"
        }
        let isCellular = NetworkTransport.shared.isCellular
        return AppSettings.describeEndpoint(url: url, isCellular: isCellular)
    }
    
    private var activeChannelSubtitle: String {
        let isCellular = NetworkTransport.shared.isCellular
        let netType = isCellular ? "当前处于蜂窝移动网络" : "当前已接入 Wi-Fi 网络"
        if settings.activeServerURL != nil {
            return "\(netType) · 智能多通道路由就绪"
        }
        return "\(netType) · 请点击下方按钮探测并连接"
    }
    
    private var activeChannelLatency: Double? {
        guard let active = settings.activeServerURL else { return nil }
        return connectionManager.endpointStatuses[active]?.latencyMs
    }
    
    private func latencyColor(_ ms: Double) -> Color {
        if ms <= 30 { return .green }
        if ms <= 100 { return .blue }
        if ms <= 250 { return .orange }
        return .red
    }
    
    // MARK: - 异步探活逻辑
    
    private func runSmartProbe() async {
        isTesting = true
        testStatus = "正在并发探测候选通道..."
        testSuccess = nil
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        
        _ = await connectionManager.probeEndpoints()
        
        guard let url = settings.serverURL else {
            testStatus = "未找到可用端点，请检查配置"
            testSuccess = false
            isTesting = false
            return
        }
        
        do {
            let status = try await APIClient.shared.testConnection(baseURL: url)
            let isCellular = (status.usedInterface == "cellular") || (NetworkTransport.shared.isCellular && !NetworkTransport.shared.isWifi)
            let ifaceDesc = status.connectionDescription ?? AppSettings.describeEndpoint(url: url, isCellular: isCellular)
            
            var latencyPart = ""
            if let epStatus = connectionManager.endpointStatuses[url.absoluteString] ??
                              connectionManager.endpointStatuses[settings.activeServerURL ?? ""],
               epStatus.isReachable {
                latencyPart = " · \(Int(epStatus.latencyMs))ms"
            }
            
            if status.status == "connected", let upstream = status.upstream {
                testStatus = "连接成功: PID \(upstream.pid ?? 0)\(latencyPart) (\(ifaceDesc))"
                testSuccess = true
                UINotificationFeedbackGenerator().notificationOccurred(.success)
            } else {
                testStatus = "网关在线但上游尚未就绪 (\(ifaceDesc))"
                testSuccess = true
            }
        } catch {
            testStatus = "探测失败: \(error.localizedDescription)"
            testSuccess = false
            UINotificationFeedbackGenerator().notificationOccurred(.error)
        }
        
        isTesting = false
    }
}
