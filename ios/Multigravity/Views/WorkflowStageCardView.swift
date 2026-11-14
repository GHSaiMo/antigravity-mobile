import SwiftUI

/// 动态多智能体工作流拓扑折叠看板 (Antigravity 2.32+ Dynamic Workflows)。
/// 展示阶段流转轴 (Stage Pipeline Stepper)、并发子智能体拓扑网格、Live 日志流与结构化 Schema 产物。
public struct WorkflowStageCardView: View {
    public let workflow: WorkflowItem
    public let onStopWorkflow: (() async -> Void)?
    public let onSelectSubagent: ((SubagentItem) -> Void)?

    @State private var isExpanded: Bool = true
    @State private var selectedPhaseIndex: Int = 0
    @State private var isLogsExpanded: Bool = false
    @State private var isStopping: Bool = false

    public init(
        workflow: WorkflowItem,
        onStopWorkflow: (() async -> Void)? = nil,
        onSelectSubagent: ((SubagentItem) -> Void)? = nil
    ) {
        self.workflow = workflow
        self.onStopWorkflow = onStopWorkflow
        self.onSelectSubagent = onSelectSubagent
    }

    private var isRunning: Bool {
        workflow.status.caseInsensitiveCompare("running") == .orderedSame
    }

    private var isError: Bool {
        workflow.status.caseInsensitiveCompare("error") == .orderedSame || workflow.status.caseInsensitiveCompare("failed") == .orderedSame
    }

    private var statusColor: Color {
        if isRunning {
            return .green
        } else if isError {
            return .red
        }
        return .secondary
    }

    private var statusText: String {
        if isRunning {
            return "编排执行中"
        } else if isError {
            return "执行失败"
        }
        return "工作流完成"
    }

    private var allSubagents: [SubagentItem] {
        if !workflow.phases.isEmpty && selectedPhaseIndex < workflow.phases.count {
            let phase = workflow.phases[selectedPhaseIndex]
            if !phase.subagents.isEmpty {
                return phase.subagents
            }
        }
        return workflow.phases.flatMap { $0.subagents }
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Header Row
            HStack(spacing: 8) {
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
                                .font(.system(size: 13.5, weight: .semibold))
                                .foregroundColor(.primary)
                                .lineLimit(1)

                            HStack(spacing: 5) {
                                Circle()
                                    .fill(statusColor)
                                    .frame(width: 6, height: 6)
                                Text(statusText)
                                    .font(.system(size: 11))
                                    .foregroundColor(.secondary)
                                if let dur = workflow.duration, !dur.isEmpty {
                                    Text("• \(dur)")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                }
                            }
                        }
                    }
                }
                .buttonStyle(.plain)

                Spacer()

                // Stop Workflow action (if running)
                if isRunning && onStopWorkflow != nil {
                    Button(action: {
                        Task {
                            isStopping = true
                            await onStopWorkflow?()
                            isStopping = false
                        }
                    }) {
                        HStack(spacing: 3) {
                            if isStopping {
                                ProgressView()
                                    .tint(.red)
                                    .scaleEffect(0.65)
                            } else {
                                Image(systemName: "stop.circle")
                                    .font(.system(size: 11, weight: .bold))
                            }
                            Text("终止")
                                .font(.system(size: 11, weight: .semibold))
                        }
                        .foregroundColor(.red)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .background(Color.red.opacity(0.12))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .disabled(isStopping)
                }

                // Expand Chevron
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                        isExpanded.toggle()
                    }
                }) {
                    Image(systemName: "chevron.down")
                        .font(.system(size: 12, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : -90))
                        .frame(width: 22, height: 22)
                }
                .buttonStyle(.plain)
            }

            if isExpanded {
                VStack(alignment: .leading, spacing: 10) {
                    // 1. Stage Pipeline Stepper
                    if !workflow.phases.isEmpty {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 8) {
                                ForEach(Array(workflow.phases.enumerated()), id: \.offset) { idx, phase in
                                    Button(action: {
                                        withAnimation {
                                            selectedPhaseIndex = idx
                                        }
                                    }) {
                                        HStack(spacing: 6) {
                                            ZStack {
                                                if phase.status == "done" {
                                                    Image(systemName: "checkmark.circle.fill")
                                                        .foregroundColor(.green)
                                                        .font(.system(size: 13))
                                                } else if phase.status == "running" {
                                                    ProgressView()
                                                        .tint(.indigo)
                                                        .scaleEffect(0.7)
                                                } else if phase.status == "failed" {
                                                    Image(systemName: "exclamationmark.circle.fill")
                                                        .foregroundColor(.red)
                                                        .font(.system(size: 13))
                                                } else {
                                                    Circle()
                                                        .stroke(Color.secondary.opacity(0.4), lineWidth: 1.5)
                                                        .frame(width: 12, height: 12)
                                                }
                                            }
                                            .frame(width: 16, height: 16)

                                            Text("P\(idx + 1): \(phase.name)")
                                                .font(.system(size: 12, weight: selectedPhaseIndex == idx ? .bold : .medium))
                                                .foregroundColor(selectedPhaseIndex == idx ? .primary : .secondary)

                                            if !phase.subagents.isEmpty {
                                                Text("(\(phase.subagents.count))")
                                                    .font(.system(size: 10, design: .monospaced))
                                                    .foregroundColor(.indigo)
                                            }
                                        }
                                        .padding(.horizontal, 9)
                                        .padding(.vertical, 5)
                                        .background(selectedPhaseIndex == idx ? Color.indigo.opacity(0.15) : Color(uiColor: .tertiarySystemFill).opacity(0.6))
                                        .cornerRadius(8)
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 8)
                                                .stroke(selectedPhaseIndex == idx ? Color.indigo.opacity(0.4) : Color.clear, lineWidth: 1)
                                        )
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }

                    // 2. Subagent Matrix Grid
                    if !allSubagents.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Text("并发子代理 (Sub-Agents)")
                                    .font(.system(size: 11, weight: .bold))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Text("轻触穿透查看")
                                    .font(.system(size: 10))
                                    .foregroundColor(.secondary)
                            }

                            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 6) {
                                ForEach(Array(allSubagents.enumerated()), id: \.offset) { _, sa in
                                    Button(action: {
                                        onSelectSubagent?(sa)
                                    }) {
                                        HStack(spacing: 6) {
                                            ZStack {
                                                Circle()
                                                    .fill(sa.isRunning ? Color.green.opacity(0.2) : Color.secondary.opacity(0.15))
                                                    .frame(width: 22, height: 22)
                                                Image(systemName: "cpu")
                                                    .font(.system(size: 10, weight: .bold))
                                                    .foregroundColor(sa.isRunning ? .green : .secondary)
                                            }

                                            VStack(alignment: .leading, spacing: 1) {
                                                Text(sa.displayName)
                                                    .font(.system(size: 11.5, weight: .semibold))
                                                    .foregroundColor(.primary)
                                                    .lineLimit(1)
                                                
                                                HStack(spacing: 3) {
                                                    if let tier = sa.modelTier, !tier.isEmpty {
                                                        Text(tier.uppercased())
                                                            .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                                                            .foregroundColor(tier.lowercased() == "pro" ? .purple : .blue)
                                                    }
                                                    Text(sa.isRunning ? "运行中" : "已就绪")
                                                        .font(.system(size: 9.5))
                                                        .foregroundColor(.secondary)
                                                }
                                            }

                                            Spacer()

                                            Image(systemName: "chevron.right")
                                                .font(.system(size: 9, weight: .bold))
                                                .foregroundColor(Color(uiColor: .tertiaryLabel))
                                        }
                                        .padding(8)
                                        .background(Color(uiColor: .tertiarySystemFill).opacity(0.5))
                                        .cornerRadius(8)
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }

                    // 3. Live Logs Stream (Collapsible)
                    let currentLogs: [String] = {
                        if !workflow.phases.isEmpty && selectedPhaseIndex < workflow.phases.count {
                            let pLogs = workflow.phases[selectedPhaseIndex].logs
                            if !pLogs.isEmpty { return pLogs }
                        }
                        return workflow.logs
                    }()

                    if !currentLogs.isEmpty {
                        VStack(alignment: .leading, spacing: 4) {
                            Button(action: {
                                withAnimation {
                                    isLogsExpanded.toggle()
                                }
                            }) {
                                HStack(spacing: 4) {
                                    Image(systemName: "terminal")
                                        .font(.system(size: 10))
                                    Text(isLogsExpanded ? "收起日志输出" : "展开执行日志 (\(currentLogs.count) 行)")
                                        .font(.system(size: 11, weight: .semibold))
                                    Spacer()
                                    Image(systemName: "chevron.right")
                                        .font(.system(size: 9, weight: .bold))
                                        .rotationEffect(.degrees(isLogsExpanded ? 90 : 0))
                                }
                                .foregroundColor(.secondary)
                            }
                            .buttonStyle(.plain)

                            if isLogsExpanded {
                                Text(currentLogs.joined(separator: "\n"))
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(.primary)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .padding(8)
                                    .background(Color.black.opacity(0.3))
                                    .cornerRadius(8)
                            }
                        }
                    }

                    // 4. Structured Output (JSON Schema result)
                    if let resultJson = workflow.resultJson, !resultJson.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                HStack(spacing: 4) {
                                    Image(systemName: "curlybraces")
                                        .font(.system(size: 11, weight: .bold))
                                        .foregroundColor(.orange)
                                    Text("结构化产物 (Schema Output)")
                                        .font(.system(size: 10.5, weight: .bold))
                                        .foregroundColor(.orange)
                                }
                                Spacer()
                                Button(action: {
                                    UIPasteboard.general.string = resultJson
                                }) {
                                    Image(systemName: "doc.on.doc")
                                        .font(.system(size: 10))
                                        .foregroundColor(.secondary)
                                }
                                .buttonStyle(.plain)
                            }

                            Text(resultJson)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.primary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(8)
                                .background(Color.black.opacity(0.35))
                                .cornerRadius(8)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 8)
                                        .stroke(Color.white.opacity(0.08), lineWidth: 0.5)
                                )
                        }
                    }

                    // 5. Error Banner (if failed)
                    if isError {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundColor(.red)
                                .font(.system(size: 12))
                            Text(workflow.error?.isEmpty == false ? workflow.error! : "工作流在执行中遇到未捕获异常或单节点失败")
                                .font(.system(size: 11))
                                .foregroundColor(.red)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(8)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.red.opacity(0.12))
                        .cornerRadius(8)
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
                .stroke(isError ? Color.red.opacity(0.4) : (isRunning ? Color.indigo.opacity(0.35) : Color(uiColor: .separator).opacity(0.4)), lineWidth: 0.8)
        )
    }
}
