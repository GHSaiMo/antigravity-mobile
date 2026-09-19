import SwiftUI

public struct OnboardingGuideView: View {
    public var onScanTapped: () -> Void
    public var onManualInputTapped: () -> Void
    public var onEasterEggTap: (() -> Void)?
    
    @Environment(\.openURL) private var openURL
    
    private let installCommand = "curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash"
    private let runCommand = "mgy"
    @State private var isCopied = false
    
    public init(
        onScanTapped: @escaping () -> Void,
        onManualInputTapped: @escaping () -> Void,
        onEasterEggTap: (() -> Void)? = nil
    ) {
        self.onScanTapped = onScanTapped
        self.onManualInputTapped = onManualInputTapped
        self.onEasterEggTap = onEasterEggTap
    }
    
    private func copyInstallCommand() {
        UIPasteboard.general.string = installCommand
        let generator = UINotificationFeedbackGenerator()
        generator.notificationOccurred(.success)
        withAnimation(.easeInOut(duration: 0.2)) {
            isCopied = true
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
            withAnimation(.easeInOut(duration: 0.2)) {
                isCopied = false
            }
        }
    }
    
    public var body: some View {
        GeometryReader { geometry in
            ScrollView(.vertical, showsIndicators: false) {
                VStack(spacing: 14) {
                    // Header & Branding
                    VStack(spacing: 8) {
                        Image("AppLogo")
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(width: 60, height: 60)
                            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                            )
                            .shadow(color: Color.black.opacity(0.12), radius: 8, x: 0, y: 4)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                onEasterEggTap?()
                            }
                        
                        Text("欢迎使用 Multigravity")
                            .font(.system(size: 21, weight: .bold))
                            .foregroundColor(.primary)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                onEasterEggTap?()
                            }
                        
                        Text("Multigravity 智能体全栈移动伴侣\n随时随地监控思考流、下发指令与决策")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 16)
                    }
                    
                    // Step 1: Download & Run Gateway
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 8) {
                            ZStack {
                                Circle()
                                    .fill(Color.blue.opacity(0.15))
                                    .frame(width: 24, height: 24)
                                Text("1")
                                    .font(.system(size: 13, weight: .bold))
                                    .foregroundColor(.blue)
                            }
                            Text("在 Mac 上启动网关服务")
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundColor(.primary)
                        }
                        
                        Text("手机端需配合运行在 Mac 上的本地网关协同工作，嗅探后台实例打通直连。")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineSpacing(2)
                        
                        // Mac Terminal one-click installation script block
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                HStack(spacing: 5) {
                                    Image(systemName: "terminal.fill")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                    Text("Mac 终端一键安装脚本")
                                        .font(.system(size: 11.5, weight: .medium))
                                        .foregroundColor(.secondary)
                                }
                                Spacer()
                                Button(action: copyInstallCommand) {
                                    HStack(spacing: 4) {
                                        Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                                            .font(.system(size: 11, weight: .semibold))
                                        Text(isCopied ? "已复制" : "一键复制")
                                            .font(.system(size: 11, weight: .semibold))
                                    }
                                    .foregroundColor(isCopied ? .green : .blue)
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 4)
                                    .background(
                                        RoundedRectangle(cornerRadius: 6, style: .continuous)
                                            .fill(isCopied ? Color.green.opacity(0.12) : Color.blue.opacity(0.1))
                                    )
                                }
                                .buttonStyle(.plain)
                            }
                            
                            // Horizontal scrollable single-line code block
                            Button(action: copyInstallCommand) {
                                ScrollView(.horizontal, showsIndicators: false) {
                                    Text(installCommand)
                                        .font(.system(size: 11, weight: .regular, design: .monospaced))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 7)
                                }
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(
                                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                                        .fill(Color(uiColor: .tertiarySystemBackground))
                                )
                                .overlay(
                                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                                        .stroke(Color.primary.opacity(0.06), lineWidth: 1)
                                )
                            }
                            .buttonStyle(.plain)
                            
                            // Run command helper row for mgy (no copy button)
                            HStack(spacing: 6) {
                                Text("若已安装网关，直接在终端执行")
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.secondary)
                                Text(runCommand)
                                    .font(.system(size: 11.5, weight: .bold, design: .monospaced))
                                    .foregroundColor(.indigo)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 1.5)
                                    .background(
                                        RoundedRectangle(cornerRadius: 4, style: .continuous)
                                            .fill(Color(uiColor: .tertiarySystemBackground))
                                    )
                            }
                            .padding(.horizontal, 2)
                            .padding(.top, 1)
                        }
                        .padding(10)
                        .background(
                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                .fill(Color(uiColor: .tertiarySystemFill).opacity(0.5))
                        )
                        
                        // GitHub guide button
                        Button(action: {
                            if let url = URL(string: "https://github.com/GHSaiMo/antigravity-mobile#readme") {
                                openURL(url)
                            }
                        }) {
                            HStack {
                                Image(systemName: "arrow.down.circle.fill")
                                Text("查看 GitHub 部署指南与下载")
                                Spacer()
                                Image(systemName: "arrow.up.right")
                                    .font(.system(size: 11))
                            }
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.blue)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .background(Color.blue.opacity(0.08))
                            .cornerRadius(8)
                        }
                    }
                    .padding(14)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(14)
                    .padding(.horizontal, 20)
                    
                    // Step 2: Scan QR Code & Pair
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 8) {
                            ZStack {
                                Circle()
                                    .fill(Color.indigo.opacity(0.15))
                                    .frame(width: 24, height: 24)
                                Text("2")
                                    .font(.system(size: 13, weight: .bold))
                                    .foregroundColor(.indigo)
                            }
                            Text("扫码一键自动配对")
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundColor(.primary)
                        }
                        
                        Text("电脑终端运行网关后会自动生成复合二维码，手机扫码即可直接交换凭证并绑定，零手动配置。")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineSpacing(2)
                        
                        Button(action: onScanTapped) {
                            HStack {
                                Image(systemName: "qrcode.viewfinder")
                                    .font(.system(size: 16, weight: .bold))
                                Text("扫描电脑端配对二维码")
                                    .font(.system(size: 14.5, weight: .semibold))
                            }
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 12)
                            .foregroundColor(.white)
                            .background(
                                LinearGradient(
                                    colors: [Color.indigo, Color.purple],
                                    startPoint: .leading,
                                    endPoint: .trailing
                                )
                            )
                            .cornerRadius(10)
                            .shadow(color: Color.indigo.opacity(0.25), radius: 6, x: 0, y: 3)
                        }
                        .padding(.top, 2)
                    }
                    .padding(14)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(14)
                    .padding(.horizontal, 20)
                    
                    // Secondary action: Manual Input
                    Button(action: onManualInputTapped) {
                        HStack(spacing: 5) {
                            Image(systemName: "keyboard")
                            Text("高级选项：手动输入网址或配对码")
                        }
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.secondary)
                    }
                }
                .padding(.vertical, 14)
                .frame(maxWidth: .infinity)
                .frame(minHeight: geometry.size.height)
            }
        }
    }
}
