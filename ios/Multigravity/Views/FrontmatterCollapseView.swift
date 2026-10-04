import SwiftUI

// MARK: - Frontmatter Collapse View

public struct FrontmatterCollapseView: View {
    public let content: String
    public let lineCount: Int
    @State private var isExpanded: Bool = false
    
    public init(content: String, lineCount: Int) {
        self.content = content
        self.lineCount = lineCount
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button(action: {
                withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                    isExpanded.toggle()
                }
            }) {
                HStack(spacing: 7) {
                    Image(systemName: "slider.horizontal.3")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.secondary)
                    
                    Text("已自动隐藏文档配置与排版样式 (\(lineCount)行)")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.secondary)
                    
                    Spacer()
                    
                    Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.8))
                }
                .padding(.horizontal, 11)
                .padding(.vertical, 7)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .buttonStyle(.plain)
            .zIndex(10)
            
            if isExpanded {
                ScrollView(.horizontal, showsIndicators: false) {
                    Text(content)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(.primary.opacity(0.85))
                        .padding(10)
                }
                .fixedSize(horizontal: false, vertical: true)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .stroke(Color.secondary.opacity(0.2), lineWidth: 0.8)
                )
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
        .padding(.bottom, 4)
    }
}
