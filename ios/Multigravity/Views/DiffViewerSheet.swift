import SwiftUI

public struct DiffSpan: Identifiable, Sendable {
    public let id = UUID()
    public let text: String
    public let isChanged: Bool
    
    public init(text: String, isChanged: Bool) {
        self.text = text
        self.isChanged = isChanged
    }
}

public struct ProcessedDiffLine: Identifiable, Sendable {
    public let id: Int
    public let line: RevertDiffLine
    public let spans: [DiffSpan]
    
    public init(id: Int, line: RevertDiffLine, spans: [DiffSpan]) {
        self.id = id
        self.line = line
        self.spans = spans
    }
}

public struct DiffViewerSheet: View {
    public let file: RevertPreviewFile
    @Environment(\.dismiss) private var dismiss
    
    public init(file: RevertPreviewFile) {
        self.file = file
    }
    
    private var processedLines: [ProcessedDiffLine] {
        Self.processDiffLines(file.diffLines)
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
                            ForEach(processedLines) { item in
                                diffLineRow(item: item)
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
    private func diffLineRow(item: ProcessedDiffLine) -> some View {
        let line = item.line
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
            
            Text(buildAttributedLine(spans: item.spans, isInsert: isInsert, isDelete: isDelete))
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(textColor)
                .fixedSize(horizontal: true, vertical: false)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 2)
        .frame(minWidth: UIScreen.main.bounds.width, maxWidth: .infinity, alignment: .leading)
        .background(bgColor)
    }
    
    private func buildAttributedLine(spans: [DiffSpan], isInsert: Bool, isDelete: Bool) -> AttributedString {
        var result = AttributedString("")
        for span in spans {
            var part = AttributedString(span.text)
            if span.isChanged {
                if isInsert {
                    part.backgroundColor = Color.green.opacity(0.35)
                } else if isDelete {
                    part.backgroundColor = Color.red.opacity(0.35)
                }
            }
            result.append(part)
        }
        return result
    }
    
    public static func processDiffLines(_ lines: [RevertDiffLine]) -> [ProcessedDiffLine] {
        var result: [ProcessedDiffLine] = []
        var i = 0
        let n = lines.count
        while i < n {
            let type = lines[i].type.uppercased()
            if type == "DELETE" {
                var delEnd = i
                while delEnd < n && lines[delEnd].type.uppercased() == "DELETE" {
                    delEnd += 1
                }
                var insEnd = delEnd
                while insEnd < n && lines[insEnd].type.uppercased() == "INSERT" {
                    insEnd += 1
                }
                
                let delCount = delEnd - i
                let insCount = insEnd - delEnd
                
                if insCount > 0 {
                    let pairCount = min(delCount, insCount)
                    for k in 0..<pairCount {
                        let (delSpans, _) = computeTokenDiff(oldText: lines[i + k].text, newText: lines[delEnd + k].text)
                        result.append(ProcessedDiffLine(id: i + k, line: lines[i + k], spans: delSpans))
                    }
                    for k in pairCount..<delCount {
                        let text = lines[i + k].text.isEmpty ? " " : lines[i + k].text
                        result.append(ProcessedDiffLine(id: i + k, line: lines[i + k], spans: [DiffSpan(text: text, isChanged: false)]))
                    }
                    for k in 0..<pairCount {
                        let (_, insSpans) = computeTokenDiff(oldText: lines[i + k].text, newText: lines[delEnd + k].text)
                        result.append(ProcessedDiffLine(id: delEnd + k, line: lines[delEnd + k], spans: insSpans))
                    }
                    for k in pairCount..<insCount {
                        let text = lines[delEnd + k].text.isEmpty ? " " : lines[delEnd + k].text
                        result.append(ProcessedDiffLine(id: delEnd + k, line: lines[delEnd + k], spans: [DiffSpan(text: text, isChanged: false)]))
                    }
                    i = insEnd
                    continue
                }
            }
            
            let text = lines[i].text.isEmpty ? " " : lines[i].text
            result.append(ProcessedDiffLine(id: i, line: lines[i], spans: [DiffSpan(text: text, isChanged: false)]))
            i += 1
        }
        return result
    }
    
    public static func tokenize(_ text: String) -> [String] {
        var tokens: [String] = []
        var current = ""
        var currentKind = 0 // 1: word, 2: space, 3: other
        
        for char in text {
            let kind: Int
            if char.isWhitespace {
                kind = 2
            } else if char.isLetter || char.isNumber || char == "_" {
                kind = 1
            } else {
                kind = 3
            }
            
            if kind == currentKind && kind != 3 {
                current.append(char)
            } else {
                if !current.isEmpty {
                    tokens.append(current)
                }
                current = String(char)
                currentKind = kind
            }
        }
        if !current.isEmpty {
            tokens.append(current)
        }
        return tokens
    }
    
    public static func computeTokenDiff(oldText: String, newText: String) -> (oldSpans: [DiffSpan], newSpans: [DiffSpan]) {
        let tokensA = tokenize(oldText.isEmpty ? " " : oldText)
        let tokensB = tokenize(newText.isEmpty ? " " : newText)
        
        let m = tokensA.count
        let n = tokensB.count
        if m == 0 || n == 0 {
            let spansA = tokensA.map { DiffSpan(text: $0, isChanged: true) }
            let spansB = tokensB.map { DiffSpan(text: $0, isChanged: true) }
            return (spansA, spansB)
        }
        
        var dp = Array(repeating: Array(repeating: 0, count: n + 1), count: m + 1)
        for i in 1...m {
            for j in 1...n {
                if tokensA[i - 1] == tokensB[j - 1] {
                    dp[i][j] = dp[i - 1][j - 1] + 1
                } else {
                    dp[i][j] = max(dp[i - 1][j], dp[i][j - 1])
                }
            }
        }
        
        var commonA = Set<Int>()
        var commonB = Set<Int>()
        var i = m, j = n
        while i > 0 && j > 0 {
            if tokensA[i - 1] == tokensB[j - 1] {
                commonA.insert(i - 1)
                commonB.insert(j - 1)
                i -= 1
                j -= 1
            } else if dp[i - 1][j] >= dp[i][j - 1] {
                i -= 1
            } else {
                j -= 1
            }
        }
        
        let spansA = tokensA.enumerated().map { idx, tok in
            DiffSpan(text: tok, isChanged: !commonA.contains(idx))
        }
        let spansB = tokensB.enumerated().map { idx, tok in
            DiffSpan(text: tok, isChanged: !commonB.contains(idx))
        }
        return (spansA, spansB)
    }
}
