import SwiftUI
import UIKit

// MARK: - Markdown → Plain Text (for copy / free selection)

/// Converts Markdown into the text the user actually sees on screen:
/// inline markers (`**bold**`, `*italic*`, `` `code` ``, `[link](url)`) are removed,
/// while line breaks, list bullets / numbers, table rows and code block bodies are kept.
public enum MarkdownPlainText {
    public static func convert(_ content: String) -> String {
        let blocks = MarkdownParser.parse(content)
        var parts: [String] = []

        for block in blocks {
            switch block {
            case .frontmatter(_, let raw, _):
                parts.append(raw.trimmingCharacters(in: .whitespacesAndNewlines))
            case .heading(_, _, let text):
                parts.append(inline(text))
            case .divider:
                parts.append("────────")
            case .codeBlock(_, _, let code):
                parts.append(code.trimmingCharacters(in: .newlines))
            case .table(_, let headers, let rows, _):
                var lines = [headers.map(inline).joined(separator: "\t")]
                lines += rows.map { $0.map(inline).joined(separator: "\t") }
                parts.append(lines.joined(separator: "\n"))
            case .list(_, let items):
                parts.append(items.map { "• " + inline($0) }.joined(separator: "\n"))
            case .orderedList(_, let start, let items):
                parts.append(items.enumerated().map { "\(start + $0.offset). " + inline($0.element) }.joined(separator: "\n"))
            case .paragraph(_, let text):
                parts.append(inline(text))
            case .image(_, let alt, _):
                if !alt.isEmpty { parts.append(alt) }
            }
        }
        return parts.filter { !$0.isEmpty }.joined(separator: "\n\n")
    }

    static func inline(_ text: String) -> String {
        let processed = MathSymbolProcessor.process(text)
        let attr = MarkdownContentView.renderInlineMarkdown(processed)
        return String(attr.characters).trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
