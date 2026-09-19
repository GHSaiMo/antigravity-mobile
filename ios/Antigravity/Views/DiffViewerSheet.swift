import SwiftUI

public struct DiffViewerSheet: View {
    public let file: RevertPreviewFile
    @Environment(\.dismiss) private var dismiss
    
    public init(file: RevertPreviewFile) {
        self.file = file
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // File summary bar
                HStack(spacing: 10) {
                    actionBadge(file.actionType)
                    
                    Text(file.fileUri)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    
                    Spacer()
                    
                    if file.additions > 0 {
                        Text("+\(file.additions)")
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundColor(.green)
                    }
                    if file.deletions > 0 {
                        Text("-\(file.deletions)")
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundColor(.red)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .background(Color(uiColor: .secondarySystemBackground))
                
                Divider()
                
                // Diff content lines
                if file.diffLines.isEmpty {
                    VStack(spacing: 8) {
                        Spacer()
                        Image(systemName: "doc.text.magnifyingglass")
                            .font(.system(size: 36))
                            .foregroundColor(.secondary)
                        Text("无行级别差异预览")
                            .font(.subheadline)
                            .foregroundColor(.secondary)
                        Spacer()
                    }
                } else {
                    ScrollView([.horizontal, .vertical], showsIndicators: true) {
                        VStack(alignment: .leading, spacing: 0) {
                            ForEach(Array(file.diffLines.enumerated()), id: \.offset) { index, line in
                                diffLineRow(index: index + 1, line: line)
                            }
                        }
                        .padding(.vertical, 4)
                        .frame(minWidth: UIScreen.main.bounds.width, alignment: .leading)
                    }
                    .background(Color(uiColor: .systemBackground))
                }
            }
            .navigationTitle(file.fileName)
            .navigationBarTitleDisplayMode(.inline)
            .presentationDragIndicator(.visible)
        }
    }
    
    @ViewBuilder
    private func actionBadge(_ action: String) -> some View {
        let (color, text): (Color, String) = {
            switch action.uppercased() {
            case "CREATE":
                return (.green, "新增")
            case "DELETE":
                return (.red, "删除")
            default:
                return (.blue, "修改")
            }
        }()
        
        Text(text)
            .font(.system(size: 11, weight: .bold))
            .foregroundColor(color)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 4))
    }
    
    @ViewBuilder
    private func diffLineRow(index: Int, line: RevertDiffLine) -> some View {
        let isInsert = line.type.uppercased() == "INSERT"
        let isDelete = line.type.uppercased() == "DELETE"
        
        let bgColor: Color = {
            if isInsert { return Color.green.opacity(0.15) }
            if isDelete { return Color.red.opacity(0.15) }
            return Color.clear
        }()
        
        let textColor: Color = {
            if isInsert { return Color.green }
            if isDelete { return Color.red }
            return Color.primary
        }()
        
        let prefix: String = {
            if isInsert { return "+" }
            if isDelete { return "-" }
            return " "
        }()
        
        HStack(alignment: .top, spacing: 8) {
            Text(prefix)
                .font(.system(size: 12, weight: .bold, design: .monospaced))
                .foregroundColor(textColor)
                .frame(width: 14, alignment: .center)
            
            Text(line.text.isEmpty ? " " : line.text)
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(textColor)
                .fixedSize(horizontal: true, vertical: false)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 2)
        .frame(minWidth: UIScreen.main.bounds.width, maxWidth: .infinity, alignment: .leading)
        .background(bgColor)
    }
}
