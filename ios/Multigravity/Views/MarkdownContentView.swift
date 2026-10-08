import SwiftUI

public struct MarkdownContentView: View {
    public let content: String
    public let onImageTap: ((URL) -> Void)?
    let blocks: [MarkdownBlock]
    
    @State var internalPreviewImage: IdentifiableImage? = nil
    @Environment(\.isShareExport) var isShareExport
    
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
                    } else {
                        codeBlockView(lang: lang, code: code)
                    }
                    
                case .table(_, let headers, let rows, let alignments):
                    tableView(headers: headers, rows: rows, alignments: alignments)
                    
                case .list(_, let items):
                    listView(items: items)
                    
                case .orderedList(_, let startIndex, let items):
                    orderedListView(startIndex: startIndex, items: items)
                    
                case .paragraph(_, let text):
                    paragraphView(text: text, size: 15)
                    
                case .image(_, let alt, let url):
                    markdownImageView(alt: alt, urlString: url)
                    
                case .agentEmbed(_, let src):
                    if isShareExport {
                        shareExportPlaceholder("交互内容，请在 App 中查看")
                    } else {
                        AgentEmbedView(src: src)
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
