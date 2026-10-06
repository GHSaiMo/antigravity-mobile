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

// MARK: - Selectable Text Sheet

private struct SelectableTextView: UIViewRepresentable {
    let text: String

    func makeUIView(context: Context) -> UITextView {
        let tv = UITextView()
        tv.isEditable = false
        tv.isSelectable = true
        tv.font = .systemFont(ofSize: 16)
        tv.backgroundColor = .clear
        tv.textContainerInset = UIEdgeInsets(top: 12, left: 12, bottom: 12, right: 12)
        tv.text = text
        return tv
    }

    func updateUIView(_ uiView: UITextView, context: Context) {
        if uiView.text != text { uiView.text = text }
    }
}

/// Sheet that shows markdown-stripped text in a native UITextView so the user can
/// long-press and freely choose a range; the system "Copy" then yields plain text.
struct SelectableTextSheet: View {
    let text: String
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            SelectableTextView(text: text)
                .navigationTitle("选择文字")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("复制全部") {
                            UIPasteboard.general.string = text
                            UINotificationFeedbackGenerator().notificationOccurred(.success)
                            dismiss()
                        }
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("完成") { dismiss() }
                    }
                }
        }
        .presentationDetents([.medium, .large])
    }
}

struct SelectableTextItem: Identifiable {
    let id = UUID()
    let text: String
}
