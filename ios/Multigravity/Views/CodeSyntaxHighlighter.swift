import Foundation
import SwiftUI
import UIKit

private struct ShareCardUseNordKey: EnvironmentKey {
    static let defaultValue: Bool? = nil
}

private struct ShareCardThemeIDKey: EnvironmentKey {
    static let defaultValue: String? = nil
}

extension EnvironmentValues {
    public var shareCardUseNord: Bool? {
        get { self[ShareCardUseNordKey.self] }
        set { self[ShareCardUseNordKey.self] = newValue }
    }

    public var shareCardThemeID: String? {
        get { self[ShareCardThemeIDKey.self] }
        set { self[ShareCardThemeIDKey.self] = newValue }
    }
}

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

    private typealias Palette = ThemeSyntaxPalette

    private static let rRegex = try? NSRegularExpression(
        pattern: #"(#[^\n]*)|("(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*')|(```\{[rR][^}]*\}|```)|\b(0x[0-9a-fA-F]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?[iL]?)\b|(<-|->|<<-|->>|\|>|%>%)|\b(function|if|else|repeat|while|for|in|next|break|return|TRUE|FALSE|NULL|NA|NA_integer_|NA_real_|NA_character_|NA_complex_|Inf|NaN|library|require|source|switch|tryCatch|stop|warning|message)\b|\b([a-zA-Z.][a-zA-Z0-9._]*)(?=\s*\()"#,
        options: []
    )

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

    public static func highlight(
        code: String,
        language: String,
        isDark: Bool,
        useNord: Bool = false,
        themeID: String? = nil
    ) -> AttributedString {
        let utf16Len = (code as NSString).length
        guard utf16Len > 0, utf16Len <= maxHighlightUTF16Length else {
            return AttributedString(code)
        }

        let normLang = normalizeLanguage(language)
        if normLang == "text" {
            return AttributedString(code)
        }

        let resolvedKey: String
        if let themeID, !themeID.isEmpty {
            resolvedKey = themeID
        } else if useNord {
            resolvedKey = "nord"
        } else {
            resolvedKey = AppSettings.shared.activeThemePreset(for: isDark ? .dark : .light).storageKey
        }
        let cacheKey = "\(resolvedKey):\(isDark ? 1 : 0):\(normLang):\(utf16Len):\(code.hashValue)" as NSString
        if let hit = cache.object(forKey: cacheKey), hit.source == code {
            return hit.attributed
        }

        let palette = ThemePresetCatalog.findByThemeKey(resolvedKey, fallbackIsDark: isDark).syntax
        let nsAttr = NSMutableAttributedString(string: code)
        let fullRange = NSRange(location: 0, length: utf16Len)

        switch normLang {
        case "diff":
            highlightDiff(code: code, into: nsAttr, palette: palette)
        case "r":
            if let regex = rRegex {
                for m in regex.matches(in: code, range: fullRange) {
                    if m.range(at: 1).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.comment, range: m.range(at: 1))
                    } else if m.range(at: 2).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.string, range: m.range(at: 2))
                    } else if m.range(at: 3).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.property, range: m.range(at: 3))
                    } else if m.range(at: 4).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.number, range: m.range(at: 4))
                    } else if m.range(at: 5).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 5))
                    } else if m.range(at: 6).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.keyword, range: m.range(at: 6))
                    } else if m.range(at: 7).location != NSNotFound {
                        nsAttr.addAttribute(.foregroundColor, value: palette.function, range: m.range(at: 7))
                    }
                }
            }
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
        cache.setObject(Entry(source: code, attributed: result), forKey: cacheKey)
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
        case "r", "rmd", "rmarkdown", "rprofile", "renviron", "namespace":
            return "r"
        default:
            return "code"
        }
    }
}
