import SwiftUI

private struct CurrentCascadeIdKey: EnvironmentKey {
    static let defaultValue: String? = nil
}

private struct MarkdownAvailableWidthKey: EnvironmentKey {
    static let defaultValue: CGFloat? = nil
}

private struct MarkdownFontScaleMultiplierKey: EnvironmentKey {
    static let defaultValue: CGFloat = 1.0
}

private struct MarkdownBlockSpacingKey: EnvironmentKey {
    static let defaultValue: CGFloat = 10
}

extension EnvironmentValues {
    public var currentCascadeId: String? {
        get { self[CurrentCascadeIdKey.self] }
        set { self[CurrentCascadeIdKey.self] = newValue }
    }

    public var markdownAvailableWidth: CGFloat? {
        get { self[MarkdownAvailableWidthKey.self] }
        set { self[MarkdownAvailableWidthKey.self] = newValue }
    }

    public var markdownFontScaleMultiplier: CGFloat {
        get { self[MarkdownFontScaleMultiplierKey.self] }
        set { self[MarkdownFontScaleMultiplierKey.self] = newValue }
    }

    public var markdownBlockSpacing: CGFloat {
        get { self[MarkdownBlockSpacingKey.self] }
        set { self[MarkdownBlockSpacingKey.self] = newValue }
    }
}

public struct MarkdownContentView: View {
    public let content: String
    public let isStreaming: Bool
    public let onImageTap: ((URL) -> Void)?
    let blocks: [MarkdownBlock]
    
    @State var internalPreviewImage: IdentifiableImage? = nil
    @Environment(\.colorScheme) var colorScheme
    @Environment(\.isShareExport) var isShareExport
    @Environment(\.shareCardUseNord) var shareCardUseNord
    @Environment(\.shareCardThemeID) var shareCardThemeID
    @Environment(\.currentCascadeId) private var currentCascadeId
    @Environment(\.markdownAvailableWidth) var markdownAvailableWidth
    @Environment(\.markdownFontScaleMultiplier) var markdownFontScaleMultiplier
    @Environment(\.markdownBlockSpacing) var markdownBlockSpacing

    private var fontScale: AppSettings.FontSizeScale {
        isShareExport ? .standard : AppSettings.shared.fontSizeScale
    }
    
    public init(content: String, isStreaming: Bool = false, onImageTap: ((URL) -> Void)? = nil) {
        self.content = content
        self.isStreaming = isStreaming
        self.onImageTap = onImageTap
        let effectiveContent = isStreaming ? StreamingMarkdownStabilizer.stabilize(content) : content
        self.blocks = MarkdownBlockCache.shared.blocks(for: effectiveContent, cacheExactResult: !isStreaming)
    }
    
    public var body: some View {
        let activeFontScale = fontScale
        let bodyScale = activeFontScale.bodyScale * markdownFontScaleMultiplier
        let codeScale = activeFontScale.codeScale * markdownFontScaleMultiplier
        VStack(alignment: .leading, spacing: markdownBlockSpacing) {
            ForEach(blocks) { block in
                EquatableMarkdownBlock(
                    block: block,
                    bodyScale: bodyScale,
                    codeScale: codeScale,
                    colorScheme: colorScheme,
                    isShareExport: isShareExport,
                    shareCardUseNord: shareCardUseNord,
                    shareCardThemeID: shareCardThemeID,
                    currentCascadeId: currentCascadeId,
                    markdownAvailableWidth: markdownAvailableWidth
                ) {
                    renderBlock(block, bodyScale: bodyScale, codeScale: codeScale)
                }
                .equatable()
            }
        }
        .fullScreenCover(item: $internalPreviewImage) { item in
            ImageViewerSheet(item: item)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
    }

    @ViewBuilder
    private func renderBlock(_ block: MarkdownBlock, bodyScale: CGFloat, codeScale: CGFloat) -> some View {
        switch block {
        case .frontmatter(_, let rawContent, let lineCount):
            FrontmatterCollapseView(content: rawContent, lineCount: lineCount)
            
        case .heading(_, let level, let text):
            headingView(level: level, text: text, bodyScale: bodyScale)
            
        case .divider:
            Divider()
                .background(Color.secondary.opacity(0.3))
                .padding(.vertical, 4)
            
        case .codeBlock(_, let lang, let code):
            if lang.trimmingCharacters(in: .whitespaces).lowercased() == "mermaid" {
                if isShareExport {
                    shareExportPlaceholder(String(localized: "Mermaid 图表，请在 App 中查看"))
                } else {
                    MermaidDiagramView(code: code)
                }
            } else if isShareExport {
                let lines = code.split(separator: "\n", omittingEmptySubsequences: false).count
                shareExportPlaceholder(String(localized: "代码块（\(lines) 行），请在 App 中查看"))
            } else {
                codeBlockView(lang: lang, code: code, codeScale: codeScale)
            }
            
        case .mathBlock(_, let latex):
            MathBlockView(latex: latex, bodyScale: bodyScale)
            
        case .table(_, let headers, let rows, let alignments):
            let columns = max(headers.count, rows.map(\.count).max() ?? 0)
            if isShareExport && (columns >= 4 || rows.count > 12) {
                shareExportPlaceholder(String(localized: "表格（\(rows.count) 行 × \(columns) 列），请在 App 中查看"))
            } else {
                tableView(headers: headers, rows: rows, alignments: alignments, bodyScale: bodyScale)
            }
            
        case .list(_, let items, let indents):
            listView(items: items, indents: indents, bodyScale: bodyScale)
            
        case .orderedList(_, let startIndex, let items):
            orderedListView(startIndex: startIndex, items: items, bodyScale: bodyScale)
            
        case .alert(_, let type, let alertContent):
            alertView(type: type, content: alertContent)
            
        case .blockquote(_, let quoteContent):
            blockquoteView(content: quoteContent)
            
        case .paragraph(_, let text):
            paragraphView(text: text, size: 15 * bodyScale)
            
        case .image(_, let alt, let url):
            markdownImageView(alt: alt, urlString: url)
            
        case .agentEmbed(_, let src):
            if isShareExport {
                shareExportPlaceholder(String(localized: "交互内容，请在 App 中查看"))
            } else {
                AgentEmbedView(src: src)
            }

        case .pluginCard(_, let catalogId):
            if isShareExport {
                shareExportPlaceholder(String(localized: "推荐插件：\(catalogId)，请在 App 中查看"))
            } else {
                PluginCardBlockView(catalogId: catalogId, conversationId: currentCascadeId)
            }
            
        case .carousel(_, let slides):
            if isShareExport {
                shareExportPlaceholder(String(localized: "图片轮播（\(slides.count) 张），请在 App 中查看"))
            } else {
                MarkdownCarouselView(slides: slides, onImageTap: { url in
                    handleImageTap(url: url)
                })
            }
        }
    }
}

/// 通过 `Equatable` 隔离单个 Markdown 块：流式吐字时，上方已完成的表格、引用块、代码块、标题与段落
/// 全部跳过 SwiftUI body 重求值与 AutoTableLayout 重测，仅重绘末尾正在增长的单个块。
private struct EquatableMarkdownBlock<Content: View>: View, Equatable {
    let block: MarkdownBlock
    let bodyScale: CGFloat
    let codeScale: CGFloat
    let colorScheme: ColorScheme
    let isShareExport: Bool
    let shareCardUseNord: Bool?
    let shareCardThemeID: String?
    let currentCascadeId: String?
    let markdownAvailableWidth: CGFloat?
    @ViewBuilder let content: () -> Content

    static func == (lhs: Self, rhs: Self) -> Bool {
        lhs.block == rhs.block
            && lhs.bodyScale == rhs.bodyScale
            && lhs.codeScale == rhs.codeScale
            && lhs.colorScheme == rhs.colorScheme
            && lhs.isShareExport == rhs.isShareExport
            && lhs.shareCardUseNord == rhs.shareCardUseNord
            && lhs.shareCardThemeID == rhs.shareCardThemeID
            && lhs.currentCascadeId == rhs.currentCascadeId
            && lhs.markdownAvailableWidth == rhs.markdownAvailableWidth
    }

    var body: some View {
        content()
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

            Text(statusText ?? String(localized: "来自插件市场，安装后自动续聊恢复任务。"))
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
            statusText = String(localized: "未连接网关")
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
                statusText = resp.resumed == true ? String(localized: "插件已安装，已自动续接当前对话。") : String(localized: "插件安装成功。")
                UINotificationFeedbackGenerator().notificationOccurred(.success)
            } catch {
                statusText = String(localized: "安装失败: \(error.localizedDescription)")
            }
            isInstalling = false
        }
    }
}
