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
            
            // MARK: - 2. 主连接 · 专属公网域名 (Cloudflare HTTPS)
            Section(
                header: Text("主连接 · 专属公网域名"),
                footer: Text("默认统一使用专属分配的 HTTPS 域名。无论在外使用蜂窝网络还是 Wi-Fi，无需 VPN 即可安全直连电脑。")
            ) {
                let cloudURL = settings.primaryCloudURL ?? ""
                let isCloudActive = (settings.activeServerURL == cloudURL && !cloudURL.isEmpty)
                let cloudStatus = connectionManager.endpointStatuses[cloudURL]
                
                VStack(alignment: .leading, spacing: 6) {
                    HStack {
                        Text("Cloudflare 专属域名")
                            .font(.system(size: 14, weight: .medium))
                        
                        Spacer()
                        
                        if isCloudActive {
                            Text("默认首选 (生效中)")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundColor(.green)
                        } else if !cloudURL.isEmpty {
                            Button("设为主连接") {
                                settings.activeServerURL = cloudURL
                                settings.rawServerURL = cloudURL
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            }
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.blue)
                        }
                        
                        if let status = cloudStatus, status.isReachable {
                            Text("\(Int(status.latencyMs))ms")
                                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                .foregroundColor(latencyColor(status.latencyMs))
                        }
                    }
                    
                    HStack {
                        Text(cloudURL.isEmpty ? "未配置专属公网域名 (扫码配对自动下发)" : cloudURL)
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundColor(cloudURL.isEmpty ? .secondary : .primary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                        
                        Spacer()
                        
                        if !cloudURL.isEmpty {
                            Button(action: {
                                UIPasteboard.general.string = cloudURL
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
                .padding(.vertical, 4)
            }
            
            // MARK: - 3. 备用连接 · 自定义与局域网
            Section(
                header: Text("备用连接 · 自定义与局域网"),
                footer: Text("扫码配对默认已填入电脑局域网 Wi-Fi 地址。在同一 Wi-Fi 下可手动切换为此通道，享受 0 延迟响应。")
            ) {
                endpointInputRow(
                    title: "局域网 Wi-Fi / 自定义地址",
                    placeholder: "如 http://192.168.1.50:58900",
                    text: Binding(
                        get: { settings.customServerURL ?? settings.lanServerURL ?? "" },
                        set: { settings.customServerURL = $0.isEmpty ? nil : $0 }
                    ),
                    urlString: settings.customServerURL ?? settings.lanServerURL
                )
            }
            
            // MARK: - 4. 链路探活与智能测速
            Section(
                header: Text("通道测速与链路检查"),
                footer: Text("并发探测专属公网域名与备用地址的健康状态并回显最新往返延迟。")
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
