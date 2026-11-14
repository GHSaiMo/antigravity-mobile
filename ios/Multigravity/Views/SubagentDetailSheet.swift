import SwiftUI

/// 工作流子智能体原位详情半屏抽屉 (SubagentDetailSheet)。
/// 用户在工作流拓扑看板中轻触 Sub-Agent 胶囊时呼出，不跳出主会话即可翻阅子智能体状态、Prompt 与产物。
public struct SubagentDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let subagent: SubagentItem
    public let onStopSubagent: ((SubagentItem) async -> Void)?
    
    @State private var isStopping: Bool = false
    
    public init(
        subagent: SubagentItem,
        onStopSubagent: ((SubagentItem) async -> Void)? = nil
    ) {
        self.subagent = subagent
        self.onStopSubagent = onStopSubagent
    }
    
    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    // Header Card
                    HStack(spacing: 12) {
                        ZStack {
                            Circle()
                                .fill(Color.indigo.opacity(0.18))
                                .frame(width: 44, height: 44)
                            Image(systemName: "cpu")
                                .font(.system(size: 20, weight: .bold))
                                .foregroundColor(.indigo)
                        }
                        
                        VStack(alignment: .leading, spacing: 3) {
                            Text(subagent.displayName)
                                .font(.system(size: 16, weight: .bold))
                                .foregroundColor(.primary)
                            
                            HStack(spacing: 6) {
                                if let tier = subagent.modelTier, !tier.isEmpty {
                                    Text(tier.uppercased())
                                        .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                                        .foregroundColor(.white)
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(tier.lowercased() == "pro" ? Color.purple : Color.blue)
                                        .clipShape(Capsule())
                                }
                                
                                HStack(spacing: 4) {
                                    Circle()
                                        .fill(subagent.isRunning ? Color.green : Color.secondary)
                                        .frame(width: 6, height: 6)
                                    Text(subagent.statusText.isEmpty ? (subagent.isRunning ? "运行中" : "已就绪") : subagent.statusText)
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                }
                                
                                if let steps = subagent.stepCount, steps > 0 {
                                    Text("• \(steps) 步")
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                }
                            }
                        }
                        
                        Spacer()
                    }
                    .padding(12)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(12)
                    
                    // Initial Prompt Section
                    if let prompt = subagent.prompt, !prompt.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("分配指令 (Initial Prompt)")
                                .font(.system(size: 11.5, weight: .bold))
                                .foregroundColor(.secondary)
                            
                            Text(prompt)
                                .font(.system(size: 13))
                                .foregroundColor(.primary)
                                .padding(10)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .background(Color(uiColor: .tertiarySystemFill).opacity(0.6))
                                .cornerRadius(10)
                        }
                    }
                    
                    // Metadata Info
                    VStack(alignment: .leading, spacing: 8) {
                        Text("运行时元数据")
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundColor(.secondary)
                        
                        VStack(spacing: 6) {
                            HStack {
                                Text("智能体类型")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Text(subagent.typeName ?? "self")
                                    .font(.system(size: 12, design: .monospaced))
                                    .foregroundColor(.primary)
                            }
                            Divider()
                            HStack {
                                Text("会话 ID")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Text(subagent.conversationId.isEmpty ? "未生成" : subagent.conversationId)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                            }
                        }
                        .padding(10)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .cornerRadius(10)
                    }
                    
                    // Stop Subagent Button (if running)
                    if subagent.isRunning && onStopSubagent != nil {
                        Button(action: {
                            Task {
                                isStopping = true
                                await onStopSubagent?(subagent)
                                isStopping = false
                                dismiss()
                            }
                        }) {
                            HStack {
                                if isStopping {
                                    ProgressView()
                                        .tint(.white)
                                        .scaleEffect(0.8)
                                }
                                Image(systemName: "stop.circle.fill")
                                Text("终止该子代理")
                            }
                            .font(.system(size: 13, weight: .bold))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 10)
                            .background(Color.red)
                            .foregroundColor(.white)
                            .cornerRadius(10)
                        }
                        .buttonStyle(.plain)
                        .disabled(isStopping)
                        .padding(.top, 6)
                    }
                }
                .padding(16)
            }
            .navigationTitle("子代理详情")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("关闭") {
                        dismiss()
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
    }
}
