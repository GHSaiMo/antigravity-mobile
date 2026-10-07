import SwiftUI
import UIKit

// MARK: - Text Selection Card Sheet (Fine-grained Text Selection)

public struct TextSelectionSheet: View {
    @Environment(\.dismiss) var dismiss
    public let title: String
    public let content: String
    
    public init(title: String = "选择文本", content: String) {
        self.title = title
        self.content = content
    }
    
    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Hint Banner
                HStack(spacing: 6) {
                    Image(systemName: "hand.tap")
                        .font(.system(size: 12, weight: .medium))
                    Text("长按拖动手柄可精细选择局部文本")
                        .font(.system(size: 12))
                }
                .foregroundColor(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .background(Color(uiColor: .tertiarySystemBackground).opacity(0.6))
                
                Divider()
                
                SelectableTextViewRepresentable(text: content)
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
                
                ToolbarItem(placement: .topBarTrailing) {
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
        CopiedHUD.show("已复制全部文字")
    }
}

// MARK: - Selectable UITextView Representable

struct SelectableTextViewRepresentable: UIViewRepresentable {
    let text: String
    var fontSize: CGFloat = 15.5
    
    func makeUIView(context: Context) -> CustomSelectableTextView {
        let textView = CustomSelectableTextView()
        textView.isEditable = false
        textView.isSelectable = true
        textView.isScrollEnabled = true
        textView.alwaysBounceVertical = true
        textView.backgroundColor = .clear
        textView.font = .systemFont(ofSize: fontSize)
        textView.textColor = .label
        textView.tintColor = .systemIndigo
        textView.textContainerInset = UIEdgeInsets(top: 14, left: 16, bottom: 24, right: 16)
        textView.text = text
        return textView
    }
    
    func updateUIView(_ uiView: CustomSelectableTextView, context: Context) {
        if uiView.text != text {
            uiView.text = text
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
        CopiedHUD.show("已复制选中文本")
    }
}
