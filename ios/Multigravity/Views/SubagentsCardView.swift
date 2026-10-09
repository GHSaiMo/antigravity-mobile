import SwiftUI

/// 当前会话派发的子代理：点一行进入子会话（只读），运行中的可单独关停。
public struct SubagentsCardView: View {
    public let items: [SubagentItem]
    public let canStop: Bool
    public let onStop: @MainActor @Sendable (SubagentItem) -> Void
    public let onToggleExpand: (@MainActor @Sendable (Bool) -> Void)?

    @State private var isExpanded: Bool = true
    @State private var pendingStop: SubagentItem? = nil

    public init(
        items: [SubagentItem],
        canStop: Bool = true,
        onStop: @escaping @MainActor @Sendable (SubagentItem) -> Void,
        onToggleExpand: (@MainActor @Sendable (Bool) -> Void)? = nil
    ) {
        self.items = items
        self.canStop = canStop
        self.onStop = onStop
        self.onToggleExpand = onToggleExpand
    }

    private var runningCount: Int { items.filter(\.isRunning).count }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                if runningCount > 0 {
                    ProgressView().controlSize(.small).tint(.accentColor)
                } else {
                    Image(systemName: "person.2.fill")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                }
                Text(runningCount > 0 ? "\(runningCount) 个子代理运行中" : "\(items.count) 个子代理")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                Text("\(items.count)")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2.5)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(Capsule())
                Spacer()
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) { isExpanded.toggle() }
                    onToggleExpand?(isExpanded)
                }) {
                    Image(systemName: "chevron.down")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : 180))
                        .frame(width: 28, height: 28)
                }
                .buttonStyle(.plain)
            }

            if isExpanded {
                VStack(spacing: 6) {
                    ForEach(items) { item in
                        row(item)
                        if item.id != items.last?.id { Divider().opacity(0.5) }
                    }
                }
                .transition(.opacity)
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
        )
        .shadow(color: Color.black.opacity(0.12), radius: 10, x: 0, y: 4)
        .padding(.horizontal, 12)
        .padding(.bottom, 6)
        .confirmationDialog(
            "关停子代理？",
            isPresented: Binding(get: { pendingStop != nil }, set: { if !$0 { pendingStop = nil } }),
            titleVisibility: .visible,
            presenting: pendingStop
        ) { item in
            Button("关停「\(item.displayName)」", role: .destructive) { onStop(item) }
            Button("取消", role: .cancel) {}
        } message: { _ in
            Text("它正在进行的工作会被中断，已完成的内容会保留。")
        }
    }

    @ViewBuilder
    private func row(_ item: SubagentItem) -> some View {
        HStack(alignment: .center, spacing: 12) {
            if item.isGone {
                info(item)
            } else {
                NavigationLink(value: item.conversationItem) {
                    HStack(spacing: 6) {
                        info(item)
                        Image(systemName: "chevron.right")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(.secondary.opacity(0.6))
                    }
                }
                .buttonStyle(.plain)
            }

            if item.isRunning && canStop {
                Button(action: {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    pendingStop = item
                }) {
                    Image(systemName: "stop.circle.fill")
                        .font(.system(size: 22))
                        .foregroundColor(.red.opacity(0.9))
                }
                .buttonStyle(.plain)
                .accessibilityLabel("关停子代理")
            }
        }
        .padding(.vertical, 4)
    }

    private func info(_ item: SubagentItem) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Circle()
                    .fill(item.isRunning ? Color.green : Color.secondary.opacity(0.5))
                    .frame(width: 7, height: 7)
                Text(item.displayName)
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(item.isGone ? .secondary : .primary)
                    .lineLimit(1)
                if let t = item.typeName, !t.isEmpty, t != item.displayName {
                    Text(t)
                        .font(.system(size: 10, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Capsule())
                }
            }
            HStack(spacing: 6) {
                if !item.statusText.isEmpty {
                    Text(item.statusText)
                }
                if let n = item.stepCount, n > 0 {
                    Text("· \(n) 步")
                }
            }
            .font(.system(size: 11))
            .foregroundColor(.secondary)
            if let p = item.prompt, !p.isEmpty {
                Text(p)
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .lineLimit(2)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
    }
}

/// 子代理会话顶部的只读提示条（替代输入栏）。
public struct SubagentReadOnlyBar: View {
    public let role: String?
    public let isRunning: Bool
    public let canStop: Bool
    public let onStop: @MainActor @Sendable () -> Void

    @State private var confirmStop = false

    public init(role: String?, isRunning: Bool, canStop: Bool, onStop: @escaping @MainActor @Sendable () -> Void) {
        self.role = role
        self.isRunning = isRunning
        self.canStop = canStop
        self.onStop = onStop
    }

    public var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "person.2.fill")
                .foregroundColor(.secondary)
            VStack(alignment: .leading, spacing: 2) {
                Text("子代理会话 · 只读")
                    .font(.system(size: 13, weight: .semibold))
                if let role, !role.isEmpty {
                    Text(role)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer()
            if isRunning && canStop {
                Button("关停") { confirmStop = true }
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.red)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .confirmationDialog("关停这个子代理？", isPresented: $confirmStop, titleVisibility: .visible) {
            Button("关停", role: .destructive) { onStop() }
            Button("取消", role: .cancel) {}
        } message: {
            Text("它正在进行的工作会被中断，已完成的内容会保留。")
        }
    }
}

/// 消息流里内联的子代理卡片（对齐桌面端）：显示角色、类型、状态，点一下进入子会话（只读）。
public struct SubagentInlineCardView: View {
    public let item: SubagentItem

    public init(item: SubagentItem) {
        self.item = item
    }

    public var body: some View {
        let card = HStack(spacing: 10) {
            statusIcon
            VStack(alignment: .leading, spacing: 2) {
                Text(item.displayName)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(item.isGone ? .secondary : .primary)
                    .lineLimit(1)
                HStack(spacing: 6) {
                    if let t = item.typeName, !t.isEmpty, t != item.displayName {
                        Text(t)
                    }
                    if !item.statusText.isEmpty, item.status != "done" {
                        Text(item.statusText)
                    }
                    if let n = item.stepCount, n > 0, item.isRunning {
                        Text("\(n) 步")
                    }
                }
                .font(.system(size: 12))
                .foregroundColor(.secondary)
            }
            Spacer(minLength: 8)
            if !item.isGone {
                Image(systemName: "chevron.right")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(.secondary.opacity(0.6))
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
        )
        .contentShape(Rectangle())

        if item.isGone {
            card
        } else {
            NavigationLink(value: item.conversationItem) { card }
                .buttonStyle(.plain)
        }
    }

    @ViewBuilder
    private var statusIcon: some View {
        if item.isRunning {
            ProgressView().controlSize(.small)
        } else if item.isGone {
            Image(systemName: "questionmark.circle")
                .font(.system(size: 16))
                .foregroundColor(.secondary)
        } else {
            Image(systemName: "checkmark.circle")
                .font(.system(size: 16))
                .foregroundColor(.secondary)
        }
    }
}
