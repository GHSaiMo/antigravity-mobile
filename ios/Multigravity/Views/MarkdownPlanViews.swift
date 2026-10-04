import SwiftUI

// MARK: - Plan Segment Models & Flow Layout

public enum PlanSegment: Identifiable {
    case text(id: String, content: String)
    case planButton(id: String, title: String, uri: String)
    
    public var id: String {
        switch self {
        case .text(let id, _): return id
        case .planButton(let id, _, _): return id
        }
    }
    
    public var isPlanButton: Bool {
        if case .planButton = self { return true }
        return false
    }
}

public struct PlanButtonStyle: ButtonStyle {
    public init() {}
    
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1.0)
            .opacity(configuration.isPressed ? 0.75 : 1.0)
            .animation(.easeInOut(duration: 0.12), value: configuration.isPressed)
    }
}

public struct PlanButtonView: View {
    @Environment(\.openURL) private var openURL
    
    public let title: String
    public let uri: String
    
    public init(title: String, uri: String) {
        self.title = title
        self.uri = uri
    }
    
    public var body: some View {
        Button(action: {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            let target = uri.trimmingCharacters(in: .whitespacesAndNewlines)
            if let url = URL(string: target) ?? URL(string: target.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? target) {
                openURL(url)
            }
        }) {
            HStack(spacing: 5) {
                if let iconName = FileIconResolver.resolveIcon(for: uri) ?? FileIconResolver.resolveIcon(for: title) {
                    Image(iconName)
                        .resizable()
                        .scaledToFit()
                        .frame(width: 14, height: 14)
                } else {
                    Image(systemName: "doc.text.magnifyingglass")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.blue)
                }
                
                Text(title)
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                    .foregroundColor(.blue)
                    .lineLimit(1)
                
                Image(systemName: "arrow.up.forward.app")
                    .font(.system(size: 10.5, weight: .medium))
                    .foregroundColor(.blue.opacity(0.65))
            }
            .padding(.horizontal, 9)
            .padding(.vertical, 4.5)
            .background(Color.blue.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .stroke(Color.blue.opacity(0.35), lineWidth: 1)
            )
        }
        .buttonStyle(PlanButtonStyle())
    }
}

public struct FlowLayout: Layout {
    public var horizontalSpacing: CGFloat
    public var verticalSpacing: CGFloat
    
    public init(horizontalSpacing: CGFloat = 4, verticalSpacing: CGFloat = 6) {
        self.horizontalSpacing = horizontalSpacing
        self.verticalSpacing = verticalSpacing
    }
    
    public struct LayoutRow {
        public var elements: [(subview: LayoutSubview, size: CGSize, origin: CGPoint)]
        public var frame: CGRect
    }
    
    public struct LayoutResult {
        public var size: CGSize
        public var rows: [LayoutRow]
    }
    
    private func computeLayout(proposal: ProposedViewSize, subviews: Subviews) -> LayoutResult {
        let maxAvailableWidth = proposal.width ?? .infinity
        var rows: [LayoutRow] = []
        var currentRowElements: [(subview: LayoutSubview, size: CGSize, origin: CGPoint)] = []
        
        var currentX: CGFloat = 0
        var currentY: CGFloat = 0
        var currentLineHeight: CGFloat = 0
        var maxWidth: CGFloat = 0
        
        for subview in subviews {
            let size = subview.sizeThatFits(ProposedViewSize(width: maxAvailableWidth, height: nil))
            
            if currentX + size.width > maxAvailableWidth + 0.5 && currentX > 0 {
                let rowFrame = CGRect(
                    x: 0,
                    y: currentY,
                    width: max(0, currentX - horizontalSpacing),
                    height: currentLineHeight
                )
                rows.append(LayoutRow(elements: currentRowElements, frame: rowFrame))
                
                currentRowElements = []
                currentX = 0
                currentY += currentLineHeight + verticalSpacing
                currentLineHeight = 0
            }
            
            currentRowElements.append((subview: subview, size: size, origin: CGPoint(x: currentX, y: 0)))
            currentLineHeight = max(currentLineHeight, size.height)
            currentX += size.width + horizontalSpacing
            maxWidth = max(maxWidth, currentX - horizontalSpacing)
        }
        
        if !currentRowElements.isEmpty {
            let rowFrame = CGRect(
                x: 0,
                y: currentY,
                width: max(0, currentX - horizontalSpacing),
                height: currentLineHeight
            )
            rows.append(LayoutRow(elements: currentRowElements, frame: rowFrame))
            currentY += currentLineHeight
        }
        
        let totalWidth = min(maxWidth, maxAvailableWidth)
        let totalHeight = currentY
        
        return LayoutResult(size: CGSize(width: totalWidth, height: totalHeight), rows: rows)
    }
    
    public func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let result = computeLayout(proposal: proposal, subviews: subviews)
        return result.size
    }
    
    public func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let result = computeLayout(
            proposal: ProposedViewSize(width: bounds.width, height: bounds.height),
            subviews: subviews
        )
        
        for row in result.rows {
            let rowY = bounds.minY + row.frame.origin.y
            let rowHeight = row.frame.height
            for item in row.elements {
                let x = bounds.minX + item.origin.x
                let y = rowY + (rowHeight - item.size.height) / 2
                item.subview.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(item.size))
            }
        }
    }
}
