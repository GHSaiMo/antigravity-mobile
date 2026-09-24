import SwiftUI

public struct OnboardingGuideView: View {
    public var onScanTapped: () -> Void
    public var onManualInputTapped: () -> Void
    public var onEasterEggTap: (() -> Void)?
    
    @Environment(\.openURL) private var openURL
    
    private let macInstallCommand = "curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash"
    private let winInstallCommand = "irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex"
    private let runCommand = "mgy"
    @State private var isMacCopied = false
    @State private var isWinCopied = false
    
    public init(
        onScanTapped: @escaping () -> Void,
        onManualInputTapped: @escaping () -> Void,
        onEasterEggTap: (() -> Void)? = nil
    ) {
        self.onScanTapped = onScanTapped
        self.onManualInputTapped = onManualInputTapped
        self.onEasterEggTap = onEasterEggTap
    }
    
    private func copyCommand(_ command: String, isMac: Bool) {
        UIPasteboard.general.string = command
        let generator = UINotificationFeedbackGenerator()
        generator.notificationOccurred(.success)
        if isMac {
            isMacCopied = true
        } else {
            isWinCopied = true
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
            if isMac {
                isMacCopied = false
            } else {
                isWinCopied = false
            }
        }
    }
    
    public var body: some View {
        GeometryReader { geometry in
            ScrollView(.vertical, showsIndicators: false) {
                VStack(spacing: 18) {
                    // Header & Branding
                    VStack(spacing: 10) {
                        Image("AppLogo")
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(width: 68, height: 68)
                            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 16, style: .continuous)
                                    .stroke(Color.primary.opacity(0.08), lineWidth: 1)
                            )
                            .shadow(color: Color.black.opacity(0.15), radius: 10, x: 0, y: 5)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                onEasterEggTap?()
                            }
                        
                        Text("欢迎使用 Multigravity")
                            .font(.system(size: 23, weight: .bold))
                            .foregroundColor(.primary)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                onEasterEggTap?()
                            }
                        
                        Text("Multigravity 智能体全栈移动伴侣\n随时随地监控思考流、下发指令与决策")
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .lineSpacing(4)
                            .padding(.horizontal, 16)
                    }
                    .padding(.top, 4)
                    
                    // Step 1: Download & Run Gateway
                    VStack(alignment: .leading, spacing: 12) {
                        HStack(spacing: 9) {
                            ZStack {
                                Circle()
                                    .fill(Color.blue.opacity(0.15))
                                    .frame(width: 26, height: 26)
                                Text("1")
                                    .font(.system(size: 13.5, weight: .bold))
                                    .foregroundColor(.blue)
                            }
                            Text("在电脑上启动网关服务")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundColor(.primary)
                        }
                        
                        Text("手机端需配合运行在 Mac 或 Windows 电脑上的本地网关协同工作，嗅探后台实例打通直连。")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                            .lineSpacing(3.5)
                            .lineLimit(nil)
                            .fixedSize(horizontal: false, vertical: true)
                        
                        // Terminal one-click installation script blocks container
                        VStack(alignment: .leading, spacing: 12) {
                            // macOS
                            VStack(alignment: .leading, spacing: 6) {
                                HStack {
                                    HStack(spacing: 5) {
                                        Image(systemName: "applelogo")
                                            .font(.system(size: 11.5))
                                            .foregroundColor(.secondary)
                                            .frame(width: 12, height: 12)
                                        Text("macOS (Apple Silicon & Intel)")
                                            .font(.system(size: 11.5, weight: .semibold))
                                            .foregroundColor(.primary)
                                            .lineLimit(1)
                                            .minimumScaleFactor(0.85)
                                    }
                                    Spacer()
                                    Button(action: { copyCommand(macInstallCommand, isMac: true) }) {
                                        HStack(spacing: 4) {
                                            Image(systemName: isMacCopied ? "checkmark" : "doc.on.doc")
                                                .font(.system(size: 11, weight: .semibold))
                                                .frame(width: 13, height: 13)
                                            Text(isMacCopied ? "已复制" : "一键复制")
                                                .font(.system(size: 11, weight: .semibold))
                                        }
                                        .foregroundColor(isMacCopied ? .green : .blue)
                                        .frame(width: 70, height: 24)
                                        .background(
                                            RoundedRectangle(cornerRadius: 6, style: .continuous)
                                                .fill(isMacCopied ? Color.green.opacity(0.12) : Color.blue.opacity(0.1))
                                        )
                                    }
                                    .buttonStyle(.plain)
                                    .animation(.easeInOut(duration: 0.2), value: isMacCopied)
                                }
                                .frame(height: 24)
                                
                                Button(action: { copyCommand(macInstallCommand, isMac: true) }) {
                                    ScrollView(.horizontal, showsIndicators: false) {
                                        Text(macInstallCommand)
                                            .font(.system(size: 11.5, weight: .regular, design: .monospaced))
                                            .foregroundColor(.primary)
                                            .lineLimit(1)
                                            .padding(.horizontal, 9)
                                            .padding(.vertical, 8.5)
                                    }
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(
                                        RoundedRectangle(cornerRadius: 7, style: .continuous)
                                            .fill(Color(uiColor: .tertiarySystemBackground))
                                    )
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 7, style: .continuous)
                                            .stroke(Color.primary.opacity(0.06), lineWidth: 1)
                                    )
                                }
                                .buttonStyle(.plain)
                            }
                            
                            // Windows (PowerShell)
                            VStack(alignment: .leading, spacing: 6) {
                                HStack {
                                    HStack(spacing: 5) {
                                        WindowsLogoView(size: 11.5)
                                            .frame(width: 12, height: 12)
                                        Text("Windows (PowerShell)")
                                            .font(.system(size: 11.5, weight: .semibold))
                                            .foregroundColor(.primary)
                                            .lineLimit(1)
                                    }
                                    Spacer()
                                    Button(action: { copyCommand(winInstallCommand, isMac: false) }) {
                                        HStack(spacing: 4) {
                                            Image(systemName: isWinCopied ? "checkmark" : "doc.on.doc")
                                                .font(.system(size: 11, weight: .semibold))
                                                .frame(width: 13, height: 13)
                                            Text(isWinCopied ? "已复制" : "一键复制")
                                                .font(.system(size: 11, weight: .semibold))
                                        }
                                        .foregroundColor(isWinCopied ? .green : .blue)
                                        .frame(width: 70, height: 24)
                                        .background(
                                            RoundedRectangle(cornerRadius: 6, style: .continuous)
                                                .fill(isWinCopied ? Color.green.opacity(0.12) : Color.blue.opacity(0.1))
                                        )
                                    }
                                    .buttonStyle(.plain)
                                    .animation(.easeInOut(duration: 0.2), value: isWinCopied)
                                }
                                .frame(height: 24)
                                
                                Button(action: { copyCommand(winInstallCommand, isMac: false) }) {
                                    ScrollView(.horizontal, showsIndicators: false) {
                                        Text(winInstallCommand)
                                            .font(.system(size: 11.5, weight: .regular, design: .monospaced))
                                            .foregroundColor(.primary)
                                            .lineLimit(1)
                                            .padding(.horizontal, 9)
                                            .padding(.vertical, 8.5)
                                    }
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(
                                        RoundedRectangle(cornerRadius: 7, style: .continuous)
                                            .fill(Color(uiColor: .tertiarySystemBackground))
                                    )
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 7, style: .continuous)
                                            .stroke(Color.primary.opacity(0.06), lineWidth: 1)
                                    )
                                }
                                .buttonStyle(.plain)
                            }
                            
                            // Run command helper row for mgy (no copy button)
                            HStack(spacing: 6) {
                                Text("若已安装网关，直接在终端执行")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                                Text(runCommand)
                                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                                    .foregroundColor(.indigo)
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 2)
                                    .background(
                                        RoundedRectangle(cornerRadius: 5, style: .continuous)
                                            .fill(Color(uiColor: .tertiarySystemBackground))
                                    )
                            }
                            .padding(.horizontal, 2)
                            .padding(.top, 2)
                        }
                        .padding(12)
                        .background(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
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
                                    .font(.system(size: 14))
                                Text("查看 GitHub 部署指南与下载")
                                    .font(.system(size: 13.5, weight: .medium))
                                Spacer()
                                Image(systemName: "arrow.up.right")
                                    .font(.system(size: 11.5))
                            }
                            .foregroundColor(.blue)
                            .padding(.horizontal, 14)
                            .padding(.vertical, 10)
                            .background(Color.blue.opacity(0.08))
                            .cornerRadius(9)
                        }
                    }
                    .padding(16)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(16)
                    .padding(.horizontal, 18)
                    
                    // Step 2: Scan QR Code & Pair
                    VStack(alignment: .leading, spacing: 12) {
                        HStack(spacing: 9) {
                            ZStack {
                                Circle()
                                    .fill(Color.indigo.opacity(0.15))
                                    .frame(width: 26, height: 26)
                                Text("2")
                                    .font(.system(size: 13.5, weight: .bold))
                                    .foregroundColor(.indigo)
                            }
                            Text("扫码一键自动配对")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundColor(.primary)
                        }
                        
                        Text("电脑终端运行网关后会自动生成复合二维码，手机扫码即可直接交换凭证并绑定，零手动配置。")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                            .lineSpacing(3.5)
                            .lineLimit(nil)
                            .fixedSize(horizontal: false, vertical: true)
                        
                        Button(action: onScanTapped) {
                            HStack(spacing: 8) {
                                Image(systemName: "qrcode.viewfinder")
                                    .font(.system(size: 17, weight: .bold))
                                Text("扫描电脑端配对二维码")
                                    .font(.system(size: 15.5, weight: .semibold))
                            }
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 14)
                            .foregroundColor(.white)
                            .background(
                                LinearGradient(
                                    colors: [Color.indigo, Color.purple],
                                    startPoint: .leading,
                                    endPoint: .trailing
                                )
                            )
                            .cornerRadius(12)
                            .shadow(color: Color.indigo.opacity(0.25), radius: 6, x: 0, y: 3)
                        }
                        .padding(.top, 2)
                    }
                    .padding(16)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(16)
                    .padding(.horizontal, 18)
                    
                    // Secondary action: Manual Input
                    Button(action: onManualInputTapped) {
                        HStack(spacing: 6) {
                            Image(systemName: "keyboard")
                                .font(.system(size: 13.5))
                            Text("高级选项：手动输入网址或配对码")
                                .font(.system(size: 13, weight: .medium))
                        }
                        .foregroundColor(.secondary)
                        .padding(.vertical, 6)
                        .padding(.horizontal, 12)
                    }
                    .padding(.top, 2)
                }
                .padding(.top, 16)
                .padding(.bottom, 24)
                .frame(maxWidth: .infinity)
                .frame(minHeight: geometry.size.height)
            }
        }
    }
}

public struct WindowsLogoView: View {
    public var size: CGFloat
    public var color: Color?
    
    public init(size: CGFloat = 11.5, color: Color? = nil) {
        self.size = size
        self.color = color
    }
    
    public var body: some View {
        let gap: CGFloat = max(1.2, size * 0.14)
        let blockSize: CGFloat = (size - gap) / 2
        let cornerRadius: CGFloat = max(0.6, size * 0.08)
        let fillColor = color ?? Color.secondary
        
        VStack(spacing: gap) {
            HStack(spacing: gap) {
                RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                    .fill(fillColor)
                    .frame(width: blockSize, height: blockSize)
                RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                    .fill(fillColor)
                    .frame(width: blockSize, height: blockSize)
            }
            HStack(spacing: gap) {
                RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                    .fill(fillColor)
                    .frame(width: blockSize, height: blockSize)
                RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                    .fill(fillColor)
                    .frame(width: blockSize, height: blockSize)
            }
        }
        .frame(width: size, height: size)
    }
}
