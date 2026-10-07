import SwiftUI

extension MarkdownContentView {
    // MARK: - Plan Link Segmentation & Paragraph Flow
    
    @ViewBuilder
    func paragraphView(text: String, size: CGFloat = 15) -> some View {
        let cleanText = Self.replaceHtmlBreaks(in: text).trimmingCharacters(in: .whitespacesAndNewlines)
        let segments = Self.parsePlanSegments(cleanText)
        if segments.count <= 1 && (segments.first?.isPlanButton != true) {
            Self.renderRichText(cleanText, size: size)
                .font(.system(size: size))
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
        } else {
            FlowLayout(horizontalSpacing: 4, verticalSpacing: 6) {
                ForEach(segments) { segment in
                    switch segment {
                    case .text(_, let content):
                        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
                        if !trimmed.isEmpty {
                            Self.renderRichText(trimmed, size: size)
                                .font(.system(size: size))
                                .lineSpacing(3)
                        }
                    case .planButton(_, let title, let uri):
                        PlanButtonView(title: title, uri: uri)
                    }
                }
            }
        }
    }
    
    static let planRegex = try? NSRegularExpression(
        pattern: #"(?:(?<!\!)\[([^\]]+)\]\(([^)]+)\)|(?<![a-zA-Z0-9_\-\.\/])((?:implementation_plan|walkthrough)\.md)(?![a-zA-Z0-9_\-\.\/]))"#,
        options: [.caseInsensitive]
    )
    
    public static func parsePlanSegments(_ rawText: String) -> [PlanSegment] {
        guard rawText.contains("implementation_plan") || rawText.contains("walkthrough") else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        guard let regex = planRegex else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        let nsText = rawText as NSString
        let allMatches = regex.matches(in: rawText, range: NSRange(location: 0, length: nsText.length))
        
        let codeSpans = rawText.contains("`") ? codeSpanIndexSet(in: rawText) : IndexSet()
        
        let matches = allMatches.filter { m in
            if !codeSpans.isEmpty {
                let r = m.range
                if !codeSpans.intersection(IndexSet(integersIn: r.location ..< (r.location + r.length))).isEmpty {
                    return false
                }
            }
            if m.range(at: 1).location != NSNotFound && m.range(at: 2).location != NSNotFound {
                let g1 = nsText.substring(with: m.range(at: 1)).lowercased()
                let g2 = nsText.substring(with: m.range(at: 2)).lowercased()
                return g1.contains("implementation_plan") || g1.contains("walkthrough") ||
                       g2.contains("implementation_plan") || g2.contains("walkthrough")
            } else if m.range(at: 3).location != NSNotFound {
                return true
            }
            return false
        }
        
        guard !matches.isEmpty else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        var segments: [PlanSegment] = []
        var lastEnd = 0
        var segIdx = 0
        
        for (idx, match) in matches.enumerated() {
            let matchRange = match.range
            var prefix = ""
            if matchRange.location > lastEnd {
                prefix = nsText.substring(with: NSRange(location: lastEnd, length: matchRange.location - lastEnd))
            }
            
            let nextIndex = matchRange.location + matchRange.length
            let suffixLength = (idx + 1 < matches.count) ? (matches[idx + 1].range.location - nextIndex) : (nsText.length - nextIndex)
            let suffixPreview = nsText.substring(with: NSRange(location: nextIndex, length: suffixLength))
            
            // If the plan link was wrapped in markdown delimiters (e.g. **[implementation_plan.md](...)**),
            // strip them from prefix and suffix so no dangling asterisks/ticks surround the button.
            var strippedLength = 0
            if prefix.hasSuffix("**") && suffixPreview.hasPrefix("**") {
                prefix = String(prefix.dropLast(2))
                strippedLength = 2
            } else if prefix.hasSuffix("*") && suffixPreview.hasPrefix("*") {
                prefix = String(prefix.dropLast(1))
                strippedLength = 1
            } else if prefix.hasSuffix("__") && suffixPreview.hasPrefix("__") {
                prefix = String(prefix.dropLast(2))
                strippedLength = 2
            } else if prefix.hasSuffix("`") && suffixPreview.hasPrefix("`") {
                prefix = String(prefix.dropLast(1))
                strippedLength = 1
            }
            
            let trimmedPrefix = prefix.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmedPrefix.isEmpty {
                segments.append(.text(id: "seg-\(segIdx)", content: prefix))
                segIdx += 1
            }
            
            var title = "implementation_plan.md"
            var uri = "implementation_plan.md"
            
            if match.range(at: 1).location != NSNotFound && match.range(at: 2).location != NSNotFound {
                title = nsText.substring(with: match.range(at: 1)).trimmingCharacters(in: .whitespaces)
                uri = nsText.substring(with: match.range(at: 2)).trimmingCharacters(in: .whitespaces)
            } else if match.range(at: 3).location != NSNotFound {
                let fn = nsText.substring(with: match.range(at: 3)).trimmingCharacters(in: .whitespaces)
                title = fn
                uri = fn
            }
            
            segments.append(.planButton(id: "seg-\(segIdx)", title: title, uri: uri))
            segIdx += 1
            
            lastEnd = matchRange.location + matchRange.length + strippedLength
        }
        
        if lastEnd < nsText.length {
            let suffix = nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
            let punctChars: Set<Character> = ["。", ".", "，", ",", "！", "!", "？", "?", "；", ";", "：", ":"]
            let trimmedSuffix = suffix.trimmingCharacters(in: .whitespacesAndNewlines)
            // If suffix only consists of trailing punctuation and whitespace, drop it to avoid dangling orphan punctuation.
            // If it contains meaningful text, preserve it as a unified text segment.
            let hasMeaningfulContent = trimmedSuffix.contains { !punctChars.contains($0) }
            if hasMeaningfulContent {
                segments.append(.text(id: "seg-\(segIdx)", content: suffix))
                segIdx += 1
            }
        }
        
        return segments
    }
}
