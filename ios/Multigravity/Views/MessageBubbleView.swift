import SwiftUI
import Photos

public struct MessageBubbleView: View {
    @Environment(\.openURL) var openURL
    public let message: ChatMessage
    public let isActiveToolBatch: Bool
    public let onUndo: ((ChatMessage) -> Void)?
    let shareContext: ShareCardContext?
    @State var showShareCard: Bool = false
    @State var isThinkingExpanded: Bool = false
    
    @State var previewGallery: ImageGalleryData? = nil
    @State var showTextSelectionSheet: Bool = false
    
    init(message: ChatMessage, isActiveToolBatch: Bool = false, onUndo: ((ChatMessage) -> Void)? = nil, shareContext: ShareCardContext? = nil) {
        self.message = message
        self.isActiveToolBatch = isActiveToolBatch
        self.onUndo = onUndo
        self.shareContext = shareContext
    }
    
    public var body: some View {
        Group {
            switch message.sender {
            case .user:
                HStack(alignment: .bottom, spacing: 8) {
                    Spacer(minLength: 40)
                    userBubble
                }
            case .agent:
                HStack(alignment: .bottom, spacing: 8) {
                    agentBubble
                    Spacer(minLength: 20)
                }
            case .toolBatch(let count, let tools):
                if isActiveToolBatch {
                    activeToolBatchCard(count: count, tools: tools)
                } else {
                    ToolBatchAccordionView(count: count, tools: tools)
                }
            case .error:
                errorCard
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 4)
        .fullScreenCover(item: $previewGallery) { gallery in
            ImageViewerSheet(gallery: gallery)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
        .sheet(isPresented: $showShareCard) {
            MessageShareCardSheet(
                message: message,
                context: shareContext ?? ShareCardContext(sessionTitle: "", modelName: nil, previousQuestion: nil),
                extraImages: shareCardImages
            )
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: $showTextSelectionSheet) {
            TextSelectionSheet(
                title: "选择文本",
                content: agentPlainText
            )
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
        }
    }
}

extension MessageBubbleView {
    var shareCardImages: [IdentifiableImage] {
        if case .user = message.sender { return userAttachmentItems }
        return fallbackAgentImageURLs.compactMap { URL(string: $0) }.map { IdentifiableImage(url: $0) }
    }
    
    @ViewBuilder
    var shareLongImageButton: some View {
        Button {
            showShareCard = true
        } label: {
            Label("分享为长图", systemImage: "photo.badge.plus")
        }
    }
    
    var agentPlainText: String {
        let converted = MarkdownPlainText.convert(message.content)
        return converted.isEmpty ? message.content : converted
    }
    
    func copyWholeText(_ text: String) {
        let formatted = text.hasSuffix("\n") ? text : "\(text)\n"
        UIPasteboard.general.string = formatted
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        CopiedHUD.show()
    }
}
