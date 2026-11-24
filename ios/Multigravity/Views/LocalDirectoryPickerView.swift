import SwiftUI
import Combine
import UIKit

// MARK: - Shared Local Directory Picker ViewModel

@MainActor
final class LocalDirectoryPickerViewModel: ObservableObject {
    @Published var currentPath: String = ""
    @Published var currentDisplay: String = ""
    @Published var parentPath: String = ""
    @Published var canGoUp: Bool = false
    @Published var recommendedDirs: [LocalDirQuickItem] = []
    @Published var entries: [LocalDirEntry] = []
    @Published var isLoading: Bool = false
    @Published var isCreatingDir: Bool = false
    @Published var errorMessage: String? = nil

    func load(path: String? = nil) async {
        guard let url = AppSettings.shared.gatewayURL else {
            errorMessage = String(localized: "未配置网关地址")
            return
        }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let resp = try await APIClient.shared.fetchLocalDirs(path: path, baseURL: url)
            if resp.ok == false {
                errorMessage = resp.error ?? String(localized: "读取本机目录失败")
                return
            }
            currentPath = resp.currentPath
            currentDisplay = (resp.currentDisplay?.isEmpty == false ? resp.currentDisplay! : resp.currentPath)
            parentPath = resp.parentPath ?? ""
            canGoUp = resp.canGoUp
            recommendedDirs = resp.recommendedDirs
            entries = resp.entries
        } catch {
            errorMessage = String(localized: "读取本机目录失败: \(error.localizedDescription)")
        }
    }

    func createDirectory(input: String) async -> String? {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        guard let url = AppSettings.shared.gatewayURL else {
            errorMessage = String(localized: "未配置网关地址")
            return nil
        }
        isCreatingDir = true
        errorMessage = nil
        defer { isCreatingDir = false }
        do {
            let isFullPath = trimmed.hasPrefix("/") || trimmed.hasPrefix("~") || trimmed.contains(":\\") || trimmed.contains(":/")
            let resp = try await APIClient.shared.createLocalDir(
                parentPath: isFullPath ? nil : currentPath,
                name: isFullPath ? nil : trimmed,
                path: isFullPath ? trimmed : nil,
                baseURL: url
            )
            if resp.ok == false {
                errorMessage = resp.error ?? String(localized: "创建本地目录失败")
                return nil
            }
            if let created = resp.path, !created.isEmpty {
                await load(path: created)
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                return created
            }
            return nil
        } catch {
            errorMessage = String(localized: "创建本地目录失败: \(error.localizedDescription)")
            return nil
        }
    }
}

// MARK: - Reusable Local Directory Browser Section

struct LocalDirectoryBrowserSection: View {
    @ObservedObject var viewModel: LocalDirectoryPickerViewModel
    let sectionTitle: String
    let subtitle: String
    var onSelectDirPathChanged: ((String) -> Void)? = nil

    @State private var showNewFolderAlert: Bool = false
    @State private var newFolderName: String = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .center) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(sectionTitle)
                        .font(.system(size: 13.5, weight: .semibold))
                        .foregroundColor(.primary)
                    Text(subtitle)
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                }
                Spacer()
                Button {
                    newFolderName = ""
                    showNewFolderAlert = true
                } label: {
                    HStack(spacing: 4) {
                        if viewModel.isCreatingDir {
                            ProgressView()
                                .scaleEffect(0.7)
                        } else {
                            Image(systemName: "folder.badge.plus")
                                .font(.system(size: 12.5, weight: .semibold))
                        }
                        Text("新建目录")
                            .font(.system(size: 12.5, weight: .semibold))
                    }
                    .foregroundColor(.accentColor)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(Color.accentColor.opacity(0.12))
                    .clipShape(Capsule())
                }
                .disabled(viewModel.isCreatingDir || viewModel.currentPath.isEmpty)
            }

            // Quick recommended project root directories
            if !viewModel.recommendedDirs.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(viewModel.recommendedDirs) { item in
                            let isSelected = viewModel.currentPath == item.path
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                Task { await viewModel.load(path: item.path) }
                            } label: {
                                HStack(spacing: 5) {
                                    Image(systemName: item.projectCount > 0 ? "folder.fill" : "house.fill")
                                        .font(.system(size: 11))
                                    Text(item.name)
                                        .font(.system(size: 12.5, weight: isSelected ? .semibold : .medium))
                                        .lineLimit(1)
                                    if item.projectCount > 0 {
                                        Text("\(item.projectCount)")
                                            .font(.system(size: 10.5, weight: .semibold))
                                            .padding(.horizontal, 5)
                                            .padding(.vertical, 1)
                                            .background(isSelected ? Color.white.opacity(0.22) : Color.secondary.opacity(0.15))
                                            .clipShape(Capsule())
                                    }
                                }
                                .foregroundColor(isSelected ? .white : .primary)
                                .padding(.horizontal, 11)
                                .padding(.vertical, 7)
                                .background(isSelected ? Color.accentColor : Color(uiColor: .secondarySystemBackground))
                                .clipShape(Capsule())
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.vertical, 1)
                }
            }

            // Current directory breadcrumb & up button
            VStack(spacing: 0) {
                HStack(spacing: 8) {
                    if viewModel.canGoUp {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await viewModel.load(path: viewModel.parentPath) }
                        } label: {
                            HStack(spacing: 3) {
                                Image(systemName: "chevron.left")
                                    .font(.system(size: 11.5, weight: .semibold))
                                Text("上一级")
                                    .font(.system(size: 12, weight: .medium))
                            }
                            .foregroundColor(.accentColor)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 5)
                            .background(Color.accentColor.opacity(0.1))
                            .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                        }
                        .buttonStyle(.plain)
                    }

                    Image(systemName: "folder.fill")
                        .font(.system(size: 13))
                        .foregroundColor(.accentColor)

                    Text(viewModel.currentDisplay.isEmpty ? String(localized: "加载中…") : viewModel.currentDisplay)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                        .truncationMode(.middle)

                    Spacer(minLength: 4)

                    if viewModel.isLoading {
                        ProgressView()
                            .scaleEffect(0.72)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 10)
                .background(Color(uiColor: .tertiarySystemBackground))

                Divider()

                if let err = viewModel.errorMessage {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundColor(.orange)
                            .font(.system(size: 13))
                        Text(err)
                            .font(.system(size: 12))
                            .foregroundColor(.primary)
                            .fixedSize(horizontal: false, vertical: true)
                        Spacer()
                    }
                    .padding(12)
                } else if viewModel.isLoading && viewModel.entries.isEmpty {
                    HStack(spacing: 8) {
                        ProgressView()
                            .scaleEffect(0.85)
                        Text("正在读取本机目录…")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 24)
                } else if viewModel.entries.isEmpty {
                    VStack(spacing: 5) {
                        Image(systemName: "folder")
                            .font(.system(size: 20))
                            .foregroundColor(.secondary.opacity(0.6))
                        Text("当前目录下暂无子文件夹")
                            .font(.system(size: 12.5, weight: .medium))
                            .foregroundColor(.secondary)
                        Text("可直接选用此目录，或点击右上角「新建目录」")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary.opacity(0.85))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 22)
                } else {
                    LazyVStack(spacing: 0) {
                        ForEach(Array(viewModel.entries.enumerated()), id: \.element.id) { idx, entry in
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                Task { await viewModel.load(path: entry.path) }
                            } label: {
                                HStack(spacing: 10) {
                                    Image(systemName: entry.isProject ? "briefcase.fill" : "folder.fill")
                                        .font(.system(size: 14))
                                        .foregroundColor(entry.isProject ? .accentColor : (entry.isGitRepo ? .indigo : .secondary))
                                        .frame(width: 20)

                                    Text(entry.name)
                                        .font(.system(size: 13.5, weight: .medium))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                        .truncationMode(.tail)

                                    Spacer(minLength: 6)

                                    if entry.isProject {
                                        Text("项目")
                                            .font(.system(size: 10, weight: .semibold))
                                            .foregroundColor(.accentColor)
                                            .padding(.horizontal, 6)
                                            .padding(.vertical, 2)
                                            .background(Color.accentColor.opacity(0.12))
                                            .clipShape(Capsule())
                                    } else if entry.isGitRepo {
                                        Text("Git")
                                            .font(.system(size: 10, weight: .semibold))
                                            .foregroundColor(.indigo)
                                            .padding(.horizontal, 6)
                                            .padding(.vertical, 2)
                                            .background(Color.indigo.opacity(0.12))
                                            .clipShape(Capsule())
                                    }

                                    Image(systemName: "chevron.right")
                                        .font(.system(size: 11, weight: .semibold))
                                        .foregroundColor(Color(uiColor: .tertiaryLabel))
                                }
                                .padding(.horizontal, 12)
                                .padding(.vertical, 10)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)

                            if idx < viewModel.entries.count - 1 {
                                Divider()
                                    .padding(.leading, 42)
                            }
                        }
                    }
                }
            }
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 13, style: .continuous)
                    .stroke(Color.primary.opacity(0.07), lineWidth: 1)
            )
        }
        .onChange(of: viewModel.currentPath) { _, newVal in
            onSelectDirPathChanged?(newVal)
        }
        .alert("新建本地目录", isPresented: $showNewFolderAlert) {
            TextField("文件夹名称 或 ~/自定义路径", text: $newFolderName)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            Button("取消", role: .cancel) { newFolderName = "" }
            Button("创建并选中") {
                let raw = newFolderName.trimmingCharacters(in: .whitespacesAndNewlines)
                newFolderName = ""
                guard !raw.isEmpty else { return }
                Task { _ = await viewModel.createDirectory(input: raw) }
            }
        } message: {
            Text("将在 \(viewModel.currentDisplay.isEmpty ? "~" : viewModel.currentDisplay) 下新建目录，或输入 ~/... 完整路径。")
        }
    }
}

// MARK: - GitHub Repo Clone Page

struct CloneGitHubRepoView: View {
    @Environment(\.dismiss) private var dismiss

    let onProjectReady: (ProjectItem) -> Void
    let onStartConversation: (ProjectItem) -> Void

    @StateObject private var dirVM = LocalDirectoryPickerViewModel()
    @State private var repoInput: String = ""
    @State private var customFolderName: String = ""
    @State private var showFolderRename: Bool = false

    private enum Phase {
        case form
        case running(jobId: String, progress: Int, message: String, targetPath: String)
        case completed(project: ProjectItem, repoSlug: String, alreadyExists: Bool)
    }

    @State private var phase: Phase = .form
    @State private var errorMessage: String? = nil
    @State private var conflictAlreadyClonedPath: String? = nil
    @State private var isSubmitting: Bool = false
    @State private var pollTask: Task<Void, Never>? = nil

    private var parsedRepo: (slug: String, repoName: String)? {
        Self.parseGitHubRepo(repoInput)
    }

    private var effectiveFolderName: String {
        let custom = customFolderName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !custom.isEmpty { return custom }
        return parsedRepo?.repoName ?? ""
    }

    private var previewTargetPath: String {
        guard !dirVM.currentPath.isEmpty else { return "" }
        let base = dirVM.currentDisplay.isEmpty ? dirVM.currentPath : dirVM.currentDisplay
        let folder = effectiveFolderName
        guard !folder.isEmpty else { return base }
        let sep = dirVM.currentPath.contains("\\") ? "\\" : "/"
        return base.hasSuffix(sep) ? "\(base)\(folder)" : "\(base)\(sep)\(folder)"
    }

    private var canSubmit: Bool {
        parsedRepo != nil && !dirVM.currentPath.isEmpty && !isSubmitting && !effectiveFolderName.isEmpty
    }

    var body: some View {
        Group {
            switch phase {
            case .form:
                formBody
            case let .running(jobId, progress, message, targetPath):
                runningBody(jobId: jobId, progress: progress, message: message, targetPath: targetPath)
            case let .completed(project, repoSlug, alreadyExists):
                completedBody(project: project, repoSlug: repoSlug, alreadyExists: alreadyExists)
            }
        }
        .navigationTitle("从 GitHub 克隆仓库")
        .navigationBarTitleDisplayMode(.inline)
        .navigationBarBackButtonHidden(isRunningPhase)
        .task {
            if dirVM.currentPath.isEmpty {
                await dirVM.load()
            }
        }
        .onDisappear {
            pollTask?.cancel()
            pollTask = nil
        }
    }

    private var isRunningPhase: Bool {
        if case .running = phase { return true }
        return false
    }

    private var formBody: some View {
        VStack(spacing: 0) {
            ScrollView(.vertical, showsIndicators: true) {
                VStack(alignment: .leading, spacing: 18) {
                    // Section 1: GitHub repository input
                    VStack(alignment: .leading, spacing: 8) {
                        Text("GitHub 仓库地址")
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundColor(.primary)

                        HStack(spacing: 8) {
                            TextField("owner/repo 或 https://github.com/...", text: $repoInput)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .font(.system(size: 14, design: .monospaced))
                                .onChange(of: repoInput) { _, _ in
                                    errorMessage = nil
                                    conflictAlreadyClonedPath = nil
                                }

                            if !repoInput.isEmpty {
                                Button {
                                    repoInput = ""
                                    customFolderName = ""
                                    errorMessage = nil
                                    conflictAlreadyClonedPath = nil
                                } label: {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundColor(.secondary)
                                        .font(.system(size: 15))
                                }
                                .buttonStyle(.plain)
                            } else {
                                Button {
                                    if let pasted = UIPasteboard.general.string {
                                        repoInput = pasted.trimmingCharacters(in: .whitespacesAndNewlines)
                                    }
                                } label: {
                                    Text("粘贴")
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundColor(.accentColor)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 4)
                                        .background(Color.accentColor.opacity(0.12))
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 11)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        .overlay(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .stroke(parsedRepo != nil ? Color.green.opacity(0.45) : Color.primary.opacity(0.08), lineWidth: 1)
                        )

                        HStack(spacing: 8) {
                            if let parsed = parsedRepo {
                                Label(parsed.slug, systemImage: "checkmark.circle.fill")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundColor(.green)
                                Spacer()
                                Button {
                                    withAnimation(.easeInOut(duration: 0.18)) {
                                        showFolderRename.toggle()
                                    }
                                } label: {
                                    Text(showFolderRename ? "收起目录名" : "自定义目录名")
                                        .font(.system(size: 11.5, weight: .medium))
                                        .foregroundColor(.accentColor)
                                }
                                .buttonStyle(.plain)
                            } else if !repoInput.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                                Label("支持 owner/repo、HTTPS 或 git@github.com 地址", systemImage: "info.circle")
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.orange)
                            } else {
                                Text("克隆完成后会自动加入 Antigravity 项目列表")
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.secondary)
                            }
                        }

                        if showFolderRename {
                            HStack(spacing: 8) {
                                Text("文件夹名")
                                    .font(.system(size: 12.5, weight: .medium))
                                    .foregroundColor(.secondary)
                                TextField(parsedRepo?.repoName ?? "repo-name", text: $customFolderName)
                                    .textInputAutocapitalization(.never)
                                    .autocorrectionDisabled()
                                    .font(.system(size: 13.5, design: .monospaced))
                                    .onChange(of: customFolderName) { _, _ in
                                        errorMessage = nil
                                        conflictAlreadyClonedPath = nil
                                    }
                            }
                            .padding(.horizontal, 12)
                            .padding(.vertical, 9)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                        }
                    }

                    // Section 2: Local destination folder picker
                    LocalDirectoryBrowserSection(
                        viewModel: dirVM,
                        sectionTitle: String(localized: "选择本机存放目录"),
                        subtitle: String(localized: "可从已有项目目录选择，浏览子目录，或新建本地目录"),
                        onSelectDirPathChanged: { _ in
                            errorMessage = nil
                            conflictAlreadyClonedPath = nil
                        }
                    )

                    // Section 3: Final path preview & error / conflict card
                    if !previewTargetPath.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("克隆目标路径")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(.secondary)
                            Text(previewTargetPath)
                                .font(.system(size: 12.5, design: .monospaced))
                                .foregroundColor(.primary)
                                .textSelection(.enabled)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(12)
                        .background(Color.accentColor.opacity(0.08))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }

                    if let existingPath = conflictAlreadyClonedPath {
                        VStack(alignment: .leading, spacing: 10) {
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: "folder.badge.questionmark")
                                    .foregroundColor(.orange)
                                    .font(.system(size: 16))
                                VStack(alignment: .leading, spacing: 3) {
                                    Text("该目录已存在同一 GitHub 仓库")
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                    Text(existingPath)
                                        .font(.system(size: 11.5, design: .monospaced))
                                        .foregroundColor(.secondary)
                                }
                            }
                            Button {
                                startClone(importExisting: true)
                            } label: {
                                HStack {
                                    Spacer()
                                    if isSubmitting {
                                        ProgressView()
                                            .tint(.white)
                                            .scaleEffect(0.85)
                                    } else {
                                        Text("直接导入到 Antigravity 项目列表")
                                            .font(.system(size: 13.5, weight: .semibold))
                                    }
                                    Spacer()
                                }
                                .foregroundColor(.white)
                                .padding(.vertical, 10)
                                .background(Color.orange)
                                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            }
                            .disabled(isSubmitting)
                        }
                        .padding(12)
                        .background(Color.orange.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    } else if let err = errorMessage {
                        HStack(alignment: .top, spacing: 8) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundColor(.orange)
                                .font(.system(size: 14))
                            Text(err)
                                .font(.system(size: 12.5))
                                .foregroundColor(.primary)
                                .fixedSize(horizontal: false, vertical: true)
                            Spacer()
                        }
                        .padding(12)
                        .background(Color.orange.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }
                }
                .padding(.horizontal, 16)
                .padding(.top, 14)
                .padding(.bottom, 20)
            }

            Divider()

            VStack(spacing: 0) {
                Button {
                    startClone(importExisting: false)
                } label: {
                    HStack(spacing: 8) {
                        if isSubmitting {
                            ProgressView()
                                .tint(.white)
                                .scaleEffect(0.85)
                        } else {
                            Image(systemName: "arrow.down.circle.fill")
                                .font(.system(size: 16))
                        }
                        Text(isSubmitting ? "正在启动克隆…" : "确认并克隆")
                            .font(.system(size: 15.5, weight: .semibold))
                    }
                    .foregroundColor(.white)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 13)
                    .background(canSubmit ? Color.accentColor : Color.secondary.opacity(0.35))
                    .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
                }
                .disabled(!canSubmit)
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
            }
            .background(Color(uiColor: .systemBackground))
        }
    }

    private func runningBody(jobId: String, progress: Int, message: String, targetPath: String) -> some View {
        VStack(spacing: 22) {
            Spacer()

            ZStack {
                Circle()
                    .fill(Color.accentColor.opacity(0.12))
                    .frame(width: 68, height: 68)
                Image(systemName: "arrow.down.to.line.compact")
                    .font(.system(size: 28, weight: .semibold))
                    .foregroundColor(.accentColor)
            }

            VStack(spacing: 6) {
                Text(parsedRepo?.slug ?? repoInput)
                    .font(.system(size: 17, weight: .bold))
                    .foregroundColor(.primary)
                    .multilineTextAlignment(.center)
                Text(message)
                    .font(.system(size: 13.5))
                    .foregroundColor(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, 24)

            VStack(spacing: 8) {
                ProgressView(value: Double(min(max(progress, 0), 100)), total: 100.0)
                    .progressViewStyle(.linear)
                    .tint(.accentColor)

                HStack {
                    Text(targetPath)
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Spacer()
                    Text("\(min(max(progress, 0), 100))%")
                        .font(.system(size: 12, weight: .semibold, design: .monospaced))
                        .foregroundColor(.accentColor)
                }
            }
            .padding(16)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .padding(.horizontal, 20)

            Button(role: .destructive) {
                cancelActiveJob(jobId: jobId)
            } label: {
                Text("取消克隆")
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.red)
                    .padding(.horizontal, 24)
                    .padding(.vertical, 10)
                    .background(Color.red.opacity(0.1))
                    .clipShape(Capsule())
            }
            .buttonStyle(.plain)

            Spacer()
        }
    }

    private func completedBody(project: ProjectItem, repoSlug: String, alreadyExists: Bool) -> some View {
        VStack(spacing: 20) {
            Spacer()

            ZStack {
                Circle()
                    .fill(Color.green.opacity(0.14))
                    .frame(width: 70, height: 70)
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 36))
                    .foregroundColor(.green)
            }

            VStack(spacing: 6) {
                Text(alreadyExists ? "已导入 Antigravity 项目列表" : "克隆完成，已加入项目列表")
                    .font(.system(size: 18, weight: .bold))
                    .foregroundColor(.primary)
                    .multilineTextAlignment(.center)
                Text("现在可以在「新建会话」工作区列表中随时选用该项目")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, 24)

            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 12) {
                    ZStack {
                        Circle()
                            .fill(Color.accentColor.opacity(0.14))
                            .frame(width: 42, height: 42)
                        Image(systemName: "folder.fill")
                            .font(.system(size: 17))
                            .foregroundColor(.accentColor)
                    }
                    VStack(alignment: .leading, spacing: 3) {
                        Text(project.displayName)
                            .font(.system(size: 16, weight: .semibold))
                            .foregroundColor(.primary)
                        if !repoSlug.isEmpty {
                            Text(repoSlug)
                                .font(.system(size: 12, design: .monospaced))
                                .foregroundColor(.secondary)
                        }
                    }
                    Spacer()
                }
                Divider()
                Text(project.path)
                    .font(.system(size: 11.5, design: .monospaced))
                    .foregroundColor(.secondary)
                    .textSelection(.enabled)
            }
            .padding(14)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .padding(.horizontal, 20)

            Spacer()

            VStack(spacing: 10) {
                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    onStartConversation(project)
                } label: {
                    Text("立即开启新会话")
                        .font(.system(size: 15.5, weight: .semibold))
                        .foregroundColor(.white)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 13)
                        .background(Color.accentColor)
                        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
                }

                Button {
                    dismiss()
                } label: {
                    Text("返回工作区列表")
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.primary)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 12)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
                }
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 20)
        }
    }

    private func startClone(importExisting: Bool) {
        guard let url = AppSettings.shared.gatewayURL, let parsed = parsedRepo, !isSubmitting else { return }
        isSubmitting = true
        errorMessage = nil
        conflictAlreadyClonedPath = nil
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()

        let folderArg = customFolderName.trimmingCharacters(in: .whitespacesAndNewlines)
        let parentArg = dirVM.currentPath

        Task {
            defer { isSubmitting = false }
            do {
                let resp = try await APIClient.shared.startCloneGitHubRepo(
                    repoSlug: repoInput.trimmingCharacters(in: .whitespacesAndNewlines),
                    parentDir: parentArg,
                    folderName: folderArg.isEmpty ? nil : folderArg,
                    asyncMode: true,
                    importExisting: importExisting,
                    startSession: false,
                    baseURL: url
                )
                if resp.code == "already_cloned" {
                    conflictAlreadyClonedPath = resp.existingPath ?? resp.targetPath ?? previewTargetPath
                    return
                }
                if resp.code == "target_exists" || resp.code == "path_is_file" {
                    showFolderRename = true
                    errorMessage = resp.error ?? String(localized: "目标目录已存在，请更换位置或修改文件夹名")
                    return
                }
                if resp.ok == false {
                    errorMessage = resp.error ?? String(localized: "克隆仓库失败")
                    return
                }
                if resp.status == "done", let project = resp.project {
                    finishWithProject(project, repoSlug: resp.repoSlug ?? parsed.slug, alreadyExists: resp.alreadyExists ?? false)
                    return
                }
                if let jobId = resp.jobId, !jobId.isEmpty {
                    let target = resp.targetPath ?? previewTargetPath
                    phase = .running(
                        jobId: jobId,
                        progress: resp.progress ?? 5,
                        message: resp.message ?? String(localized: "正在连接 GitHub 并克隆仓库…"),
                        targetPath: target
                    )
                    beginPolling(jobId: jobId, repoSlug: parsed.slug, baseURL: url, fallbackTarget: target)
                } else {
                    errorMessage = resp.error ?? String(localized: "网关未返回有效的克隆任务 ID")
                }
            } catch {
                errorMessage = String(localized: "克隆仓库失败: \(error.localizedDescription)")
            }
        }
    }

    private func beginPolling(jobId: String, repoSlug: String, baseURL: URL, fallbackTarget: String) {
        pollTask?.cancel()
        pollTask = Task {
            var transientFailures = 0
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 500_000_000)
                if Task.isCancelled { break }
                do {
                    let st = try await APIClient.shared.fetchCloneGitHubRepoStatus(jobId: jobId, baseURL: baseURL)
                    transientFailures = 0
                    let target = st.targetPath ?? st.path ?? fallbackTarget
                    switch st.status {
                    case "done":
                        if let project = st.project {
                            finishWithProject(project, repoSlug: st.repoSlug ?? repoSlug, alreadyExists: st.alreadyExists ?? false)
                        } else {
                            let fallbackProj = ProjectItem(
                                name: URL(fileURLWithPath: target).lastPathComponent,
                                uri: st.folderUri ?? "file://\(target)",
                                path: st.path ?? target,
                                isWorkspace: false,
                                lastActive: Date()
                            )
                            finishWithProject(fallbackProj, repoSlug: st.repoSlug ?? repoSlug, alreadyExists: st.alreadyExists ?? false)
                        }
                        return
                    case "failed":
                        phase = .form
                        errorMessage = st.error ?? st.message ?? String(localized: "克隆失败")
                        UINotificationFeedbackGenerator().notificationOccurred(.error)
                        return
                    case "cancelled":
                        phase = .form
                        errorMessage = String(localized: "已取消克隆")
                        return
                    default:
                        phase = .running(
                            jobId: jobId,
                            progress: st.progress ?? 10,
                            message: st.message ?? String(localized: "正在克隆仓库…"),
                            targetPath: target
                        )
                    }
                } catch {
                    transientFailures += 1
                    if transientFailures >= 6 {
                        phase = .form
                        errorMessage = String(localized: "查询克隆进度失败: \(error.localizedDescription)")
                        return
                    }
                }
            }
        }
    }

    private func cancelActiveJob(jobId: String) {
        pollTask?.cancel()
        pollTask = nil
        phase = .form
        errorMessage = String(localized: "已取消克隆")
        guard let url = AppSettings.shared.gatewayURL else { return }
        Task {
            _ = try? await APIClient.shared.cancelCloneGitHubRepo(jobId: jobId, baseURL: url)
        }
    }

    private func finishWithProject(_ project: ProjectItem, repoSlug: String, alreadyExists: Bool) {
        pollTask?.cancel()
        pollTask = nil
        onProjectReady(project)
        phase = .completed(project: project, repoSlug: repoSlug, alreadyExists: alreadyExists)
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    static func parseGitHubRepo(_ raw: String) -> (slug: String, repoName: String)? {
        var s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !s.isEmpty, !s.hasPrefix("-") else { return nil }
        var fromURL = false
        if s.hasPrefix("git@github.com:") {
            s = String(s.dropFirst("git@github.com:".count))
            fromURL = true
        } else if s.hasPrefix("https://github.com/") {
            s = String(s.dropFirst("https://github.com/".count))
            fromURL = true
        } else if s.hasPrefix("http://github.com/") {
            s = String(s.dropFirst("http://github.com/".count))
            fromURL = true
        } else if s.contains("://") || s.contains("@") {
            return nil
        }
        if let qIdx = s.firstIndex(where: { $0 == "?" || $0 == "#" }) {
            s = String(s[..<qIdx])
        }
        s = s.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let parts = s.split(separator: "/").map(String.init)
        guard parts.count >= 2, (fromURL || parts.count == 2) else { return nil }
        let owner = parts[0]
        var repo = parts[1]
        if repo.hasSuffix(".git") {
            repo = String(repo.dropLast(4))
        }
        guard owner != ".", owner != "..", repo != ".", repo != ".." else { return nil }
        let segPattern = #"^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$"#
        guard owner.range(of: segPattern, options: .regularExpression) != nil,
              repo.range(of: segPattern, options: .regularExpression) != nil else {
            return nil
        }
        return ("\(owner)/\(repo)", repo)
    }
}

// MARK: - Visual Add Local Workspace Page

struct AddLocalWorkspaceView: View {
    @Environment(\.dismiss) private var dismiss

    let onProjectReady: (ProjectItem) -> Void
    let onStartConversation: (ProjectItem) -> Void

    @StateObject private var dirVM = LocalDirectoryPickerViewModel()
    @State private var isSubmitting: Bool = false
    @State private var errorMessage: String? = nil
    @State private var showManualPathAlert: Bool = false
    @State private var manualPathInput: String = ""
    @State private var completedProject: ProjectItem? = nil
    @State private var completedAlreadyExists: Bool = false

    var body: some View {
        Group {
            if let project = completedProject {
                completedBody(project: project)
            } else {
                pickerBody
            }
        }
        .navigationTitle("添加本地工作区")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if completedProject == nil {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        manualPathInput = dirVM.currentPath
                        showManualPathAlert = true
                    } label: {
                        Text("手输路径")
                            .font(.system(size: 13.5, weight: .medium))
                    }
                }
            }
        }
        .alert("输入本机目录路径", isPresented: $showManualPathAlert) {
            TextField(manualPlaceholder, text: $manualPathInput)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            Button("取消", role: .cancel) {}
            Button("跳转") {
                let p = manualPathInput.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !p.isEmpty else { return }
                Task { await dirVM.load(path: p) }
            }
            Button("直接添加") {
                let p = manualPathInput.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !p.isEmpty else { return }
                addWorkspace(path: p)
            }
        } message: {
            Text("可跳转到该目录继续浏览，或直接添加为工作区。")
        }
        .task {
            if dirVM.currentPath.isEmpty {
                await dirVM.load()
            }
        }
    }

    private var manualPlaceholder: String {
        if AppSettings.shared.gatewayPlatform == "windows" {
            return #"C:\Users\...\project"#
        }
        return String(localized: "/Users/... 或 ~/Projects/...")
    }

    private var pickerBody: some View {
        VStack(spacing: 0) {
            ScrollView(.vertical, showsIndicators: true) {
                VStack(alignment: .leading, spacing: 16) {
                    LocalDirectoryBrowserSection(
                        viewModel: dirVM,
                        sectionTitle: String(localized: "浏览或新建本机文件夹"),
                        subtitle: String(localized: "进入目标目录后点击底部按钮，即可将其加入项目列表"),
                        onSelectDirPathChanged: { _ in
                            errorMessage = nil
                        }
                    )

                    if !dirVM.currentPath.isEmpty {
                        VStack(alignment: .leading, spacing: 5) {
                            Text("当前选定目录")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(.secondary)
                            Text(dirVM.currentPath)
                                .font(.system(size: 12.5, design: .monospaced))
                                .foregroundColor(.primary)
                                .textSelection(.enabled)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(12)
                        .background(Color.accentColor.opacity(0.08))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }

                    if let err = errorMessage {
                        HStack(alignment: .top, spacing: 8) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundColor(.orange)
                                .font(.system(size: 14))
                            Text(err)
                                .font(.system(size: 12.5))
                                .foregroundColor(.primary)
                            Spacer()
                        }
                        .padding(12)
                        .background(Color.orange.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }
                }
                .padding(.horizontal, 16)
                .padding(.top, 14)
                .padding(.bottom, 20)
            }

            Divider()

            Button {
                addWorkspace(path: dirVM.currentPath)
            } label: {
                HStack(spacing: 8) {
                    if isSubmitting {
                        ProgressView()
                            .tint(.white)
                            .scaleEffect(0.85)
                    } else {
                        Image(systemName: "plus.circle.fill")
                            .font(.system(size: 16))
                    }
                    Text(isSubmitting ? "正在添加工作区…" : "添加当前目录为工作区")
                        .font(.system(size: 15.5, weight: .semibold))
                }
                .foregroundColor(.white)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 13)
                .background(!dirVM.currentPath.isEmpty && !isSubmitting ? Color.accentColor : Color.secondary.opacity(0.35))
                .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
            }
            .disabled(dirVM.currentPath.isEmpty || isSubmitting)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color(uiColor: .systemBackground))
        }
    }

    private func completedBody(project: ProjectItem) -> some View {
        VStack(spacing: 20) {
            Spacer()
            ZStack {
                Circle()
                    .fill(Color.green.opacity(0.14))
                    .frame(width: 70, height: 70)
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 36))
                    .foregroundColor(.green)
            }
            VStack(spacing: 6) {
                Text(completedAlreadyExists ? "工作区已在项目列表中" : "已添加到 Antigravity 项目列表")
                    .font(.system(size: 18, weight: .bold))
                Text(project.path)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 24)
            }
            Spacer()
            VStack(spacing: 10) {
                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    onStartConversation(project)
                } label: {
                    Text("立即开启新会话")
                        .font(.system(size: 15.5, weight: .semibold))
                        .foregroundColor(.white)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 13)
                        .background(Color.accentColor)
                        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
                }
                Button {
                    dismiss()
                } label: {
                    Text("返回工作区列表")
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.primary)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 12)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
                }
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 20)
        }
    }

    private func addWorkspace(path: String) {
        let trimmed = path.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = AppSettings.shared.gatewayURL, !trimmed.isEmpty, !isSubmitting else { return }
        isSubmitting = true
        errorMessage = nil
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { isSubmitting = false }
            do {
                let resp = try await APIClient.shared.createProjectWorkspace(
                    path: trimmed,
                    startSession: false,
                    baseURL: url
                )
                if resp.ok == false {
                    errorMessage = resp.error ?? String(localized: "添加工作区失败")
                    return
                }
                let proj = resp.project ?? ProjectItem(
                    name: URL(fileURLWithPath: trimmed).lastPathComponent,
                    uri: "file://\(trimmed)",
                    path: trimmed,
                    isWorkspace: false,
                    lastActive: Date()
                )
                onProjectReady(proj)
                completedAlreadyExists = resp.alreadyExists ?? false
                completedProject = proj
                UINotificationFeedbackGenerator().notificationOccurred(.success)
            } catch {
                errorMessage = String(localized: "添加工作区失败: \(error.localizedDescription)")
            }
        }
    }
}
