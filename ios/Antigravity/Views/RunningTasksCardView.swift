import SwiftUI

public struct RunningTasksCardView: View {
    public let items: [RunningTaskItem]
    public let onStop: @MainActor @Sendable (RunningTaskItem) -> Void
    
    @State private var isExpanded: Bool = true
    
    public init(
        items: [RunningTaskItem],
        onStop: @escaping @MainActor @Sendable (RunningTaskItem) -> Void
    ) {
        self.items = items
        self.onStop = onStop
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header Row
            HStack(spacing: 8) {
                // Progress spinning indicator
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
                
                // Expand / Collapse Toggle
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                        isExpanded.toggle()
                    }
                }) {
                    Image(systemName: "chevron.down")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : 180))
                        .frame(width: 28, height: 28)
                }
                .buttonStyle(.plain)
                .help(isExpanded ? "折叠任务" : "展开任务")
            }
            
            // Expandable Content
            if isExpanded {
                VStack(spacing: 6) {
                    ForEach(items) { item in
                        HStack(alignment: .bottom, spacing: 12) {
                            VStack(alignment: .leading, spacing: 3) {
                                if let desc = item.toolSummary ?? item.toolAction ?? item.toolName, !desc.isEmpty {
                                    Text(desc)
                                        .font(.system(size: 12, weight: .medium))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                }
                                
                                Text(item.commandLine)
                                    .font(.system(size: 12, design: .monospaced))
                                    .lineLimit(2)
                                    .foregroundColor(.secondary)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 3)
                                    .background(Color(uiColor: .tertiarySystemFill))
                                    .cornerRadius(6)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            
                            // Stop Task Button
                            Button(action: {
                                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                                onStop(item)
                            }) {
                                Image(systemName: "stop.circle.fill")
                                    .font(.system(size: 22))
                                    .foregroundColor(.red.opacity(0.9))
                            }
                            .buttonStyle(.plain)
                            .help("终止任务")
                        }
                        .padding(.vertical, 4)
                        
                        if item.id != items.last?.id {
                            Divider().opacity(0.5)
                        }
                    }
                }
                .transition(.opacity.combined(with: .move(edge: .top)))
            }
        }
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
}
