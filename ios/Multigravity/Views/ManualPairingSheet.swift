import SwiftUI
import UIKit

/// 类似于 Android 版的手动配对输入面板，使用 iOS 原生毛玻璃材质组件构建。
public struct ManualPairingSheet: View {
    @Environment(\.dismiss) private var dismiss
    
    public var onPair: (PairingInfo) -> Void
    
    @State private var host: String = ""
    @State private var port: String = "58900"
    @State private var code: String = ""
    @State private var ssl: Bool = false
    @State private var clipboardURI: String? = nil
    @State private var errorMessage: String? = nil
    
    public init(onPair: @escaping (PairingInfo) -> Void) {
        self.onPair = onPair
    }
    
    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
                    // Quick clipboard paste banner if available
                    if let clip = clipboardURI {
                        Button(action: {
                            handleQuickPair(from: clip)
                        }) {
                            HStack(spacing: 10) {
                                Image(systemName: "doc.on.clipboard.fill")
                                    .font(.system(size: 16))
                                    .foregroundColor(.indigo)
                                
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("检测到剪贴板配对链接")
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                    Text("点击直接导入参数并立即连接")
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                }
                                
                                Spacer()
                                
                                Image(systemName: "arrow.right.circle.fill")
                                    .font(.system(size: 18))
                                    .foregroundColor(.indigo)
                            }
                            .padding(.horizontal, 14)
                            .padding(.vertical, 12)
                            .background(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .fill(.ultraThinMaterial)
                            )
                            .overlay(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .stroke(Color.indigo.opacity(0.35), lineWidth: 1)
                            )
                        }
                        .buttonStyle(.plain)
                    }
                    
                    // Host input
                    VStack(alignment: .leading, spacing: 6) {
                        Text("网关 IP 或域名 (Host)")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.secondary)
                        
                        HStack(spacing: 8) {
                            Image(systemName: "network")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                                .frame(width: 20)
                            
                            TextField("例如 192.168.1.100 或 [2408:...]", text: $host)
                                .font(.system(size: 14.5))
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .onChange(of: host) { _, newVal in
                                    // If user accidentally pastes an agy:// link directly into host field
                                    if newVal.lowercased().hasPrefix("agy://pair") {
                                        parseAndPopulate(from: newVal)
                                    }
                                }
                            
                            if !host.isEmpty {
                                Button(action: { host = "" }) {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundColor(.secondary.opacity(0.8))
                                }
                            }
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .fill(.ultraThinMaterial)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                        )
                    }
                    
                    // Port input
                    VStack(alignment: .leading, spacing: 6) {
                        Text("端口号 (Port)")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.secondary)
                        
                        HStack(spacing: 8) {
                            Image(systemName: "number")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                                .frame(width: 20)
                            
                            TextField("默认 58900", text: $port)
                                .font(.system(size: 14.5))
                                .keyboardType(.numberPad)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                            
                            if !port.isEmpty && port != "58900" {
                                Button(action: { port = "58900" }) {
                                    Text("重置")
                                        .font(.system(size: 12, weight: .medium))
                                        .foregroundColor(.indigo)
                                }
                            }
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .fill(.ultraThinMaterial)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                        )
                    }
                    
                    // Code input
                    VStack(alignment: .leading, spacing: 6) {
                        Text("配对码 (Code)")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.secondary)
                        
                        HStack(spacing: 8) {
                            Image(systemName: "key.fill")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                                .frame(width: 20)
                            
                            TextField("终端输出的配对码 (5分钟有效)", text: $code)
                                .font(.system(size: 14.5, design: .monospaced))
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                            
                            if !code.isEmpty {
                                Button(action: { code = "" }) {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundColor(.secondary.opacity(0.8))
                                }
                            }
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .fill(.ultraThinMaterial)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                        )
                    }
                    
                    // SSL Toggle
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("开启 HTTPS / SSL")
                                .font(.system(size: 15, weight: .medium))
                                .foregroundColor(.primary)
                            Text("若网关配置了 SSL 证书请开启")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }
                        Spacer()
                        Toggle("", isOn: $ssl)
                            .labelsHidden()
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                    .background(
                        RoundedRectangle(cornerRadius: 12, style: .continuous)
                            .fill(.ultraThinMaterial)
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 12, style: .continuous)
                            .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                    )
                    
                    // Submit button
                    Button(action: submitManualPairing) {
                        HStack(spacing: 8) {
                            Image(systemName: "link")
                                .font(.system(size: 15, weight: .semibold))
                            Text("连接并配对")
                                .font(.system(size: 16, weight: .semibold))
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                        .foregroundColor(.white)
                        .background(
                            isFormValid
                            ? LinearGradient(
                                colors: [Color.indigo, Color.purple],
                                startPoint: .leading,
                                endPoint: .trailing
                            )
                            : LinearGradient(
                                colors: [Color.gray.opacity(0.4), Color.gray.opacity(0.4)],
                                startPoint: .leading,
                                endPoint: .trailing
                            )
                        )
                        .cornerRadius(12)
                        .shadow(
                            color: isFormValid ? Color.indigo.opacity(0.3) : Color.clear,
                            radius: 8, x: 0, y: 4
                        )
                    }
                    .disabled(!isFormValid)
                    .padding(.top, 8)
                }
                .padding(20)
            }
            .navigationTitle("手动输入配对信息")
            .navigationBarTitleDisplayMode(.inline)
            .alert("输入错误", isPresented: Binding(
                get: { errorMessage != nil },
                set: { if !$0 { errorMessage = nil } }
            )) {
                Button("好", role: .cancel) {}
            } message: {
                Text(errorMessage ?? "")
            }
        }
        .onAppear {
            checkClipboard()
        }
        .presentationDetents([.fraction(0.7), .large])
        .presentationDragIndicator(.visible)
        .presentationCornerRadius(24)
        .presentationBackground(.ultraThinMaterial)
    }
    
    private var isFormValid: Bool {
        !host.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !code.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
    
    private func checkClipboard() {
        guard let clip = UIPasteboard.general.string?.trimmingCharacters(in: .whitespacesAndNewlines),
              clip.lowercased().hasPrefix("agy://pair") else {
            return
        }
        clipboardURI = clip
    }
    
    private func handleQuickPair(from uri: String) {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        switch PairingService.shared.parsePairingURI(uri) {
        case .success(let info):
            dismiss()
            onPair(info)
        case .failure(let err):
            errorMessage = err.localizedDescription
        }
    }
    
    private func parseAndPopulate(from uri: String) {
        switch PairingService.shared.parsePairingURI(uri) {
        case .success(let info):
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            self.host = info.host
            self.port = String(info.port)
            self.code = info.code
            self.ssl = info.ssl
        case .failure:
            break
        }
    }
    
    private func submitManualPairing() {
        let trimmedHost = host.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedCode = code.trimmingCharacters(in: .whitespacesAndNewlines)
        let parsedPort = Int(port.trimmingCharacters(in: .whitespacesAndNewlines)) ?? 58900
        
        guard !trimmedHost.isEmpty else {
            errorMessage = "请输入网关 IP 或域名"
            return
        }
        guard !trimmedCode.isEmpty else {
            errorMessage = "请输入配对码"
            return
        }
        
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        let info = PairingInfo(
            host: trimmedHost,
            port: parsedPort,
            code: trimmedCode,
            ssl: ssl
        )
        
        dismiss()
        onPair(info)
    }
}
