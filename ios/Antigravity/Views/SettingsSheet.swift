import SwiftUI

public struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared
    @State private var testStatus: String? = nil
    @State private var isTesting: Bool = false
    
    @State private var showCloudflareAuth = false
    
    public init() {}
    
    public var body: some View {
        NavigationStack {
            Form {
                Section(header: Text("服务器连接配置"), footer: Text("支持输入 Cloudflare Tunnel 外网域名（如 https://agy.yourdomain.com）或本地网关地址（如 http://127.0.0.1:58900）")) {
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
                
                Section(header: Text("Cloudflare 邮箱认证"), footer: Text("公网访问且开启了 Cloudflare Access 邮箱验证时，点击此按钮在 App 内输入邮箱收验证码完成授权。")) {
                    Button(action: { showCloudflareAuth = true }) {
                        HStack {
                            Image(systemName: "envelope.badge.shield.half.filled")
                                .foregroundColor(.blue)
                            Text("进行 Cloudflare 邮箱验证")
                                .foregroundColor(.primary)
                            Spacer()
                            Image(systemName: "chevron.right")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
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
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                }
            }
            .sheet(isPresented: $showCloudflareAuth) {
                if let url = settings.serverURL {
                    CloudflareLoginView(serverURL: url, onLoginSuccess: {
                        testConnection()
                    })
                }
            }
        }
    }
    
    private func testConnection() {
        guard let url = settings.serverURL else {
            testStatus = "地址格式无效"
            return
        }
        
        isTesting = true
        testStatus = nil
        
        Task {
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
