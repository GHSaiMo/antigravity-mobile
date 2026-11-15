import Foundation
import SwiftUI
import UIKit

/// Zero-dependency offline code syntax highlighter with bounded LRU/NSCache memoization.
/// Produces adaptive Light/Dark `AttributedString` values without WebViews or remote assets.
public enum CodeSyntaxHighlighter {
    private final class Entry: NSObject {
        let source: String
        let attributed: AttributedString
        init(source: String, attributed: AttributedString) {
            self.source = source
            self.attributed = attributed
        }
    }

    nonisolated(unsafe) private static let cache: NSCache<NSString, Entry> = {
        let c = NSCache<NSString, Entry>()
        c.countLimit = 256
        return c
    }()

    private static let maxHighlightUTF16Length = 24_000

    private struct Palette {
        let keyword: UIColor
        let string: UIColor
        let comment: UIColor
        let number: UIColor
        let typeName: UIColor
        let function: UIColor
        let property: UIColor
        let diffAdd: UIColor
        let diffDel: UIColor

        static let dark = Palette(
            keyword: UIColor(red: 1.00, green: 0.48, blue: 0.45, alpha: 1.0),   // #ff7b72
            string: UIColor(red: 0.65, green: 0.84, blue: 1.00, alpha: 1.0),    // #a5d6ff
            comment: UIColor(red: 0.55, green: 0.58, blue: 0.62, alpha: 1.0),   // #8b949e
            number: UIColor(red: 0.47, green: 0.75, blue: 1.00, alpha: 1.0),    // #79c0ff
            typeName: UIColor(red: 1.00, green: 0.65, blue: 0.34, alpha: 1.0),  // #ffa657
            function: UIColor(red: 0.82, green: 0.66, blue: 1.00, alpha: 1.0),  // #d2a8ff
            property: UIColor(red: 0.49, green: 0.91, blue: 0.53, alpha: 1.0),  // #7ee787
            diffAdd: UIColor(red: 0.34, green: 0.83, blue: 0.39, alpha: 1.0),
            diffDel: UIColor(red: 0.97, green: 0.32, blue: 0.29, alpha: 1.0)
        )

        static let light = Palette(
            keyword: UIColor(red: 0.81, green: 0.13, blue: 0.18, alpha: 1.0),   // #cf222e
            string: UIColor(red: 0.04, green: 0.19, blue: 0.41, alpha: 1.0),    // #0a3069
            comment: UIColor(red: 0.43, green: 0.47, blue: 0.51, alpha: 1.0),   // #6e7781
            number: UIColor(red: 0.02, green: 0.31, blue: 0.68, alpha: 1.0),    // #0550ae
            typeName: UIColor(red: 0.58, green: 0.22, blue: 0.00, alpha: 1.0),  // #953800
            function: UIColor(red: 0.51, green: 0.31, blue: 0.87, alpha: 1.0),  // #8250df
            property: UIColor(red: 0.07, green: 0.39, blue: 0.16, alpha: 1.0),  // #116329
            diffAdd: UIColor(red: 0.10, green: 0.50, blue: 0.22, alpha: 1.0),
            diffDel: UIColor(red: 0.81, green: 0.13, blue: 0.18, alpha: 1.0)
        )
    }

    private static let jsonRegex = try? NSRegularExpression(
        pattern: #"("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)"#,
        options: []
    )

    private static let yamlRegex = try? NSRegularExpression(
        pattern: #"(#[^\n]*)|^(\s*[\w.-]+)\s*:|("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|\b(true|false|null|yes|no)\b|(\b-?\d+(?:\.\d+)?\b)"#,
        options: [.anchorsMatchLines]
    )

    private static let markupRegex = try? NSRegularExpression(
        pattern: #"(<!--[\s\S]*?-->)|(</?[\w:-]+)|(\s[\w:-]+)(?==)|("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')"#,
        options: []
    )

    private static let codeRegex = try? NSRegularExpression(
        pattern: #"(\/\/[^\n]*|\/\*[\s\S]*?\*\/|#(?!!\[)[^\n]*)|("""[\s\S]*?"""|"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*'|`(?:\\.|[^`\\])*`)|(@[A-Za-z_]\w*)|\b(0x[0-9a-fA-F_]+|0b[01_]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)\b|\b(func|fn|def|function|return|if|else|elif|for|while|loop|match|switch|case|default|break|continue|struct|class|enum|interface|trait|protocol|extension|impl|type|typealias|import|from|export|package|module|pub|public|private|internal|fileprivate|protected|static|final|const|let|var|val|mut|async|await|try|catch|throw|throws|defer|go|select|chan|range|in|as|is|nil|null|None|True|False|true|false|self|Self|super|this|new|guard|where|actor|nonisolated|some|any|override|open|suspend|data|sealed|companion|object)\b|\b([A-Z][A-Za-z0-9_]*)\b|\b([a-zA-Z_]\w*)(?=\s*\()"#,
        options: []
    )

    public static func highlight(code: String, language: String, isDark: Bool) -> AttributedString {
        let utf16Len = (code as NSString).length
        guard utf16Len > 0, utf16Len <= maxHighlightUTF16Length else {
            return AttributedString(code)
        }

        let normLang = normalizeLanguage(language)
        if normLang == "text" {
            return AttributedString(code)
        }

        let key = "\(isDark ? "d" : "l"):\(normLang):\(utf16Len):\(code.hashValue)" as NSString
        if let hit = cache.object(forKey: key), hit.source == code {
            return hit.attributed
        }

        let palette = isDark ? Palette.dark : Palette.light
        let nsAttr = NSMutableAttributedString(string: code)
        let fullRange = NSRange(location: 0, length: utf16Len)

        switch normLang {
        case "diff":
            highlightDiff(code: code, into: nsAttr, palette: palette)
        case "json":
            if let regex = jsonRegex {
                for m in regex.matches(in: code, range: fullRange) {
                    if m.range(at: 1).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.property, range: m.range(at: 1))
                    } else if m.range(at: 2).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.string, range: m.range(at: 2))
                    } else if m.range(at: 3).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 3))
                    } else if m.range(at: 4).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.number, range: m.range(at: 4))
                    }
                }
            }
        case "yaml":
            if let regex = yamlRegex {
                for m in regex.matches(in: code, range: fullRange) {
                    if m.range(at: 1).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.comment, range: m.range(at: 1))
                    } else if m.range(at: 2).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.property, range: m.range(at: 2))
                    } else if m.range(at: 3).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.string, range: m.range(at: 3))
                    } else if m.range(at: 4).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 4))
                    } else if m.range(at: 5).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.number, range: m.range(at: 5))
                    }
                }
            }
        case "markup":
            if let regex = markupRegex {
                for m in regex.matches(in: code, range: fullRange) {
                    if m.range(at: 1).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.comment, range: m.range(at: 1))
                    } else if m.range(at: 2).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 2))
                    } else if m.range(at: 3).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.property, range: m.range(at: 3))
                    } else if m.range(at: 4).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.string, range: m.range(at: 4))
                    }
                }
            }
        default:
            if let regex = codeRegex {
                for m in regex.matches(in: code, range: fullRange) {
                    if m.range(at: 1).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.comment, range: m.range(at: 1))
                    } else if m.range(at: 2).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.string, range: m.range(at: 2))
                    } else if m.range(at: 3).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.function, range: m.range(at: 3))
                    } else if m.range(at: 4).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.number, range: m.range(at: 4))
                    } else if m.range(at: 5).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 5))
                    } else if m.range(at: 6).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.typeName, range: m.range(at: 6))
                    } else if m.range(at: 7).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.function, range: m.range(at: 7))
                    }
                }
            }
        }

        let result = (try? AttributedString(nsAttr, including: \.uiKit)) ?? AttributedString(code)
        cache.setObject(Entry(source: code, attributed: result), forKey: key)
        return result
    }

    private static func highlightDiff(code: String, into nsAttr: NSMutableAttributedString, palette: Palette) {
        let nsStr = code as NSString
        var lineStart = 0
        let total = nsStr.length
        while lineStart < total {
            let lineRange = nsStr.lineRange(for: NSRange(location: lineStart, length: 0))
            if lineRange.length > 0 {
                let firstChar = nsStr.character(at: lineRange.location)
                if firstChar == 0x2B /* '+' */ {
                    nsAttr.addAttribute(.foregroundColor, value: palette.diffAdd, range: lineRange)
                } else if firstChar == 0x2D /* '-' */ {
                    nsAttr.addAttribute(.foregroundColor, value: palette.diffDel, range: lineRange)
                } else if firstChar == 0x40 /* '@' */ {
                    nsAttr.addAttribute(.foregroundColor, value: palette.function, range: lineRange)
                }
            }
            lineStart = NSMaxRange(lineRange)
        }
    }

    private static func normalizeLanguage(_ raw: String) -> String {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        switch s {
        case "txt", "plain", "plaintext", "output", "log":
            return "text"
        case "json", "jsonc", "jsonl":
            return "json"
        case "yaml", "yml", "toml":
            return "yaml"
        case "diff", "patch":
            return "diff"
        case "html", "xml", "svg":
            return "markup"
        default:
            return "code"
        }
    }
}
