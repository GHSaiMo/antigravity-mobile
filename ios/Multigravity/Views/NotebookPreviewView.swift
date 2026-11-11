import SwiftUI

// MARK: - Jupyter Notebook Data Structures

public struct NotebookDocument {
    public let cells: [NotebookCell]
}

public struct NotebookCell: Identifiable {
    public let id = UUID()
    public let cellType: String // "markdown", "code", "raw"
    public let source: String
    public let executionCount: Int?
    public let outputs: [NotebookOutput]
}

public struct NotebookOutput: Identifiable {
    public let id = UUID()
    public let outputType: String // "stream", "execute_result", "display_data", "error"
    public let text: String?
    public let imageBase64: String?
}

// MARK: - Notebook Preview View

public struct NotebookPreviewView: View {
    public let url: URL
    @State private var document: NotebookDocument? = nil
    @State private var errorMessage: String? = nil
    @State private var isLoading: Bool = true
    @State private var copiedCellIndex: Int? = nil

    public init(url: URL) {
        self.url = url
    }

    public var body: some View {
        Group {
            if isLoading {
                ProgressView("正在解析 Jupyter Notebook...")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let error = errorMessage {
                VStack(spacing: 12) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.system(size: 36))
                        .foregroundColor(.orange)
                    Text("无法解析 Notebook")
                        .font(.system(size: 16, weight: .semibold))
                    Text(error)
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 24)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let doc = document {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 14) {
                        ForEach(Array(doc.cells.enumerated()), id: \.element.id) { index, cell in
                            cellCard(cell: cell, index: index)
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                }
            }
        }
        .task {
            loadNotebook()
        }
    }

    @ViewBuilder
    private func cellCard(cell: NotebookCell, index: Int) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            if cell.cellType == "markdown" {
                // Markdown cell
                MarkdownContentView(content: cell.source)
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .secondarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            } else if cell.cellType == "code" {
                // Code cell
                VStack(alignment: .leading, spacing: 6) {
                    // Cell header
                    HStack {
                        Text("In [\(cell.executionCount.map(String.init) ?? " ")]:")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(.accentColor)

                        Spacer()

                        Button(action: {
                            UIPasteboard.general.string = cell.source
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            copiedCellIndex = index
                            DispatchQueue.main.asyncAfter(deadline: .now() + 1.8) {
                                if copiedCellIndex == index { copiedCellIndex = nil }
                            }
                        }) {
                            HStack(spacing: 3) {
                                Image(systemName: copiedCellIndex == index ? "checkmark" : "doc.on.doc")
                                    .font(.system(size: 10))
                                Text(copiedCellIndex == index ? "已复制" : "复制")
                                    .font(.system(size: 10.5))
                            }
                            .foregroundColor(.secondary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 3)
                            .background(Color.secondary.opacity(0.1))
                            .clipShape(Capsule())
                        }
                    }

                    // Code body
                    ScrollView(.horizontal, showsIndicators: false) {
                        Text(cell.source)
                            .font(.system(size: 12.5, design: .monospaced))
                            .foregroundColor(.primary)
                            .lineSpacing(2)
                            .padding(.vertical, 4)
                    }

                    // Cell outputs
                    if !cell.outputs.isEmpty {
                        Divider()
                            .padding(.vertical, 2)

                        ForEach(cell.outputs) { out in
                            outputView(out)
                        }
                    }
                }
                .padding(12)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(Color.secondary.opacity(0.15), lineWidth: 0.8)
                )
            } else {
                Text(cell.source)
                    .font(.system(size: 12.5, design: .monospaced))
                    .padding(10)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
        }
    }

    @ViewBuilder
    private func outputView(_ out: NotebookOutput) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            if let uiImage = decodeBase64Image(out.imageBase64) {
                Image(uiImage: uiImage)
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: .infinity, alignment: .center)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .padding(.vertical, 4)
            }

            if let txt = out.text, !txt.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    Text(txt)
                        .font(.system(size: 11.5 * AppSettings.shared.fontSizeScale.codeScale, design: .monospaced))
                        .foregroundColor(out.outputType == "error" ? .red : .secondary)
                        .lineSpacing(2)
                }
                .padding(8)
                .background(Color(uiColor: .tertiarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
            }
        }
    }

    private func decodeBase64Image(_ raw: String?) -> UIImage? {
        guard var s = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty else {
            return nil
        }
        if let commaRange = s.range(of: "base64,") {
            s = String(s[commaRange.upperBound...])
        }
        guard let data = Data(base64Encoded: s, options: .ignoreUnknownCharacters) else {
            return nil
        }
        return UIImage(data: data)
    }

    private func loadNotebook() {
        isLoading = true
        errorMessage = nil
        do {
            let data = try Data(contentsOf: url)
            guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let cellsRaw = json["cells"] as? [[String: Any]] else {
                errorMessage = "无法识别的 Notebook JSON 结构"
                isLoading = false
                return
            }

            var cells: [NotebookCell] = []
            for c in cellsRaw {
                let cellType = c["cell_type"] as? String ?? "raw"
                let source = extractText(c["source"])
                let execCount = c["execution_count"] as? Int

                var outputs: [NotebookOutput] = []
                if let outs = c["outputs"] as? [[String: Any]] {
                    for o in outs {
                        let otype = o["output_type"] as? String ?? ""
                        var outText: String? = nil
                        var imgB64: String? = nil

                        if let textRaw = o["text"] {
                            outText = extractText(textRaw)
                        }
                        if let dataDict = o["data"] as? [String: Any] {
                            if let plain = dataDict["text/plain"] {
                                outText = (outText ?? "") + extractText(plain)
                            }
                            if let pngRaw = dataDict["image/png"] {
                                let extracted = extractText(pngRaw).trimmingCharacters(in: .whitespacesAndNewlines)
                                if !extracted.isEmpty { imgB64 = extracted }
                            }
                        }
                        if let traceback = o["traceback"] as? [String] {
                            outText = traceback.joined(separator: "\n")
                        }

                        outputs.append(NotebookOutput(outputType: otype, text: outText, imageBase64: imgB64))
                    }
                }

                cells.append(NotebookCell(cellType: cellType, source: source, executionCount: execCount, outputs: outputs))
            }

            self.document = NotebookDocument(cells: cells)
        } catch {
            self.errorMessage = error.localizedDescription
        }
        isLoading = false
    }

    private func extractText(_ raw: Any?) -> String {
        if let str = raw as? String {
            return str
        }
        if let arr = raw as? [String] {
            return arr.joined()
        }
        return ""
    }
}
