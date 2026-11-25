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
    @State private var parsedPairingInfo: PairingInfo? = nil
    @State private var parsedSnapshot: (host: String, port: String, code: String, ssl: Bool)? = nil
    @State private var errorMessage: String? = nil
    
    public init(onPair: @escaping (PairingInfo) -> Void) {
        self.onPair = onPair
    }
    
    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
                    // Always-available quick paste row using native PasteButton (zero iOS clipboard permission prompt)
                    HStack(spacing: 12) {
                        Image(systemName: "doc.on.clipboard.fill")
                            .font(.system(size: 16))
                            .foregroundColor(.indigo)
                        
                        VStack(alignment: .leading, spacing: 2) {
                            Text("粘贴配对链接")
                                .font(.system(size: 13.5, weight: .semibold))
                                .foregroundColor(.primary)
                            Text("一键导入终端输出的 agy://pair 链接")
                                .font(.system(size: 11.5))
                                .foregroundColor(.secondary)
                                .lineLimit(2)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        
                        Spacer(minLength: 4)
                        
                        PasteButton(payloadType: String.self) { strings in
                            handlePastedPayloads(strings)
                        }
                        .buttonBorderShape(.capsule)
                        .controlSize(.small)
                        .tint(.indigo)
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
                                    // If user pastes an agy:// link (even with prefix/line breaks) into host field
                                    if PairingService.shared.extractPairingURI(from: newVal) != nil {
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
                                .onChange(of: code) { _, newVal in
                                    if PairingService.shared.extractPairingURI(from: newVal) != nil {
                                        parseAndPopulate(from: newVal)
                                    }
                                }
                            
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
        .presentationDetents([.fraction(0.72), .large])
        .presentationDragIndicator(.visible)
        .presentationCornerRadius(24)
        .presentationBackground(.ultraThinMaterial)
    }
    
    private var isFormValid: Bool {
        !host.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !code.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private func handlePastedPayloads(_ strings: [String]) {
        let combined = strings.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !combined.isEmpty,
              let extracted = PairingService.shared.extractPairingURI(from: combined) else {
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            errorMessage = String(localized: "剪贴板中未找到有效的配对链接（需包含 agy://pair）")
            return
        }
        handleQuickPair(from: extracted)
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
            let portStr = String(info.port)
            self.parsedPairingInfo = info
            self.parsedSnapshot = (host: info.host, port: portStr, code: info.code, ssl: info.ssl)
            self.host = info.host
            self.port = portStr
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
            errorMessage = String(localized: "请输入网关 IP 或域名")
            return
        }
        guard !trimmedCode.isEmpty else {
            errorMessage = String(localized: "请输入配对码")
            return
        }
        
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()

        // Preserve full parsed PairingInfo (including E2EE public key `pk` and LAN/relay endpoints)
        // when the form fields were populated from a pasted pairing URI and not manually altered.
        if let parsed = parsedPairingInfo,
           let snap = parsedSnapshot,
           trimmedHost == snap.host,
           String(parsedPort) == snap.port,
           trimmedCode == snap.code,
           ssl == snap.ssl {
            dismiss()
            onPair(parsed)
            return
        }
        
        var effectiveHost = trimmedHost
        if !effectiveHost.contains(".") && !effectiveHost.contains(":") && effectiveHost.lowercased() != "localhost" {
            effectiveHost = "\(effectiveHost).jiuge.space"
        }
        
        let info = PairingInfo(
            host: effectiveHost,
            port: parsedPort,
            code: trimmedCode,
            ssl: ssl
        )
        
        dismiss()
        onPair(info)
    }
}
