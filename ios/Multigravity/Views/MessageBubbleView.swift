import SwiftUI
import Photos

public struct MessageBubbleView: View {
    @Environment(\.openURL) var openURL
    public let message: ChatMessage
    public let isActiveToolBatch: Bool
    public let onUndo: ((ChatMessage) -> Void)?
    @State var isThinkingExpanded: Bool = false
    
    @State var previewGallery: ImageGalleryData? = nil
    
    public init(message: ChatMessage, isActiveToolBatch: Bool = false, onUndo: ((ChatMessage) -> Void)? = nil) {
        self.message = message
        self.isActiveToolBatch = isActiveToolBatch
        self.onUndo = onUndo
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
    }
}

extension MessageBubbleView {
    func copyWholeText(_ text: String) {
        UIPasteboard.general.string = text
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        CopiedHUD.show()
    }
}
