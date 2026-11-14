import SwiftUI

/// 动态多智能体工作流预设模板定义
public struct WorkflowTemplateItem: Identifiable, Sendable {
    public let id: String
    public let title: String
    public let subtitle: String
    public let icon: String
    public let iconColor: Color
    public let templatePrompt: String
}

/// 动态多智能体工作流预设模板选择抽屉 (WorkflowTemplateSheet)。
public struct WorkflowTemplateSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let onSelectTemplate: (String) -> Void
    
    public static let templates: [WorkflowTemplateItem] = [
        WorkflowTemplateItem(
            id: "code_review",
            title: "并发代码审查",
            subtitle: "并行启动安全、性能、类型规范多角度 Sub-Agents 进行深度审计",
            icon: "shield.lefthalf.filled",
            iconColor: .indigo,
            templatePrompt: "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。"
        ),
        WorkflowTemplateItem(
            id: "test_and_fix",
            title: "并发单测与故障修复",
            subtitle: "并行运行各模块单元测试，定位失败用例并尝试由子代理生成补丁",
            icon: "testtube.2",
            iconColor: .green,
            templatePrompt: "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。"
        ),
        WorkflowTemplateItem(
            id: "research",
            title: "多维度综合调研",
            subtitle: "分派多个子代理从技术实现、竞品方案、生产实践分头研究并归纳",
            icon: "magnifyingglass.circle.fill",
            iconColor: .blue,
            templatePrompt: "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。"
        ),
        WorkflowTemplateItem(
            id: "custom",
            title: "自定义动态工作流",
            subtitle: "自由描述任务，让 Agent 依据依赖自主规划 parallel / pipeline 阶段",
            icon: "point.3.connected.trianglepath.dotted",
            iconColor: .purple,
            templatePrompt: "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。"
        )
    ]
    
    public init(onSelectTemplate: @escaping (String) -> Void) {
        self.onSelectTemplate = onSelectTemplate
    }
    
    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    // Header Banner
                    HStack(spacing: 10) {
                        ZStack {
                            Circle()
                                .fill(Color.indigo.opacity(0.18))
                                .frame(width: 36, height: 36)
                            Image(systemName: "point.3.connected.trianglepath.dotted")
                                .font(.system(size: 17, weight: .bold))
                                .foregroundColor(.indigo)
                        }
                        
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Antigravity 动态工作流")
                                .font(.system(size: 15, weight: .bold))
                                .foregroundColor(.primary)
                            Text("选择常用模板快速分派多智能体并发任务")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }
                    }
                    .padding(.vertical, 4)
                    
                    // Template Cards List
                    VStack(spacing: 10) {
                        ForEach(Self.templates) { item in
                            Button(action: {
                                onSelectTemplate(item.templatePrompt)
                                dismiss()
                            }) {
                                HStack(spacing: 12) {
                                    ZStack {
                                        RoundedRectangle(cornerRadius: 10)
                                            .fill(item.iconColor.opacity(0.15))
                                            .frame(width: 40, height: 40)
                                        Image(systemName: item.icon)
                                            .font(.system(size: 18, weight: .semibold))
                                            .foregroundColor(item.iconColor)
                                    }
                                    
                                    VStack(alignment: .leading, spacing: 3) {
                                        Text(item.title)
                                            .font(.system(size: 14, weight: .semibold))
                                            .foregroundColor(.primary)
                                        Text(item.subtitle)
                                            .font(.system(size: 11.5))
                                            .foregroundColor(.secondary)
                                            .lineLimit(2)
                                            .fixedSize(horizontal: false, vertical: true)
                                    }
                                    
                                    Spacer()
                                    
                                    Image(systemName: "chevron.right")
                                        .font(.system(size: 12, weight: .bold))
                                        .foregroundColor(Color(uiColor: .tertiaryLabel))
                                }
                                .padding(12)
                                .background(Color(uiColor: .secondarySystemBackground))
                                .cornerRadius(12)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 12)
                                        .stroke(Color(uiColor: .separator).opacity(0.4), lineWidth: 0.5)
                                    )
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
                .padding(16)
            }
            .navigationTitle("Workflow 模板")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium, .large])
    }
}
