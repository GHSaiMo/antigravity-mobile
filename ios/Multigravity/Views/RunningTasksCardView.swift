import SwiftUI

public struct RunningTasksCardView: View {
    public let items: [RunningTaskItem]
    public let onStop: @MainActor @Sendable (RunningTaskItem) -> Void
    public let onToggleExpand: (@MainActor @Sendable (Bool) -> Void)?
    
    @State private var isCardExpanded: Bool = true
    @State private var expandedCommandIds: Set<String> = []
    @State private var copiedCommandId: String? = nil
    
    public init(
        items: [RunningTaskItem],
        onStop: @escaping @MainActor @Sendable (RunningTaskItem) -> Void,
        onToggleExpand: (@MainActor @Sendable (Bool) -> Void)? = nil
    ) {
        self.items = items
        self.onStop = onStop
        self.onToggleExpand = onToggleExpand
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header Row
            HStack(spacing: 8) {
                ProgressView()
                    .controlSize(.small)
                    .tint(.accentColor)
                
                Text("\(items.count) 个任务正在执行")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                
                // Count Badge
                Text("\(items.count)")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2.5)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(Capsule())
                
                Spacer()
                
                // Expand / Collapse Card Toggle
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                        isCardExpanded.toggle()
                    }
                    onToggleExpand?(isCardExpanded)
                }) {
                    Image(systemName: "chevron.down")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isCardExpanded ? 0 : 180))
                        .frame(width: 28, height: 28)
                }
                .buttonStyle(.plain)
                .help(isCardExpanded ? "折叠任务" : "展开任务")
            }
            .zIndex(10)
            
            // Expandable Content
            if isCardExpanded {
                VStack(spacing: 8) {
                    ForEach(items) { item in
                        taskRow(item: item)
                        
                        if item.id != items.last?.id {
                            Divider().opacity(0.4)
                        }
                    }
                }
                .clipped()
                .zIndex(1)
                .transition(
                    .asymmetric(
                        insertion: .opacity.combined(with: .scale(scale: 0.98, anchor: .top)),
                        removal: .opacity
                    )
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .clipped()
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
    }
    
    @ViewBuilder
    private func taskRow(item: RunningTaskItem) -> some View {
        let isCmdExpanded = expandedCommandIds.contains(item.id)
        let isCopied = copiedCommandId == item.id
        let isLong = isCommandLong(item.commandLine)
        
        VStack(alignment: .leading, spacing: 6) {
            // Task title and action buttons
            HStack(alignment: .center, spacing: 8) {
                let desc = item.toolSummary ?? item.toolAction ?? ToolLocalization.localizedName(for: item.toolName)
                if !desc.isEmpty {
                    Text(desc)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                }
                
                Spacer()
                
                // Copy Command Button
                Button(action: {
                    copyCommand(item.commandLine, id: item.id)
                }) {
                    HStack(spacing: 3) {
                        Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                            .font(.system(size: 11, weight: .medium))
                        if isCopied {
                            Text("已复制")
                                .font(.system(size: 10, weight: .medium))
                        }
                    }
                    .foregroundColor(isCopied ? .green : .secondary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 3)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                }
                .buttonStyle(.plain)
                .help("复制完整命令行")
                
                // Stop Task Button
                Button(action: {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    onStop(item)
                }) {
                    Image(systemName: "stop.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.red.opacity(0.9))
                }
                .buttonStyle(.plain)
                .help("终止任务")
            }
            
            // Terminal-style command block
            VStack(alignment: .leading, spacing: 4) {
                Text(highlightCommand(item.commandLine))
                    .font(.system(size: 11, design: .monospaced))
                    .lineLimit(isCmdExpanded ? nil : 3)
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                
                // Expand / Collapse command text button
                if isLong {
                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.2)) {
                            if isCmdExpanded {
                                expandedCommandIds.remove(item.id)
                            } else {
                                expandedCommandIds.insert(item.id)
                            }
                        }
                    }) {
                        HStack(spacing: 2) {
                            Text(isCmdExpanded ? "收起" : "展开完整命令")
                                .font(.system(size: 10, weight: .medium))
                            Image(systemName: isCmdExpanded ? "chevron.up" : "chevron.down")
                                .font(.system(size: 8, weight: .bold))
                        }
                        .foregroundColor(.accentColor)
                    }
                    .buttonStyle(.plain)
                    .padding(.top, 2)
                }
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(uiColor: .tertiarySystemFill).opacity(0.8))
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
        .padding(.vertical, 2)
    }
    
    private func isCommandLong(_ cmd: String) -> Bool {
        return cmd.count > 100 || cmd.components(separatedBy: "\n").count > 3
    }
    
    private func copyCommand(_ cmd: String, id: String) {
        UIPasteboard.general.string = cmd
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        copiedCommandId = id
        Task {
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            if copiedCommandId == id {
                copiedCommandId = nil
            }
        }
    }
    
    private func highlightCommand(_ command: String) -> AttributedString {
        var result = AttributedString("$ ")
        result.foregroundColor = Color.green.opacity(0.8)
        result.font = .system(size: 11, weight: .bold, design: .monospaced)
        
        let parts = command.components(separatedBy: " ")
        var isFirstWord = true
        for (idx, part) in parts.enumerated() {
            if idx > 0 {
                result.append(AttributedString(" "))
            }
            var attrPart = AttributedString(part)
            attrPart.font = .system(size: 11, design: .monospaced)
            if part.hasPrefix("-") {
                attrPart.foregroundColor = Color.orange
            } else if part == "&&" || part == "||" || part == "|" || part == ";" {
                attrPart.foregroundColor = Color.purple
                isFirstWord = true
            } else if isFirstWord && !part.isEmpty {
                attrPart.foregroundColor = Color.accentColor
                attrPart.font = .system(size: 11, weight: .semibold, design: .monospaced)
                isFirstWord = false
            } else {
                attrPart.foregroundColor = Color.primary.opacity(0.85)
            }
            result.append(attrPart)
        }
        return result
    }
}
