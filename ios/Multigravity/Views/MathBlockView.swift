import SwiftUI
import UIKit

/// Native display math block (`$$ ... $$` / `\[ ... \]`).
/// Renders offline KaTeX snapshots via the shared KaTeXRenderEngine with instant Unicode
/// fallback while rendering or on incomplete streaming syntax, plus horizontal scroll for wide formulas.
public struct MathBlockView: View {
    public let latex: String
    public let bodyScale: CGFloat

    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.isShareExport) private var isShareExport
    @Environment(\.shareCardUseNord) private var shareCardUseNord
    @Environment(\.shareCardThemeID) private var shareCardThemeID

    @State private var rendered: RenderedMathFormula? = nil
    @State private var didAttemptRender = false

    public init(latex: String, bodyScale: CGFloat = 1.0) {
        self.latex = latex
        self.bodyScale = bodyScale
    }

    private var fontSize: CGFloat {
        16.5 * bodyScale
    }

    private var resolvedSharePreset: ThemePreset? {
        if let themeID = shareCardThemeID, !themeID.isEmpty {
            return ThemePresetCatalog.findByThemeKey(themeID, fallbackIsDark: colorScheme == .dark)
        }
        if shareCardUseNord == true {
            return ThemePresetCatalog.findByThemeKey("nord", fallbackIsDark: true)
        }
        return nil
    }

    private var effectiveIsDark: Bool {
        resolvedSharePreset?.isDark ?? (colorScheme == .dark)
    }

    private var formulaForeground: Color {
        resolvedSharePreset.flatMap { $0.isDefaultStyle ? nil : $0.fg } ?? .primary
    }

    private var cardBackground: Color {
        resolvedSharePreset.map { $0.isDefaultStyle ? Color(uiColor: .tertiarySystemBackground).opacity(0.55) : $0.codeBlockBg }
            ?? Color(uiColor: .tertiarySystemBackground).opacity(0.55)
    }

    private var cardBorder: Color {
        resolvedSharePreset.map { $0.isDefaultStyle ? Color.secondary.opacity(0.18) : $0.border }
            ?? Color.secondary.opacity(0.18)
    }

    private var renderTaskKey: String {
        "\(latex)|\(fontSize)|\(effectiveIsDark)"
    }

    public var body: some View {
        let activeFormula = rendered ?? KaTeXRenderEngine.shared.cached(
            latex: latex,
            displayMode: true,
            fontSize: fontSize,
            isDark: effectiveIsDark
        )

        Group {
            if let formula = activeFormula {
                if isShareExport {
                    formulaImage(formula)
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding(.horizontal, 14)
                } else {
                    ViewThatFits(in: .horizontal) {
                        formulaImage(formula)
                            .padding(.horizontal, 14)
                            .frame(maxWidth: .infinity, alignment: .center)
                            .padding(.vertical, 2)
                        ScrollView(.horizontal, showsIndicators: false) {
                            formulaImage(formula)
                                .padding(.horizontal, 14)
                                .padding(.vertical, 2)
                        }
                    }
                }
            } else {
                let fallbackText = MathSymbolProcessor.cleanMathExpression(
                    latex.replacingOccurrences(of: #"\\\\"#, with: "\n")
                )
                Group {
                    if isShareExport {
                        Text(fallbackText)
                            .font(.system(size: 15.5 * bodyScale, design: .serif))
                            .foregroundColor(formulaForeground)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 14)
                            .frame(maxWidth: .infinity, alignment: .center)
                    } else {
                        ViewThatFits(in: .horizontal) {
                            Text(fallbackText)
                                .font(.system(size: 15.5 * bodyScale, design: .serif))
                                .foregroundColor(formulaForeground)
                                .lineSpacing(4)
                                .multilineTextAlignment(.center)
                                .fixedSize(horizontal: true, vertical: true)
                                .padding(.horizontal, 14)
                                .frame(maxWidth: .infinity, alignment: .center)
                            ScrollView(.horizontal, showsIndicators: false) {
                                Text(fallbackText)
                                    .font(.system(size: 15.5 * bodyScale, design: .serif))
                                    .foregroundColor(formulaForeground)
                                    .lineSpacing(4)
                                    .fixedSize(horizontal: true, vertical: true)
                                    .padding(.horizontal, 14)
                            }
                        }
                    }
                }
            }
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(cardBorder, lineWidth: 0.8)
        )
        .contextMenu {
            if !isShareExport {
                Button {
                    UIPasteboard.general.string = latex.trimmingCharacters(in: .whitespacesAndNewlines)
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    CopiedHUD.show(String(localized: "已复制 LaTeX 公式"))
                } label: {
                    Label("复制 LaTeX", systemImage: "doc.on.doc")
                }
                Button {
                    let plain = MathSymbolProcessor.cleanMathExpression(
                        latex.replacingOccurrences(of: #"\\\\"#, with: "\n")
                    )
                    UIPasteboard.general.string = plain
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    CopiedHUD.show(String(localized: "已复制公式文本"))
                } label: {
                    Label("复制文本", systemImage: "text.alignleft")
                }
            }
        }
        .task(id: renderTaskKey) {
            if isShareExport { return }
            if let hit = KaTeXRenderEngine.shared.cached(
                latex: latex,
                displayMode: true,
                fontSize: fontSize,
                isDark: effectiveIsDark
            ) {
                rendered = hit
                didAttemptRender = true
                return
            }
            let result = await KaTeXRenderEngine.shared.render(
                latex: latex,
                displayMode: true,
                fontSize: fontSize,
                isDark: effectiveIsDark
            )
            rendered = result
            didAttemptRender = true
        }
    }

    @ViewBuilder
    private func formulaImage(_ formula: RenderedMathFormula) -> some View {
        let baseImg = Image(uiImage: formula.image)
            .interpolation(.high)
        if isShareExport {
            baseImg
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(maxWidth: formula.width, maxHeight: formula.height)
                .foregroundColor(formulaForeground)
        } else {
            baseImg
                .resizable()
                .frame(width: formula.width, height: formula.height)
                .foregroundColor(formulaForeground)
        }
    }
}
