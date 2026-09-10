import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var testStatus: String? = nil
    @State private var isTesting: Bool = false
    @State private var showQRScanner: Bool = false
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
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

                Section(header: Text("服务器连接配置"), footer: Text("支持输入局域网/DDNS 地址（如 http://mac.yourdomain.com:58900）、IPv6 地址（如 [240e:...]:58900）或 Tailscale 虚拟 IP。注意：网关默认采用 http 协议。")) {
                    TextField("网关地址", text: $settings.rawServerURL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    
                    Button(action: testConnection) {
                        HStack {
                            Text("测试连接")
                            Spacer()
                            if isTesting {
                                ProgressView()
                            } else if let status = testStatus {
                                Text(status)
                                    .font(.system(size: 13))
                                    .foregroundColor(status.contains("成功") ? .green : .red)
                            }
                        }
                    }
                    .disabled(isTesting)
                }
                
                Section(header: Text("网络直连策略"), footer: Text("开启后，即使手机连接了局域网/Wi-Fi，也优先通过移动蜂窝网络（自带 IPv6）直连 Mac 端，解决外部公共 Wi-Fi 无 IPv6 导致的连接失败问题。")) {
                    Toggle("优先走手机蜂窝网络 (IPv6 直连)", isOn: $settings.preferCellularNetwork)
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
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                }
            }
            .sheet(isPresented: $showQRScanner) {
                QRScannerView()
            }
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
                if status.status == "connected", let upstream = status.upstream {
                    testStatus = "连接成功 (PID \(upstream.pid ?? 0))"
                } else {
                    testStatus = "网关在线，上游未就绪"
                }
            } catch {
                testStatus = "失败: \(error.localizedDescription)"
            }
            isTesting = false
        }
    }
}
