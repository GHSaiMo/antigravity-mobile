import SwiftUI

public struct QueuedMessagesCardView: View {
    public let items: [QueuedMessageItem]
    public let onSendNow: @Sendable (QueuedMessageItem) -> Void
    public let onEdit: @Sendable (QueuedMessageItem) -> Void
    public let onDelete: @Sendable (QueuedMessageItem) -> Void
    
    @State private var isExpanded: Bool = true
    
    public init(
        items: [QueuedMessageItem],
        onSendNow: @escaping @Sendable (QueuedMessageItem) -> Void,
        onEdit: @escaping @Sendable (QueuedMessageItem) -> Void,
        onDelete: @escaping @Sendable (QueuedMessageItem) -> Void
    ) {
        self.items = items
        self.onSendNow = onSendNow
        self.onEdit = onEdit
        self.onDelete = onDelete
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header Row
            HStack(spacing: 8) {
                Text("队列")
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
                
                Text("当前任务完成后自动发送")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                
                Spacer()
                
                // Expand / Collapse Toggle
                Button(action: {
                    withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                        isExpanded.toggle()
                    }
                }) {
                    Image(systemName: "chevron.up")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isExpanded ? 0 : 180))
                        .frame(width: 28, height: 28)
                }
                .buttonStyle(.plain)
                .help(isExpanded ? "折叠队列" : "展开队列")
            }
            
            // Expandable Content
            if isExpanded {
                VStack(spacing: 6) {
                    ForEach(items) { item in
                        HStack(alignment: .center, spacing: 12) {
                            Text(item.text)
                                .font(.system(size: 14))
                                .lineSpacing(2)
                                .foregroundColor(.primary)
                                .lineLimit(3)
                                .frame(maxWidth: .infinity, alignment: .leading)
                            
                            // Action Decorators (Send Now, Edit, Delete)
                            HStack(spacing: 6) {
                                // Send Now
                                Button(action: { onSendNow(item) }) {
                                    Image(systemName: "arrow.right.circle.fill")
                                        .font(.system(size: 20))
                                        .foregroundColor(.accentColor)
                                }
                                .buttonStyle(.plain)
                                .help("立即发送")
                                
                                // Edit
                                Button(action: { onEdit(item) }) {
                                    Image(systemName: "pencil.circle.fill")
                                        .font(.system(size: 20))
                                        .foregroundColor(.secondary)
                                }
                                .buttonStyle(.plain)
                                .help("编辑")
                                
                                // Delete
                                Button(action: { onDelete(item) }) {
                                    Image(systemName: "trash.circle.fill")
                                        .font(.system(size: 20))
                                        .foregroundColor(.red.opacity(0.85))
                                }
                                .buttonStyle(.plain)
                                .help("删除")
                            }
                        }
                        .padding(.vertical, 6)
                        
                        if item.id != items.last?.id {
                            Divider().opacity(0.5)
                        }
                    }
                }
                .transition(.opacity.combined(with: .move(edge: .top)))
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 13)
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
