import SwiftUI
import Photos

extension MessageBubbleView {
    struct NoTapAnimationButtonStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
        }
    }
    
    public struct ToolBatchAccordionView: View {
        public let count: Int
        public let tools: [String]
        public let thinking: String?
        public let thinkingDuration: String?
        public let toolDetails: [ToolDetailItem]?
        @State private var isExpanded: Bool = false
        
        public init(
            count: Int,
            tools: [String],
            thinking: String? = nil,
            thinkingDuration: String? = nil,
            toolDetails: [ToolDetailItem]? = nil
        ) {
            self.count = count
            self.tools = tools
            self.thinking = thinking
            self.thinkingDuration = thinkingDuration
            self.toolDetails = toolDetails
        }
        
        private var hasTools: Bool {
            max(count, tools.count) > 0 || !(toolDetails?.isEmpty ?? true)
        }

        private var resolvedTools: [String] {
            if !tools.isEmpty {
                return tools
            }
            let actualCount = max(count, 1)
            return (0..<actualCount).map { _ in "tool_call" }
        }
        
        private var summaryText: String {
            let actualCount = max(count, tools.count)
            let durSuffix = (thinkingDuration?.isEmpty == false) ? " · \(thinkingDuration!)" : ""
            if actualCount == 0, let thinking, !thinking.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return String(localized: "已深度思考") + durSuffix
            }
            let uniqueNames = Array(NSOrderedSet(array: tools)).compactMap { $0 as? String }
            if !uniqueNames.isEmpty {
                let localizedNames = uniqueNames.map { ToolLocalization.localizedName(for: $0) }
                let namesSummary = localizedNames.joined(separator: ", ")
                return String(localized: "已思考并执行 \(actualCount) 项操作 (\(namesSummary))") + durSuffix
            }
            return String(localized: "已思考并执行 \(actualCount) 项操作") + durSuffix
        }
        
        public var body: some View {
            VStack(alignment: .leading, spacing: 0) {
                // Header Capsule: stays strictly static, never dims on tap, chevron rotates smoothly
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                        isExpanded.toggle()
                    }
                }) {
                    HStack(spacing: 8) {
                        Image(systemName: (count == 0 && tools.isEmpty && !(thinking ?? "").isEmpty) ? "brain.head.profile" : "bolt.fill")
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundColor((count == 0 && tools.isEmpty && !(thinking ?? "").isEmpty) ? .indigo : .orange)
                        
                        Text(summaryText)
                            .font(.system(size: 12.5, weight: .medium))
                            .foregroundColor(.primary.opacity(0.85))
                            .lineLimit(1)
                        
                        Spacer(minLength: 6)
                        
                        Image(systemName: "chevron.down")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(.secondary.opacity(0.7))
                            .rotationEffect(.degrees(isExpanded ? 180 : 0))
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(Capsule())
                    .overlay(
                        Capsule()
                            .strokeBorder(Color.primary.opacity(0.08), lineWidth: 0.8)
                    )
                }
                .buttonStyle(NoTapAnimationButtonStyle())
                .zIndex(2)
                
                // Expandable tool details: lazily mounted & strictly clipped below the header capsule
                AccordionRevealContainer(isExpanded: isExpanded, topSpacing: 6) {
                    VStack(alignment: .leading, spacing: 5) {
                        if let thinking = thinking?.trimmingCharacters(in: .whitespacesAndNewlines), !thinking.isEmpty {
                            CollapsibleThinkingBlockView(
                                thinking: thinking,
                                initiallyExpanded: !hasTools
                            )
                        }

                        if let details = toolDetails, !details.isEmpty {
                            ForEach(Array(details.enumerated()), id: \.offset) { _, detail in
                                toolDetailRow(detail)
                            }
                        } else if hasTools {
                            ForEach(Array(resolvedTools.enumerated()), id: \.offset) { _, toolName in
                                HStack(spacing: 8) {
                                    Image(systemName: "puzzlepiece.extension.fill")
                                        .font(.system(size: 11, weight: .semibold))
                                        .foregroundColor(Color(red: 0.3, green: 0.85, blue: 0.4))
                                    
                                    Text(ToolLocalization.localizedName(for: toolName))
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                    
                                    Spacer()
                                }
                                .padding(.horizontal, 12)
                                .padding(.vertical, 7)
                                .background(Color(uiColor: .tertiarySystemFill).opacity(0.8))
                                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                                        .strokeBorder(Color.primary.opacity(0.05), lineWidth: 0.5)
                                )
                            }
                        }
                    }
                }
                .zIndex(1)
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }

        private func toolDetailRow(_ item: ToolDetailItem) -> some View {
            let isErr = item.status.lowercased().contains("err") || item.status.lowercased().contains("fail")
            return HStack(spacing: 8) {
                Image(systemName: isErr ? "xmark.circle.fill" : "puzzlepiece.extension.fill")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(isErr ? .red : Color(red: 0.3, green: 0.85, blue: 0.4))

                VStack(alignment: .leading, spacing: 1) {
                    Text(ToolLocalization.localizedName(for: item.name))
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    if !item.detail.isEmpty {
                        Text(item.detail)
                            .font(.system(size: 10.5, design: .monospaced))
                            .foregroundColor(.secondary)
                            .lineLimit(2)
                    }
                }

                Spacer(minLength: 4)

                if let dur = item.duration, !dur.isEmpty {
                    Text(dur)
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 2)
                        .background(Color.secondary.opacity(0.12))
                        .clipShape(Capsule())
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 7)
            .background(Color(uiColor: .tertiarySystemFill).opacity(0.8))
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .strokeBorder(Color.primary.opacity(0.05), lineWidth: 0.5)
            )
        }
    }
    
}
