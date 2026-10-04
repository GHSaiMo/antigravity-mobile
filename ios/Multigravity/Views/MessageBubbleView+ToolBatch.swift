import SwiftUI
import Photos

extension MessageBubbleView {
    struct ToolContentHeightPreferenceKey: PreferenceKey {
        static var defaultValue: CGFloat = 0
        static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
            let next = nextValue()
            if next > 0 {
                value = next
            }
        }
    }
    
    struct NoTapAnimationButtonStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
        }
    }
    
    public struct ToolBatchAccordionView: View {
        public let count: Int
        public let tools: [String]
        @State private var isExpanded: Bool = false
        @State private var contentHeight: CGFloat = 0
        
        public init(count: Int, tools: [String]) {
            self.count = count
            self.tools = tools
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
            let uniqueNames = Array(NSOrderedSet(array: tools)).compactMap { $0 as? String }
            if !uniqueNames.isEmpty {
                let namesSummary = uniqueNames.joined(separator: ", ")
                return "已思考并执行 \(actualCount) 项操作 (\(namesSummary))"
            }
            return "已思考并执行 \(actualCount) 项操作"
        }
        
        public var body: some View {
            VStack(alignment: .leading, spacing: 0) {
                // Header Capsule: stays strictly static, never dims on tap, chevron rotates smoothly
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.32, dampingFraction: 0.95)) {
                        isExpanded.toggle()
                    }
                }) {
                    HStack(spacing: 8) {
                        Image(systemName: "bolt.fill")
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundColor(.orange)
                        
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
                            .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                    )
                }
                .buttonStyle(NoTapAnimationButtonStyle())
                .zIndex(2)
                
                // Expandable tool details: strictly below the header, unfolds downward on expand, folds upward on collapse
                VStack(alignment: .leading, spacing: 5) {
                    ForEach(Array(resolvedTools.enumerated()), id: \.offset) { idx, toolName in
                        HStack(spacing: 8) {
                            Image(systemName: "puzzlepiece.extension.fill")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundColor(Color(red: 0.3, green: 0.85, blue: 0.4))
                            
                            Text(toolName)
                                .font(.system(size: 12, weight: .semibold, design: .monospaced))
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
                                .stroke(Color.primary.opacity(0.05), lineWidth: 0.5)
                        )
                    }
                }
                .fixedSize(horizontal: false, vertical: true)
                .background(
                    GeometryReader { geo in
                        Color.clear.preference(
                            key: ToolContentHeightPreferenceKey.self,
                            value: geo.size.height
                        )
                    }
                )
                .padding(.top, isExpanded ? 6 : 0)
                .frame(
                    maxWidth: .infinity,
                    maxHeight: isExpanded ? (contentHeight > 0 ? contentHeight + 6 : nil) : 0,
                    alignment: .topLeading
                )
                .opacity(isExpanded ? 1 : 0)
                .clipped()
                .allowsHitTesting(isExpanded)
                .zIndex(1)
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
            .onPreferenceChange(ToolContentHeightPreferenceKey.self) { height in
                if height > 0 {
                    contentHeight = height
                }
            }
        }
    }
    
}
