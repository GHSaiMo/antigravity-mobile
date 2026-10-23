import SwiftUI

/// 动态多智能体工作流折叠卡片 (Antigravity 2.26+ Dynamic Workflows)。
/// 展示工作流阶段 (Phases)、并发子智能体状态与结构化 Schema 产物。
public struct WorkflowStageCardView: View {
    public let workflow: WorkflowItem

    @State private var isExpanded: Bool = true

    public init(workflow: WorkflowItem) {
        self.workflow = workflow
    }

    private var isRunning: Bool {
        workflow.status.caseInsensitiveCompare("running") == .orderedSame
    }

    private var statusColor: Color {
        if isRunning {
            return .green
        } else if workflow.status.caseInsensitiveCompare("error") == .orderedSame {
            return .red
        }
        return .secondary
    }

    private var statusText: String {
        if isRunning {
            return "编排执行中"
        } else if workflow.status.caseInsensitiveCompare("error") == .orderedSame {
            return "执行失败"
        }
        return "工作流完成"
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Header Row
            Button(action: {
                withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                    isExpanded.toggle()
                }
            }) {
                HStack(spacing: 8) {
                    ZStack {
                        Circle()
                            .fill(Color.indigo.opacity(0.18))
                            .frame(width: 28, height: 28)
                        Image(systemName: "point.3.connected.trianglepath.dotted")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.indigo)
                    }

                    VStack(alignment: .leading, spacing: 2) {
                        Text(workflow.description?.isEmpty == false ? workflow.description! : "动态多智能体工作流")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)

                        HStack(spacing: 4) {
                            Circle()
                                .fill(statusColor)
                                .frame(width: 6, height: 6)
                            Text(statusText)
                                .font(.system(size: 11.5))
                                .foregroundColor(.secondary)
                        }
                    }

                    Spacer()

                    Image(systemName: "chevron.down")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : -90))
                        .frame(width: 24, height: 24)
                }
            }
            .buttonStyle(.plain)

            if isExpanded {
                VStack(alignment: .leading, spacing: 10) {
                    // Staged Execution Phases
                    if !workflow.phases.isEmpty {
                        Text("执行阶段 (Phases)")
                            .font(.system(size: 11, weight: .bold))
                            .foregroundColor(.secondary)

                        ForEach(Array(workflow.phases.enumerated()), id: \.offset) { idx, phase in
                            VStack(alignment: .leading, spacing: 6) {
                                HStack {
                                    Text("Phase \(idx + 1): \(phase.name)")
                                        .font(.system(size: 12.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                    Spacer()
                                    Image(systemName: phase.status == "done" ? "checkmark.circle.fill" : "clock.fill")
                                        .font(.system(size: 12))
                                        .foregroundColor(phase.status == "done" ? .green : .orange)
                                }

                                if !phase.logs.isEmpty {
                                    Text(phase.logs.joined(separator: "\n"))
                                        .font(.system(size: 11, design: .monospaced))
                                        .foregroundColor(.secondary)
                                        .lineLimit(4)
                                }
                            }
                            .padding(10)
                            .background(Color(uiColor: .tertiarySystemFill).opacity(0.6))
                            .cornerRadius(10)
                        }
                    }

                    // Structured Output (JSON Schema result)
                    if let resultJson = workflow.resultJson, !resultJson.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        Text("结构化产物契约 (JSON Schema Output)")
                            .font(.system(size: 11, weight: .bold))
                            .foregroundColor(.secondary)

                        VStack(alignment: .leading, spacing: 4) {
                            HStack(spacing: 4) {
                                Image(systemName: "curlybraces")
                                    .font(.system(size: 11, weight: .bold))
                                    .foregroundColor(.orange)
                                Text("Schema Output")
                                    .font(.system(size: 10.5, weight: .bold))
                                    .foregroundColor(.orange)
                            }
                            Text(resultJson)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.primary)
                        }
                        .padding(10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.black.opacity(0.35))
                        .cornerRadius(10)
                        .overlay(
                            RoundedRectangle(cornerRadius: 10)
                                .stroke(Color.white.opacity(0.08), lineWidth: 0.5)
                        )
                    }
                }
                .padding(.top, 4)
            }
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemBackground))
        .cornerRadius(16)
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(Color(uiColor: .separator).opacity(0.5), lineWidth: 0.5)
        )
    }
}
