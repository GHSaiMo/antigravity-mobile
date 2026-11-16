import SwiftUI

/// 动态多智能体工作流安全审批卡片 (Antigravity 2.32+ Dynamic Workflows)。
/// 用于在移动端直观审查 Python 编排脚本、查看预估 Phase/Subagent 结构，并提供四档快速决策。
public struct WorkflowApprovalCardView: View {
    public let interaction: PendingInteraction
    public let isSubmitting: Bool
    public let onSubmit: (String, String?) -> Void
    
    @State private var isCodeExpanded: Bool = false
    @State private var isScriptCopied: Bool = false
    @State private var showFeedbackInput: Bool = false
    @State private var feedbackText: String = ""
    
    public init(
        interaction: PendingInteraction,
        isSubmitting: Bool,
        onSubmit: @escaping (String, String?) -> Void
    ) {
        self.interaction = interaction
        self.isSubmitting = isSubmitting
        self.onSubmit = onSubmit
    }
    
    private var approval: WorkflowApprovalDetails? {
        interaction.workflowApproval
    }
    
    private var scriptContent: String {
        approval?.script?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    /// 静态提取工作流脚本的关键权限风险，无需展开通读全部源码即可预判边界。
    private var scriptCapabilityTags: [(icon: String, label: String, color: Color)] {
        guard !scriptContent.isEmpty else { return [] }
        let lower = scriptContent.lowercased()
        var tags: [(String, String, Color)] = []
        let canWrite = lower.contains("enablewritetools=true")
            || lower.contains("enablewritetools = true")
            || lower.contains("write_file")
            || lower.contains("edit_file")
            || lower.contains("shutil.")
            || lower.contains("os.remove")
            || lower.contains("unlink(")
        let canRunCommand = lower.contains("run_command")
            || lower.contains("subprocess")
            || lower.contains("os.system")
            || lower.contains("popen(")
        if canWrite {
            tags.append(("pencil.and.outline", String(localized: "可修改或写入文件"), .orange))
        }
        if canRunCommand {
            tags.append(("terminal.fill", String(localized: "可执行本地命令"), .red))
        }
        if tags.isEmpty {
            tags.append(("eye", String(localized: "仅只读分析与调度"), .green))
        }
        return tags
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            // Header
            HStack(spacing: 8) {
                ZStack {
                    Circle()
                        .fill(Color.indigo.opacity(0.18))
                        .frame(width: 32, height: 32)
                    Image(systemName: "point.3.connected.trianglepath.dotted")
                        .font(.system(size: 15, weight: .bold))
                        .foregroundColor(.indigo)
                }
                
                VStack(alignment: .leading, spacing: 2) {
                    Text("Workflow 执行审批")
                        .font(.system(size: 14.5, weight: .bold))
                        .foregroundColor(.primary)
                    Text("Agent 申请执行多智能体 Python 编排脚本")
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                }
                
                Spacer()
                
                Text("安全确认")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(.indigo)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .background(Color.indigo.opacity(0.12))
                    .clipShape(Capsule())
            }
            
            // Goal Description
            if let desc = approval?.description, !desc.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("目标与范围")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.secondary)
                    Text(desc)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.primary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color(uiColor: .tertiarySystemFill).opacity(0.5))
                .cornerRadius(10)
            }
            
            // Estimated Phases / Concurrency
            if let phases = approval?.estimatedPhases, !phases.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    HStack {
                        Text("预估执行阶段 (\(phases.count))")
                            .font(.system(size: 11, weight: .bold))
                            .foregroundColor(.secondary)
                        Spacer()
                        if let conc = approval?.maxConcurrency, conc > 1 {
                            Text("最大并发: \(conc)")
                                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                .foregroundColor(.indigo)
                        }
                    }
                    
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 6) {
                            ForEach(Array(phases.enumerated()), id: \.offset) { idx, p in
                                HStack(spacing: 4) {
                                    Text("\(idx + 1)")
                                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                                        .foregroundColor(.white)
                                        .frame(width: 16, height: 16)
                                        .background(Color.indigo)
                                        .clipShape(Circle())
                                    Text(p)
                                        .font(.system(size: 11.5, weight: .medium))
                                        .foregroundColor(.primary)
                                }
                                .padding(.horizontal, 8)
                                .padding(.vertical, 5)
                                .background(Color(uiColor: .tertiarySystemFill).opacity(0.7))
                                .cornerRadius(8)
                            }
                        }
                    }
                }
            }
            
            // Python Script Code Preview (Collapsible)
            if !scriptContent.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    if !scriptCapabilityTags.isEmpty {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 6) {
                                ForEach(Array(scriptCapabilityTags.enumerated()), id: \.offset) { _, tag in
                                    HStack(spacing: 4) {
                                        Image(systemName: tag.icon)
                                            .font(.system(size: 10, weight: .semibold))
                                        Text(tag.label)
                                            .font(.system(size: 11, weight: .semibold))
                                    }
                                    .foregroundColor(tag.color)
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 3.5)
                                    .background(tag.color.opacity(0.12))
                                    .clipShape(Capsule())
                                }
                            }
                        }
                    }

                    HStack {
                        Button(action: {
                            withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                                isCodeExpanded.toggle()
                            }
                        }) {
                            HStack(spacing: 5) {
                                Image(systemName: "chevron.right")
                                    .font(.system(size: 10.5, weight: .bold))
                                    .rotationEffect(.degrees(isCodeExpanded ? 90 : 0))
                                let lineCount = scriptContent.components(separatedBy: "\n").count
                                Text(isCodeExpanded ? String(localized: "收起 Python 脚本") : String(localized: "查看 Python 脚本源码"))
                                    .font(.system(size: 11.5, weight: .semibold))
                                Text("(\(lineCount) 行)")
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                            }
                            .foregroundColor(.indigo)
                        }
                        .buttonStyle(.plain)
                        
                        Spacer()
                        
                        Button(action: {
                            UIPasteboard.general.string = scriptContent
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            isScriptCopied = true
                            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                                isScriptCopied = false
                            }
                        }) {
                            HStack(spacing: 3) {
                                Image(systemName: isScriptCopied ? "checkmark" : "doc.on.doc")
                                    .font(.system(size: 10))
                                Text(isScriptCopied ? String(localized: "已复制") : String(localized: "复制"))
                                    .font(.system(size: 10.5))
                            }
                            .foregroundColor(isScriptCopied ? .green : .secondary)
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("复制脚本源码")
                    }
                    
                    if isCodeExpanded {
                        ScrollView(.vertical) {
                            Text(scriptContent)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.primary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(8)
                        }
                        .frame(maxHeight: 180)
                        .background(Color(uiColor: .tertiarySystemBackground))
                        .cornerRadius(8)
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(Color(uiColor: .separator).opacity(0.45), lineWidth: 0.6)
                        )
                    }
                }
            }
            
            // Feedback write-in field (if selected)
            if showFeedbackInput {
                VStack(alignment: .leading, spacing: 4) {
                    Text("向 Agent 说明打回原因或修改诉求:")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.secondary)
                    
                    HStack {
                        TextField("例如：请减少并发数，先审查 auth 模块...", text: $feedbackText)
                            .font(.system(size: 12.5))
                            .textFieldStyle(.roundedBorder)
                        
                        Button(action: {
                            onSubmit("5", feedbackText.isEmpty ? String(localized: "用户拒绝执行该工作流") : feedbackText)
                        }) {
                            Text("发送打回")
                                .font(.system(size: 12, weight: .bold))
                                .foregroundColor(.white)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 6)
                                .background(Color.red)
                                .cornerRadius(6)
                        }
                        .disabled(isSubmitting)
                    }
                }
                .padding(.top, 2)
            }
            
            // 4 Decision Actions
            VStack(spacing: 8) {
                // Primary: Allow Once
                Button(action: {
                    onSubmit("1", nil)
                }) {
                    HStack {
                        if isSubmitting {
                            ProgressView()
                                .tint(.white)
                                .scaleEffect(0.8)
                        }
                        Image(systemName: "checkmark.circle.fill")
                            .font(.system(size: 13, weight: .bold))
                        Text("允许本次执行")
                            .font(.system(size: 13.5, weight: .bold))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                    .background(Color.indigo)
                    .foregroundColor(.white)
                    .cornerRadius(10)
                    .shadow(color: Color.indigo.opacity(0.3), radius: 4, x: 0, y: 2)
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting)
                
                HStack(spacing: 8) {
                    // Session Allow
                    Button(action: {
                        onSubmit("2", nil)
                    }) {
                        Text("本会话始终允许")
                            .font(.system(size: 12, weight: .medium))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 8)
                            .background(Color(uiColor: .tertiarySystemFill))
                            .foregroundColor(.primary)
                            .cornerRadius(8)
                    }
                    .buttonStyle(.plain)
                    .disabled(isSubmitting)
                    
                    // Deny
                    Button(action: {
                        onSubmit("5", nil)
                    }) {
                        Text("拒绝执行")
                            .font(.system(size: 12, weight: .medium))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 8)
                            .background(Color.red.opacity(0.12))
                            .foregroundColor(.red)
                            .cornerRadius(8)
                    }
                    .buttonStyle(.plain)
                    .disabled(isSubmitting)
                    
                    // Reject with Feedback toggle
                    Button(action: {
                        withAnimation {
                            showFeedbackInput.toggle()
                        }
                    }) {
                        Text(showFeedbackInput ? String(localized: "收起反馈") : String(localized: "补充反馈打回"))
                            .font(.system(size: 12, weight: .medium))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 8)
                            .background(Color(uiColor: .tertiarySystemFill))
                            .foregroundColor(.secondary)
                            .cornerRadius(8)
                    }
                    .buttonStyle(.plain)
                    .disabled(isSubmitting)
                }
            }
            .padding(.top, 4)
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemBackground))
        .cornerRadius(16)
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(Color.indigo.opacity(0.35), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.08), radius: 6, x: 0, y: 3)
    }
}
