import SwiftUI

public struct TipsAndGesturesGuideView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme

    public init() {}

    private func badgeColor(light: Color, dark: Color) -> Color {
        colorScheme == .dark ? dark : light
    }

    public var body: some View {
        List {
            // Header Hero Banner
            Section {
                VStack(spacing: 12) {
                    ZStack {
                        Circle()
                            .fill(Color.orange.opacity(0.15))
                            .frame(width: 60, height: 60)
                        Image(systemName: "lightbulb.fill")
                            .font(.system(size: 28, weight: .semibold))
                            .foregroundColor(.orange)
                    }
                    .padding(.top, 4)

                    Text(String(localized: "使用技巧与快捷手势"))
                        .font(.headline)
                        .foregroundColor(.primary)

                    Text(String(localized: "掌握核心隐蔽手势与快捷操作，无需繁琐步骤，在日常交互中获得更高效的陪伴体验。"))
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 8)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 8)
                .listRowBackground(Color.clear)
            }

            // 1. 会话列表管理
            Section(header: Text(String(localized: "会话列表管理"))) {
                TipRow(
                    icon: "trash.fill",
                    badgeColor: badgeColor(
                        light: .red,
                        dark: Color(red: 0.66, green: 0.36, blue: 0.36) // 深色模式保持灰调砖红
                    ),
                    title: String(localized: "左滑会话卡片"),
                    description: String(localized: "向左轻扫任意会话，呼出快捷删除操作。")
                )
                TipRow(
                    icon: "pin.fill",
                    badgeColor: badgeColor(
                        light: .orange,
                        dark: Color(red: 0.68, green: 0.48, blue: 0.32) // 深色模式保持灰调暖橙
                    ),
                    title: String(localized: "右滑固定置顶"),
                    description: String(localized: "向右轻扫会话卡片，一键将当前重点会话固定在列表最顶端。")
                )
                TipRow(
                    icon: "folder.fill",
                    badgeColor: badgeColor(
                        light: Color(red: 0.06, green: 0.73, blue: 0.51), // 浅色模式恢复清透翡翠绿
                        dark: Color(red: 0.35, green: 0.50, blue: 0.44) // 深色模式保持沉稳鼠尾草绿
                    ),
                    title: String(localized: "点击左下角项目图标"),
                    description: String(localized: "精准轻按卡片左下角的文件夹或气泡图标，一键聚合筛选该项目的所有会话；在聚合模式下，点击「新建对话」即可直接在该项目里新建对话。")
                )
                TipRow(
                    icon: "archivebox.fill",
                    badgeColor: badgeColor(
                        light: Color(red: 0.39, green: 0.45, blue: 0.55), // 浅色模式与左滑归档按钮一致的石板灰蓝
                        dark: Color(red: 0.35, green: 0.40, blue: 0.47) // 深色模式保持沉稳低饱和石板灰蓝
                    ),
                    title: String(localized: "左滑归档会话"),
                    description: String(localized: "向左轻扫会话卡片可一键归档，保持列表清爽；在聚合模式下选择「已归档」，即可随时找到已归档的会话。")
                )
                TipRow(
                    icon: "desktopcomputer",
                    badgeColor: badgeColor(
                        light: .indigo,
                        dark: Color(red: 0.38, green: 0.44, blue: 0.62)
                    ),
                    title: String(localized: "多主机快捷切换"),
                    description: String(localized: "长按顶部「Multigravity」标题并下滑可添加或切换多台电脑网关；绑定 2 台及以上主机后，左右轻扫标题可快速轮换，列表上滑收拢后轻点居中主机名也可一键展开切换面板。")
                )
            }

            // 2. 对话与模型调度
            Section(header: Text(String(localized: "对话与模型调度"))) {
                TipRow(
                    icon: "arrow.triangle.2.circlepath",
                    badgeColor: badgeColor(
                        light: .blue,
                        dark: Color(red: 0.32, green: 0.48, blue: 0.64) // 深色模式保持灰调柔蓝
                    ),
                    title: String(localized: "短按模型胶囊"),
                    description: String(localized: "轻按输入框底部的模型胶囊，在已设定的首选与次选模型（如 Gemini / Claude）之间一键极速轮换。")
                )
                TipRow(
                    icon: "sparkles",
                    badgeColor: badgeColor(
                        light: .indigo,
                        dark: Color(red: 0.42, green: 0.44, blue: 0.72) // 深色模式保持灰调靛蓝
                    ),
                    title: String(localized: "提示词建议胶囊"),
                    description: String(localized: "短按建议胶囊可将其填入输入框方便修改；长按 1.5 秒待进度条走满后直接发送，提前松手即可取消防误触。")
                )
                TipRow(
                    icon: "hand.tap.fill",
                    badgeColor: badgeColor(
                        light: Color(red: 0.87, green: 0.45, blue: 0.26), // 浅色模式 Claude 品牌暖橙 (#DE7342)
                        dark: Color(red: 0.66, green: 0.43, blue: 0.33) // 深色模式低饱和 Claude 陶土橙 (#A86E54)
                    ),
                    title: String(localized: "长按模型胶囊上滑"),
                    description: String(localized: "按住模型胶囊 0.2 秒后向上滑动，即可呼出全量模型转盘与思考强度选单，松手即刻选定。")
                )
                TipRow(
                    icon: "text.badge.plus",
                    badgeColor: badgeColor(
                        light: .teal,
                        dark: Color(red: 0.28, green: 0.49, blue: 0.52) // 深色模式保持灰青水鸭
                    ),
                    title: String(localized: "Workflow 提示词模板"),
                    description: String(localized: "短按打开满屏提示词模板库；长按上滑可极速将高频工程模板填入输入框。")
                )
                TipRow(
                    icon: "wrench.and.screwdriver.fill",
                    badgeColor: badgeColor(
                        light: .indigo,
                        dark: Color(red: 0.35, green: 0.44, blue: 0.50) // 深色模式保持蓝灰深青
                    ),
                    title: String(localized: "会话右上角工具与任务看板"),
                    description: String(localized: "轻点秒开控制看板，随时查看当前 MCP 工具、系统技能与后台任务；长按可快捷切换全局权限模式（严格审批 / 沙箱 / 全自动）或开启旁路快问。")
                )
            }

            // 3. 附件与媒体
            Section(header: Text(String(localized: "照片与媒体功能"))) {
                TipRow(
                    icon: "photo.on.rectangle.angled",
                    badgeColor: badgeColor(
                        light: .green,
                        dark: Color(red: 0.38, green: 0.50, blue: 0.38) // 深色模式保持灰橄榄绿
                    ),
                    title: String(localized: "相册顶部标题切换文件夹"),
                    description: String(localized: "在照片选择抽屉中，轻点顶部「最近项目」标题即可呼出相册选择器，精准定位「截屏」或指定相册，无需无休止向上滑动。")
                )
                TipRow(
                    icon: "mic.fill",
                    badgeColor: badgeColor(
                        light: .pink,
                        dark: Color(red: 0.65, green: 0.38, blue: 0.42) // 深色模式保持灰调干枯玫瑰
                    ),
                    title: String(localized: "双模语音交互"),
                    description: String(localized: "空闲时轻按右下角麦克风开启实时全双工语音通话；长按则直接唤起系统高精度语音听写转文字。")
                )
            }
        }
        .navigationTitle(String(localized: "手势与使用技巧"))
        .navigationBarTitleDisplayMode(.inline)
    }
}

private struct TipRow: View {
    let icon: String
    let badgeColor: Color
    let title: String
    let description: String

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            ZStack {
                RoundedRectangle(cornerRadius: 7, style: .continuous)
                    .fill(badgeColor)
                    .frame(width: 30, height: 30)

                Image(systemName: icon)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.white)
            }
            .padding(.top, 2)

            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.primary)

                Text(description)
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.vertical, 4)
    }
}
