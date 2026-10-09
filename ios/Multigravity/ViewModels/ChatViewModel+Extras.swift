import Foundation
import Observation
import UIKit

/// 待分享的导出文件（Identifiable，用于 `.sheet(item:)`）。
public struct ExportedMarkdownFile: Identifiable, Sendable {
    public let id = UUID()
    public let url: URL
}

extension ChatViewModel {
    private var isLocalDraftSession: Bool {
        cascadeId.isEmpty || cascadeId.hasPrefix("local_draft_") || cascadeId.hasPrefix("draft_")
    }

    // MARK: - 导出 Markdown

    public func exportMarkdown() {
        guard let url = settings.serverURL else { return }
        guard !isLocalDraftSession else {
            errorMessage = "新会话还没有内容可导出"
            return
        }
        let id = cascadeId
        let title = currentTitle
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let md = try await apiClient.exportConversationMarkdown(cascadeId: id, baseURL: url)
                guard !md.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                    self.errorMessage = "会话内容为空，无法导出"
                    return
                }
                let file = try MarkdownExportWriter.write(markdown: md, title: title)
                self.exportedMarkdownFile = ExportedMarkdownFile(url: file)
            } catch {
                self.errorMessage = "导出失败: \(error.localizedDescription)"
            }
        }
    }
}

/// 把导出的 Markdown 写到临时目录，文件名取会话标题。
public enum MarkdownExportWriter {
    /// 去掉路径与保留字符，限制长度，空标题回退到固定名。
    static func safeFileName(_ title: String) -> String {
        let reserved = CharacterSet(charactersIn: "\\/:*?\"<>|").union(.controlCharacters)
        var cleaned = title.unicodeScalars.map { reserved.contains($0) ? " " : String($0) }.joined()
        cleaned = cleaned.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
        cleaned = String(cleaned.prefix(60)).trimmingCharacters(in: CharacterSet(charactersIn: ". ").union(.whitespaces))
        return cleaned.isEmpty ? "conversation" : cleaned
    }

    static func write(markdown: String, title: String) throws -> URL {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("markdown-exports", isDirectory: true)
        // 清掉旧导出，避免临时目录无限增长
        try? FileManager.default.removeItem(at: dir)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("\(safeFileName(title)).md")
        try markdown.write(to: file, atomically: true, encoding: .utf8)
        return file
    }
}
