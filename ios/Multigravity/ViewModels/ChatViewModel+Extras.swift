import Foundation
import Observation
import UIKit

/// 「本会话改动」浮窗状态。
public struct ChangesSheetState: Sendable {
    public var isLoading: Bool = true
    public var data: CascadeChangesResponse? = nil
    public var error: String? = nil
}

/// Git 提交浮窗状态。
public struct GitSheetState: Sendable {
    public var isLoading: Bool = true
    public var status: GitStatusResponse? = nil
    public var error: String? = nil
    public var isWorking: Bool = false
    public var result: GitCommitResponse? = nil
    public var actionError: String? = nil
}

/// 待分享的导出文件（Identifiable，用于 `.sheet(item:)`）。
public struct ExportedMarkdownFile: Identifiable, Sendable {
    public let id = UUID()
    public let url: URL
}

extension ChatViewModel {
    private var isLocalDraftSession: Bool {
        cascadeId.isEmpty || cascadeId.hasPrefix("local_draft_") || cascadeId.hasPrefix("draft_")
    }

    // MARK: - 本会话改动

    public func openChangesSheet() {
        guard let url = settings.serverURL else { return }
        guard !isLocalDraftSession else {
            errorMessage = "新会话还没有任何改动"
            return
        }
        changesSheet = ChangesSheetState()
        let id = cascadeId
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let res = try await apiClient.fetchCascadeChanges(cascadeId: id, baseURL: url)
                self.changesSheet = ChangesSheetState(isLoading: false, data: res)
            } catch {
                self.changesSheet = ChangesSheetState(isLoading: false, error: error.localizedDescription)
            }
        }
    }

    public func closeChangesSheet() {
        changesSheet = nil
    }

    // MARK: - Git 直接提交

    public func openGitSheet() {
        guard settings.serverURL != nil else { return }
        guard !isLocalDraftSession else {
            errorMessage = "新会话还没有关联的工作区"
            return
        }
        gitSheet = GitSheetState()
        refreshGitStatus()
    }

    public func refreshGitStatus() {
        guard let url = settings.serverURL, var sheet = gitSheet else { return }
        sheet.isLoading = true
        sheet.error = nil
        gitSheet = sheet
        let id = cascadeId
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let st = try await apiClient.fetchGitStatus(cascadeId: id, baseURL: url)
                guard var s = self.gitSheet else { return }
                s.isLoading = false
                s.status = st
                s.error = nil
                self.gitSheet = s
            } catch {
                guard var s = self.gitSheet else { return }
                s.isLoading = false
                s.error = error.localizedDescription
                self.gitSheet = s
            }
        }
    }

    /// 直接调用网关提交（可选推送）。paths 为空表示提交全部改动。
    public func commitGit(message: String, paths: [String], push: Bool) {
        guard let url = settings.serverURL, var sheet = gitSheet, !sheet.isWorking else { return }
        sheet.isWorking = true
        sheet.actionError = nil
        gitSheet = sheet
        let id = cascadeId
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let res = try await apiClient.gitCommit(cascadeId: id, message: message, paths: paths, push: push, baseURL: url)
                guard var s = self.gitSheet else { return }
                s.isWorking = false
                s.result = res
                self.gitSheet = s
            } catch {
                guard var s = self.gitSheet else { return }
                s.isWorking = false
                s.actionError = error.localizedDescription
                self.gitSheet = s
            }
        }
    }

    /// 提交已成功但推送失败时，单独重试推送；工作区干净但有未推送提交时也走这里。
    public func retryGitPush() {
        guard let url = settings.serverURL, var sheet = gitSheet, !sheet.isWorking else { return }
        sheet.isWorking = true
        sheet.actionError = nil
        gitSheet = sheet
        let id = cascadeId
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                _ = try await apiClient.gitPush(cascadeId: id, baseURL: url)
                guard var s = self.gitSheet else { return }
                s.isWorking = false
                if var r = s.result {
                    r.pushed = true
                    r.pushError = nil
                    s.result = r
                } else {
                    // 干净工作区直接推送：刷新状态，ahead 会归零
                    s.result = nil
                }
                self.gitSheet = s
                if s.result == nil { self.refreshGitStatus() }
            } catch {
                guard var s = self.gitSheet else { return }
                s.isWorking = false
                s.actionError = error.localizedDescription
                self.gitSheet = s
            }
        }
    }

    public func closeGitSheet() {
        gitSheet = nil
    }

    // MARK: - 斜杠命令

    /// 首次键入 "/" 时懒加载命令列表；已加载或正在加载时什么都不做。失败后下次键入 "/" 会重试。
    public func ensureSlashCommands() {
        guard slashCommands.isEmpty, !isLoadingSlashCommands, let url = settings.serverURL else { return }
        isLoadingSlashCommands = true
        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.isLoadingSlashCommands = false }
            if let list = try? await self.apiClient.fetchSlashCommands(baseURL: url) {
                self.slashCommands = list
            }
        }
    }

    /// 选中命令：输入框里的 "/xx" 清掉，命令变成输入框上方的标签。
    public func selectSlashCommand(_ command: SlashCommandOption) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        inputText = ""
        selectedSlashCommand = command
        focusInputTrigger += 1
    }

    public func clearSlashCommand() {
        selectedSlashCommand = nil
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
