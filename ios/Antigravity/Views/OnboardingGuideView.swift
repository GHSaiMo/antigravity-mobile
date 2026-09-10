import SwiftUI

public struct OnboardingGuideView: View {
    public var onScanTapped: () -> Void
    public var onManualInputTapped: () -> Void
    
    @Environment(\.openURL) private var openURL
    
    public init(
        onScanTapped: @escaping () -> Void,
        onManualInputTapped: @escaping () -> Void
    ) {
        self.onScanTapped = onScanTapped
        self.onManualInputTapped = onManualInputTapped
    }
    
    public var body: some View {
        ScrollView {
            VStack(spacing: 28) {
                // Header & Branding
                VStack(spacing: 12) {
                    ZStack {
                        Circle()
                            .fill(
                                LinearGradient(
                                    colors: [Color.indigo.opacity(0.8), Color.purple.opacity(0.8)],
                                    startPoint: .topLeading,
                                    endPoint: .bottomTrailing
                                )
                            )
                            .frame(width: 80, height: 80)
                            .shadow(color: Color.indigo.opacity(0.3), radius: 12, x: 0, y: 6)
                        
                        Image(systemName: "sparkles")
                            .font(.system(size: 38, weight: .semibold))
                            .foregroundColor(.white)
                    }
                    .padding(.top, 24)
                    
                    Text("欢迎使用 Antigravity")
                        .font(.system(size: 26, weight: .bold))
                        .foregroundColor(.primary)
                    
                    Text("Google Antigravity 智能体全栈移动伴侣\n随时随地监控思考流、下发指令与方案决策")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 24)
                }
                
                // Step 1: Download & Run Gateway
                VStack(alignment: .leading, spacing: 14) {
                    HStack(spacing: 10) {
                        ZStack {
                            Circle()
                                .fill(Color.blue.opacity(0.15))
                                .frame(width: 28, height: 28)
                            Text("1")
                                .font(.system(size: 14, weight: .bold))
                                .foregroundColor(.blue)
                        }
                        Text("在 Mac 上启动网关服务")
                            .font(.system(size: 16, weight: .semibold))
                            .foregroundColor(.primary)
                    }
                    
                    Text("手机端需配合运行在 Mac 电脑上的 Antigravity 本地网关协同工作。网关会自动嗅探后台实例并打通安全直连。")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                        .lineSpacing(3)
                    
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
                                .font(.system(size: 12))
                        }
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.blue)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 10)
                        .background(Color.blue.opacity(0.08))
                        .cornerRadius(10)
                    }
                }
                .padding(18)
                .background(Color(uiColor: .secondarySystemBackground))
                .cornerRadius(16)
                .padding(.horizontal, 20)
                
                // Step 2: Scan QR Code & Pair
                VStack(alignment: .leading, spacing: 14) {
                    HStack(spacing: 10) {
                        ZStack {
                            Circle()
                                .fill(Color.indigo.opacity(0.15))
                                .frame(width: 28, height: 28)
                            Text("2")
                                .font(.system(size: 14, weight: .bold))
                                .foregroundColor(.indigo)
                        }
                        Text("扫码一键自动配对")
                            .font(.system(size: 16, weight: .semibold))
                            .foregroundColor(.primary)
                    }
                    
                    Text("电脑终端运行网关后会自动生成复合二维码，同时包含 Wi-Fi 局域网与外网 IPv6 网址。手机扫码即可直接交换凭证并绑定，零手动配置。")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                        .lineSpacing(3)
                    
                    Button(action: onScanTapped) {
                        HStack {
                            Image(systemName: "qrcode.viewfinder")
                                .font(.system(size: 18, weight: .bold))
                            Text("扫描电脑端配对二维码")
                                .font(.system(size: 16, weight: .semibold))
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
                        .shadow(color: Color.indigo.opacity(0.3), radius: 8, x: 0, y: 4)
                    }
                    .padding(.top, 4)
                }
                .padding(18)
                .background(Color(uiColor: .secondarySystemBackground))
                .cornerRadius(16)
                .padding(.horizontal, 20)
                
                // Secondary action: Manual Input
                Button(action: onManualInputTapped) {
                    HStack(spacing: 6) {
                        Image(systemName: "keyboard")
                        Text("高级选项：手动输入网址或配对码")
                    }
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.secondary)
                }
                .padding(.bottom, 24)
            }
        }
    }
}
