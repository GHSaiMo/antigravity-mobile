import SwiftUI
import UIKit
import Foundation
import ImageIO

// MARK: - Jupyter Notebook Data Structures

nonisolated public struct NotebookDocument: Sendable {
    public let cells: [NotebookCell]
}

nonisolated public struct NotebookCell: Identifiable, Sendable {
    public let id = UUID()
    public let cellType: String // "markdown", "code", "raw"
    public let source: String
    public let executionCount: Int?
    public let outputs: [NotebookOutput]
}

nonisolated public struct NotebookOutput: Identifiable, Sendable {
    public let id = UUID()
    public let outputType: String // "stream", "execute_result", "display_data", "error"
    public let text: String?
    public let imageBase64: String?
    public let decodedImage: UIImage?
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
        .task(id: url) {
            await loadNotebook()
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
                                Text(copiedCellIndex == index ? String(localized: "已复制") : String(localized: "复制"))
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
            if let uiImage = out.decodedImage {
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

    private func loadNotebook() async {
        isLoading = true
        errorMessage = nil
        let targetURL = url
        let invalidMsg = String(localized: "无法识别的 Notebook JSON 结构")
        let result: Result<NotebookDocument, Error> = await Task.detached(priority: .userInitiated) {
            Self.parseNotebookFile(at: targetURL, invalidStructureMessage: invalidMsg)
        }.value

        switch result {
        case .success(let parsedDoc):
            self.document = parsedDoc
            self.errorMessage = nil
        case .failure(let err):
            self.document = nil
            self.errorMessage = err.localizedDescription
        }
        isLoading = false
    }

    nonisolated private static func parseNotebookFile(
        at url: URL,
        invalidStructureMessage: String
    ) -> Result<NotebookDocument, Error> {
        do {
            let data = try Data(contentsOf: url, options: [.mappedIfSafe])
            guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let cellsRaw = json["cells"] as? [[String: Any]] else {
                let err = NSError(
                    domain: "NotebookPreviewView",
                    code: 1,
                    userInfo: [NSLocalizedDescriptionKey: invalidStructureMessage]
                )
                return .failure(err)
            }

            var cells: [NotebookCell] = []
            cells.reserveCapacity(cellsRaw.count)
            for c in cellsRaw {
                let cellType = c["cell_type"] as? String ?? "raw"
                let source = extractText(c["source"])
                let execCount = c["execution_count"] as? Int

                var outputs: [NotebookOutput] = []
                if let outs = c["outputs"] as? [[String: Any]] {
                    outputs.reserveCapacity(outs.count)
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

                        let predecoded = decodeDownsampledImage(imgB64)
                        outputs.append(
                            NotebookOutput(
                                outputType: otype,
                                text: outText,
                                imageBase64: nil,
                                decodedImage: predecoded
                            )
                        )
                    }
                }

                cells.append(NotebookCell(cellType: cellType, source: source, executionCount: execCount, outputs: outputs))
            }

            return .success(NotebookDocument(cells: cells))
        } catch {
            return .failure(error)
        }
    }

    nonisolated private static func decodeDownsampledImage(_ raw: String?) -> UIImage? {
        guard var s = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty else {
            return nil
        }
        if let commaRange = s.range(of: "base64,") {
            s = String(s[commaRange.upperBound...])
        }
        guard let data = Data(base64Encoded: s, options: .ignoreUnknownCharacters) else {
            return nil
        }
        if let source = CGImageSourceCreateWithData(data as CFData, [kCGImageSourceShouldCache: false] as CFDictionary) {
            let thumbOptions: [CFString: Any] = [
                kCGImageSourceCreateThumbnailFromImageAlways: true,
                kCGImageSourceShouldCacheImmediately: true,
                kCGImageSourceCreateThumbnailWithTransform: true,
                kCGImageSourceThumbnailMaxPixelSize: 1600
            ]
            if let cgImg = CGImageSourceCreateThumbnailAtIndex(source, 0, thumbOptions as CFDictionary) {
                return UIImage(cgImage: cgImg)
            }
        }
        return UIImage(data: data)
    }

    nonisolated private static func extractText(_ raw: Any?) -> String {
        if let str = raw as? String {
            return str
        }
        if let arr = raw as? [String] {
            return arr.joined()
        }
        return ""
    }
}
