import SwiftUI

/// 动态多智能体工作流安全审批卡片 (Antigravity 2.32+ Dynamic Workflows)。
/// 用于在移动端直观审查 Python 编排脚本、查看预估 Phase/Subagent 结构，并提供四档快速决策。
public struct WorkflowApprovalCardView: View {
    public let interaction: PendingInteraction
    public let isSubmitting: Bool
    public let onSubmit: (String, String?) -> Void
    
    @State private var isCodeExpanded: Bool = false
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
                                Text(isCodeExpanded ? "收起 Python 脚本" : "查看 Python 脚本源码")
                                    .font(.system(size: 11.5, weight: .semibold))
                                Text("(\(scriptContent.components(separatedBy: "\n").count) 行)")
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                            }
                            .foregroundColor(.indigo)
                        }
                        .buttonStyle(.plain)
                        
                        Spacer()
                        
                        Button(action: {
                            UIPasteboard.general.string = scriptContent
                        }) {
                            HStack(spacing: 3) {
                                Image(systemName: "doc.on.doc")
                                    .font(.system(size: 10))
                                Text("复制")
                                    .font(.system(size: 10.5))
                            }
                            .foregroundColor(.secondary)
                        }
                        .buttonStyle(.plain)
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
                        .background(Color.black.opacity(0.35))
                        .cornerRadius(8)
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(Color.white.opacity(0.08), lineWidth: 0.5)
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
                            onSubmit("5", feedbackText.isEmpty ? "用户拒绝执行该工作流" : feedbackText)
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
                        Text("允许本次执行 (Allow Once)")
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
                        Text(showFeedbackInput ? "收起反馈" : "补充反馈打回")
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
