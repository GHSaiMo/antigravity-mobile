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
            // MARK: - 1. 当前活动链路
            Section(header: Text("当前活动链路")) {
                HStack {
                    Text("当前通道")
                    Spacer()
                    Text(activeChannelTitle)
                        .foregroundColor(.secondary)
                }
                
                HStack {
                    Text("连接状态")
                    Spacer()
                    if settings.activeServerURL != nil {
                        HStack(spacing: 4) {
                            Circle()
                                .fill(Color.green)
                                .frame(width: 6, height: 6)
                            Text("已连接")
                                .foregroundColor(.secondary)
                        }
                    } else {
                        Text("未连接")
                            .foregroundColor(.secondary)
                    }
                }
                
                if let latency = activeChannelLatency {
                    HStack {
                        Text("网络延迟")
                        Spacer()
                        Text("\(Int(latency))ms")
                            .font(.system(size: 14, design: .monospaced))
                            .foregroundColor(latencyColor(latency))
                    }
                }
                
                if let active = settings.activeServerURL, !active.isEmpty {
                    HStack {
                        Text(active)
                            .font(.system(size: 12, design: .monospaced))
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
                }
            }
            
            // MARK: - 2. 智能并发探活与自动测速
            Section(
                header: Text("智能选路与测速"),
                footer: Text("并发探测所有已配置通道并自动优选延迟最低的链路。")
            ) {
                Button(action: {
                    Task {
                        await runSmartProbe()
                    }
                }) {
                    HStack {
                        Text("一键并发探活与测速")
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
                    Text(status)
                        .font(.system(size: 12))
                        .foregroundColor((testSuccess ?? false) ? .secondary : .red)
                }
            }
            
            // MARK: - 3. 多通道候选端点配置
            Section(
                header: Text("路由端点配置"),
                footer: Text("扫码配对后会自动填入所有可用通道，也可手动指定各端点地址。")
            ) {
                // 局域网 Wi-Fi
                endpointInputRow(
                    title: "局域网 Wi-Fi (LAN IPv4)",
                    placeholder: "未设置 (如 http://192.168.1.50:58900)",
                    text: Binding(
                        get: { settings.lanServerURL ?? "" },
                        set: { settings.lanServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.lanServerURL
                )
                
                // 外网直连 IPv6
                endpointInputRow(
                    title: "外网直连 (Public IPv6)",
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
                    placeholder: "未设置 (如 https://mac.yourdomain.com)",
                    text: Binding(
                        get: { settings.customServerURL ?? "" },
                        set: { settings.customServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.customServerURL
                )
            }
            
            // MARK: - 4. 路由策略指南
            Section(
                header: Text("智能多通道路由策略"),
                footer: Text("手机在同一 Wi-Fi 时优先局域网直连；外出移动网络时优先 IPv6 端到端直连；网络受限时自动通过云服务器中继兜底。")
            ) {
                HStack {
                    Text("1. 局域网优先")
                    Spacer()
                    Text("LAN First (~1ms)")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
                HStack {
                    Text("2. 蜂窝网络直连")
                    Spacer()
                    Text("Cellular IPv6")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
                HStack {
                    Text("3. 云服务器中继")
                    Spacer()
                    Text("Cloud Relay")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
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
        placeholder: String,
        text: Binding<String>,
        urlString: String?
    ) -> some View {
        let isActive = (urlString != nil && !urlString!.isEmpty && settings.activeServerURL == urlString)
        let probeStatus = (urlString != nil) ? connectionManager.endpointStatuses[urlString!] : nil
        
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(title)
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.primary)
                
                Spacer()
                
                if isActive {
                    Text("当前生效")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundColor(.green)
                }
            }
            
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
        }
        .padding(.vertical, 2)
    }
    
    // MARK: - 动态计算属性
    
    private var activeChannelTitle: String {
        guard let active = settings.activeServerURL, let url = URL(string: active) else {
            return "未配置或未选定通道"
        }
        let isCellular = NetworkTransport.shared.isCellular
        return AppSettings.describeEndpoint(url: url, isCellular: isCellular)
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
