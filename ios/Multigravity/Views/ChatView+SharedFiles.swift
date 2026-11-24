import SwiftUI

/// Files shared into the app from other apps and routed to this conversation (see `ShareInbox`).
extension ChatView {
    /// Drops delivered files into the input bar as attachments. Never sends anything by itself.
    @MainActor
    func consumeSharedFiles() {
        var keys: [String] = []
        let vmKey = viewModel.draftKey
        if !vmKey.isEmpty { keys.append(vmKey) }
        if let initId = initialConversation?.id, !initId.isEmpty, !keys.contains(initId) {
            keys.append(initId)
        }
        var shared: [SharedFile] = []
        for key in keys {
            shared.append(contentsOf: ShareInbox.shared.takeDelivery(for: key))
        }
        guard !shared.isEmpty else { return }
        
        let docs = shared.filter { !$0.isImage }
        if !docs.isEmpty {
            // Hydrate any previously persisted draft files before appending so new shares don't overwrite them.
            viewModel.restoreDraftFiles()
            viewModel.addFiles(from: docs.map(\.url))
        }
        
        var compressed: [Data] = []
        for file in shared where file.isImage {
            if let image = UIImage(contentsOfFile: file.url.path), let data = compressAndResizeImage(image) {
                compressed.append(data)
            }
        }
        if !compressed.isEmpty { viewModel.appendDraftImages(compressed) }
        
        // addFiles copies synchronously, so the staged copies are no longer needed.
        for file in shared {
            try? FileManager.default.removeItem(at: file.url.deletingLastPathComponent())
        }
    }
}
