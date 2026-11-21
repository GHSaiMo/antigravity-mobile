import SwiftUI

public struct TipsAndGesturesGuideView: View {
    @Environment(\.dismiss) private var dismiss

    public init() {}

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
                    icon: "arrow.left",
                    badgeColor: Color(red: 0.66, green: 0.36, blue: 0.36), // 灰调砖红（与下方沉稳低饱和保持一致）
                    title: String(localized: "左滑会话卡片"),
                    description: String(localized: "向左轻扫任意会话，呼出快捷删除与归档操作。")
                )
                TipRow(
                    icon: "pin.fill",
                    badgeColor: Color(red: 0.68, green: 0.48, blue: 0.32), // 灰调暖橙（与下方沉稳低饱和保持一致）
                    title: String(localized: "右滑固定置顶"),
                    description: String(localized: "向右轻扫会话卡片，一键将当前重点会话固定在列表最顶端。")
                )
                TipRow(
                    icon: "folder.fill",
                    badgeColor: Color(red: 0.35, green: 0.50, blue: 0.44), // 沉稳鼠尾草绿
                    title: String(localized: "点击左下角项目图标"),
                    description: String(localized: "精准轻按卡片左下角的文件夹或气泡图标，一键聚合筛选该项目的所有会话；点击项目名称可直接进入会话。")
                )
            }

            // 2. 对话与模型调度
            Section(header: Text(String(localized: "对话与模型调度"))) {
                TipRow(
                    icon: "arrow.triangle.2.circlepath",
                    badgeColor: Color(red: 0.32, green: 0.48, blue: 0.64), // 灰调柔蓝
                    title: String(localized: "短按模型胶囊"),
                    description: String(localized: "轻按输入框底部的模型胶囊，在已设定的首选与次选模型（如 Gemini / Claude）之间一键极速轮换。")
                )
                TipRow(
                    icon: "hand.tap.fill",
                    badgeColor: Color(red: 0.46, green: 0.39, blue: 0.53), // 柔灰紫
                    title: String(localized: "长按模型胶囊上滑"),
                    description: String(localized: "按住模型胶囊 0.2 秒后向上滑动，即可呼出全量模型转盘与思考强度选单，松手即刻选定。")
                )
                TipRow(
                    icon: "text.badge.plus",
                    badgeColor: Color(red: 0.28, green: 0.49, blue: 0.52), // 灰青水鸭
                    title: String(localized: "Workflow 提示词模板"),
                    description: String(localized: "短按打开满屏提示词模板库；长按上滑可极速将高频工程模板填入输入框。")
                )
                TipRow(
                    icon: "wrench.and.screwdriver.fill",
                    badgeColor: Color(red: 0.35, green: 0.44, blue: 0.50), // 蓝灰深青
                    title: String(localized: "会话右上角工具与任务看板"),
                    description: String(localized: "轻点秒开控制看板，随时查看当前 MCP 工具、系统技能与后台任务；长按可快捷切换全局权限模式（严格审批 / 沙箱 / 全自动）或开启旁路快问。")
                )
            }

            // 3. 附件与媒体
            Section(header: Text(String(localized: "照片与媒体功能"))) {
                TipRow(
                    icon: "photo.on.rectangle.angled",
                    badgeColor: Color(red: 0.38, green: 0.50, blue: 0.38), // 灰橄榄绿
                    title: String(localized: "相册顶部标题切换文件夹"),
                    description: String(localized: "在照片选择抽屉中，轻点顶部「最近项目」标题即可呼出相册选择器，精准定位「截屏」或指定相册，无需无休止向上滑动。")
                )
                TipRow(
                    icon: "mic.fill",
                    badgeColor: Color(red: 0.65, green: 0.38, blue: 0.42), // 灰调干枯玫瑰
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
