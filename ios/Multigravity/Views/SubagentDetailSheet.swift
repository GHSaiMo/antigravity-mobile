import SwiftUI

/// 工作流子智能体原位详情半屏抽屉 (SubagentDetailSheet)。
/// 用户在工作流拓扑看板中轻触 Sub-Agent 胶囊时呼出，不跳出主会话即可翻阅子智能体状态、Prompt 与产物。
public struct SubagentDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let subagent: SubagentItem
    public let onStopSubagent: ((SubagentItem) async -> Void)?
    public let onAskBtw: ((String) -> Void)?
    
    @State private var isStopping: Bool = false
    @State private var guidanceText: String = ""
    @State private var isSendingGuidance: Bool = false
    @State private var guidanceFeedback: String? = nil
    @State private var browserState: BrowserSubagentResponse? = nil
    @State private var isBrowserActionBusy: Bool = false
    @State private var browserFeedback: String? = nil

    private var isBrowserSubagent: Bool {
        let roleLower = (subagent.role ?? "").lowercased()
        let typeLower = (subagent.typeName ?? "").lowercased()
        return typeLower.contains("browser") || roleLower.contains("browser") || roleLower.contains("web")
    }
    
    public init(
        subagent: SubagentItem,
        onStopSubagent: ((SubagentItem) async -> Void)? = nil,
        onAskBtw: ((String) -> Void)? = nil
    ) {
        self.subagent = subagent
        self.onStopSubagent = onStopSubagent
        self.onAskBtw = onAskBtw
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
                                if let badge = subagent.modelBadgeText, !badge.isEmpty {
                                    let lower = (subagent.runtimeModel ?? subagent.modelTier ?? "").lowercased()
                                    let isHighTier = lower.contains("pro") || lower.contains("neon") || lower.contains("opus") || lower.contains("carbon")
                                    Text(badge)
                                        .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                                        .foregroundColor(.white)
                                        .lineLimit(1)
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(isHighTier ? Color.purple : Color.blue)
                                        .clipShape(Capsule())
                                }
                                
                                HStack(spacing: 4) {
                                    Circle()
                                        .fill(subagent.isRunning ? Color.green : Color.secondary)
                                        .frame(width: 6, height: 6)
                                    Text(subagent.statusText.isEmpty ? (subagent.isRunning ? String(localized: "运行中") : String(localized: "已就绪")) : subagent.statusText)
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
                            HStack {
                                Text("分配指令 (Initial Prompt)")
                                    .font(.system(size: 11.5, weight: .bold))
                                    .foregroundColor(.secondary)
                                Spacer()
                                if let onAskBtw {
                                    Button {
                                        dismiss()
                                        onAskBtw(prompt)
                                    } label: {
                                        Label("旁路快问", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                                            .font(.system(size: 11.5, weight: .semibold))
                                    }
                                    .buttonStyle(.bordered)
                                    .controlSize(.mini)
                                    .tint(.indigo)
                                }
                            }
                            
                            Text(prompt)
                                .font(.system(size: 13))
                                .foregroundColor(.primary)
                                .textSelection(.enabled)
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
                            if let rt = subagent.runtimeModel?.trimmingCharacters(in: .whitespacesAndNewlines), !rt.isEmpty {
                                Divider()
                                HStack(alignment: .top, spacing: 8) {
                                    Text("实际模型")
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                    Spacer()
                                    let suffix = subagent.requestedTierDisplay.map { " (\(String(localized: "请求")): \($0))" } ?? ""
                                    Text("\(rt)\(suffix)")
                                        .font(.system(size: 11.5, design: .monospaced))
                                        .foregroundColor(.primary)
                                        .multilineTextAlignment(.trailing)
                                }
                            }
                            Divider()
                            HStack {
                                Text("会话 ID")
                                    .font(.system(size: 12))
                                    .foregroundColor(.secondary)
                                Spacer()
                                Text(subagent.conversationId.isEmpty ? String(localized: "未生成") : subagent.conversationId)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                            }
                        }
                        .padding(10)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .cornerRadius(10)
                    }
                    
                    // Mid-run Guidance Input
                    if subagent.isRunning && !subagent.conversationId.isEmpty {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("向子代理发送指令")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.secondary)

                            HStack(spacing: 8) {
                                TextField("输入补充要求或纠偏指令...", text: $guidanceText)
                                    .font(.system(size: 13.5))
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 8)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))

                                Button {
                                    sendGuidance()
                                } label: {
                                    if isSendingGuidance {
                                        ProgressView().scaleEffect(0.8)
                                    } else {
                                        Text("发送")
                                            .font(.system(size: 13, weight: .semibold))
                                    }
                                }
                                .buttonStyle(.borderedProminent)
                                .tint(.indigo)
                                .disabled(isSendingGuidance || guidanceText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                            }

                            if let fb = guidanceFeedback {
                                Text(fb)
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.secondary)
                            }
                        }
                    }

                    // Browser Subagent Snapshot & Controls
                    if isBrowserSubagent && !subagent.conversationId.isEmpty {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Label("浏览器实时画面与拦截控制", systemImage: "safari")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.indigo)
                                Spacer()
                                Button {
                                    Task { await loadBrowserSnapshot() }
                                } label: {
                                    Image(systemName: "arrow.clockwise")
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                }
                            }

                            if let url = browserState?.url, !url.isEmpty {
                                Text(url)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.secondary)
                                    .lineLimit(2)
                            }

                            if let b64 = browserState?.imageBase64,
                               let imgData = Data(base64Encoded: b64),
                               let uiImg = UIImage(data: imgData) {
                                Image(uiImage: uiImg)
                                    .resizable()
                                    .scaledToFit()
                                    .frame(maxHeight: 220)
                                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            }

                            HStack(spacing: 10) {
                                if let curUrl = browserState?.url, !curUrl.isEmpty {
                                    Button {
                                        whitelistCurrentURL(curUrl)
                                    } label: {
                                        Label("加白名单", systemImage: "checkmark.shield")
                                            .font(.system(size: 12.5, weight: .semibold))
                                            .frame(maxWidth: .infinity)
                                    }
                                    .buttonStyle(.bordered)
                                    .tint(.indigo)
                                    .disabled(isBrowserActionBusy)
                                }

                                Button {
                                    skipCurrentBrowserStep()
                                } label: {
                                    Label("跳过此步", systemImage: "forward.frame")
                                        .font(.system(size: 12.5, weight: .semibold))
                                        .frame(maxWidth: .infinity)
                                }
                                .buttonStyle(.bordered)
                                .tint(.orange)
                                .disabled(isBrowserActionBusy)
                            }

                            if let bfb = browserFeedback {
                                Text(bfb)
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.secondary)
                            }
                        }
                        .padding(10)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
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
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .task {
            if isBrowserSubagent && !subagent.conversationId.isEmpty {
                await loadBrowserSnapshot()
            }
        }
    }

    private func sendGuidance() {
        let msg = guidanceText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !msg.isEmpty, let gwUrl = AppSettings.shared.gatewayURL else { return }
        isSendingGuidance = true
        Task {
            defer { isSendingGuidance = false }
            do {
                try await APIClient.shared.sendSubagentMessage(
                    recipient: subagent.conversationId,
                    content: msg,
                    baseURL: gwUrl
                )
                guidanceText = ""
                guidanceFeedback = String(localized: "指令已发送给子代理")
            } catch {
                guidanceFeedback = String(localized: "发送失败: \(error.localizedDescription)")
            }
        }
    }

    private func loadBrowserSnapshot() async {
        guard let gwUrl = AppSettings.shared.gatewayURL else { return }
        if let resp = try? await APIClient.shared.fetchBrowserSubagent(cascadeId: subagent.conversationId, baseURL: gwUrl) {
            browserState = resp
        }
    }

    private func whitelistCurrentURL(_ url: String) {
        guard let gwUrl = AppSettings.shared.gatewayURL else { return }
        isBrowserActionBusy = true
        Task {
            defer { isBrowserActionBusy = false }
            do {
                try await APIClient.shared.whitelistBrowserUrl(url: url, baseURL: gwUrl)
                browserFeedback = String(localized: "已加入站点白名单")
            } catch {
                browserFeedback = String(localized: "加白名单失败: \(error.localizedDescription)")
            }
        }
    }

    private func skipCurrentBrowserStep() {
        guard let gwUrl = AppSettings.shared.gatewayURL else { return }
        isBrowserActionBusy = true
        Task {
            defer { isBrowserActionBusy = false }
            do {
                try await APIClient.shared.skipBrowserSubagent(cascadeId: subagent.conversationId, baseURL: gwUrl)
                browserFeedback = String(localized: "已通知跳过当前浏览器拦截步骤")
                await loadBrowserSnapshot()
            } catch {
                browserFeedback = String(localized: "跳过失败: \(error.localizedDescription)")
            }
        }
    }
}
