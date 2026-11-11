import SwiftUI

private struct CurrentCascadeIdKey: EnvironmentKey {
    static let defaultValue: String? = nil
}

extension EnvironmentValues {
    public var currentCascadeId: String? {
        get { self[CurrentCascadeIdKey.self] }
        set { self[CurrentCascadeIdKey.self] = newValue }
    }
}

public struct MarkdownContentView: View {
    public let content: String
    public let onImageTap: ((URL) -> Void)?
    let blocks: [MarkdownBlock]
    
    @State var internalPreviewImage: IdentifiableImage? = nil
    @Environment(\.isShareExport) var isShareExport
    @Environment(\.currentCascadeId) private var currentCascadeId

    private var fontScale: AppSettings.FontSizeScale {
        isShareExport ? .standard : AppSettings.shared.fontSizeScale
    }
    
    public init(content: String, onImageTap: ((URL) -> Void)? = nil) {
        self.content = content
        self.onImageTap = onImageTap
        self.blocks = MarkdownBlockCache.shared.blocks(for: content)
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(blocks) { block in
                switch block {
                case .frontmatter(_, let rawContent, let lineCount):
                    FrontmatterCollapseView(content: rawContent, lineCount: lineCount)
                    
                case .heading(_, let level, let text):
                    headingView(level: level, text: text)
                    
                case .divider:
                    Divider()
                        .background(Color.secondary.opacity(0.3))
                        .padding(.vertical, 4)
                    
                case .codeBlock(_, let lang, let code):
                    if lang.trimmingCharacters(in: .whitespaces).lowercased() == "mermaid" {
                        if isShareExport {
                            shareExportPlaceholder("Mermaid 图表，请在 App 中查看")
                        } else {
                            MermaidDiagramView(code: code)
                        }
                    } else if isShareExport {
                        let lines = code.split(separator: "\n", omittingEmptySubsequences: false).count
                        shareExportPlaceholder("代码块（\(lines) 行），请在 App 中查看")
                    } else {
                        codeBlockView(lang: lang, code: code)
                    }
                    
                case .table(_, let headers, let rows, let alignments):
                    let columns = max(headers.count, rows.map(\.count).max() ?? 0)
                    if isShareExport && (columns >= 4 || rows.count > 12) {
                        shareExportPlaceholder("表格（\(rows.count) 行 × \(columns) 列），请在 App 中查看")
                    } else {
                        tableView(headers: headers, rows: rows, alignments: alignments)
                    }
                    
                case .list(_, let items):
                    listView(items: items)
                    
                case .orderedList(_, let startIndex, let items):
                    orderedListView(startIndex: startIndex, items: items)
                    
                case .alert(_, let type, let alertContent):
                    alertView(type: type, content: alertContent)
                    
                case .blockquote(_, let quoteContent):
                    blockquoteView(content: quoteContent)
                    
                case .paragraph(_, let text):
                    paragraphView(text: text, size: 15 * fontScale.bodyScale)
                    
                case .image(_, let alt, let url):
                    markdownImageView(alt: alt, urlString: url)
                    
                case .agentEmbed(_, let src):
                    if isShareExport {
                        shareExportPlaceholder("交互内容，请在 App 中查看")
                    } else {
                        AgentEmbedView(src: src)
                    }

                case .pluginCard(_, let catalogId):
                    if isShareExport {
                        shareExportPlaceholder("推荐插件：\(catalogId)，请在 App 中查看")
                    } else {
                        PluginCardBlockView(catalogId: catalogId, conversationId: currentCascadeId)
                    }
                    
                case .carousel(_, let slides):
                    if isShareExport {
                        shareExportPlaceholder("图片轮播（\(slides.count) 张），请在 App 中查看")
                    } else {
                        MarkdownCarouselView(slides: slides, onImageTap: { url in
                            handleImageTap(url: url)
                        })
                    }
                }
            }
        }
        .fullScreenCover(item: $internalPreviewImage) { item in
            ImageViewerSheet(item: item)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
    }
}

private struct PluginCardBlockView: View {
    let catalogId: String
    let conversationId: String?

    @State private var isInstalling = false
    @State private var isInstalled = false
    @State private var statusText: String? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "puzzlepiece.extension.fill")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.blue)

                Text(catalogId)
                    .font(.system(size: 13.5, weight: .bold, design: .monospaced))
                    .foregroundColor(.primary)
                    .lineLimit(1)

                Spacer()

                if isInstalled {
                    Label("已安装", systemImage: "checkmark.circle.fill")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.green)
                } else {
                    Button(action: install) {
                        if isInstalling {
                            ProgressView().controlSize(.small)
                        } else {
                            Text("一键安装")
                                .font(.system(size: 12, weight: .semibold))
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.small)
                    .disabled(isInstalling)
                }
            }

            Text(statusText ?? "来自 Antigravity 官方 / 社区插件市场，安装后自动续聊恢复任务。")
                .font(.system(size: 12))
                .foregroundColor(.secondary)
        }
        .padding(12)
        .background(Color.blue.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.blue.opacity(0.25), lineWidth: 0.8)
        )
    }

    private func install() {
        guard let baseURL = AppSettings.shared.serverURL else {
            statusText = "未连接网关"
            return
        }
        isInstalling = true
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            do {
                let resp = try await APIClient.shared.installPlugin(
                    id: catalogId,
                    conversationId: conversationId,
                    autoResume: true,
                    baseURL: baseURL
                )
                isInstalled = resp.isSuccess
                statusText = resp.resumed == true ? "插件已安装，已自动续接当前对话。" : "插件安装成功。"
                UINotificationFeedbackGenerator().notificationOccurred(.success)
            } catch {
                statusText = "安装失败: \(error.localizedDescription)"
            }
            isInstalling = false
        }
    }
}
