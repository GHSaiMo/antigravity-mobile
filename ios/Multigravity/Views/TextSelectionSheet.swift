import SwiftUI
import UIKit

// MARK: - Text Selection Card Sheet (Fine-grained Text Selection)

public struct TextSelectionSheet: View {
    @Environment(\.dismiss) var dismiss
    public let title: String
    public let content: String
    public let onAskBtw: ((String) -> Void)?

    @State private var showSearchBar: Bool = false
    @State private var searchQuery: String = ""
    @State private var currentSelection: String = ""
    
    public init(title: String = String(localized: "选择文本"), content: String, onAskBtw: ((String) -> Void)? = nil) {
        self.title = title
        self.content = content
        self.onAskBtw = onAskBtw
    }

    private var matchCount: Int {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return 0 }
        var count = 0
        var searchRange = content.startIndex..<content.endIndex
        while let r = content.range(of: q, options: [.caseInsensitive], range: searchRange) {
            count += 1
            searchRange = r.upperBound..<content.endIndex
        }
        return count
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                if showSearchBar {
                    HStack(spacing: 8) {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                        TextField("页内查找关键词...", text: $searchQuery)
                            .font(.system(size: 14))
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                        if !searchQuery.isEmpty {
                            Text("\(matchCount) 处")
                                .font(.system(size: 11.5, weight: .semibold, design: .monospaced))
                                .foregroundColor(.secondary)
                            Button {
                                searchQuery = ""
                            } label: {
                                Image(systemName: "xmark.circle.fill")
                                    .foregroundColor(.secondary)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color(uiColor: .secondarySystemBackground))
                    Divider()
                }

                // Hint Banner
                HStack(spacing: 6) {
                    Image(systemName: "hand.tap")
                        .font(.system(size: 12, weight: .medium))
                    Text("长按拖动手柄可精细选择局部文本")
                        .font(.system(size: 12))
                    Spacer()
                    if let onAskBtw {
                        Button {
                            let target = currentSelection.trimmingCharacters(in: .whitespacesAndNewlines)
                            dismiss()
                            onAskBtw(target.isEmpty ? content : target)
                        } label: {
                            Label(currentSelection.isEmpty ? "/btw" : "/btw 选中", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                                .font(.system(size: 11.5, weight: .semibold))
                        }
                        .buttonStyle(.bordered)
                        .controlSize(.mini)
                        .tint(.indigo)
                    }
                }
                .foregroundColor(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .background(Color(uiColor: .tertiarySystemBackground).opacity(0.6))
                
                Divider()
                
                SelectableTextViewRepresentable(text: content, searchQuery: searchQuery, currentSelection: $currentSelection)
                    .background(Color(uiColor: .systemBackground))
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("完成") {
                        dismiss()
                    }
                    .font(.system(size: 16, weight: .medium))
                }
                
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Button {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            showSearchBar.toggle()
                        }
                    } label: {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 14, weight: .medium))
                    }
                    .accessibilityLabel("页内查找")

                    Button {
                        copyAllText()
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "doc.on.doc")
                                .font(.system(size: 13))
                            Text("全部复制")
                                .font(.system(size: 15, weight: .medium))
                        }
                    }
                }
            }
        }
    }
    
    private func copyAllText() {
        let formatted = content.hasSuffix("\n") ? content : "\(content)\n"
        UIPasteboard.general.string = formatted
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        CopiedHUD.show(String(localized: "已复制全部文字"))
    }
}

// MARK: - Selectable UITextView Representable

struct SelectableTextViewRepresentable: UIViewRepresentable {
    let text: String
    var fontSize: CGFloat = 15.5
    var searchQuery: String = ""
    @Binding var currentSelection: String

    final class Coordinator: NSObject, UITextViewDelegate {
        var onSelectionChanged: (String) -> Void
        var lastText: String = ""
        var lastQuery: String = ""
        init(onSelectionChanged: @escaping (String) -> Void) {
            self.onSelectionChanged = onSelectionChanged
        }
        func textViewDidChangeSelection(_ textView: UITextView) {
            if let range = textView.selectedTextRange, let s = textView.text(in: range) {
                onSelectionChanged(s)
            } else {
                onSelectionChanged("")
            }
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator { sel in
            DispatchQueue.main.async {
                currentSelection = sel
            }
        }
    }
    
    func makeUIView(context: Context) -> CustomSelectableTextView {
        let textView = CustomSelectableTextView()
        textView.delegate = context.coordinator
        textView.isEditable = false
        textView.isSelectable = true
        textView.isScrollEnabled = true
        textView.alwaysBounceVertical = true
        textView.backgroundColor = .clear
        textView.font = .systemFont(ofSize: fontSize)
        textView.textColor = .label
        textView.tintColor = .systemIndigo
        textView.textContainerInset = UIEdgeInsets(top: 14, left: 16, bottom: 24, right: 16)
        applyAttributedContent(to: textView, coordinator: context.coordinator)
        return textView
    }
    
    func updateUIView(_ uiView: CustomSelectableTextView, context: Context) {
        if context.coordinator.lastText != text || context.coordinator.lastQuery != searchQuery {
            applyAttributedContent(to: uiView, coordinator: context.coordinator)
        }
    }

    private func applyAttributedContent(to textView: UITextView, coordinator: Coordinator) {
        coordinator.lastText = text
        coordinator.lastQuery = searchQuery
        let attr = NSMutableAttributedString(string: text, attributes: [
            .font: UIFont.systemFont(ofSize: fontSize),
            .foregroundColor: UIColor.label
        ])
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        if !q.isEmpty {
            let ns = text as NSString
            var range = NSRange(location: 0, length: ns.length)
            var firstHit: NSRange? = nil
            while range.location < ns.length {
                let found = ns.range(of: q, options: [.caseInsensitive], range: range)
                if found.location == NSNotFound { break }
                if firstHit == nil { firstHit = found }
                attr.addAttribute(.backgroundColor, value: UIColor.systemYellow.withAlphaComponent(0.38), range: found)
                let nextLoc = found.location + max(1, found.length)
                range = NSRange(location: nextLoc, length: ns.length - nextLoc)
            }
            textView.attributedText = attr
            if let hit = firstHit {
                textView.scrollRangeToVisible(hit)
            }
        } else {
            textView.attributedText = attr
        }
    }
}

// MARK: - Custom UITextView Intercepting Selection Copy with Linebreak

final class CustomSelectableTextView: UITextView {
    override func copy(_ sender: Any?) {
        guard let selectedRange = selectedTextRange,
              let selectedText = text(in: selectedRange),
              !selectedText.isEmpty else {
            super.copy(sender)
            return
        }
        
        let formatted = selectedText.hasSuffix("\n") ? selectedText : "\(selectedText)\n"
        UIPasteboard.general.string = formatted
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        CopiedHUD.show(String(localized: "已复制选中文本"))
    }
}
